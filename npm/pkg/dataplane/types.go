package dataplane

import (
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/ipsets"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/policies"
)

type GenericDataplane interface {
	InitializeDataPlane() error
	ResetDataPlane() error
	CreateIPSet(setNames []*ipsets.IPSetMetadata)
	DeleteIPSet(setMetadata *ipsets.IPSetMetadata)
	AddToSet(setNames []*ipsets.IPSetMetadata, podMetadata *PodMetadata) error
	RemoveFromSet(setNames []*ipsets.IPSetMetadata, podMetadata *PodMetadata) error
	AddToLists(listName []*ipsets.IPSetMetadata, setNames []*ipsets.IPSetMetadata, podMetadata *PodMetadata) error
	RemoveFromList(listName *ipsets.IPSetMetadata, setNames []*ipsets.IPSetMetadata, podMetadata *PodMetadata) error
	ApplyDataPlane() error
	AddPolicy(policies *policies.NPMNetworkPolicy) error
	RemovePolicy(policyName string) error
	UpdatePolicy(policies *policies.NPMNetworkPolicy) error
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
