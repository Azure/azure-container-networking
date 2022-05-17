#!/bin/bash
argc=$#

if [[ $argc -ne 1 ]]
then 
	echo "cmd [1 (create pods), 2 (delete pods), 3 (list pod), 4 (list deployment)]"
	exit -1
fi

# Deploy pods on hollownodes managed by master cluster. 
export KUBECONFIG=$(pwd)/master.json

cmd=$1
case $cmd in
"1")
	echo "Deploy pods on hollow nodes of master K8s cluster"
	for index in {1..10}
	do
		kubectl apply -f deployments/deploy$index.yaml &
	done
	;;
"2")
	echo "Delete pods from hollow nodes of master K8s cluster"
	for index in {1..10}
	do
		kubectl delete -f deployments/deploy$index.yaml &
	done
	;;
"3")
	echo "list pods on master K8s cluster"
	kubectl get pods --all-namespaces -o wide
	;;	
"4")
	echo "list deployment on master K8s cluster"
	watch -n1 kubectl get deployment --all-namespaces -o wide
	;;	
*)
	echo "Unknown command :", $cmd
	;;
esac 
