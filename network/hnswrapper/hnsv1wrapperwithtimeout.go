//go:build windows
// +build windows

package hnswrapper

import (
	"context"
	"time"

	"github.com/Microsoft/hcsshim"
	"github.com/pkg/errors"
)

type Hnsv1wrapperwithtimeout struct {
	Hnsv1 HnsV1WrapperInterface
	// hnsCallTimeout indicates the time to wait for hns calls before timing out
	HnsCallTimeout time.Duration
}

type EndpointFuncResult struct {
	endpoint *hcsshim.HNSEndpoint
	Err      error
}

type NetworkFuncResult struct {
	network *hcsshim.HNSNetwork
	Err     error
}

type EndpointAttachedFuncResult struct {
	isAttached bool
	Err        error
}

type HnsGlobalFuncResult struct {
	HnsGlobals *hcsshim.HNSGlobals
	Err        error
}

func (h Hnsv1wrapperwithtimeout) CreateEndpoint(endpoint *hcsshim.HNSEndpoint, path string) (*hcsshim.HNSEndpoint, error) {
	r := make(chan EndpointFuncResult)
	ctx, cancel := context.WithTimeout(context.TODO(), h.HnsCallTimeout)
	defer cancel()

	go func() {
		endpoint, err := h.Hnsv1.CreateEndpoint(endpoint, path)

		r <- EndpointFuncResult{
			endpoint: endpoint,
			Err:      err,
		}
	}()

	select {
	case res := <-r:
		return res.endpoint, res.Err
	case <-ctx.Done():
		return nil, errors.Wrapf(ErrHNSCallTimeout, "CreateEndpoint timeout value is %v ", h.HnsCallTimeout.String())
	}
}

func (h Hnsv1wrapperwithtimeout) DeleteEndpoint(endpointId string) (*hcsshim.HNSEndpoint, error) {
	r := make(chan EndpointFuncResult)
	ctx, cancel := context.WithTimeout(context.TODO(), h.HnsCallTimeout)
	defer cancel()

	go func() {
		endpoint, err := h.Hnsv1.DeleteEndpoint(endpointId)

		r <- EndpointFuncResult{
			endpoint: endpoint,
			Err:      err,
		}
	}()

	select {
	case res := <-r:
		return res.endpoint, res.Err
	case <-ctx.Done():
		return nil, errors.Wrapf(ErrHNSCallTimeout, "DeleteEndpoint timeout value is %v ", h.HnsCallTimeout.String())
	}
}

func (h Hnsv1wrapperwithtimeout) CreateNetwork(network *hcsshim.HNSNetwork, path string) (*hcsshim.HNSNetwork, error) {
	r := make(chan NetworkFuncResult)
	ctx, cancel := context.WithTimeout(context.TODO(), h.HnsCallTimeout)
	defer cancel()

	go func() {
		network, err := h.Hnsv1.CreateNetwork(network, path)

		r <- NetworkFuncResult{
			network: network,
			Err:     err,
		}
	}()

	select {
	case res := <-r:
		return res.network, res.Err
	case <-ctx.Done():
		return nil, errors.Wrapf(ErrHNSCallTimeout, "CreateNetwork timeout value is %v ", h.HnsCallTimeout.String())
	}
}

func (h Hnsv1wrapperwithtimeout) DeleteNetwork(networkId string) (*hcsshim.HNSNetwork, error) {
	r := make(chan NetworkFuncResult)
	ctx, cancel := context.WithTimeout(context.TODO(), h.HnsCallTimeout)
	defer cancel()

	go func() {
		network, err := h.Hnsv1.DeleteNetwork(networkId)

		r <- NetworkFuncResult{
			network: network,
			Err:     err,
		}
	}()

	select {
	case res := <-r:
		return res.network, res.Err
	case <-ctx.Done():
		return nil, errors.Wrapf(ErrHNSCallTimeout, "DeleteNetwork timeout value is %v ", h.HnsCallTimeout.String())
	}
}

func (h Hnsv1wrapperwithtimeout) GetHNSEndpointByName(endpointName string) (*hcsshim.HNSEndpoint, error) {
	r := make(chan EndpointFuncResult)
	ctx, cancel := context.WithTimeout(context.TODO(), h.HnsCallTimeout)
	defer cancel()

	go func() {
		endpoint, err := h.Hnsv1.GetHNSEndpointByName(endpointName)

		r <- EndpointFuncResult{
			endpoint: endpoint,
			Err:      err,
		}
	}()

	select {
	case res := <-r:
		return res.endpoint, res.Err
	case <-ctx.Done():
		return nil, errors.Wrapf(ErrHNSCallTimeout, "GetHNSEndpointByName timeout value is %v ", h.HnsCallTimeout.String())
	}
}

func (h Hnsv1wrapperwithtimeout) GetHNSEndpointByID(endpointID string) (*hcsshim.HNSEndpoint, error) {
	r := make(chan EndpointFuncResult)
	ctx, cancel := context.WithTimeout(context.TODO(), h.HnsCallTimeout)
	defer cancel()

	go func() {
		endpoint, err := h.Hnsv1.GetHNSEndpointByName(endpointID)

		r <- EndpointFuncResult{
			endpoint: endpoint,
			Err:      err,
		}
	}()

	select {
	case res := <-r:
		return res.endpoint, res.Err
	case <-ctx.Done():
		return nil, errors.Wrapf(ErrHNSCallTimeout, "GetHNSEndpointByID timeout value is %v ", h.HnsCallTimeout.String())
	}
}

func (h Hnsv1wrapperwithtimeout) HotAttachEndpoint(containerID string, endpointID string) error {
	r := make(chan error)
	ctx, cancel := context.WithTimeout(context.TODO(), h.HnsCallTimeout)
	defer cancel()

	go func() {
		r <- h.Hnsv1.HotAttachEndpoint(containerID, endpointID)
	}()

	select {
	case res := <-r:
		return res
	case <-ctx.Done():
		return errors.Wrapf(ErrHNSCallTimeout, "HotAttachEndpoint timeout value is %v ", h.HnsCallTimeout.String())
	}
}

func (h Hnsv1wrapperwithtimeout) IsAttached(hnsep *hcsshim.HNSEndpoint, containerID string) (bool, error) {
	r := make(chan EndpointAttachedFuncResult)
	ctx, cancel := context.WithTimeout(context.TODO(), h.HnsCallTimeout)
	defer cancel()

	go func() {
		isAttached, err := h.Hnsv1.IsAttached(hnsep, containerID)

		r <- EndpointAttachedFuncResult{
			isAttached: isAttached,
			Err:        err,
		}
	}()

	select {
	case res := <-r:
		return res.isAttached, res.Err
	case <-ctx.Done():
		return false, errors.Wrapf(ErrHNSCallTimeout, "IsHnsEndpointAttached timeout value is %v ", h.HnsCallTimeout.String())
	}
}

func (h Hnsv1wrapperwithtimeout) GetHNSGlobals() (*hcsshim.HNSGlobals, error) {
	r := make(chan HnsGlobalFuncResult)
	ctx, cancel := context.WithTimeout(context.TODO(), h.HnsCallTimeout)
	defer cancel()

	go func() {
		hnsGlobals, err := h.Hnsv1.GetHNSGlobals()

		r <- HnsGlobalFuncResult{
			HnsGlobals: hnsGlobals,
			Err:        err,
		}
	}()

	select {
	case res := <-r:
		return res.HnsGlobals, res.Err
	case <-ctx.Done():
		return nil, errors.Wrapf(ErrHNSCallTimeout, "GetHNSGlobals timeout value is %v ", h.HnsCallTimeout.String())
	}
}
