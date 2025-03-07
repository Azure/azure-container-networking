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
	FinishBootupPhase()
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

type endpointCache struct {
	sync.Mutex
	cache map[string]*npmEndpoint
}

func newEndpointCache() *endpointCache {
	return &endpointCache{cache: make(map[string]*npmEndpoint)}
}

type applyInfo struct {
	sync.Mutex
	numBatches    int
	inBootupPhase bool
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

// updatePodCache's ordered queue implementation is similar to that of netPolQueue.
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

	if ok && pod.NodeName != m.NodeName {
		// Currently, don't expect this path to be taken because dataplane makes sure to only enqueue on-node Pods.
		// If the pod is already in the cache but the node name has changed, we need to requeue it.
		// Can discard the old Pod info since the Pod must have been deleted and brought back up on a different node.
		klog.Infof("[DataPlane] pod already in cache but node name has changed. deleting the old pod object from the queue. podKey: %s", m.PodKey)

		// remove the old pod from the cache and queue
		delete(c.cache, m.PodKey)
		i := 0
		for i = 0; i < len(c.queue); i++ {
			if c.queue[i] == m.PodKey {
				break
			}
		}

		if i < len(c.queue) {
			// this should always be true since we should always find the item in the queue
			c.queue = append(c.queue[:i], c.queue[i+1:]...)
		}

		ok = false
	}

	if !ok {
		// TODO: Refactor non-error/warning klogs with Zap and set the following logs to "debug" level
		// klog.Infof("[DataPlane] pod key %s not found in updatePodCache. creating a new obj", m.PodKey)

		pod = newUpdateNPMPod(m)
		c.cache[m.PodKey] = pod
		c.queue = append(c.queue, m.PodKey)
	}

	return pod
}

// dequeue returns the first pod in the queue and removes it from the queue.
func (c *updatePodCache) dequeue() *updateNPMPod {
	if c.isEmpty() {
		// TODO: Refactor non-error/warning klogs with Zap and set the following logs to "debug" level
		// klog.Infof("[DataPlane] updatePodCache is empty. returning nil for dequeue()")
		return nil
	}

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
		// TODO: Refactor non-error/warning klogs with Zap and set the following logs to "debug" level
		// klog.Infof("[DataPlane] pod key %s already exists in updatePodCache. skipping requeue", pod.PodKey)
		return
	}

	c.cache[pod.PodKey] = pod
	c.queue = append(c.queue, pod.PodKey)
}

// isEmpty returns true if the queue is empty
func (c *updatePodCache) isEmpty() bool {
	return len(c.queue) == 0
}

// netPolQueue contains NetPols to add. Currently implemented as an unordered map
type netPolQueue struct {
	sync.Mutex
	toAdd map[string]*policies.NPMNetworkPolicy
}

func newNetPolQueue() *netPolQueue {
	return &netPolQueue{
		toAdd: make(map[string]*policies.NPMNetworkPolicy),
	}
}

func (q *netPolQueue) len() int {
	return len(q.toAdd)
}

// enqueue adds a NetPol to the queue. If the NetPol already exists in the queue, the NetPol object is updated.
func (q *netPolQueue) enqueue(policy *policies.NPMNetworkPolicy) {
	// TODO: Refactor non-error/warning klogs with Zap and set the following logs to "debug" level
	// if _, ok := q.toAdd[policy.PolicyKey]; ok {
	// 	klog.Infof("[DataPlane] policy %s exists in netPolQueue. updating", policy.PolicyKey)
	// } else {
	// 	klog.Infof("[DataPlane] enqueuing policy %s in netPolQueue", policy.PolicyKey)
	// }
	q.toAdd[policy.PolicyKey] = policy
}

// delete removes the NetPol from the queue
func (q *netPolQueue) delete(policyKey string) {
	delete(q.toAdd, policyKey)
}

// dump returns a copy of the queue
func (q *netPolQueue) dump() []*policies.NPMNetworkPolicy {
	result := make([]*policies.NPMNetworkPolicy, 0, len(q.toAdd))
	for _, policy := range q.toAdd {
		result = append(result, policy)
	}
	return result
}

func (q *netPolQueue) clear() {
	q.toAdd = make(map[string]*policies.NPMNetworkPolicy)
}
