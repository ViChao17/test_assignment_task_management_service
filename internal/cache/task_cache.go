// Package cache — кеш списков задач в Redis.
package cache

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"

	"taskmanager/internal/model"
)

// TaskCache хранит сериализованные страницы списка задач.
type TaskCache interface {
	Version(ctx context.Context, teamID int64) (int64, error)
	Get(ctx context.Context, teamID, version int64, filter model.TaskFilter) ([]byte, error)
	Set(ctx context.Context, teamID, version int64, filter model.TaskFilter, payload []byte) error
	Invalidate(ctx context.Context, teamID int64) error
	Ping(ctx context.Context) error
	Close() error
}

type RedisTaskCache struct {
	client *redis.Client
	ttl    time.Duration
}

func NewRedisTaskCache(addr, password string, database int, ttl time.Duration) *RedisTaskCache {
	return &RedisTaskCache{
		client: redis.NewClient(&redis.Options{
			Addr:         addr,
			Password:     password,
			DB:           database,
			DialTimeout:  3 * time.Second,
			ReadTimeout:  2 * time.Second,
			WriteTimeout: 2 * time.Second,
			PoolSize:     20,
		}),
		ttl: ttl,
	}
}

func (c *RedisTaskCache) Version(ctx context.Context, teamID int64) (int64, error) {
	value, err := c.client.Get(ctx, versionKey(teamID)).Result()
	if errors.Is(err, redis.Nil) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	version, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse cache version: %w", err)
	}
	return version, nil
}

func (c *RedisTaskCache) Get(ctx context.Context, teamID, version int64, filter model.TaskFilter) ([]byte, error) {
	value, err := c.client.Get(ctx, listKey(teamID, version, filter)).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return value, nil
}

func (c *RedisTaskCache) Set(ctx context.Context, teamID, version int64, filter model.TaskFilter, payload []byte) error {
	return c.client.Set(ctx, listKey(teamID, version, filter), payload, c.ttl).Err()
}

// Invalidate атомарно начинает новое пространство ключей для команды.
// Старые страницы истекают по TTL и больше не читаются, так как версия входит в ключ.
func (c *RedisTaskCache) Invalidate(ctx context.Context, teamID int64) error {
	return c.client.Incr(ctx, versionKey(teamID)).Err()
}

func (c *RedisTaskCache) Ping(ctx context.Context) error {
	return c.client.Ping(ctx).Err()
}

func (c *RedisTaskCache) Close() error {
	return c.client.Close()
}

func versionKey(teamID int64) string {
	return fmt.Sprintf("tasks:version:%d", teamID)
}

func listKey(teamID, version int64, filter model.TaskFilter) string {
	assignee := "any"
	if filter.AssigneeID != nil {
		assignee = strconv.FormatInt(*filter.AssigneeID, 10)
	}
	raw := fmt.Sprintf("%d|%d|%s|%s|%d|%d", teamID, version, filter.Status, assignee, filter.Limit, filter.Offset)
	sum := sha256.Sum256([]byte(raw))
	return fmt.Sprintf("tasks:list:%d:%d:%x", teamID, version, sum[:16])
}

// NoopTaskCache используется в тестах и при отключённом кеше.
type NoopTaskCache struct{}

func (NoopTaskCache) Version(context.Context, int64) (int64, error) { return 0, nil }
func (NoopTaskCache) Get(context.Context, int64, int64, model.TaskFilter) ([]byte, error) {
	return nil, nil
}
func (NoopTaskCache) Set(context.Context, int64, int64, model.TaskFilter, []byte) error {
	return nil
}
func (NoopTaskCache) Invalidate(context.Context, int64) error { return nil }
func (NoopTaskCache) Ping(context.Context) error              { return nil }
func (NoopTaskCache) Close() error                            { return nil }
