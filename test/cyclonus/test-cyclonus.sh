#!/usr/bin/env bash

set -eo pipefail
set -xv

if [ -z "$1" == "windows"]
  then
    cyclonusProfile="./install-cyclonus-windows.yaml"
  else
    cyclonusProfile="./install-cyclonus.yaml"
fi


if [ -z "$1" ]
  then
    echo "Running with default profile: $cyclonusProfile"
elif [[ $1 == "extended" ]]; 
then
    # extended will exclude SCTP and will run 214 testcases with cyclonus 
    cyclonusProfile="./install-cyclonus-exclude-sctp.yaml"
    echo "Running with exclude SCTP profile with 214 testcases: $cyclonusProfile"
fi

echo "Running with cyclonus profile: $cyclonusProfile"
kubectl delete --ignore-not-found=true clusterrolebinding cyclonus 
kubectl delete --ignore-not-found=true sa cyclonus -n kube-system
kubectl delete --ignore-not-found=true -f $cyclonusProfile
kubectl delete --ignore-not-found=true ns x y z

sleep 5

# set up cyclonus
kubectl create clusterrolebinding cyclonus --clusterrole=cluster-admin --serviceaccount=kube-system:cyclonus
kubectl create sa cyclonus -n kube-system
kubectl create -f $cyclonusProfile

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
kubectl delete --ignore-not-found=true -f $cyclonusProfile

# might need to redirect to /dev/null 2>&1 instead of just grepping with -q to avoid "cat: write error: Broken pipe"
rc=999
cat $LOG_FILE | grep "SummaryTable:" > /dev/null 2>&1 && rc=$?
echo $rc
if [ $rc -ne 0 ]; then
    echo "FAILING because cyclonus tests did not complete"
    exit 2
fi

rc=0
cat $LOG_FILE | grep "failed" > /dev/null 2>&1 || rc=$?
echo $rc
if [ $rc -eq 0 ]; then
    echo "FAILING because cyclonus completed but failures detected"
    exit 3
fi
