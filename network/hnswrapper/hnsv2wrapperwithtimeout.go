//go:build windows
// +build windows

package hnswrapper

import (
	"context"
	"fmt"
	"time"

	"github.com/pkg/errors"

	"github.com/Microsoft/hcsshim/hcn"
)

// ErrHNSCallTimeout - hns call timeout
var ErrHNSCallTimeout = errors.New("timed out calling hns")

type Hnsv2wrapperwithtimeout struct {
	Hnsv2 HnsV2WrapperInterface
	// hnsCallTimeout indicates the time to wait for hns calls before timing out
	HnsCallTimeout time.Duration
}

type CreateEndpointFuncResult struct {
	endpoint *hcn.HostComputeEndpoint
	Err      error
}

type GetEndpointByIDFuncResult struct {
	endpoint *hcn.HostComputeEndpoint
	Err      error
}

type ListEndpointsFuncResult struct {
	endpoints []hcn.HostComputeEndpoint
	Err       error
}

type CreateNetworkFuncResult struct {
	network *hcn.HostComputeNetwork
	Err     error
}

type GetNamespaceByIDFuncResult struct {
	namespace *hcn.HostComputeNamespace
	Err       error
}

type GetNetworkByNameFuncResult struct {
	network *hcn.HostComputeNetwork
	Err     error
}

type GetNetworkByIDFuncResult struct {
	network *hcn.HostComputeNetwork
	Err     error
}

func (h Hnsv2wrapperwithtimeout) CreateEndpoint(endpoint *hcn.HostComputeEndpoint) (*hcn.HostComputeEndpoint, error) {
	r := make(chan CreateEndpointFuncResult)
	ctx, cancel := context.WithTimeout(context.TODO(), h.HnsCallTimeout)
	defer cancel()

	go func() {
		endpoint, err := h.Hnsv2.CreateEndpoint(endpoint)

		r <- CreateEndpointFuncResult{
			endpoint: endpoint,
			Err:      err,
		}
	}()

	// Listen on our channel AND a timeout channel - which ever happens first.
	select {
	case res := <-r:
		return res.endpoint, res.Err
	case <-ctx.Done():
		return nil, errors.Wrapf(ErrHNSCallTimeout, "CreateEndpoint timeout value is %v ", h.HnsCallTimeout.String())
	}
}

func (h Hnsv2wrapperwithtimeout) DeleteEndpoint(endpoint *hcn.HostComputeEndpoint) error {
	r := make(chan error)
	ctx, cancel := context.WithTimeout(context.TODO(), h.HnsCallTimeout)
	defer cancel()

	go func() {
		r <- h.Hnsv2.DeleteEndpoint(endpoint)
	}()

	// Listen on our channel AND a timeout channel - which ever happens first.
	select {
	case res := <-r:
		return res
	case <-ctx.Done():
		return errors.Wrapf(ErrHNSCallTimeout, "DeleteEndpoint timeout value is %v ", h.HnsCallTimeout.String())
	}
}

func (h Hnsv2wrapperwithtimeout) CreateNetwork(network *hcn.HostComputeNetwork) (*hcn.HostComputeNetwork, error) {
	r := make(chan CreateNetworkFuncResult)
	ctx, cancel := context.WithTimeout(context.TODO(), h.HnsCallTimeout)
	defer cancel()

	go func() {
		network, err := h.Hnsv2.CreateNetwork(network)

		r <- CreateNetworkFuncResult{
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

func (h Hnsv2wrapperwithtimeout) DeleteNetwork(network *hcn.HostComputeNetwork) error {
	r := make(chan error)
	ctx, cancel := context.WithTimeout(context.TODO(), h.HnsCallTimeout)
	defer cancel()

	go func() {
		r <- h.Hnsv2.DeleteNetwork(network)
	}()

	// Listen on our channel AND a timeout channel - which ever happens first.
	select {
	case res := <-r:
		return res
	case <-ctx.Done():
		return errors.Wrapf(ErrHNSCallTimeout, "DeleteNetwork timeout value is %v ", h.HnsCallTimeout.String())
	}
}

func (h Hnsv2wrapperwithtimeout) ModifyNetworkSettings(network *hcn.HostComputeNetwork, request *hcn.ModifyNetworkSettingRequest) error {
	r := make(chan error)
	ctx, cancel := context.WithTimeout(context.TODO(), h.HnsCallTimeout)
	defer cancel()

	go func() {
		r <- h.Hnsv2.ModifyNetworkSettings(network, request)
	}()

	// Listen on our channel AND a timeout channel - which ever happens first.
	select {
	case res := <-r:
		return res
	case <-ctx.Done():
		return errors.Wrapf(ErrHNSCallTimeout, "ModifyNetworkSettings timeout value is %v ", h.HnsCallTimeout.String())
	}
}

func (h Hnsv2wrapperwithtimeout) AddNetworkPolicy(network *hcn.HostComputeNetwork, networkPolicy hcn.PolicyNetworkRequest) error {
	r := make(chan error)
	ctx, cancel := context.WithTimeout(context.TODO(), h.HnsCallTimeout)
	defer cancel()

	go func() {
		r <- h.Hnsv2.AddNetworkPolicy(network, networkPolicy)
	}()

	// Listen on our channel AND a timeout channel - which ever happens first.
	select {
	case res := <-r:
		return res
	case <-ctx.Done():
		return errors.Wrapf(ErrHNSCallTimeout, "AddNetworkPolicy timeout value is %v ", h.HnsCallTimeout.String())
	}
}

func (h Hnsv2wrapperwithtimeout) RemoveNetworkPolicy(network *hcn.HostComputeNetwork, networkPolicy hcn.PolicyNetworkRequest) error {
	r := make(chan error)
	ctx, cancel := context.WithTimeout(context.TODO(), h.HnsCallTimeout)
	defer cancel()

	go func() {
		r <- h.Hnsv2.RemoveNetworkPolicy(network, networkPolicy)
	}()

	// Listen on our channel AND a timeout channel - which ever happens first.
	select {
	case res := <-r:
		return res
	case <-ctx.Done():
		return errors.Wrapf(ErrHNSCallTimeout, "RemoveNetworkPolicy timeout value is %v ", h.HnsCallTimeout.String())
	}
}

func (h Hnsv2wrapperwithtimeout) GetNamespaceByID(netNamespacePath string) (*hcn.HostComputeNamespace, error) {
	r := make(chan GetNamespaceByIDFuncResult)
	ctx, cancel := context.WithTimeout(context.TODO(), h.HnsCallTimeout)
	defer cancel()

	go func() {
		namespace, err := h.Hnsv2.GetNamespaceByID(netNamespacePath)

		r <- GetNamespaceByIDFuncResult{
			namespace: namespace,
			Err:       err,
		}
	}()

	// Listen on our channel AND a timeout channel - which ever happens first.
	select {
	case res := <-r:
		return res.namespace, res.Err
	case <-ctx.Done():
		return nil, errors.Wrapf(ErrHNSCallTimeout, "GetNamespaceByID timeout value is %v ", h.HnsCallTimeout.String())
	}
}

func (h Hnsv2wrapperwithtimeout) AddNamespaceEndpoint(namespaceId string, endpointId string) error {
	r := make(chan error)
	ctx, cancel := context.WithTimeout(context.TODO(), h.HnsCallTimeout)
	defer cancel()

	go func() {
		r <- h.Hnsv2.AddNamespaceEndpoint(namespaceId, endpointId)
	}()

	// Listen on our channel AND a timeout channel - which ever happens first.
	select {
	case res := <-r:
		return res
	case <-ctx.Done():
		return fmt.Errorf("AddNamespaceEndpoint %w , timeout value is %s seconds", ErrHNSCallTimeout, h.HnsCallTimeout.String())
	}
}

func (h Hnsv2wrapperwithtimeout) RemoveNamespaceEndpoint(namespaceId string, endpointId string) error {
	r := make(chan error)
	ctx, cancel := context.WithTimeout(context.TODO(), h.HnsCallTimeout)
	defer cancel()

	go func() {
		r <- h.Hnsv2.RemoveNamespaceEndpoint(namespaceId, endpointId)
	}()

	// Listen on our channel AND a timeout channel - which ever happens first.
	select {
	case res := <-r:
		return res
	case <-ctx.Done():
		return errors.Wrapf(ErrHNSCallTimeout, "RemoveNamespaceEndpoint %w , timeout value is %s seconds", h.HnsCallTimeout.String())
	}
}

func (h Hnsv2wrapperwithtimeout) GetNetworkByName(networkName string) (*hcn.HostComputeNetwork, error) {
	r := make(chan GetNetworkByNameFuncResult)
	ctx, cancel := context.WithTimeout(context.TODO(), h.HnsCallTimeout)
	defer cancel()

	go func() {
		network, err := h.Hnsv2.GetNetworkByName(networkName)

		r <- GetNetworkByNameFuncResult{
			network: network,
			Err:     err,
		}
	}()

	// Listen on our channel AND a timeout channel - which ever happens first.
	select {
	case res := <-r:
		return res.network, res.Err
	case <-ctx.Done():
		return nil, errors.Wrapf(ErrHNSCallTimeout, "GetNetworkByName %w , timeout value is %s seconds", h.HnsCallTimeout.String())
	}
}

func (h Hnsv2wrapperwithtimeout) GetNetworkByID(networkId string) (*hcn.HostComputeNetwork, error) {
	r := make(chan GetNetworkByIDFuncResult)
	ctx, cancel := context.WithTimeout(context.TODO(), h.HnsCallTimeout)
	defer cancel()

	go func() {
		network, err := h.Hnsv2.GetNetworkByID(networkId)

		r <- GetNetworkByIDFuncResult{
			network: network,
			Err:     err,
		}
	}()

	// Listen on our channel AND a timeout channel - which ever happens first.
	select {
	case res := <-r:
		return res.network, res.Err
	case <-ctx.Done():
		return nil, errors.Wrapf(ErrHNSCallTimeout, "GetNetworkByID %w , timeout value is %s seconds", h.HnsCallTimeout.String())
	}
}

func (h Hnsv2wrapperwithtimeout) GetEndpointByID(endpointId string) (*hcn.HostComputeEndpoint, error) {
	r := make(chan GetEndpointByIDFuncResult)
	ctx, cancel := context.WithTimeout(context.TODO(), h.HnsCallTimeout)
	defer cancel()

	go func() {
		endpoint, err := h.Hnsv2.GetEndpointByID(endpointId)

		r <- GetEndpointByIDFuncResult{
			endpoint: endpoint,
			Err:      err,
		}
	}()

	// Listen on our channel AND a timeout channel - which ever happens first.
	select {
	case res := <-r:
		return res.endpoint, res.Err
	case <-ctx.Done():
		return nil, errors.Wrapf(ErrHNSCallTimeout, "GetEndpointByID %w , timeout value is %s seconds", h.HnsCallTimeout.String())
	}
}

func (h Hnsv2wrapperwithtimeout) ListEndpointsOfNetwork(networkId string) ([]hcn.HostComputeEndpoint, error) {
	r := make(chan ListEndpointsFuncResult)
	ctx, cancel := context.WithTimeout(context.TODO(), h.HnsCallTimeout)
	defer cancel()
	go func() {
		endpoints, err := h.Hnsv2.ListEndpointsOfNetwork(networkId)

		r <- ListEndpointsFuncResult{
			endpoints: endpoints,
			Err:       err,
		}
	}()

	// Listen on our channel AND a timeout channel - which ever happens first.
	select {
	case res := <-r:
		return res.endpoints, res.Err
	case <-ctx.Done():
		return nil, errors.Wrapf(ErrHNSCallTimeout, "ListEndpointsOfNetwork %w , timeout value is %s seconds", h.HnsCallTimeout.String())
	}
}

// NOTE see assumptions of hnsv2wrapperfake.ListEndpointsQuery
func (h Hnsv2wrapperwithtimeout) ListEndpointsQuery(query hcn.HostComputeQuery) ([]hcn.HostComputeEndpoint, error) {
	// must be a buffered chan of size 1, otherwise it leaks goroutines when trying to send to the channel after the timeout has fired
	r := make(chan ListEndpointsFuncResult, 1)
	ctx, cancel := context.WithTimeout(context.TODO(), h.HnsCallTimeout)
	defer cancel()
	go func() {
		endpoints, err := h.Hnsv2.ListEndpointsQuery(query)

		r <- ListEndpointsFuncResult{
			endpoints: endpoints,
			Err:       err,
		}
	}()

	// Listen on our channel AND a timeout channel - which ever happens first.
	select {
	case res := <-r:
		return res.endpoints, res.Err
	case <-ctx.Done():
		return nil, errors.Wrapf(ErrHNSCallTimeout, "ListEndpointsOfNetwork, timeout value is %s seconds", h.HnsCallTimeout.String())
	}
}

func (h Hnsv2wrapperwithtimeout) ApplyEndpointPolicy(endpoint *hcn.HostComputeEndpoint, requestType hcn.RequestType, endpointPolicy hcn.PolicyEndpointRequest) error {
	r := make(chan error)
	ctx, cancel := context.WithTimeout(context.TODO(), h.HnsCallTimeout)
	defer cancel()
	go func() {
		r <- h.Hnsv2.ApplyEndpointPolicy(endpoint, requestType, endpointPolicy)
	}()

	// Listen on our channel AND a timeout channel - which ever happens first.
	select {
	case res := <-r:
		return res
	case <-ctx.Done():
		return errors.Wrapf(ErrHNSCallTimeout, "ApplyEndpointPolicy %w , timeout value is %s seconds", h.HnsCallTimeout.String())
	}
}

func (h Hnsv2wrapperwithtimeout) GetEndpointByName(endpointName string) (*hcn.HostComputeEndpoint, error) {
	r := make(chan GetEndpointByIDFuncResult)
	ctx, cancel := context.WithTimeout(context.TODO(), h.HnsCallTimeout)
	defer cancel()
	go func() {
		endpoint, err := h.Hnsv2.GetEndpointByName(endpointName)

		r <- GetEndpointByIDFuncResult{
			endpoint: endpoint,
			Err:      err,
		}
	}()

	// Listen on our channel AND a timeout channel - which ever happens first.
	select {
	case res := <-r:
		return res.endpoint, res.Err
	case <-ctx.Done():
		return nil, errors.Wrapf(ErrHNSCallTimeout, "GetEndpointByName %w , timeout value is %s seconds", h.HnsCallTimeout.String())
	}
}
