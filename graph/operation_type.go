package graph

import (
	"apollosolutions/uplink-relay/graph/model"
	"apollosolutions/uplink-relay/uplink"
)

var operationEnumMapping = map[model.OperationType]string{
	model.OperationTypeSchema:                 uplink.SupergraphQuery,
	model.OperationTypeEntitlement:            uplink.LicenseQuery,
	model.OperationTypePersistedQueryManifest: uplink.PersistedQueriesQuery,
}
