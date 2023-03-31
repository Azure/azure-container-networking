## Overview
Scripts for scale testing our components with both real resources and fake resources via [KWOK](https://github.com/kubernetes-sigs/kwok).

### Why KWOK?
KWOK saves time/resources, especially in Windows.

## Usage
1. Create AKS cluster with `--uptime-sla` and create any nodepools.
2. If making KWOK Pods, run `run-kwok.sh` in the background.
3. Scale with `test-scale.sh`. Specify number of Deployments, Pod replicas, NetworkPolicies, and labels for Pods.
4. Test connectivity with `connectivity/test-connectivity.sh`.
