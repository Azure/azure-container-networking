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

type MultiJobTestCase struct {
	Description string
	Jobs        map[string][]*Action
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
	// Do models events in HNS
	Do(hns *hnswrapper.Hnsv2wrapperFake) error
}

type EndpointCreateAction struct {
	ID       string
	IP       string
	IsRemote bool
}

func CreateEndpoint(id, ip string) *Action {
	return &Action{
		HNSAction: &EndpointCreateAction{
			ID:       id,
			IP:       ip,
			IsRemote: false,
		},
	}
}

func CreateRemoteEndpoint(id, ip string) *Action {
	return &Action{
		HNSAction: &EndpointCreateAction{
			ID:       id,
			IP:       ip,
			IsRemote: true,
		},
	}
}

// Do models endpoint creation in HNS
func (e *EndpointCreateAction) Do(hns *hnswrapper.Hnsv2wrapperFake) error {
	var ep *hcn.HostComputeEndpoint
	if e.IsRemote {
		ep = dptestutils.RemoteEndpoint(e.ID, e.IP)
	} else {
		ep = dptestutils.Endpoint(e.ID, e.IP)
	}

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

// Do models endpoint deletion in HNS
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
	// Do models interactions with the DataPlane
	Do(dp *DataPlane) error
}

type ApplyDPAction struct{}

func ApplyDP() *Action {
	return &Action{
		DPAction: &ApplyDPAction{},
	}
}

// Do applies the dataplane
func (*ApplyDPAction) Do(dp *DataPlane) error {
	if err := dp.ApplyDataPlane(); err != nil {
		return errors.Wrapf(err, "[ApplyDPAction] failed to apply")
	}
	return nil
}

type ReconcileDPAction struct{}

func ReconcileDP() *Action {
	return &Action{
		DPAction: &ReconcileDPAction{},
	}
}

// Do reconciles the IPSetManager and PolicyManager
func (*ReconcileDPAction) Do(dp *DataPlane) error {
	dp.ipsetMgr.Reconcile()
	// currently does nothing in windows
	dp.policyMgr.Reconcile()
	return nil
}

type PodCreateAction struct {
	Pod    *PodMetadata
	Labels map[string]string
}

func CreatePod(namespace, name, ip, node string, labels map[string]string) *Action {
	podKey := fmt.Sprintf("%s/%s", namespace, name)
	return &Action{
		DPAction: &PodCreateAction{
			Pod:    NewPodMetadata(podKey, ip, node),
			Labels: labels,
		},
	}
}

// Do models pod creation in the PodController
func (p *PodCreateAction) Do(dp *DataPlane) error {
	context := fmt.Sprintf("create context: [pod: %+v. labels: %+v]", p.Pod, p.Labels)

	nsIPSet := []*ipsets.IPSetMetadata{ipsets.NewIPSetMetadata(p.Pod.Namespace(), ipsets.Namespace)}
	// PodController technically wouldn't call this if the namespace already existed
	if err := dp.AddToLists([]*ipsets.IPSetMetadata{allNamespaces}, nsIPSet); err != nil {
		return errors.Wrapf(err, "[PodCreateAction] failed to add ns set to all-namespaces list. %s", context)
	}

	if err := dp.AddToSets(nsIPSet, p.Pod); err != nil {
		return errors.Wrapf(err, "[PodCreateAction] failed to add pod ip to ns set. %s", context)
	}

	for key, val := range p.Labels {
		keyVal := fmt.Sprintf("%s:%s", key, val)
		labelIPSets := []*ipsets.IPSetMetadata{
			ipsets.NewIPSetMetadata(key, ipsets.KeyLabelOfPod),
			ipsets.NewIPSetMetadata(keyVal, ipsets.KeyValueLabelOfPod),
		}

		if err := dp.AddToSets(labelIPSets, p.Pod); err != nil {
			return errors.Wrapf(err, "[PodCreateAction] failed to add pod ip to label sets %+v. %s", labelIPSets, context)
		}
	}

	return nil
}

type PodUpdateAction struct {
	OldPod         *PodMetadata
	NewPod         *PodMetadata
	LabelsToRemove map[string]string
	LabelsToAdd    map[string]string
}

func UpdatePod(namespace, name, oldIP, oldNode, newIP, newNode string, labelsToRemove, labelsToAdd map[string]string) *Action {
	podKey := fmt.Sprintf("%s/%s", namespace, name)
	return &Action{
		DPAction: &PodUpdateAction{
			OldPod:         NewPodMetadata(podKey, oldIP, oldNode),
			NewPod:         NewPodMetadata(podKey, newIP, newNode),
			LabelsToRemove: labelsToRemove,
			LabelsToAdd:    labelsToAdd,
		},
	}
}

func UpdatePodLabels(namespace, name, ip, node string, labelsToRemove, labelsToAdd map[string]string) *Action {
	return UpdatePod(namespace, name, ip, node, ip, node, labelsToRemove, labelsToAdd)
}

// Do models pod updates in the PodController
func (p *PodUpdateAction) Do(dp *DataPlane) error {
	context := fmt.Sprintf("update context: [old pod: %+v. current IP: %+v. old labels: %+v. new labels: %+v]", p.OldPod, p.NewPod.PodIP, p.LabelsToRemove, p.LabelsToAdd)

	// think it's impossible for this to be called on an UPDATE
	// dp.AddToLists([]*ipsets.IPSetMetadata{allNamespaces}, []*ipsets.IPSetMetadata{nsIPSet})

	for k, v := range p.LabelsToRemove {
		keyVal := fmt.Sprintf("%s:%s", k, v)
		sets := []*ipsets.IPSetMetadata{
			ipsets.NewIPSetMetadata(k, ipsets.KeyLabelOfPod),
			ipsets.NewIPSetMetadata(keyVal, ipsets.KeyValueLabelOfPod),
		}
		for _, toRemoveSet := range sets {
			if err := dp.RemoveFromSets([]*ipsets.IPSetMetadata{toRemoveSet}, p.OldPod); err != nil {
				return errors.Wrapf(err, "[PodUpdateAction] failed to remove old pod ip from set %s. %s", toRemoveSet.GetPrefixName(), context)
			}
		}
	}

	for k, v := range p.LabelsToAdd {
		keyVal := fmt.Sprintf("%s:%s", k, v)
		sets := []*ipsets.IPSetMetadata{
			ipsets.NewIPSetMetadata(k, ipsets.KeyLabelOfPod),
			ipsets.NewIPSetMetadata(keyVal, ipsets.KeyValueLabelOfPod),
		}
		for _, toAddSet := range sets {
			if err := dp.AddToSets([]*ipsets.IPSetMetadata{toAddSet}, p.NewPod); err != nil {
				return errors.Wrapf(err, "[PodUpdateAction] failed to add new pod ip to set %s. %s", toAddSet.GetPrefixName(), context)
			}
		}
	}

	return nil
}

type PodDeleteAction struct {
	Pod    *PodMetadata
	Labels map[string]string
}

func DeletePod(namespace, name, ip string, labels map[string]string) *Action {
	podKey := fmt.Sprintf("%s/%s", namespace, name)
	return &Action{
		DPAction: &PodDeleteAction{
			// currently, the PodController doesn't share the node name
			Pod:    NewPodMetadata(podKey, ip, ""),
			Labels: labels,
		},
	}
}

// Do models pod deletion in the PodController
func (p *PodDeleteAction) Do(dp *DataPlane) error {
	context := fmt.Sprintf("delete context: [pod: %+v. labels: %+v]", p.Pod, p.Labels)

	nsIPSet := []*ipsets.IPSetMetadata{ipsets.NewIPSetMetadata(p.Pod.Namespace(), ipsets.Namespace)}
	if err := dp.RemoveFromSets(nsIPSet, p.Pod); err != nil {
		return errors.Wrapf(err, "[PodDeleteAction] failed to remove pod ip from ns set. %s", context)
	}

	for key, val := range p.Labels {
		keyVal := fmt.Sprintf("%s:%s", key, val)
		labelIPSets := []*ipsets.IPSetMetadata{
			ipsets.NewIPSetMetadata(key, ipsets.KeyLabelOfPod),
			ipsets.NewIPSetMetadata(keyVal, ipsets.KeyValueLabelOfPod),
		}

		if err := dp.RemoveFromSets(labelIPSets, p.Pod); err != nil {
			return errors.Wrapf(err, "[PodDeleteAction] failed to remove pod ip from label set %+v. %s", labelIPSets, context)
		}
	}

	return nil
}

type NamespaceCreateAction struct {
	NS     string
	Labels map[string]string
}

func CreateNamespace(ns string, labels map[string]string) *Action {
	return &Action{
		DPAction: &NamespaceCreateAction{
			NS:     ns,
			Labels: labels,
		},
	}
}

// Do models namespace creation in the NamespaceController
func (n *NamespaceCreateAction) Do(dp *DataPlane) error {
	nsIPSet := []*ipsets.IPSetMetadata{ipsets.NewIPSetMetadata(n.NS, ipsets.Namespace)}

	listsToAddTo := []*ipsets.IPSetMetadata{allNamespaces}
	for k, v := range n.Labels {
		keyVal := fmt.Sprintf("%s:%s", k, v)
		listsToAddTo = append(listsToAddTo,
			ipsets.NewIPSetMetadata(k, ipsets.KeyLabelOfNamespace),
			ipsets.NewIPSetMetadata(keyVal, ipsets.KeyValueLabelOfNamespace))
	}

	if err := dp.AddToLists(listsToAddTo, nsIPSet); err != nil {
		return errors.Wrapf(err, "[NamespaceCreateAction] failed to add ns ipset to all lists. Action: %+v", n)
	}

	return nil
}

type NamespaceUpdateAction struct {
	NS             string
	LabelsToRemove map[string]string
	LabelsToAdd    map[string]string
}

func UpdateNamespace(ns string, labelsToRemove, labelsToAdd map[string]string) *Action {
	return &Action{
		DPAction: &NamespaceUpdateAction{
			NS:             ns,
			LabelsToRemove: labelsToRemove,
			LabelsToAdd:    labelsToAdd,
		},
	}
}

// Do models namespace updates in the NamespaceController
func (n *NamespaceUpdateAction) Do(dp *DataPlane) error {
	nsIPSet := []*ipsets.IPSetMetadata{ipsets.NewIPSetMetadata(n.NS, ipsets.Namespace)}

	for k, v := range n.LabelsToRemove {
		keyVal := fmt.Sprintf("%s:%s", k, v)
		lists := []*ipsets.IPSetMetadata{
			ipsets.NewIPSetMetadata(k, ipsets.KeyLabelOfNamespace),
			ipsets.NewIPSetMetadata(keyVal, ipsets.KeyValueLabelOfNamespace),
		}
		for _, listToRemoveFrom := range lists {
			if err := dp.RemoveFromList(listToRemoveFrom, nsIPSet); err != nil {
				return errors.Wrapf(err, "[NamespaceUpdateAction] failed to remove ns ipset from list %s. Action: %+v", listToRemoveFrom.GetPrefixName(), n)
			}
		}
	}

	for k, v := range n.LabelsToAdd {
		keyVal := fmt.Sprintf("%s:%s", k, v)
		lists := []*ipsets.IPSetMetadata{
			ipsets.NewIPSetMetadata(k, ipsets.KeyLabelOfNamespace),
			ipsets.NewIPSetMetadata(keyVal, ipsets.KeyValueLabelOfNamespace),
		}
		for _, listToAddTo := range lists {
			if err := dp.RemoveFromList(listToAddTo, nsIPSet); err != nil {
				return errors.Wrapf(err, "[NamespaceUpdateAction] failed to add ns ipset to list %s. Action: %+v", listToAddTo.GetPrefixName(), n)
			}
		}
	}

	return nil
}

type NamespaceDeleteAction struct {
	NS     string
	Labels map[string]string
}

func DeleteNamespace(ns string, labels map[string]string) *Action {
	return &Action{
		DPAction: &NamespaceDeleteAction{
			NS:     ns,
			Labels: labels,
		},
	}
}

// Do models namespace deletion in the NamespaceController
func (n *NamespaceDeleteAction) Do(dp *DataPlane) error {
	nsIPSet := []*ipsets.IPSetMetadata{ipsets.NewIPSetMetadata(n.NS, ipsets.Namespace)}

	for k, v := range n.Labels {
		keyVal := fmt.Sprintf("%s:%s", k, v)
		lists := []*ipsets.IPSetMetadata{
			ipsets.NewIPSetMetadata(k, ipsets.KeyLabelOfNamespace),
			ipsets.NewIPSetMetadata(keyVal, ipsets.KeyValueLabelOfNamespace),
		}
		for _, listToRemoveFrom := range lists {
			if err := dp.RemoveFromList(listToRemoveFrom, nsIPSet); err != nil {
				return errors.Wrapf(err, "[NamespaceDeleteAction] failed to remove ns ipset from list %s. Action: %+v", listToRemoveFrom.GetPrefixName(), n)
			}
		}
	}

	if err := dp.RemoveFromList(allNamespaces, nsIPSet); err != nil {
		return errors.Wrapf(err, "[NamespaceDeleteAction] failed to remove ns ipset from all-namespaces list. Action: %+v", n)
	}

	return nil
}

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

// Do models policy updates in the NetworkPolicyController
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

// Do models policy deletion in the NetworkPolicyController
func (p *PolicyDeleteAction) Do(dp *DataPlane) error {
	policyKey := fmt.Sprintf("%s/%s", p.Namespace, p.Name)
	if err := dp.RemovePolicy(policyKey); err != nil {
		return errors.Wrapf(err, "[PolicyDeleteAction] failed to update policy with key %s", policyKey)
	}
	return nil
}

type FinishBootupPhaseAction struct{}

func FinishBootupPhase() *Action {
	return &Action{
		DPAction: &FinishBootupPhaseAction{},
	}
}

// Do signifies npMgr starting the Pod controller
func (f *FinishBootupPhaseAction) Do(dp *DataPlane) error {
	dp.FinishBootupPhase()
	return nil
}
