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
	*DPAction
	// TODO Pod name and node instead of podmetadata object (infer podkey from name and namespace)
	Pod       *dataplane.PodMetadata
	Namespace string
	Labels    map[string]string
}

func (p *PodCreateAction) Do() error {
	if p.dp == nil {
		return errors.New("no DP") // FIXME make constant type for lint
	}

	context := fmt.Sprintf("create context: [pod: %+v. ns: %s. labels: %+v]", p.Pod, p.Namespace, p.Labels)

	nsIPSet := []*ipsets.IPSetMetadata{ipsets.NewIPSetMetadata(p.Namespace, ipsets.Namespace)}
	// PodController technically wouldn't call this if the namespace already existed
	if err := p.dp.AddToLists([]*ipsets.IPSetMetadata{allNamespaces}, nsIPSet); err != nil {
		return errors.Wrapf(err, "[podCreateEvent] failed to add ns set to all namespaces list. %s", context)
	}

	if err := p.dp.AddToSets(nsIPSet, p.Pod); err != nil {
		return errors.Wrapf(err, "[podCreateEvent] failed to add pod ip to ns set. %s", context)
	}

	for key, val := range p.Labels {
		keyVal := fmt.Sprintf("%s:%s", key, val)
		labelIPSets := []*ipsets.IPSetMetadata{
			ipsets.NewIPSetMetadata(key, ipsets.KeyLabelOfPod),
			ipsets.NewIPSetMetadata(keyVal, ipsets.KeyValueLabelOfNamespace),
		}

		if err := p.dp.AddToSets(labelIPSets, p.Pod); err != nil {
			return errors.Wrapf(err, "[podCreateEvent] failed to add pod ip to label sets. %s", context)
		}

	}

	return nil
}

type ApplyDPAction struct {
	*DPAction
}

func (a *ApplyDPAction) Do() error {
	if a.dp == nil {
		return errors.New("no DP") // FIXME make constant type for lint
	}

	if err := a.dp.ApplyDataPlane(); err != nil {
		return errors.Wrapf(err, "[ApplyDPAction] failed to apply")
	}
	return nil
}

type PolicyUpdateAction struct {
	*DPAction
	Policy *networkingv1.NetworkPolicy
}

func (p *PolicyUpdateAction) Do() error {
	if p.dp == nil {
		return errors.New("no DP") // FIXME make constant type for lint
	}

	npmNetPol, err := translation.TranslatePolicy(p.Policy)
	if err != nil {
		return errors.Wrapf(err, "[PolicyUpdateAction] failed to translate policy with key %s/%s", p.Policy.Namespace, p.Policy.Name)
	}

	if err := p.dp.UpdatePolicy(npmNetPol); err != nil {
		return errors.Wrapf(err, "[PolicyUpdateAction] failed to update policy with key %s/%s", p.Policy.Namespace, p.Policy.Name)
	}
	return nil
}

type PolicyDeleteAction struct {
	*DPAction
	Namespace string
	Name      string
}

func (p *PolicyDeleteAction) Do() error {
	if p.dp == nil {
		return errors.New("no DP") // FIXME make constant type for lint
	}

	policyKey := fmt.Sprintf("%s/%s", p.Namespace, p.Name)
	if err := p.dp.RemovePolicy(policyKey); err != nil {
		return errors.Wrapf(err, "[PolicyDeleteAction] failed to update policy with key %s", policyKey)
	}
	return nil
}
