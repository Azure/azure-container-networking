package nodenetworkconfig

import (
	"context"
	"net/netip"
	"testing"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/logger"
	cnstypes "github.com/Azure/azure-container-networking/cns/types"
	"github.com/Azure/azure-container-networking/crd/nodenetworkconfig/api/v1alpha"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// TestReconcileRetriesRouteInitialization checks that route failures retry without repeating call to the CNS state initializer.
func TestReconcileRetriesRouteInitialization(t *testing.T) {
	logger.InitLogger("", 0, 0, "") //nolint:staticcheck // legacy reconciler logger
	initializerCalls, createCalls := 0, 0
	routeErr := errors.New("route failed")
	routes := &fakeRouteClient{err: routeErr}
	client := &mockCNSClient{
		state: cnsClientState{reqsByNCID: make(map[string]*cns.CreateNetworkContainerRequest)},
		createOrUpdateNC: func(*cns.CreateNetworkContainerRequest) cnstypes.ResponseCode {
			createCalls++
			return cnstypes.Success
		},
	}
	r := NewReconciler(client, func(*v1alpha.NodeNetworkConfig) error {
		initializerCalls++
		return nil
	}, client, nodeIP, false, 0, routes)
	r.nnccli = &mockNCGetter{get: func(context.Context, types.NamespacedName) (*v1alpha.NodeNetworkConfig, error) {
		return &v1alpha.NodeNetworkConfig{Status: v1alpha.NodeNetworkConfigStatus{NetworkContainers: []v1alpha.NetworkContainer{validOverlayNC}}}, nil
	}}

	_, err := r.Reconcile(context.Background(), reconcile.Request{})
	require.ErrorIs(t, err, routeErr)
	assert.Equal(t, 1, initializerCalls)
	require.Len(t, routes.prefixes, 1)
	assert.Zero(t, createCalls)
	select {
	case <-r.started:
		t.Fatal("reconciler started despite route programming failure")
	default:
	}

	routes.err = nil
	_, err = r.Reconcile(context.Background(), reconcile.Request{})
	require.NoError(t, err)
	assert.Equal(t, 1, initializerCalls, "route retry must not repeat successful CNS initialization")
	require.Len(t, routes.prefixes, 2)
	assert.Equal(t, 1, createCalls)
	select {
	case <-r.started:
	default:
		t.Fatal("reconciler did not start after route retry succeeded")
	}
}

// TestReconcileInitializesRoutesOnce checks that two reconciles program routes once with a client and skip routing without one.
func TestReconcileInitializesRoutesOnce(t *testing.T) {
	logger.InitLogger("", 0, 0, "") //nolint:staticcheck // legacy reconciler logger
	for _, enabled := range []bool{false, true} {
		name := "disabled"
		if enabled {
			name = "enabled"
		}
		t.Run(name, func(t *testing.T) {
			routes := &fakeRouteClient{}
			var pendingRoutes RouteClient
			if enabled {
				pendingRoutes = routes
			}
			client := &mockCNSClient{
				state: cnsClientState{reqsByNCID: make(map[string]*cns.CreateNetworkContainerRequest)},
				createOrUpdateNC: func(*cns.CreateNetworkContainerRequest) cnstypes.ResponseCode {
					return cnstypes.Success
				},
			}
			r := NewReconciler(client, nil, client, nodeIP, false, 0, pendingRoutes)
			r.nnccli = &mockNCGetter{get: func(context.Context, types.NamespacedName) (*v1alpha.NodeNetworkConfig, error) {
				return &v1alpha.NodeNetworkConfig{Status: v1alpha.NodeNetworkConfigStatus{NetworkContainers: []v1alpha.NetworkContainer{validOverlayNC}}}, nil
			}}

			_, err := r.Reconcile(context.Background(), reconcile.Request{})
			require.NoError(t, err)
			_, err = r.Reconcile(context.Background(), reconcile.Request{})
			require.NoError(t, err)
			if enabled {
				assert.Equal(t, []netip.Prefix{netip.MustParsePrefix("10.0.0.0/30")}, routes.prefixes)
			} else {
				assert.Empty(t, routes.prefixes)
			}
		})
	}
}
