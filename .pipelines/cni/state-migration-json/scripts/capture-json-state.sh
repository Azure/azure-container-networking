#!/usr/bin/env bash
set -euo pipefail

os="${1:?operating system is required}"
output_dir="${2:?output directory is required}"

case "$os" in
  linux)
    selector="k8s-app=azure-cns"
    ;;
  windows)
    selector="k8s-app=azure-cns-win"
    ;;
  *)
    echo "unsupported operating system: $os" >&2
    exit 2
    ;;
esac

mkdir -p "$output_dir"
kubectl get nodes -o json > "$output_dir/nodes.json"
kubectl get pods -A -o json > "$output_dir/pods.json"

mapfile -t cns_pods < <(
  kubectl get pods -n kube-system -l "$selector" \
    -o jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}'
)
if [[ ${#cns_pods[@]} -eq 0 ]]; then
  echo "no CNS pods found for selector $selector" >&2
  exit 1
fi

for pod in "${cns_pods[@]}"; do
  node="$(
    kubectl get pod "$pod" -n kube-system \
      -o jsonpath='{.spec.nodeName}'
  )"
  node_dir="$output_dir/$node"
  mkdir -p "$node_dir"

  if [[ "$os" == "linux" ]]; then
    kubectl exec "$pod" -n kube-system -- \
      cat /etc/azure-cns/cns_config.json > "$node_dir/cns_config.json"
    kubectl exec "$pod" -n kube-system -- \
      cat /var/lib/azure-network/azure-cns.json > "$node_dir/azure-cns.json"
    kubectl exec "$pod" -n kube-system -- \
      curl --fail --silent --show-error localhost:10090/debug/ipaddresses \
        -d '{"IPConfigStateFilter":["Assigned"]}' > "$node_dir/assigned-ip-configs.json"
  else
    kubectl exec "$pod" -n kube-system -- powershell -NoProfile -NonInteractive \
      -Command "Get-Content -Raw etc/azure-cns/cns_config.json" \
      > "$node_dir/cns_config.json"
    kubectl exec "$pod" -n kube-system -- powershell -NoProfile -NonInteractive \
      -Command "Get-Content -Raw k/azurecns/azure-cns.json" \
      > "$node_dir/azure-cns.json"
    kubectl exec "$pod" -n kube-system -- powershell -NoProfile -NonInteractive \
      -Command 'Invoke-WebRequest -Uri 127.0.0.1:10090/debug/ipaddresses -Method Post -ContentType application/x-www-form-urlencoded -Body "{`"IPConfigStateFilter`":[`"Assigned`"]}" -UseBasicParsing | Select-Object -Expand Content' \
      > "$node_dir/assigned-ip-configs.json"
  fi

  jq -e . "$node_dir/cns_config.json" >/dev/null
  jq -e . "$node_dir/azure-cns.json" >/dev/null
  jq -e . "$node_dir/assigned-ip-configs.json" >/dev/null
done
