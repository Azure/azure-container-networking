package kubecontroller

import (
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

type NodeNetworkConfigFilter struct {
	predicate.Funcs
	nodeName  string
	namespace string
}

// Returns true if request is to be processed by Reconciler
// Checks that old generation equals new generation because status changes don't change generation number
func (n NodeNetworkConfigFilter) Update(e event.UpdateEvent) bool {
	isNamespace := n.isNamespace(e.MetaNew.GetNamespace())
	isNodeName := n.isNodeName(e.MetaOld.GetName())
	oldGeneration := e.MetaOld.GetGeneration()
	newGeneration := e.MetaNew.GetGeneration()
	return (oldGeneration == newGeneration) && isNodeName && isNamespace
}

// Only process create events if CRD name equals this host's name
func (n NodeNetworkConfigFilter) Create(e event.CreateEvent) bool {
	return n.isNodeName(e.Meta.GetName()) && n.isNamespace(e.Meta.GetNamespace())
}

//TODO: Decide what deleteing crd means with DNC
// Ignore all for now
func (n NodeNetworkConfigFilter) Delete(e event.DeleteEvent) bool {
	return false
}

// Given a string, returns if that string equals the nodename running this program
func (n NodeNetworkConfigFilter) isNodeName(metaName string) bool {
	return metaName == n.nodeName
}

// Given a string, returns if that string equals the namespace running the nncs
func (n NodeNetworkConfigFilter) isNamespace(namespace string) bool {
	return n.namespace == namespace
}
