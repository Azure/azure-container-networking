OS=$1
TYPE=$2

# Map of upstream Kubernetes feature gates that are alpha-default-off until the
# minor version listed below (at which point they go Beta-default-on). AKS does
# not expose kube-apiserver --feature-gates, so on clusters older than the
# default-on version we skip the corresponding [FeatureGate:*] labeled e2e
# specs (otherwise the spec contacts an apiserver that rejects it on validation
# and fails). When AKS catches up to the default-on minor, the entry can be
# dropped from this map and the spec will run for real.
declare -A FEATURE_GATE_DEFAULT_ON_FROM=(
  [RelaxedServiceNameValidation]="1.36"
)

K8S_VER=$(cat ./k8s-version 2>/dev/null | sed 's/^v//')
K8S_MM=$(echo "$K8S_VER" | awk -F. '{ if ($1 != "") printf "%d.%d", $1, $2 }')

ver_lt() {
  [ "$1" != "$2" ] && [ "$(printf '%s\n%s\n' "$1" "$2" | sort -V | head -1)" = "$1" ]
}

GATE_SKIP=""
for gate in "${!FEATURE_GATE_DEFAULT_ON_FROM[@]}"; do
  min="${FEATURE_GATE_DEFAULT_ON_FROM[$gate]}"
  # If we couldn't read the cluster version, be conservative and skip.
  if [ -z "$K8S_MM" ] || ver_lt "$K8S_MM" "$min"; then
    GATE_SKIP="${GATE_SKIP}|\\[FeatureGate:${gate}\\]"
  fi
done
echo "Cluster k8s version: ${K8S_VER:-unknown}; feature-gate skips: ${GATE_SKIP:-<none>}"

# Taint Linux nodes so that windows tests do not run on them and ensure no LinuxOnly tests run on windows nodes
if [[ 'windows' == $OS ]]
then
SKIP="|LinuxOnly"
kubectl taint nodes -l kubernetes.azure.com/mode=system node-role.kubernetes.io/control-plane:NoSchedule
fi



if [[ 'basic' == $TYPE ]]
then
echo "Testing Datapath"
echo "./ginkgo --nodes=4 \
./e2e.test -- \
--num-nodes=2 \
--provider=skeleton \
--ginkgo.focus='(.*).Networking.should|(.*).Networking.Granular|(.*)kubernetes.api' \
--ginkgo.skip='SCTP|Disruptive|Slow|hostNetwork|kube-proxy|IPv6${SKIP}${GATE_SKIP}' \
--ginkgo.flakeAttempts=3 \
--ginkgo.v \
--node-os-distro=$OS \
--kubeconfig=$HOME/.kube/config"
./ginkgo --nodes=4 \
./e2e.test -- \
--num-nodes=2 \
--provider=skeleton \
--ginkgo.focus='(.*).Networking.should|(.*).Networking.Granular|(.*)kubernetes.api' \
--ginkgo.skip="SCTP|Disruptive|Slow|hostNetwork|kube-proxy|IPv6$SKIP$GATE_SKIP" \
--ginkgo.flakeAttempts=3 \
--ginkgo.v \
--node-os-distro=$OS \
--kubeconfig=$HOME/.kube/config
else
echo "Testing Datapath, DNS, PortForward, Service, and Hostport"
echo "./ginkgo --nodes=4 \
./e2e.test -- \
--num-nodes=2 \
--provider=skeleton \
--ginkgo.focus='(.*).Networking.should|(.*).Networking.Granular|(.*)kubernetes.api|\[sig-network\].DNS.should|\[sig-cli\].Kubectl.Port|Services.*\[Conformance\].*|\[sig-network\](.*)HostPort|\[sig-scheduling\](.*)hostPort' \
--ginkgo.skip='SCTP|Disruptive|Slow|hostNetwork|kube-proxy|IPv6|resolv|exists conflict${SKIP}${GATE_SKIP}' \
--ginkgo.flakeAttempts=3 \
--ginkgo.v \
--node-os-distro=$OS \
--kubeconfig=$HOME/.kube/config"
./ginkgo --nodes=4 \
./e2e.test -- \
--num-nodes=2 \
--provider=skeleton \
--ginkgo.focus='(.*).Networking.should|(.*).Networking.Granular|(.*)kubernetes.api|\[sig-network\].DNS.should|\[sig-cli\].Kubectl.Port|Services.*\[Conformance\].*|\[sig-network\](.*)HostPort|\[sig-scheduling\](.*)hostPort' \
--ginkgo.skip="SCTP|Disruptive|Slow|hostNetwork|kube-proxy|IPv6|resolv|exists conflict$SKIP$GATE_SKIP" \
--ginkgo.flakeAttempts=3 \
--ginkgo.v \
--node-os-distro=$OS \
--kubeconfig=$HOME/.kube/config
fi

# Untaint Linux nodes once testing is complete
if [[ 'windows' == $OS ]]
then
kubectl taint nodes -l kubernetes.azure.com/mode=system node-role.kubernetes.io/control-plane:NoSchedule-
fi
