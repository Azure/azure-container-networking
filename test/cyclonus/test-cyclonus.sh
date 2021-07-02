#!/usr/bin/env bash

set -eo pipefail
set -xv

kubectl delete --ignore-not-found=true clusterrolebinding cyclonus 
kubectl delete --ignore-not-found=true sa cyclonus -n kube-system
kubectl delete --ignore-not-found=true -f ./install-cyclonus.yaml
kubectl delete --ignore-not-found=true ns x y z

sleep 5

# set up cyclonus
kubectl create clusterrolebinding cyclonus --clusterrole=cluster-admin --serviceaccount=kube-system:cyclonus
kubectl create sa cyclonus -n kube-system
kubectl create -f ./install-cyclonus.yaml

sleep 5

time kubectl wait --for=condition=ready --timeout=5m pod -n kube-system -l job-name=cyclonus

#!/bin/bash
{ kubectl logs -f -n kube-system job.batch/cyclonus;  } &
{ time kubectl wait --for=condition=completed --timeout=600m pod -n kube-system -l job-name=cyclonus;  } &
wait -n
pkill -P $$
echo done

# grab the job logs
LOG_FILE=cyclonus-test.txt
kubectl logs -n kube-system job.batch/cyclonus | tee "$LOG_FILE"
cat "$LOG_FILE"

kubectl delete --ignore-not-found=true clusterrolebinding cyclonus 
kubectl delete --ignore-not-found=true sa cyclonus -n kube-system
kubectl delete --ignore-not-found=true -f ./install-cyclonus.yaml

# if 'failure' is in the logs, fail; otherwise succeed
rc=0

cat "$LOG_FILE" | grep "failed" > /dev/null 2>&1 || rc=$?
echo $rc
if [ $rc -eq 0 ]; then
    exit 1
fi
