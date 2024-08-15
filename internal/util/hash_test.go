package util

import (
	"fmt"
	"testing"
)

func TestHashString(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{input: "apollo", expected: "8d3aa1a6f227d714692a9d5a7fbbda496fb09f17f7207a11ffd0a4cca6cf35b7"},
		{input: "graphql", expected: "aed31c11881f20f3691240ece722db9c529d06e17515bb47208b1099c48591dc"},
		{input: "uplink", expected: "c2fa26d57cd69ed4cf86f1a9cf8f0232cc20b24387e3299aa267224722bd31ef"},
		{input: fmt.Sprintf("%v", map[string]interface{}{
			"graph_ref": "graph_ref", "ifAfterId": "",
		},
		), expected: "fa226ff5e5a9693c20844f39bb83c2d9312f74f4f90d4decc53de45c1512cabf"},
	}
	// Run tests
	for _, test := range tests {
		result := HashString(test.input)
		if result != test.expected {
			t.Errorf("HashString(%s) = %s, expected %s", test.input, result, test.expected)
		}
	}
}
