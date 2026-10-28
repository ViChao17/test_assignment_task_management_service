// Package app_errors содержит ошибки приложения, безопасные для возврата клиенту API.
package app_errors

import "fmt"

// Error — ожидаемая ошибка приложения с HTTP-статусом и стабильным кодом.
type Error struct {
	Status  int
	Code    string
	Message string
	Details any
	Err     error
}

func (e *Error) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	return e.Message
}

func (e *Error) Unwrap() error { return e.Err }

func New(status int, code, message string) *Error {
	return &Error{Status: status, Code: code, Message: message}
}

func WithDetails(err *Error, details any) *Error {
	err.Details = details
	return err
}

func BadRequest(message string) *Error {
	return New(400, "validation_error", message)
}

func Unauthorized(message string) *Error {
	if message == "" {
		message = "authentication required"
	}
	return New(401, "unauthorized", message)
}

func Forbidden(message string) *Error {
	if message == "" {
		message = "insufficient permissions"
	}
	return New(403, "forbidden", message)
}

func NotFound(message string) *Error {
	if message == "" {
		message = "resource not found"
	}
	return New(404, "not_found", message)
}

func Conflict(code, message string) *Error {
	if code == "" {
		code = "conflict"
	}
	return New(409, code, message)
}

func Unprocessable(code, message string) *Error {
	if code == "" {
		code = "unprocessable_entity"
	}
	return New(422, code, message)
}

func Internal(err error) *Error {
	return &Error{
		Status:  500,
		Code:    "internal_error",
		Message: "internal server error",
		Err:     err,
	}
}
