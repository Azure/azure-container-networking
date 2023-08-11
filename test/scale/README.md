## Overview
Scripts for scale testing our components in AKS with fake and/or real resources.

### Fake Resources
Scripts can use [KWOK](https://github.com/kubernetes-sigs/kwok) to simulate running Pods. KWOK can instantly run thousands of fake VMs and Pods.

This saves us from:
1. Large resource costs.
2. Hours waiting for VMs and Pods to bootup.

## Usage
1. Create AKS cluster with `--uptime-sla` and create any nodepools.
2. If making KWOK Pods, run `run-kwok.sh` in the background.
3. Scale with `test-scale.sh`. Specify number of Deployments, Pod replicas, NetworkPolicies, and labels for Pods. Can also delete/re-add objects to cause churn.
4. Test connectivity with `connectivity/test-connectivity.sh`.

### Example Runs
```
./test-scale.sh --max-kwok-pods-per-node=50 \
    --num-kwok-deployments=10 \
    --num-kwok-replicas=1 \
    --max-real-pods-per-node=30 \
    --num-real-deployments=5 \
    --num-real-services=4 \
    --num-real-replicas=2 \
    --num-network-policies=1 \
    --num-unapplied-network-policies=10 \
    --num-unique-labels-per-pod=2 \
    --num-unique-labels-per-deployment=2 \
    --num-shared-labels-per-pod=10 \
    --delete-labels \
    --delete-labels-interval=30 \
    --delete-labels-times=2 \
    --delete-netpols \
    --delete-netpols-interval=0 \
    --delete-netpols-times=1 \
    --delete-kwok-pods=10 \
    --delete-real-pods=5 \
    --delete-pods-interval=120 \
    --delete-pods-times=2
```

Note: you must run `./test-scale.sh` first with `--num-network-policies=1` or more, and `--num-shared-labels-per-pod=3` or more.
```
./test-connectivity.sh --num-scale-pods-to-verify=all \
    --max-wait-for-initial-connectivity=600 \
    --max-wait-after-adding-netpol=120
```
