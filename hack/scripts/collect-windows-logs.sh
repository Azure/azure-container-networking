#!/bin/bash

echo "Ensure that privileged pod exists on each node"
kubectl apply -f ../../test/integration/manifests/load/privileged-daemonset-windows.yaml
kubectl rollout status ds -n kube-system privileged-daemonset

echo "------ Log work ------"
kubectl get pods -n kube-system -l os=windows,app=privileged-daemonset -owide
echo "Capture logs from each windows node. Files located in \k"
podList=`kubectl get pods -n kube-system -l os=windows,app=privileged-daemonset -owide --no-headers | awk '{print $1}'`
for pod in $podList; do
  files=`kubectl exec -i -n kube-system $pod -- powershell "ls ../../k/azure*.log*" | grep azure | awk '{print $6}'`
  node=`kubectl get pod -n kube-system $pod -o custom-columns=NODE:.spec.nodeName,NAME:.metadata.name --no-headers | awk '{print $1}'`
  mkdir -p ${acnLogs}/"$node"_logs/log-output/
  echo "Directory created: ${acnLogs}/"$node"_logs/log-output/"

  for file in $files; do
    kubectl exec -i -n kube-system $pod -- powershell "cat ../../k/$file" > ${acnLogs}/"$node"_logs/log-output/$file
    echo "Azure-*.log, $file, captured: ${acnLogs}/"$node"_logs/log-output/$file"
  done
  if [ ${cni} = 'cniv2' ]; then
    file="azure-cns.log"
    kubectl exec -i -n kube-system $pod -- cat k/azurecns/$file > ${acnLogs}/"$node"_logs/log-output/$file
    echo "CNS Log, $file, captured: ${acnLogs}/"$node"_logs/log-output/$file"
  fi
done

echo "------ Privileged work ------"
kubectl get pods -n kube-system -l os=windows,app=privileged-daemonset -owide
echo "Capture State Files from privileged pods"
for pod in $podList; do
  node=`kubectl get pod -n kube-system $pod -o custom-columns=NODE:.spec.nodeName,NAME:.metadata.name --no-headers | awk '{print $1}'`
  mkdir -p ${acnLogs}/"$node"_logs/privileged-output/
  echo "Directory created: ${acnLogs}/"$node"_logs/privileged-output/"

  file="azure-vnet.json"
  kubectl exec -i -n kube-system $pod -- powershell cat ../../k/$file > ${acnLogs}/"$node"_logs/privileged-output/$file
  echo "CNI State, $file, captured: ${acnLogs}/"$node"_logs/privileged-output/$file"
  if [ ${cni} = 'cniv1' ]; then
    file="azure-vnet-ipam.json"
    kubectl exec -i -n kube-system $pod -- powershell cat ../../k/$file > ${acnLogs}/"$node"_logs/privileged-output/$file
    echo "CNI IPAM, $file, captured: ${acnLogs}/"$node"_logs/privileged-output/$file"
  fi
done

if [ ${cni} = 'cniv2' ]; then
  echo "------ CNS work ------"


  kubectl get pods -n kube-system -l k8s-app=azure-cns-win --no-headers
  echo "Capture State Files from CNS pods"
  cnsPods=`kubectl get pods -n kube-system -l k8s-app=azure-cns-win --no-headers | awk '{print $1}'`
  for pod in $cnsPods; do
    managed=`kubectl exec -i -n kube-system pod -- powershell cat etc/azure-cns/cns_config.json | jq .ManageEndpointState`
    node=`kubectl get pod -n kube-system $pod -o custom-columns=NODE:.spec.nodeName,NAME:.metadata.name --no-headers | awk '{print $1}'`
    mkdir -p ${acnLogs}/"$node"_logs/CNS-output/
    echo "Directory created: ${acnLogs}/"$node"_logs/CNS-output/"

    file="cnsCache.txt"
    kubectl exec -i -n kube-system $pod -- powershell 'Invoke-WebRequest -Uri 127.0.0.1:10090/debug/ipaddresses -Method Post -ContentType application/x-www-form-urlencoded -Body "{`"IPConfigStateFilter`":[`"Assigned`"]}" -UseBasicParsing | Select-Object -Expand Content' > ${acnLogs}/"$node"_logs/CNS-output/$file
    echo "CNS cache, $file, captured: ${acnLogs}/"$node"_logs/CNS-output/$file"

    file="azure-cns.json"
    kubectl exec -i -n kube-system $pod -- powershell cat k/azurecns/azure-cns.json > ${acnLogs}/"$node"_logs/CNS-output/$file
    echo "CNS State, $file, captured: ${acnLogs}/"$node"_logs/CNS-output/$file"
    if [ $managed = "true" ]; then
      file="azure-endpoints.json"
      kubectl exec -i -n kube-system $pod -- cat k/azurecns/$file > ${acnLogs}/"$node"_logs/CNS-output/$file
      echo "CNS Managed State, $file, captured: ${acnLogs}/"$node"_logs/CNS-output/$file"
    fi
  done
fi