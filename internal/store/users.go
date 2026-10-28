package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"taskmanager/internal/model"
)

func (s *Store) CreateUser(ctx context.Context, email, passwordHash, name string) (*model.User, error) {
	now := time.Now().UTC()
	result, err := s.q(ctx).ExecContext(ctx,
		`INSERT INTO users (email, password_hash, name, created_at) VALUES (?, ?, ?, ?)`,
		email, passwordHash, name, now,
	)
	if err != nil {
		if isDuplicateKey(err) {
			return nil, ErrDuplicate
		}
		return nil, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}
	return &model.User{
		ID:        id,
		Email:     email,
		Name:      name,
		CreatedAt: now,
	}, nil
}

func (s *Store) GetUserByEmail(ctx context.Context, email string) (*model.User, error) {
	return scanUser(s.q(ctx).QueryRowContext(ctx,
		`SELECT id, email, password_hash, name, created_at FROM users WHERE email = ?`,
		email,
	))
}

func (s *Store) GetUserByID(ctx context.Context, id int64) (*model.User, error) {
	return scanUser(s.q(ctx).QueryRowContext(ctx,
		`SELECT id, email, password_hash, name, created_at FROM users WHERE id = ?`,
		id,
	))
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanUser(row rowScanner) (*model.User, error) {
	var user model.User
	if err := row.Scan(&user.ID, &user.Email, &user.PasswordHash, &user.Name, &user.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, sql.ErrNoRows
		}
		return nil, err
	}
	return &user, nil
}
