package main

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

// FileLineReader interface for reading lines from files
type FileLineReader interface {
	Read(filename string) ([]string, error)
}

// IPTablesClient interface for iptables operations
type IPTablesClient interface {
	ListChains(table string) ([]string, error)
	List(table, chain string) ([]string, error)
}

// KubeClient interface with direct methods for testing
type KubeClient interface {
	GetNode(ctx context.Context, name string) (*corev1.Node, error)
	CreateEvent(ctx context.Context, namespace string, event *corev1.Event) (*corev1.Event, error)
}

// DynamicClient interface with direct method for testing
type DynamicClient interface {
	PatchResource(ctx context.Context, gvr schema.GroupVersionResource, name string, patchType types.PatchType, data []byte) error
}

// EBPFClient interface for eBPF operations
type EBPFClient interface {
	GetBPFMapValue(pinPath string) (uint64, error)
}

// Dependencies struct holds all external dependencies
type Dependencies struct {
	KubeClient    KubeClient
	DynamicClient DynamicClient
	IPTablesV4    IPTablesClient
	IPTablesV6    IPTablesClient
	EBPFClient    EBPFClient
	FileReader    FileLineReader
}

// Config struct holds runtime configuration
type Config struct {
	ConfigPath4        string
	ConfigPath6        string
	CheckInterval      int
	SendEvents         bool
	IPv6Enabled        bool
	CheckMap           bool
	PinPath            string
	NodeName           string
	TerminateOnSuccess bool
}

// Implementation types that wrap real k8s clients

// realKubeClient wraps kubernetes.Interface to implement our KubeClient interface
type realKubeClient struct {
	client kubernetes.Interface
}

func NewKubeClient(client kubernetes.Interface) KubeClient {
	return &realKubeClient{client: client}
}

func (k *realKubeClient) GetNode(ctx context.Context, name string) (*corev1.Node, error) {
	return k.client.CoreV1().Nodes().Get(ctx, name, metav1.GetOptions{}) // nolint
}

func (k *realKubeClient) CreateEvent(ctx context.Context, namespace string, event *corev1.Event) (*corev1.Event, error) {
	return k.client.CoreV1().Events(namespace).Create(ctx, event, metav1.CreateOptions{}) // nolint
}

// realDynamicClient wraps dynamic.Interface
type realDynamicClient struct {
	client dynamic.Interface
}

func NewDynamicClient(client dynamic.Interface) DynamicClient {
	return &realDynamicClient{client: client}
}

func (d *realDynamicClient) PatchResource(ctx context.Context, gvr schema.GroupVersionResource, name string, patchType types.PatchType, data []byte) error {
	_, err := d.client.Resource(gvr).Patch(ctx, name, patchType, data, metav1.PatchOptions{})
	return err // nolint
}
