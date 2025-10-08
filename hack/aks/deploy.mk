EBPF_CILIUM_DIR				     		?= 1.17
EBPF_CILIUM_VERSION_TAG               	?= v1.17.7-250927
EBPF_CILIUM_IMAGE_REGISTRY           	?= mcr.microsoft.com/containernetworking
IPV6_HP_BPF_VERSION               		?= v0.0.1
AZURE_IPTABLES_MONITOR_IMAGE_REGISTRY	?= mcr.microsoft.com/containernetworking
AZURE_IPTABLES_MONITOR_TAG          	?= v0.0.3
AZURE_IP_MASQ_MERGER_IMAGE_REGISTRY		?= mcr.microsoft.com/containernetworking
AZURE_IP_MASQ_MERGER_TAG            	?= v0.0.1-0

deploy-overlay-ebpf-cilium:
	@kubectl apply -f ../../test/integration/manifests/cilium/v$(EBPF_CILIUM_DIR)/cilium-agent/files/
	@kubectl apply -f ../../test/integration/manifests/cilium/v$(EBPF_CILIUM_DIR)/cilium-operator/files/
	@kubectl apply -f ../../test/integration/manifests/cilium/v$(EBPF_CILIUM_DIR)/ebpf/common/
	@kubectl apply -f ../../test/integration/manifests/cilium/v$(EBPF_CILIUM_DIR)/ebpf/overlay/static/
	envsubst '$${EBPF_CILIUM_VERSION_TAG},$${EBPF_CILIUM_IMAGE_REGISTRY},$${IPV6_HP_BPF_VERSION}' < ../../test/integration/manifests/cilium/v1.17/cilium-operator/templates/deployment.yaml | kubectl apply -f -
	envsubst '$${EBPF_CILIUM_VERSION_TAG},$${EBPF_CILIUM_IMAGE_REGISTRY},$${IPV6_HP_BPF_VERSION},$${AZURE_IPTABLES_MONITOR_IMAGE_REGISTRY},$${AZURE_IPTABLES_MONITOR_TAG},$${AZURE_IP_MASQ_MERGER_IMAGE_REGISTRY},$${AZURE_IP_MASQ_MERGER_TAG}' < ../../test/integration/manifests/cilium/v1.17/ebpf/overlay/cilium-overlay.yaml | kubectl apply -f -
