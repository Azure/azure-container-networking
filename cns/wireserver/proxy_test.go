package wireserver

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/url"
	"testing"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/stretchr/testify/require"
)

type testDo struct {
	do func(*http.Request) (*http.Response, error)
}

func (t *testDo) Do(req *http.Request) (*http.Response, error) {
	return t.do(req)
}

const (
	useLegacyChannelFalse = "false"
	interfaceID           = "iface-1"
	networkContainerID    = "nc-1"
	authToken             = "token-1"
)

func TestProxyRNCPublisherQueryParam(t *testing.T) {
	tests := []struct {
		name               string
		call               func(*Proxy) (*http.Response, error)
		expectedFlag       string
		expectLegacySwitch bool
		expectedTypePath   string
	}{
		{
			name: "JoinNetwork adds useLegacyChannel=false for RNC",
			call: func(p *Proxy) (*http.Response, error) {
				return p.JoinNetwork(context.Background(), "vnet-1", true)
			},
			expectedFlag:       useLegacyChannelFalse,
			expectLegacySwitch: true,
			expectedTypePath:   "NetworkManagement/joinedVirtualNetworks/vnet-1/api-version/1",
		},
		{
			name: "PublishNC adds useLegacyChannel=false for RNC",
			call: func(p *Proxy) (*http.Response, error) {
				return p.PublishNC(context.Background(), cns.NetworkContainerParameters{
					AssociatedInterfaceID: interfaceID,
					NCID:                  networkContainerID,
					AuthToken:             authToken,
				}, []byte(`{}`), true)
			},
			expectedFlag:       useLegacyChannelFalse,
			expectLegacySwitch: true,
			expectedTypePath:   "NetworkManagement/interfaces/iface-1/networkContainers/nc-1/authenticationToken/token-1/api-version/1",
		},
		{
			name: "UnpublishNC adds useLegacyChannel=false for RNC",
			call: func(p *Proxy) (*http.Response, error) {
				return p.UnpublishNC(context.Background(), cns.NetworkContainerParameters{
					AssociatedInterfaceID: interfaceID,
					NCID:                  networkContainerID,
					AuthToken:             authToken,
				}, []byte(`{}`), true)
			},
			expectedFlag:       useLegacyChannelFalse,
			expectLegacySwitch: true,
			expectedTypePath:   "NetworkManagement/interfaces/iface-1/networkContainers/nc-1/authenticationToken/token-1/api-version/1/method/DELETE",
		},
		{
			name: "JoinSubnet includes useLegacyChannel=false",
			call: func(p *Proxy) (*http.Response, error) {
				return p.JoinSubnet(context.Background(), "vnet-1", "subnet-1", cns.NetworkContainerParameters{
					AuthToken: authToken,
				})
			},
			expectedFlag:       useLegacyChannelFalse,
			expectLegacySwitch: true,
			expectedTypePath:   "NetworkManagement/joinedVirtualNetworks/vnet-1/joinedSubnets/subnet-1/authenticationToken/token-1/api-version/1",
		},
		{
			name: "JoinNetwork does not include useLegacyChannel when RNC disabled",
			call: func(p *Proxy) (*http.Response, error) {
				return p.JoinNetwork(context.Background(), "vnet-1", false)
			},
			expectLegacySwitch: false,
			expectedTypePath:   "NetworkManagement/joinedVirtualNetworks/vnet-1/api-version/1",
		},
		{
			name: "PublishNC does not include useLegacyChannel when RNC disabled",
			call: func(p *Proxy) (*http.Response, error) {
				return p.PublishNC(context.Background(), cns.NetworkContainerParameters{
					AssociatedInterfaceID: interfaceID,
					NCID:                  networkContainerID,
					AuthToken:             authToken,
				}, []byte(`{}`), false)
			},
			expectLegacySwitch: false,
			expectedTypePath:   "NetworkManagement/interfaces/iface-1/networkContainers/nc-1/authenticationToken/token-1/api-version/1",
		},
		{
			name: "UnpublishNC does not include useLegacyChannel when RNC disabled",
			call: func(p *Proxy) (*http.Response, error) {
				return p.UnpublishNC(context.Background(), cns.NetworkContainerParameters{
					AssociatedInterfaceID: interfaceID,
					NCID:                  networkContainerID,
					AuthToken:             authToken,
				}, []byte(`{}`), false)
			},
			expectLegacySwitch: false,
			expectedTypePath:   "NetworkManagement/interfaces/iface-1/networkContainers/nc-1/authenticationToken/token-1/api-version/1/method/DELETE",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var reqURL *url.URL

			p := &Proxy{
				Host: "127.0.0.1:9001",
				HTTPClient: &testDo{
					do: func(req *http.Request) (*http.Response, error) {
						reqURL = req.URL
						return &http.Response{
							StatusCode: http.StatusOK,
							Body:       io.NopCloser(bytes.NewBufferString(`{}`)),
						}, nil
					},
				},
			}

			resp, err := tt.call(p)
			require.NoError(t, err)
			t.Cleanup(func() {
				if resp != nil && resp.Body != nil {
					_ = resp.Body.Close()
				}
			})
			require.NotNil(t, reqURL)

			q := reqURL.Query()
			typeVal := q.Get("type")
			require.NotContains(t, typeVal, "?useLegacyChannel=false")
			require.Equal(t, tt.expectedTypePath, typeVal)

			if tt.expectLegacySwitch {
				require.Equal(t, tt.expectedFlag, q.Get("useLegacyChannel"))
			} else {
				_, exists := q["useLegacyChannel"]
				require.False(t, exists)
			}
		})
	}
}
