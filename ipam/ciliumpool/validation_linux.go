package ciliumpool

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/netip"
	"slices"

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

func readState(node *unstructured.Unstructured, binding Binding) (State, []string, error) {
	var state State
	d := json.NewDecoder(bytes.NewBufferString(node.GetAnnotations()[stateAnnotation]))
	d.DisallowUnknownFields()
	if err := d.Decode(&state); err != nil {
		return state, nil, fmt.Errorf("decoding remote ledger: %w", err)
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return state, nil, fmt.Errorf("%w: trailing ledger data", ErrOwnership)
	}
	if state.Version != 1 || state.NodeUID != binding.NodeUID || state.VMID != binding.VMID ||
		state.Reservations == nil || state.Pending == nil || state.Withdrawn == nil ||
		len(state.Reservations) > 255 || len(state.Pending) > 255 || len(state.Withdrawn) > 255 {
		return state, nil, fmt.Errorf("%w: ledger identity or schema mismatch", ErrOwnership)
	}
	if err := validateProtocol(node); err != nil {
		return state, nil, err
	}
	subnet, err := netip.ParsePrefix(binding.Subnet)
	if err != nil || !subnet.Addr().Is4() {
		return state, nil, fmt.Errorf("%w: invalid subnet", ErrOwnership)
	}
	retained, err := retainedPrefixes(node, subnet)
	if err != nil {
		return state, nil, err
	}
	if validationErr := validateOwnership(state, binding, subnet, retained); validationErr != nil {
		return state, nil, validationErr
	}
	return state, retained, nil
}

func validateProtocol(node *unstructured.Unstructured) error {
	used, _, err := unstructured.NestedMap(node.Object, statusField, ipamField, "used")
	if err != nil || len(used) != 0 {
		return fmt.Errorf("%w: legacy usage must be empty", ErrOwnership)
	}
	releases, _, err := unstructured.NestedMap(node.Object, statusField, ipamField, "release-ips")
	if err != nil || len(releases) != 0 {
		return fmt.Errorf("%w: legacy release handshake is incompatible", ErrOwnership)
	}
	requests, _, err := unstructured.NestedSlice(node.Object, specField, ipamField, poolsField, "requested")
	if err != nil || len(requests) != 1 {
		return fmt.Errorf("%w: expected default pool demand", ErrOwnership)
	}
	request, ok := requests[0].(map[string]any)
	if !ok || request[poolField] != defaultPool {
		return fmt.Errorf("%w: foreign requested pool", ErrOwnership)
	}
	count, found, err := unstructured.NestedInt64(request, "needed", "ipv4-addrs")
	if err != nil || !found || count < 0 {
		return fmt.Errorf("%w: invalid requested capacity", ErrOwnership)
	}
	ipv6, _, err := unstructured.NestedInt64(request, "needed", "ipv6-addrs")
	if err != nil || ipv6 != 0 {
		return fmt.Errorf("%w: IPv6 demand is unsupported", ErrOwnership)
	}
	return nil
}

func retainedPrefixes(node *unstructured.Unstructured, subnet netip.Prefix) ([]string, error) {
	raw, found, err := unstructured.NestedSlice(node.Object, specField, ipamField, poolsField, "allocated")
	if err != nil || !found || len(raw) != 1 {
		return nil, fmt.Errorf("%w: expected one retained pool", ErrOwnership)
	}
	pool, ok := raw[0].(map[string]any)
	if !ok || pool[poolField] != defaultPool {
		return nil, fmt.Errorf("%w: expected default retained pool", ErrOwnership)
	}
	retained, _, err := unstructured.NestedStringSlice(pool, "cidrs")
	if err != nil {
		return nil, fmt.Errorf("reading retained prefixes: %w", err)
	}
	seen := map[string]bool{}
	for _, cidr := range retained {
		prefix, parseErr := netip.ParsePrefix(cidr)
		if parseErr != nil || !prefix.Addr().Is4() || prefix.Bits() != 32 ||
			!subnet.Contains(prefix.Addr()) || prefix.String() != cidr || seen[cidr] {
			return nil, fmt.Errorf("%w: invalid retained prefix", ErrOwnership)
		}
		seen[cidr] = true
	}
	return retained, nil
}

func validateOwnership(state State, binding Binding, subnet netip.Prefix, retained []string) error {
	owners := map[Owner]bool{}
	for ip, reservation := range state.Reservations {
		owner := reservation.Owner
		boot, bootErr := parseBootID([]byte(reservation.BootID))
		if !slices.Contains(retained, ip+"/32") || reservation.Resource != binding.Resource ||
			bootErr != nil || boot != reservation.BootID || !owner.valid() || owners[owner] || state.Withdrawn[ip] {
			return fmt.Errorf("%w: reservation outside retained resource", ErrOwnership)
		}
		owners[owner] = true
	}
	for key, pending := range state.Pending {
		o := pending.Owner
		boot, bootErr := parseBootID([]byte(pending.BootID))
		if key != o.key() || !o.valid() || bootErr != nil || boot != pending.BootID || pending.Expires.IsZero() || owners[o] {
			return fmt.Errorf("%w: invalid pending demand", ErrOwnership)
		}
	}
	for raw, excluded := range state.Withdrawn {
		ip, err := netip.ParseAddr(raw)
		if !excluded || err != nil || !ip.Is4() || !subnet.Contains(ip) || slices.Contains(retained, raw+"/32") {
			return fmt.Errorf("%w: invalid withdrawn address", ErrOwnership)
		}
	}
	return nil
}
