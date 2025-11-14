//go:build lrp

package lrp

import (
	"context"
	"testing"

	"github.com/Azure/azure-container-networking/test/internal/kubernetes"
	ciliumClientset "github.com/cilium/cilium/pkg/k8s/client/clientset/versioned"
	"github.com/stretchr/testify/require"
)

var (
	fqdnCNPPath    = ciliumManifestsDir + "fqdn-cnp.yaml"
	enableFQDNFlag = "enable-l7-proxy"
)

// TestLRPFQDN tests if the local redirect policy in a cilium cluster is functioning with a
// FQDN Cilium Network Policy. As such, enable-l7-proxy should be enabled in the config
// The test assumes the current kubeconfig points to a cluster with cilium, cns,
// and kube-dns already installed. The lrp feature flag should also be enabled in the cilium config
// Does not check if cluster is in a stable state
// Resources created are automatically cleaned up
// From the lrp folder, run: go test ./ -v -tags "lrp" -run ^TestLRPFQDN$
func TestLRPFQDN(t *testing.T) {
	ctx := context.Background()

	selectedPod, cleanupFn := setupLRP(t, ctx)
	defer cleanupFn()
	require.NotNil(t, selectedPod)

	cs := kubernetes.MustGetClientset()
	config := kubernetes.MustGetRestConfig()
	ciliumCS, err := ciliumClientset.NewForConfig(config)
	require.NoError(t, err)

	// ensure enable l7 proxy flag is enabled
	ciliumCM, err := kubernetes.GetConfigmap(ctx, cs, kubeSystemNamespace, ciliumConfigmapName)
	require.NoError(t, err)
	require.Equal(t, "true", ciliumCM.Data[enableFQDNFlag], "enable-l7-proxy not set to true in cilium-config")

	_, cleanupCNP := kubernetes.MustSetupCNP(ctx, ciliumCS, fqdnCNPPath)
	defer cleanupCNP()

	tests := []struct {
		name                   string
		command                []string
		expectedMsgContains    string
		expectedErrMsgContains string
		shouldError            bool
		countIncreases         bool
	}{
		{
			name:           "nslookup google succeeds",
			command:        []string{"nslookup", "www.google.com", "10.0.0.10"},
			countIncreases: true,
			shouldError:    false,
		},
		{
			name:           "nslookup google succeeds without explicit dns server",
			command:        []string{"nslookup", "www.google.com"},
			countIncreases: true,
			shouldError:    false,
		},
		{
			name:                   "wget google succeeds",
			command:                []string{"wget", "-O", "index.html", "www.google.com", "--timeout=5"},
			expectedErrMsgContains: "saved",
			countIncreases:         true,
			shouldError:            false,
		},
		{
			name:           "nslookup cloudflare succeeds",
			command:        []string{"nslookup", "www.cloudflare.com", "10.0.0.10"},
			countIncreases: true,
			shouldError:    false,
		},
		{
			name:                   "wget cloudflare fails but dns succeeds",
			command:                []string{"wget", "-O", "index.html", "www.cloudflare.com", "--timeout=5"},
			expectedErrMsgContains: "timed out",
			countIncreases:         true,
			shouldError:            true,
		},
		{
			name:                "nslookup example fails",
			command:             []string{"nslookup", "www.example.com", "10.0.0.10"},
			expectedMsgContains: "REFUSED",
			countIncreases:      false,
			shouldError:         true,
		},
		{
			// won't be able to nslookup, let alone query the website
			name:                   "wget example fails",
			command:                []string{"wget", "-O", "index.html", "www.example.com", "--timeout=5"},
			expectedErrMsgContains: "bad address",
			countIncreases:         false,
			shouldError:            true,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			testLRPCase(t, ctx, *selectedPod, tt.command, tt.expectedMsgContains, tt.expectedErrMsgContains, tt.shouldError, tt.countIncreases, getPrometheusAddress(initialPrometheusPort))
		})
	}
}
