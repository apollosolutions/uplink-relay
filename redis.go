package main

import (
	"fmt"
	"log"
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
		log.Printf("Failed to get key %s: %v", key, err)
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
