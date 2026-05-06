// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package network

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/Azure/azure-container-networking/platform"
	"go.uber.org/zap"
)

var errTransparentTunnelGatewayNilOrUnspecified = errors.New("transparent-tunnel: gateway is nil or unspecified, cannot add tunnel routes")

// ipv4Gateway returns the gateway IP from the first IPv4 subnet, or nil if none found.
func ipv4Gateway(subnets []SubnetInfo) net.IP {
	for i := range subnets {
		if gw := subnets[i].Gateway; gw != nil && gw.To4() != nil {
			return gw
		}
	}
	return nil
}

// addTransparentTunnelRoutes adds a /32 host route for each IPv4 address of the endpoint,
// forcing same-node traffic to that pod through the gateway (and VFP) for NSG enforcement.
// This is the Windows equivalent of the Linux fwmark+policy-routing approach.
// On Windows, a /32 route is more specific than the subnet's /16 on-link route, so it
// takes priority and steers traffic out through the physical NIC where VFP can enforce NSGs.
// Returns an error if any route fails to be added (excluding idempotent duplicates).
func addTransparentTunnelRoutes(plc platform.ExecClient, ipAddresses []net.IPNet, gateway net.IP) error {
	if gateway == nil || gateway.IsUnspecified() {
		return errTransparentTunnelGatewayNilOrUnspecified
	}

	gwStr := gateway.String()
	var addedRoutes []string

	for _, ipAddr := range ipAddresses {
		if ipAddr.IP.To4() == nil {
			continue // skip IPv6
		}
		podIP := ipAddr.IP.String()
		_, err := plc.ExecuteCommand(context.TODO(), "route", "add", podIP, "mask", "255.255.255.255", gwStr)
		if err != nil {
			// "route add" is not idempotent on Windows — it fails if the route exists.
			// Treat "already exists" as success (another pod create may have raced us,
			// or the route survived a CNI restart).
			if strings.Contains(strings.ToLower(err.Error()), "already") {
				logger.Info("transparent-tunnel: /32 route already exists, skipping",
					zap.String("podIP", podIP), zap.String("gw", gwStr))
				continue
			}
			// Real failure — roll back any routes we already added in this call.
			for _, rollbackIP := range addedRoutes {
				if _, rbErr := plc.ExecuteCommand(context.TODO(), "route", "delete", rollbackIP, "mask", "255.255.255.255"); rbErr != nil {
					logger.Error("transparent-tunnel: failed to roll back /32 route",
						zap.String("podIP", rollbackIP), zap.Error(rbErr))
				}
			}
			return fmt.Errorf("transparent-tunnel: failed to add /32 route for %s via %s: %w", podIP, gwStr, err)
		}
		addedRoutes = append(addedRoutes, podIP)
		logger.Info("transparent-tunnel: added /32 host route via gateway",
			zap.String("podIP", podIP), zap.String("gw", gwStr))
	}

	return nil
}

// deleteTransparentTunnelRoutes removes the /32 host routes added by addTransparentTunnelRoutes.
// Best-effort: errors are logged but do not block endpoint deletion.
func deleteTransparentTunnelRoutes(plc platform.ExecClient, ipAddresses []net.IPNet) {
	for _, ipAddr := range ipAddresses {
		if ipAddr.IP.To4() == nil {
			continue
		}
		podIP := ipAddr.IP.String()
		if _, err := plc.ExecuteCommand(context.TODO(), "route", "delete", podIP, "mask", "255.255.255.255"); err != nil {
			logger.Error("transparent-tunnel: failed to delete /32 route",
				zap.String("podIP", podIP), zap.Error(err))
		} else {
			logger.Info("transparent-tunnel: deleted /32 host route", zap.String("podIP", podIP))
		}
	}
}
