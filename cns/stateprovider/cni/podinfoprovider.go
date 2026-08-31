package cni

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"sort"
	"strings"

	"github.com/Azure/azure-container-networking/cni/api"
	"github.com/Azure/azure-container-networking/cni/client"
	"github.com/Azure/azure-container-networking/cns"
	pkgerrors "github.com/pkg/errors"
	kexec "k8s.io/utils/exec"
)

var (
	errNilCNIEndpointContext       = errors.New("reading CNI endpoint state: context is nil")
	errCNIEndpointQueryFailed      = errors.New("reading CNI endpoint state: CNI query failed")
	errNilCNIEndpointState         = errors.New("CNI endpoint state is nil")
	errEmptyCNIInterfaceKey        = errors.New("CNI interface key is empty")
	errEmptyCNIContainerID         = errors.New("CNI interface container ID is empty")
	errEmptyCNIPodEndpointID       = errors.New("CNI interface pod endpoint ID is empty")
	errEmptyCNIPodName             = errors.New("CNI interface pod name is empty")
	errEmptyCNIPodNamespace        = errors.New("CNI interface pod namespace is empty")
	errEmptyCNIInterfaceName       = errors.New("CNI interface name is empty")
	errEmptyCNIIPAddresses         = errors.New("CNI interface has no IP addresses")
	errDuplicateCNIPodEndpointID   = errors.New("duplicate CNI pod endpoint ID")
	errDuplicateCNIInterface       = errors.New("duplicate CNI interface identity")
	errInvalidCNIProviderIPAddress = errors.New("invalid CNI IP address")
	errInvalidCNIProviderIPMask    = errors.New("invalid CNI IP mask")
)

// New returns an implementation of cns.PodInfoByIPProvider
// that execs out to the CNI and uses the response to build the PodInfo map.
func New() (cns.PodInfoByIPProvider, error) {
	return podInfoProvider(kexec.New())
}

func podInfoProvider(exec kexec.Interface) (cns.PodInfoByIPProvider, error) {
	cli := client.New(exec)
	state, err := cli.GetEndpointState()
	if err != nil {
		return nil, fmt.Errorf("failed to invoke CNI client.GetEndpointState(): %w", err)
	}
	return cns.PodInfoByIPProviderFunc(func() (map[string]cns.PodInfo, error) {
		return cniStateToPodInfoByIP(state)
	}), nil
}

// NewEndpointStateProvider returns a function that queries stateful Azure CNI
// on every call. The CNI exec API cannot interrupt a running command, so the
// context is checked immediately before and after that bounded operation.
func NewEndpointStateProvider() cns.CNIEndpointStateProvider {
	return endpointStateProvider(kexec.New())
}

func endpointStateProvider(exec kexec.Interface) cns.CNIEndpointStateProvider {
	cli := client.New(exec)
	return func(ctx context.Context) ([]cns.CNIEndpointState, error) {
		if ctx == nil {
			return nil, errNilCNIEndpointContext
		}
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("reading CNI endpoint state: %w", err)
		}
		state, err := cli.GetEndpointState()
		if err != nil {
			return nil, errCNIEndpointQueryFailed
		}
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("reading CNI endpoint state: %w", err)
		}
		return translateEndpointState(state)
	}
}

// cniStateToPodInfoByIP converts an AzureCNIState dumped from a CNI exec
// into a PodInfo map, using the endpoint IPs as keys in the map.
// for pods with multiple IPs (such as in dualstack cases), this means multiple keys in the map
// will point to the same pod information.
func cniStateToPodInfoByIP(state *api.AzureCNIState) (map[string]cns.PodInfo, error) {
	podInfoByIP := map[string]cns.PodInfo{}
	for _, endpoint := range state.ContainerInterfaces {
		for _, epIP := range endpoint.IPAddresses {
			podInfo := cns.NewPodInfo(endpoint.ContainerID, endpoint.PodEndpointId, endpoint.PodName, endpoint.PodNamespace)

			ipKey := epIP.IP.String()
			if prevPodInfo, ok := podInfoByIP[ipKey]; ok {
				return nil, pkgerrors.Wrapf(cns.ErrDuplicateIP, "duplicate ip %s found for different pods: pod: %+v, pod: %+v", ipKey, podInfo, prevPodInfo)
			}

			podInfoByIP[ipKey] = podInfo
		}
	}
	return podInfoByIP, nil
}

func translateEndpointState(state *api.AzureCNIState) ([]cns.CNIEndpointState, error) {
	if state == nil {
		return nil, errNilCNIEndpointState
	}
	keys := make([]string, 0, len(state.ContainerInterfaces))
	for key := range state.ContainerInterfaces {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	records := make([]cns.CNIEndpointState, 0, len(keys))
	seenAddresses := make(map[netip.Addr]string)
	seenEndpointIDs := make(map[string]string)
	seenInterfaces := make(map[string]string)
	for _, rawKey := range keys {
		key := strings.TrimSpace(rawKey)
		endpoint := state.ContainerInterfaces[rawKey]
		containerID := strings.TrimSpace(endpoint.ContainerID)
		endpointID := strings.TrimSpace(endpoint.PodEndpointId)
		podName := strings.TrimSpace(endpoint.PodName)
		podNamespace := strings.TrimSpace(endpoint.PodNamespace)
		ifName := strings.TrimSpace(endpoint.IfName)
		if ifName == "" {
			ifName = interfaceNameFromKey(key)
		}
		switch {
		case key == "":
			return nil, errEmptyCNIInterfaceKey
		case containerID == "":
			return nil, fmt.Errorf("%w: %q", errEmptyCNIContainerID, key)
		case endpointID == "":
			return nil, fmt.Errorf("%w: %q", errEmptyCNIPodEndpointID, key)
		case podName == "":
			return nil, fmt.Errorf("%w: %q", errEmptyCNIPodName, key)
		case podNamespace == "":
			return nil, fmt.Errorf("%w: %q", errEmptyCNIPodNamespace, key)
		case ifName == "":
			return nil, fmt.Errorf("%w: %q", errEmptyCNIInterfaceName, key)
		case len(endpoint.IPAddresses) == 0:
			return nil, fmt.Errorf("%w: %q", errEmptyCNIIPAddresses, key)
		}
		if other, ok := seenEndpointIDs[endpointID]; ok {
			return nil, fmt.Errorf("%w: %q and %q", errDuplicateCNIPodEndpointID, other, key)
		}
		identity := containerID + "\x00" + ifName
		if other, ok := seenInterfaces[identity]; ok {
			return nil, fmt.Errorf("%w: %q and %q", errDuplicateCNIInterface, other, key)
		}

		ipAddresses := make([]net.IPNet, 0, len(endpoint.IPAddresses))
		for index, value := range endpoint.IPAddresses {
			address, prefix, err := parseIPNet(value)
			if err != nil {
				return nil, fmt.Errorf("CNI interface %q IP %d: %w", key, index, err)
			}
			if other, ok := seenAddresses[address]; ok {
				return nil, fmt.Errorf(
					"%w: CNI interfaces %q and %q contain duplicate address",
					cns.ErrDuplicateIP,
					other,
					key,
				)
			}
			seenAddresses[address] = key
			ipAddresses = append(ipAddresses, prefix)
		}
		sort.Slice(ipAddresses, func(i, j int) bool {
			left, _ := netip.AddrFromSlice(ipAddresses[i].IP)
			right, _ := netip.AddrFromSlice(ipAddresses[j].IP)
			return left.Unmap().Compare(right.Unmap()) < 0
		})
		seenEndpointIDs[endpointID] = key
		seenInterfaces[identity] = key
		records = append(records, cns.CNIEndpointState{
			InfraContainerID: containerID,
			PodEndpointID:    endpointID,
			PodName:          podName,
			PodNamespace:     podNamespace,
			InterfaceKey:     key,
			InterfaceName:    ifName,
			IPAddresses:      ipAddresses,
		})
	}
	return records, nil
}

func parseIPNet(value net.IPNet) (netip.Addr, net.IPNet, error) {
	address, ok := netip.AddrFromSlice(value.IP)
	if !ok {
		return netip.Addr{}, net.IPNet{}, errInvalidCNIProviderIPAddress
	}
	address = address.Unmap()
	ones, bits := value.Mask.Size()
	expectedBits := 128
	if address.Is4() {
		expectedBits = 32
	}
	if bits != expectedBits || ones < 0 {
		return netip.Addr{}, net.IPNet{}, fmt.Errorf("%w: %d-bit address", errInvalidCNIProviderIPMask, address.BitLen())
	}
	return address, net.IPNet{
		IP:   net.IP(address.AsSlice()),
		Mask: net.CIDRMask(ones, expectedBits),
	}, nil
}

func interfaceNameFromKey(key string) string {
	index := strings.LastIndex(key, "-eth")
	if index < 0 {
		return "eth0"
	}
	name := key[index+1:]
	for _, value := range strings.TrimPrefix(name, "eth") {
		if value < '0' || value > '9' {
			return ""
		}
	}
	if name == "eth" {
		return ""
	}
	return name
}
