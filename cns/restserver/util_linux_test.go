package restserver

import (
	"context"
	"testing"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/stretchr/testify/require"
)

func TestGetPnpIDMapping(t *testing.T) {
	svc := getTestService(cns.KubernetesCRD)
	svc.state.PnpIDByMacAddress = map[string]string{
		"macaddress1": "value1",
	}
	pnpID, _ := svc.getPNPIDFromMacAddress(context.Background(), "macaddress1")
	require.NotEmpty(t, pnpID)

	// Backend network adapter not found
	_, err := svc.getPNPIDFromMacAddress(context.Background(), "macaddress8")
	require.Error(t, err)

	// Empty pnpidmacaddress mapping
	svc.state.PnpIDByMacAddress = map[string]string{}
	_, err = svc.getPNPIDFromMacAddress(context.Background(), "macaddress8")
	require.Error(t, err)
}
