//go:build windows
// +build windows

package hnswrapper

import (
	"time"

	"github.com/Microsoft/hcsshim"
)

type Hnsv1wrapperfake struct {
	Delay time.Duration
}

func NewHnsv1wrapperFake() *Hnsv1wrapperfake {
	return &Hnsv1wrapperfake{}
}

func (h Hnsv1wrapperfake) CreateEndpoint(endpoint *hcsshim.HNSEndpoint, path string) (*hcsshim.HNSEndpoint, error) {
	delayHnsCall(h.Delay)
	return &hcsshim.HNSEndpoint{}, nil
}

func (h Hnsv1wrapperfake) DeleteEndpoint(endpointId string) (*hcsshim.HNSEndpoint, error) {
	delayHnsCall(h.Delay)
	return &hcsshim.HNSEndpoint{}, nil
}

func (h Hnsv1wrapperfake) CreateNetwork(network *hcsshim.HNSNetwork, path string) (*hcsshim.HNSNetwork, error) {
	delayHnsCall(h.Delay)
	return &hcsshim.HNSNetwork{}, nil
}

func (h Hnsv1wrapperfake) DeleteNetwork(networkId string) (*hcsshim.HNSNetwork, error) {
	delayHnsCall(h.Delay)
	return &hcsshim.HNSNetwork{}, nil
}

func (h Hnsv1wrapperfake) GetHNSEndpointByName(endpointName string) (*hcsshim.HNSEndpoint, error) {
	delayHnsCall(h.Delay)
	return &hcsshim.HNSEndpoint{}, nil
}

func (h Hnsv1wrapperfake) GetHNSEndpointByID(endpointID string) (*hcsshim.HNSEndpoint, error) {
	delayHnsCall(h.Delay)
	return &hcsshim.HNSEndpoint{}, nil
}

func (h Hnsv1wrapperfake) HotAttachEndpoint(containerID string, endpointID string) error {
	delayHnsCall(h.Delay)
	return nil
}

func (h Hnsv1wrapperfake) IsAttached(hnsep *hcsshim.HNSEndpoint, containerID string) (bool, error) {
	delayHnsCall(h.Delay)
	return true, nil
}

func (h Hnsv1wrapperfake) GetHNSGlobals() (*hcsshim.HNSGlobals, error) {
	delayHnsCall(h.Delay)
	return &hcsshim.HNSGlobals{}, nil
}
