package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"taskmanager/internal/model"
)

func (s *Store) InsertTask(ctx context.Context, task *model.Task) error {
	result, err := s.q(ctx).ExecContext(ctx, `
		INSERT INTO tasks
			(team_id, title, description, status, created_by, assignee_id,
			 created_at, updated_at, closed_at, version)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		task.TeamID, task.Title, task.Description, task.Status, task.CreatedBy, task.AssigneeID,
		task.CreatedAt, task.UpdatedAt, task.ClosedAt, task.Version,
	)
	if err != nil {
		return err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	task.ID = id
	return nil
}

func (s *Store) LockTask(ctx context.Context, taskID int64) (*model.Task, error) {
	var task model.Task
	var assigneeID sql.NullInt64
	var closedAt sql.NullTime
	err := s.q(ctx).QueryRowContext(ctx, `
		SELECT id, team_id, title, description, status, created_by, assignee_id,
		       created_at, updated_at, closed_at, version
		FROM tasks
		WHERE id = ?
		FOR UPDATE
	`, taskID).Scan(
		&task.ID, &task.TeamID, &task.Title, &task.Description, &task.Status, &task.CreatedBy,
		&assigneeID, &task.CreatedAt, &task.UpdatedAt, &closedAt, &task.Version,
	)
	if err != nil {
		return nil, err
	}
	if assigneeID.Valid {
		value := assigneeID.Int64
		task.AssigneeID = &value
	}
	if closedAt.Valid {
		value := closedAt.Time
		task.ClosedAt = &value
	}
	return &task, nil
}

func (s *Store) GetTask(ctx context.Context, taskID int64) (*model.Task, error) {
	return scanTaskFull(s.q(ctx).QueryRowContext(ctx, `
		SELECT t.id, t.team_id, t.title, t.description, t.status, t.created_by, creator.name,
		       t.assignee_id, assignee.name, t.created_at, t.updated_at, t.closed_at, t.version
		FROM tasks t
		INNER JOIN users creator ON creator.id = t.created_by
		LEFT JOIN users assignee ON assignee.id = t.assignee_id
		WHERE t.id = ?
	`, taskID))
}

func (s *Store) UpdateTask(ctx context.Context, task *model.Task, expectedVersion int64) error {
	result, err := s.q(ctx).ExecContext(ctx, `
		UPDATE tasks
		SET title = ?, description = ?, status = ?, assignee_id = ?,
		    updated_at = ?, closed_at = ?, version = ?
		WHERE id = ? AND version = ?
	`,
		task.Title, task.Description, task.Status, task.AssigneeID,
		task.UpdatedAt, task.ClosedAt, task.Version, task.ID, expectedVersion,
	)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return ErrVersionConflict
	}
	return nil
}

func (s *Store) ListTasks(ctx context.Context, filter model.TaskFilter) ([]model.Task, int, error) {
	conditions := []string{"t.team_id = ?"}
	args := []any{filter.TeamID}
	if filter.Status != "" {
		conditions = append(conditions, "t.status = ?")
		args = append(args, filter.Status)
	}
	if filter.AssigneeID != nil {
		conditions = append(conditions, "t.assignee_id = ?")
		args = append(args, *filter.AssigneeID)
	}
	where := strings.Join(conditions, " AND ")

	var total int
	if err := s.q(ctx).QueryRowContext(ctx,
		"SELECT COUNT(*) FROM tasks t WHERE "+where,
		args...,
	).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := `
		SELECT t.id, t.team_id, t.title, t.description, t.status, t.created_by, creator.name,
		       t.assignee_id, assignee.name, t.created_at, t.updated_at, t.closed_at, t.version
		FROM tasks t
		INNER JOIN users creator ON creator.id = t.created_by
		LEFT JOIN users assignee ON assignee.id = t.assignee_id
		WHERE ` + where + `
		ORDER BY t.created_at DESC, t.id DESC
		LIMIT ? OFFSET ?
	`
	listArgs := append(append([]any{}, args...), filter.Limit, filter.Offset)
	rows, err := s.q(ctx).QueryContext(ctx, query, listArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	tasks := make([]model.Task, 0)
	for rows.Next() {
		task, err := scanTaskFull(rows)
		if err != nil {
			return nil, 0, err
		}
		tasks = append(tasks, *task)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return tasks, total, nil
}

func scanTaskFull(row rowScanner) (*model.Task, error) {
	var task model.Task
	var assigneeID sql.NullInt64
	var assigneeName sql.NullString
	var closedAt sql.NullTime
	err := row.Scan(
		&task.ID, &task.TeamID, &task.Title, &task.Description, &task.Status,
		&task.CreatedBy, &task.CreatorName, &assigneeID, &assigneeName,
		&task.CreatedAt, &task.UpdatedAt, &closedAt, &task.Version,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, sql.ErrNoRows
		}
		return nil, err
	}
	if assigneeID.Valid {
		value := assigneeID.Int64
		task.AssigneeID = &value
	}
	if assigneeName.Valid {
		value := assigneeName.String
		task.AssigneeName = &value
	}
	if closedAt.Valid {
		value := closedAt.Time
		task.ClosedAt = &value
	}
	return &task, nil
}

func (s *Store) InsertTaskHistory(ctx context.Context, taskID, changedBy int64, changes []byte, createdAt time.Time) error {
	if !json.Valid(changes) {
		return fmt.Errorf("task history changes must be valid JSON")
	}
	_, err := s.q(ctx).ExecContext(ctx,
		`INSERT INTO task_history (task_id, changed_by, changes, created_at) VALUES (?, ?, ?, ?)`,
		taskID, changedBy, changes, createdAt,
	)
	return err
}

func (s *Store) ListTaskHistory(ctx context.Context, taskID int64) ([]model.TaskHistory, error) {
	rows, err := s.q(ctx).QueryContext(ctx, `
		SELECT h.id, h.task_id, h.changed_by, u.name, h.changes, h.created_at
		FROM task_history h
		INNER JOIN users u ON u.id = h.changed_by
		WHERE h.task_id = ?
		ORDER BY h.created_at ASC, h.id ASC
	`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	history := make([]model.TaskHistory, 0)
	for rows.Next() {
		var entry model.TaskHistory
		var changes []byte
		if err := rows.Scan(&entry.ID, &entry.TaskID, &entry.ChangedBy, &entry.ChangedByName, &changes, &entry.CreatedAt); err != nil {
			return nil, err
		}
		entry.Changes = json.RawMessage(changes)
		history = append(history, entry)
	}
	return history, rows.Err()
}

func (s *Store) InsertComment(ctx context.Context, comment *model.Comment) error {
	result, err := s.q(ctx).ExecContext(ctx,
		`INSERT INTO task_comments (task_id, user_id, content, created_at) VALUES (?, ?, ?, ?)`,
		comment.TaskID, comment.UserID, comment.Content, comment.CreatedAt,
	)
	if err != nil {
		return err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	comment.ID = id
	return nil
}

func (s *Store) ListComments(ctx context.Context, taskID int64) ([]model.Comment, error) {
	rows, err := s.q(ctx).QueryContext(ctx, `
		SELECT c.id, c.task_id, c.user_id, u.name, c.content, c.created_at
		FROM task_comments c
		INNER JOIN users u ON u.id = c.user_id
		WHERE c.task_id = ?
		ORDER BY c.created_at ASC, c.id ASC
	`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	comments := make([]model.Comment, 0)
	for rows.Next() {
		var comment model.Comment
		if err := rows.Scan(&comment.ID, &comment.TaskID, &comment.UserID, &comment.UserName, &comment.Content, &comment.CreatedAt); err != nil {
			return nil, err
		}
		comments = append(comments, comment)
	}
	return comments, rows.Err()
}
