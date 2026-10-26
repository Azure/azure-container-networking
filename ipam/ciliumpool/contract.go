// Package ciliumpool adapts Cilium's Azure IPAM contract without running its agent.
package ciliumpool

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"slices"
	"strings"

	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/ipam"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	CiliumNodes           = schema.GroupVersionResource{Group: "cilium.io", Version: "v2", Resource: "ciliumnodes"}
	Nodes                 = schema.GroupVersionResource{Version: "v1", Resource: "nodes"}
	errInvalidContract    = errors.New("ciliumpool: invalid contract")
	errMissingNICMetadata = errors.New("ciliumpool: waiting for selected NIC metadata")
)

const (
	consumerAnnotation = "azure-container-networking.io/ipam-consumer"
	consumer           = "azure-vnet-ipam-bridge"
	jsonPatchAdd       = "add"
)

// This wire subset is pinned to Cilium v1.20.1, commit
// 7d68cfb394f2960e10aa72e76d0d51e66c1b2ebc. Unowned fields remain in the
// original unstructured object and are never serialized back wholesale.
type nodeContract struct {
	Metadata metav1.ObjectMeta `json:"metadata"`
	Spec     struct {
		InstanceID string `json:"instance-id"`
		Azure      struct {
			InterfaceName string `json:"interface-name"`
		} `json:"azure"`
		IPAM struct {
			Pool        map[string]ipam.CiliumPoolUsage `json:"pool"`
			MinAllocate int                             `json:"min-allocate"`
			PreAllocate int                             `json:"pre-allocate"`
		} `json:"ipam"`
	} `json:"spec"`
	Status struct {
		Azure struct {
			Interfaces []struct {
				ID      string `json:"id"`
				Name    string `json:"name"`
				MAC     string `json:"mac"`
				IP      string `json:"ip"`
				State   string `json:"state"`
				Gateway string `json:"gateway"`
				Subnet  struct {
					ID   string `json:"id"`
					CIDR string `json:"cidr"`
				} `json:"subnet"`
				Addresses []struct {
					IP    string `json:"ip"`
					State string `json:"state"`
				} `json:"addresses"`
			} `json:"interfaces"`
		} `json:"azure"`
		IPAM struct {
			Used       map[string]ipam.CiliumPoolUsage `json:"used"`
			ReleaseIPs map[string]string               `json:"release-ips"`
		} `json:"ipam"`
	} `json:"status"`
}

func resourceID(raw, resourceType string) (string, error) {
	raw = strings.TrimPrefix(raw, "azure://")
	id, err := arm.ParseResourceID(raw)
	if err != nil || id.SubscriptionID == "" || id.ResourceGroupName == "" || id.Name == "" ||
		!strings.EqualFold(id.ResourceType.String(), resourceType) {
		return "", fmt.Errorf("%w: invalid %s resource ID %q", errInvalidContract, resourceType, raw)
	}
	return strings.ToLower(id.String()), nil
}

func identity(cfg *common.CiliumPoolConfig, node *corev1.Node, cn *unstructured.Unstructured,
	g Guest,
) (ipam.CiliumPoolIdentity, error) {
	instance, err := resourceID(g.InstanceID, "Microsoft.Compute/virtualMachines")
	if err != nil {
		return ipam.CiliumPoolIdentity{}, err
	}
	if node.Name != cfg.NodeName || node.UID == "" || node.DeletionTimestamp != nil || g.VMID == "" {
		return ipam.CiliumPoolIdentity{}, fmt.Errorf("%w: invalid node or VM identity", errInvalidContract)
	}
	if node.Spec.ProviderID != "" {
		provider, providerErr := resourceID(node.Spec.ProviderID, "Microsoft.Compute/virtualMachines")
		if providerErr != nil || provider != instance {
			return ipam.CiliumPoolIdentity{}, fmt.Errorf("%w: node provider ID does not match IMDS", errInvalidContract)
		}
	}
	matches := false
	for _, addr := range node.Status.Addresses {
		if addr.Type == corev1.NodeInternalIP && addr.Address == g.Primary.String() {
			matches = true
		}
	}
	if !matches {
		return ipam.CiliumPoolIdentity{}, fmt.Errorf("%w: node internal IP does not match selected primary NIC", errInvalidContract)
	}
	nic, err := resourceID(cfg.NICID, "Microsoft.Network/networkInterfaces")
	if err != nil {
		return ipam.CiliumPoolIdentity{}, err
	}
	var contract nodeContract
	data, err := cn.MarshalJSON()
	if err != nil {
		return ipam.CiliumPoolIdentity{}, fmt.Errorf("encoding cilium node: %w", err)
	}
	if err = json.Unmarshal(data, &contract); err != nil {
		return ipam.CiliumPoolIdentity{}, fmt.Errorf("decoding cilium node: %w", err)
	}
	if cn.GetUID() == "" || cn.GetDeletionTimestamp() != nil ||
		cn.GetAnnotations()[consumerAnnotation] != consumer ||
		contract.Spec.InstanceID != instance ||
		contract.Spec.Azure.InterfaceName != cfg.NICID[strings.LastIndex(cfg.NICID, "/")+1:] ||
		contract.Spec.IPAM.MinAllocate != cfg.MinAllocate ||
		contract.Spec.IPAM.PreAllocate != cfg.PreAllocate {
		return ipam.CiliumPoolIdentity{}, fmt.Errorf("%w: node consumer or allocation contract changed", errInvalidContract)
	}
	owners := cn.GetOwnerReferences()
	if len(owners) != 1 || owners[0].APIVersion != "v1" || owners[0].Kind != "Node" ||
		owners[0].Name != node.Name || owners[0].UID != node.UID {
		return ipam.CiliumPoolIdentity{}, fmt.Errorf("%w: cilium node owner does not match node UID", errInvalidContract)
	}
	return ipam.CiliumPoolIdentity{
		NodeUID: string(node.UID), CiliumNodeUID: string(cn.GetUID()),
		InstanceID: instance, VMID: g.VMID, NICID: nic,
		Interface: cfg.Interface, Subnet: cfg.Subnet,
	}, nil
}

func eligible(cn *unstructured.Unstructured, id ipam.CiliumPoolIdentity, g Guest) ([]ipam.CiliumPoolAddress, int, error) {
	data, err := cn.MarshalJSON()
	if err != nil {
		return nil, 0, fmt.Errorf("encoding cilium pool: %w", err)
	}
	var n nodeContract
	if err = json.Unmarshal(data, &n); err != nil {
		return nil, 0, fmt.Errorf("decoding cilium pool: %w", err)
	}
	prefix, err := netip.ParsePrefix(id.Subnet)
	if err != nil || prefix != g.Subnet || prefix != prefix.Masked() || !prefix.Addr().Is4() {
		return nil, 0, fmt.Errorf("%w: subnet does not match guest", errInvalidContract)
	}
	for i := range n.Status.Azure.Interfaces {
		iface := &n.Status.Azure.Interfaces[i]
		if !strings.EqualFold(iface.ID, id.NICID) {
			continue
		}
		mac, err := net.ParseMAC(iface.MAC)
		if err != nil || !bytes.Equal(mac, g.MAC) || iface.State != "succeeded" ||
			iface.IP != g.Primary.String() || iface.Subnet.CIDR != id.Subnet ||
			iface.Subnet.ID == "" || iface.Name != n.Spec.Azure.InterfaceName ||
			iface.Gateway != prefix.Addr().Next().String() {
			return nil, 0, fmt.Errorf("%w: operator NIC metadata does not match guest", errInvalidContract)
		}
		programmed := make(map[netip.Addr]bool)
		for _, raw := range iface.Addresses {
			addr, err := netip.ParseAddr(raw.IP)
			if err != nil {
				return nil, 0, fmt.Errorf("%w: invalid operator NIC address", errInvalidContract)
			}
			if raw.State == "succeeded" {
				programmed[addr] = true
			}
		}
		var supply []ipam.CiliumPoolAddress
		for raw, allocation := range n.Spec.IPAM.Pool {
			addr, err := netip.ParseAddr(raw)
			if err != nil || !addr.Is4() || !prefix.Contains(addr) ||
				!strings.EqualFold(allocation.Resource, id.NICID) || addr == g.Primary {
				return nil, 0, fmt.Errorf("%w: foreign, primary or invalid pool entry %q", errInvalidContract, raw)
			}
			if _, release := n.Status.IPAM.ReleaseIPs[raw]; release {
				continue
			}
			if programmed[addr] && g.Secondaries[addr] {
				supply = append(supply, ipam.CiliumPoolAddress{Address: addr, Resource: allocation.Resource})
			}
		}
		slices.SortFunc(supply, func(a, b ipam.CiliumPoolAddress) int { return a.Address.Compare(b.Address) })
		return supply, len(n.Spec.IPAM.Pool), nil
	}
	return nil, len(n.Spec.IPAM.Pool), errMissingNICMetadata
}

func usagePatch(cn *unstructured.Unstructured, used map[string]ipam.CiliumPoolUsage) ([]byte, error) {
	type operation struct {
		Op    string      `json:"op"`
		Path  string      `json:"path"`
		Value interface{} `json:"value"`
	}
	ops := []operation{
		{"test", "/metadata/resourceVersion", cn.GetResourceVersion()},
		{"test", "/metadata/uid", string(cn.GetUID())},
	}
	if _, found, err := unstructured.NestedMap(cn.Object, "status"); err != nil {
		return nil, fmt.Errorf("reading cilium node status: %w", err)
	} else if !found {
		ops = append(ops, operation{jsonPatchAdd, "/status", map[string]interface{}{}})
	}
	if _, found, err := unstructured.NestedMap(cn.Object, "status", "ipam"); err != nil {
		return nil, fmt.Errorf("reading cilium node IPAM status: %w", err)
	} else if !found {
		ops = append(ops, operation{jsonPatchAdd, "/status/ipam", map[string]interface{}{}})
	}
	ops = append(ops, operation{jsonPatchAdd, "/status/ipam/used", used})
	data, err := json.Marshal(ops)
	if err != nil {
		return nil, fmt.Errorf("encoding usage patch: %w", err)
	}
	return data, nil
}
