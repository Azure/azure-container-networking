package ciliumpool

import (
	"fmt"
	"net/netip"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type Binding struct {
	Primary    string
	MAC        string
	NodeName   string
	InstanceID string
	NodeUID    string
	VMID       string
	Resource   string
	Subnet     string
}

func readState(node *unstructured.Unstructured, binding Binding) (State, error) {
	state := newState()
	if _, present := node.GetAnnotations()["azure-container-networking.io/ipam-state"]; present {
		return state, fmt.Errorf("%w: annotation-backed allocations require a drained node before switching consumers", ErrOwnership)
	}
	pools, _, err := unstructured.NestedMap(node.Object, specField, ipamField, "pools")
	if err != nil || len(pools) != 0 {
		return state, fmt.Errorf("%w: multi-pool state is unsupported", ErrOwnership)
	}
	releases, _, err := unstructured.NestedMap(node.Object, statusField, ipamField, "release-ips")
	if err != nil || len(releases) != 0 {
		return state, fmt.Errorf("%w: cloud release is unsupported", ErrOwnership)
	}
	used, _, err := unstructured.NestedMap(node.Object, statusField, ipamField, "used")
	if err != nil {
		return state, fmt.Errorf("reading usage: %w", err)
	}
	if len(used) > 255 {
		return state, fmt.Errorf("%w: node reservation limit exceeded", ErrOwnership)
	}
	subnet, err := netip.ParsePrefix(binding.Subnet)
	if err != nil || !subnet.Addr().Is4() {
		return state, ErrOwnership
	}
	owners := map[Owner]bool{}
	for raw, record := range used {
		ip, err := netip.ParseAddr(raw)
		if err != nil || !ip.Is4() || ip.String() != raw || !subnet.Contains(ip) || raw == binding.Primary {
			return state, fmt.Errorf("%w: invalid reserved address", ErrOwnership)
		}
		entry, ok := record.(map[string]any)
		if !ok {
			return state, ErrOwnership
		}
		owner, ok := entry[ownerField].(string)
		if !ok {
			return state, ErrOwnership
		}
		reservation, err := decodeOwner(owner, binding.VMID)
		resource, resourceOK := entry[resourceField].(string)
		if err != nil || !resourceOK || !strings.EqualFold(resource, binding.Resource) || owners[reservation.Owner] {
			return state, fmt.Errorf("%w: foreign or duplicate reservation", ErrOwnership)
		}
		reservation.Resource = binding.Resource
		state.Reservations[raw] = reservation
		owners[reservation.Owner] = true
	}
	return state, nil
}
