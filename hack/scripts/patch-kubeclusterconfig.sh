#!/bin/bash
# Attempts to patch kubeclusterconfig.json on Windows nodes to set Cni.Name to "azure"
# Usage: bash patch-kubeclusterconfig.sh

echo "Patching kubeclusterconfig.json CNI name on Windows nodes"
kubectl apply -f ../../test/integration/manifests/load/privileged-daemonset-windows.yaml
kubectl rollout status ds -n kube-system privileged-daemonset --timeout=5m

podList=$(kubectl get pods -n kube-system -l os=windows,app=privileged-daemonset --no-headers -o custom-columns=NAME:.metadata.name)
allSucceeded=true
for pod in $podList; do
  succeeded=false
  for attempt in 1 2 3; do
    echo "Attempt $attempt: Patching kubeclusterconfig.json on $pod"
    if kubectl exec -n kube-system "$pod" -- powershell.exe -command \
      'Get-Content "c:\k\kubeclusterconfig.json" -Raw | ConvertFrom-Json | % { $_.Cni.Name = "azure"; $_ } | ConvertTo-Json -Depth 20 | Set-Content "c:\k\kubeclusterconfig.json"'; then
      echo "Successfully patched kubeclusterconfig.json on $pod"
      succeeded=true
      break
    else
      echo "Failed to patch kubeclusterconfig.json on $pod (attempt $attempt)"
      sleep 20
    fi
  done
  if [ "$succeeded" = false ]; then
    echo "WARNING: Failed to patch kubeclusterconfig.json on $pod after 3 attempts"
    allSucceeded=false
  fi
done

if [ "$allSucceeded" = true ]; then
  echo "All nodes patched successfully"
else
  echo "WARNING: Some nodes failed to patch, continuing anyway"
fi

echo "Cleaning up privileged daemonset"
kubectl delete ds -n kube-system privileged-daemonset --ignore-not-found
