#!/bin/bash
# Usage: acnLogs="./win-logs/" cni="cniv2" bash collect-windows-logs.sh
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
    echo "Capture all CNS logs from k/azurecns"
    cnsLogFiles=`kubectl exec -i -n kube-system $pod -- powershell "Get-ChildItem ../../k/azurecns/*.log* -Name" 2>/dev/null | tr -d '\r'`
    if [ -z "$cnsLogFiles" ]; then
      echo "No CNS logs found in k/azurecns for $pod"
    fi
    for cnsFile in $cnsLogFiles; do
      kubectl exec -i -n kube-system $pod -- powershell "cat ../../k/azurecns/$cnsFile" > ${acnLogs}/"$node"_logs/log-output/$cnsFile
      echo "CNS Log, $cnsFile, captured: ${acnLogs}/"$node"_logs/log-output/$cnsFile"
    done
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

  file="windowsnodereset.log"
  kubectl exec -i -n kube-system $pod -- powershell cat ../../k/$file > ${acnLogs}/"$node"_logs/privileged-output/$file
  echo "Node Reset Log, $file, captured: ${acnLogs}/"$node"_logs/privileged-output/$file"

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
  managed=`kubectl get cm cns-win-config -n kube-system -o jsonpath='{.data.cns_config\.json}' | jq .ManageEndpointState`
  for pod in $podList; do
    node=`kubectl get pod -n kube-system $pod -o custom-columns=NODE:.spec.nodeName,NAME:.metadata.name --no-headers | awk '{print $1}'`
    mkdir -p ${acnLogs}/"$node"_logs/CNS-output/
    echo "Directory created: ${acnLogs}/"$node"_logs/CNS-output/"

    file="cnsCache.txt"
    kubectl exec -i -n kube-system $pod -- powershell 'Invoke-WebRequest -Uri 127.0.0.1:10090/debug/ipaddresses -Method Post -ContentType application/x-www-form-urlencoded -Body "{`"IPConfigStateFilter`":[`"Assigned`"]}" -UseBasicParsing | Select-Object -Expand Content' > ${acnLogs}/"$node"_logs/CNS-output/$file
    echo "CNS cache, $file, captured: ${acnLogs}/"$node"_logs/CNS-output/$file"

    file="azure-cns.json"
    kubectl exec -i -n kube-system $pod -- powershell cat ../../k/azurecns/azure-cns.json > ${acnLogs}/"$node"_logs/CNS-output/$file
    echo "CNS State, $file, captured: ${acnLogs}/"$node"_logs/CNS-output/$file"
    if [ $managed = "true" ]; then
      file="azure-endpoints.json"
      kubectl exec -i -n kube-system $pod -- powershell cat ../../k/azurecns/$file > ${acnLogs}/"$node"_logs/CNS-output/$file
      echo "CNS Managed State, $file, captured: ${acnLogs}/"$node"_logs/CNS-output/$file"
    fi
  done
fi

echo "------ HNS work ------"
kubectl get pods -n kube-system -l os=windows,app=privileged-daemonset -owide
echo "Capture HNS network and endpoint state from privileged pods"
for pod in $podList; do
  node=`kubectl get pod -n kube-system $pod -o custom-columns=NODE:.spec.nodeName,NAME:.metadata.name --no-headers | awk '{print $1}'`
  mkdir -p ${acnLogs}/"$node"_logs/HNS-output/
  echo "Directory created: ${acnLogs}/"$node"_logs/HNS-output/"

  file="hns-network.json"
  kubectl exec -i -n kube-system $pod -- powershell "Import-Module ../../k/hns.psm1 -ErrorAction SilentlyContinue; if (Get-Command Get-HnsNetwork -ErrorAction SilentlyContinue) { Get-HnsNetwork | ConvertTo-Json -Depth 20 } else { hnsdiag list networks | ConvertTo-Json -Depth 20 }" > ${acnLogs}/"$node"_logs/HNS-output/$file
  echo "HNS Networks, $file, captured: ${acnLogs}/"$node"_logs/HNS-output/$file"

  file="hns-endpoint.json"
  kubectl exec -i -n kube-system $pod -- powershell "Import-Module ../../k/hns.psm1 -ErrorAction SilentlyContinue; if (Get-Command Get-HnsEndpoint -ErrorAction SilentlyContinue) { Get-HnsEndpoint | ConvertTo-Json -Depth 20 } else { hnsdiag list endpoints | ConvertTo-Json -Depth 20 }" > ${acnLogs}/"$node"_logs/HNS-output/$file
  echo "HNS Endpoints, $file, captured: ${acnLogs}/"$node"_logs/HNS-output/$file"
done

# AKS-canonical comprehensive Windows collector. On by default (fullWindowsLogs
# env var, default true); set fullWindowsLogs=false to skip and keep artifacts
# small. The targeted azure*.log / azurecns / HNS captures always run regardless.
if [ "${fullWindowsLogs:-true}" = "true" ]; then
  echo "------ Full Windows log bundle (collect-windows-logs.ps1) ------"
  for pod in $podList; do
    node=`kubectl get pod -n kube-system $pod -o custom-columns=NODE:.spec.nodeName,NAME:.metadata.name --no-headers | awk '{print $1}'`
    mkdir -p ${acnLogs}/"$node"_logs/full-windows-logs/
    echo "Running collect-windows-logs.ps1 on $node (best-effort)"
    # Run the AKS canonical collector, capturing all streams (*>&1). Newer AKS node
    # images do NOT emit a .zip; the script gathers files into C:\k\debug\<random>\
    # and prints "Logs are available at <dir>". We capture that dir, archive it
    # ourselves, and always write collector-run.log so the folder is never empty.
    collectorOut=`kubectl exec -i -n kube-system $pod -- powershell "if (Test-Path ../../k/debug/collect-windows-logs.ps1) { Push-Location ../../k/debug; & .\collect-windows-logs.ps1 *>&1; Pop-Location } elseif (Test-Path ../../k/collect-windows-logs.ps1) { Push-Location ../../k; & .\collect-windows-logs.ps1 *>&1; Pop-Location } else { Write-Output 'collect-windows-logs.ps1 not found under ../../k/debug or ../../k' }"`
    echo "$collectorOut" | tr -d '\r' > ${acnLogs}/"$node"_logs/full-windows-logs/collector-run.log
    echo "collector-run.log captured: ${acnLogs}/"$node"_logs/full-windows-logs/collector-run.log"
    logDir=`echo "$collectorOut" | tr -d '\r' | grep -i "Logs are available at" | tail -1 | sed 's/.*[Ll]ogs are available at //;s/[[:space:]]*$//'`
    if [ -n "$logDir" ]; then
      echo "Collector output dir: $logDir - archiving to windows-logs.zip"
      kubectl exec -i -n kube-system $pod -- powershell "Compress-Archive -Path '$logDir\*' -DestinationPath '$logDir.zip' -Force -ErrorAction SilentlyContinue; if (Test-Path '$logDir.zip') { [Convert]::ToBase64String([IO.File]::ReadAllBytes('$logDir.zip')) }" | tr -d '\r' | base64 -d > ${acnLogs}/"$node"_logs/full-windows-logs/windows-logs.zip
      echo "Full Windows log bundle captured: ${acnLogs}/"$node"_logs/full-windows-logs/windows-logs.zip"
      # Extract the bundle so its text files (hnsdiag, vfpOutput, routes, kubelet,
      # containerd, etc.) are walked by the failure-agent, which only parses text
      # files and does not open zips. The zip is retained for human download.
      if [ -s ${acnLogs}/"$node"_logs/full-windows-logs/windows-logs.zip ] && command -v unzip >/dev/null 2>&1; then
        unzip -o -q ${acnLogs}/"$node"_logs/full-windows-logs/windows-logs.zip -d ${acnLogs}/"$node"_logs/full-windows-logs/extracted/ && echo "Extracted bundle for agent ingestion: ${acnLogs}/"$node"_logs/full-windows-logs/extracted/" || echo "unzip failed on $node (zip retained for humans)"
      else
        echo "unzip unavailable or empty zip on $node; agent reads collector-run.log, zip retained for humans"
      fi
    else
      # Older collectors may drop a pre-made zip; grab the newest under c:\k as a fallback.
      zipInfo=`kubectl exec -i -n kube-system $pod -- powershell '$zip = Get-ChildItem -Path "../../k/debug","../../k" -Recurse -Filter "*.zip" -ErrorAction SilentlyContinue | Sort-Object LastWriteTime -Descending | Select-Object -First 1; if ($zip) { Write-Output "$($zip.Name)|$($zip.FullName)" }' | tr -d '\r'`
      zipName=${zipInfo%%|*}
      zipPath=${zipInfo#*|}
      if [ -n "$zipInfo" ] && [ "$zipName" != "$zipPath" ]; then
        kubectl exec -i -n kube-system $pod -- powershell "[Convert]::ToBase64String([IO.File]::ReadAllBytes('$zipPath'))" | tr -d '\r' | base64 -d > ${acnLogs}/"$node"_logs/full-windows-logs/"$zipName"
        echo "Full Windows log bundle, $zipName, captured: ${acnLogs}/"$node"_logs/full-windows-logs/$zipName"
      else
        echo "collect-windows-logs.ps1 produced no output dir or zip on $node (see collector-run.log)"
      fi
    fi
  done
fi
