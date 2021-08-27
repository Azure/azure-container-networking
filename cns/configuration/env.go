package configuration

import (
	"os"

	"github.com/pkg/errors"
)

const nodeNameEnvVar = "NODENAME"

// ErrNodeNameNotDefined indicates the node name env var is unset.
var ErrNodeNameNotDefined = errors.Errorf("must set %s environment variable", nodeNameEnvVar)

// NodeNameFromEnv return value of the node name env var or an error if unset.
func NodeNameFromEnv() (string, error) {
	// Check that NODENAME environment variable is set. NODENAME is name of node running this program
	nodeName := os.Getenv(nodeNameEnvVar)
	if nodeName == "" {
		return "", ErrNodeNameNotDefined
	}
	return nodeName, nil
}
