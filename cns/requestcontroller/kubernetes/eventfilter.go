package kubernetes

import (
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

type NodeNetworkConfigFilter struct {
	predicate.Funcs
}

// If the generations are the same, it means it's status change, and we should return true, so that the
// reconcile loop is triggered by it.
// If they're different, it means a spec change, and we should ignore, by returning false, to avoid redundant calls to cns when the
// status hasn't changed
// See https://kubernetes.io/docs/tasks/extend-kubernetes/custom-resources/custom-resource-definitions/#status-subresource
// for more details
func (NodeNetworkConfigFilter) Update(e event.UpdateEvent) bool {
	oldGeneration := e.MetaOld.GetGeneration()
	newGeneration := e.MetaNew.GetGeneration()
	return oldGeneration == newGeneration
}
