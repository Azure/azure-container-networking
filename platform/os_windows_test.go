package platform

import (
	"errors"
	"os/exec"
	"strings"
	"testing"

	"github.com/Azure/azure-container-networking/platform/windows/adapter/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestExecuteCommand(t *testing.T) {
	out, err := NewExecClient(nil).ExecuteCommand("dir")
	require.NoError(t, err)
	require.NotEmpty(t, out)
}

func TestExecuteCommandError(t *testing.T) {
	_, err := NewExecClient(nil).ExecuteCommand("dontaddtopath")
	require.Error(t, err)

	var xErr *exec.ExitError
	assert.ErrorAs(t, err, &xErr)
	assert.Equal(t, 1, xErr.ExitCode())
}

func TestSetSdnRemoteArpMacAddress_hnsNotEnabled(t *testing.T) {
	mockExecClient := NewMockExecClient(false)
	// testing skip setting SdnRemoteArpMacAddress when hns not enabled
	mockExecClient.SetPowershellCommandResponder(func(_ string) (string, error) {
		return "False", nil
	})
	err := SetSdnRemoteArpMacAddress(mockExecClient)
	assert.NoError(t, err)
	assert.Equal(t, false, sdnRemoteArpMacAddressSet)

	// testing the scenario when there is an error in checking if hns is enabled or not
	mockExecClient.SetPowershellCommandResponder(func(_ string) (string, error) {
		return "", errTestFailure
	})
	err = SetSdnRemoteArpMacAddress(mockExecClient)
	assert.ErrorAs(t, err, &errTestFailure)
	assert.Equal(t, false, sdnRemoteArpMacAddressSet)
}

func TestSetSdnRemoteArpMacAddress_hnsEnabled(t *testing.T) {
	mockExecClient := NewMockExecClient(false)
	// happy path
	mockExecClient.SetPowershellCommandResponder(func(cmd string) (string, error) {
		if strings.Contains(cmd, "Test-Path") {
			return "True", nil
		}
		return "", nil
	})
	err := SetSdnRemoteArpMacAddress(mockExecClient)
	assert.NoError(t, err)
	assert.Equal(t, true, sdnRemoteArpMacAddressSet)
	// reset sdnRemoteArpMacAddressSet
	sdnRemoteArpMacAddressSet = false
}
