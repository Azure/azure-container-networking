DIR				     					?= v1.17
CILIUM_VERSION_TAG               		?= v1.17.7
CILIUM_IMAGE_REGISTRY           		?= mcr.microsoft.com/containernetworking
IPV6_HP_BPF_VERSION               		?= v0.0.1
AZURE_IPTABLES_MONITOR_IMAGE_REGISTRY	?= mcr.microsoft.com/containernetworking
AZURE_IPTABLES_MONITOR_TAG          	?= v0.0.3
AZURE_IP_MASQ_MERGER_IMAGE_REGISTRY		?= mcr.microsoft.com/containernetworking
AZURE_IP_MASQ_MERGER_TAG            	?= v0.0.1-0

deploy-overlay-ebpf-cilium:
	@kubectl apply -f ../../test/integration/manifests/cilium/$(DIR)/cilium-agent/files/
	@kubectl apply -f ../../test/integration/manifests/cilium/$(DIR)/cilium-operator/files/
	@kubectl apply -f ../../test/integration/manifests/cilium/$(DIR)/ebpf/common/
	@kubectl apply -f ../../test/integration/manifests/cilium/$(DIR)/ebpf/overlay/static/
	envsubst '$${CILIUM_VERSION_TAG},$${CILIUM_IMAGE_REGISTRY},$${IPV6_HP_BPF_VERSION}' < ../../test/integration/manifests/cilium/v1.17/cilium-operator/templates/deployment.yaml | kubectl apply -f -
	envsubst '$${CILIUM_VERSION_TAG},$${CILIUM_IMAGE_REGISTRY},$${IPV6_HP_BPF_VERSION},$${AZURE_IPTABLES_MONITOR_IMAGE_REGISTRY},$${AZURE_IPTABLES_MONITOR_TAG},$${AZURE_IP_MASQ_MERGER_IMAGE_REGISTRY},$${AZURE_IP_MASQ_MERGER_TAG}' < ../../test/integration/manifests/cilium/v1.17/ebpf/overlay/cilium-overlay.yaml | kubectl apply -f -
