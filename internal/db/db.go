// Package db открывает MySQL и применяет встроенные миграции.
package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/go-sql-driver/mysql"
)

func Open(ctx context.Context, dsn string, timeout time.Duration) (*sql.DB, error) {
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse MYSQL_DSN: %w", err)
	}
	cfg.ParseTime = true
	cfg.Loc = time.UTC
	// golang-migrate выполняет каждый файл миграции одним Exec, а в файлах
	// несколько DDL-операторов, поэтому нужно разрешить multiStatements.
	cfg.MultiStatements = true
	if cfg.Params == nil {
		cfg.Params = map[string]string{}
	}
	// Сессия MySQL должна быть в UTC: время хранится в DATETIME.
	cfg.Params["time_zone"] = "'+00:00'"

	connector, err := mysql.NewConnector(cfg)
	if err != nil {
		return nil, fmt.Errorf("create mysql connector: %w", err)
	}
	database := sql.OpenDB(connector)
	database.SetMaxOpenConns(25)
	database.SetMaxIdleConns(10)
	database.SetConnMaxLifetime(30 * time.Minute)
	database.SetConnMaxIdleTime(5 * time.Minute)

	deadline := time.Now().Add(timeout)
	for {
		pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		err = database.PingContext(pingCtx)
		cancel()
		if err == nil {
			return database, nil
		}
		if time.Now().After(deadline) {
			_ = database.Close()
			return nil, fmt.Errorf("connect to mysql: %w", err)
		}
		select {
		case <-ctx.Done():
			_ = database.Close()
			return nil, ctx.Err()
		case <-time.After(time.Second):
		}
	}
}
