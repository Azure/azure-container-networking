package main

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
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

// MockKubeClient for event handling
type MockKubeClient struct {
	Node  *corev1.Node
	Event *corev1.Event
	Error error
}

func NewMockKubeClient() *MockKubeClient {
	return &MockKubeClient{}
}

func (m *MockKubeClient) GetNode(_ context.Context, _ string) (*corev1.Node, error) {
	return m.Node, m.Error
}

func (m *MockKubeClient) CreateEvent(_ context.Context, _ string, _ *corev1.Event) (*corev1.Event, error) {
	return m.Event, m.Error
}

// MockDynamicClient for patching
type MockDynamicClient struct {
	Error error

	PatchCalls []PatchCall
}

type PatchCall struct {
	GVR       schema.GroupVersionResource
	Name      string
	PatchType types.PatchType
	Data      []byte
}

func NewMockDynamicClient() *MockDynamicClient {
	return &MockDynamicClient{
		PatchCalls: make([]PatchCall, 0),
	}
}

func (m *MockDynamicClient) PatchResource(_ context.Context, gvr schema.GroupVersionResource, name string, patchType types.PatchType, data []byte) error {
	m.PatchCalls = append(m.PatchCalls, PatchCall{
		GVR:       gvr,
		Name:      name,
		PatchType: patchType,
		Data:      data,
	})

	return m.Error
}

// MockEBPFClient for bpf map reading
type MockEBPFClient struct {
	Value uint64
	Error error

	MapValueCalls []string
}

func NewMockEBPFClient() *MockEBPFClient {
	return &MockEBPFClient{
		MapValueCalls: make([]string, 0),
	}
}

func (m *MockEBPFClient) GetBPFMapValue(pinPath string) (uint64, error) {
	m.MapValueCalls = append(m.MapValueCalls, pinPath)

	return m.Value, m.Error
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

func TestCheck(t *testing.T) {
	testCases := []struct {
		name                   string
		config                 Config
		setupMocks             func() Dependencies
		previousBlocks         uint64
		expectedResult         bool
		expectedPreviousBlocks uint64
		expectedPatchCalls     int
		expectedEBPFCalls      int
		expectedEventCalls     int
	}{
		{
			name: "no user rules found",
			config: Config{
				ConfigPath4: "/etc/config/ipv4",
				ConfigPath6: "/etc/config/ipv6",
				SendEvents:  true, // if no user iptables rules, no events sent
				IPv6Enabled: false,
				CheckMap:    false,
				NodeName:    "test-node",
			},
			setupMocks: func() Dependencies {
				fileReader := NewMockFileLineReader()
				fileReader.files["/etc/config/ipv4/filter"] = []string{"^-A INPUT -j ACCEPT$"}

				iptablesV4 := NewMockIPTablesClient()
				iptablesV4.rules = map[string]map[string][]string{
					"filter": {
						"INPUT": []string{"-A INPUT -j ACCEPT"},
					},
				}

				return Dependencies{
					KubeClient:    NewMockKubeClient(),
					DynamicClient: NewMockDynamicClient(),
					IPTablesV4:    iptablesV4,
					IPTablesV6:    NewMockIPTablesClient(),
					EBPFClient:    NewMockEBPFClient(),
					FileReader:    fileReader,
				}
			},
			previousBlocks:         0,
			expectedResult:         false,
			expectedPreviousBlocks: 0,
			expectedPatchCalls:     1,
			expectedEBPFCalls:      0,
			expectedEventCalls:     0,
		},
		{
			name: "user rules found with events enabled",
			config: Config{
				ConfigPath4: "/etc/config/ipv4",
				ConfigPath6: "/etc/config/ipv6",
				SendEvents:  true,
				IPv6Enabled: false,
				CheckMap:    false,
				NodeName:    "test-node",
			},
			setupMocks: func() Dependencies {
				fileReader := NewMockFileLineReader()
				fileReader.files["/etc/config/ipv4/filter"] = []string{"^-A INPUT -j ACCEPT$"} // Pattern won't match DROP

				iptablesV4 := NewMockIPTablesClient()
				iptablesV4.rules = map[string]map[string][]string{
					"filter": {
						"INPUT": []string{"-A INPUT -j DROP"}, // This won't match the pattern
					},
				}

				kubeClient := NewMockKubeClient()
				kubeClient.Node = &corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-node",
						UID:  "test-uid",
					},
				}
				kubeClient.Event = &corev1.Event{}

				return Dependencies{
					KubeClient:    kubeClient,
					DynamicClient: NewMockDynamicClient(),
					IPTablesV4:    iptablesV4,
					IPTablesV6:    NewMockIPTablesClient(),
					EBPFClient:    NewMockEBPFClient(),
					FileReader:    fileReader,
				}
			},
			previousBlocks:         0,
			expectedResult:         true,
			expectedPreviousBlocks: 0,
			expectedPatchCalls:     1,
			expectedEBPFCalls:      0,
			expectedEventCalls:     2, // GetNode + CreateEvent
		},
		{
			name: "ebpf map check with increased blocks",
			config: Config{
				ConfigPath4: "/etc/config/ipv4",
				ConfigPath6: "/etc/config/ipv6",
				SendEvents:  true,
				IPv6Enabled: false,
				CheckMap:    true,
				PinPath:     "/sys/fs/bpf/test",
				NodeName:    "test-node",
			},
			setupMocks: func() Dependencies {
				fileReader := NewMockFileLineReader()
				fileReader.files["/etc/config/ipv4/filter"] = []string{"^-A INPUT -j ACCEPT$"}

				iptablesV4 := NewMockIPTablesClient()
				iptablesV4.rules = map[string]map[string][]string{
					"filter": {
						"INPUT": []string{"-A INPUT -j DROP"}, // This won't match the pattern
					},
				}

				kubeClient := NewMockKubeClient()
				kubeClient.Node = &corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-node",
						UID:  "test-uid",
					},
				}
				kubeClient.Event = &corev1.Event{}

				ebpfClient := NewMockEBPFClient()
				ebpfClient.Value = 5 // More than previous blocks

				return Dependencies{
					KubeClient:    kubeClient,
					DynamicClient: NewMockDynamicClient(),
					IPTablesV4:    iptablesV4,
					IPTablesV6:    NewMockIPTablesClient(),
					EBPFClient:    ebpfClient,
					FileReader:    fileReader,
				}
			},
			previousBlocks:         2,
			expectedResult:         true,
			expectedPreviousBlocks: 5,
			expectedPatchCalls:     1,
			expectedEBPFCalls:      1,
			expectedEventCalls:     4, // GetNode + CreateEvent for blocked rules, GetNode + CreateEvent for user iptables rules
		},
		{
			name: "ipv6 enabled with user rules",
			config: Config{
				ConfigPath4: "/etc/config/ipv4",
				ConfigPath6: "/etc/config/ipv6",
				SendEvents:  false,
				IPv6Enabled: true,
				CheckMap:    false,
				NodeName:    "test-node",
			},
			setupMocks: func() Dependencies {
				fileReader := NewMockFileLineReader()
				fileReader.files["/etc/config/ipv4/filter"] = []string{"^-A INPUT -j ACCEPT$"}
				fileReader.files["/etc/config/ipv6/filter"] = []string{"^-A INPUT -j ACCEPT$"}

				iptablesV4 := NewMockIPTablesClient()
				iptablesV4.rules = map[string]map[string][]string{
					"filter": {
						"INPUT": []string{"-A INPUT -j ACCEPT"},
					},
				}

				iptablesV6 := NewMockIPTablesClient()
				iptablesV6.rules = map[string]map[string][]string{
					"filter": {
						"INPUT": []string{"-A INPUT -j DROP"}, // This won't match the pattern
					},
				}

				return Dependencies{
					KubeClient:    NewMockKubeClient(),
					DynamicClient: NewMockDynamicClient(),
					IPTablesV4:    iptablesV4,
					IPTablesV6:    iptablesV6,
					EBPFClient:    NewMockEBPFClient(),
					FileReader:    fileReader,
				}
			},
			previousBlocks:         0,
			expectedResult:         true,
			expectedPreviousBlocks: 0,
			expectedPatchCalls:     1,
			expectedEBPFCalls:      0,
			expectedEventCalls:     0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			deps := tc.setupMocks()
			previousBlocks := tc.previousBlocks

			result := Check(tc.config, deps, &previousBlocks)

			// Verify the result
			require.Equal(t, tc.expectedResult, result, "Check result should match expected")
			require.Equal(t, tc.expectedPreviousBlocks, previousBlocks, "Previous blocks should be updated correctly")

			// Verify mock calls
			mockDynamic := deps.DynamicClient.(*MockDynamicClient)
			require.Len(t, mockDynamic.PatchCalls, tc.expectedPatchCalls, "Expected PatchResource calls")

			if tc.expectedEBPFCalls > 0 {
				mockEBPF := deps.EBPFClient.(*MockEBPFClient)
				require.Len(t, mockEBPF.MapValueCalls, tc.expectedEBPFCalls, "Expected GetBPFMapValue calls")
				require.Equal(t, tc.config.PinPath, mockEBPF.MapValueCalls[0], "Expected correct pin path")
			}

			// Verify patch call details
			if tc.expectedPatchCalls > 0 {
				patchCall := mockDynamic.PatchCalls[0]
				require.Equal(t, "cilium.io", patchCall.GVR.Group, "Expected correct GVR group")
				require.Equal(t, "v2", patchCall.GVR.Version, "Expected correct GVR version")
				require.Equal(t, "ciliumnodes", patchCall.GVR.Resource, "Expected correct GVR resource")
				require.Equal(t, tc.config.NodeName, patchCall.Name, "Expected correct node name")
			}
		})
	}
}

func TestRunWithTerminateOnSuccess(t *testing.T) {
	config := Config{
		ConfigPath4:        "/etc/config/ipv4",
		ConfigPath6:        "/etc/config/ipv6",
		CheckInterval:      9999, // should not matter
		NodeName:           "test-node",
		TerminateOnSuccess: true,
	}

	fileReader := NewMockFileLineReader()
	fileReader.files["/etc/config/ipv4/filter"] = []string{"^-A INPUT -j ACCEPT$"}

	iptablesV4 := NewMockIPTablesClient()
	iptablesV4.rules = map[string]map[string][]string{
		"filter": {
			"INPUT": []string{"-A INPUT -j ACCEPT"}, // This matches the pattern
		},
	}

	deps := Dependencies{
		KubeClient:    NewMockKubeClient(),
		DynamicClient: NewMockDynamicClient(),
		IPTablesV4:    iptablesV4,
		IPTablesV6:    NewMockIPTablesClient(),
		EBPFClient:    NewMockEBPFClient(),
		FileReader:    fileReader,
	}

	Run(config, deps)

	// Verify that DynamicClient was called to patch the label
	mockDynamic := deps.DynamicClient.(*MockDynamicClient)
	require.Len(t, mockDynamic.PatchCalls, 1, "Expected one PatchResource call")
}
