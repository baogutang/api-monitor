package cache

import (
	"context"
	"encoding/json"
	"time"

	"github.com/redis/go-redis/v9"
)

type Cache struct {
	client *redis.Client
}

func New(addr, password string, db int) *Cache {
	return &Cache{client: redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})}
}

func (c *Cache) Ping(ctx context.Context) error {
	return c.client.Ping(ctx).Err()
}

func (c *Cache) Client() *redis.Client {
	return c.client
}

func (c *Cache) GetJSON(ctx context.Context, key string, out any) (bool, error) {
	value, err := c.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, json.Unmarshal([]byte(value), out)
}

func (c *Cache) SetJSON(ctx context.Context, key string, value any, ttl time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return c.client.Set(ctx, key, data, ttl).Err()
}

func (c *Cache) InvalidateConfig(ctx context.Context) error {
	return c.client.Publish(ctx, "api_monitor:config:invalidate", time.Now().UTC().Format(time.RFC3339Nano)).Err()
}

func (c *Cache) Close() error {
	return c.client.Close()
}
