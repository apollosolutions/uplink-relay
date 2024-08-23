package graph

// This file will be automatically regenerated based on the schema, any resolver implementations
// will be copied through when generating and any unknown code will be moved to the end.
// Code generated by github.com/99designs/gqlgen version v0.17.49

import (
	"apollosolutions/uplink-relay/cache"
	"apollosolutions/uplink-relay/config"
	"apollosolutions/uplink-relay/entitlements"
	"apollosolutions/uplink-relay/graph/model"
	"apollosolutions/uplink-relay/internal/util"
	persistedqueries "apollosolutions/uplink-relay/persisted_queries"
	"apollosolutions/uplink-relay/pinning"
	"apollosolutions/uplink-relay/schema"
	"apollosolutions/uplink-relay/uplink"
	"context"
	"fmt"
)

// DeleteCacheEntry is the resolver for the deleteCacheEntry field.
func (r *mutationResolver) DeleteCacheEntry(ctx context.Context, input model.DeleteCacheEntryInput) (*model.DeleteCacheEntryResult, error) {
	resolverContext := resolverContext(ctx)
	if resolverContext == nil {
		return nil, fmt.Errorf("error retrieving resolver context")
	}

	for _, operationName := range input.Operation {
		prefix := ""
		switch operationName {
		case model.OperationTypeSchema:
			prefix = cache.MakeCachePrefix(input.GraphRef, uplink.SupergraphQuery)
		case model.OperationTypeEntitlement:
			prefix = cache.MakeCachePrefix(input.GraphRef, uplink.LicenseQuery)
		case model.OperationTypePersistedQueryManifest:
			prefix = cache.MakeCachePrefix(input.GraphRef, uplink.PersistedQueriesQuery)
			graphID, _, err := util.ParseGraphRef(input.GraphRef)
			if err != nil {
				return nil, err
			}
			// we also need to delete the persisted query chunks
			err = resolverContext.SystemCache.DeleteWithPrefix(fmt.Sprintf("pq:%s/", graphID))
			if err != nil {
				return nil, err
			}
		default:
			return nil, fmt.Errorf("invalid operation type: %s", operationName)
		}
		err := resolverContext.SystemCache.DeleteWithPrefix(prefix)
		if err != nil {
			return nil, err
		}
	}
	return &model.DeleteCacheEntryResult{
		Success:       true,
		Configuration: resolverContext.GetConfigDetails(),
	}, nil
}

// PinSchema is the resolver for the pinSchema field.
func (r *mutationResolver) PinSchema(ctx context.Context, input model.PinSchemaInput) (*model.PinSchemaResult, error) {
	resolverContext := resolverContext(ctx)
	if resolverContext == nil {
		return nil, fmt.Errorf("error retrieving resolver context")
	}

	supergraphConfig, err := config.FindSupergraphConfigFromGraphRef(input.GraphRef, resolverContext.UserConfig)
	if err != nil {
		return nil, err
	}

	if supergraphConfig.LaunchID == input.LaunchID {
		return &model.PinSchemaResult{
			Success:       true,
			Configuration: resolverContext.GetConfigDetails(),
		}, nil
	}

	// Pin the schema
	err = pinning.PinLaunchID(resolverContext.UserConfig, resolverContext.Logger, resolverContext.SystemCache, input.LaunchID, input.GraphRef)
	if err != nil {
		return nil, err
	}
	return &model.PinSchemaResult{
		Success:       true,
		Configuration: resolverContext.GetConfigDetails(),
	}, nil
}

// PinPersistedQueryManifest is the resolver for the pinPersistedQueryManifest field.
func (r *mutationResolver) PinPersistedQueryManifest(ctx context.Context, input model.PinPersistedQueryManifestInput) (*model.PinPersistedQueryManifestResult, error) {
	resolverContext := resolverContext(ctx)
	if resolverContext == nil {
		return nil, fmt.Errorf("error retrieving resolver context")
	}

	supergraphConfig, err := config.FindSupergraphConfigFromGraphRef(input.GraphRef, resolverContext.UserConfig)
	if err != nil {
		return nil, err
	}

	if supergraphConfig.PersistedQueryVersion == input.ID {
		return &model.PinPersistedQueryManifestResult{
			Success:       true,
			Configuration: resolverContext.GetConfigDetails(),
		}, nil
	}

	// Pin the persisted query manifest
	err = pinning.PinPersistedQueries(resolverContext.UserConfig, resolverContext.Logger, resolverContext.SystemCache, input.GraphRef, input.ID)
	if err != nil {
		return nil, err
	}

	return &model.PinPersistedQueryManifestResult{
		Success:       true,
		Configuration: resolverContext.GetConfigDetails(),
	}, nil
}

// ForceUpdate is the resolver for the forceUpdate field.
func (r *mutationResolver) ForceUpdate(ctx context.Context, input model.ForceUpdateInput) (*model.ForceUpdateResult, error) {
	resolverContext := resolverContext(ctx)
	if resolverContext == nil {
		return nil, fmt.Errorf("error retrieving resolver context")
	}

	for _, operation := range input.Operations {
		switch operation {
		case model.OperationTypeSchema:
			err := schema.FetchSchema(resolverContext.UserConfig, resolverContext.SystemCache, resolverContext.Logger, input.GraphRef)
			if err != nil {
				return nil, err
			}
		case model.OperationTypeEntitlement:
			err := entitlements.FetchRouterLicense(resolverContext.UserConfig, resolverContext.SystemCache, resolverContext.Logger, input.GraphRef)
			if err != nil {
				return nil, err
			}
		case model.OperationTypePersistedQueryManifest:
			err := persistedqueries.FetchPQManifest(resolverContext.UserConfig, resolverContext.SystemCache, resolverContext.Logger, input.GraphRef, "")
			if err != nil {
				return nil, err
			}
		default:
			return nil, fmt.Errorf("invalid operation type: %s", operation)
		}
	}
	return &model.ForceUpdateResult{
		Success:       true,
		Configuration: resolverContext.GetConfigDetails(),
	}, nil
}

// Health is the resolver for the health field.
func (r *queryResolver) Health(ctx context.Context) (model.HealthStatus, error) {
	// TODO: check for artifacts in the cache when using pinned artifacts
	return model.HealthStatusOk, nil
}

// CurrentConfiguration is the resolver for the currentConfiguration field.
func (r *queryResolver) CurrentConfiguration(ctx context.Context) (*model.Configuration, error) {
	resolverContext := resolverContext(ctx)
	if resolverContext == nil {
		return nil, fmt.Errorf("error retrieving resolver context")
	}

	return resolverContext.GetConfigDetails(), nil
}

// Mutation returns MutationResolver implementation.
func (r *Resolver) Mutation() MutationResolver { return &mutationResolver{r} }

// Query returns QueryResolver implementation.
func (r *Resolver) Query() QueryResolver { return &queryResolver{r} }

type mutationResolver struct{ *Resolver }
type queryResolver struct{ *Resolver }
