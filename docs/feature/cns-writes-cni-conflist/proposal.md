# CNS Writes CNI Conflist Proposal

## Problem
Today, when a new Node joins an AKS cluster, its network setup is not factored into its readiness status. In other words, even if the CNI is not ready to give IPs to the Pods, the Node will be marked Ready and Pods will get assigned. Until certain network conditions are met, Pods will be stuck in a ContainerCreating backoff loop with CNI returning and error and a message like "failed to connect to 127.0.0.1:10900" or "Waiting for CNS to request more IPs".

This is problematic because we are delaying application startup - if an app gets scheduled to a new Node before the Node is truly ready, it will stay scheduled to the Node and only come up when the Node is finally ready. However, if the Node is correctly reporting that it is not ready, the scheduler would choose a different Node in the cluster to assign the Pod.

Further, this is problematic in the Azure CNI Overlay scenario, as the Node might start accepting pods or even kube-proxy Service traffic before the node's'overlay network routing is setup.

## Solution
A Node is considered NotReady if it does not have a CNI conflist. We can take advantage of that fact, and the fact that CNS knows when a Node's network has been correctly setup, by bringing up the Nodes with no CNI conflist file and allowing CNS to write the conflist file when the network is ready.

## Option 1 (Recommended) - CNS writes only the CNI conflist, the CNI dropgz installer writes only the CNI binary
In this solution, CNS is responsible for writing the CNI conflist and CNI dropgz installer, which runs as an init container in the CNS daemonset, only writes the CNI binary. Then, CNS writes the CNI conflist when the network has been setup.

### Pros
 - CNS makes decisions about CNI config without the dropgz installer having to know about all of the different CNI scenarios and config options
 - CNI dropgz installer is still versioned independently from CNS

### Cons
 - CNI and conflist are inherently related, so versioning between what CNS writes and what CNI understands is important
   - This is probably not a big deal because the CNS and CNI installer are deployed in lockstep together by nature of dropgz being an init container of CNS, so we'll always know which versions are getting bundled together. They can't rollout separately at different times like CNS and DNC, for example.

## Option 2 - CNS writes both CNI conflist and binary
In this solution, we would decom the CNI dropgz installer and instead install the CNI and conflist in CNS.

### Pros
 - CNS is managing everything (is this really a pro?)

### Cons
 - CNI dropgz installer is by-design intended to keep this logic separate from CNS
 - Changes the design of the dropgz installer which probably has more side-effects than I can enumerate

## Option 3 - CNI installer writes both CNI binary and conflist
In this solution, the CNI dropgz installer would be extended to either know when a network is ready, or know how to query CNS to find out. Then, it would write the CNI binary and conflist after.

### Pros
 - All filesystem logic is in dropgz

### Cons
 - Either we need to add a lot more logic to dropgz, or it can't be run as an init container to CNS if it takes a dependency on CNS running
 - Also schanges the design of the dropgz installer which probably has more side-effects than I can enumerate
