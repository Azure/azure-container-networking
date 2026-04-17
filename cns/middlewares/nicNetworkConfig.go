package middlewares

import (
	"context"

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
		if mac != "" {
			result[mac] = &cns.NICNCInfo{
				NetworkID:  nicNCList.Items[i].Spec.NetworkID,
				SubnetName: nicNCList.Items[i].Spec.SubnetName,
			}
		}
	}

	logger.Printf("[NICNetworkConfigMiddleware] fetched %d NICNetworkConfigs, %d with MAC addresses", len(nicNCList.Items), len(result))
	return result, nil
}
