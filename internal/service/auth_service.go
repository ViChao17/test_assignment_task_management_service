package service

import (
	"context"
	"database/sql"
	"errors"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	"taskmanager/internal/app_errors"
	"taskmanager/internal/auth"
	"taskmanager/internal/model"
	"taskmanager/internal/store"
)

type AuthService struct {
	store  *store.Store
	tokens *auth.TokenManager
}

func NewAuthService(s *store.Store, tokens *auth.TokenManager) *AuthService {
	return &AuthService{store: s, tokens: tokens}
}

type RegisterInput struct {
	Email    string
	Password string
	Name     string
}

type LoginInput struct {
	Email    string
	Password string
}

type LoginResult struct {
	Token     string
	ExpiresAt time.Time
	User      *model.User
}

func (s *AuthService) Register(ctx context.Context, input RegisterInput) (*model.User, error) {
	email := validateEmail(input.Email)
	if err := checkEmail(email); err != nil {
		return nil, err
	}
	name, err := validateName(input.Name, 1, 120)
	if err != nil {
		return nil, err
	}
	if len(input.Password) < 8 || len(input.Password) > 72 {
		return nil, app_errors.BadRequest("password must contain between 8 and 72 bytes")
	}
	hash, err := auth.HashPassword(input.Password)
	if err != nil {
		return nil, app_errors.Internal(err)
	}
	user, err := s.store.CreateUser(ctx, email, hash, name)
	if err != nil {
		if errors.Is(err, store.ErrDuplicate) {
			return nil, app_errors.Conflict("email_taken", "an account with this email already exists")
		}
		return nil, app_errors.Internal(err)
	}
	return user, nil
}

func (s *AuthService) Login(ctx context.Context, input LoginInput) (*LoginResult, error) {
	email := validateEmail(input.Email)
	if email == "" || input.Password == "" {
		return nil, app_errors.Unauthorized("invalid email or password")
	}
	user, err := s.store.GetUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, app_errors.Unauthorized("invalid email or password")
		}
		return nil, app_errors.Internal(err)
	}
	if !auth.CheckPassword(user.PasswordHash, input.Password) {
		return nil, app_errors.Unauthorized("invalid email or password")
	}
	token, expiresAt, err := s.tokens.Issue(user.ID)
	if err != nil {
		return nil, app_errors.Internal(err)
	}
	return &LoginResult{Token: token, ExpiresAt: expiresAt, User: user}, nil
}

func checkEmail(email string) error {
	if email == "" || len(email) > 255 || strings.ContainsAny(email, " \t\r\n") {
		return app_errors.BadRequest("invalid email address")
	}
	parsed, err := mail.ParseAddress(email)
	if err != nil || parsed.Address != email {
		return app_errors.BadRequest("invalid email address")
	}
	if utf8.RuneCountInString(email) > 255 {
		return app_errors.BadRequest("invalid email address")
	}
	return nil
}

