package v2

import (
	"context"
	"math/rand"
	"net/netip"
	"testing"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/types"
	"github.com/Azure/azure-container-networking/crd/nodenetworkconfig/api/v1alpha"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"golang.org/x/exp/maps"
)

type ipStateStoreMock struct {
	pendingReleaseIPConfigs map[string]cns.IPConfigurationStatus
	err                     error
}

func (m *ipStateStoreMock) GetPendingReleaseIPConfigs() []cns.IPConfigurationStatus {
	return maps.Values(m.pendingReleaseIPConfigs)
}

func (m *ipStateStoreMock) MarkNIPsPendingRelease(n int) (map[string]cns.IPConfigurationStatus, error) {
	if m.err != nil {
		return nil, m.err
	}
	newPendingRelease := pendingReleaseGenerator(n)
	maps.Copy(newPendingRelease, m.pendingReleaseIPConfigs)
	m.pendingReleaseIPConfigs = newPendingRelease
	return m.pendingReleaseIPConfigs, nil
}

// pendingReleaseGenerator generates a variable number of random pendingRelease IPConfigs.
func pendingReleaseGenerator(n int) map[string]cns.IPConfigurationStatus {
	m := make(map[string]cns.IPConfigurationStatus, n)
	ip := netip.MustParseAddr("10.0.0.0")
	for i := 0; i < n; i++ {
		id := uuid.New().String()
		ip = ip.Next()
		status := cns.IPConfigurationStatus{
			ID:        id,
			IPAddress: ip.String(),
		}
		status.SetState(types.PendingRelease)
		m[id] = status
	}
	return m
}

func TestPendingReleaseIPConfigsGenerator(t *testing.T) {
	t.Parallel()
	n := rand.Intn(100) //nolint:gosec // test
	m := pendingReleaseGenerator(n)
	assert.Len(t, m, n, "pendingReleaseGenerator made the wrong quantity")
	for k, v := range m {
		_, err := uuid.Parse(v.ID)
		require.NoError(t, err, "pendingReleaseGenerator made a bad UUID")
		assert.Equal(t, k, v.ID, "pendingReleaseGenerator stored using the wrong key ")
		_, err = netip.ParseAddr(v.IPAddress)
		require.NoError(t, err, "pendingReleaseGenerator made a bad IP")
		assert.Equal(t, types.PendingRelease, v.GetState(), "pendingReleaseGenerator set the wrong State")
	}
}

func TestBuildNNCSpec(t *testing.T) {
	tests := []struct {
		name                    string
		pendingReleaseIPConfigs map[string]cns.IPConfigurationStatus
		request                 int64
	}{
		{
			name:    "without no pending release",
			request: 16,
		},
		{
			name:                    "with pending release",
			pendingReleaseIPConfigs: pendingReleaseGenerator(16),
			request:                 16,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			pm := &Monitor{
				store: &ipStateStoreMock{
					pendingReleaseIPConfigs: tt.pendingReleaseIPConfigs,
				},
			}
			spec := pm.buildNNCSpec(tt.request)
			assert.Equal(t, tt.request, spec.RequestedIPCount)
			assert.Equal(t, len(tt.pendingReleaseIPConfigs), len(spec.IPsNotInUse))
			assert.ElementsMatch(t, maps.Keys(tt.pendingReleaseIPConfigs), spec.IPsNotInUse)
		})
	}
}

type nncClientMock struct {
	req v1alpha.NodeNetworkConfigSpec
	err error
}

func (m *nncClientMock) PatchSpec(_ context.Context, spec *v1alpha.NodeNetworkConfigSpec, _ string) (*v1alpha.NodeNetworkConfig, error) {
	if m.err != nil {
		return nil, m.err
	}
	m.req = *spec
	return nil, nil
}

func TestReconcile(t *testing.T) {
	tests := []struct {
		name               string
		demand             int64
		request            int64
		scaler             scaler
		nnccli             nncClientMock
		store              ipStateStoreMock
		wantRequest        int64
		wantPendingRelease int
		wantErr            bool
	}{
		// no-op case
		{
			name:    "no delta",
			demand:  5,
			request: 16,
			scaler: scaler{
				batch:  16,
				buffer: .5,
				max:    250,
			},
			nnccli: nncClientMock{
				req: v1alpha.NodeNetworkConfigSpec{
					RequestedIPCount: 16,
				},
			},
			store:       ipStateStoreMock{},
			wantRequest: 16,
		},
		// fail to mark IPs pending release
		{
			name:    "fail to release",
			demand:  6,
			request: 32,
			scaler: scaler{
				batch:  16,
				buffer: .5,
				max:    250,
			},
			nnccli: nncClientMock{
				req: v1alpha.NodeNetworkConfigSpec{
					RequestedIPCount: 32,
				},
			},
			store: ipStateStoreMock{
				err: errors.Errorf("failed to mark IPs pending release"),
			},
			wantRequest: 32,
			wantErr:     true,
		},
		// fail to Patch NNC Spec
		{
			name:    "fail to patch",
			demand:  20,
			request: 16,
			scaler: scaler{
				batch:  16,
				buffer: .5,
				max:    250,
			},
			nnccli: nncClientMock{
				req: v1alpha.NodeNetworkConfigSpec{
					RequestedIPCount: 16,
				},
				err: errors.Errorf("failed to patch NNC Spec"),
			},
			store:       ipStateStoreMock{},
			wantRequest: 16,
			wantErr:     true,
		},
		// normal scale ups with no pending release
		{
			name:    "single scale up",
			demand:  15,
			request: 16,
			scaler: scaler{
				batch:  16,
				buffer: .5,
				max:    250,
			},
			nnccli:      nncClientMock{},
			store:       ipStateStoreMock{},
			wantRequest: 32,
		},
		{
			name:    "big scale up",
			demand:  75,
			request: 16,
			scaler: scaler{
				batch:  16,
				buffer: .5,
				max:    250,
			},
			nnccli:      nncClientMock{},
			store:       ipStateStoreMock{},
			wantRequest: 96,
		},
		{
			name:    "capped scale up",
			demand:  300,
			request: 16,
			scaler: scaler{
				batch:  16,
				buffer: .5,
				max:    250,
			},
			nnccli:      nncClientMock{},
			store:       ipStateStoreMock{},
			wantRequest: 250,
		},
		// normal scale down with no previously pending release
		{
			name:    "single scale down",
			demand:  5,
			request: 32,
			scaler: scaler{
				batch:  16,
				buffer: .5,
				max:    250,
			},
			nnccli:             nncClientMock{},
			store:              ipStateStoreMock{},
			wantRequest:        16,
			wantPendingRelease: 16,
		},
		{
			name:    "big scale down",
			demand:  5,
			request: 128,
			scaler: scaler{
				batch:  16,
				buffer: .5,
				max:    250,
			},
			nnccli:             nncClientMock{},
			store:              ipStateStoreMock{},
			wantRequest:        16,
			wantPendingRelease: 112,
		},
		{
			name:    "capped scale down",
			demand:  0,
			request: 32,
			scaler: scaler{
				batch:  16,
				buffer: .5,
				max:    250,
			},
			nnccli:             nncClientMock{},
			store:              ipStateStoreMock{},
			wantRequest:        16,
			wantPendingRelease: 16,
		},
		// realign to batch if request is skewed
		{
			name:    "scale up unskew",
			demand:  15,
			request: 3,
			scaler: scaler{
				batch:  16,
				buffer: .5,
				max:    250,
			},
			nnccli:      nncClientMock{},
			store:       ipStateStoreMock{},
			wantRequest: 32,
		},
		{
			name:    "scale down unskew",
			demand:  5,
			request: 37,
			scaler: scaler{
				batch:  16,
				buffer: .5,
				max:    250,
			},
			nnccli:             nncClientMock{},
			store:              ipStateStoreMock{},
			wantRequest:        16,
			wantPendingRelease: 21,
		},
		// normal scale up with previous pending release
		{
			name:    "single scale up with pending release",
			demand:  20,
			request: 16,
			scaler: scaler{
				batch:  16,
				buffer: .5,
				max:    250,
			},
			nnccli: nncClientMock{},
			store: ipStateStoreMock{
				pendingReleaseIPConfigs: pendingReleaseGenerator(16),
			},
			wantRequest:        32,
			wantPendingRelease: 16,
		},
		// normal scale down with previous pending release
		{
			name:    "single scale down with pending release",
			demand:  5,
			request: 32,
			scaler: scaler{
				batch:  16,
				buffer: .5,
				max:    250,
			},
			nnccli: nncClientMock{},
			store: ipStateStoreMock{
				pendingReleaseIPConfigs: pendingReleaseGenerator(16),
			},
			wantRequest:        16,
			wantPendingRelease: 32,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt := tt
			t.Parallel()
			pm := &Monitor{
				z:       zap.NewNop(),
				demand:  tt.demand,
				request: tt.request,
				scaler:  tt.scaler,
				nnccli:  &tt.nnccli,
				store:   &tt.store,
			}
			err := pm.reconcile(context.Background())
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.wantRequest, pm.request)
			assert.Equal(t, tt.wantRequest, tt.nnccli.req.RequestedIPCount)
			assert.Len(t, tt.nnccli.req.IPsNotInUse, tt.wantPendingRelease)
			assert.Equal(t, tt.wantRequest, pm.request)
		})
	}
}
