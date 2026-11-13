package ciliumpool

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const (
	vmAnnotation       = "azure-container-networking.io/vm-id"
	consumerAnnotation = "azure-container-networking.io/ipam-consumer"
)

func bindGuest(cfg Config, node *corev1.Node, guest Guest) (Binding, error) {
	instance, err := arm.ParseResourceID(guest.InstanceID)
	if err != nil || !strings.EqualFold(instance.ResourceType.String(), "Microsoft.Compute/virtualMachines") ||
		node.Name != cfg.NodeName || node.UID == "" || node.DeletionTimestamp != nil ||
		guest.VMID == "" || guest.BootID == "" {
		return Binding{}, fmt.Errorf("%w: invalid node or VM identity", ErrOwnership)
	}
	id := strings.ToLower(instance.String())
	if node.Spec.ProviderID != "" && !strings.EqualFold(strings.TrimPrefix(node.Spec.ProviderID, "azure://"), id) {
		return Binding{}, fmt.Errorf("%w: provider ID differs from guest", ErrOwnership)
	}
	if guest.Primary.IsValid() {
		found := false
		for _, address := range node.Status.Addresses {
			found = found || address.Type == corev1.NodeInternalIP && address.Address == guest.Primary.String()
		}
		if !found {
			return Binding{}, fmt.Errorf("%w: Node internal IP differs from guest", ErrOwnership)
		}
	}
	return Binding{
		NodeUID: string(node.UID), VMID: guest.VMID, Resource: cfg.NICID, Subnet: cfg.Subnet,
		NodeName: node.Name, InstanceID: id, Primary: guest.Primary.String(), MAC: guest.MAC.String(),
	}, nil
}

func validateCiliumNode(node *unstructured.Unstructured, binding Binding) error {
	owners := node.GetOwnerReferences()
	instance, _, err := unstructured.NestedString(node.Object, specField, "instance-id")
	if err != nil {
		return fmt.Errorf("reading instance identity: %w", err)
	}
	if node.GetName() != binding.NodeName || node.GetUID() == "" || node.GetResourceVersion() == "" ||
		node.GetDeletionTimestamp() != nil || len(owners) != 1 || owners[0].APIVersion != "v1" ||
		owners[0].Kind != "Node" || owners[0].Name != binding.NodeName || string(owners[0].UID) != binding.NodeUID ||
		node.GetAnnotations()[vmAnnotation] != binding.VMID ||
		node.GetAnnotations()[consumerAnnotation] != PluginName || !strings.EqualFold(instance, binding.InstanceID) {
		return fmt.Errorf("%w: CiliumNode incarnation or consumer differs", ErrOwnership)
	}
	return nil
}

func registration(cfg Config, node *corev1.Node, guest Guest) (*unstructured.Unstructured, Binding, error) {
	binding, err := bindGuest(cfg, node, guest)
	if err != nil {
		return nil, Binding{}, err
	}
	if !guest.Primary.IsValid() {
		return nil, Binding{}, fmt.Errorf("%w: registration needs primary guest address", ErrOwnership)
	}
	state, err := json.Marshal(newState(binding.NodeUID, binding.VMID))
	if err != nil {
		return nil, Binding{}, fmt.Errorf("encoding initial ledger: %w", err)
	}
	cn := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "cilium.io/v2", "kind": "CiliumNode",
		"metadata": map[string]any{
			"name":            node.Name,
			"annotations":     map[string]any{consumerAnnotation: PluginName, vmAnnotation: guest.VMID, stateAnnotation: string(state)},
			"ownerReferences": []any{map[string]any{"apiVersion": "v1", "kind": "Node", "name": node.Name, "uid": string(node.UID)}},
		}, specField: map[string]any{
			"instance-id": binding.InstanceID,
			"addresses":   []any{map[string]any{"type": "InternalIP", "ip": guest.Primary.String()}},
			"azure":       map[string]any{"interface-name": cfg.NICID[strings.LastIndex(cfg.NICID, "/")+1:]}, ipamField: map[string]any{
				"min-allocate": int64(cfg.MinAllocate), "pre-allocate": int64(cfg.PreAllocate), "max-above-watermark": int64(cfg.MaxAboveWatermark), poolsField: map[string]any{
					"requested": []any{map[string]any{poolField: defaultPool, "needed": map[string]any{"ipv4-addrs": int64(cfg.PreAllocate)}}},
					"allocated": []any{map[string]any{poolField: defaultPool, "cidrs": []any{}}},
				},
			},
		},
	}}
	return cn, binding, nil
}
