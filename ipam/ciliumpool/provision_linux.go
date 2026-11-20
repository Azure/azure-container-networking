package ciliumpool

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

var CiliumNodes = schema.GroupVersionResource{Group: "cilium.io", Version: "v2", Resource: "ciliumnodes"}

func readNode(ctx context.Context, name string, client dynamic.Interface) (*corev1.Node, error) {
	raw, err := client.Resource(schema.GroupVersionResource{Version: "v1", Resource: "nodes"}).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("reading Kubernetes node: %w", err)
	}
	node := &corev1.Node{}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(raw.Object, node); err != nil {
		return nil, fmt.Errorf("decoding Kubernetes node: %w", err)
	}
	return node, nil
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

func Provision(ctx context.Context, cfg Config, g Guest, client dynamic.Interface) error {
	node, err := readNode(ctx, cfg.NodeName, client)
	if err != nil {
		return err
	}
	initial, binding, err := registration(cfg, node, g)
	if err != nil {
		return err
	}
	nodes := client.Resource(CiliumNodes)
	current, err := nodes.Get(ctx, cfg.NodeName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		current, err = nodes.Create(ctx, initial, metav1.CreateOptions{})
		if apierrors.IsAlreadyExists(err) {
			current, err = nodes.Get(ctx, cfg.NodeName, metav1.GetOptions{})
		}
	}
	if err != nil {
		return fmt.Errorf("registering CiliumNode: %w", err)
	}
	if callErr := validateCiliumNode(current, binding); callErr != nil {
		return callErr
	}
	_, err = readState(current, binding)
	return err
}
