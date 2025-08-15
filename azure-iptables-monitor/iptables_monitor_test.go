package main

import (
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

type MockFileLineReader struct {
	files map[string][]string
}

func NewMockFileLineReader() *MockFileLineReader {
	return &MockFileLineReader{
		files: make(map[string][]string),
	}
}

var ErrFileNotFound = errors.New("file not found")

func (m *MockFileLineReader) Read(filename string) ([]string, error) {
	if lines, exists := m.files[filename]; exists {
		return lines, nil
	}
	return nil, fmt.Errorf("reading file %q: %w", filename, ErrFileNotFound)
}

// MockIPTablesClient implements IPTablesClient for testing
type MockIPTablesClient struct {
	// rules is organized as: table -> chain -> []rules
	rules map[string]map[string][]string
}

// NewMockIPTablesClient creates a new mock client with empty rules
func NewMockIPTablesClient() *MockIPTablesClient {
	return &MockIPTablesClient{
		rules: make(map[string]map[string][]string),
	}
}

// ListChains returns all chain names for the given table
func (m *MockIPTablesClient) ListChains(table string) ([]string, error) {
	chains, exists := m.rules[table]
	if !exists {
		return []string{}, nil
	}

	chainNames := make([]string, 0, len(chains))
	for chainName := range chains {
		chainNames = append(chainNames, chainName)
	}
	return chainNames, nil
}

// List returns all rules for the given table and chain
func (m *MockIPTablesClient) List(table, chain string) ([]string, error) {
	tableChains, exists := m.rules[table]
	if !exists {
		return []string{}, nil
	}

	rules, exists := tableChains[chain]
	if !exists {
		return []string{}, nil
	}

	return rules, nil
}

func TestHasUnexpectedRules(t *testing.T) {
	testCases := []struct {
		name            string
		currentRules    []string
		allowedPatterns []string
		expected        bool // true if we expect one of our rules to not match our allowedPatterns
	}{
		{
			name:            "no rules, no patterns",
			currentRules:    []string{},
			allowedPatterns: []string{},
			expected:        false,
		},
		{
			name:         "all rules match patterns",
			currentRules: []string{"ACCEPT all -- anywhere anywhere", "DROP all -- 192.168.1.0/24 anywhere"},
			allowedPatterns: []string{
				"^ACCEPT.*anywhere.*anywhere$",
				"^DROP.*192\\.168\\..*",
			},
			expected: false,
		},
		{
			name:         "some rules don't match patterns",
			currentRules: []string{"ACCEPT all -- anywhere anywhere", "CUSTOM_RULE something unexpected"},
			allowedPatterns: []string{
				"^ACCEPT.*anywhere.*anywhere$",
			},
			expected: true,
		},
		{
			name:            "no patterns provided, rules exist",
			currentRules:    []string{"ACCEPT all -- anywhere anywhere"},
			allowedPatterns: []string{},
			expected:        true,
		},
		{
			name:         "invalid regex pattern",
			currentRules: []string{"ACCEPT all -- anywhere anywhere"},
			allowedPatterns: []string{
				"^ACCEPT.*anywhere.*anywhere$",
				"[invalid regex", // This will fail to compile
			},
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := hasUnexpectedRules(tc.currentRules, tc.allowedPatterns)
			require.Equal(t, tc.expected, result, "hasUnexpectedRules result mismatch")
		})
	}
}

func TestNodeHasUserIPTablesRules(t *testing.T) {
	testCases := []struct {
		name        string
		files       map[string][]string
		rules       map[string]map[string][]string
		expected    bool
		description string
	}{
		{
			name: "no unexpected rules",
			files: map[string][]string{
				filepath.Join("/etc/config/", "global"): { // nolint
					"^-A.*INPUT.*lo.*",
				},
				filepath.Join("/etc/config/", "nat"): { // nolint
					"^-A.*MASQUERADE.*",
				},
			},
			rules: map[string]map[string][]string{
				"nat": {
					"POSTROUTING": {
						"-A POSTROUTING -s 10.0.0.0/8 -j MASQUERADE",
					},
					"INPUT": {
						"-A INPUT -i lo -j ACCEPT",
					},
				},
				"filter": {
					"INPUT": {
						"-A INPUT -i lo -j ACCEPT",
					},
				},
			},
			expected:    false,
			description: "all rules match expected patterns",
		},
		{
			name: "has unexpected rules",
			files: map[string][]string{
				filepath.Join("/etc/config/", "nat"): { // nolint
					"^-A.*CUSTOM_CHAIN.*",
				},
			},
			rules: map[string]map[string][]string{
				"nat": {
					"CUSTOM_CHAIN": {
						"-A CUSTOM_CHAIN -j DROP",
					},
				},
				"filter": {
					"CUSTOM_CHAIN": {
						"-A CUSTOM_CHAIN -j DROP", // This won't match any pattern
					},
				},
			},
			expected:    true,
			description: "unexpected custom rule found",
		},
		{
			name:  "no pattern files exist",
			files: map[string][]string{},
			rules: map[string]map[string][]string{
				"nat": {
					"POSTROUTING": {
						"-A POSTROUTING -j MASQUERADE",
					},
				},
			},
			expected:    true,
			description: "no patterns means all rules are unexpected",
		},
		{
			name: "empty iptables rules",
			files: map[string][]string{
				filepath.Join("/etc/config/", "global"): { // nolint
					"^-A.*ACCEPT.*",
				},
			},
			rules:       map[string]map[string][]string{},
			expected:    false,
			description: "no rules means no unexpected rules",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fileReader := NewMockFileLineReader()
			iptablesClient := NewMockIPTablesClient()

			fileReader.files = tc.files
			iptablesClient.rules = tc.rules

			result := nodeHasUserIPTablesRules(fileReader, "/etc/config/", iptablesClient)
			require.Equal(t, tc.expected, result, tc.description)
		})
	}
}
