# general cilium variables
DIR 									?= 1.17
CILIUM_VERSION_TAG               		?= v1.17.7-250927
CILIUM_IMAGE_REGISTRY           		?= mcr.microsoft.com/containernetworking
IPV6_IMAGE_REGISTRY						?= mcr.microsoft.com/containernetworking
IPV6_HP_BPF_VERSION               		?= v0.0.1
CILIUM_NIGHTLY_VERSION_TAG 				?= cilium-nightly-pipeline

# ebpf cilium variables
EBPF_CILIUM_DIR				     		?= 1.17
# we don't use CILIUM_VERSION_TAG or CILIUM_IMAGE_REGISTRY because we want to use the version supported by ebpf
EBPF_CILIUM_IMAGE_REGISTRY           	?= mcr.microsoft.com/containernetworking
EBPF_CILIUM_VERSION_TAG               	?= v1.17.7-250927
AZURE_IPTABLES_MONITOR_IMAGE_REGISTRY	?= mcr.microsoft.com/containernetworking
AZURE_IPTABLES_MONITOR_TAG          	?= v0.0.3
AZURE_IP_MASQ_MERGER_IMAGE_REGISTRY		?= mcr.microsoft.com/containernetworking
AZURE_IP_MASQ_MERGER_TAG            	?= v0.0.1-0

# so we can use in envsubst
export IPV6_IMAGE_REGISTRY
export IPV6_HP_BPF_VERSION
export CILIUM_VERSION_TAG
export CILIUM_IMAGE_REGISTRY
export CILIUM_NIGHTLY_VERSION_TAG

# ebpf
export AZURE_IPTABLES_MONITOR_IMAGE_REGISTRY
export AZURE_IPTABLES_MONITOR_TAG
export AZURE_IP_MASQ_MERGER_IMAGE_REGISTRY
export AZURE_IP_MASQ_MERGER_TAG

# print variable targets
print-cilium-vars:
	@echo "DIR: $(DIR)"
	@echo "CILIUM_IMAGE_REGISTRY: $(CILIUM_IMAGE_REGISTRY)"
	@echo "CILIUM_VERSION_TAG: $(CILIUM_VERSION_TAG)"

print-cilium-nightly-vars:
	@echo "CILIUM_IMAGE_REGISTRY: $(CILIUM_IMAGE_REGISTRY)"
	@echo "CILIUM_NIGHTLY_VERSION_TAG: $(CILIUM_NIGHTLY_VERSION_TAG)"

print-cilium-dualstack-vars: print-cilium-vars
	@echo "IPV6_IMAGE_REGISTRY: $(IPV6_IMAGE_REGISTRY)"
	@echo "IPV6_HP_BPF_VERSION: $(IPV6_HP_BPF_VERSION)"

print-ebpf-cilium-vars:
	@echo "EBPF_CILIUM_DIR: $(EBPF_CILIUM_DIR)"
	@echo "EBPF_CILIUM_IMAGE_REGISTRY: $(EBPF_CILIUM_IMAGE_REGISTRY)"
	@echo "EBPF_CILIUM_VERSION_TAG: $(EBPF_CILIUM_VERSION_TAG)"

wait-for-cilium:
	@kubectl rollout status deployment/cilium-operator -n kube-system --timeout=1200s
	@kubectl rollout status daemonset/cilium -n kube-system --timeout=1200s

# vanilla cilium deployment
deploy-cilium-config:
	kubectl apply -f ../../test/integration/manifests/cilium/v$(DIR)/cilium-config/cilium-config.yaml

deploy-cilium-agent:
	kubectl apply -f ../../test/integration/manifests/cilium/v$(DIR)/cilium-agent/files
	envsubst '$${CILIUM_VERSION_TAG},$${CILIUM_IMAGE_REGISTRY}' < ../../test/integration/manifests/cilium/v$(DIR)/cilium-agent/templates/daemonset.yaml | kubectl apply -f -

deploy-cilium-operator:
	kubectl apply -f ../../test/integration/manifests/cilium/v$(DIR)/cilium-operator/files
	envsubst '$${CILIUM_VERSION_TAG},$${CILIUM_IMAGE_REGISTRY}' < ../../test/integration/manifests/cilium/v$(DIR)/cilium-operator/templates/deployment.yaml | kubectl apply -f -

deploy-cilium: print-cilium-vars deploy-cilium-config deploy-cilium-agent deploy-cilium-operator wait-for-cilium

# cilium with hubble deployment
deploy-cilium-config-hubble:
	kubectl apply -f ../../test/integration/manifests/cilium/v$(DIR)/cilium-config/cilium-config-hubble.yaml

deploy-cilium-hubble: print-cilium-vars deploy-cilium-config-hubble deploy-cilium-agent deploy-cilium-operator wait-for-cilium

# deploys the hubble components
deploy-hubble: print-cilium-vars
	kubectl apply -f ../../test/integration/manifests/cilium/hubble/hubble-peer-svc.yaml
	kubectl apply -f ../../test/integration/manifests/cilium/v${DIR}/cilium-config/cilium-config-hubble.yaml

# cilium nightly deployment
deploy-cilium-config-nightly:
	kubectl apply -f ../../test/integration/manifests/cilium/cilium-nightly-config.yaml

deploy-cilium-agent-nightly:
	CILIUM_VERSION_TAG=$(CILIUM_NIGHTLY_VERSION_TAG) \
		envsubst '$${CILIUM_VERSION_TAG},$${CILIUM_IMAGE_REGISTRY}' < ../../test/integration/manifests/cilium/daemonset.yaml | kubectl apply -f -
	kubectl apply -f ../../test/integration/manifests/cilium/cilium-nightly-agent

deploy-cilium-operator-nightly:
	CILIUM_VERSION_TAG=$(CILIUM_NIGHTLY_VERSION_TAG) \
		envsubst '$${CILIUM_VERSION_TAG},$${CILIUM_IMAGE_REGISTRY}' < ../../test/integration/manifests/cilium/deployment.yaml | kubectl apply -f -
	kubectl apply -f ../../test/integration/manifests/cilium/cilium-nightly-operator

deploy-cilium-nightly: print-cilium-nightly-vars deploy-cilium-config-nightly deploy-cilium-agent-nightly deploy-cilium-operator-nightly wait-for-cilium

# cilium dualstack deployment
deploy-cilium-config-dualstack:
	kubectl apply -f ../../test/integration/manifests/cilium/v$(DIR)/cilium-config/cilium-config-dualstack.yaml

deploy-cilium-agent-dualstack:
	kubectl apply -f ../../test/integration/manifests/cilium/v$(DIR)/cilium-agent/files
	envsubst '$${CILIUM_VERSION_TAG},$${CILIUM_IMAGE_REGISTRY},$${IPV6_IMAGE_REGISTRY},$${IPV6_HP_BPF_VERSION}' < ../../test/integration/manifests/cilium/v$(DIR)/cilium-agent/templates/daemonset-dualstack.yaml | kubectl apply -f -

deploy-cilium-operator-dualstack:
	kubectl apply -f ../../test/integration/manifests/cilium/v$(DIR)/cilium-operator/files
	envsubst '$${CILIUM_VERSION_TAG},$${CILIUM_IMAGE_REGISTRY}' < ../../test/integration/manifests/cilium/v$(DIR)/cilium-operator/templates/deployment.yaml | kubectl apply -f -

deploy-cilium-dualstack: print-cilium-dualstack-vars deploy-cilium-config-dualstack deploy-cilium-agent-dualstack deploy-cilium-operator-dualstack wait-for-cilium

# ebpf
deploy-common-ebpf-cilium:
	@kubectl apply -f ../../test/integration/manifests/cilium/v$(EBPF_CILIUM_DIR)/cilium-agent/files/
	@kubectl apply -f ../../test/integration/manifests/cilium/v$(EBPF_CILIUM_DIR)/cilium-operator/files/
# set cilium version tag and registry here so they are visible as env vars to envsubst
	CILIUM_VERSION_TAG=$(EBPF_CILIUM_VERSION_TAG) CILIUM_IMAGE_REGISTRY=$(EBPF_CILIUM_IMAGE_REGISTRY) \
		envsubst '$${CILIUM_VERSION_TAG},$${CILIUM_IMAGE_REGISTRY},$${IPV6_HP_BPF_VERSION}' < \
		../../test/integration/manifests/cilium/v$(EBPF_CILIUM_DIR)/cilium-operator/templates/deployment.yaml \
		| kubectl apply -f -
	@kubectl apply -f ../../test/integration/manifests/cilium/v$(EBPF_CILIUM_DIR)/ebpf/common/ciliumclusterwidenetworkpolicies.yaml
	@kubectl wait --for=condition=Established crd/ciliumclusterwidenetworkpolicies.cilium.io
	@kubectl apply -f ../../test/integration/manifests/cilium/v$(EBPF_CILIUM_DIR)/ebpf/common/

deploy-ebpf-overlay-cilium: print-ebpf-cilium-vars deploy-common-ebpf-cilium
	@kubectl apply -f ../../test/integration/manifests/cilium/v$(EBPF_CILIUM_DIR)/ebpf/overlay/static/
	CILIUM_VERSION_TAG=$(EBPF_CILIUM_VERSION_TAG) CILIUM_IMAGE_REGISTRY=$(EBPF_CILIUM_IMAGE_REGISTRY) \
		envsubst '$${CILIUM_VERSION_TAG},$${CILIUM_IMAGE_REGISTRY},$${IPV6_HP_BPF_VERSION},$${AZURE_IPTABLES_MONITOR_IMAGE_REGISTRY},$${AZURE_IPTABLES_MONITOR_TAG},$${AZURE_IP_MASQ_MERGER_IMAGE_REGISTRY},$${AZURE_IP_MASQ_MERGER_TAG}' < \
		../../test/integration/manifests/cilium/v$(EBPF_CILIUM_DIR)/ebpf/overlay/cilium.yaml \
		| kubectl apply -f -
	@$(MAKE) wait-for-cilium

deploy-ebpf-podsubnet-cilium: print-ebpf-cilium-vars deploy-common-ebpf-cilium
	@kubectl apply -f ../../test/integration/manifests/cilium/v$(EBPF_CILIUM_DIR)/ebpf/podsubnet/static/
# ebpf podsubnet does not have ip masq merger 
	CILIUM_VERSION_TAG=$(EBPF_CILIUM_VERSION_TAG) CILIUM_IMAGE_REGISTRY=$(EBPF_CILIUM_IMAGE_REGISTRY) \
		envsubst '$${CILIUM_VERSION_TAG},$${CILIUM_IMAGE_REGISTRY},$${IPV6_HP_BPF_VERSION},$${AZURE_IPTABLES_MONITOR_IMAGE_REGISTRY},$${AZURE_IPTABLES_MONITOR_TAG}' < \
		../../test/integration/manifests/cilium/v$(EBPF_CILIUM_DIR)/ebpf/podsubnet/cilium.yaml \
		| kubectl apply -f -
	@$(MAKE) wait-for-cilium

