package cniconflist_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/Azure/azure-container-networking/cns/cniconflist"
	"github.com/stretchr/testify/require"
)

type windowsBufferWriteCloser struct {
	*bytes.Buffer
}

func (b *windowsBufferWriteCloser) Close() error {
	return nil
}

func TestGenerateWindowsV4OverlayConflist(t *testing.T) {
	buffer := new(bytes.Buffer)
	generator := cniconflist.V4OverlayGenerator{
		Writer: &windowsBufferWriteCloser{
			Buffer: buffer,
		},
		WindowsSettings: cniconflist.WindowsSettings{
			DNSServiceIP: "10.0.0.10",
			ClusterCIDRs: []string{
				"10.240.0.0/16",
			},
			ServiceCIDRs: []string{
				"10.0.0.0/8",
			},
			VNetCIDRs: []string{
				"10.1.0.0/16",
			},
		},
	}

	err := generator.Generate()
	require.NoError(t, err)

	var conflist struct {
		CNIVersion string `json:"cniVersion"`
		Name       string `json:"name"`
		Plugins    []struct {
			Type string `json:"type"`
			IPAM struct {
				Type string `json:"type"`
				Mode string `json:"mode"`
			} `json:"ipam"`
			DNS struct {
				Nameservers []string `json:"Nameservers"`
			} `json:"dns"`
			AdditionalArgs []struct {
				Name  string `json:"Name"`
				Value struct {
					Type              string   `json:"Type"`
					ExceptionList     []string `json:"ExceptionList"`
					DestinationPrefix string   `json:"DestinationPrefix"`
					NeedEncap         bool     `json:"NeedEncap"`
				} `json:"Value"`
			} `json:"AdditionalArgs"`
		} `json:"plugins"`
	}
	err = json.Unmarshal(buffer.Bytes(), &conflist)
	require.NoError(t, err)

	require.Equal(t, "0.3.0", conflist.CNIVersion)
	require.Equal(t, "azure", conflist.Name)
	require.Len(t, conflist.Plugins, 1)
	require.Equal(t, "azure-vnet", conflist.Plugins[0].Type)
	require.Equal(t, "azure-cns", conflist.Plugins[0].IPAM.Type)
	require.Equal(t, "overlay", conflist.Plugins[0].IPAM.Mode)
	require.Equal(t, []string{"10.0.0.10", "168.63.129.16"}, conflist.Plugins[0].DNS.Nameservers)
	require.Len(t, conflist.Plugins[0].AdditionalArgs, 2)
	require.Equal(t, "OutBoundNAT", conflist.Plugins[0].AdditionalArgs[0].Value.Type)
	require.ElementsMatch(
		t,
		[]string{"10.240.0.0/16", "10.0.0.0/8", "10.1.0.0/16"},
		conflist.Plugins[0].AdditionalArgs[0].Value.ExceptionList,
	)
	require.Equal(t, "ROUTE", conflist.Plugins[0].AdditionalArgs[1].Value.Type)
	require.Equal(t, "10.0.0.0/8", conflist.Plugins[0].AdditionalArgs[1].Value.DestinationPrefix)
	require.True(t, conflist.Plugins[0].AdditionalArgs[1].Value.NeedEncap)
}
