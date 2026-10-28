package api

import (
	"net/http"

	"taskmanager/internal/service"
)

func (s *Server) handleCreateTask(w http.ResponseWriter, r *http.Request) {
	var request createTaskRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, r, s.logger, err)
		return
	}
	task, err := s.tasks.Create(r.Context(), userIDFromContext(r.Context()), service.CreateTaskInput{
		TeamID:      request.TeamID,
		Title:       request.Title,
		Description: request.Description,
		Status:      request.Status,
		AssigneeID:  request.AssigneeID,
	})
	if err != nil {
		writeError(w, r, s.logger, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"task": task})
}

func (s *Server) handleListTasks(w http.ResponseWriter, r *http.Request) {
	filter, err := parseTaskFilter(r)
	if err != nil {
		writeError(w, r, s.logger, err)
		return
	}
	result, err := s.tasks.List(r.Context(), userIDFromContext(r.Context()), filter)
	if err != nil {
		writeError(w, r, s.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleGetTask(w http.ResponseWriter, r *http.Request) {
	taskID, err := parseID(r, "id")
	if err != nil {
		writeError(w, r, s.logger, err)
		return
	}
	task, err := s.tasks.Get(r.Context(), userIDFromContext(r.Context()), taskID)
	if err != nil {
		writeError(w, r, s.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"task": task})
}

func (s *Server) handleUpdateTask(w http.ResponseWriter, r *http.Request) {
	taskID, err := parseID(r, "id")
	if err != nil {
		writeError(w, r, s.logger, err)
		return
	}
	var request updateTaskRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, r, s.logger, err)
		return
	}
	version := int64(0)
	if request.Version != nil {
		version = *request.Version
	}
	assigneeID, assigneeSet, err := request.assignee()
	if err != nil {
		writeError(w, r, s.logger, err)
		return
	}
	task, err := s.tasks.Update(r.Context(), userIDFromContext(r.Context()), taskID, service.UpdateTaskInput{
		Version:     version,
		Title:       request.Title,
		Description: request.Description,
		Status:      request.Status,
		AssigneeID:  assigneeID,
		AssigneeSet: assigneeSet,
	})
	if err != nil {
		writeError(w, r, s.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"task": task})
}

func (s *Server) handleTaskHistory(w http.ResponseWriter, r *http.Request) {
	taskID, err := parseID(r, "id")
	if err != nil {
		writeError(w, r, s.logger, err)
		return
	}
	history, err := s.tasks.History(r.Context(), userIDFromContext(r.Context()), taskID)
	if err != nil {
		writeError(w, r, s.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": history})
}

func (s *Server) handleCreateComment(w http.ResponseWriter, r *http.Request) {
	taskID, err := parseID(r, "id")
	if err != nil {
		writeError(w, r, s.logger, err)
		return
	}
	var request commentRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, r, s.logger, err)
		return
	}
	comment, err := s.tasks.AddComment(r.Context(), userIDFromContext(r.Context()), taskID, request.Content)
	if err != nil {
		writeError(w, r, s.logger, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"comment": comment})
}

func (s *Server) handleListComments(w http.ResponseWriter, r *http.Request) {
	taskID, err := parseID(r, "id")
	if err != nil {
		writeError(w, r, s.logger, err)
		return
	}
	comments, err := s.tasks.Comments(r.Context(), userIDFromContext(r.Context()), taskID)
	if err != nil {
		writeError(w, r, s.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": comments})
}
