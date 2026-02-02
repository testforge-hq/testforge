package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/testforge/testforge/internal/config"
	"github.com/testforge/testforge/internal/domain"
)

// Cache provides Redis caching functionality
type Cache struct {
	client *redis.Client
}

// Key prefixes for different cache types
const (
	PrefixTenant   = "tenant:"
	PrefixProject  = "project:"
	PrefixTestRun  = "testrun:"
	PrefixSession  = "session:"
	PrefixRateLimit = "ratelimit:"
)

// Default TTLs
const (
	DefaultTTL      = 15 * time.Minute
	TenantTTL       = 1 * time.Hour
	SessionTTL      = 24 * time.Hour
	RateLimitWindow = 1 * time.Minute
)

// New creates a new Redis cache client
func New(cfg config.RedisConfig) (*Cache, error) {
	client := redis.NewClient(&redis.Options{
		Addr:         cfg.Addr(),
		Password:     cfg.Password,
		DB:           cfg.DB,
		PoolSize:     cfg.PoolSize,
		MinIdleConns: cfg.MinIdleConns,
		DialTimeout:  cfg.DialTimeout,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
	})

	// Verify connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("connecting to redis: %w", err)
	}

	return &Cache{client: client}, nil
}

// Close closes the Redis connection
func (c *Cache) Close() error {
	return c.client.Close()
}

// Health checks Redis connectivity
func (c *Cache) Health(ctx context.Context) error {
	return c.client.Ping(ctx).Err()
}

// Client returns the underlying Redis client for advanced operations
func (c *Cache) Client() *redis.Client {
	return c.client
}

// Tenant caching methods

// GetTenant retrieves a cached tenant
func (c *Cache) GetTenant(ctx context.Context, id uuid.UUID) (*domain.Tenant, error) {
	key := PrefixTenant + id.String()
	data, err := c.client.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, err
	}

	var tenant domain.Tenant
	if err := json.Unmarshal(data, &tenant); err != nil {
		return nil, err
	}

	return &tenant, nil
}

// SetTenant caches a tenant
func (c *Cache) SetTenant(ctx context.Context, tenant *domain.Tenant) error {
	key := PrefixTenant + tenant.ID.String()
	data, err := json.Marshal(tenant)
	if err != nil {
		return err
	}

	return c.client.Set(ctx, key, data, TenantTTL).Err()
}

// InvalidateTenant removes a tenant from cache
func (c *Cache) InvalidateTenant(ctx context.Context, id uuid.UUID) error {
	key := PrefixTenant + id.String()
	return c.client.Del(ctx, key).Err()
}

// GetTenantBySlug retrieves a cached tenant by slug
func (c *Cache) GetTenantBySlug(ctx context.Context, slug string) (*domain.Tenant, error) {
	key := PrefixTenant + "slug:" + slug
	data, err := c.client.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, err
	}

	var tenant domain.Tenant
	if err := json.Unmarshal(data, &tenant); err != nil {
		return nil, err
	}

	return &tenant, nil
}

// SetTenantBySlug caches a tenant by slug
func (c *Cache) SetTenantBySlug(ctx context.Context, tenant *domain.Tenant) error {
	key := PrefixTenant + "slug:" + tenant.Slug
	data, err := json.Marshal(tenant)
	if err != nil {
		return err
	}

	return c.client.Set(ctx, key, data, TenantTTL).Err()
}

// Test Run status caching

// GetTestRunStatus retrieves cached test run status
func (c *Cache) GetTestRunStatus(ctx context.Context, id uuid.UUID) (domain.RunStatus, error) {
	key := PrefixTestRun + id.String() + ":status"
	status, err := c.client.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return "", nil
		}
		return "", err
	}

	return domain.RunStatus(status), nil
}

// SetTestRunStatus caches test run status
func (c *Cache) SetTestRunStatus(ctx context.Context, id uuid.UUID, status domain.RunStatus) error {
	key := PrefixTestRun + id.String() + ":status"
	return c.client.Set(ctx, key, string(status), DefaultTTL).Err()
}

// Rate limiting

// CheckRateLimit checks and increments rate limit counter
func (c *Cache) CheckRateLimit(ctx context.Context, key string, limit int) (bool, int, error) {
	fullKey := PrefixRateLimit + key

	pipe := c.client.Pipeline()
	incr := pipe.Incr(ctx, fullKey)
	pipe.Expire(ctx, fullKey, RateLimitWindow)

	_, err := pipe.Exec(ctx)
	if err != nil {
		return false, 0, err
	}

	count := int(incr.Val())
	return count <= limit, count, nil
}

// GetRateLimitRemaining returns remaining rate limit
func (c *Cache) GetRateLimitRemaining(ctx context.Context, key string, limit int) (int, error) {
	fullKey := PrefixRateLimit + key
	count, err := c.client.Get(ctx, fullKey).Int()
	if err != nil {
		if err == redis.Nil {
			return limit, nil
		}
		return 0, err
	}

	remaining := limit - count
	if remaining < 0 {
		remaining = 0
	}

	return remaining, nil
}

// Session management

// SetSession stores a session
func (c *Cache) SetSession(ctx context.Context, sessionID string, data map[string]any) error {
	key := PrefixSession + sessionID
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	return c.client.Set(ctx, key, jsonData, SessionTTL).Err()
}

// GetSession retrieves a session
func (c *Cache) GetSession(ctx context.Context, sessionID string) (map[string]any, error) {
	key := PrefixSession + sessionID
	data, err := c.client.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, err
	}

	var session map[string]any
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, err
	}

	return session, nil
}

// DeleteSession removes a session
func (c *Cache) DeleteSession(ctx context.Context, sessionID string) error {
	key := PrefixSession + sessionID
	return c.client.Del(ctx, key).Err()
}

// Generic caching methods

// Get retrieves a value from cache
func (c *Cache) Get(ctx context.Context, key string) ([]byte, error) {
	data, err := c.client.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, err
	}
	return data, nil
}

// Set stores a value in cache
func (c *Cache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	return c.client.Set(ctx, key, value, ttl).Err()
}

// Delete removes a value from cache
func (c *Cache) Delete(ctx context.Context, key string) error {
	return c.client.Del(ctx, key).Err()
}

// DeletePattern removes all keys matching a pattern
func (c *Cache) DeletePattern(ctx context.Context, pattern string) error {
	iter := c.client.Scan(ctx, 0, pattern, 100).Iterator()
	var keys []string
	for iter.Next(ctx) {
		keys = append(keys, iter.Val())
	}
	if err := iter.Err(); err != nil {
		return err
	}

	if len(keys) > 0 {
		return c.client.Del(ctx, keys...).Err()
	}

	return nil
}

// Pub/Sub for real-time updates

// Publish publishes a message to a channel
func (c *Cache) Publish(ctx context.Context, channel string, message any) error {
	data, err := json.Marshal(message)
	if err != nil {
		return err
	}
	return c.client.Publish(ctx, channel, data).Err()
}

// Subscribe subscribes to a channel
func (c *Cache) Subscribe(ctx context.Context, channel string) *redis.PubSub {
	return c.client.Subscribe(ctx, channel)
}
