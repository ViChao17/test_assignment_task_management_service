package service

import (
	"strings"
	"unicode/utf8"

	"taskmanager/internal/app_errors"
)

func validateEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func validateName(name string, min, max int) (string, error) {
	name = strings.TrimSpace(name)
	length := utf8.RuneCountInString(name)
	if length < min || length > max {
		return "", app_errors.BadRequest("invalid name length")
	}
	return name, nil
}

func validateTitle(title string) (string, error) {
	title = strings.TrimSpace(title)
	length := utf8.RuneCountInString(title)
	if length < 1 || length > 255 {
		return "", app_errors.BadRequest("title must contain between 1 and 255 characters")
	}
	return title, nil
}

func validateDescription(description string) (string, error) {
	if utf8.RuneCountInString(description) > 20000 {
		return "", app_errors.BadRequest("description must not exceed 20000 characters")
	}
	return description, nil
}

func validateComment(content string) (string, error) {
	content = strings.TrimSpace(content)
	length := utf8.RuneCountInString(content)
	if length < 1 || length > 5000 {
		return "", app_errors.BadRequest("comment must contain between 1 and 5000 characters")
	}
	return content, nil
}

func validateTeamName(name string) (string, error) {
	name = strings.TrimSpace(name)
	length := utf8.RuneCountInString(name)
	if length < 2 || length > 120 {
		return "", app_errors.BadRequest("team name must contain between 2 and 120 characters")
	}
	return name, nil
}

