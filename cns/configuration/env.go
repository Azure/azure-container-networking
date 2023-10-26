package configuration

import (
	"os"

	"github.com/pkg/errors"
)

const (
	// EnvNodeName is the NODENAME env var string key.
	EnvNodeName = "NODENAME"
	// EnvNodeIP is the IP of the node running this CNS binary
	EnvNodeIP = "NODE_IP"
	// LabelNodeSwiftV2 is the Node label for Swift V2
	LabelNodeSwiftV2 = "kubernetes.azure.com/podnetwork-multi-tenancy-enabled"
	// LabelPodSwiftV2 is the Pod label for Swift V2
	LabelPodSwiftV2 = "kubernetes.azure.com/pod-network"
	EnvPodCIDRs     = "POD_CIDRs"
	EnvServiceCIDRs = "SERVICE_CIDRs"
	EnvNodeCIDR     = "NODE_CIDR"
)

// ErrNodeNameUnset indicates the the $EnvNodeName variable is unset in the environment.
var ErrNodeNameUnset = errors.Errorf("must declare %s environment variable", EnvNodeName)

// ErrPodCIDRsUnset indicates the the $EnvPodCIDRs variable is unset in the environment.
var ErrPodCIDRsUnset = errors.Errorf("must declare %s environment variable", EnvPodCIDRs)

// ErrServiceCIDRsUnset indicates the the $EnvServiceCIDRs variable is unset in the environment.
var ErrServiceCIDRsUnset = errors.Errorf("must declare %s environment variable", EnvServiceCIDRs)

// ErrNodeCIDRUnset indicates the the $EnvNodeCIDR variable is unset in the environment.
var ErrNodeCIDRUnset = errors.Errorf("must declare %s environment variable", EnvNodeCIDR)

// NodeName checks the environment variables for the NODENAME and returns it or an error if unset.
func NodeName() (string, error) {
	nodeName := os.Getenv(EnvNodeName)
	if nodeName == "" {
		return "", ErrNodeNameUnset
	}
	return nodeName, nil
}

// NodeIP returns the value of the NODE_IP environment variable, or empty string if unset.
func NodeIP() string {
	return os.Getenv(EnvNodeIP)
}

func PodCIDRs() (string, error) {
	podCIDRs := os.Getenv(EnvPodCIDRs)
	if podCIDRs == "" {
		return "", ErrPodCIDRsUnset
	}
	return podCIDRs, nil
}

func ServiceCIDRs() (string, error) {
	serviceCIDRs := os.Getenv(EnvServiceCIDRs)
	if serviceCIDRs == "" {
		return "", ErrServiceCIDRsUnset
	}
	return serviceCIDRs, nil
}

func NodeCIDR() (string, error) {
	nodeCIDR := os.Getenv(EnvNodeCIDR)
	if nodeCIDR == "" {
		return "", ErrNodeCIDRUnset
	}
	return nodeCIDR, nil
}
