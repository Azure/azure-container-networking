#!/bin/bash

echo "Ensure that privileged pod exists on each node"
kubectl apply -f ../../test/integration/manifests/load/privileged-daemonset.yaml
kubectl rollout status ds -n kube-system privileged-daemonset

echo "------ Log work ------"
kubectl get pods -n kube-system -l os=linux,app=privileged-daemonset -owide
echo "Capture logs from each linux node. Files located in var/logs/*."
podList=`kubectl get pods -n kube-system -l os=linux,app=privileged-daemonset -owide --no-headers | awk '{print $1}'`
for pod in $podList; do
  index=0
  files=(`kubectl exec -i -n kube-system $pod -- find ./var/log -maxdepth 2 -name "azure-*" -type f`)
  fileBase=(`kubectl exec -i -n kube-system $pod -- find ./var/log -maxdepth 2 -name "azure-*" -type f -printf "%f\n"`)

  node=`kubectl get pod -n kube-system $pod -o custom-columns=NODE:.spec.nodeName,NAME:.metadata.name --no-headers | awk '{print $1}'`
  mkdir -p ${acnLogs}/"$node"_logs/log-output/
  echo "Directory created: ${acnLogs}/"$node"_logs/"

  for file in ${files[*]}; do
    kubectl exec -i -n kube-system $pod -- cat $file > ${acnLogs}/"$node"_logs/log-output/${fileBase[$index]}
    echo "Azure-*.log, ${fileBase[$index]}, captured: ${acnLogs}/"$node"_logs/log-output/${fileBase[$index]}"
    ((index++))
  done
  if [ ${cni} = 'cilium' ]; then
    file="cilium-cni.log"
    kubectl exec -i -n kube-system $pod -- cat var/log/$file > ${acnLogs}/"$node"_logs/log-output/$file
    echo "Cilium log, $file, captured: ${acnLogs}/"$node"_logs/log-output/$file"
  fi
done

if ! [ ${cni} = 'cilium' ]; then
  echo "------ Privileged work ------"
  kubectl get pods -n kube-system -l os=linux,app=privileged-daemonset -owide
  echo "Capture State Files from privileged pods"
  for pod in $podList; do
    node=`kubectl get pod -n kube-system $pod -o custom-columns=NODE:.spec.nodeName,NAME:.metadata.name --no-headers | awk '{print $1}'`
    mkdir -p ${acnLogs}/"$node"_logs/privileged-output/
    echo "Directory created: ${acnLogs}/"$node"_logs/privileged-output/"

    file="azure-vnet.json"
    kubectl exec -i -n kube-system $pod -- cat /var/run/$file > ${acnLogs}/"$node"_logs/privileged-output/$file
    echo "CNI State, $file, captured: ${acnLogs}/"$node"_logs/privileged-output/$file"
    if [ ${cni} = 'cniv1' ]; then
      file="azure-vnet-ipam.json"
      kubectl exec -i -n kube-system $pod -- cat /var/run/$file > ${acnLogs}/"$node"_logs/privileged-output/$file
      echo "CNIv1 IPAM, $file, captured: ${acnLogs}/"$node"_logs/privileged-output/$file"
    fi
  done
fi

if [ ${cni} = 'cilium' ] || [ ${cni} = 'cniv2' ]; then
  echo "------ CNS work ------"


  kubectl get pods -n kube-system -l k8s-app=azure-cns
  echo "Capture State Files from CNS pods"
  cnsPods=`kubectl get pods -n kube-system -l k8s-app=azure-cns --no-headers | awk '{print $1}'`
  for pod in $cnsPods; do
    managed=`kubectl exec -i -n kube-system $pod -- cat etc/azure-cns/cns_config.json | jq .ManageEndpointState`
    node=`kubectl get pod -n kube-system $pod -o custom-columns=NODE:.spec.nodeName,NAME:.metadata.name --no-headers | awk '{print $1}'`
    mkdir -p ${acnLogs}/"$node"_logs/CNS-output/
    echo "Directory created: ${acnLogs}/"$node"_logs/CNS-output/"

    file="cnsCache.txt"
    kubectl exec -i -n kube-system $pod -- curl localhost:10090/debug/ipaddresses -d {\"IPConfigStateFilter\":[\"Assigned\"]} > ${acnLogs}/"$node"_logs/CNS-output/$file
    echo "CNS cache, $file, captured: ${acnLogs}/"$node"_logs/CNS-output/$file"

    file="azure-cns.json"
    kubectl exec -i -n kube-system $pod -- cat /var/lib/azure-network/$file > ${acnLogs}/"$node"_logs/CNS-output/$file
    echo "CNS State, $file, captured: ${acnLogs}/"$node"_logs/CNS-output/$file"
    if [ $managed = "true" ]; then
      file="azure-endpoints.json"
      kubectl exec -i -n kube-system $pod -- cat /var/run/azure-cns/$file > ${acnLogs}/"$node"_logs/CNS-output/$file
      echo "CNS Managed State, $file, captured: ${acnLogs}/"$node"_logs/CNS-output/$file"
    fi
  done
fi

if [ ${cni} = 'cilium' ]; then
  echo "------ Cilium work ------"
  kubectl get pods -n kube-system -l k8s-app=cilium
  echo "Capture State Files from Cilium pods"
  ciliumPods=`kubectl get pods -n kube-system -l k8s-app=cilium --no-headers | awk '{print $1}'`
  for pod in $ciliumPods; do
    node=`kubectl get pod -n kube-system $pod -o custom-columns=NODE:.spec.nodeName,NAME:.metadata.name --no-headers | awk '{print $1}'`
    mkdir -p ${acnLogs}/"$node"_logs/Cilium-output/
    echo "Directory created: ${acnLogs}/"$node"_logs/Cilium-output/"

    file="cilium-endpoint.json"
    kubectl exec -i -n kube-system $pod -- cilium endpoint list -o json > ${acnLogs}/"$node"_logs/Cilium-output/$file
    echo "Cilium, $file, captured: ${acnLogs}/"$node"_logs/Cilium-output/$file"
  done
fi
