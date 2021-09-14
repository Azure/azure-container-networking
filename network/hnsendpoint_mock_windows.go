package network

import (
	"errors"
	"github.com/Microsoft/hcsshim"
)

type MockHNSEndpoint struct {
	HnsIDMap         map[string]*hcsshim.HNSEndpoint
	IsAttachedFlag   bool
	HotAttachFailure bool
}

func NewMockHNSEndpoint(isAttached bool, hotAttachFailure bool) *MockHNSEndpoint {
	return &MockHNSEndpoint{HnsIDMap: make(map[string]*hcsshim.HNSEndpoint),
		IsAttachedFlag:   isAttached,
		HotAttachFailure: hotAttachFailure}
}

func (az *MockHNSEndpoint) GetHNSEndpointByName(endpointName string) (*hcsshim.HNSEndpoint, error) {
	hnsep := &hcsshim.HNSEndpoint{Id: "test"}
	az.HnsIDMap["test"] = hnsep
	return hnsep, nil
}

func (az *MockHNSEndpoint) GetHNSEndpointByID(id string) (*hcsshim.HNSEndpoint, error) {
	return az.HnsIDMap[id], nil
}

func (az *MockHNSEndpoint) HotAttachEndpoint(containerID, endpointID string) error {
	if az.HotAttachFailure {
		return errors.New("hotattach error")
	}

	return nil
}

func (az *MockHNSEndpoint) IsAttached(hnsep *hcsshim.HNSEndpoint, containerID string) (bool, error) {
	return az.IsAttachedFlag, nil
}
