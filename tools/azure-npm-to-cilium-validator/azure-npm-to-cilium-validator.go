package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"strings"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// Use this tool to validate if your cluster is ready to migrate from Azure Network Policy Manager (NPM) to Cilium.
// go run azure-npm-to-cilium-validator.go --kubeconfig ~/.kube/config

func main() {
	// Parse the kubeconfig flag
	kubeconfig := flag.String("kubeconfig", "~/.kube/config", "absolute path to the kubeconfig file")
	flag.Parse()

	// Build the Kubernetes client config
	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		log.Fatalf("Error building kubeconfig: %v", err)
	}

	// Create a Kubernetes client
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatalf("Error creating Kubernetes client: %v", err)
	}

	// Get namespaces
	namespaces, err := clientset.CoreV1().Namespaces().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		log.Fatalf("Error getting namespaces: %v\n", err)
	}

	// Copy namespaces.Items into a slice of pointers
	namespacePointers := make([]*corev1.Namespace, len(namespaces.Items))
	for i := range namespaces.Items {
		namespacePointers[i] = &namespaces.Items[i]
	}

	// Store network policies and services in maps
	policiesByNamespace := make(map[string][]*networkingv1.NetworkPolicy)
	servicesByNamespace := make(map[string][]*corev1.Service)

	// Iterate over namespaces and store policies/services
	for _, ns := range namespacePointers {
		// Get network policies
		networkPolicies, err := clientset.NetworkingV1().NetworkPolicies(ns.Name).List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			fmt.Printf("Error getting network policies in namespace %s: %v\n", ns.Name, err)
			continue
		}
		policiesByNamespace[ns.Name] = make([]*networkingv1.NetworkPolicy, len(networkPolicies.Items))
		for i := range networkPolicies.Items {
			policiesByNamespace[ns.Name][i] = &networkPolicies.Items[i]
		}

		// Get services
		services, err := clientset.CoreV1().Services(ns.Name).List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			fmt.Printf("Error getting services in namespace %s: %v\n", ns.Name, err)
			continue
		}
		servicesByNamespace[ns.Name] = make([]*corev1.Service, len(services.Items))
		for i := range services.Items {
			servicesByNamespace[ns.Name][i] = &services.Items[i]
		}
	}

	// Print the migration summary
	printMigrationSummary(namespaces, policiesByNamespace, servicesByNamespace)
}

func getEndportNetworkPolicies(policiesByNamespace map[string][]*networkingv1.NetworkPolicy) (ingressPoliciesWithEndport, egressPoliciesWithEndport []string) {
	for namespace, policies := range policiesByNamespace {
		for _, policy := range policies {
			// Check the ingress field for endport
			for _, ingress := range policy.Spec.Ingress {
				foundEndPort := checkEndportInPolicyRules(ingress.Ports)
				if foundEndPort {
					ingressPoliciesWithEndport = append(ingressPoliciesWithEndport, fmt.Sprintf("%s/%s", namespace, policy.Name))
					break
				}
			}
			for _, egress := range policy.Spec.Egress {
				foundEndPort := checkEndportInPolicyRules(egress.Ports)
				if foundEndPort {
					egressPoliciesWithEndport = append(egressPoliciesWithEndport, fmt.Sprintf("%s/%s", namespace, policy.Name))
					break
				}
			}
		}
	}
	return ingressPoliciesWithEndport, egressPoliciesWithEndport
}

func checkEndportInPolicyRules(ports []networkingv1.NetworkPolicyPort) bool {
	for _, port := range ports {
		if port.EndPort != nil {
			return true
		}
	}
	return false
}

func getCIDRNetworkPolicies(policiesByNamespace map[string][]*networkingv1.NetworkPolicy) (ingressPoliciesWithCIDR, egressPoliciesWithCIDR []string) {
	for namespace, policies := range policiesByNamespace {
		for _, policy := range policies {
			// Check the ingress field for cidr
			for _, ingress := range policy.Spec.Ingress {
				foundCIDRIngress := checkCIDRInPolicyRules(ingress.From)
				if foundCIDRIngress {
					ingressPoliciesWithCIDR = append(ingressPoliciesWithCIDR, fmt.Sprintf("%s/%s", namespace, policy.Name))
					break
				}
			}
			// Check the egress field for cidr
			for _, egress := range policy.Spec.Egress {
				foundCIDREgress := checkCIDRInPolicyRules(egress.To)
				if foundCIDREgress {
					egressPoliciesWithCIDR = append(egressPoliciesWithCIDR, fmt.Sprintf("%s/%s", namespace, policy.Name))
					break
				}
			}
		}
	}
	return ingressPoliciesWithCIDR, egressPoliciesWithCIDR
}

// Check for CIDR in ingress or egress rules
func checkCIDRInPolicyRules(rules []networkingv1.NetworkPolicyPeer) bool {
	for _, rule := range rules {
		if rule.IPBlock != nil && rule.IPBlock.CIDR != "" {
			return true
		}
	}
	return false
}

func getEgressPolicies(policiesByNamespace map[string][]*networkingv1.NetworkPolicy) []string {
	var egressPolicies []string
	for namespace, policies := range policiesByNamespace {
		for _, policy := range policies {
			for _, policyType := range policy.Spec.PolicyTypes {
				// If the policy is an egress type and has no egress field it is an deny all flag it
				if policyType == networkingv1.PolicyTypeEgress && len(policy.Spec.Egress) == 0 {
					egressPolicies = append(egressPolicies, fmt.Sprintf("%s/%s", namespace, policy.Name))
					break
				}
			}
			for _, egress := range policy.Spec.Egress {
				// If the policy has a egress field thats not an egress allow all flag it
				if len(egress.To) > 0 || len(egress.Ports) > 0 {
					egressPolicies = append(egressPolicies, fmt.Sprintf("%s/%s", namespace, policy.Name))
					break
				}
			}
		}
	}
	return egressPolicies
}

func getExternalTrafficPolicyClusterServices(
	namespaces *corev1.NamespaceList,
	servicesByNamespace map[string][]*corev1.Service,
	policiesByNamespace map[string][]*networkingv1.NetworkPolicy,
) (unsafeRiskServices, unsafeNoSelectorServices []string) {
	var riskServices, noSelectorServices, safeServices []string

	for i := range namespaces.Items {
		namespace := &namespaces.Items[i]
		// Check if are there ingress policies in the namespace if not skip
		policyListAtNamespace := policiesByNamespace[namespace.Name]
		if !hasIngressPolicies(policyListAtNamespace) {
			continue
		}
		serviceListAtNamespace := servicesByNamespace[namespace.Name]

		// Check if are there services with externalTrafficPolicy=Cluster (applicable if Type=NodePort or Type=LoadBalancer)
		for _, service := range serviceListAtNamespace {
			if service.Spec.Type == corev1.ServiceTypeLoadBalancer || service.Spec.Type == corev1.ServiceTypeNodePort {
				externalTrafficPolicy := service.Spec.ExternalTrafficPolicy
				// If the service has externalTrafficPolicy is set to "Cluster" add it to the riskServices list (ExternalTrafficPolicy: "" defaults to Cluster)
				if externalTrafficPolicy != corev1.ServiceExternalTrafficPolicyTypeLocal {
					// Any service with externalTrafficPolicy=Cluster is at risk so need to elimate any services that are incorrectly flagged
					riskServices = append(riskServices, fmt.Sprintf("%s/%s", namespace.Name, service.Name))
					// If the service has no selector add it to the noSelectorServices list
					if service.Spec.Selector == nil {
						noSelectorServices = append(noSelectorServices, fmt.Sprintf("%s/%s", namespace.Name, service.Name))
					}
					// Check if are there services with selector that match the network policy
					if checkNoServiceRisk(service, policyListAtNamespace) {
						safeServices = append(safeServices, fmt.Sprintf("%s/%s", namespace.Name, service.Name))
					}
				}
			}
		}
	}

	// Remove all the safe services from the services at risk
	unsafeRiskServices = difference(riskServices, safeServices)
	// Remove all the safe services from the no selector services
	unsafeNoSelectorServices = difference(noSelectorServices, safeServices)
	return unsafeRiskServices, unsafeNoSelectorServices
}

func hasIngressPolicies(policies []*networkingv1.NetworkPolicy) bool {
	// Check if any policy is ingress (including allow all and deny all)
	for _, policy := range policies {
		for _, policyType := range policy.Spec.PolicyTypes {
			if policyType == networkingv1.PolicyTypeIngress {
				return true
			}
		}
	}
	return false
}

func checkNoServiceRisk(service *corev1.Service, policiesListAtNamespace []*networkingv1.NetworkPolicy) bool {
	for _, policy := range policiesListAtNamespace {
		// Skips deny all policies as they do not have any ingress rules
		for _, ingress := range policy.Spec.Ingress {
			// Check if there is an allow all ingress policy that matches labels the service is safe
			if len(ingress.From) == 0 && len(ingress.Ports) == 0 {
				// Check if there is an allow all ingress policy with empty selectors or matching service labels as the policy allows all services in the namespace
				if checkPolicyMatchServiceLabels(service.Spec.Selector, policy.Spec.PodSelector) {
					return true
				}
			}
			// If there are no ingress from but there are ports in the policy; check if the service is safe
			if len(ingress.From) == 0 && len(ingress.Ports) > 0 {
				// If the policy targets all pods (allow all) or only pods that are in the service selector, check if traffic is allowed to all the service's target ports
				if checkPolicyMatchServiceLabels(service.Spec.Selector, policy.Spec.PodSelector) && checkServiceTargetPortMatchPolicyPorts(service.Spec.Ports, ingress.Ports) {
					return true
				}
			}
		}
	}
	return false
}

func checkPolicyMatchServiceLabels(serviceLabels map[string]string, podSelector metav1.LabelSelector) bool {
	// Check if there is an allow all ingress policy with empty selectors if so the service is safe
	if len(podSelector.MatchLabels) == 0 && len(podSelector.MatchExpressions) == 0 {
		return true
	}

	// Return false if the policy has no labels or more labels than the service
	if len(podSelector.MatchLabels) == 0 || len(podSelector.MatchLabels) > len(serviceLabels) {
		return false
	}

	// Check for each policy label that that label is present in the service labels
	// Note: does not check matchExpressions
	for policyKey, policyValue := range podSelector.MatchLabels {
		matchedPolicyLabelToServiceLabel := false
		for serviceKey, serviceValue := range serviceLabels {
			if policyKey == serviceKey && policyValue == serviceValue {
				matchedPolicyLabelToServiceLabel = true
				break
			}
		}
		if !matchedPolicyLabelToServiceLabel {
			return false
		}
	}
	return true
}

func checkServiceTargetPortMatchPolicyPorts(servicePorts []corev1.ServicePort, policyPorts []networkingv1.NetworkPolicyPort) bool {
	// If the service has no ports then it is at risk
	if len(servicePorts) == 0 {
		return false
	}

	for _, servicePort := range servicePorts {
		// If the target port is a string then it is a named port and service is at risk
		if servicePort.TargetPort.Type == intstr.String {
			return false
		}

		// If the target port is 0 then it is at risk as Cilium treats port 0 in a special way
		if servicePort.TargetPort.IntValue() == 0 {
			return false
		}

		// Check if all the services target ports are in the policies ingress ports
		matchedserviceTargetPortToPolicyPort := false
		for _, policyPort := range policyPorts {
			// Check if the policys port and protocol exists
			if policyPort.Port == nil && policyPort.Protocol == nil {
				return false
			}
			// If the policy only has a protocol check the protocol against the service
			// Note: if a network policy on NPM just targets a protocol it will allow all traffic with containing that protocol (ignoring the port)
			// Note: an empty protocols default to "TCP" for both policies and services
			if policyPort.Port == nil && policyPort.Protocol != nil {
				if string(servicePort.Protocol) == string(*policyPort.Protocol) {
					matchedserviceTargetPortToPolicyPort = true
					break
				}
				continue
			}
			// If the port is a string then it is a named port and it cant be evaluated
			if policyPort.Port.Type == intstr.String {
				continue
			}
			// If the target port is 0 then it is at risk as Cilium treats port 0 in a special way
			if int(policyPort.Port.IntVal) == 0 {
				return false
			}
			// Check if the service target port and protocol matches the policy port and protocol
			// Note: that the service target port will never been undefined as it defaults to port which is a required field when Ports is defined
			// Note: an empty protocols default to "TCP" for both policies and services
			if servicePort.TargetPort.IntValue() == int(policyPort.Port.IntVal) && string(servicePort.Protocol) == string(*policyPort.Protocol) {
				matchedserviceTargetPortToPolicyPort = true
				break
			}
		}
		if !matchedserviceTargetPortToPolicyPort {
			return false
		}
	}
	return true
}

func difference(slice1, slice2 []string) []string {
	m := make(map[string]struct{})
	for _, s := range slice2 {
		m[s] = struct{}{}
	}
	var diff []string
	for _, s := range slice1 {
		if _, ok := m[s]; !ok {
			diff = append(diff, s)
		}
	}
	return diff
}

func printMigrationSummary(namespaces *corev1.NamespaceList, policiesByNamespace map[string][]*networkingv1.NetworkPolicy, servicesByNamespace map[string][]*corev1.Service) {
	fmt.Println("Migration Summary:")
	fmt.Println("+------------------------------+-------------------------------+")
	fmt.Printf("%-30s | %-30s \n", "Breaking Change", "No Policy Changes Needed")
	fmt.Println("+------------------------------+-------------------------------+")

	// Get the endports of the network policies
	ingressEndportNetworkPolicy, egressEndportNetworkPolicy := getEndportNetworkPolicies(policiesByNamespace)

	// Print the network policies with endport
	printPoliciesWithEndport(ingressEndportNetworkPolicy, egressEndportNetworkPolicy)

	fmt.Println("+------------------------------+-------------------------------+")

	// Get the cidr of the network policies
	ingressPoliciesWithCIDR, egressPoliciesWithCIDR := getCIDRNetworkPolicies(policiesByNamespace)

	// Print the network policies with CIDR
	printPoliciesWithCIDR(ingressPoliciesWithCIDR, egressPoliciesWithCIDR)

	fmt.Println("+------------------------------+-------------------------------+")

	// Get the egress of the network policies
	egressPolicies := getEgressPolicies(policiesByNamespace)

	// Print the network policies with egress
	printEgressPolicies(egressPolicies)

	fmt.Println("+------------------------------+-------------------------------+")

	// Get services that have externalTrafficPolicy!=Local
	unsafeRiskServices, unsafeNoSelectorServices := getExternalTrafficPolicyClusterServices(namespaces, servicesByNamespace, policiesByNamespace)

	// Print the services that are at risk
	printUnsafeServices(unsafeRiskServices, unsafeNoSelectorServices)

	fmt.Println("+------------------------------+-------------------------------+")
	if len(ingressEndportNetworkPolicy) > 0 || len(egressEndportNetworkPolicy) > 0 ||
		len(ingressPoliciesWithCIDR) > 0 || len(egressPoliciesWithCIDR) > 0 ||
		len(egressPolicies) > 0 ||
		len(unsafeRiskServices) > 0 || len(unsafeNoSelectorServices) > 0 {
		fmt.Println("\033[31m✘ Review above issues before migration.\033[0m")
		fmt.Println("Please see \033[32maka.ms/azurenpmtocilium\033[0m for instructions on how to evaluate/assess the above warnings marked by ❌.")
		fmt.Println("NOTE: rerun this script if any modifications (create/update/delete) are made to services or policies.")
	} else {
		fmt.Println("\033[32m✔ Safe to migrate this cluster.\033[0m")
		fmt.Println("For more details please see \033[32maka.ms/azurenpmtocilium\033[0m.")
	}
}

func printPoliciesWithEndport(ingressEndportNetworkPolicy, egressEndportNetworkPolicy []string) {
	if len(ingressEndportNetworkPolicy) == 0 && len(egressEndportNetworkPolicy) == 0 {
		fmt.Printf("%-30s | %-30s \n", "NetworkPolicy with endport", "✅")
	} else {
		fmt.Printf("%-30s | %-30s \n", "NetworkPolicy with endport", "❌")
		fmt.Println("Policies affected:")
		for _, policy := range ingressEndportNetworkPolicy {
			policyNamespace := strings.Split(policy, "/")[0]
			policyName := strings.Split(policy, "/")[1]
			fmt.Printf("❌ Found NetworkPolicy: \033[31m%s\033[0m with ingress endPort field in namespace: \033[31m%s\033[0m\n", policyName, policyNamespace)
		}
		for _, policy := range egressEndportNetworkPolicy {
			policyNamespace := strings.Split(policy, "/")[0]
			policyName := strings.Split(policy, "/")[1]
			fmt.Printf("❌ Found NetworkPolicy: \033[31m%s\033[0m with engress endPort field in namespace: \033[31m%s\033[0m\n", policyName, policyNamespace)
		}
	}
}

func printPoliciesWithCIDR(ingressPoliciesWithCIDR, egressPoliciesWithCIDR []string) {
	if len(ingressPoliciesWithCIDR) == 0 && len(egressPoliciesWithCIDR) == 0 {
		fmt.Printf("%-30s | %-30s \n", "NetworkPolicy with CIDR", "✅")
	} else {
		fmt.Printf("%-30s | %-30s \n", "NetworkPolicy with CIDR", "❌")
		fmt.Println("Policies affected:")
		for _, policy := range ingressPoliciesWithCIDR {
			policyNamespace := strings.Split(policy, "/")[0]
			policyName := strings.Split(policy, "/")[1]
			fmt.Printf("❌ Found NetworkPolicy: \033[31m%s\033[0m with ingress CIDR field in namespace: \033[31m%s\033[0m\n", policyName, policyNamespace)
		}
		for _, policy := range egressPoliciesWithCIDR {
			policyNamespace := strings.Split(policy, "/")[0]
			policyName := strings.Split(policy, "/")[1]
			fmt.Printf("❌ Found NetworkPolicy: \033[31m%s\033[0m with egress CIDR field in namespace: \033[31m%s\033[0m\n", policyName, policyNamespace)
		}
	}
}

func printEgressPolicies(egressPolicies []string) {
	if len(egressPolicies) == 0 {
		fmt.Printf("%-30s | %-30s \n", "NetworkPolicy with egress", "✅")
	} else {
		fmt.Printf("%-30s | %-30s \n", "NetworkPolicy with egress", "❌")
		fmt.Printf("%-30s | %-30s \n", "(Not allow all egress)", "")
		fmt.Println("Policies affected:")
		for _, policy := range egressPolicies {
			policyNamespace := strings.Split(policy, "/")[0]
			policyName := strings.Split(policy, "/")[1]
			fmt.Printf("❌ Found NetworkPolicy: \033[31m%s\033[0m with egress field (non-allow all) in namespace: \033[31m%s\033[0m\n", policyName, policyNamespace)
		}
	}
}

func printUnsafeServices(unsafeRiskServices, unsafeNoSelectorServices []string) {
	// If there is no unsafe services and services with no selectors then migration is safe for services with extranalTrafficPolicy=Cluster
	if len(unsafeRiskServices) == 0 {
		fmt.Printf("%-30s | %-30s \n", "Disruption for some", "✅")
		fmt.Printf("%-30s | %-30s \n", "Services with", "")
		fmt.Printf("%-30s | %-30s \n", "externalTrafficPolicy=Cluster", "")
	} else {
		// Remove all no selector services from unsafe services to prevent repeating the same flagged service
		unsafeRiskServices = difference(unsafeRiskServices, unsafeNoSelectorServices)
		fmt.Printf("%-30s | %-30s \n", "Disruption for some", "❌")
		fmt.Printf("%-30s | %-30s \n", "Services with", "")
		fmt.Printf("%-30s | %-30s \n", "externalTrafficPolicy=Cluster", "")
		fmt.Println("Services affected:")
		// If there are any no selector services or unsafe services then print them as they could be impacted by migration
		if len(unsafeNoSelectorServices) > 0 {
			for _, service := range unsafeNoSelectorServices {
				serviceName := strings.Split(service, "/")[1]
				serviceNamespace := strings.Split(service, "/")[0]
				fmt.Printf("❌ Found Service: \033[31m%s\033[0m without selectors in namespace: \033[31m%s\033[0m\n", serviceName, serviceNamespace)
			}
		}
		if len(unsafeRiskServices) > 0 {
			for _, service := range unsafeRiskServices {
				serviceName := strings.Split(service, "/")[1]
				serviceNamespace := strings.Split(service, "/")[0]
				fmt.Printf("❌ Found Service: \033[31m%s\033[0m with selectors in namespace: \033[31m%s\033[0m\n", serviceName, serviceNamespace)
			}
		}
		fmt.Println("Manual investigation is required to evaluate if ingress is allowed to the service's backend Pods.")
		fmt.Println("Please evaluate if these services would be impacted by migration.")
	}
}
