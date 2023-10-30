## CNS watches Pods to drive IPAM scaling

### Current state
The IPAM Pool Scaling is reactive: CNS assigns IPs out of the IPAM Pool as it is asked for them by the CNI, while trying to maintain a buffer of Free IPs that is within the Scaler parameters. The CNI makes IP assignment requests serially, and as it requests that IPs are assigned or freed, CNS makes requests to scale up or down the IPAM Pool by adjusting the Requested IP Count in the NodeNetworkConfig. If CNS is unable to honor an IP assignment requests due to no free IPs, CNI returns an error to the CRI which causes the Pod sandbox to be cleaned up, and CNS will receive an IP Release request for that Pod.

In this reactive architecture, CNS is unable to track the number of incoming Pod IP assignment requests or predict how many IPs it may soon need. CNS can only reliably scale by a single Batch at a time when it runs out of Free IPs. For example:

- At $T_3$ CNI requests an IP for Pod $P_{16}$ but CNS is out of free IPs and returns an error
  - CRI tears down $P_{16}$, and CNI requests that CNS frees the IP for $P_{16}$
  - $P_{17-36}$ are similarly stuck, pending available IPs
- At $T_4$ CNS receives an additional Batch of IPs, and as the CNI retries they are assigned to $P_{16-31}$
- At $T_5$ CNS has too few unassigned IPs again and requests another Batch
- At $T_6$ $P_{32}$ is stuck, pending available IPs
- At $T_7$ CNS receives an additional Batch of IPs, and as the CNI retries they are assigned to $P_{32-35}$


| Time   | State                                                                 |
| ---- | -------------------------------------------------------------------- |
| $T_0$  | 1 Pod $P_0$ is scheduled: CNS has 1 Batch ( $16$ ) IPs                |
| $T_1$  | 35 Pods are scheduled for a Total of 36 Pods                         |
| $T_2$  | CNI is sequentially requesting IP assignments for Pods, and for Pod $P_8$, CNS has less than $B\times mf$ unassigned IPs and requests an additional Batch of IPs |
| $T_3$  | CNI requests an IP for Pod $P_{16}$ but CNS is out of free IPs and returns an error |
| $T_3+$ | - CRI tears down $P_{16}$, and CNI requests that CNS frees the IP for $P_{16}$ <br> - $P_{17-36}$ are similarly stuck, pending available IPs |
| $T_4$  | CNS receives an additional Batch of IPs, and as the CNI retries they are assigned to $P_{16-31}$ |
| $T_5$  | CNS has too few unassigned IPs again and requests another Batch        |
| $T_6$  | $P_{32}$ is stuck, pending available IPs                              |
| $T_7$  | CNS receives an additional Batch of IPs, and as the CNI retries they are assigned to $P_{32-35}$ |
| ... | ...|

By proactively watching Pods instead of waiting for the CNI requests, this process could be faster and simpler:

| Time   | State |
| ---- | -------------------------------------------------------------------- |
| $T_0$  | 1 Pod $P_0$ is scheduled: CNS has 1 Batch ( $16$ ) IPs        |
| $T_1$  | 35 Pods are scheduled for a Total of 36 Pods                 |
| $T_2$  | CNS sees 36 Pods have been scheduled and updates the Requested IP Count to $48$ according to the [Scaling Equation](1-ipam-math.md#scaling-math) |
| $T_3$  | CNS receives 48 total IPs, and as the CNI requests IP assignments they are assigned to $P_{1-35}$ |




### Performance Considerations

Migrating CNS IPAM from a reactive to a proactive architecture is a significant change to the CNS <-> Kubernetes interaactions which has the potential to increase the load on the API Server. However, this use-case  is a common one - the Kubelet, notably, also watches Pods on each Node, and it is highly optimized path.

By leverageing similar patterns and some Kubernetes provided machinery, we can make this change efficiently. 

#### SharedInformers and local Caches
Kubernetes `client-go` [provides machinery for local caching](https://github.com/kubernetes/sample-controller/blob/6d1d76794eb5f951e63a46f1ad6e097c1879d81b/docs/controller-client-go.md): Reflectors, (Shared)Informers, Indexer, and Stores

<p align="center">
  <img src="https://raw.githubusercontent.com/kubernetes/sample-controller/6d1d76794eb5f951e63a46f1ad6e097c1879d81b/docs/images/client-go-controller-interaction.jpeg" height="600" width="700"/>

  > [Image from kubernetes/sample-controller documentation](https://github.com/kubernetes/sample-controller/blob/6d1d76794eb5f951e63a46f1ad6e097c1879d81b/docs/controller-client-go.md).
</p>

By leveraging this machinery, CNS will set up a `Watch` on Pods which will open a single long-lived socket connection to the API Server and will let the API Server push incremental updates. This significantly decreases the data transferred and API Server load when compared to naively polling `List` to get Pods repeatedly.

Additionally, any read-only requests (`Get`, `List`, `Watch`) that CNS makes to Kubernetes using a cache-aware client will hit the local Cache instead of querying the remote API Server. This means that the only requests leaving CNS to the API Server for this Pod Watcher will be the Reflector's List and Watch.

#### Server-side filtering
To reduce API Server load and traffic, CNS can use an available [Field Selector](https://kubernetes.io/docs/concepts/overview/working-with-objects/field-selectors/) for Pods: [`spec.nodeName=<node>`](https://github.com/kubernetes/kubernetes/blob/691d4c3989f18e0be22c4499d22eff95d516d32b/pkg/apis/core/v1/conversion.go#L40). Field selectors are, like Label Selectors, [applied on the server-side](https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/#list-and-watch-filtering) to List and Watch queries to reduce the dataset that is returned from the API Server to the Client. 

By restricting the Watch to Pods on the current Node, the traffic generated by the Watch will be proportional to the number of Pods on that Node, and will *not* scale in relation to either the number of Nodes in the cluster or the total number of Pods in the cluster.

### Implementation
To make setting up the filters, SharedInformers, and cache-aware client easy, we will use [`controller-runtime`](https://github.com/kubernetes-sigs/controller-runtime) and create a Pod Reconciler. A controller already exists for managing the `NodeNetworkConfig` CRD lifecycle, so the necessary infrastructure (namely, a Manager) already exists in CNS.

To create a filtered Cache during the Manager instantiation, the existing `nodeScopedCache` will be expanded to include Pods:

```go
import (
  v1 "k8s.io/api/core/v1"
  "k8s.io/apimachinery/pkg/fields"
  "sigs.k8s.io/controller-runtime/pkg/cache"
  //...
)
//...
nodeName := "the-node-name"
// the nodeScopedCache sets Selector options on the Manager cache which are used
// to perform *server-side* filtering of the cached objects. This is very important
// for high node/pod count clusters, as it keeps us from watching objects at the
// whole cluster scope when we are only interested in our Node's scope.
nodeScopedCache := cache.BuilderWithOptions(cache.Options{
  SelectorsByObject: cache.SelectorsByObject{
    // existing options
    //...,
    &v1.Pod{}: {
      Field: fields.SelectorFromSet(fields.Set{"spec.nodeName": nodeName}),
    },
  },
})
//...
manager, err := ctrl.NewManager(kubeConfig, ctrl.Options{
    // existing options
    //...,
    NewCache:           nodeScopedCache,
})
```

After the local Cache and ListWatch has been set up correctly, the Reconciler should use the Manager-provided Kubernetes API Client within its event loop so that reads hit the cache instead of the real API.

```go
import (
  "context"

  v1 "k8s.io/api/core/v1"
  ctrl "sigs.k8s.io/controller-runtime"
  "sigs.k8s.io/controller-runtime/pkg/client"
  "sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type Reconciler struct {
  client client.Client
}

func (r *Reconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
  pods := v1.PodList{}
  r.client.List(ctx, &pods)
  // do things with the list of pods
  // ...
  return reconcile.Result{}, nil
}

func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
  r.client = mgr.GetClient()
  return ctrl.NewControllerManagedBy(mgr).
    For(&v1.Pod{}).
    Complete(r)
}
```

This can be further optimized by ignoring "Status" Updates to any Pods in the controller setup func:
```go
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
  r.client = mgr.GetClient()
  return ctrl.NewControllerManagedBy(mgr).
    For(&v1.Pod{}).
    WithEventFilter(predicate.Funcs{
      // check that the generation has changed - status changes don't update generation.
      UpdateFunc: func(ue event.UpdateEvent) bool {
        return ue.ObjectOld.GetGeneration() != ue.ObjectNew.GetGeneration()
      },
    }).
    Complete(r)
}
```
Note: 
The CNS RBAC will need to be updated to include permission to access Pods:
```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: pod-ro
  namespace: kube-system
rules:
- apiGroups: 
  - ""
  verbs: 
  - get
  - list
  - watch
  resources: 
  - pods
```

### The updated IPAM Pool Monitor

When CNS is watching Pods via the above mechanism, the number of Pods scheduled on the Node (after discarding `hostNetwork: true` Pods), is the instantaneous IP demand for the Node. This IP demand can be fed in to the IPAM Pool scaler in place of the "Used" quantity described in the [idempotent Pool Scaling equation](1-ipam-math.md#scaling-math):

$$
Request = B \times \lceil mf + \frac{Demand}{B} \rceil
$$

to immediately calculate the target Requested IP Count for the current actual Pod load. At this point, CNS can scale directly to the neccesary number of IPs in a single operation proactively, as soon as Pods are scheduled on the Node, without waiting for the CNI to request IPs serially.
