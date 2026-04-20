#!/bin/bash
# Attempts to patch kubeclusterconfig.json and windowsnodereset.ps1 on Windows nodes
# - Sets Cni.Name to "azure" in kubeclusterconfig.json
# - Replaces '>> $global:LogPath' with '*>> $global:LogPath' in windowsnodereset.ps1
# Usage: bash patch-kubeclusterconfig.sh

echo "Patching kubeclusterconfig.json and windowsnodereset.ps1 on Windows nodes"
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

  if [ "$succeeded" = true ]; then
    succeeded=false
    for attempt in 1 2 3; do
      echo "Attempt $attempt: Patching windowsnodereset.ps1 on $pod"
      # Replace ">> $global:LogPath" (or "*>> $global:LogPath", "**>>", etc.) with "*>> $global:LogPath"
      # so that all output streams are captured to the log file. The regex is idempotent.
      if kubectl exec -n kube-system "$pod" -- powershell.exe -command \
        "(Get-Content 'c:\k\windowsnodereset.ps1') -replace '[*]*>> \\\$global:LogPath', '*>> \$global:LogPath' | Set-Content 'c:\k\windowsnodereset.ps1'"; then
        echo "Successfully patched windowsnodereset.ps1 on $pod"
        succeeded=true
        break
      else
        echo "Failed to patch windowsnodereset.ps1 on $pod (attempt $attempt)"
        sleep 20
      fi
    done
  fi

  if [ "$succeeded" = false ]; then
    echo "WARNING: Failed to patch on $pod after 3 attempts"
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
