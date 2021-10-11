package dataplane

import (
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/ipsets"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/policies"
)

type GenericDataplane interface {
	InitializeDataPlane() error
	ResetDataPlane() error
	CreateIPSet(setMetadata *ipsets.IPSetMetadata)
	DeleteIPSet(setMetadata *ipsets.IPSetMetadata)
	AddToSet(setNames []*ipsets.IPSetMetadata, ip, podKey string) error
	RemoveFromSet(setNames []*ipsets.IPSetMetadata, ip, podKey string) error
	AddToList(listName *ipsets.IPSetMetadata, setNames []*ipsets.IPSetMetadata) error
	RemoveFromList(listName *ipsets.IPSetMetadata, setNames []*ipsets.IPSetMetadata) error
	UpdatePod(pod *UpdateNPMPod) error
	ApplyDataPlane() error
	AddPolicy(policies *policies.NPMNetworkPolicy) error
	RemovePolicy(policyName string) error
	UpdatePolicy(policies *policies.NPMNetworkPolicy) error
}
