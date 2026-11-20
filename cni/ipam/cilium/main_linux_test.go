package main

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/containernetworking/cni/pkg/skel"
	"github.com/google/go-cmp/cmp"
)

func TestResultSerializationForAdvertisedVersions(t *testing.T) {
	type wireIP struct {
		Version string `json:"version,omitempty"`
		Address string `json:"address"`
		Gateway string `json:"gateway"`
	}
	type wireResult struct {
		CNIVersion string   `json:"cniVersion"`
		IPs        []wireIP `json:"ips"`
	}
	for _, version := range []string{"0.3.0", "0.3.1", "0.4.0", "1.0.0"} {
		t.Run(version, func(t *testing.T) {
			result, err := allocationResult("10.0.0.10", "10.0.0.0/24")
			if err != nil {
				t.Fatal(err)
			}
			converted, err := result.GetAsVersion(version)
			if err != nil {
				t.Fatal(err)
			}
			var output bytes.Buffer
			if printErr := converted.PrintTo(&output); printErr != nil {
				t.Fatal(printErr)
			}
			var got wireResult
			if decodeErr := json.Unmarshal(output.Bytes(), &got); decodeErr != nil {
				t.Fatal(decodeErr)
			}
			ip := wireIP{Address: "10.0.0.10/24", Gateway: "10.0.0.1"}
			if version != "1.0.0" {
				ip.Version = "4"
			}
			if diff := cmp.Diff(wireResult{CNIVersion: version, IPs: []wireIP{ip}}, got); diff != "" {
				t.Fatalf("serialized result differs (-want +got): %s", diff)
			}
		})
	}
}

func TestResultPreservesHostAddressAndSubnetMask(t *testing.T) {
	result, err := allocationResult("10.0.0.10", "10.0.0.0/24")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.IPs) != 1 || result.IPs[0].Address.String() != "10.0.0.10/24" ||
		result.IPs[0].Gateway.String() != "10.0.0.1" {
		t.Fatalf("incorrect delegated result: %+v", result)
	}
}

func TestInvalidCNIInputFailsBeforeClientOrGuestDiscovery(t *testing.T) {
	if err := cniCommand(context.Background(), "ADD", &skel.CmdArgs{StdinData: []byte(`{"name":"azure","ipam":{"type":"azure-vnet-ipam"}}`)}); err == nil {
		t.Fatal("wrong plugin type accepted")
	}
}
