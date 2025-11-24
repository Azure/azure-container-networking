package longRunningCluster

import (
	"fmt"
	"os/exec"
	"strings"
)

// NetworkTopology defines the complete network infrastructure
type NetworkTopology struct {
	Customers []Customer
	Clusters  []Cluster
}

// Customer represents a tenant with their VNets
type Customer struct {
	ID    string // "customer1", "customer2"
	VNets []VNet
}

// VNet represents a virtual network with subnets
type VNet struct {
	Name            string // "cx_vnet_a1"
	Subnets         []Subnet
	PeeredWith      []string // List of VNet names this is peered with
	PrivateEndpoint *PrivateEndpointConfig
}

// Subnet represents a delegated subnet
type Subnet struct {
	Name        string   // "s1", "s2"
	HasNSG      bool     // Whether this subnet has NSG rules
	BlockedTo   []string // Subnet names that are blocked by NSG (e.g., ["s2"])
	AllowedFrom []string // Optional: explicitly allowed subnets
}

// PrivateEndpointConfig defines private endpoint setup
type PrivateEndpointConfig struct {
	SubnetName     string // Subnet where PE is deployed (e.g., "pe")
	StorageAccount string // Which storage account (from env var)
}

// Cluster represents an AKS cluster
type Cluster struct {
	Name      string // "aks-1", "aks-2"
	NodePools []NodePool
}

// NodePool represents a group of nodes with same capabilities
type NodePool struct {
	Name        string // "default", "nplinux"
	NICCapacity string // "low-nic" (1 delegated NIC), "high-nic" (7+ delegated NICs)
	NodeCount   int    // Number of nodes in this pool
}

// GetProductionTopology returns the actual deployed infrastructure
func GetProductionTopology() NetworkTopology {
	return NetworkTopology{
		Customers: []Customer{
			{
				ID: "customer1",
				VNets: []VNet{
					{
						Name:       "cx_vnet_a1",
						PeeredWith: []string{"cx_vnet_a2", "cx_vnet_a3"},
						Subnets: []Subnet{
							{
								Name:      "s1",
								HasNSG:    true,
								BlockedTo: []string{"s2"}, // s1 ↔ s2 blocked
							},
							{
								Name:      "s2",
								HasNSG:    true,
								BlockedTo: []string{"s1"}, // s2 ↔ s1 blocked
							},
						},
						PrivateEndpoint: &PrivateEndpointConfig{
							SubnetName:     "pe",
							StorageAccount: "STORAGE_ACCOUNT_1",
						},
					},
					{
						Name:       "cx_vnet_a2",
						PeeredWith: []string{"cx_vnet_a1", "cx_vnet_a3"},
						Subnets: []Subnet{
							{Name: "s1", HasNSG: false},
						},
					},
					{
						Name:       "cx_vnet_a3",
						PeeredWith: []string{"cx_vnet_a1", "cx_vnet_a2"},
						Subnets: []Subnet{
							{Name: "s1", HasNSG: false},
						},
					},
				},
			},
			{
				ID: "customer2",
				VNets: []VNet{
					{
						Name:       "cx_vnet_b1",
						PeeredWith: []string{}, // Not peered with other VNets
						Subnets: []Subnet{
							{Name: "s1", HasNSG: false},
						},
					},
				},
			},
		},
		Clusters: []Cluster{
			{
				Name: "aks-1",
				NodePools: []NodePool{
					{Name: "default", NICCapacity: "low-nic", NodeCount: 2},
					{Name: "nplinux", NICCapacity: "high-nic", NodeCount: 2},
				},
			},
			{
				Name: "aks-2",
				NodePools: []NodePool{
					{Name: "default", NICCapacity: "low-nic", NodeCount: 2},
					{Name: "nplinux", NICCapacity: "high-nic", NodeCount: 2},
				},
			},
		},
	}
}

// PodPlacement represents where a pod should be created
type PodPlacement struct {
	CustomerID  string
	Cluster     string
	VNet        string
	Subnet      string
	NICCapacity string // "low-nic" or "high-nic"
	PodName     string // Generated name
}

// GeneratePodPlacements creates an optimal pod placement strategy
// Strategy: 
// 1. Create 5 high-nic pods (one per scenario, cycling through high-nic nodes)
// 2. Create 4 low-nic pods (all scenarios except customer1/cx_vnet_a1/s1)
// Total: 9 pods for comprehensive connectivity testing
func GeneratePodPlacements(topology NetworkTopology) []PodPlacement {
	placements := []PodPlacement{}
	
	// Collect all subnet scenarios (customer/vnet/subnet combinations)
	type SubnetScenario struct {
		CustomerID string
		VNet       string
		Subnet     string
	}
	scenarios := []SubnetScenario{}
	
	for _, customer := range topology.Customers {
		for _, vnet := range customer.VNets {
			for _, subnet := range vnet.Subnets {
				scenarios = append(scenarios, SubnetScenario{
					CustomerID: customer.ID,
					VNet:       vnet.Name,
					Subnet:     subnet.Name,
				})
			}
		}
	}
	
	// Get all nodes from all clusters using kubectl
	type NodeInfo struct {
		Name        string
		Cluster     string
		NICCapacity string // Read from nic-capacity label
	}
	lowNicNodes := []NodeInfo{}
	highNicNodes := []NodeInfo{}
	
	for _, cluster := range topology.Clusters {
		kubeconfig := fmt.Sprintf("/tmp/%s.kubeconfig", cluster.Name)
		
		// Get all nodes from this cluster with nic-capacity label
		cmd := exec.Command("kubectl", "--kubeconfig", kubeconfig, "get", "nodes",
			"-o", "custom-columns=NAME:.metadata.name,NIC:.metadata.labels.nic-capacity", "--no-headers")
		out, err := cmd.CombinedOutput()
		if err != nil {
			fmt.Printf("Warning: failed to get nodes from %s: %v\n", cluster.Name, err)
			continue
		}
		
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		for _, line := range lines {
			if line == "" {
				continue
			}
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				nodeName := fields[0]
				nicCapacity := fields[1]
				
				// Skip if nic-capacity label is not set
				if nicCapacity == "<none>" {
					continue
				}
				
				nodeInfo := NodeInfo{
					Name:        nodeName,
					Cluster:     cluster.Name,
					NICCapacity: nicCapacity,
				}
				
				// Categorize by NIC capacity
				if nicCapacity == "low-nic" {
					lowNicNodes = append(lowNicNodes, nodeInfo)
				} else if nicCapacity == "high-nic" {
					highNicNodes = append(highNicNodes, nodeInfo)
				}
			}
		}
	}
	
	// Phase 1: Create 5 high-nic pods (one per scenario, cycling through high-nic nodes)
	for i, scenario := range scenarios {
		if len(highNicNodes) == 0 {
			fmt.Printf("Warning: no high-nic nodes available, skipping high-nic pods\n")
			break
		}
		
		node := highNicNodes[i%len(highNicNodes)] // Cycle through high-nic nodes
		
		placement := PodPlacement{
			CustomerID:  scenario.CustomerID,
			Cluster:     node.Cluster,
			VNet:        scenario.VNet,
			Subnet:      scenario.Subnet,
			NICCapacity: node.NICCapacity,
			PodName:     generatePodName(scenario.CustomerID, node.Cluster, scenario.VNet, scenario.Subnet, node.NICCapacity),
		}
		placements = append(placements, placement)
	}
	
	// Phase 2: Create 4 low-nic pods (all scenarios except customer1/cx_vnet_a1/s1)
	lowNicIndex := 0
	for _, scenario := range scenarios {
		// Skip customer1/cx_vnet_a1/s1
		if scenario.CustomerID == "customer1" && scenario.VNet == "cx_vnet_a1" && scenario.Subnet == "s1" {
			continue
		}
		
		if lowNicIndex >= len(lowNicNodes) {
			fmt.Printf("Warning: no more low-nic nodes available, created %d low-nic pods\n", lowNicIndex)
			break
		}
		
		node := lowNicNodes[lowNicIndex]
		lowNicIndex++
		
		placement := PodPlacement{
			CustomerID:  scenario.CustomerID,
			Cluster:     node.Cluster,
			VNet:        scenario.VNet,
			Subnet:      scenario.Subnet,
			NICCapacity: node.NICCapacity,
			PodName:     generatePodName(scenario.CustomerID, node.Cluster, scenario.VNet, scenario.Subnet, node.NICCapacity),
		}
		placements = append(placements, placement)
	}
	
	fmt.Printf("Generated %d pod placements: %d high-nic + %d low-nic\n", 
		len(placements), len(scenarios), lowNicIndex)
	
	return placements
}

// generatePodName creates a consistent pod name from placement info
func generatePodName(customerID, cluster, vnet, subnet, nicType string) string {
	// Extract short IDs: customer1 -> c1, aks-1 -> aks1, cx_vnet_a1 -> a1
	custShort := customerID[len(customerID)-1:] // "customer1" -> "1"
	clusterShort := cluster[4:]                 // "aks-1" -> "1"
	vnetShort := vnet[len(vnet)-2:]             // "cx_vnet_a1" -> "a1"

	nicShort := "low"
	if nicType == "high-nic" {
		nicShort = "high"
	}

	return "pod-c" + custShort + "-aks" + clusterShort + "-" + vnetShort + subnet + "-" + nicShort
}

// ConvertPlacementToScenario converts PodPlacement to PodScenario
func ConvertPlacementToScenario(placement PodPlacement) PodScenario {
	custShort := placement.CustomerID[len(placement.CustomerID)-1:]
	clusterShort := placement.Cluster[4:]
	vnetShort := placement.VNet[len(placement.VNet)-2:]

	return PodScenario{
		Name:          placement.PodName,
		Cluster:       placement.Cluster,
		VnetName:      placement.VNet,
		SubnetName:    placement.Subnet,
		NodeSelector:  placement.NICCapacity,
		PodNameSuffix: "c" + custShort + "-aks" + clusterShort + "-" + vnetShort + placement.Subnet + "-" + placement.NICCapacity[:4],
	}
}

// GetScenarioFromTopology generates all pod scenarios from topology
func GetScenariosFromTopology() []PodScenario {
	topology := GetProductionTopology()
	placements := GeneratePodPlacements(topology)

	scenarios := make([]PodScenario, len(placements))
	for i, placement := range placements {
		scenarios[i] = ConvertPlacementToScenario(placement)
	}

	return scenarios
}
