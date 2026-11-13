package cni

import (
	"encoding/json"
	"testing"
)

func TestCiliumIPAMSettingsSurviveDelegation(t *testing.T) {
	raw := []byte(`{"name":"azure","type":"azure-vnet","mode":"transparent-tunnel",
		"ipam":{"type":"azure-cilium-ipam","cilium":{
			"nodeName":"node-a","kubeconfig":"/etc/azure-vnet/ipam.kubeconfig","preAllocate":8}}}`)
	cfg, err := ParseNetworkConfig(raw)
	if err != nil {
		t.Fatal(err)
	}
	var delegated struct {
		IPAM struct {
			Cilium struct {
				NodeName    string `json:"nodeName"`
				Kubeconfig  string `json:"kubeconfig"`
				PreAllocate int    `json:"preAllocate"`
			} `json:"cilium"`
		} `json:"ipam"`
	}
	if err := json.Unmarshal(cfg.Serialize(), &delegated); err != nil {
		t.Fatal(err)
	}
	got := delegated.IPAM.Cilium
	if got.NodeName != "node-a" || got.Kubeconfig != "/etc/azure-vnet/ipam.kubeconfig" || got.PreAllocate != 8 {
		t.Fatalf("delegation discarded IPAM settings: %+v", got)
	}
}
