#!/bin/bash
resources=(
  "deployment/netperf-pod"
  "ds/netperf-pod"
  "ds/netperf-host"
)

sudo PERL_MM_USE_DEFAULT=1 perl -MCPAN -e 'install JSON::Parse'

for resource in "${resources[@]}"; do
  while [[ $(kubectl get "$resource" -o jsonpath='{.status.readyReplicas}') != $(kubectl get "$resource" -o jsonpath='{.spec.replicas}') ]]; do
    echo "Waiting for $resource to be ready..."
    sleep 1
  done
done

echo "All resources are ready!"

for i in {1..3}; do
    perl runNetPerfTestIperfOnly.pl --nobaseline > "out-${i}.log"
    kubectl get pods -owide | grep netperf >> "out-${i}.log"
    echo "Finished Perf Test Iter ${i}"
done

echo "Finished Perf Test"