// Copyright Microsoft. All rights reserved.
// MIT License

// Package nodesetup performs one-time node-level preparation before CNS starts
// serving. What "getting the node ready" means is an implementation detail that
// varies by platform — for example, programming IP rules on Linux or configuring
// HNS on Windows.
package nodesetup

import (
	"github.com/Azure/azure-container-networking/cns/configuration"
	"go.uber.org/zap"
)

// NodeConfiguration performs platform-specific node setup.
type NodeConfiguration struct {
	config *configuration.CNSConfig
	logger *zap.Logger
}

// New creates a NodeConfiguration with the given configuration and logger.
func New(config *configuration.CNSConfig, logger *zap.Logger) *NodeConfiguration {
	return &NodeConfiguration{config: config, logger: logger}
}
