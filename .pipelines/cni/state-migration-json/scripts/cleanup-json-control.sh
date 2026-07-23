#!/usr/bin/env bash
set -euo pipefail

kubectl delete namespace load-test --ignore-not-found --wait=true --timeout=15m
kubectl delete daemonset privileged-daemonset -n kube-system --ignore-not-found
