package validate

import (
	"encoding/json"
	"net"
	"testing"

	"github.com/Azure/azure-container-networking/cns/restserver"
)

const testMACAddress = "00-11-22-33-44-55"

func TestHNSStateFileIPs(t *testing.T) {
	tests := []struct {
		name      string
		result    string
		wantIPs   map[string]string
		wantError bool
	}{
		{
			name:      "empty output",
			result:    "",
			wantError: true,
		},
		{
			name:      "whitespace output",
			result:    " \r\n\t",
			wantError: true,
		},
		{
			name:   "single endpoint",
			result: `{"MacAddress":"` + testMACAddress + `","IPAddress":"10.0.0.4","IPv6Address":"fd00::4"}`,
			wantIPs: map[string]string{
				"10.0.0.4": testMACAddress,
				"fd00::4":  testMACAddress,
			},
		},
		{
			name: "endpoint list excludes remote endpoints",
			result: `[
				{"MacAddress":"` + testMACAddress + `","IPAddress":"10.0.0.4"},
				{"MacAddress":"00-11-22-33-44-66","IPAddress":"10.0.0.5","IsRemoteEndpoint":true}
			]`,
			wantIPs: map[string]string{
				"10.0.0.4": testMACAddress,
			},
		},
		{
			name:      "malformed output",
			result:    "not-json",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := hnsStateFileIPs([]byte(tt.result))
			if tt.wantError {
				if err == nil {
					t.Fatal("expected an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("hnsStateFileIPs() error = %v", err)
			}
			if len(got) != len(tt.wantIPs) {
				t.Fatalf("hnsStateFileIPs() returned %v, want %v", got, tt.wantIPs)
			}
			for ip, mac := range tt.wantIPs {
				if got[ip] != mac {
					t.Errorf("hnsStateFileIPs()[%q] = %q, want %q", ip, got[ip], mac)
				}
			}
		})
	}
}

func mustMarshalCNSManagedState(t *testing.T) []byte {
	t.Helper()

	state := CnsManagedState{
		Endpoints: map[string]restserver.EndpointInfo{
			"endpoint-1": {
				PodName:      "test-pod",
				PodNamespace: "default",
				IfnameToIPMap: map[string]*restserver.IPInfo{
					"eth0": {
						IPv4: []net.IPNet{{IP: net.ParseIP("10.0.0.5"), Mask: net.CIDRMask(24, 32)}},
						IPv6: []net.IPNet{{IP: net.ParseIP("fd00::5"), Mask: net.CIDRMask(64, 128)}},
					},
				},
			},
		},
	}

	out, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("failed to marshal cns managed state: %v", err)
	}
	return out
}

// State validation compares the CNS endpoint state against every address in
// Pod.Status.PodIPs, which includes IPv6 for dual-stack pods. A single-family parser
// under-reports and fails validation, so the dual-stack scenario needs its own parser.
func TestCNSManagedStateFileIPFamilies(t *testing.T) {
	state := mustMarshalCNSManagedState(t)

	tests := []struct {
		name    string
		parser  func([]byte) (map[string]string, error)
		wantIPs []string
	}{
		{
			name:    "single stack parser records only ipv4",
			parser:  cnsManagedStateFileIps,
			wantIPs: []string{"10.0.0.5"},
		},
		{
			name:    "dualstack parser records both families",
			parser:  cnsManagedStateFileDualStackIps,
			wantIPs: []string{"10.0.0.5", "fd00::5"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.parser(state)
			if err != nil {
				t.Fatalf("parser error = %v", err)
			}
			if len(got) != len(tt.wantIPs) {
				t.Fatalf("parser returned %v, want %v", got, tt.wantIPs)
			}
			for _, ip := range tt.wantIPs {
				if _, ok := got[ip]; !ok {
					t.Errorf("parser did not record %q, got %v", ip, got)
				}
			}
		})
	}
}

// Guards the CNI_TYPE values the Windows overlay pipeline templates pass to CreateValidator.
// An unknown key yields no checks, so validation would silently pass without testing anything.
func TestWindowsChecksMapPipelineKeys(t *testing.T) {
	for _, cniType := range []string{"cniv1", "cniv2", "stateless", "stateless_dualstack"} {
		if len(windowsChecksMap[cniType]) == 0 {
			t.Errorf("windowsChecksMap[%q] has no checks; a pipeline passing this CNI_TYPE would validate nothing", cniType)
		}
	}
}

// The dual-stack stateless scenario must not reuse the single-stack CNS endpoint parser.
func TestStatelessDualStackUsesDualStackCNSParser(t *testing.T) {
	state := mustMarshalCNSManagedState(t)

	var found bool
	for _, c := range windowsChecksMap["stateless_dualstack"] {
		if c.name != "cns" {
			continue
		}
		found = true
		got, err := c.stateFileIPs(state)
		if err != nil {
			t.Fatalf("cns check parser error = %v", err)
		}
		if _, ok := got["fd00::5"]; !ok {
			t.Errorf("cns check does not record IPv6 addresses, got %v", got)
		}
	}
	if !found {
		t.Fatal(`no "cns" check found in windowsChecksMap["stateless_dualstack"]`)
	}
}
