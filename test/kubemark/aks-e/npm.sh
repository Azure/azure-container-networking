#!/bin/bash
argc=$#

if [[ $argc -ne 1 ]]
then 
	echo "cmd [1 (deploy npm on agent), 2 (delete npm), 3 (print npm log), 4 (connect to npm console), 5 [top npm]]"
	exit -1
fi

export KUBECONFIG=$(pwd)"/master.json"

cmd=$1
case $cmd in
"1")
	echo "Deploy NPM pod on agent of master K8s cluster"
	kubectl apply -f azure-npm-with-kubemark.yaml 
	;;	
"2")
	echo "Deploy NPM pod on agent of master K8s cluster"
	kubectl delete -f azure-npm-with-kubemark.yaml
	;;	
"3")
	NPM_POD=$(kubectl get pods -l k8s-app=azure-npm -n kube-system -o wide --kubeconfig=$MASTER_JSON | grep agentpool | awk '{print $1}')
	kubectl logs -f $NPM_POD -n kube-system 
	;;	
"4")
	NPM_POD=$(kubectl get pods -l k8s-app=azure-npm -n kube-system -o wide --kubeconfig=$MASTER_JSON | grep agentpool | awk '{print $1}')
	kubectl exec -it $NPM_POD -n kube-system bash
	;;
"5") 
	watch -n1 kubectl top  pod -l k8s-app=azure-npm -n kube-system 
	;;
*)
	echo "Unknown command :", $cmd
	;;
esac 
