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
	// Remove timestamp from log
	log.SetFlags(0)

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

	// Store network policies and services in maps
	policiesByNamespace := make(map[string][]networkingv1.NetworkPolicy)
	servicesByNamespace := make(map[string][]corev1.Service)

	// Copy namespaces.Items into a slice of pointers
	namespacePointers := make([]*corev1.Namespace, len(namespaces.Items))
	for i := range namespaces.Items {
		namespacePointers[i] = &namespaces.Items[i]
	}

	// Iterate over namespaces and store policies/services
	for _, ns := range namespacePointers {
		// Get network policies
		networkPolicies, err := clientset.NetworkingV1().NetworkPolicies(ns.Name).List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			fmt.Printf("Error getting network policies in namespace %s: %v\n", ns.Name, err)
			continue
		}
		policiesByNamespace[ns.Name] = networkPolicies.Items

		// Get services
		services, err := clientset.CoreV1().Services(ns.Name).List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			fmt.Printf("Error getting services in namespace %s: %v\n", ns.Name, err)
			continue
		}
		servicesByNamespace[ns.Name] = services.Items
	}

	fmt.Println("Migration Summary:")
	fmt.Println("+------------------------------+-------------------------------+")
	fmt.Printf("%-30s | %-30s \n", "Breaking Change", "No Policy Changes Needed")
	fmt.Println("+------------------------------+-------------------------------+")

	// Check the endports of the network policies
	foundEnportNetworkPolicy := checkEndportNetworkPolicies(policiesByNamespace)

	fmt.Println("+------------------------------+-------------------------------+")

	// Check the cidr of the network policies
	foundCIDRNetworkPolicy := checkCIDRNetworkPolicies(policiesByNamespace)

	fmt.Println("+------------------------------+-------------------------------+")

	// Check the egress of the network policies
	foundEgressPolicy := checkForEgressPolicies(policiesByNamespace)

	fmt.Println("+------------------------------+-------------------------------+")

	// Check services that have externalTrafficPolicy!=Local
	foundServiceDispruption := checkExternalTrafficPolicyServices(namespaces, servicesByNamespace, policiesByNamespace)

	fmt.Println("+------------------------------+-------------------------------+")
	if foundEnportNetworkPolicy || foundCIDRNetworkPolicy || foundEgressPolicy || foundServiceDispruption {
		fmt.Println("\033[31m✘ Review above issues before migration.\033[0m")
		fmt.Println("Please see \033[32maka.ms/azurenpmtocilium\033[0m for instructions on how to evaluate/assess the above warnings marked by ❌.")
		fmt.Println("NOTE: rerun this script if any modifications (create/update/delete) are made to services or policies.")
	} else {
		fmt.Println("\033[32m✔ Safe to migrate this cluster.\033[0m")
		fmt.Println("For more details please see \033[32maka.ms/azurenpmtocilium\033[0m.")
	}
}

func checkEndportNetworkPolicies(policiesByNamespace map[string][]networkingv1.NetworkPolicy) bool {
	foundNetworkPolicyWithEndport := false
	for namespace, policies := range policiesByNamespace {
		for _, policy := range policies {
			// Check the ingress field for endport
			for _, ingress := range policy.Spec.Ingress {
				foundEndPort := checkEndportInPolicyRules(ingress.Ports, policy.Name, namespace, "ingress", foundNetworkPolicyWithEndport)
				if foundEndPort {
					foundNetworkPolicyWithEndport = true
					break
				}
			}
			for _, egress := range policy.Spec.Egress {
				foundEndPort := checkEndportInPolicyRules(egress.Ports, policy.Name, namespace, "egress", foundNetworkPolicyWithEndport)
				if foundEndPort {
					foundNetworkPolicyWithEndport = true
					break
				}
			}
		}
	}
	// Print no impact if no network policy has endport
	if !foundNetworkPolicyWithEndport {
		log.Printf("%-30s | %-30s \n", "NetworkPolicy with endPort", "✅")
		return false
	}
	return true
}

func checkEndportInPolicyRules(ports []networkingv1.NetworkPolicyPort, policyName, namespace string, direction string, foundNetworkPolicyWithEndport bool) bool {
	foundEndPort := false
	for _, port := range ports {
		if port.EndPort != nil {
			foundEndPort = true
			if !foundNetworkPolicyWithEndport {
				log.Printf("%-30s | %-30s \n", "NetworkPolicy with endPort", "❌")
				log.Println("Policies affected:")
			}
			log.Printf("❌ Found NetworkPolicy: \033[31m%s\033[0m with %s endPort field in namespace: \033[31m%s\033[0m\n", policyName, direction, namespace)
			break
		}
	}
	return foundEndPort
}

func checkCIDRNetworkPolicies(policiesByNamespace map[string][]networkingv1.NetworkPolicy) bool {
	foundNetworkPolicyWithCIDR := false
	for namespace, policies := range policiesByNamespace {
		for _, policy := range policies {
			// Check the ingress field for cidr
			for _, ingress := range policy.Spec.Ingress {
				foundCIDRIngress := checkCIDRInPolicyRules(ingress.From, policy.Name, namespace, "ingress", foundNetworkPolicyWithCIDR)
				if foundCIDRIngress {
					foundNetworkPolicyWithCIDR = true
					break
				}
			}
			// Check the egress field for cidr
			for _, egress := range policy.Spec.Egress {
				foundCIDREgress := checkCIDRInPolicyRules(egress.To, policy.Name, namespace, "egress", foundNetworkPolicyWithCIDR)
				if foundCIDREgress {
					foundNetworkPolicyWithCIDR = true
					break
				}
			}
		}
	}
	// Print no impact if no network policy has cidr
	if !foundNetworkPolicyWithCIDR {
		log.Printf("%-30s | %-30s \n", "NetworkPolicy with cidr", "✅")
		return false
	}
	return true
}

// Check for CIDR in ingress or egress rules
func checkCIDRInPolicyRules(rules []networkingv1.NetworkPolicyPeer, policyName, namespace string, direction string, foundNetworkPolicyWithCIDR bool) bool {
	foundCIDR := false
	for _, rule := range rules {
		if rule.IPBlock != nil && rule.IPBlock.CIDR != "" {
			foundCIDR = true
			if !foundNetworkPolicyWithCIDR {
				log.Printf("%-30s | %-30s \n", "NetworkPolicy with cidr", "❌")
				log.Println("Policies affected:")
			}
			log.Printf("❌ Found NetworkPolicy: \033[31m%s\033[0m with %s cidr field in namespace: \033[31m%s\033[0m\n", policyName, direction, namespace)
			break
		}
	}
	return foundCIDR
}

func checkForEgressPolicies(policiesByNamespace map[string][]networkingv1.NetworkPolicy) bool {
	foundNetworkPolicyWithEgress := false
	for namespace, policies := range policiesByNamespace {
		for _, policy := range policies {
			for _, egress := range policy.Spec.Egress {
				// If the policy has a egress field thats not an egress allow all flag it
				if len(egress.To) > 0 || len(egress.Ports) > 0 {
					if !foundNetworkPolicyWithEgress {
						log.Printf("%-30s | %-30s \n", "NetworkPolicy with egress", "❌")
						log.Printf("%-30s | %-30s \n", "(Not allow all egress)", "")
						log.Println("Policies affected:")
						foundNetworkPolicyWithEgress = true
					}
					log.Printf("❌ Found NetworkPolicy: \033[31m%s\033[0m with egress field (non-allow all) in namespace: \033[31m%s\033[0m\n", policy.Name, namespace)
					break
				}
			}
		}
	}
	if !foundNetworkPolicyWithEgress {
		log.Printf("%-30s | %-30s \n", "NetworkPolicy with egress", "✅")
		return false
	}
	return true
}

func checkExternalTrafficPolicyServices(namespaces *corev1.NamespaceList, servicesByNamespace map[string][]corev1.Service, policiesByNamespace map[string][]networkingv1.NetworkPolicy) bool {
	var servicesAtRisk, noSelectorServices, safeServices []string

	for _, namespace := range namespaces.Items {
		// Check if are there ingress policies in the namespace if not skip
		if !hasIngressPolicies(policiesByNamespace[namespace.Name]) {
			continue
		}
		serviceListAtNamespace := servicesByNamespace[namespace.Name]

		// Check if are there services with externalTrafficPolicy=Cluster (applicable if Type=NodePort or Type=LoadBalancer)
		for _, service := range serviceListAtNamespace {
			if service.Spec.Type == corev1.ServiceTypeLoadBalancer || service.Spec.Type == corev1.ServiceTypeNodePort {
				externalTrafficPolicy := service.Spec.ExternalTrafficPolicy
				// If the service has externalTrafficPolicy is set to "Cluster" add it to the servicesAtRisk list (ExternalTrafficPolicy: "" defaults to Cluster)
				if externalTrafficPolicy != corev1.ServiceExternalTrafficPolicyTypeLocal {
					// Any service with externalTrafficPolicy=Cluster is at risk so need to elimate any services that are incorrectly flagged
					servicesAtRisk = append(servicesAtRisk, fmt.Sprintf("%s/%s", namespace.Name, service.Name))
					// If the service has no selector add it to the noSelectorServices list
					if service.Spec.Selector == nil {
						noSelectorServices = append(noSelectorServices, fmt.Sprintf("%s/%s", namespace.Name, service.Name))
					} else {
						// Check if are there services with selector that match the network policy
						if checkServiceRisk(service, namespace.Name, policiesByNamespace[namespace.Name]) {
							safeServices = append(safeServices, fmt.Sprintf("%s/%s", namespace.Name, service.Name))
						}
					}
				}
			}
		}
	}

	// Get the services that are at risk but not in the safe services or no selector services lists
	unsafeServices := difference(servicesAtRisk, safeServices, noSelectorServices)

	// If there is no unsafe services then migration is safe for services with extranalTrafficPolicy=Cluster
	if len(unsafeServices) == 0 {
		fmt.Printf("%-30s | %-30s \n", "Disruption for some", "✅")
		fmt.Printf("%-30s | %-30s \n", "Services with", "")
		fmt.Printf("%-30s | %-30s \n", "externalTrafficPolicy=Cluster", "")
		return false
	} else {
		fmt.Printf("%-30s | %-30s \n", "Disruption for some", "❌")
		fmt.Printf("%-30s | %-30s \n", "Services with", "")
		fmt.Printf("%-30s | %-30s \n", "externalTrafficPolicy=Cluster", "")
		fmt.Println("Services affected:")
		// If there are any no selector services or unsafe services then print them as they could be impacted by migration
		if len(noSelectorServices) > 0 {
			for _, service := range noSelectorServices {
				serviceName := strings.Split(service, "/")[1]
				serviceNamespace := strings.Split(service, "/")[0]
				fmt.Printf("❌ Found Service: \033[31m%s\033[0m without selectors in namespace: \033[31m%s\033[0m\n", serviceName, serviceNamespace)
			}
		}
		if len(unsafeServices) > 0 {
			for _, service := range unsafeServices {
				serviceName := strings.Split(service, "/")[1]
				serviceNamespace := strings.Split(service, "/")[0]
				fmt.Printf("❌ Found Service: \033[31m%s\033[0m with selectors in namespace: \033[31m%s\033[0m\n", serviceName, serviceNamespace)
			}
		}
		fmt.Println("Manual investigation is required to evaluate if ingress is allowed to the service's backend Pods.")
		fmt.Println("Please evaluate if these services would be impacted by migration.")
		return true
	}

}

func hasIngressPolicies(policies []networkingv1.NetworkPolicy) bool {
	// Check if any policy is ingress
	for _, policy := range policies {
		for _, ingress := range policy.Spec.Ingress {
			if len(ingress.From) > 0 || len(ingress.Ports) > 0 {
				return true
			}
		}
	}
	return false
}

func checkServiceRisk(service corev1.Service, namespace string, policiesListAtNamespace []networkingv1.NetworkPolicy) bool {
	for _, policy := range policiesListAtNamespace {
		for _, ingress := range policy.Spec.Ingress {
			// Check if there is an allow all ingress policy that matches labels the service is safe
			if len(ingress.From) == 0 && len(ingress.Ports) == 0 {
				// Check if there is an allow all ingress policy with empty selectors return true as the policy allows all services in the namespace
				if checkPolicySelectorsAreEmpty(policy.Spec.PodSelector) {
					fmt.Printf("found an allow all ingress policy: %s with empty selectors so service %s in the namespace %s is safe\n", policy.Name, service.Name, namespace)
					return true
				}
				// Check if there is an allow all ingress policy that matches the service labels
				if checkPolicyMatchServiceLabels(service.Spec.Selector, policy.Spec.PodSelector.MatchLabels) {
					// TODO add this to above logic and check in one if statement after i am done printing the logs
					fmt.Printf("found an allow all ingress policy: %s with matching selectors so service %s in the namespace %s is safe\n", policy.Name, service.Name, namespace)
					return true
				}
			}
			// If there are no ingress from but there are ports in the policy; check if the service is safe
			if len(ingress.From) == 0 && len(ingress.Ports) > 0 {
				// If the policy targets all pods (allow all) or only pods that are in the service selector, check if traffic is allowed to all the service's target ports
				if checkPolicySelectorsAreEmpty(policy.Spec.PodSelector) || checkPolicyMatchServiceLabels(service.Spec.Selector, policy.Spec.PodSelector.MatchLabels) {
					if checkServiceTargetPortMatchPolicyPorts(service.Spec.Ports, ingress.Ports) {
						fmt.Printf("found an ingress port policy: %s with matching selectors and target ports so service %s in the namespace %s is safe\n", policy.Name, service.Name, namespace)
						return true
					}
				}
			}
		}
	}
	return false
}

func checkPolicySelectorsAreEmpty(podSelector metav1.LabelSelector) bool {
	return len(podSelector.MatchLabels) == 0 && len(podSelector.MatchExpressions) == 0
}

func checkPolicyMatchServiceLabels(serviceLabels, policyLabels map[string]string) bool {
	// Return false if the policy has more labels than the service
	if len(policyLabels) > len(serviceLabels) {
		return false
	}

	// Check for each policy label that that label is present in the service labels
	// Note does not check matchExpressions
	for policyKey, policyValue := range policyLabels {
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
	ingressPorts := []string{}
	for _, port := range policyPorts {
		ingressPorts = append(ingressPorts, fmt.Sprintf("%d/%s", port.Port.IntVal, string(*port.Protocol)))
	}

	// Check if all the services target ports are in the policies ingress ports
	for _, port := range servicePorts {
		// If the target port is a string then it is a named port and service is at risk
		if port.TargetPort.Type == intstr.String {
			return false
		}
		servicePort := fmt.Sprintf("%d/%s", port.TargetPort.IntValue(), port.Protocol)
		if !contains(ingressPorts, servicePort) {
			return false
		}
	}
	return true
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func difference(slice1, slice2, slice3 []string) []string {
	m := make(map[string]bool)
	for _, s := range slice2 {
		m[s] = true
	}
	for _, s := range slice3 {
		m[s] = true
	}
	var diff []string
	for _, s := range slice1 {
		if !m[s] {
			diff = append(diff, s)
		}
	}
	return diff
}
