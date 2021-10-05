package kubecontroller

import (
	"context"
	"testing"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/crd/nodenetworkconfig/api/v1alpha"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type cnsClientState struct {
	req    cns.CreateNetworkContainerRequest
	scaler v1alpha.Scaler
	spec   v1alpha.NodeNetworkConfigSpec
}

type mockCNSClient struct {
	state                 cnsClientState
	createOrUpdateNC      func(cns.CreateNetworkContainerRequest) error
	updateIPAMPoolMonitor func(v1alpha.Scaler, v1alpha.NodeNetworkConfigSpec)
}

//nolint:gocritic // ignore hugeParam pls
func (m *mockCNSClient) CreateOrUpdateNC(req cns.CreateNetworkContainerRequest) error {
	m.state.req = req
	return m.createOrUpdateNC(req)
}

func (m *mockCNSClient) UpdateIPAMPoolMonitor(scaler v1alpha.Scaler, spec v1alpha.NodeNetworkConfigSpec) {
	m.state.scaler = scaler
	m.state.spec = spec
	m.updateIPAMPoolMonitor(scaler, spec)
}

type mockNCGetter struct {
	get func(context.Context, types.NamespacedName) (*v1alpha.NodeNetworkConfig, error)
}

func (m *mockNCGetter) Get(ctx context.Context, key types.NamespacedName) (*v1alpha.NodeNetworkConfig, error) {
	return m.get(ctx, key)
}

func TestReconcile(t *testing.T) {
	logger.InitLogger("", 0, 0, "")
	tests := []struct {
		name               string
		in                 reconcile.Request
		ncGetter           mockNCGetter
		cnsClient          mockCNSClient
		want               reconcile.Result
		wantCNSClientState cnsClientState
		wantErr            bool
	}{
		{
			name: "unknown get err",
			ncGetter: mockNCGetter{
				get: func(context.Context, types.NamespacedName) (*v1alpha.NodeNetworkConfig, error) {
					return nil, errors.New("")
				},
			},
			wantErr: true,
		},
		{
			name: "not found",
			ncGetter: mockNCGetter{
				get: func(context.Context, types.NamespacedName) (*v1alpha.NodeNetworkConfig, error) {
					return nil, apierrors.NewNotFound(schema.GroupResource{}, "")
				},
			},
			wantErr: false,
		},
		{
			name: "no NCs",
			ncGetter: mockNCGetter{
				get: func(context.Context, types.NamespacedName) (*v1alpha.NodeNetworkConfig, error) {
					return &v1alpha.NodeNetworkConfig{}, nil
				},
			},
			wantErr: false,
		},
		{
			name: "invalid NCs",
			ncGetter: mockNCGetter{
				get: func(context.Context, types.NamespacedName) (*v1alpha.NodeNetworkConfig, error) {
					return &v1alpha.NodeNetworkConfig{
						Status: invalidStatusMultiNC,
					}, nil
				},
			},
			wantErr: true,
		},
		{
			name: "err in CreateOrUpdateNC",
			ncGetter: mockNCGetter{
				get: func(context.Context, types.NamespacedName) (*v1alpha.NodeNetworkConfig, error) {
					return &v1alpha.NodeNetworkConfig{
						Status: validStatus,
					}, nil
				},
			},
			cnsClient: mockCNSClient{
				createOrUpdateNC: func(cns.CreateNetworkContainerRequest) error {
					return errors.New("")
				},
			},
			wantErr: true,
			wantCNSClientState: cnsClientState{
				req: validRequest,
			},
		},
		{
			name: "success",
			ncGetter: mockNCGetter{
				get: func(context.Context, types.NamespacedName) (*v1alpha.NodeNetworkConfig, error) {
					return &v1alpha.NodeNetworkConfig{
						Status: validStatus,
						Spec: v1alpha.NodeNetworkConfigSpec{
							RequestedIPCount: 1,
						},
					}, nil
				},
			},
			cnsClient: mockCNSClient{
				createOrUpdateNC: func(cns.CreateNetworkContainerRequest) error {
					return nil
				},
				updateIPAMPoolMonitor: func(v1alpha.Scaler, v1alpha.NodeNetworkConfigSpec) {},
			},
			wantErr: false,
			wantCNSClientState: cnsClientState{
				req:    validRequest,
				scaler: validStatus.Scaler,
				spec: v1alpha.NodeNetworkConfigSpec{
					RequestedIPCount: 1,
				},
			},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			r := New(&tt.ncGetter, &tt.cnsClient)
			got, err := r.Reconcile(context.Background(), tt.in)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.wantCNSClientState, tt.cnsClient.state)
		})
	}
}
