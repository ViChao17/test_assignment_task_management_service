package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"taskmanager/internal/app_errors"
	"taskmanager/internal/model"
)

type registerRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type createTeamRequest struct {
	Name string `json:"name"`
}

type inviteRequest struct {
	UserID int64  `json:"user_id"`
	Role   string `json:"role"`
}

type updateRoleRequest struct {
	Role string `json:"role"`
}

type createTaskRequest struct {
	TeamID      int64  `json:"team_id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Status      string `json:"status"`
	AssigneeID  *int64 `json:"assignee_id"`
}

type updateTaskRequest struct {
	Version     *int64          `json:"version"`
	Title       *string         `json:"title"`
	Description *string         `json:"description"`
	Status      *string         `json:"status"`
	AssigneeID  json.RawMessage `json:"assignee_id"`
}

// assignee разбирает необязательное поле assignee_id. Второй результат
// показывает, было ли поле в запросе, чтобы отличить явный null от его отсутствия.
func (r updateTaskRequest) assignee() (*int64, bool, error) {
	if len(r.AssigneeID) == 0 {
		return nil, false, nil
	}
	if bytes.Equal(bytes.TrimSpace(r.AssigneeID), []byte("null")) {
		return nil, true, nil
	}

	var value int64
	if err := json.Unmarshal(r.AssigneeID, &value); err != nil || value <= 0 {
		return nil, true, app_errors.BadRequest("assignee_id must be a positive integer or null")
	}
	return &value, true, nil
}

type commentRequest struct {
	Content string `json:"content"`
}

func parseID(r *http.Request, name string) (int64, error) {
	raw := r.PathValue(name)
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		return 0, app_errors.BadRequest("path identifier must be a positive integer")
	}
	return value, nil
}

func parseTaskFilter(r *http.Request) (model.TaskFilter, error) {
	query := r.URL.Query()
	rawTeamID := strings.TrimSpace(query.Get("team_id"))
	if rawTeamID == "" {
		return model.TaskFilter{}, app_errors.BadRequest("team_id query parameter is required")
	}
	teamID, err := strconv.ParseInt(rawTeamID, 10, 64)
	if err != nil || teamID <= 0 {
		return model.TaskFilter{}, app_errors.BadRequest("team_id must be a positive integer")
	}

	filter := model.TaskFilter{TeamID: teamID, Status: strings.TrimSpace(query.Get("status")), Limit: 20}
	if raw := strings.TrimSpace(query.Get("assignee_id")); raw != "" {
		assigneeID, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || assigneeID <= 0 {
			return model.TaskFilter{}, app_errors.BadRequest("assignee_id must be a positive integer")
		}
		filter.AssigneeID = &assigneeID
	}
	if raw := strings.TrimSpace(query.Get("limit")); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil {
			return model.TaskFilter{}, app_errors.BadRequest("limit must be an integer")
		}
		filter.Limit = limit
	}
	if raw := strings.TrimSpace(query.Get("offset")); raw != "" {
		offset, err := strconv.Atoi(raw)
		if err != nil {
			return model.TaskFilter{}, app_errors.BadRequest("offset must be an integer")
		}
		filter.Offset = offset
	}
	return filter, nil
}
