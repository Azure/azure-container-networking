package e2e

import (
	"fmt"

	"github.com/Azure/azure-container-networking/npm/pkg/controlplane/translation"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/ipsets"
	"github.com/pkg/errors"

	networkingv1 "k8s.io/api/networking/v1"
)

type PodCreateAction struct {
	Pod    *dataplane.PodMetadata
	Labels map[string]string
}

func CreatePod(namespace, name, ip, node string, labels map[string]string) *Action {
	return &Action{
		DPAction: &PodCreateAction{
			Pod:    dataplane.NewPodMetadata(fmt.Sprintf("%s/%s", namespace, name), ip, node),
			Labels: labels,
		},
	}
}

func (p *PodCreateAction) Do(dp *dataplane.DataPlane) error {
	context := fmt.Sprintf("create context: [pod: %+v. labels: %+v]", p.Pod, p.Labels)

	nsIPSet := []*ipsets.IPSetMetadata{ipsets.NewIPSetMetadata(p.Pod.Namespace(), ipsets.Namespace)}
	// PodController technically wouldn't call this if the namespace already existed
	if err := dp.AddToLists([]*ipsets.IPSetMetadata{allNamespaces}, nsIPSet); err != nil {
		return errors.Wrapf(err, "[podCreateEvent] failed to add ns set to all namespaces list. %s", context)
	}

	if err := dp.AddToSets(nsIPSet, p.Pod); err != nil {
		return errors.Wrapf(err, "[podCreateEvent] failed to add pod ip to ns set. %s", context)
	}

	for key, val := range p.Labels {
		keyVal := fmt.Sprintf("%s:%s", key, val)
		labelIPSets := []*ipsets.IPSetMetadata{
			ipsets.NewIPSetMetadata(key, ipsets.KeyLabelOfPod),
			ipsets.NewIPSetMetadata(keyVal, ipsets.KeyValueLabelOfNamespace),
		}

		if err := dp.AddToSets(labelIPSets, p.Pod); err != nil {
			return errors.Wrapf(err, "[podCreateEvent] failed to add pod ip to label sets. %s", context)
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

func (*ApplyDPAction) Do(dp *dataplane.DataPlane) error {
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

func (p *PolicyUpdateAction) Do(dp *dataplane.DataPlane) error {
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

func (p *PolicyDeleteAction) Do(dp *dataplane.DataPlane) error {
	policyKey := fmt.Sprintf("%s/%s", p.Namespace, p.Name)
	if err := dp.RemovePolicy(policyKey); err != nil {
		return errors.Wrapf(err, "[PolicyDeleteAction] failed to update policy with key %s", policyKey)
	}
	return nil
}
