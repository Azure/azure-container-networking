package ciliumpool

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const (
	stateAnnotation = "azure-container-networking.io/ipam-state"
	defaultPool     = "default"
	jsonPatchAdd    = "add"
	ipamField       = "ipam"
	statusField     = "status"
	macField        = "mac"
	cidrField       = "cidr"
	gatewayField    = "gateway"
	stateField      = "state"
	poolField       = "pool"
	specField       = "spec"
	poolsField      = "pools"
	subnetField     = "subnet"
	stateSucceeded  = "succeeded"
)

type patchOperation struct {
	Op    string `json:"op"`
	Path  string `json:"path"`
	Value any    `json:"value"`
}

func consumerPatch(node *unstructured.Unstructured, state State, retained []string, spare int) ([]byte, error) {
	ledger, err := json.Marshal(state)
	if err != nil {
		return nil, fmt.Errorf("encoding reservation ledger: %w", err)
	}
	size := len(stateAnnotation) + len(ledger)
	for key, value := range node.GetAnnotations() {
		if key != stateAnnotation {
			size += len(key) + len(value)
		}
	}
	if size > 256*1024 {
		return nil, fmt.Errorf("%w: annotations exceed Kubernetes size limit", ErrOwnership)
	}
	cidrs := make([]any, len(retained))
	for i, cidr := range retained {
		cidrs[i] = cidr
	}
	requested := []any{map[string]any{poolField: defaultPool, "needed": map[string]any{"ipv4-addrs": int64(len(state.Reservations) + len(state.Pending) + spare)}}}
	allocated := []any{map[string]any{poolField: defaultPool, "cidrs": cidrs}}
	oldRequested, _, err := unstructured.NestedSlice(node.Object, specField, ipamField, poolsField, "requested")
	if err != nil {
		return nil, fmt.Errorf("reading existing demand: %w", err)
	}
	oldAllocated, _, err := unstructured.NestedSlice(node.Object, specField, ipamField, poolsField, "allocated")
	if err != nil {
		return nil, fmt.Errorf("reading existing retention: %w", err)
	}
	if node.GetAnnotations()[stateAnnotation] == string(ledger) &&
		reflect.DeepEqual(oldRequested, requested) && reflect.DeepEqual(oldAllocated, allocated) {
		return nil, nil
	}
	ops := []patchOperation{
		{"test", "/metadata/uid", string(node.GetUID())},
		{"test", "/metadata/resourceVersion", node.GetResourceVersion()},
		{jsonPatchAdd, "/metadata/annotations/" + strings.ReplaceAll(stateAnnotation, "/", "~1"), string(ledger)},
		{jsonPatchAdd, "/spec/ipam/pools/requested", requested},
		{jsonPatchAdd, "/spec/ipam/pools/allocated", allocated},
	}
	result, err := json.Marshal(ops)
	if err != nil {
		return nil, fmt.Errorf("encoding consumer update: %w", err)
	}
	return result, nil
}
