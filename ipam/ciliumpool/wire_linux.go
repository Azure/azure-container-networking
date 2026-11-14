package ciliumpool

import (
	"encoding/json"
	"fmt"
	"net/url"
	"reflect"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const (
	apiVersionField      = "apiVersion"
	kindField            = "kind"
	ciliumAPIVersion     = "cilium.io/v2"
	annotationsField     = "annotations"
	uidField             = "uid"
	resourceVersionField = "resourceVersion"
	ownerField           = "owner"
	resourceField        = "resource"
	ciliumNodeKind       = "CiliumNode"
	metadataField        = "metadata"
	ownerPrefix          = "azure-cilium-ipam/v1/"
	ipamField            = "ipam"
	statusField          = "status"
	specField            = "spec"
	macField             = "mac"
	cidrField            = "cidr"
	gatewayField         = "gateway"
	stateField           = "state"
	poolField            = "pool"
	subnetField          = "subnet"
	stateSucceeded       = "succeeded"
	jsonPatchAdd         = "add"
)

type patchOperation struct {
	Op    string `json:"op"`
	Path  string `json:"path"`
	Value any    `json:"value"`
}

func encodeOwner(vmID string, r Reservation) (string, error) {
	boot, err := parseBootID([]byte(r.BootID))
	if err != nil || boot != r.BootID || !r.Owner.valid() || vmID == "" {
		return "", ErrOwnership
	}
	parts := []string{vmID, boot, r.Owner.Network, r.Owner.Sandbox, r.Owner.Interface}
	for i := range parts {
		parts[i] = url.PathEscape(parts[i])
	}
	return ownerPrefix + strings.Join(parts, "/"), nil
}

func decodeOwner(raw, vmID string) (Reservation, error) {
	if !strings.HasPrefix(raw, ownerPrefix) {
		return Reservation{}, ErrOwnership
	}
	parts := strings.Split(strings.TrimPrefix(raw, ownerPrefix), "/")
	if len(parts) != 5 {
		return Reservation{}, ErrOwnership
	}
	for i := range parts {
		value, err := url.PathUnescape(parts[i])
		if err != nil {
			return Reservation{}, fmt.Errorf("decoding reservation owner: %w", err)
		}
		parts[i] = value
	}
	r := Reservation{Owner: Owner{Network: parts[2], Sandbox: parts[3], Interface: parts[4]}, BootID: parts[1]}
	canonical, err := encodeOwner(vmID, r)
	if err != nil || parts[0] != vmID || canonical != raw {
		return Reservation{}, ErrOwnership
	}
	return r, nil
}

func usagePatch(node *unstructured.Unstructured, state State, vmID string) ([]byte, error) {
	used := make(map[string]any, len(state.Reservations))
	for ip, entry := range state.Reservations {
		owner, err := encodeOwner(vmID, entry)
		if err != nil {
			return nil, err
		}
		used[ip] = map[string]any{ownerField: owner, resourceField: entry.Resource}
	}
	current, _, err := unstructured.NestedMap(node.Object, statusField, ipamField, "used")
	if err != nil {
		return nil, fmt.Errorf("reading current usage: %w", err)
	}
	if reflect.DeepEqual(current, used) || len(current) == 0 && len(used) == 0 {
		return nil, nil
	}
	ops := []patchOperation{
		{"test", "/metadata/uid", string(node.GetUID())},
		{"test", "/metadata/resourceVersion", node.GetResourceVersion()},
	}
	for _, parent := range [][]string{{statusField}, {statusField, ipamField}} {
		_, found, parentErr := unstructured.NestedMap(node.Object, parent...)
		if parentErr != nil {
			return nil, fmt.Errorf("reading usage parent: %w", parentErr)
		}
		if !found {
			ops = append(ops, patchOperation{jsonPatchAdd, "/" + strings.Join(parent, "/"), map[string]any{}})
		}
	}
	ops = append(ops, patchOperation{jsonPatchAdd, "/status/ipam/used", used})
	data, err := json.Marshal(ops)
	if err != nil {
		return nil, fmt.Errorf("encoding usage update: %w", err)
	}
	return data, nil
}
