package api

import (
	"net/http"
	"time"

	"taskmanager/internal/service"
)

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var request registerRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, r, s.logger, err)
		return
	}
	user, err := s.auth.Register(r.Context(), service.RegisterInput{
		Email:    request.Email,
		Password: request.Password,
		Name:     request.Name,
	})
	if err != nil {
		writeError(w, r, s.logger, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"user": user})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var request loginRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, r, s.logger, err)
		return
	}
	result, err := s.auth.Login(r.Context(), service.LoginInput{
		Email:    request.Email,
		Password: request.Password,
	})
	if err != nil {
		writeError(w, r, s.logger, err)
		return
	}
	expiresIn := int64(time.Until(result.ExpiresAt).Seconds())
	if expiresIn < 0 {
		expiresIn = 0
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"token":      result.Token,
		"token_type": "Bearer",
		"expires_at": result.ExpiresAt,
		"expires_in": expiresIn,
		"user":       result.User,
	})
}
