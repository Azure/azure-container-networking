package dataplane

import (
	"github.com/Azure/azure-container-networking/npm"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/ipsets"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/policies"
)

type GenericDataplane interface {
	InitializeDataPlane() error
	ResetDataPlane() error
	CreateIPSet(setName string, setType ipsets.SetType) error
	DeleteIPSet(name string) error
	AddToSet(setNames []string, ip, podKey string) error
	RemoveFromSet(setNames []string, ip, podKey string) error
	AddToList(listName string, setNames []string) error
	RemoveFromList(listName string, setNames []string) error
	UpdatePod(pod *npm.NpmPod) error
	ApplyDataPlane() error
	AddPolicy(policies *policies.NPMNetworkPolicy) error
	RemovePolicy(policyName string) error
	UpdatePolicy(policies *policies.NPMNetworkPolicy) error
}
