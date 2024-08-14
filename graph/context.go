package graph

import (
	"apollosolutions/uplink-relay/cache"
	"apollosolutions/uplink-relay/config"
	"context"
	"log/slog"
)

// This file will not be regenerated automatically.

type ResolverContext struct {
	Logger      *slog.Logger
	SystemCache cache.Cache
	UserConfig  *config.Config
}

type keyType string

const ResolverKey keyType = "resolver"

func resolverContext(ctx context.Context) *ResolverContext {
	return ctx.Value(ResolverKey).(*ResolverContext)
}
