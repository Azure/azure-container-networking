package ciliumpool

import (
	"strings"
	"testing"
)

const testConfig = `{"cniVersion":"1.0.0","name":"azure","master":"eth0","type":"azure-vnet","mode":"transparent-tunnel",
	"ipam":{"type":"azure-cilium-ipam","cilium":{
		"nodeName":"node","kubeconfig":"/etc/azure-vnet/ipam.kubeconfig",
		"nicID":"/subscriptions/11111111-1111-1111-1111-111111111111/resourceGroups/rg/providers/Microsoft.Network/networkInterfaces/nic",
		"subnet":"10.0.0.0/24","preAllocate":8}}}`

func TestConfigUsesMasterAsOnlyInterfaceSetting(t *testing.T) {
	_, cfg, err := ParseConfig([]byte(testConfig))
	if err != nil || cfg.Interface != testIfName {
		t.Fatalf("master-only configuration = %+v, %v", cfg, err)
	}
	for _, invalid := range []string{
		strings.Replace(testConfig, `"master":"eth0",`, "", 1),
		strings.Replace(testConfig, `"preAllocate":8`, `"preAllocate":8,"interface":"eth0"`, 1),
	} {
		if _, _, parseErr := ParseConfig([]byte(invalid)); parseErr == nil {
			t.Fatal("missing master or duplicate interface setting accepted")
		}
	}
}

func TestConfigAcceptsConflistAndRejectsFallbackOrUnqualifiedFeatures(t *testing.T) {
	list := `{"cniVersion":"1.0.0","name":"azure","plugins":[` + testConfig + `,{"type":"portmap"}]}`
	for _, raw := range []string{testConfig, list} {
		nw, cfg, err := ParseConfig([]byte(raw))
		if err != nil || nw.Name != testNetwork || cfg.NodeName != testNodeName || cfg.PreAllocate != 8 {
			t.Fatalf("configuration = %+v, %+v, %v", nw, cfg, err)
		}

	}
	for _, tt := range []struct{ name, raw string }{
		{"wrong plugin", strings.Replace(testConfig, `"type":"azure-cilium-ipam"`, `"type":"azure-vnet-ipam"`, 1)},
		{"wrong mode", strings.Replace(testConfig, `"transparent-tunnel"`, `"transparent"`, 1)},
		{"relative credential", strings.Replace(testConfig, `"/etc/azure-vnet/ipam.kubeconfig"`, `"relative"`, 1)},
		{"invalid subnet", strings.Replace(testConfig, `"10.0.0.0/24"`, `"10.0.0.10/24"`, 1)},
		{"unknown persistence", strings.Replace(testConfig, `"preAllocate":8`, `"preAllocate":8,"stateDir":"/var/lib/ipam"`, 1)},
		{"shrink", strings.Replace(testConfig, `"preAllocate":8`, `"preAllocate":8,"releaseExcessIPs":true`, 1)},
		{"peers", strings.Replace(testConfig, `"preAllocate":8`, `"preAllocate":8,"coordinatePeers":true`, 1)},
		{"ipv6", strings.Replace(testConfig, `"name":"azure"`, `"name":"azure","ipv6Mode":"ipv6"`, 1)},
		{"baremetal", strings.Replace(testConfig, `"name":"azure"`, `"name":"azure","executionMode":"baremetal"`, 1)},
		{"multitenant", strings.Replace(testConfig, `"name":"azure"`, `"name":"azure","multiTenancy":true`, 1)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, err := ParseConfig([]byte(tt.raw)); err == nil {
				t.Fatal("invalid configuration accepted")
			}
		})
	}
}
