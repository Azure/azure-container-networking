package main

import (
	"context"
	"fmt"

	"github.com/cilium/ebpf"
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
	return k.client.CoreV1().Nodes().Get(ctx, name, metav1.GetOptions{})
}

func (k *realKubeClient) CreateEvent(ctx context.Context, namespace string, event *corev1.Event) (*corev1.Event, error) {
	return k.client.CoreV1().Events(namespace).Create(ctx, event, metav1.CreateOptions{})
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
	return err
}

// realEBPFClient provides eBPF map operations
type realEBPFClient struct{}

func NewEBPFClient() EBPFClient {
	return &realEBPFClient{}
}

func (e *realEBPFClient) GetBPFMapValue(pinPath string) (uint64, error) {
	bpfMap, err := ebpf.LoadPinnedMap(pinPath, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to load pinned map %s: %w", pinPath, err)
	}
	defer bpfMap.Close()

	// 0 is the key for # of blocks
	key := uint32(0)
	value := uint64(0)

	if err := bpfMap.Lookup(&key, &value); err != nil {
		return 0, fmt.Errorf("failed to lookup key %d in bpf map: %w", key, err)
	}

	return value, nil
}
