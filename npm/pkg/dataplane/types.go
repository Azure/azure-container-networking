package dataplane

import (
	"strings"
	"sync"

	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/ipsets"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/policies"
	"github.com/Azure/azure-container-networking/npm/util"
	"k8s.io/klog"
)

type GenericDataplane interface {
	BootupDataplane() error
	RunPeriodicTasks()
	GetAllIPSets() map[string]string
	GetIPSet(setName string) *ipsets.IPSet
	CreateIPSets(setMetadatas []*ipsets.IPSetMetadata)
	DeleteIPSet(setMetadata *ipsets.IPSetMetadata, deleteOption util.DeleteOption)
	AddToSets(setMetadatas []*ipsets.IPSetMetadata, podMetadata *PodMetadata) error
	RemoveFromSets(setMetadatas []*ipsets.IPSetMetadata, podMetadata *PodMetadata) error
	AddToLists(listMetadatas []*ipsets.IPSetMetadata, setMetadatas []*ipsets.IPSetMetadata) error
	RemoveFromList(listMetadata *ipsets.IPSetMetadata, setMetadatas []*ipsets.IPSetMetadata) error
	ApplyDataPlane() error
	// GetAllPolicies is deprecated and only used in the goalstateprocessor, which is deprecated
	GetAllPolicies() []string
	AddPolicy(policies *policies.NPMNetworkPolicy) error
	RemovePolicy(PolicyKey string) error
	UpdatePolicy(policies *policies.NPMNetworkPolicy) error
}

// UpdateNPMPod pod controller will populate and send this datastructure to dataplane
// to update the dataplane with the latest pod information
// this helps in calculating if any update needs to have policies applied or removed
type updateNPMPod struct {
	*PodMetadata
	IPSetsToAdd    []string
	IPSetsToRemove []string
}

// PodMetadata is what is passed to dataplane to specify pod ipset
// todo definitely requires further optimization between the intersection
// of types, PodMetadata, NpmPod and corev1.pod
type PodMetadata struct {
	PodKey   string
	PodIP    string
	NodeName string
}

func NewPodMetadata(podKey, podIP, nodeName string) *PodMetadata {
	return &PodMetadata{
		PodKey:   podKey,
		PodIP:    podIP,
		NodeName: nodeName,
	}
}

func (p *PodMetadata) Namespace() string {
	return strings.Split(p.PodKey, "/")[0]
}

func newUpdateNPMPod(podMetadata *PodMetadata) *updateNPMPod {
	return &updateNPMPod{
		PodMetadata:    podMetadata,
		IPSetsToAdd:    make([]string, 0),
		IPSetsToRemove: make([]string, 0),
	}
}

func (npmPod *updateNPMPod) updateIPSetsToAdd(setNames []*ipsets.IPSetMetadata) {
	for _, set := range setNames {
		npmPod.IPSetsToAdd = append(npmPod.IPSetsToAdd, set.GetPrefixName())
	}
}

func (npmPod *updateNPMPod) updateIPSetsToRemove(setNames []*ipsets.IPSetMetadata) {
	for _, set := range setNames {
		npmPod.IPSetsToRemove = append(npmPod.IPSetsToRemove, set.GetPrefixName())
	}
}

type updatePodCache struct {
	sync.Mutex
	cache map[string]*updateNPMPod
	// queue maintains the FIFO queue of the pods in the cache.
	// This makes sure we handle issue 1729 with the same queue as the control plane.
	// It also lets us update Pod ACLs in the same queue as the control plane so that
	// e.g. the first Pod created is the first Pod to have proper connectivity.
	queue           []string
	initialCapacity int
}

// newUpdatePodCache creates a new updatePodCache with the given initial capacity for the queue
func newUpdatePodCache(initialCapacity int) *updatePodCache {
	return &updatePodCache{
		cache:           make(map[string]*updateNPMPod),
		queue:           make([]string, 0, initialCapacity),
		initialCapacity: initialCapacity,
	}
}

// enqueue adds a pod to the queue if necessary and returns the pod object used
func (c *updatePodCache) enqueue(m *PodMetadata) *updateNPMPod {
	pod, ok := c.cache[m.PodKey]
	if !ok {
		klog.Infof("[DataPlane] pod key %s not found in updatePodCache. creating a new obj", m.PodKey)

		pod = newUpdateNPMPod(m)
		c.cache[m.PodKey] = pod
		c.queue = append(c.queue, m.PodKey)
	}

	return pod
}

// dequeue returns the first pod in the queue and removes it from the queue.
// Caller should ensure the queue is not empty.
// Otherwise, the following will occur: "panic: runtime error: index out of range [0] with length 0"
func (c *updatePodCache) dequeue() *updateNPMPod {
	pod := c.cache[c.queue[0]]
	c.queue = c.queue[1:]
	delete(c.cache, pod.PodKey)

	if c.isEmpty() {
		// reset the slice to make sure the underlying array is garbage collected (not sure if this is necessary)
		c.queue = make([]string, 0, c.initialCapacity)
	}

	return pod
}

// requeue adds the pod to the end of the queue
func (c *updatePodCache) requeue(pod *updateNPMPod) {
	if _, ok := c.cache[pod.PodKey]; ok {
		// should not happen
		klog.Infof("[DataPlane] pod key %s already exists in updatePodCache. skipping requeue", pod.PodKey)
		return
	}

	c.cache[pod.PodKey] = pod
	c.queue = append(c.queue, pod.PodKey)
}

// isEmpty returns true if the queue is empty
func (c *updatePodCache) isEmpty() bool {
	return len(c.queue) == 0
}
