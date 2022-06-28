#!/bin/bash

argc=$#

if [[ $argc -ne 1 ]]
then 
	echo "cmd [1 (create hollownodes), 2 (clean-up hollownodes), 3 (list hollownodes info), 4 (delete label from hollow-node)]"
	exit -1
fi


MASTER_JSON=$(pwd)/master.json
EXTERNAL_JSON=$(pwd)/external.json

cmd=$1
case $cmd in
"1")
	echo "Deploy hollow nodes on external cluster"
	# 1. Deploy "hollow-node" pods on external K8s cluster
    kubectl apply -f ./hollow-node.yaml --kubeconfig=$EXTERNAL_JSON
	# 2. check whether all "hollow-node" pods are running (i.e., Running status) on external K8s cluster
    watch -n1 kubectl get pods -n kubemark --kubeconfig=$EXTERNAL_JSON
	# 3. check whether all "hollow-node" pods are registed as "hollow-node" nodes on master K8s cluster
    watch -n1 kubectl get nodes -l "hollow-node=" --kubeconfig=$MASTER_JSON
	;;
"2")
	echo "Delete hollow nodes on external cluster and clean-up them on kubemark cluster"
	# 1. Delete "hollow-node" pods on external K8s cluster
    kubectl delete -f ./hollow-node.yaml --kubeconfig=$EXTERNAL_JSON
	# 2. Delete "hollow-node" nodes from master K8s cluster
	# You may need to run below command multiple times since the node is removed when its status is "NotReady"
    kubectl get nodes --kubeconfig=$MASTER_JSON | grep NotReady | awk '{print $1}' |  xargs kubectl delete node --kubeconfig=$MASTER_JSON
	;;
"3")
	echo "Get hollow nodes information on kubemark cluster"
	kubectl get nodes -o wide --kubeconfig=$MASTER_JSON
	echo ""
	kubectl get nodes -o wide --show-labels --kubeconfig=$MASTER_JSON | grep "hollow-node"
	;;
"4")
	# Need to delete "kubernetes.io/os" label not to deploy add-on pods 
	# (e.g., azure-cni-networkmonitor-cfmgs, csi-secrets-store-cn9dp, kube-proxy-r9lpq, etc)
	# Something does not work.. Cannot delete "kubernetes.io/os" label on hollownode 
	nodes=$(kubectl get nodes -l hollow-node= --kubeconfig=$MASTER_JSON | grep hollow | awk '{print $1}')
	for node in ${nodes[@]}
	do
		echo $node
		kubectl label node $node kubernetes.io/os- --kubeconfig=$MASTER_JSON
	done
	kubectl get nodes -l hollow-node= --show-labels --kubeconfig=$MASTER_JSON
	;;
*)
	echo "Unknown command :", $cmd
	;;
esac 

