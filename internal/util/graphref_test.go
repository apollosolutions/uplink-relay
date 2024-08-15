package util

import (
	"testing"
)

func TestParseGraphRef(t *testing.T) {
	tests := []struct {
		graphRef          string
		expectedGraphID   string
		expectedVariantID string
		expectedError     bool
	}{
		{
			graphRef:          "graphID@variantID",
			expectedGraphID:   "graphID",
			expectedVariantID: "variantID",
			expectedError:     false,
		},
		{
			graphRef:          "graphID",
			expectedGraphID:   "",
			expectedVariantID: "",
			expectedError:     true,
		},
	}

	for _, test := range tests {
		graphID, variantID, err := ParseGraphRef(test.graphRef)

		if graphID != test.expectedGraphID {
			t.Errorf("Expected graph ID: %s, but got: %s", test.expectedGraphID, graphID)
		}

		if variantID != test.expectedVariantID {
			t.Errorf("Expected variant ID: %s, but got: %s", test.expectedVariantID, variantID)
		}

		if test.expectedError && err == nil {
			t.Error("Expected an error but got nil")
		}

		if !test.expectedError && err != nil {
			t.Errorf("Expected no error but got: %s", err)
		}
	}
}
