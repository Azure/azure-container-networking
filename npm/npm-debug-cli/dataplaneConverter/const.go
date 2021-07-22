package converter

import (
	"github.com/Azure/azure-container-networking/npm"
	networkingv1 "k8s.io/api/networking/v1"
)

var (
	RequiredChains = []string{
		"AZURE-NPM-INGRESS-DROPS",
		"AZURE-NPM-INGRESS-FROM",
		"AZURE-NPM-INGRESS-PORT",
		"AZURE-NPM-EGRESS-DROPS",
		"AZURE-NPM-EGRESS-PORT",
		"AZURE-NPM-EGRESS-TO",
	}
)

type NPMCache struct {
	Exec             interface{}
	Nodename         string
	NsMap            map[string]*npm.Namespace
	PodMap           map[string]*npm.NpmPod
	RawNpMap         map[string]*networkingv1.NetworkPolicy
	ProcessedNpMap   map[string]*networkingv1.NetworkPolicy
	TelemetryEnabled bool
}
