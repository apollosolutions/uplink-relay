package tiered_cache

import (
	"apollosolutions/uplink-relay/cache"
	"log/slog"
)

const PERMISSIONS = 0644

type TieredCache struct {
	caches   []cache.Cache
	logger   *slog.Logger
	duration int
}

func NewTieredCache(caches []cache.Cache, logger *slog.Logger, duration int) (*TieredCache, error) {
	return &TieredCache{caches, logger, duration}, nil
}

func (c *TieredCache) Get(key string) ([]byte, bool) {
	/// Attempt to get the content from each cache in the order they were provided
	/// If the content is found in any cache, return it
	/// If the content is not found in any cache, return false
	missedCaches := []cache.Cache{}
	var updateContent []byte
	for index, cache := range c.caches {
		content, ok := cache.Get(key)
		c.logger.Debug("Got content from cache", "content", content, "ok", ok, "cache", cache.Name())
		if ok {
			if index > 0 {
				updateContent = content
			}
			return content, true
		} else {
			missedCaches = append(missedCaches, cache)
		}
	}
	if len(missedCaches) > 0 && len(updateContent) > 0 {
		go func() {
			for _, cache := range missedCaches {
				c.logger.Debug("Setting content into missed cache", "cache", cache, "cache", cache.Name())
				err := cache.Set(key, string(updateContent), c.duration)
				if err != nil {
					c.logger.Error("Failed to set content in cache", "err", err, "cache", cache.Name())
				}
			}
		}()
	}
	return nil, false
}

func (c *TieredCache) Set(key string, content string, duration int) error {
	/// Set the content in each cache in the order they were provided
	/// If an error occurs while setting the content in any cache, return the error after trying each cache
	/// This ensures that the content is set in all caches if possible instead of stopping at the first error
	var err error
	for _, cache := range c.caches {
		err = cache.Set(key, content, duration)
		if err != nil {
			c.logger.Error("Failed to set content in cache", "err", err, "cache", cache.Name())
		}
	}
	return err
}

func (c *TieredCache) DeleteWithPrefix(prefix string) error {
	var err error
	for _, cache := range c.caches {
		err = cache.DeleteWithPrefix(prefix)
		c.logger.Error("Failed to delete content from cache", "err", err, "cache", cache.Name())
	}
	return err
}

func (c *TieredCache) Name() string {
	return "Tiered"
}
