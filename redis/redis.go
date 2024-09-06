package redis

import (
	"fmt"
	"time"

	"github.com/go-redis/redis"
)

type RedisCache struct {
	client *redis.Client
}

func NewRedisCache(client *redis.Client) *RedisCache {
	return &RedisCache{client: client}
}

func (c *RedisCache) Get(key string) ([]byte, bool) {
	val, err := c.client.Get(key).Result()
	if err == redis.Nil {
		return nil, false
	} else if err != nil {
		return nil, false
	}
	return []byte(val), true
}

func (c *RedisCache) Set(key string, content string, duration int) error {
	var expiration time.Duration
	if duration == -1 {
		expiration = 0 // 0 means the key has no expiration time
	} else {
		expiration = time.Duration(duration) * time.Second
	}
	err := c.client.Set(key, content, expiration).Err()
	if err != nil {
		return fmt.Errorf("failed to set key %s: %v", key, err)
	}
	return nil
}

func (c *RedisCache) DeleteWithPrefix(prefix string) error {
	// Delete all keys with the given prefix from the cache.
	// Redis provides no way to delete multiple keys at once, so we have to first get all keys with the given prefix
	keys := c.client.Keys(prefix + "*").Val()

	// If there are no keys with the given prefix, we can return early
	if len(keys) == 0 {
		return nil
	}

	// and then we can delete them all at once.
	res := c.client.Del(keys...)
	if res.Err() != nil {
		return fmt.Errorf("failed to delete keys with prefix %s: %v", prefix, res.Err())
	}
	return nil
}

func (c *RedisCache) Name() string {
	return "Redis"
}
