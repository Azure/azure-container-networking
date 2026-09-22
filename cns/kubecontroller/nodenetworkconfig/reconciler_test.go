package nodenetworkconfig

import (
	"context"
	"testing"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/cns/restserver"
	cnstypes "github.com/Azure/azure-container-networking/cns/types"
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
	reqsByNCID map[string]*cns.CreateNetworkContainerRequest
	nnc        *v1alpha.NodeNetworkConfig
}

type mockCNSClient struct {
	state            cnsClientState
	createOrUpdateNC func(*cns.CreateNetworkContainerRequest) cnstypes.ResponseCode
	update           func(*v1alpha.NodeNetworkConfig) error
	lastGoal         restserver.NNCGoal
}

func (m *mockCNSClient) ApplyNNCGoal(goal restserver.NNCGoal) cnstypes.ResponseCode {
	m.lastGoal = goal
	for i := range goal.NetworkContainers {
		req := &goal.NetworkContainers[i].Request
		if m.createOrUpdateNC != nil {
			if responseCode := m.createOrUpdateNC(req); responseCode != cnstypes.Success {
				return responseCode
			}
		}
	}

	valid := make(map[string]struct{}, len(goal.NetworkContainerIDs))
	for _, ncID := range goal.NetworkContainerIDs {
		valid[ncID] = struct{}{}
	}
	for ncID := range m.state.reqsByNCID {
		if _, ok := valid[ncID]; !ok {
			delete(m.state.reqsByNCID, ncID)
		}
	}
	for i := range goal.NetworkContainers {
		req := goal.NetworkContainers[i].Request
		m.state.reqsByNCID[req.NetworkContainerid] = &req
	}
	return cnstypes.Success
}

func (m *mockCNSClient) Update(nnc *v1alpha.NodeNetworkConfig) error {
	m.state.nnc = nnc
	return m.update(nnc)
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
		nodeIP             string
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
						Status: validSwiftStatus,
					}, nil
				},
			},
			cnsClient: mockCNSClient{
				createOrUpdateNC: func(*cns.CreateNetworkContainerRequest) cnstypes.ResponseCode {
					return cnstypes.UnexpectedError
				},
			},
			wantErr: true,
			wantCNSClientState: cnsClientState{
				reqsByNCID: map[string]*cns.CreateNetworkContainerRequest{},
			},
		},
		{
			name: "success",
			ncGetter: mockNCGetter{
				get: func(context.Context, types.NamespacedName) (*v1alpha.NodeNetworkConfig, error) {
					return &v1alpha.NodeNetworkConfig{
						Status: validSwiftStatus,
						Spec: v1alpha.NodeNetworkConfigSpec{
							RequestedIPCount: 1,
						},
					}, nil
				},
			},
			cnsClient: mockCNSClient{
				createOrUpdateNC: func(*cns.CreateNetworkContainerRequest) cnstypes.ResponseCode {
					return cnstypes.Success
				},
				update: func(*v1alpha.NodeNetworkConfig) error {
					return nil
				},
			},
			wantErr: false,
			wantCNSClientState: cnsClientState{
				reqsByNCID: map[string]*cns.CreateNetworkContainerRequest{validSwiftRequest.NetworkContainerid: validSwiftRequest},
				nnc: &v1alpha.NodeNetworkConfig{
					Status: validSwiftStatus,
					Spec: v1alpha.NodeNetworkConfigSpec{
						RequestedIPCount: 1,
					},
				},
			},
		},
		{
			name: "node IP mismatch",
			ncGetter: mockNCGetter{
				get: func(context.Context, types.NamespacedName) (*v1alpha.NodeNetworkConfig, error) {
					return &v1alpha.NodeNetworkConfig{
						Status: validSwiftStatus,
						Spec: v1alpha.NodeNetworkConfigSpec{
							RequestedIPCount: 1,
						},
					}, nil
				},
			},
			cnsClient: mockCNSClient{
				createOrUpdateNC: func(*cns.CreateNetworkContainerRequest) cnstypes.ResponseCode {
					return cnstypes.Success
				},
				update: func(*v1alpha.NodeNetworkConfig) error {
					return nil
				},
			},
			nodeIP:             "192.168.1.5", // nodeIP in above NNC status is 10.1.0.5
			wantErr:            false,
			wantCNSClientState: cnsClientState{}, // state should be empty since we should skip this NC
		},
	}
	for _, tt := range tests {
		tt := tt
		tt.cnsClient.state.reqsByNCID = make(map[string]*cns.CreateNetworkContainerRequest)
		if tt.wantCNSClientState.reqsByNCID == nil {
			tt.wantCNSClientState.reqsByNCID = make(map[string]*cns.CreateNetworkContainerRequest)
		}

		t.Run(tt.name, func(t *testing.T) {
			r := NewReconciler(&tt.cnsClient, func(*v1alpha.NodeNetworkConfig) error { return nil }, &tt.cnsClient, tt.nodeIP, false, 0)
			r.nnccli = &tt.ncGetter
			got, err := r.Reconcile(context.Background(), tt.in)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.wantCNSClientState, tt.cnsClient.state)
		})
	}
}

func TestReconcileStaleNCs(t *testing.T) {
	logger.InitLogger("", 0, 0, "")

	cnsClient := mockCNSClient{
		state:            cnsClientState{reqsByNCID: make(map[string]*cns.CreateNetworkContainerRequest)},
		createOrUpdateNC: func(*cns.CreateNetworkContainerRequest) cnstypes.ResponseCode { return cnstypes.Success },
		update:           func(*v1alpha.NodeNetworkConfig) error { return nil },
	}

	nodeIP := "10.0.0.10"

	nncv1 := v1alpha.NodeNetworkConfig{
		Status: v1alpha.NodeNetworkConfigStatus{
			NetworkContainers: []v1alpha.NetworkContainer{
				{ID: "nc1", PrimaryIP: "10.1.0.10", SubnetAddressSpace: "10.1.0.0/24", NodeIP: nodeIP},
				{ID: "nc2", PrimaryIP: "10.1.0.11", SubnetAddressSpace: "10.1.0.0/24", NodeIP: nodeIP},
			},
		},
		Spec: v1alpha.NodeNetworkConfigSpec{RequestedIPCount: 10},
	}

	nncv2 := v1alpha.NodeNetworkConfig{
		Status: v1alpha.NodeNetworkConfigStatus{
			NetworkContainers: []v1alpha.NetworkContainer{
				{ID: "nc3", PrimaryIP: "10.1.0.12", SubnetAddressSpace: "10.1.0.0/24", NodeIP: nodeIP},
				{ID: "nc4", PrimaryIP: "10.1.0.13", SubnetAddressSpace: "10.1.0.0/24", NodeIP: nodeIP},
			},
		},
		Spec: v1alpha.NodeNetworkConfigSpec{RequestedIPCount: 10},
	}

	i := 0
	nncIterator := func(context.Context, types.NamespacedName) (*v1alpha.NodeNetworkConfig, error) {
		nncLog := []v1alpha.NodeNetworkConfig{nncv1, nncv2}
		for i < len(nncLog) {
			j := i
			i++
			return &nncLog[j], nil
		}

		return &nncLog[len(nncLog)-1], nil
	}

	r := NewReconciler(&cnsClient, func(*v1alpha.NodeNetworkConfig) error { return nil }, &cnsClient, nodeIP, false, 0)
	r.nnccli = &mockNCGetter{get: nncIterator}

	_, err := r.Reconcile(context.Background(), reconcile.Request{})
	require.NoError(t, err)

	assert.Contains(t, cnsClient.state.reqsByNCID, "nc1")
	assert.Contains(t, cnsClient.state.reqsByNCID, "nc2")

	_, err = r.Reconcile(context.Background(), reconcile.Request{})
	require.NoError(t, err)

	assert.NotContains(t, cnsClient.state.reqsByNCID, "nc1")
	assert.NotContains(t, cnsClient.state.reqsByNCID, "nc2")
	assert.Contains(t, cnsClient.state.reqsByNCID, "nc3")
	assert.Contains(t, cnsClient.state.reqsByNCID, "nc4")
}

func TestReconcileIncludesFilteredNetworkContainerInCompleteGoal(t *testing.T) {
	logger.InitLogger("", 0, 0, "") //nolint:staticcheck // Keep test setup consistent with this package.

	const localNodeIP = "10.0.0.10"
	nnc := &v1alpha.NodeNetworkConfig{
		Status: v1alpha.NodeNetworkConfigStatus{
			NetworkContainers: []v1alpha.NetworkContainer{
				{ID: "local", PrimaryIP: "10.1.0.10", SubnetAddressSpace: "10.1.0.0/24", NodeIP: localNodeIP},
				{ID: "filtered", PrimaryIP: "10.1.0.11", SubnetAddressSpace: "10.1.0.0/24", NodeIP: "10.0.0.11"},
			},
		},
	}
	cnsClient := mockCNSClient{
		state:            cnsClientState{reqsByNCID: make(map[string]*cns.CreateNetworkContainerRequest)},
		createOrUpdateNC: func(*cns.CreateNetworkContainerRequest) cnstypes.ResponseCode { return cnstypes.Success },
		update:           func(*v1alpha.NodeNetworkConfig) error { return nil },
	}
	ncGetter := mockNCGetter{get: func(context.Context, types.NamespacedName) (*v1alpha.NodeNetworkConfig, error) {
		return nnc, nil
	}}
	r := NewReconciler(&cnsClient, func(*v1alpha.NodeNetworkConfig) error { return nil }, &cnsClient, localNodeIP, false, 0)
	r.nnccli = &ncGetter

	_, err := r.Reconcile(context.Background(), reconcile.Request{})

	require.NoError(t, err)
	assert.Equal(t, []string{"local", "filtered"}, cnsClient.lastGoal.NetworkContainerIDs)
	require.Len(t, cnsClient.lastGoal.NetworkContainers, 1)
	assert.Equal(t, "local", cnsClient.lastGoal.NetworkContainers[0].Request.NetworkContainerid)
}

func TestReconcileInitializerRunsOnceOnSuccess(t *testing.T) {
	logger.InitLogger("", 0, 0, "")

	initializerCalls := 0
	initializer := func(*v1alpha.NodeNetworkConfig) error {
		initializerCalls++
		return nil
	}

	cnsClient := mockCNSClient{
		state:            cnsClientState{reqsByNCID: make(map[string]*cns.CreateNetworkContainerRequest)},
		createOrUpdateNC: func(*cns.CreateNetworkContainerRequest) cnstypes.ResponseCode { return cnstypes.Success },
		update:           func(*v1alpha.NodeNetworkConfig) error { return nil },
	}

	ncGetter := mockNCGetter{get: func(context.Context, types.NamespacedName) (*v1alpha.NodeNetworkConfig, error) {
		return &v1alpha.NodeNetworkConfig{Status: validSwiftStatus}, nil
	}}

	r := NewReconciler(&cnsClient, initializer, &cnsClient, "", false, 0)
	r.nnccli = &ncGetter

	_, err := r.Reconcile(context.Background(), reconcile.Request{})
	require.NoError(t, err)
	_, err = r.Reconcile(context.Background(), reconcile.Request{})
	require.NoError(t, err)

	assert.Equal(t, 1, initializerCalls)
	assert.Nil(t, r.initializer)
}

func TestReconcileRemovesStaleNetworkContainersBeforeInitializer(t *testing.T) {
	logger.InitLogger("", 0, 0, "") //nolint:staticcheck // Keep test setup consistent with this package.

	cnsClient := mockCNSClient{
		state: cnsClientState{
			reqsByNCID: map[string]*cns.CreateNetworkContainerRequest{
				"stale": {NetworkContainerid: "stale"},
			},
		},
		createOrUpdateNC: func(*cns.CreateNetworkContainerRequest) cnstypes.ResponseCode { return cnstypes.Success },
		update:           func(*v1alpha.NodeNetworkConfig) error { return nil },
	}
	initializer := func(*v1alpha.NodeNetworkConfig) error {
		if _, exists := cnsClient.state.reqsByNCID["stale"]; exists {
			return errors.New("stale network container remains")
		}
		return nil
	}
	ncGetter := mockNCGetter{get: func(context.Context, types.NamespacedName) (*v1alpha.NodeNetworkConfig, error) {
		return &v1alpha.NodeNetworkConfig{Status: validSwiftStatus}, nil
	}}
	r := NewReconciler(&cnsClient, initializer, &cnsClient, "", false, 0)
	r.nnccli = &ncGetter

	_, err := r.Reconcile(context.Background(), reconcile.Request{})

	require.NoError(t, err)
	assert.NotContains(t, cnsClient.state.reqsByNCID, "stale")
	assert.Nil(t, r.initializer)
}

func TestReconcileInitializerRetriesAfterFailure(t *testing.T) {
	logger.InitLogger("", 0, 0, "")

	initializerCalls := 0
	initializer := func(*v1alpha.NodeNetworkConfig) error {
		initializerCalls++
		if initializerCalls == 1 {
			return errors.New("init failed")
		}
		return nil
	}

	createCalls := 0
	cnsClient := mockCNSClient{
		state: cnsClientState{reqsByNCID: make(map[string]*cns.CreateNetworkContainerRequest)},
		createOrUpdateNC: func(*cns.CreateNetworkContainerRequest) cnstypes.ResponseCode {
			createCalls++
			return cnstypes.Success
		},
		update: func(*v1alpha.NodeNetworkConfig) error { return nil },
	}

	ncGetter := mockNCGetter{get: func(context.Context, types.NamespacedName) (*v1alpha.NodeNetworkConfig, error) {
		return &v1alpha.NodeNetworkConfig{Status: validSwiftStatus}, nil
	}}

	r := NewReconciler(&cnsClient, initializer, &cnsClient, "", false, 0)
	r.nnccli = &ncGetter

	_, err := r.Reconcile(context.Background(), reconcile.Request{})
	require.Error(t, err)
	assert.NotNil(t, r.initializer)
	assert.Equal(t, 1, initializerCalls)
	assert.Equal(t, 1, createCalls)

	_, err = r.Reconcile(context.Background(), reconcile.Request{})
	require.NoError(t, err)
	assert.Equal(t, 2, initializerCalls)
	assert.Equal(t, 2, createCalls)
	assert.Nil(t, r.initializer)
}
