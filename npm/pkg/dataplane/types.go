package dataplane

import (
	"strings"

	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/ipsets"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/policies"
	"github.com/Azure/azure-container-networking/npm/util"
)

type GenericDataplane interface {
	BootupDataplane() error
	RunPeriodicTasks()
	GetAllIPSets() map[string]string
	GetIPSet(setName string) *ipsets.IPSet
	CreateIPSets(setMetadatas []*ipsets.IPSetMetadata)
	DeleteIPSet(setMetadata *ipsets.IPSetMetadata, deleteOption util.DeleteOption)
	AddToSets(setMetadatas []*ipsets.IPSetMetadata, podMetadata *PodMetadata) error
	RemoveFromSets(setMetadatas []*ipsets.IPSetMetadata, podMetadata *PodMetadata) error
	AddToLists(listMetadatas []*ipsets.IPSetMetadata, setMetadatas []*ipsets.IPSetMetadata) error
	RemoveFromList(listMetadata *ipsets.IPSetMetadata, setMetadatas []*ipsets.IPSetMetadata) error
	ApplyDataPlane() error
	// GetAllPolicies is deprecated and only used in the goalstateprocessor, which is deprecated
	GetAllPolicies() []string
	AddPolicy(policies *policies.NPMNetworkPolicy) error
	RemovePolicy(PolicyKey string) error
	UpdatePolicy(policies *policies.NPMNetworkPolicy) error
}

// UpdateNPMPod pod controller will populate and send this datastructure to dataplane
// to update the dataplane with the latest pod information
// this helps in calculating if any update needs to have policies applied or removed
type updateNPMPod struct {
	*PodMetadata
	IPSetsToAdd    []string
	IPSetsToRemove []string
}

// PodMetadata is what is passed to dataplane to specify pod ipset
// todo definitely requires further optimization between the intersection
// of types, PodMetadata, NpmPod and corev1.pod
type PodMetadata struct {
	PodKey   string
	PodIP    string
	NodeName string
}

func NewPodMetadata(podKey, podIP, nodeName string) *PodMetadata {
	return &PodMetadata{
		PodKey:   podKey,
		PodIP:    podIP,
		NodeName: nodeName,
	}
}

func (p *PodMetadata) Namespace() string {
	return strings.Split(p.PodKey, "/")[0]
}

func newUpdateNPMPod(podMetadata *PodMetadata) *updateNPMPod {
	return &updateNPMPod{
		PodMetadata:    podMetadata,
		IPSetsToAdd:    make([]string, 0),
		IPSetsToRemove: make([]string, 0),
	}
}

func (npmPod *updateNPMPod) updateIPSetsToAdd(setNames []*ipsets.IPSetMetadata) {
	for _, set := range setNames {
		npmPod.IPSetsToAdd = append(npmPod.IPSetsToAdd, set.GetPrefixName())
	}
}

func (npmPod *updateNPMPod) updateIPSetsToRemove(setNames []*ipsets.IPSetMetadata) {
	for _, set := range setNames {
		npmPod.IPSetsToRemove = append(npmPod.IPSetsToRemove, set.GetPrefixName())
	}
}
