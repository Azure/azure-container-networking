package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/Azure/azure-container-networking/npm/metrics"
	"github.com/olekukonko/tablewriter"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
)

// Note: The operationID is set to a high number so it doesn't conflict with other telemetry
const scriptMetricOperationID = 10000

// Use this tool to validate if your cluster is ready to migrate from Azure Network Policy Manager (NPM) to Cilium.
func main() {
	// Parse the kubeconfig flag
	kubeconfig := flag.String("kubeconfig", "~/.kube/config", "absolute path to the kubeconfig file")
	detailedMigrationSummary := flag.Bool("detailed-migration-summary", false, "display flagged network polices/services and total cluster resource count")
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
	podsByNamespace := make(map[string][]*corev1.Pod)

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

		// Get pods
		pods, err := clientset.CoreV1().Pods(ns.Name).List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			fmt.Printf("Error getting pods in namespace %s: %v\n", ns.Name, err)
			continue
		}
		podsByNamespace[ns.Name] = make([]*corev1.Pod, len(pods.Items))
		for i := range pods.Items {
			podsByNamespace[ns.Name][i] = &pods.Items[i]
		}
	}

	// Create telemetry handle
	// Note: npmVersionNum and imageVersion telemetry is not needed for this tool so they are set to abitrary values
	err = metrics.CreateTelemetryHandle(0, "NPM-script-v0.0.1", "014c22bd-4107-459e-8475-67909e96edcb")

	if err != nil {
		klog.Infof("CreateTelemetryHandle failed with error %v. AITelemetry is not initialized.", err)
	}

	// Print the migration summary
	printMigrationSummary(detailedMigrationSummary, namespaces, policiesByNamespace, servicesByNamespace, podsByNamespace)
}

func printMigrationSummary(
	detailedMigrationSummary *bool,
	namespaces *corev1.NamespaceList,
	policiesByNamespace map[string][]*networkingv1.NetworkPolicy,
	servicesByNamespace map[string][]*corev1.Service,
	podsByNamespace map[string][]*corev1.Pod,
) {
	// Get the network policies with endports
	ingressEndportNetworkPolicy, egressEndportNetworkPolicy := getEndportNetworkPolicies(policiesByNamespace)

	// Send endPort telemetry
	metrics.SendLog(scriptMetricOperationID, fmt.Sprintf("[migration script] Found %d network policies with endPort", len(ingressEndportNetworkPolicy)+len(egressEndportNetworkPolicy)), metrics.DonotPrint)

	// Get the network policies with cidr
	ingressPoliciesWithCIDR, egressPoliciesWithCIDR := getCIDRNetworkPolicies(policiesByNamespace)

	// Send cidr telemetry
	metrics.SendLog(scriptMetricOperationID, fmt.Sprintf("[migration script] Found %d network policies with CIDR", len(ingressPoliciesWithCIDR)+len(egressPoliciesWithCIDR)), metrics.DonotPrint)

	// Get the named port
	ingressPoliciesWithNamedPort, egressPoliciesWithNamedPort := getNamedPortPolicies(policiesByNamespace)

	// Send named port telemetry
	metrics.SendLog(scriptMetricOperationID, fmt.Sprintf("[migration script] Found %d network policies with named port", len(ingressPoliciesWithNamedPort)+len(egressPoliciesWithNamedPort)), metrics.DonotPrint)

	// Get the network policies with egress (except not egress allow all)
	egressPolicies := getEgressPolicies(policiesByNamespace)

	// Send egress telemetry
	metrics.SendLog(scriptMetricOperationID, fmt.Sprintf("[migration script] Found %d network policies with egress", len(egressPolicies)), metrics.DonotPrint)

	// Get services that have externalTrafficPolicy!=Local that are unsafe (might have traffic disruption)
	unsafeServices := getUnsafeExternalTrafficPolicyClusterServices(namespaces, servicesByNamespace, policiesByNamespace)

	// Send unsafe services telemetry
	metrics.SendLog(scriptMetricOperationID, fmt.Sprintf("[migration script] Found %d services with externalTrafficPolicy=Cluster", len(unsafeServices)), metrics.DonotPrint)

	unsafeNetworkPolicesInCluster := false
	unsafeServicesInCluster := false
	if len(ingressEndportNetworkPolicy) > 0 || len(egressEndportNetworkPolicy) > 0 ||
		len(ingressPoliciesWithCIDR) > 0 || len(egressPoliciesWithCIDR) > 0 ||
		len(ingressPoliciesWithNamedPort) > 0 || len(egressPoliciesWithNamedPort) > 0 ||
		len(egressPolicies) > 0 {
		unsafeNetworkPolicesInCluster = true
	}
	if len(unsafeServices) > 0 {
		unsafeServicesInCluster = true
	}

	if unsafeNetworkPolicesInCluster || unsafeServicesInCluster {
		// Send cluster unsafe telemetry
		metrics.SendLog(scriptMetricOperationID, "[migration script] Fails some checks. Unsafe to migrate this cluster", metrics.DonotPrint)
	} else {
		// Send cluster safe telemetry
		metrics.SendLog(scriptMetricOperationID, "[migration script] Passes all checks. Safe to migrate this cluster", metrics.DonotPrint)
	}

	// Close the metrics before table is rendered and wait one second to prevent formatting issues
	metrics.Close()
	time.Sleep(time.Second)

	// Print the migration summary table
	renderMigrationSummaryTable(ingressEndportNetworkPolicy, egressEndportNetworkPolicy, ingressPoliciesWithCIDR, egressPoliciesWithCIDR, ingressPoliciesWithNamedPort, egressPoliciesWithNamedPort, egressPolicies, unsafeServices)

	// Print the flagged resource table and cluster resource table if the detailed-report flag is set
	if *detailedMigrationSummary {
		if unsafeNetworkPolicesInCluster {
			renderFlaggedNetworkPolicyTable(ingressEndportNetworkPolicy, egressEndportNetworkPolicy, ingressPoliciesWithCIDR, egressPoliciesWithCIDR, ingressPoliciesWithNamedPort, egressPoliciesWithNamedPort, egressPolicies)
		}
		if unsafeServicesInCluster {
			renderFlaggedServiceTable(unsafeServices)
		}
		renderClusterResourceTable(policiesByNamespace, servicesByNamespace, podsByNamespace)
	}

	// Print if the cluster is safe to migrate
	if unsafeNetworkPolicesInCluster || unsafeServicesInCluster {
		fmt.Println("\n\033[31m✘ Review above issues before migration.\033[0m")
		fmt.Println("Please see \033[32maka.ms/azurenpmtocilium\033[0m for instructions on how to evaluate/assess the above warnings marked by ❌.")
		fmt.Println("NOTE: rerun this script if any modifications (create/update/delete) are made to services or policies.")
	} else {
		fmt.Println("\n\033[32m✔ Safe to migrate this cluster.\033[0m")
		fmt.Println("For more details please see \033[32maka.ms/azurenpmtocilium\033[0m.")
	}
}

func renderMigrationSummaryTable(
	ingressEndportNetworkPolicy,
	egressEndportNetworkPolicy,
	ingressPoliciesWithCIDR,
	egressPoliciesWithCIDR,
	ingressPoliciesWithNamedPort,
	egressPoliciesWithNamedPort,
	egressPolicies,
	unsafeServices []string,
) {
	migrationSummarytable := tablewriter.NewWriter(os.Stdout)
	migrationSummarytable.SetHeader([]string{"Breaking Change", "Upgrade compatibility", "Count"})
	migrationSummarytable.SetRowLine(true)
	if len(ingressEndportNetworkPolicy) == 0 && len(egressEndportNetworkPolicy) == 0 {
		migrationSummarytable.Append([]string{"NetworkPolicy with endPort", "✅", fmt.Sprintf("0")})
	} else {
		migrationSummarytable.Append([]string{"NetworkPolicy with endPort", "❌", fmt.Sprintf("%d", len(ingressEndportNetworkPolicy)+len(egressEndportNetworkPolicy))})
	}
	if len(ingressPoliciesWithCIDR) == 0 && len(egressPoliciesWithCIDR) == 0 {
		migrationSummarytable.Append([]string{"NetworkPolicy with CIDR", "✅", "0"})
	} else {
		migrationSummarytable.Append([]string{"NetworkPolicy with CIDR", "❌", fmt.Sprintf("%d", len(ingressPoliciesWithCIDR)+len(egressPoliciesWithCIDR))})
	}
	if len(ingressPoliciesWithNamedPort) == 0 && len(egressPoliciesWithNamedPort) == 0 {
		migrationSummarytable.Append([]string{"NetworkPolicy with Named Port", "✅", "0"})
	} else {
		migrationSummarytable.Append([]string{"NetworkPolicy with Named Port", "❌", fmt.Sprintf("%d", len(ingressPoliciesWithNamedPort)+len(egressPoliciesWithNamedPort))})
	}
	if len(egressPolicies) == 0 {
		migrationSummarytable.Append([]string{"NetworkPolicy with Egress (Not Allow All Egress)", "✅", "0"})
	} else {
		migrationSummarytable.Append([]string{"NetworkPolicy with Egress (Not Allow All Egress)", "❌", fmt.Sprintf("%d", len(egressPolicies))})
	}
	if len(unsafeServices) == 0 {
		migrationSummarytable.Append([]string{"Disruption for some Services with externalTrafficPolicy=Cluster", "✅", "0"})
	} else {
		migrationSummarytable.Append([]string{"Disruption for some Services with externalTrafficPolicy=Cluster", "❌", fmt.Sprintf("%d", len(unsafeServices))})
	}

	fmt.Println("\nMigration Summary:")
	migrationSummarytable.Render()
}

func renderFlaggedNetworkPolicyTable(
	ingressEndportNetworkPolicy,
	egressEndportNetworkPolicy,
	ingressPoliciesWithCIDR,
	egressPoliciesWithCIDR,
	ingressPoliciesWithNamedPort,
	egressPoliciesWithNamedPort,
	egressPolicies []string,
) {
	flaggedResourceTable := tablewriter.NewWriter(os.Stdout)
	flaggedResourceTable.SetHeader([]string{"Network Policy", "NetworkPolicy with endPort", "NetworkPolicy with CIDR", "NetworkPolicy with Named Port", "NetworkPolicy with Egress (Not Allow All Egress)"})
	flaggedResourceTable.SetRowLine(true)

	// Create a map to store the policies and their flags
	policyFlags := make(map[string][]string)

	// Helper function to add a flag to a policy
	addFlag := func(policy string, flag string) {
		if _, exists := policyFlags[policy]; !exists {
			policyFlags[policy] = []string{"✅", "✅", "✅", "✅"}
		}
		switch flag {
		case "ingressEndPort":
			policyFlags[policy][0] = "❌ (ingress)"
		case "egressEndPort":
			policyFlags[policy][0] = "❌ (egress)"
		case "ingressCIDR":
			policyFlags[policy][1] = "❌ (ingress)"
		case "egressCIDR":
			policyFlags[policy][1] = "❌ (egress)"
		case "ingressNamedPort":
			policyFlags[policy][2] = "❌ (ingress)"
		case "egressNamedPort":
			policyFlags[policy][2] = "❌ (egress)"
		case "Egress":
			policyFlags[policy][3] = "❌"
		}
	}

	// Add flags for each policy
	for _, policy := range ingressEndportNetworkPolicy {
		addFlag(policy, "ingressEndPort")
	}
	for _, policy := range egressEndportNetworkPolicy {
		addFlag(policy, "egressEndPort")
	}
	for _, policy := range ingressPoliciesWithCIDR {
		addFlag(policy, "ingressCIDR")
	}
	for _, policy := range egressPoliciesWithCIDR {
		addFlag(policy, "egressCIDR")
	}
	for _, policy := range ingressPoliciesWithNamedPort {
		addFlag(policy, "ingressNamedPort")
	}
	for _, policy := range egressPoliciesWithNamedPort {
		addFlag(policy, "egressNamedPort")
	}
	for _, policy := range egressPolicies {
		addFlag(policy, "Egress")
	}

	// Append the policies and their flags to the table
	for policy, flags := range policyFlags {
		flaggedResourceTable.Append([]string{policy, flags[0], flags[1], flags[2], flags[3]})
	}

	fmt.Println("\nFlagged Network Policies:")
	flaggedResourceTable.Render()
}

func renderFlaggedServiceTable(unsafeServices []string) {
	fmt.Println("\nFlagged Services:")
	flaggedResourceTable := tablewriter.NewWriter(os.Stdout)
	flaggedResourceTable.SetHeader([]string{"Service", "Disruption for some Services with externalTrafficPolicy=Cluster"})
	flaggedResourceTable.SetRowLine(true)
	for _, service := range unsafeServices {
		flaggedResourceTable.Append([]string{fmt.Sprintf("%s", service), "❌"})
	}
	flaggedResourceTable.Render()
}

func renderClusterResourceTable(policiesByNamespace map[string][]*networkingv1.NetworkPolicy, servicesByNamespace map[string][]*corev1.Service, podsByNamespace map[string][]*corev1.Pod) {
	resourceTable := tablewriter.NewWriter(os.Stdout)
	resourceTable.SetHeader([]string{"Resource", "Count"})
	resourceTable.SetRowLine(true)

	// Count the total number of policies
	totalPolicies := 0
	for _, policies := range policiesByNamespace {
		totalPolicies += len(policies)
	}
	resourceTable.Append([]string{"NetworkPolicy", fmt.Sprintf("%d", totalPolicies)})

	// Count the total number of services
	totalServices := 0
	for _, services := range servicesByNamespace {
		totalServices += len(services)
	}
	resourceTable.Append([]string{"Service", fmt.Sprintf("%d", totalServices)})

	// Count the total number of pods
	totalPods := 0
	for _, pods := range podsByNamespace {
		totalPods += len(pods)
	}
	resourceTable.Append([]string{"Pod", fmt.Sprintf("%d", totalPods)})

	fmt.Println("\nCluster Resources:")
	resourceTable.Render()
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
			// Check the egress field for endport
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
func checkCIDRInPolicyRules(to []networkingv1.NetworkPolicyPeer) bool {
	for _, toRule := range to {
		if toRule.IPBlock != nil && toRule.IPBlock.CIDR != "" {
			return true
		}
	}
	return false
}

func getNamedPortPolicies(policiesByNamespace map[string][]*networkingv1.NetworkPolicy) (ingressPoliciesWithNamedPort, egressPoliciesWithNamedPort []string) {
	for namespace, policies := range policiesByNamespace {
		for _, policy := range policies {
			// Check the ingress field for named port
			for _, ingress := range policy.Spec.Ingress {
				if checkNamedPortInPolicyRules(ingress.Ports) {
					ingressPoliciesWithNamedPort = append(ingressPoliciesWithNamedPort, fmt.Sprintf("%s/%s", namespace, policy.Name))
					break
				}
			}
			// Check the egress field for named port
			for _, egress := range policy.Spec.Egress {
				if checkNamedPortInPolicyRules(egress.Ports) {
					egressPoliciesWithNamedPort = append(egressPoliciesWithNamedPort, fmt.Sprintf("%s/%s", namespace, policy.Name))
					break
				}
			}
		}
	}
	return ingressPoliciesWithNamedPort, egressPoliciesWithNamedPort
}

func checkNamedPortInPolicyRules(ports []networkingv1.NetworkPolicyPort) bool {
	for _, port := range ports {
		// If port is a string it is a named port
		if port.Port.Type == intstr.String {
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

func getUnsafeExternalTrafficPolicyClusterServices(
	namespaces *corev1.NamespaceList,
	servicesByNamespace map[string][]*corev1.Service,
	policiesByNamespace map[string][]*networkingv1.NetworkPolicy,
) (unsafeServices []string) {
	var riskServices, safeServices []string

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
					// Check if are there services with selector that are allowed by a network policy that can be safely migrated
					if checkNoServiceRisk(service, policyListAtNamespace) {
						safeServices = append(safeServices, fmt.Sprintf("%s/%s", namespace.Name, service.Name))
					}
				}
			}
		}
	}

	// Remove all the safe services from the services at risk
	unsafeServices = difference(riskServices, safeServices)
	return unsafeServices
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
			// Check for each policy label that that label is present in the service labels meaning the service is being targeted by the policy
			if checkPolicyMatchServiceLabels(service.Spec.Selector, policy.Spec.PodSelector) {
				// Check if there is an allow all ingress policy as the policy allows all services in the namespace
				if len(ingress.From) == 0 && len(ingress.Ports) == 0 {
					return true
				}
				// If there are no ingress from but there are ports in the policy; check if the service is safe
				if len(ingress.From) == 0 {
					// If the policy targets all pods (allow all) or only pods that are in the service selector, check if traffic is allowed to all the service's target ports
					// Note: ingress.Ports.protocol will never be nil if len(ingress.Ports) is greater than 0. It defaults to "TCP" if not set
					// Note: for loadbalancer services the health probe always hits the service target ports
					if checkServiceTargetPortMatchPolicyPorts(service.Spec.Ports, ingress.Ports) {
						return true
					}
				}
			}
		}
	}
	return false
}

func checkPolicyMatchServiceLabels(serviceLabels map[string]string, podSelector metav1.LabelSelector) bool {
	// Check if there is an target all ingress policy with empty selectors if so the service is safe
	if len(podSelector.MatchLabels) == 0 && len(podSelector.MatchExpressions) == 0 {
		return true
	}

	// Return false if the policy has matchExpressions
	// Note: does not check matchExpressions. It will only validate based on matchLabels
	if len(podSelector.MatchExpressions) > 0 {
		return false
	}

	// Return false if the policy has more labels than the service
	if len(podSelector.MatchLabels) > len(serviceLabels) {
		return false
	}

	// Check for each policy label that that label is present in the service labels
	// Note: a policy with no matchLabels is an allow all policy
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
			// Cilium treats port 0 in a special way so skip policys allowing port 0
			if int(policyPort.Port.IntVal) == 0 {
				continue
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
