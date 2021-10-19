//go:build !ignore_uncovered
// +build !ignore_uncovered

// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package fakes

import (
	"context"

	"github.com/Azure/azure-container-networking/cns/imds"
)

var (
	// HostPrimaryIP 10.0.0.4
	HostPrimaryIP = "10.0.0.4"
	// HostSubnet 10.0.0.0/24
	HostSubnet = "10.0.0.0/24"
)

type IMDSFake struct{}

func (imdsClient *IMDSFake) GetInterfaces(ctx context.Context) (*imds.GetInterfacesResult, error) {
	return &imds.GetInterfacesResult{
		Interface: []imds.Interface{
			{
				IsPrimary: true,
				IPSubnet: []imds.Subnet{
					{
						Prefix: HostSubnet,
						IPAddress: []imds.Address{
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
