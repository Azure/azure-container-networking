package e2e

import (
	"github.com/Azure/azure-container-networking/network/hnswrapper"
	dptestutils "github.com/Azure/azure-container-networking/npm/pkg/dataplane/testutils"
	"github.com/Microsoft/hcsshim/hcn"
	"github.com/pkg/errors"
)

type EndpointCreateAction struct {
	ID string
	IP string
}

func CreateEndpoint(id, ip string) *Action {
	return &Action{
		HNSAction: &EndpointCreateAction{
			ID: id,
			IP: ip,
		},
	}
}

func (e *EndpointCreateAction) Do(hns *hnswrapper.Hnsv2wrapperFake) error {
	ep := dptestutils.Endpoint(e.ID, e.IP)
	_, err := hns.CreateEndpoint(ep)
	if err != nil {
		return errors.Wrapf(err, "[EndpointCreateAction] failed to create endpoint. ep: [%+v]", ep)
	}
	return nil
}

type EndpointDeleteAction struct {
	ID string
}

func DeleteEndpoint(id string) *Action {
	return &Action{
		HNSAction: &EndpointDeleteAction{
			ID: id,
		},
	}
}

func (e *EndpointDeleteAction) Do(hns *hnswrapper.Hnsv2wrapperFake) error {
	ep := &hcn.HostComputeEndpoint{
		Id: e.ID,
	}
	if err := hns.DeleteEndpoint(ep); err != nil {
		return errors.Wrapf(err, "[EndpointDeleteAction] failed to delete endpoint. ep: [%+v]", ep)
	}
	return nil
}
