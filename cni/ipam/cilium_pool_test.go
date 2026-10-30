package ipam

import (
	"strings"
	"testing"

	"github.com/Azure/azure-container-networking/common"
)

func TestCiliumPoolConfigurationCannotSelectIndependentSource(t *testing.T) {
	config := &common.PluginConfig{CiliumPool: &common.CiliumPoolConfig{}}
	plugin, err := NewPlugin("azure-vnet-ipam", config)
	if err != nil {
		t.Fatal(err)
	}
	if err = plugin.Initialize(config); err != nil {
		t.Fatal(err)
	}
	nw, err := plugin.Configure([]byte(`{"cniVersion":"0.3.0","name":"azure","ipam":{"type":"azure-vnet-ipam"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if nw.IPAM.Environment != common.OptEnvironmentCiliumPool {
		t.Errorf("IPAM environment = %q, want %q", nw.IPAM.Environment, common.OptEnvironmentCiliumPool)
	}
	_, err = plugin.Configure([]byte(`{"cniVersion":"0.3.0","name":"azure","ipam":{"type":"azure-vnet-ipam","environment":"azure"}}`))
	if err == nil || !strings.Contains(err.Error(), "conflicts") {
		t.Errorf("Configure with independent source: got %v, want conflict error", err)
	}
	plain, err := NewPlugin("azure-vnet-ipam", &common.PluginConfig{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = plain.Configure([]byte(`{"cniVersion":"0.3.0","name":"azure","ipam":{"type":"azure-vnet-ipam","environment":"cilium-pool"}}`))
	if err == nil || !strings.Contains(err.Error(), "host configuration") {
		t.Errorf("Configure without host configuration: got %v, want host configuration error", err)
	}
}
