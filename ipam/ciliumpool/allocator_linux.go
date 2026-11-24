package ciliumpool

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strings"
	"time"

	"github.com/avast/retry-go/v4"
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

func NewAllocator(ctx context.Context, cfg Config, g Guest, client dynamic.Interface) (*Allocator, error) {
	node, err := readNode(ctx, cfg.NodeName, client)
	if err != nil {
		return nil, err
	}
	binding, err := bindGuest(cfg, node, g)
	if err != nil {
		return nil, err
	}
	return &Allocator{
		Nodes: client.Resource(CiliumNodes), NodeName: cfg.NodeName, Binding: binding,
		BootID: g.BootID, PreAllocate: cfg.PreAllocate, Supply: g.Supply,
	}, nil
}

type operation uint8

const (
	addOperation operation = iota
	deleteOperation
)

var errConflictRetries = errors.New("CiliumNode conflict retry budget exhausted")

func (a *Allocator) Add(ctx context.Context, owner Owner) (string, error) {
	return a.update(ctx, owner, "", addOperation)
}

func (a *Allocator) Delete(ctx context.Context, owner Owner, address string) error {
	_, err := a.update(ctx, owner, address, deleteOperation)
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
	ip, err := retry.DoWithData(func() (string, error) {
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
		if command != deleteOperation && (err != nil || !found || spare != int64(a.PreAllocate)) {
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
		return "", errConflictRetries
	}, retry.Context(ctx), retry.Attempts(8), retry.LastErrorOnly(true),
		retry.RetryIf(func(err error) bool { return errors.Is(err, errConflictRetries) }),
		retry.Delay(10*time.Millisecond), retry.MaxDelay(80*time.Millisecond), retry.DelayType(retry.BackOffDelay))
	if err != nil {
		return "", fmt.Errorf("updating IPAM state: %w", err)
	}
	return ip, nil
}

func (a *Allocator) changeReservation(state *State, owner Owner, address string, command operation, supply []string) (string, error) {
	switch command {
	case addOperation:
		return state.Reserve(owner, a.BootID, a.Binding.Resource, supply)
	case deleteOperation:
		return "", state.Release(owner, address)
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
		prefix, err := validateInterface(iface, binding)
		if err != nil {
			return nil, err
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

func validateInterface(iface map[string]any, binding Binding) (netip.Prefix, error) {
	macText, _ := iface[macField].(string)
	mac, err := net.ParseMAC(macText)
	if err != nil || mac.String() != binding.MAC {
		return netip.Prefix{}, fmt.Errorf("%w: operator MAC differs from guest", ErrOwnership)
	}
	if iface["ip"] != binding.Primary {
		return netip.Prefix{}, fmt.Errorf("%w: operator primary IP differs from guest", ErrOwnership)
	}
	subnet, _, err := unstructured.NestedString(iface, subnetField, cidrField)
	if err != nil || subnet != binding.Subnet {
		return netip.Prefix{}, fmt.Errorf("%w: operator subnet differs from guest", ErrOwnership)
	}
	prefix, err := netip.ParsePrefix(binding.Subnet)
	if err != nil {
		return netip.Prefix{}, fmt.Errorf("%w: invalid bound subnet", ErrOwnership)
	}
	if iface[gatewayField] != prefix.Addr().Next().String() {
		return netip.Prefix{}, fmt.Errorf("%w: operator gateway differs from guest", ErrOwnership)
	}
	switch iface[stateField] {
	case nil, "", stateSucceeded:
		return prefix, nil
	default:
		return netip.Prefix{}, fmt.Errorf("%w: operator interface provisioning is not successful", ErrOwnership)
	}
}
