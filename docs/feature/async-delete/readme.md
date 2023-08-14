## Asyc Delete

### Introduction

In AKS with Azure CNI, the Azure CNS service manages the CNI IPAM. The `azure-vnet` CNI plugin (and any CNI using delegated IPAM through `azure-ipam`) makes IP requests to the CNS API to request an IP during Pod creation or to release an IP during Pod deletion. The CNS API is a synchronous API, which means that the IP request is not completed until the IP is allocated or released in CNS internal IPAM state.

There is a deadlock scenario possible when the CNS API is not available (due to daemonset rollouts or for other reason):
If the Node is fully saturated with Pods (scheduled pods == maxPods), and a CNS Pod is not present (for example, a CNS daemonset rollout _deletes_ the existing Pod, then schedules the upgraded Pod), the scheduler will attempt to preempt a low priority Pod to make room for the CNS Pod. However, with no CNS Pod currently running, the CNI delete call will fail, and the Pod will be stuck in the `Terminating` state since the CRI cannot clean up the netns. The scheduler will not be able to schedule the CNS Pod, and the Node will deadlock without manual intervention to decrease the Pod pressure.

### Proposal

To address this deadlock issue, the CNI calls to CNS to release an IP address from a Pod need to be made asynchronously with a failsafe in such a way that if CNS is unavailable, it can recover these events when it does eventually start.

### Design

The CNI plugins (`azure-vnet`, `azure-ipam`) will be modified to treat a non-response from CNS during IP release as a non-fatal error and execution will proceed. A positive error response will still be treated as a real error and returned to the CRI for retry.

If the Pod IP release was not acknowledged by CNS, the CNI plugins will fall back to a file-system backed system to save these events. When the CNI does not get a response, it will write that Container ID to a "release queue" directory/file, and proceed with cleaning up the Pod netns.

When CNS starts, it will create a watch on the "release queue" directory/file, and process the Pod IDs in the queue. IPs for those Pods will then be released in CNS IPAM state. 

This will allow the CNI to recover from the CNS unavailability, unwedging the Pod deletion process, and allowing the scheduler to start the CNS Pod to get back to steady-state.
