## Migrating the Scaler properties to the ClusterSubnet CRD [[Phase 3 Design]](../proposal.md#2-3-scaler-properties-move-to-the-clustersubnet-crd)
Currently, the [`v1alpha/NodeNetworkConfig` contains the Scaler inputs](https://github.com/Azure/azure-container-networking/blob/eae2389f888468e3b863cb28045ba613a5562360/crd/nodenetworkconfig/api/v1alpha/nodenetworkconfig.go#L66-L72) which CNS will use to scale the local IPAM pool:

```yaml
...
status:
    scaler:
        batchSize: X
        releaseThresholdPercent: X
        requestThresholdPercent: X
        maxIPCount: X
```
Since the Scaler values are dependent on the state of the Subnet, the Scaler object will be moved to the ClusterSubnet CRD and optimized. 

### ClusterSubnet Scaler
The ClusterSubnet `Status.Scaler` definition will be: 
```yaml
...
status:
    scaler:
        batch: X // equal to batchSize
        buffer: X // equal to requestThresholdPercent
```

Additionally, the `Spec` of the ClusterSubnet will accept `Scaler` values to be used as runtime overrides. DNC-RC will read and validate the `Spec`, then write the values back out to the `Status` if present.
```yaml
...
spec:
    scaler:
        <...>
```



Note: 
- The `scaler.maxIPCount` will not be migrated, as the maxIPCount is a property of the Node and not the Subnet.
- The `scaler.releaseThresholdPercent` will not be migrated, as it is redundant. The `buffer` (and in fact the `requestThresholdPercent`), imply a `releaseThresholdPercent` and one does not need to be specified explicitly. The [IPAM Scaling Math](../phase-2/2-scalingmath.md) incorporates only a single threshold value and fully describes the behavior of the system.

#### Migration
When the Scaler is added to the ClusterSubnet CRD definiton, DNC-RC will begin replicating the `batch` and `buffer` properties from the NodeNetworkConfig, keeping both up to date.

CNS, which already watches the ClusterSubnet CRD for known Subnets, will use the Scaler properties from that object as a priority, and will fall back to using the NNC Scaler properties if they are not present in the ClusterSubnet.
