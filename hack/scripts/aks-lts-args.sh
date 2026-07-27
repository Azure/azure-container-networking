# Echo the extra `az aks create` flags needed when K8S_VER is on an LTS-only
# Kubernetes minor (one that has left community support), otherwise echo nothing.
# Used by hack/aks/Makefile's `LTS=auto` mode.
#
# Inputs (from the environment):
#   K8S_VER            requested Kubernetes version, e.g. 1.33 or 1.33.2
#   REGION             AKS region to query
#   AZCLI              az command (may be "az" or a dockerized wrapper)
#   LTS_PREMIUM_ARGS   flags to emit when the minor is LTS-only

# Reduce the requested version to its minor, e.g. 1.33.2 -> 1.33.
minor=$(echo "$K8S_VER" | cut -d. -f1,2)

# A minor is "LTS-only" when AKS advertises the AKSLongTermSupport support plan
# for it but not KubernetesOfficial; such minors cannot be created without the
# LTS flags. List every LTS-only minor offered in the region.
lts_only_minors=$($AZCLI aks get-versions --location "$REGION" \
	--query "values[?contains(capabilities.supportPlan,'AKSLongTermSupport') && !contains(capabilities.supportPlan,'KubernetesOfficial')].version" \
	--output tsv 2>/dev/null)

# If the requested minor is one of them, emit the LTS args.
if echo "$lts_only_minors" | grep -Fqx "$minor"; then
	echo "$LTS_PREMIUM_ARGS"
fi
