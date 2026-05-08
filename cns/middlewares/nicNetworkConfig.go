package middlewares

import (
	"context"
	"strings"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/crd/multitenancy/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// NICNetworkConfigMiddleware enriches NICResource data with NICNetworkConfig CRD information.
type NICNetworkConfigMiddleware struct {
	Cli client.Client
}

// GetNICNCInfoByMAC lists all NICNetworkConfig CRDs and returns a map keyed by MAC address.
func (m *NICNetworkConfigMiddleware) GetNICNCInfoByMAC(ctx context.Context) (map[string]*cns.NICNCInfo, error) {
	var nicNCList v1alpha1.NICNetworkConfigList
	if err := m.Cli.List(ctx, &nicNCList); err != nil {
		return nil, err
	}

	result := make(map[string]*cns.NICNCInfo, len(nicNCList.Items))
	for i := range nicNCList.Items {
		mac := nicNCList.Items[i].Status.MacAddress
		if mac == "" {
			continue
		}
		// Status.PrimaryIP is in CIDR form (e.g., "165.0.0.16/28"). Strip the
		// prefix length so consumers get just the IP address.
		primaryIP := nicNCList.Items[i].Status.PrimaryIP
		if idx := strings.IndexByte(primaryIP, '/'); idx > 0 {
			primaryIP = primaryIP[:idx]
		}
		result[mac] = &cns.NICNCInfo{
			NetworkID: nicNCList.Items[i].Spec.NetworkID,
			SubnetID:  nicNCList.Items[i].Spec.SubnetID,
			PrimaryIP: primaryIP,
		}
	}

	logger.Printf("[NICNetworkConfigMiddleware] fetched %d NICNetworkConfigs, %d with MAC addresses", len(nicNCList.Items), len(result))
	return result, nil
}
