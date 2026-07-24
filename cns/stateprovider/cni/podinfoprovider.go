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
	kexec "k8s.io/utils/exec"
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
			return nil, errors.New("reading CNI endpoint state: context is nil")
		}
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("reading CNI endpoint state: %w", err)
		}
		state, err := cli.GetEndpointState()
		if err != nil {
			return nil, errors.New("reading CNI endpoint state: CNI query failed")
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
	records, err := translateEndpointState(state)
	if err != nil {
		return nil, err
	}
	podInfoByIP := make(map[string]cns.PodInfo)
	for _, record := range records {
		podInfo := cns.NewPodInfo(
			record.InfraContainerID,
			record.PodEndpointID,
			record.PodName,
			record.PodNamespace,
		)
		for _, prefix := range record.IPAddresses {
			address, _ := netip.AddrFromSlice(prefix.IP)
			podInfoByIP[address.Unmap().String()] = podInfo
		}
	}
	return podInfoByIP, nil
}

func translateEndpointState(state *api.AzureCNIState) ([]cns.CNIEndpointState, error) {
	if state == nil {
		return nil, errors.New("CNI endpoint state is nil")
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
			return nil, errors.New("CNI interface key is empty")
		case containerID == "":
			return nil, fmt.Errorf("CNI interface %q has empty container ID", key)
		case endpointID == "":
			return nil, fmt.Errorf("CNI interface %q has empty pod endpoint ID", key)
		case podName == "":
			return nil, fmt.Errorf("CNI interface %q has empty pod name", key)
		case podNamespace == "":
			return nil, fmt.Errorf("CNI interface %q has empty pod namespace", key)
		case ifName == "":
			return nil, fmt.Errorf("CNI interface %q has empty interface name", key)
		case len(endpoint.IPAddresses) == 0:
			return nil, fmt.Errorf("CNI interface %q has no IP addresses", key)
		}
		if other, ok := seenEndpointIDs[endpointID]; ok {
			return nil, fmt.Errorf("CNI interfaces %q and %q have duplicate pod endpoint ID", other, key)
		}
		identity := containerID + "\x00" + ifName
		if other, ok := seenInterfaces[identity]; ok {
			return nil, fmt.Errorf("CNI interfaces %q and %q have duplicate interface identity", other, key)
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
		return netip.Addr{}, net.IPNet{}, errors.New("invalid IP address")
	}
	address = address.Unmap()
	ones, bits := value.Mask.Size()
	expectedBits := 128
	if address.Is4() {
		expectedBits = 32
	}
	if bits != expectedBits || ones < 0 {
		return netip.Addr{}, net.IPNet{}, fmt.Errorf("invalid mask for %d-bit address", address.BitLen())
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
