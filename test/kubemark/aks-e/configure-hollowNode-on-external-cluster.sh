#!/bin/bash

MASTER_JSON=$(pwd)/master.json
EXTERNAL_JSON=$(pwd)/external.json

kubectl create namespace kubemark --kubeconfig=$EXTERNAL_JSON
kubectl create configmap node-configmap -n kubemark --from-literal=content.type="test-cluster" --kubeconfig=$EXTERNAL_JSON
kubectl create secret generic kubeconfig --type=Opaque --namespace=kubemark --from-file=kubelet.kubeconfig=$MASTER_JSON --from-file=kubeproxy.kubeconfig=$MASTER_JSON --kubeconfig=$EXTERNAL_JSON