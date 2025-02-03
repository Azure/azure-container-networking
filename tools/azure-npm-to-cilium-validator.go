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
		fmt.Printf("Writing policies and services for namespace %s...\n", ns.Name)

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
	fmt.Printf("%-30s | %-30s \n", "Breaking Change", "No Impact / Safe to Migrate")
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
	networkPolicyWithEndport := false
	for namespace, policies := range policiesByNamespace {
		for _, policy := range policies {
			foundEndPort := false
			for _, egress := range policy.Spec.Egress {
				for _, port := range egress.Ports {
					if port.EndPort != nil {
						foundEndPort = true
						if !networkPolicyWithEndport {
							fmt.Printf("%-30s | %-30s \n", "NetworkPolicy with endPort", "❌")
							fmt.Println("Policies affected:")
							networkPolicyWithEndport = true
						}
						fmt.Printf("❌ Found NetworkPolicy: \033[31m%s\033[0m with endPort field in namespace: \033[31m%s\033[0m\n", policy.Name, namespace)
						// Exit egress.port loop
						break
					}
				}
				if foundEndPort {
					// Exit egress loop
					break
				}
			}
		}
	}
	// Print no impact if no network policy has endport
	if !networkPolicyWithEndport {
		fmt.Printf("%-30s | %-30s \n", "NetworkPolicy with endPort", "✅")
		return false
	}
	return true
}

func checkCIDRNetworkPolicies(policiesByNamespace map[string][]networkingv1.NetworkPolicy) bool {
	networkPolicyWithCIDR := false
	for namespace, policies := range policiesByNamespace {
		for _, policy := range policies {
			foundCIDRIngress := false
			foundCIDREgress := false
			// Check the ingress field for cidr
			for _, ingress := range policy.Spec.Ingress {
				for _, from := range ingress.From {
					if from.IPBlock != nil {
						if from.IPBlock.CIDR != "" {
							foundCIDRIngress = true
							// Print the network policy if it has an ingress cidr
							if !networkPolicyWithCIDR {
								fmt.Printf("%-30s | %-30s \n", "NetworkPolicy with cidr", "❌")
								fmt.Println("Policies affected:")
								networkPolicyWithCIDR = true
							}
							fmt.Printf("❌ Found NetworkPolicy: \033[31m%s\033[0m with ingress cidr field in namespace: \033[31m%s\033[0m\n", policy.Name, namespace)

							// Exit ingress.from.ipBlock loop
							break
						}
					}
				}
				if foundCIDRIngress {
					// Exit ingress loop
					break
				}
			}
			// Check the egress field for cidr
			for _, egress := range policy.Spec.Egress {
				for _, to := range egress.To {
					if to.IPBlock != nil {
						if to.IPBlock.CIDR != "" {
							foundCIDREgress = true
							// Print the network policy if it has an egress cidr
							if !networkPolicyWithCIDR {
								fmt.Printf("%-30s | %-30s \n", "NetworkPolicy with cidr", "❌")
								fmt.Println("Policies affected:")
								networkPolicyWithCIDR = true
							}
							fmt.Printf("❌ Found NetworkPolicy: \033[31m%s\033[0m with egress cidr field in namespace: \033[31m%s\033[0m\n", policy.Name, namespace)

							// Exit egress.to.ipBlock loop
							break
						}
					}
				}
				if foundCIDREgress {
					// Exit egress loop
					break
				}
			}
		}
	}
	// Print no impact if no network policy has cidr
	if !networkPolicyWithCIDR {
		fmt.Printf("%-30s | %-30s \n", "NetworkPolicy with cidr", "✅")
		return false
	}
	return true
}

func checkForEgressPolicies(policiesByNamespace map[string][]networkingv1.NetworkPolicy) bool {
	networkPolicyWithEgress := false
	for namespace, policies := range policiesByNamespace {
		for _, policy := range policies {
			for _, egress := range policy.Spec.Egress {
				// If the policy has a egress field thats not an egress allow all flag it
				if len(egress.To) > 0 || len(egress.Ports) > 0 {
					if !networkPolicyWithEgress {
						fmt.Printf("%-30s | %-30s \n", "NetworkPolicy with egress", "❌")
						fmt.Printf("%-30s | %-30s \n", "(Not allow all egress)", "")
						fmt.Println("Policies affected:")
						networkPolicyWithEgress = true
					}
					fmt.Printf("❌ Found NetworkPolicy: \033[31m%s\033[0m with egress field (non-allow all) in namespace: \033[31m%s\033[0m\n", policy.Name, namespace)

					// Exit egress loop
					break
				}
			}
		}
	}
	if !networkPolicyWithEgress {
		fmt.Printf("%-30s | %-30s \n", "NetworkPolicy with egress", "✅")
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
						safeServices = checkServiceRisk(service, namespace.Name, policiesByNamespace[namespace.Name], safeServices)
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
			if len(ingress.From) > 0 {
				return true
			}
		}
	}
	return false
}

func checkServiceRisk(service corev1.Service, namespace string, policiesListAtNamespace []networkingv1.NetworkPolicy, safeServices []string) []string {
	for _, policy := range policiesListAtNamespace {
		for _, ingress := range policy.Spec.Ingress {
			// Check if there is an allow all ingress policy that matches labels the service is safe
			if len(ingress.From) == 0 && len(ingress.Ports) == 0 {
				// Check if there is an allow all ingress policy with empty selectors return true as the policy allows all services in the namespace
				if len(policy.Spec.PodSelector.MatchLabels) == 0 {
					fmt.Printf("found an allow all ingress policy: %s with empty selectors so service %s in the namespace %s is safe\n", policy.Name, service.Name, namespace)
					safeServices = append(safeServices, fmt.Sprintf("%s/%s", namespace, service.Name))
					return safeServices
				}
				// Check if there is an allow all ingress policy that matches the service labels
				if checkPolicyMatchServiceLabels(service.Spec.Selector, policy.Spec.PodSelector.MatchLabels) {
					fmt.Printf("found an allow all ingress policy: %s with matching selectors so service %s in the namespace %s is safe\n", policy.Name, service.Name, namespace)
					safeServices = append(safeServices, fmt.Sprintf("%s/%s", namespace, service.Name))
					return safeServices
				}
			}
			// If there are no ingress from but there are ports in the policy; check if the service is safe
			if len(ingress.From) == 0 && len(ingress.Ports) > 0 {
				// If the policy targets all pods (allow all) or only pods that are in the service selector, check if traffic is allowed to all the service's target ports
				if len(policy.Spec.PodSelector.MatchLabels) == 0 || checkPolicyMatchServiceLabels(service.Spec.Selector, policy.Spec.PodSelector.MatchLabels) {
					if checkServiceTargetPortMatchPolicyPorts(service.Spec.Ports, ingress.Ports) {
						safeServices = append(safeServices, fmt.Sprintf("%s/%s", namespace, service.Name))
						return safeServices
					}
				}
			}
		}
	}
	return safeServices
}

func checkPolicyMatchServiceLabels(serviceLabels, policyLabels map[string]string) bool {
	// Return false if the policy has more labels than the service
	if len(policyLabels) > len(serviceLabels) {
		return false
	}

	// Check for each policy label that that label is present in the service labels
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
