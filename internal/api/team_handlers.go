package api

import (
	"net/http"

	"taskmanager/internal/service"
)

func (s *Server) handleCreateTeam(w http.ResponseWriter, r *http.Request) {
	var request createTeamRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, r, s.logger, err)
		return
	}
	team, err := s.teams.Create(r.Context(), userIDFromContext(r.Context()), request.Name)
	if err != nil {
		writeError(w, r, s.logger, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"team": team})
}

func (s *Server) handleListTeams(w http.ResponseWriter, r *http.Request) {
	teams, err := s.teams.List(r.Context(), userIDFromContext(r.Context()))
	if err != nil {
		writeError(w, r, s.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": teams})
}

func (s *Server) handleListMembers(w http.ResponseWriter, r *http.Request) {
	teamID, err := parseID(r, "id")
	if err != nil {
		writeError(w, r, s.logger, err)
		return
	}
	members, err := s.teams.ListMembers(r.Context(), userIDFromContext(r.Context()), teamID)
	if err != nil {
		writeError(w, r, s.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": members})
}

func (s *Server) handleInvite(w http.ResponseWriter, r *http.Request) {
	teamID, err := parseID(r, "id")
	if err != nil {
		writeError(w, r, s.logger, err)
		return
	}
	var request inviteRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, r, s.logger, err)
		return
	}
	member, err := s.teams.Invite(r.Context(), userIDFromContext(r.Context()), teamID, service.InviteInput{
		UserID: request.UserID,
		Role:   request.Role,
	})
	if err != nil {
		writeError(w, r, s.logger, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"member": member})
}

func (s *Server) handleUpdateMemberRole(w http.ResponseWriter, r *http.Request) {
	teamID, err := parseID(r, "id")
	if err != nil {
		writeError(w, r, s.logger, err)
		return
	}
	targetUserID, err := parseID(r, "user_id")
	if err != nil {
		writeError(w, r, s.logger, err)
		return
	}
	var request updateRoleRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, r, s.logger, err)
		return
	}
	member, err := s.teams.UpdateMemberRole(
		r.Context(),
		userIDFromContext(r.Context()),
		teamID,
		targetUserID,
		request.Role,
	)
	if err != nil {
		writeError(w, r, s.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"member": member})
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	teamID, err := parseID(r, "id")
	if err != nil {
		writeError(w, r, s.logger, err)
		return
	}
	stats, err := s.stats.TeamStats(r.Context(), userIDFromContext(r.Context()), teamID)
	if err != nil {
		writeError(w, r, s.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, stats)
}
