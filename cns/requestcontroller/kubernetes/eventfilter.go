package kubernetes

import (
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

type NodeNetworkConfigFilter struct {
	predicate.Funcs
	hostname string
}

// If the generations are the same, it means it's status change, and we should return true, so that the
// reconcile loop is triggered by it.
// If they're different, it means a spec change, and we should ignore, by returning false, to avoid redundant calls to cns when the
// status hasn't changed
// See https://kubernetes.io/docs/tasks/extend-kubernetes/custom-resources/custom-resource-definitions/#status-subresource
// for more details
// Generation will not change on status change
func (n NodeNetworkConfigFilter) Update(e event.UpdateEvent) bool {
	isHostName := n.isHostName(e.MetaOld.GetName())
	oldGeneration := e.MetaOld.GetGeneration()
	newGeneration := e.MetaNew.GetGeneration()
	return (oldGeneration == newGeneration) && isHostName
}

// Only process create events if CRD name equals this host's name
func (n NodeNetworkConfigFilter) Create(e event.CreateEvent) bool {
	return n.isHostName(e.Meta.GetName())
}

// Only process delete events if CRD name equals this host's name
func (n NodeNetworkConfigFilter) Delete(e event.DeleteEvent) bool {
	return n.isHostName(e.Meta.GetName())
}

// Given a string, returns if that string equals the hostname running this program
func (n NodeNetworkConfigFilter) isHostName(metaName string) bool {
	return metaName == n.hostname
}
