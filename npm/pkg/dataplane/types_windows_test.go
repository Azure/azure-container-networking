package dataplane

import (
	"fmt"

	"github.com/Azure/azure-container-networking/network/hnswrapper"
	"github.com/Azure/azure-container-networking/npm/pkg/controlplane/translation"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/ipsets"
	dptestutils "github.com/Azure/azure-container-networking/npm/pkg/dataplane/testutils"
	"github.com/Microsoft/hcsshim/hcn"
	"github.com/pkg/errors"
	networkingv1 "k8s.io/api/networking/v1"
)

type Tag string

type SerialTestCase struct {
	Description string
	Actions     []*Action
	*TestCaseMetadata
}

type MultiRoutineTestCase struct {
	Description string
	Routines    map[string][]*Action
	*TestCaseMetadata
}

type TestCaseMetadata struct {
	Tags                 []Tag
	InitialEndpoints     []*hcn.HostComputeEndpoint
	DpCfg                *Config
	ExpectedSetPolicies  []*hcn.SetPolicySetting
	ExpectedEnpdointACLs map[string][]*hnswrapper.FakeEndpointPolicy
}

// Action represents a single action (either an HNSAction or a DPAction).
// Exactly one of HNSAction or DPAction should be non-nil.
type Action struct {
	HNSAction
	DPAction
}

type HNSAction interface {
	Do(hns *hnswrapper.Hnsv2wrapperFake) error
}

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

type DPAction interface {
	Do(dp *DataPlane) error
}

type PodCreateAction struct {
	Pod    *PodMetadata
	Labels map[string]string
}

func CreatePod(namespace, name, ip, node string, labels map[string]string) *Action {
	return &Action{
		DPAction: &PodCreateAction{
			Pod:    NewPodMetadata(fmt.Sprintf("%s/%s", namespace, name), ip, node),
			Labels: labels,
		},
	}
}

func (p *PodCreateAction) Do(dp *DataPlane) error {
	context := fmt.Sprintf("create context: [pod: %+v. labels: %+v]", p.Pod, p.Labels)

	nsIPSet := []*ipsets.IPSetMetadata{ipsets.NewIPSetMetadata(p.Pod.Namespace(), ipsets.Namespace)}
	// PodController technically wouldn't call this if the namespace already existed
	if err := dp.AddToLists([]*ipsets.IPSetMetadata{allNamespaces}, nsIPSet); err != nil {
		return errors.Wrapf(err, "[PodCreateAction] failed to add ns set to all namespaces list. %s", context)
	}

	if err := dp.AddToSets(nsIPSet, p.Pod); err != nil {
		return errors.Wrapf(err, "[PodCreateAction] failed to add pod ip to ns set. %s", context)
	}

	for key, val := range p.Labels {
		keyVal := fmt.Sprintf("%s:%s", key, val)
		labelIPSets := []*ipsets.IPSetMetadata{
			ipsets.NewIPSetMetadata(key, ipsets.KeyLabelOfPod),
			ipsets.NewIPSetMetadata(keyVal, ipsets.KeyValueLabelOfNamespace),
		}

		if err := dp.AddToSets(labelIPSets, p.Pod); err != nil {
			return errors.Wrapf(err, "[PodCreateAction] failed to add pod ip to label sets. %s", context)
		}

	}

	return nil
}

type ApplyDPAction struct{}

func ApplyDP() *Action {
	return &Action{
		DPAction: &ApplyDPAction{},
	}
}

func (*ApplyDPAction) Do(dp *DataPlane) error {
	if err := dp.ApplyDataPlane(); err != nil {
		return errors.Wrapf(err, "[ApplyDPAction] failed to apply")
	}
	return nil
}

// TODO PodUpdateAction and PodDeleteAction

// TODO namespace actions

type PolicyUpdateAction struct {
	Policy *networkingv1.NetworkPolicy
}

func UpdatePolicy(policy *networkingv1.NetworkPolicy) *Action {
	return &Action{
		DPAction: &PolicyUpdateAction{
			Policy: policy,
		},
	}
}

func (p *PolicyUpdateAction) Do(dp *DataPlane) error {
	npmNetPol, err := translation.TranslatePolicy(p.Policy)
	if err != nil {
		return errors.Wrapf(err, "[PolicyUpdateAction] failed to translate policy with key %s/%s", p.Policy.Namespace, p.Policy.Name)
	}

	if err := dp.UpdatePolicy(npmNetPol); err != nil {
		return errors.Wrapf(err, "[PolicyUpdateAction] failed to update policy with key %s/%s", p.Policy.Namespace, p.Policy.Name)
	}
	return nil
}

type PolicyDeleteAction struct {
	Namespace string
	Name      string
}

func DeletePolicy(namespace, name string) *Action {
	return &Action{
		DPAction: &PolicyDeleteAction{
			Namespace: namespace,
			Name:      name,
		},
	}
}

func DeletePolicyByObject(policy *networkingv1.NetworkPolicy) *Action {
	return DeletePolicy(policy.Namespace, policy.Name)
}

func (p *PolicyDeleteAction) Do(dp *DataPlane) error {
	policyKey := fmt.Sprintf("%s/%s", p.Namespace, p.Name)
	if err := dp.RemovePolicy(policyKey); err != nil {
		return errors.Wrapf(err, "[PolicyDeleteAction] failed to update policy with key %s", policyKey)
	}
	return nil
}
