package db

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	mysqlmigrate "github.com/golang-migrate/migrate/v4/database/mysql"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

// migrationFiles содержит SQL-миграции. Пути в go:embed отсчитываются
// от этого файла, поэтому SQL лежит рядом с мигратором.
//
//go:embed migrations/*.sql
var migrationFiles embed.FS

// Migrate применяет все неприменённые встроенные миграции.
//
// Формат имен файлов миграций golang-migrate:
//
//	{version}_{name}.up.sql
//	{version}_{name}.down.sql
//
// Состояние хранится в schema_migrations. Повторный запуск безопасен:
// применённые версии пропускаются, migrate.ErrNoChange считается успехом.
func Migrate(ctx context.Context, database *sql.DB) (err error) {
	if err := ctx.Err(); err != nil {
		return err
	}

	sourceDriver, err := iofs.New(migrationFiles, "migrations")
	if err != nil {
		return fmt.Errorf("load embedded migrations: %w", err)
	}

	// Отдельное соединение вместо mysql.WithInstance: Close() последнего
	// закрывает весь *sql.DB, а *sql.Conn можно безопасно освободить.
	conn, err := database.Conn(ctx)
	if err != nil {
		return fmt.Errorf("acquire migration connection: %w", err)
	}

	databaseDriver, err := mysqlmigrate.WithConnection(ctx, conn, &mysqlmigrate.Config{})
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("prepare migration driver: %w", err)
	}

	migrator, err := migrate.NewWithInstance("iofs", sourceDriver, "mysql", databaseDriver)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("create migrator: %w", err)
	}
	defer func() {
		sourceErr, databaseErr := migrator.Close()
		if err == nil {
			switch {
			case sourceErr != nil:
				err = fmt.Errorf("close migration source: %w", sourceErr)
			case databaseErr != nil:
				err = fmt.Errorf("close migration database: %w", databaseErr)
			}
		}
	}()

	if err := migrator.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}
