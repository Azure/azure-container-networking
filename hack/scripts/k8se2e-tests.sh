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
--ginkgo.skip='SCTP|Disruptive|Slow|hostNetwork|kube-proxy|IPv6' \
--ginkgo.flakeAttempts=3 \
--ginkgo.v \
--node-os-distro=$OS \
--kubeconfig=$HOME/.kube/config"
./ginkgo --nodes=4 \
./e2e.test -- \
--num-nodes=2 \
--provider=skeleton \
--ginkgo.focus='(.*).Networking.should|(.*).Networking.Granular|(.*)kubernetes.api' \
--ginkgo.skip='SCTP|Disruptive|Slow|hostNetwork|kube-proxy|IPv6' \
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
--ginkgo.skip='SCTP|Disruptive|Slow|hostNetwork|kube-proxy|IPv6|resolv|exists conflict$SKIP' \
--ginkgo.flakeAttempts=3 \
--ginkgo.v \
--node-os-distro=$OS \
--kubeconfig=$HOME/.kube/config"
./ginkgo --nodes=4 \
./e2e.test -- \
--num-nodes=2 \
--provider=skeleton \
--ginkgo.focus='(.*).Networking.should|(.*).Networking.Granular|(.*)kubernetes.api|\[sig-network\].DNS.should|\[sig-cli\].Kubectl.Port|Services.*\[Conformance\].*|\[sig-network\](.*)HostPort|\[sig-scheduling\](.*)hostPort' \
--ginkgo.skip="SCTP|Disruptive|Slow|hostNetwork|kube-proxy|IPv6|resolv|exists conflict$SKIP" \
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
