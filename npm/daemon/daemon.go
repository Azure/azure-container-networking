// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package daemon

import (
	"context"
	"errors"
	"fmt"

	npmconfig "github.com/Azure/azure-container-networking/npm/config"
	"github.com/Azure/azure-container-networking/npm/pkg/controlplane/goalstateprocessor"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane"
	"github.com/Azure/azure-container-networking/npm/pkg/transport"
)

var aiMetadata string //nolint // aiMetadata is set in Makefile

var ErrDataplaneNotInitialized = errors.New("dataplane is not initialized")

type NetworkPolicyDaemon struct {
	ctx     context.Context
	config  npmconfig.Config
	client  *transport.EventsClient
	version string
	gsp     *goalstateprocessor.GoalStateProcessor
}

func NewNetworkPolicyDaemon(
	ctx context.Context,
	config npmconfig.Config,
	dp dataplane.GenericDataplane,
	gsp *goalstateprocessor.GoalStateProcessor,
	client *transport.EventsClient,
	npmVersion string,
) (*NetworkPolicyDaemon, error) {

	if dp == nil {
		return nil, ErrDataplaneNotInitialized
	}

	return &NetworkPolicyDaemon{
		ctx:     ctx,
		config:  config,
		gsp:     gsp,
		client:  client,
		version: npmVersion,
	}, nil
}

func (n *NetworkPolicyDaemon) Start(config npmconfig.Config, stopCh <-chan struct{}) error {
	n.gsp.Start(stopCh)
	if err := n.client.Start(stopCh); err != nil {
		return fmt.Errorf("failed to start dataplane events client: %w", err)
	}
	<-stopCh
	return nil
}

func (n *NetworkPolicyDaemon) GetAppVersion() string {
	return n.version
}
