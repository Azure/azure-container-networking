package platform

import (
	"context"
	"errors"
	"os/exec"
	"testing"
	"time"

	"github.com/Azure/azure-container-networking/platform/windows/adapter/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows/svc"
)

var errTestFailure = errors.New("test failure")

// Test if hasNetworkAdapter returns false on actual error or empty adapter name(an error)
func TestHasNetworkAdapterReturnsError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockNetworkAdapter := mocks.NewMockNetworkAdapter(ctrl)
	mockNetworkAdapter.EXPECT().GetAdapterName().Return("", errTestFailure)

	result := hasNetworkAdapter(mockNetworkAdapter)
	assert.False(t, result)
}

// Test if hasNetworkAdapter returns false on actual error or empty adapter name(an error)
func TestHasNetworkAdapterAdapterReturnsEmptyAdapterName(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockNetworkAdapter := mocks.NewMockNetworkAdapter(ctrl)
	mockNetworkAdapter.EXPECT().GetAdapterName().Return("Ethernet 3", nil)

	result := hasNetworkAdapter(mockNetworkAdapter)
	assert.True(t, result)
}

// Test if updatePriorityVLANTagIfRequired returns error on getting error on calling getpriorityvlantag
func TestUpdatePriorityVLANTagIfRequiredReturnsError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockNetworkAdapter := mocks.NewMockNetworkAdapter(ctrl)
	mockNetworkAdapter.EXPECT().GetPriorityVLANTag().Return(0, errTestFailure)
	result := updatePriorityVLANTagIfRequired(mockNetworkAdapter, 3)
	assert.EqualError(t, result, "error while getting Priority VLAN Tag value: test failure")
}

// Test if updatePriorityVLANTagIfRequired returns nil if currentval == desiredvalue (SetPriorityVLANTag not being called)
func TestUpdatePriorityVLANTagIfRequiredIfCurrentValEqualDesiredValue(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockNetworkAdapter := mocks.NewMockNetworkAdapter(ctrl)
	mockNetworkAdapter.EXPECT().GetPriorityVLANTag().Return(4, nil)
	result := updatePriorityVLANTagIfRequired(mockNetworkAdapter, 4)
	assert.NoError(t, result)
}

// Test if updatePriorityVLANTagIfRequired returns nil if SetPriorityVLANTag being called to set value
func TestUpdatePriorityVLANTagIfRequiredIfCurrentValNotEqualDesiredValAndSetReturnsNoError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockNetworkAdapter := mocks.NewMockNetworkAdapter(ctrl)
	mockNetworkAdapter.EXPECT().GetPriorityVLANTag().Return(1, nil)
	mockNetworkAdapter.EXPECT().SetPriorityVLANTag(2).Return(nil)
	result := updatePriorityVLANTagIfRequired(mockNetworkAdapter, 2)
	assert.NoError(t, result)
}

// Test if updatePriorityVLANTagIfRequired returns error if SetPriorityVLANTag throwing error

func TestUpdatePriorityVLANTagIfRequiredIfCurrentValNotEqualDesiredValAndSetReturnsError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockNetworkAdapter := mocks.NewMockNetworkAdapter(ctrl)
	mockNetworkAdapter.EXPECT().GetPriorityVLANTag().Return(1, nil)
	mockNetworkAdapter.EXPECT().SetPriorityVLANTag(5).Return(errTestFailure)
	result := updatePriorityVLANTagIfRequired(mockNetworkAdapter, 5)
	assert.EqualError(t, result, "error while setting Priority VLAN Tag value: test failure")
}

func TestExecuteRawCommand(t *testing.T) {
	out, err := NewExecClient(nil).ExecuteRawCommand("dir")
	require.NoError(t, err)
	require.NotEmpty(t, out)
}

func TestExecuteRawCommandError(t *testing.T) {
	_, err := NewExecClient(nil).ExecuteRawCommand("dontaddtopath")
	require.Error(t, err)

	var xErr *exec.ExitError
	assert.ErrorAs(t, err, &xErr)
	assert.Equal(t, 1, xErr.ExitCode())
}

func TestExecuteCommand(t *testing.T) {
	_, err := NewExecClient(nil).ExecuteCommand(context.Background(), "ping", "localhost")
	if err != nil {
		t.Errorf("TestExecuteCommand failed with error %v", err)
	}
}

func TestExecuteCommandError(t *testing.T) {
	_, err := NewExecClient(nil).ExecuteCommand(context.Background(), "dontaddtopath")
	require.Error(t, err)
	require.ErrorIs(t, err, exec.ErrNotFound)
}

func TestFetchPnpIDMapping(t *testing.T) {
	mockExecClient := NewMockExecClient(false)
	// happy path
	mockExecClient.SetPowershellCommandResponder(func(cmd string) (string, error) {
		return "6C-A1-00-50-E4-2D PCI\\VEN_8086&DEV_2723&SUBSYS_00808086&REV_1A\\4&328243d9&0&00E0\n80-6D-97-1E-CF-4E USB\\VID_17EF&PID_A359\\3010019E3", nil
	})
	vfmapping, _ := FetchMacAddressPnpIDMapping(context.Background(), mockExecClient)
	require.Len(t, vfmapping, 2)

	// Test when no adapters are found
	mockExecClient.SetPowershellCommandResponder(func(cmd string) (string, error) {
		return "", nil
	})
	vfmapping, _ = FetchMacAddressPnpIDMapping(context.Background(), mockExecClient)
	require.Empty(t, vfmapping)
	// Adding carriage returns
	mockExecClient.SetPowershellCommandResponder(func(cmd string) (string, error) {
		return "6C-A1-00-50-E4-2D PCI\\VEN_8086&DEV_2723&SUBSYS_00808086&REV_1A\\4&328243d9&0&00E0\r\n\r80-6D-97-1E-CF-4E USB\\VID_17EF&PID_A359\\3010019E3", nil
	})

	vfmapping, _ = FetchMacAddressPnpIDMapping(context.Background(), mockExecClient)
	require.Len(t, vfmapping, 2)
}

// ping -t localhost will ping indefinitely and should exceed the 5 second timeout
func TestExecuteCommandTimeout(t *testing.T) {
	const timeout = 5 * time.Second
	client := NewExecClientTimeout(timeout)

	_, err := client.ExecuteCommand(context.Background(), "ping", "-t", "localhost")
	require.Error(t, err)
}

type mockManagedService struct {
	queryFuncs  []func() (svc.Status, error)
	controlFunc func(svc.Cmd) (svc.Status, error)
	startFunc   func(args ...string) error
}

func (m *mockManagedService) Query() (svc.Status, error) {
	queryFunc := m.queryFuncs[0]
	m.queryFuncs = m.queryFuncs[1:]
	return queryFunc()
}

func (m *mockManagedService) Control(cmd svc.Cmd) (svc.Status, error) {
	return m.controlFunc(cmd)
}

func (m *mockManagedService) Start(args ...string) error {
	return m.startFunc(args...)
}

func TestTryStopServiceFn(t *testing.T) {
	tests := []struct {
		name        string
		queryFuncs  []func() (svc.Status, error)
		controlFunc func(svc.Cmd) (svc.Status, error)
		expectError bool
	}{
		{
			name: "Service already stopped",
			queryFuncs: []func() (svc.Status, error){
				func() (svc.Status, error) {
					return svc.Status{State: svc.Stopped}, nil
				},
				func() (svc.Status, error) {
					return svc.Status{State: svc.Stopped}, nil
				},
			},
			controlFunc: nil,
			expectError: false,
		},
		{
			name: "Service running and stops successfully",
			queryFuncs: []func() (svc.Status, error){
				func() (svc.Status, error) {
					return svc.Status{State: svc.Running}, nil
				},
				func() (svc.Status, error) {
					return svc.Status{State: svc.Stopped}, nil
				},
			},
			controlFunc: func(svc.Cmd) (svc.Status, error) {
				return svc.Status{State: svc.Stopped}, nil
			},
			expectError: false,
		},
		{
			name: "Service running and stops after multiple attempts",
			queryFuncs: []func() (svc.Status, error){
				func() (svc.Status, error) {
					return svc.Status{State: svc.Running}, nil
				},
				func() (svc.Status, error) {
					return svc.Status{State: svc.Running}, nil
				},
				func() (svc.Status, error) {
					return svc.Status{State: svc.Running}, nil
				},
				func() (svc.Status, error) {
					return svc.Status{State: svc.Stopped}, nil
				},
			},
			controlFunc: func(svc.Cmd) (svc.Status, error) {
				return svc.Status{State: svc.Stopped}, nil
			},
			expectError: false,
		},
		{
			name: "Service running and fails to stop",
			queryFuncs: []func() (svc.Status, error){
				func() (svc.Status, error) {
					return svc.Status{State: svc.Running}, nil
				},
			},
			controlFunc: func(svc.Cmd) (svc.Status, error) {
				return svc.Status{State: svc.Running}, errors.New("failed to stop service") //nolint:err113 // test error
			},
			expectError: true,
		},
		{
			name: "Service query fails",
			queryFuncs: []func() (svc.Status, error){
				func() (svc.Status, error) {
					return svc.Status{}, errors.New("failed to query service status") //nolint:err113 // test error
				},
			},
			controlFunc: nil,
			expectError: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := &mockManagedService{
				queryFuncs:  tt.queryFuncs,
				controlFunc: tt.controlFunc,
			}
			err := tryStopServiceFn(context.Background(), service)()
			if tt.expectError {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
		})
	}
}

func TestTryStartServiceFn(t *testing.T) {
	tests := []struct {
		name        string
		queryFuncs  []func() (svc.Status, error)
		startFunc   func(...string) error
		expectError bool
	}{
		{
			name: "Service already running",
			queryFuncs: []func() (svc.Status, error){
				func() (svc.Status, error) {
					return svc.Status{State: svc.Running}, nil
				},
				func() (svc.Status, error) {
					return svc.Status{State: svc.Running}, nil
				},
			},
			startFunc:   nil,
			expectError: false,
		},
		{
			name: "Service already starting",
			queryFuncs: []func() (svc.Status, error){
				func() (svc.Status, error) {
					return svc.Status{State: svc.StartPending}, nil
				},
				func() (svc.Status, error) {
					return svc.Status{State: svc.Running}, nil
				},
			},
			startFunc:   nil,
			expectError: false,
		},
		{
			name: "Service starts successfully",
			queryFuncs: []func() (svc.Status, error){
				func() (svc.Status, error) {
					return svc.Status{State: svc.Stopped}, nil
				},
				func() (svc.Status, error) {
					return svc.Status{State: svc.Running}, nil
				},
			},
			startFunc: func(...string) error {
				return nil
			},
			expectError: false,
		},
		{
			name: "Service fails to start",
			queryFuncs: []func() (svc.Status, error){
				func() (svc.Status, error) {
					return svc.Status{State: svc.Stopped}, nil
				},
			},
			startFunc: func(...string) error {
				return errors.New("failed to start service") //nolint:err113 // test error
			},
			expectError: true,
		},
		{
			name: "Service query fails",
			queryFuncs: []func() (svc.Status, error){
				func() (svc.Status, error) {
					return svc.Status{}, errors.New("failed to query service status") //nolint:err113 // test error
				},
			},
			startFunc:   nil,
			expectError: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := &mockManagedService{
				queryFuncs: tt.queryFuncs,
				startFunc:  tt.startFunc,
			}
			err := tryStartServiceFn(context.Background(), service)()
			if tt.expectError {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
		})
	}
}
