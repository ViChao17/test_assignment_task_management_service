// Package store содержит SQL-код хранения данных.
package store

import (
	"context"
	"database/sql"
	"errors"

	"github.com/go-sql-driver/mysql"
)

var (
	ErrDuplicate       = errors.New("duplicate key")
	ErrVersionConflict = errors.New("task version conflict")
)

type Store struct {
	db *sql.DB
}

func New(database *sql.DB) *Store {
	return &Store{db: database}
}

func (s *Store) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

type dbTX interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

type txContextKey struct{}

// WithTx выполняет fn в транзакции. Вложенные вызовы используют текущую транзакцию.
func (s *Store) WithTx(ctx context.Context, fn func(context.Context) error) (err error) {
	if _, ok := ctx.Value(txContextKey{}).(*sql.Tx); ok {
		return fn(ctx)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	txCtx := context.WithValue(ctx, txContextKey{}, tx)
	if err = fn(txCtx); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	return nil
}

func (s *Store) q(ctx context.Context) dbTX {
	if tx, ok := ctx.Value(txContextKey{}).(*sql.Tx); ok {
		return tx
	}
	return s.db
}

func isDuplicateKey(err error) bool {
	var mysqlErr *mysql.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == 1062
}
