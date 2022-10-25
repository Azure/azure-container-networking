## Revising the NodeNetworkConfig to v1beta1 [[Phase 3 Design]](../proposal.md#3-2-revise-the-nnc-to-v1beta1)

As some responsibility is shifted out of the NodeNetworkConfig (the Scaler), and the use-cases evolve, the NodeNetworkConfig needs to be updated to remain adaptable to all scenarios. Notably, to support multiple NetworkContainers per Node, the NNC should acknowledge that those may be from separate Subnets, and should map the `requestedIPCount` per known NetworkContainer. With the ClusterSubnet CRD hosting the Subnet Scaler properties, this will allow Subnets to scale independently even when they are used on the same Node.

Since this is a significant breaking change, the NodeNetworkConfig definition must be incremented. Since the spec is being incremented, some additional improvements are included.

```diff
-   apiVersion: acn.azure.com/v1alpha
+   apiVersion: acn.azure.com/v1beta1
    kind: NodeNetworkConfig
    metadata:
        name: nodename
        namespace: kube-system
    spec:
-       requestedIPCount: 16
-       ipsNotInUse:
+       releasedIPs:
        -   abc-ip-123-guid
+       secondaryIPs:
+           abc-nc-123-guid: 16
    status:
-       assignedIPCount: 1
        networkContainers:
        -   assignmentMode: dynamic
            defaultGateway: 10.241.0.1
            id: abc-nc-123-guid
-           ipAssignments:
-           -   ip: 10.241.0.2 
-               name: abc-ip-123-guid
            nodeIP: 10.240.0.5
            primaryIP: 10.241.0.38
            resourceGroupID: rg-id
+           secondaryIPCount: 1
+           secondaryIPs:
+           -   address: 10.241.0.2 
+               id: abc-ip-123-guid
            subcriptionID: abc-sub-123-guid
            subnetAddressSpace: 10.241.0.0/16
-           subnetID: podnet
            subnetName: podnet
            type: vnet
            version: 49
            vnetID: vnet-id
-       scaler:
-           batchSize: 16
-           maxIPCount: 250
-           releaseThresholdPercent: 150
-           requestThresholdPercent: 50
        status: Updating
```

In order:
- the GV is incremented to `acn.azure.com/v1beta1`
- the `spec.requestedIPCount` key is renamed to `spec.secondaryIPs`
    - the value is change from a single scalar to a map of `NC ID` to scalar values
- the `spec.ipsNotInUse` key is renamed to `spec.releasedIPs`
- the `status.assignedIPCount` field is moved and renamed to `status.networkContainers[].secondaryIPCount`
- the `status.networkContainers[].ipAssignments` field is renamed to `status.networkContainers[].secondaryIPs`
    - the keys of the secondaryIPs are renamed from `ip` and `name` to `address` and `id` respectively
- the `status.subnetID` fields is removed as a duplicate of `status.subnetName`, where both were actually the "name" and not a unique ID.
- the `status.scaler` is removed entirely

#### Migration
This update does not _add_ information to the NodeNetworkConfig, but removes and renames some properties. The transition will take place as follows:
1) The `v1beta1` CRD revision is created
    - conversion functions are added to the NodeNetworkConfig schema which translate `v1beta1` <-> `v1alpha` (via `v1beta1` as the hub and "Storage Version").
2) DNC-RC installs the new CRD definition and registers a conversion webhook.
3) CNS switches to `v1beta1`.

At this time, any mutation of existing NNCs will automatically up-convert them to the `v1beta1` definition. Any client still requesting `v1alpha` will still be served a down-converted representation of the NNC in a backwards-compatible fashion, and updates to that NNC will be stored in the `v1beta1` representation.
