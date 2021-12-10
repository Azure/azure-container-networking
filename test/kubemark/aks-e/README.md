## Kubemark
1. Enable realistic large-scale performance test with small size of K8s cluster
2. Scripts are based on K8s created by `aks-engine`.
* It seems it does not work with `aks` since aks does not allow adding `hollownode` to a managed master.


## How to use kubemark for NPM scale performance test
1. Create two K8s clusters by using `aks-engine`
* `Master K8s cluster`: one master node and one agent node are enough. NPM will be deployed on `agent` node with `azure-npm-with-kubemark.yaml`
* `External K8s cluster`: `hollownodes` will be deployed as pods. You may decide how many physical node are deployed.

2. Change json filenames after creating both K8s clusters and locate this directory.
* Name `master.json` from `Master K8s cluster`, 
* Name `external.json` from  `External K8s cluster`

3. Run `configure-hollowNode-on-external-cluster.sh` which will set up necessary information on `External K8s cluster`.
* Basically it gives the master node information of `Master K8s cluster`. So, hollownodes on `External K8s cluster` can connect to the master node of `Master K8s cluster`

4. Deploy `hollownode` pods on `External K8s cluster` with hollow-node.yaml
* Use `hollownodes.sh` helper script.
* Configure `replica` spec in `hollow-node.yaml`. Current value is 10. So, max num of pods on 10 hollownodes are 10 * (max num of pods on one node).

5. Deploy `NPM` pod on agent node of `Master K8s cluster` with `azure-npm-with-kubemark.yaml`.
* `azure-npm-with-kubemark.yaml` is configured to only deploy one NPM pod on agent node of `Master K8s cluster`
* Use `npm.sh`.


6. After confirming all `hollownode` pods running on `External K8s cluster` are connected to `Master K8s cluster` as nodes, deploying pods for scaling test.
* Use pods.sh - it deploys deployment resources located in `deployments` directory.
* Configure `replica` spec in yaml files in `deployments` directory.


## To build kubemark container image
1. Use [kubemark-random-ip-addr-for-pod branch](https://github.com/JungukCho/kubernetes/tree/kubemark-random-ip-addr-for-pod)
* It has a patch to assign random IP address to pods.
2. Use `build-kubemark-docker.sh` script
```shell
cd cluster/images/kubemark
./build-kubemark-docker.sh
```


## Reference
1. [Performance and Scalability Testing Strategy Based on Kubemark](https://ieeexplore.ieee.org/document/8725658)
