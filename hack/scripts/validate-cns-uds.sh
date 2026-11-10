#!/usr/bin/env bash
# Checks on every Linux node that CNS serves IPAM requests only on its Unix socket.
# Run this script from the repository root with a kubeconfig for the test cluster.
set -euo pipefail

kubectl apply -f test/integration/manifests/load/privileged-daemonset.yaml
kubectl -n kube-system rollout status daemonset/privileged-daemonset --timeout=5m

# The privileged pod runs this check in the host mount and network namespaces.
# shellcheck disable=SC2016 # The host shell expands these variables.
check='
set -euo pipefail
socket=/var/run/azure-cns/cns.sock
test -S "$socket"
test "$(stat -c %a "$socket")" = 600
test $(( 0$(stat -c %a /var/run/azure-cns) & 022 )) -eq 0
grep -qs "\"cnsSocketPath\": \"$socket\"" /etc/cni/net.d/*.conflist
tcp=$(curl -sS -o /dev/null -w "%{http_code}" -X POST -d "{" http://127.0.0.1:10090/network/requestipconfigs)
test "$tcp" = 404
unix=$(curl -sS -o /dev/null -w "%{http_code}" --unix-socket "$socket" -X POST -d "{" http://localhost/network/requestipconfigs)
test "$unix" != 404
'

pods=$(kubectl -n kube-system get pods -l app=privileged-daemonset -o jsonpath='{.items[*].metadata.name}')
if [ -z "$pods" ]; then
  echo "no privileged pods found" >&2
  exit 1
fi

failed=0
for pod in $pods; do
  node=$(kubectl -n kube-system get pod "$pod" -o jsonpath='{.spec.nodeName}')
  for _ in $(seq 1 60); do
    if kubectl -n kube-system exec "$pod" -- nsenter -t 1 -m -n -- bash -c "$check"; then
      echo "CNS IPAM uses the Unix socket on $node"
      continue 2
    fi
    sleep 5
  done
  echo "CNS IPAM does not use only the Unix socket on $node" >&2
  failed=1
done
exit "$failed"
