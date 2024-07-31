package pinning

import (
	"apollosolutions/uplink-relay/cache"
	"apollosolutions/uplink-relay/config"
	"apollosolutions/uplink-relay/internal/util"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/go-jose/go-jose"
)

// This isn't a complete set of the payload, but we only need WarnAt for now
type LicenseJWTPayload struct {
	WarnAt int64 `json:"warnAt"`
}

// PinOfflineLicense stores the license in the cache
func PinOfflineLicense(userConfig *config.Config, logger *slog.Logger, systemCache cache.Cache, license string, graphref string) error {
	logger.Debug("Pinning license", "graphRef", graphref)

	// Parse the JWT and extract the warnAt timestamp and subtract 30 days for the modified time
	// This just ensures the modifiedAt is properly in the past and statically set to avoid new pods creating new license entries for the same license
	token, err := jose.ParseSigned(license)
	if err != nil {
		logger.Error("Failed to parse license", "error", err)
		return err
	}

	var claims LicenseJWTPayload

	payload := token.UnsafePayloadWithoutVerification()
	if err := json.Unmarshal(payload, &claims); err != nil {
		logger.Error("Failed to unmarshal license claims", "error", err)
		return err
	}
	warnAt := time.Unix(claims.WarnAt, 0).UTC()
	modifiedTime := warnAt.AddDate(0, 0, -30)

	graphID, variantID, err := util.ParseGraphRef(graphref)
	if err != nil {
		logger.Error("Failed to parse GraphRef", "graphRef", graphref)
		return err
	}

	// Store the core schema in the cache
	if userConfig.Cache.Enabled {
		cacheEntry := cache.CacheItem{
			Expiration: modifiedTime,
			Content:    []byte(license),
		}
		cacheString, err := json.Marshal(cacheEntry)
		if err != nil {
			logger.Error("Failed to marshal cache entry", "error", err)
			return err
		}
		cacheKey := cache.MakeCacheKey(graphID, variantID, LicensePinned)
		insertPinnedCacheEntry(logger, systemCache, cacheKey, string(cacheString[:]), modifiedTime)
	}
	return nil
}
