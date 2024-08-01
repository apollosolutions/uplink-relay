package util

import (
	"fmt"
	"strings"
)

// Parses the graph_ref into graphID and variantID.
func ParseGraphRef(graphRef string) (string, string, error) {
	graphParts := strings.Split(graphRef, "@")
	if len(graphParts) != 2 {
		return "", "", fmt.Errorf("invalid graph_ref: %s", graphRef)
	}
	return graphParts[0], graphParts[1], nil
}
