package api

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"taskmanager/internal/app_errors"
)

type errorEnvelope struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Details   any    `json:"details,omitempty"`
	RequestID string `json:"request_id,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if payload == nil {
		return
	}
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, err error) {
	var appErr *app_errors.Error
	if !errors.As(err, &appErr) {
		appErr = app_errors.Internal(err)
	}
	if appErr.Status >= 500 && logger != nil {
		logger.Error("request failed",
			"code", appErr.Code,
			"error", appErr.Error(),
			"request_id", requestIDFromContext(r.Context()),
		)
	}
	writeJSON(w, appErr.Status, errorEnvelope{Error: errorBody{
		Code:      appErr.Code,
		Message:   appErr.Message,
		Details:   appErr.Details,
		RequestID: requestIDFromContext(r.Context()),
	}})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, destination any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			return app_errors.New(http.StatusRequestEntityTooLarge, "payload_too_large", "request body is too large")
		}
		if errors.Is(err, io.EOF) {
			return app_errors.BadRequest("request body must contain a JSON object")
		}
		message := err.Error()
		if strings.Contains(message, "unknown field") {
			return app_errors.BadRequest(message)
		}
		return app_errors.BadRequest("invalid JSON body")
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return app_errors.BadRequest("request body must contain exactly one JSON object")
	}
	return nil
}


