package main

import (
	"os"
	"testing"

	cnms "github.com/Azure/azure-container-networking/cnms/cnmspackage"
	"github.com/Azure/azure-container-networking/ebtables"
	"github.com/Azure/azure-container-networking/telemetry"
)

const (
	test1 = "test1.json"
	test2 = "test2.json"
)

var stateMapkey []string

func TestMain(m *testing.M) {
	stateMapkey = append(stateMapkey, "-p ARP -i eth0 --arp-op Reply -j dnat --to-dst ff:ff:ff:ff:ff:ff --dnat-target ACCEPT")
	stateMapkey = append(stateMapkey, "-p ARP --arp-op Request --arp-ip-dst 10.240.0.6 -j arpreply --arpreply-mac cc:ad:1d:4e:e5:f1")
	stateMapkey = append(stateMapkey, "-p IPv4 -i eth0 --ip-dst 10.240.0.6 -j dnat --to-dst cc:ad:1d:4e:e5:f1 --dnat-target ACCEPT")
	exitCode := m.Run()
	os.Exit(exitCode)
}

func addStateRulesToMap() map[string]string {
	rulesMap := make(map[string]string)
	for _, value := range stateMapkey {
		rulesMap[value] = ebtables.PreRouting
	}

	rulesMap["-s Unicast -o eth0 -j snat --to-src 00:0d:12:3a:5d:32 --snat-arp --snat-target ACCEPT"] = ebtables.PostRouting

	return rulesMap
}

func TestAddMissingRule(t *testing.T) {
	netMonitor := &cnms.NetworkMonitor{
		AddRulesToBeValidated:    make(map[string]int),
		DeleteRulesToBeValidated: make(map[string]int),
		CNIReport:                &telemetry.CNIReport{},
	}

	currentStateRulesMap := addStateRulesToMap()
	currentEbTableRulesMap := make(map[string]string)
	testKey := ""

	for key, value := range currentStateRulesMap {
		if value != ebtables.PostRouting {
			currentEbTableRulesMap[key] = value
		} else {
			testKey = key
		}
	}

	netMonitor.CreateRequiredL2Rules(currentEbTableRulesMap, currentStateRulesMap)
	if len(netMonitor.AddRulesToBeValidated) != 1 {
		t.Fatalf("Expected AddRulesToBeValidated length to be 1 but got %v", len(netMonitor.AddRulesToBeValidated))
	}

	netMonitor.RemoveInvalidL2Rules(currentEbTableRulesMap, currentStateRulesMap)
	if len(netMonitor.DeleteRulesToBeValidated) != 0 {
		t.Fatalf("Expected DeleteRulesToBeValidated length to be 0 but got %v", len(netMonitor.DeleteRulesToBeValidated))
	}

	for key, value := range netMonitor.AddRulesToBeValidated {
		if key != testKey {
			t.Fatalf("Expected AzurePostRouting snat but got %v", value)
		}
	}

	netMonitor.CreateRequiredL2Rules(currentEbTableRulesMap, currentStateRulesMap)
	if len(netMonitor.AddRulesToBeValidated) != 0 {
		t.Fatalf("Expected AddRulesToBeValidated length to be 0 but got %v", len(netMonitor.AddRulesToBeValidated))
	}
}

func TestDeleteInvalidRule(t *testing.T) {
	netMonitor := &cnms.NetworkMonitor{
		AddRulesToBeValidated:    make(map[string]int),
		DeleteRulesToBeValidated: make(map[string]int),
		CNIReport:                &telemetry.CNIReport{},
	}

	currentStateRulesMap := addStateRulesToMap()
	currentEbTableRulesMap := make(map[string]string)

	for key, value := range currentStateRulesMap {
		currentEbTableRulesMap[key] = value
	}

	delete(currentStateRulesMap, stateMapkey[0])

	netMonitor.CreateRequiredL2Rules(currentEbTableRulesMap, currentStateRulesMap)
	if len(netMonitor.AddRulesToBeValidated) != 0 {
		t.Fatalf("Expected AddRulesToBeValidated length to be 0 but got %v", len(netMonitor.AddRulesToBeValidated))
	}

	netMonitor.RemoveInvalidL2Rules(currentEbTableRulesMap, currentStateRulesMap)
	if len(netMonitor.DeleteRulesToBeValidated) != 1 {
		t.Fatalf("Expected DeleteRulesToBeValidated length to be 1 but got %v", len(netMonitor.DeleteRulesToBeValidated))
	}

	for key, value := range netMonitor.AddRulesToBeValidated {
		if key != stateMapkey[0] {
			t.Fatalf("Expected %v but got %v", stateMapkey[0], value)
		}
	}

	netMonitor.RemoveInvalidL2Rules(currentEbTableRulesMap, currentStateRulesMap)
	if len(netMonitor.DeleteRulesToBeValidated) != 0 {
		t.Fatalf("Expected DeleteRulesToBeValidated length to be 0 but got %v", len(netMonitor.DeleteRulesToBeValidated))
	}
}
