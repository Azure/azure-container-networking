## Overview
Scripts for scale testing our components with both real resources and fake resources via [KWOK](https://github.com/kubernetes-sigs/kwok).

Can specify number of Deployments, Pod replicas, NetworkPolicies, and labels for Pods.

### Why KWOK?
KWOK saves time/resources, especially in Windows.

## Usage
1. Create AKS cluster with `--uptime-sla` and create any nodepools.
2. To schedule real Pods on a node: `kubectl label node <name> scale-test=true`
3. Modify `scale-test.sh`: set KUBECONFIG_ARG if desired or leave empty.
4. Modify `scale-test.sh`: if not using NPM, set `USING_NPM=false`.
5. Modify `scale-test.sh`: update parameter values. Check your VMs' `--max-pod` capacity and set `maxRealPodsPerNode` accordingly (leave wiggle room for system Pods).
6. If making KWOK Pods, run: `./run-kwok.sh`
7. In another shell, run `./scale-test.sh`

Can also set `DEBUG_EXIT_AFTER_` variables in `scale-test.sh` to check configuration before actually running the scale tests.
