EBPF_CILIUM_DIR				     		?= 1.17
# we don't use CILIUM_VERSION_TAG or CILIUM_IMAGE_REGISTRY because we want to use the version supported by ebpf
EBPF_CILIUM_VERSION_TAG               	?= v1.17.7-250927
EBPF_CILIUM_IMAGE_REGISTRY           	?= mcr.microsoft.com/containernetworking
IPV6_HP_BPF_VERSION               		?= v0.0.1
AZURE_IPTABLES_MONITOR_IMAGE_REGISTRY	?= mcr.microsoft.com/containernetworking
AZURE_IPTABLES_MONITOR_TAG          	?= v0.0.3
AZURE_IP_MASQ_MERGER_IMAGE_REGISTRY		?= mcr.microsoft.com/containernetworking
AZURE_IP_MASQ_MERGER_TAG            	?= v0.0.1-0
# so we can use in envsubst
export IPV6_HP_BPF_VERSION
export AZURE_IPTABLES_MONITOR_IMAGE_REGISTRY
export AZURE_IPTABLES_MONITOR_TAG
export AZURE_IP_MASQ_MERGER_IMAGE_REGISTRY
export AZURE_IP_MASQ_MERGER_TAG

deploy-overlay-ebpf-cilium:
	@kubectl apply -f ../../test/integration/manifests/cilium/v$(EBPF_CILIUM_DIR)/cilium-agent/files/
	@kubectl apply -f ../../test/integration/manifests/cilium/v$(EBPF_CILIUM_DIR)/cilium-operator/files/
	@kubectl apply -f ../../test/integration/manifests/cilium/v$(EBPF_CILIUM_DIR)/ebpf/common/
	@kubectl apply -f ../../test/integration/manifests/cilium/v$(EBPF_CILIUM_DIR)/ebpf/overlay/static/
# set cilium version tag and registry here so they are visible as env vars to envsubst
	CILIUM_VERSION_TAG=$(EBPF_CILIUM_VERSION_TAG) CILIUM_IMAGE_REGISTRY=$(EBPF_CILIUM_IMAGE_REGISTRY) \
		envsubst '$${CILIUM_VERSION_TAG},$${CILIUM_IMAGE_REGISTRY},$${IPV6_HP_BPF_VERSION}' < \
		../../test/integration/manifests/cilium/v$(EBPF_CILIUM_DIR)/cilium-operator/templates/deployment.yaml \
		| kubectl apply -f -
	CILIUM_VERSION_TAG=$(EBPF_CILIUM_VERSION_TAG) CILIUM_IMAGE_REGISTRY=$(EBPF_CILIUM_IMAGE_REGISTRY) \
		envsubst '$${CILIUM_VERSION_TAG},$${CILIUM_IMAGE_REGISTRY},$${IPV6_HP_BPF_VERSION},$${AZURE_IPTABLES_MONITOR_IMAGE_REGISTRY},$${AZURE_IPTABLES_MONITOR_TAG},$${AZURE_IP_MASQ_MERGER_IMAGE_REGISTRY},$${AZURE_IP_MASQ_MERGER_TAG}' < \
		../../test/integration/manifests/cilium/v$(EBPF_CILIUM_DIR)/ebpf/overlay/cilium-overlay.yaml \
		| kubectl apply -f -
