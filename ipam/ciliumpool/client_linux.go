package ciliumpool

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
)

type Allocator struct {
	Nodes       dynamic.ResourceInterface
	NodeName    string
	Binding     Binding
	BootID      string
	PreAllocate int
	Supply      map[string]bool
}

type operation uint8

const (
	addOperation operation = iota
	deleteOperation
	reconcileOperation
)

var errConflictRetries = errors.New("CiliumNode conflict retry budget exhausted")

func (a *Allocator) Add(ctx context.Context, owner Owner) (string, error) {
	return a.update(ctx, owner, "", addOperation)
}

func (a *Allocator) Delete(ctx context.Context, owner Owner, address string) error {
	_, err := a.update(ctx, owner, address, deleteOperation)
	return err
}

func (a *Allocator) Reconcile(ctx context.Context) error {
	_, err := a.update(ctx, Owner{}, "", reconcileOperation)
	return err
}

func (a *Allocator) update(ctx context.Context, owner Owner, address string, command operation) (string, error) {
	boot, bootErr := parseBootID([]byte(a.BootID))
	if bootErr != nil || boot != a.BootID {
		return "", fmt.Errorf("%w: invalid current boot identity", ErrOwnership)
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	var uid types.UID
	for attempt := 0; attempt < 8; attempt++ {
		node, err := a.Nodes.Get(ctx, a.NodeName, metav1.GetOptions{})
		if err != nil {
			return "", fmt.Errorf("reading CiliumNode: %w", err)
		}
		if uid != "" && node.GetUID() != uid {
			return "", fmt.Errorf("%w: CiliumNode replaced during operation", ErrOwnership)
		}
		uid = node.GetUID()
		if callErr := validateCiliumNode(node, a.Binding); callErr != nil {
			return "", callErr
		}
		spare, found, err := unstructured.NestedInt64(node.Object, specField, ipamField, "pre-allocate")
		if err != nil || !found || spare != int64(a.PreAllocate) {
			return "", fmt.Errorf("%w: preallocation differs from operator policy", ErrConfig)
		}
		state, err := readState(node, a.Binding)
		if err != nil {
			return "", err
		}
		if callErr := state.RetireBoot(a.BootID); callErr != nil {
			return "", callErr
		}
		var supply []string
		if command != deleteOperation {
			supply, err = eligibleSupply(node, a.Binding, a.Supply)
			if err != nil {
				return "", err
			}
		}
		ip, resultErr := a.changeReservation(&state, owner, address, command, supply)
		if resultErr != nil && !errors.Is(resultErr, ErrNoSupply) {
			return "", resultErr
		}
		patch, err := usagePatch(node, state, a.Binding.VMID)
		if err != nil {
			return "", err
		}
		if patch == nil {
			return ip, resultErr
		}
		_, err = a.Nodes.Patch(ctx, a.NodeName, types.JSONPatchType, patch, metav1.PatchOptions{}, statusField)
		if err == nil {
			return ip, resultErr
		}
		if !apierrors.IsConflict(err) && !apierrors.IsInvalid(err) {
			return "", fmt.Errorf("committing IPAM state: %w", err)
		}
		current, readErr := a.Nodes.Get(ctx, a.NodeName, metav1.GetOptions{})
		if readErr != nil {
			return "", fmt.Errorf("checking conditional update failure: %w", readErr)
		}
		if current.GetUID() != uid {
			return "", fmt.Errorf("%w: CiliumNode replaced during update", ErrOwnership)
		}
		if current.GetResourceVersion() == node.GetResourceVersion() {
			return "", fmt.Errorf("conditional update failed without concurrent change: %w", err)
		}
		timer := time.NewTimer(time.Duration(attempt+1) * 10 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return "", fmt.Errorf("updating IPAM state: %w", ctx.Err())
		case <-timer.C:
		}
	}
	return "", errConflictRetries
}

func (a *Allocator) changeReservation(state *State, owner Owner, address string, command operation, supply []string) (string, error) {
	switch command {
	case addOperation:
		return state.Reserve(owner, a.BootID, a.Binding.Resource, supply)
	case deleteOperation:
		return "", state.Release(owner, address)
	case reconcileOperation:
		return "", nil
	default:
		return "", ErrConfig
	}
}

func publishedPool(node *unstructured.Unstructured, binding Binding) (map[string]any, error) {
	pool, _, poolErr := unstructured.NestedMap(node.Object, specField, ipamField, poolField)
	if poolErr != nil {
		return nil, fmt.Errorf("reading operator pool: %w", poolErr)
	}
	for raw, entry := range pool {
		ip, err := netip.ParseAddr(raw)
		allocation, ok := entry.(map[string]any)
		resource, resourceOK := allocation[resourceField].(string)
		subnet, subnetErr := netip.ParsePrefix(binding.Subnet)
		if err != nil || !ip.Is4() || subnetErr != nil || !subnet.Contains(ip) || raw == binding.Primary ||
			!ok || !resourceOK || !strings.EqualFold(resource, binding.Resource) {
			return nil, fmt.Errorf("%w: invalid operator pool entry", ErrOwnership)
		}
	}
	return pool, nil
}

func eligibleSupply(node *unstructured.Unstructured, binding Binding, guest map[string]bool) ([]string, error) {
	pool, poolErr := publishedPool(node, binding)
	if poolErr != nil {
		return nil, poolErr
	}
	interfaces, _, err := unstructured.NestedSlice(node.Object, statusField, "azure", "interfaces")
	if err != nil {
		return nil, fmt.Errorf("reading Azure interfaces: %w", err)
	}
	var result []string
	selected := false
	for _, raw := range interfaces {
		iface, ok := raw.(map[string]any)
		if !ok {
			return nil, ErrOwnership
		}
		id, _ := iface["id"].(string)
		if !strings.EqualFold(id, binding.Resource) {
			continue
		}
		if selected {
			return nil, fmt.Errorf("%w: duplicate Azure interface", ErrOwnership)
		}
		selected = true
		macText, _ := iface[macField].(string)
		mac, macErr := net.ParseMAC(macText)
		subnet, _, subnetErr := unstructured.NestedString(iface, subnetField, cidrField)
		prefix, prefixErr := netip.ParsePrefix(binding.Subnet)
		if macErr != nil || mac.String() != binding.MAC || iface["ip"] != binding.Primary ||
			subnetErr != nil || subnet != binding.Subnet || prefixErr != nil ||
			iface[gatewayField] != prefix.Addr().Next().String() ||
			(iface[stateField] != nil && iface[stateField] != "" && iface[stateField] != stateSucceeded) {
			return nil, fmt.Errorf("%w: operator metadata differs from guest", ErrOwnership)
		}
		addresses, _, err := unstructured.NestedSlice(iface, "addresses")
		if err != nil {
			return nil, fmt.Errorf("reading Azure addresses: %w", err)
		}
		for _, raw := range addresses {
			address, ok := raw.(map[string]any)
			if !ok {
				return nil, ErrOwnership
			}
			ip, ok := address["ip"].(string)
			if !ok {
				return nil, ErrOwnership
			}
			parsed, parseErr := netip.ParseAddr(ip)
			if parseErr != nil || !parsed.Is4() || !prefix.Contains(parsed) || ip == binding.Primary {
				return nil, fmt.Errorf("%w: invalid secondary address", ErrOwnership)
			}
			_, published := pool[ip]
			if address[stateField] == stateSucceeded && guest[ip] && published {
				result = append(result, ip)
			}
		}
	}
	return result, nil
}
