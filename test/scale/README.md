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
3. Scale with `test-scale.sh`. Specify number of Deployments, Pod replicas, NetworkPolicies, and labels for Pods.
4. Test connectivity with `connectivity/test-connectivity.sh`.
