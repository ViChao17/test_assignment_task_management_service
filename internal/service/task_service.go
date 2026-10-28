package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"taskmanager/internal/app_errors"
	"taskmanager/internal/cache"
	"taskmanager/internal/model"
	"taskmanager/internal/store"
)

type TaskService struct {
	store  *store.Store
	cache  cache.TaskCache
	logger *slog.Logger
}

func NewTaskService(s *store.Store, c cache.TaskCache, logger *slog.Logger) *TaskService {
	if logger == nil {
		logger = slog.Default()
	}
	return &TaskService{store: s, cache: c, logger: logger}
}

type CreateTaskInput struct {
	TeamID      int64
	Title       string
	Description string
	Status      string
	AssigneeID  *int64
}

type UpdateTaskInput struct {
	Version     int64
	Title       *string
	Description *string
	Status      *string
	AssigneeID  *int64
	// AssigneeSet показывает, было ли поле assignee_id в запросе.
	// AssigneeID == nil при AssigneeSet == true означает снятие исполнителя.
	AssigneeSet bool
}

func (s *TaskService) Create(ctx context.Context, actorID int64, input CreateTaskInput) (*model.Task, error) {
	if input.TeamID <= 0 {
		return nil, app_errors.BadRequest("team_id must be positive")
	}
	title, err := validateTitle(input.Title)
	if err != nil {
		return nil, err
	}
	description, err := validateDescription(input.Description)
	if err != nil {
		return nil, err
	}
	status := input.Status
	if status == "" {
		status = model.StatusTodo
	}
	if !model.IsValidStatus(status) {
		return nil, app_errors.BadRequest("status must be one of todo, in_progress, done")
	}
	if input.AssigneeID != nil && *input.AssigneeID <= 0 {
		return nil, app_errors.BadRequest("assignee_id must be positive")
	}

	now := time.Now().UTC()
	var created *model.Task
	err = s.store.WithTx(ctx, func(txCtx context.Context) error {
		if _, err := s.store.GetTeamMembership(txCtx, input.TeamID, actorID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return app_errors.Forbidden("you are not a member of this team")
			}
			return err
		}
		if input.AssigneeID != nil {
			ok, err := s.store.IsTeamMember(txCtx, input.TeamID, *input.AssigneeID)
			if err != nil {
				return err
			}
			if !ok {
				return app_errors.Unprocessable("invalid_assignee", "assignee must be a member of the task team")
			}
		}

		task := &model.Task{
			TeamID:      input.TeamID,
			Title:       title,
			Description: description,
			Status:      status,
			CreatedBy:   actorID,
			AssigneeID:  input.AssigneeID,
			CreatedAt:   now,
			UpdatedAt:   now,
			Version:     1,
		}
		if status == model.StatusDone {
			closedAt := now
			task.ClosedAt = &closedAt
		}
		if err := s.store.InsertTask(txCtx, task); err != nil {
			return err
		}
		changes, err := creationChanges(task)
		if err != nil {
			return err
		}
		if err := s.store.InsertTaskHistory(txCtx, task.ID, actorID, changes, now); err != nil {
			return err
		}
		created, err = s.store.GetTask(txCtx, task.ID)
		return err
	})
	if err != nil {
		var appErr *app_errors.Error
		if errors.As(err, &appErr) {
			return nil, err
		}
		return nil, app_errors.Internal(err)
	}

	s.invalidate(created.TeamID)
	return created, nil
}

func (s *TaskService) List(ctx context.Context, actorID int64, filter model.TaskFilter) (*model.TaskList, error) {
	if filter.TeamID <= 0 {
		return nil, app_errors.BadRequest("team_id is required")
	}
	if filter.Status != "" && !model.IsValidStatus(filter.Status) {
		return nil, app_errors.BadRequest("status must be one of todo, in_progress, done")
	}
	if filter.AssigneeID != nil && *filter.AssigneeID <= 0 {
		return nil, app_errors.BadRequest("assignee_id must be positive")
	}
	if filter.Limit < 1 || filter.Limit > 100 {
		return nil, app_errors.BadRequest("limit must be between 1 and 100")
	}
	if filter.Offset < 0 {
		return nil, app_errors.BadRequest("offset must not be negative")
	}

	if _, err := s.store.GetTeamMembership(ctx, filter.TeamID, actorID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, app_errors.Forbidden("you are not a member of this team")
		}
		return nil, app_errors.Internal(err)
	}

	version, cacheUsable := int64(0), true
	cachedVersion, err := s.cache.Version(ctx, filter.TeamID)
	if err != nil {
		s.logger.Warn("task cache version lookup failed", "team_id", filter.TeamID, "error", err)
		cacheUsable = false
	} else {
		version = cachedVersion
	}

	if cacheUsable {
		payload, err := s.cache.Get(ctx, filter.TeamID, version, filter)
		if err != nil {
			s.logger.Warn("task cache read failed", "team_id", filter.TeamID, "error", err)
			cacheUsable = false
		} else if payload != nil {
			var result model.TaskList
			if err := json.Unmarshal(payload, &result); err == nil {
				return &result, nil
			}
			s.logger.Warn("task cache contained invalid json", "team_id", filter.TeamID)
		}
	}

	tasks, total, err := s.store.ListTasks(ctx, filter)
	if err != nil {
		return nil, app_errors.Internal(err)
	}
	result := &model.TaskList{
		Items:  tasks,
		Total:  total,
		Limit:  filter.Limit,
		Offset: filter.Offset,
	}
	if cacheUsable {
		payload, err := json.Marshal(result)
		if err != nil {
			s.logger.Warn("failed to marshal task cache payload", "team_id", filter.TeamID, "error", err)
		} else if err := s.cache.Set(ctx, filter.TeamID, version, filter, payload); err != nil {
			s.logger.Warn("task cache write failed", "team_id", filter.TeamID, "error", err)
		}
	}
	return result, nil
}

func (s *TaskService) Get(ctx context.Context, actorID, taskID int64) (*model.Task, error) {
	task, _, err := s.accessibleTask(ctx, actorID, taskID)
	return task, err
}

func (s *TaskService) Update(ctx context.Context, actorID, taskID int64, input UpdateTaskInput) (*model.Task, error) {
	if input.Version <= 0 {
		return nil, app_errors.BadRequest("version is required and must be positive")
	}
	if input.Title != nil {
		title, err := validateTitle(*input.Title)
		if err != nil {
			return nil, err
		}
		input.Title = &title
	}
	if input.Description != nil {
		description, err := validateDescription(*input.Description)
		if err != nil {
			return nil, err
		}
		input.Description = &description
	}
	if input.Status != nil && !model.IsValidStatus(*input.Status) {
		return nil, app_errors.BadRequest("status must be one of todo, in_progress, done")
	}
	if input.AssigneeID != nil {
		// Внутренним вызовам достаточно задать ID, без отдельного флага.
		input.AssigneeSet = true
	}
	if input.AssigneeSet && input.AssigneeID != nil && *input.AssigneeID <= 0 {
		return nil, app_errors.BadRequest("assignee_id must be a positive integer or null")
	}
	mutation := TaskMutation{
		Title:       input.Title,
		Description: input.Description,
		Status:      input.Status,
		AssigneeID:  input.AssigneeID,
		AssigneeSet: input.AssigneeSet,
	}

	now := time.Now().UTC()
	var updated *model.Task
	changed := false
	err := s.store.WithTx(ctx, func(txCtx context.Context) error {
		task, err := s.store.LockTask(txCtx, taskID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return app_errors.NotFound("task not found")
			}
			return err
		}
		actor, err := s.store.GetTeamMembership(txCtx, task.TeamID, actorID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return app_errors.NotFound("task not found")
			}
			return err
		}
		if !CanEditTask(actor.Role, actorID, task, mutation) {
			return app_errors.Forbidden("you do not have permission to edit this task")
		}
		if task.Version != input.Version {
			return app_errors.WithDetails(
				app_errors.Conflict("version_conflict", "task was changed by another request"),
				map[string]any{"current_version": task.Version},
			)
		}
		if input.AssigneeSet && input.AssigneeID != nil {
			ok, err := s.store.IsTeamMember(txCtx, task.TeamID, *input.AssigneeID)
			if err != nil {
				return err
			}
			if !ok {
				return app_errors.Unprocessable("invalid_assignee", "assignee must be a member of the task team")
			}
		}

		next := *task
		if input.Title != nil {
			next.Title = *input.Title
		}
		if input.Description != nil {
			next.Description = *input.Description
		}
		if input.Status != nil {
			next.Status = *input.Status
		}
		if input.AssigneeSet {
			next.AssigneeID = input.AssigneeID
		}
		if next.Status == model.StatusDone {
			if task.Status != model.StatusDone || task.ClosedAt == nil {
				closedAt := now
				next.ClosedAt = &closedAt
			}
		} else {
			next.ClosedAt = nil
		}

		changes := map[string]any{"action": "updated"}
		if next.Title != task.Title {
			changes["title"] = fieldChange{Old: task.Title, New: next.Title}
		}
		if next.Description != task.Description {
			changes["description"] = fieldChange{Old: task.Description, New: next.Description}
		}
		if next.Status != task.Status {
			changes["status"] = fieldChange{Old: task.Status, New: next.Status}
		}
		if (next.AssigneeID == nil) != (task.AssigneeID == nil) || (next.AssigneeID != nil && *next.AssigneeID != *task.AssigneeID) {
			changes["assignee_id"] = fieldChange{Old: task.AssigneeID, New: next.AssigneeID}
		}
		if !sameTimePointer(next.ClosedAt, task.ClosedAt) {
			changes["closed_at"] = fieldChange{Old: task.ClosedAt, New: next.ClosedAt}
		}

		if len(changes) == 1 {
			updated, err = s.store.GetTask(txCtx, taskID)
			return err
		}

		changed = true
		next.Version = task.Version + 1
		next.UpdatedAt = now
		if err := s.store.UpdateTask(txCtx, &next, task.Version); err != nil {
			if errors.Is(err, store.ErrVersionConflict) {
				return app_errors.Conflict("version_conflict", "task was changed by another request")
			}
			return err
		}
		payload, err := json.Marshal(changes)
		if err != nil {
			return err
		}
		if err := s.store.InsertTaskHistory(txCtx, taskID, actorID, payload, now); err != nil {
			return err
		}
		updated, err = s.store.GetTask(txCtx, taskID)
		return err
	})
	if err != nil {
		var appErr *app_errors.Error
		if errors.As(err, &appErr) {
			return nil, err
		}
		return nil, app_errors.Internal(err)
	}
	if changed {
		s.invalidate(updated.TeamID)
	}
	return updated, nil
}

func (s *TaskService) History(ctx context.Context, actorID, taskID int64) ([]model.TaskHistory, error) {
	if _, _, err := s.accessibleTask(ctx, actorID, taskID); err != nil {
		return nil, err
	}
	history, err := s.store.ListTaskHistory(ctx, taskID)
	if err != nil {
		return nil, app_errors.Internal(err)
	}
	return history, nil
}

func (s *TaskService) AddComment(ctx context.Context, actorID, taskID int64, content string) (*model.Comment, error) {
	content, err := validateComment(content)
	if err != nil {
		return nil, err
	}
	_, member, err := s.accessibleTask(ctx, actorID, taskID)
	if err != nil {
		return nil, err
	}
	comment := &model.Comment{
		TaskID:    taskID,
		UserID:    actorID,
		UserName:  member.Name,
		Content:   content,
		CreatedAt: time.Now().UTC(),
	}
	if err := s.store.InsertComment(ctx, comment); err != nil {
		return nil, app_errors.Internal(err)
	}
	return comment, nil
}

func (s *TaskService) Comments(ctx context.Context, actorID, taskID int64) ([]model.Comment, error) {
	if _, _, err := s.accessibleTask(ctx, actorID, taskID); err != nil {
		return nil, err
	}
	comments, err := s.store.ListComments(ctx, taskID)
	if err != nil {
		return nil, app_errors.Internal(err)
	}
	return comments, nil
}

func (s *TaskService) accessibleTask(ctx context.Context, actorID, taskID int64) (*model.Task, *model.TeamMember, error) {
	task, err := s.store.GetTask(ctx, taskID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil, app_errors.NotFound("task not found")
		}
		return nil, nil, app_errors.Internal(err)
	}
	member, err := s.store.GetTeamMembership(ctx, task.TeamID, actorID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// Не раскрываем существование задачи в другой команде.
			return nil, nil, app_errors.NotFound("task not found")
		}
		return nil, nil, app_errors.Internal(err)
	}
	return task, member, nil
}

func (s *TaskService) invalidate(teamID int64) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(context.Background()), 3*time.Second)
	defer cancel()
	if err := s.cache.Invalidate(ctx, teamID); err != nil {
		s.logger.Error("task cache invalidation failed", "team_id", teamID, "error", err)
	}
}

type fieldChange struct {
	Old any `json:"old"`
	New any `json:"new"`
}

func creationChanges(task *model.Task) ([]byte, error) {
	changes := map[string]any{
		"action":      "created",
		"title":       fieldChange{Old: nil, New: task.Title},
		"description": fieldChange{Old: nil, New: task.Description},
		"status":      fieldChange{Old: nil, New: task.Status},
		"assignee_id": fieldChange{Old: nil, New: task.AssigneeID},
	}
	if task.ClosedAt != nil {
		changes["closed_at"] = fieldChange{Old: nil, New: task.ClosedAt}
	}
	return json.Marshal(changes)
}

func sameTimePointer(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Equal(*b)
}
