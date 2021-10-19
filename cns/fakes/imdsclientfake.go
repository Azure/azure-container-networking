//go:build !ignore_uncovered
// +build !ignore_uncovered

// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package fakes

import (
	"context"

	"github.com/Azure/azure-container-networking/cns/wireserver"
)

var (
	// HostPrimaryIP 10.0.0.4
	HostPrimaryIP = "10.0.0.4"
	// HostSubnet 10.0.0.0/24
	HostSubnet = "10.0.0.0/24"
)

type WireserverClientFake struct{}

func (c *WireserverClientFake) GetInterfaces(ctx context.Context) (*wireserver.GetInterfacesResult, error) {
	return &wireserver.GetInterfacesResult{
		Interface: []wireserver.Interface{
			{
				IsPrimary: true,
				IPSubnet: []wireserver.Subnet{
					{
						Prefix: HostSubnet,
						IPAddress: []wireserver.Address{
							{
								Address:   HostPrimaryIP,
								IsPrimary: true,
							},
						},
					},
				},
			},
		},
	}, nil
}
