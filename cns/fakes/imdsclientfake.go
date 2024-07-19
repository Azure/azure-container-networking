//go:build !ignore_uncovered
// +build !ignore_uncovered

// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package fakes

import (
	"context"

	"github.com/Azure/azure-container-networking/cns/imds"
	"github.com/Azure/azure-container-networking/cns/wireserver"
)

const (
	// HostPrimaryIP 10.0.0.4
	HostPrimaryIP = "10.0.0.4"
	// HostSubnet 10.0.0.0/24
	HostSubnet                   = "10.0.0.0/24"
	SimulateError MockIMDSCtxKey = "simulate-error"
)

type WireserverClientFake struct{}
type MockIMDSCtxKey string
type MockIMDSClient struct{}

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

func NewMockIMDSClient() *MockIMDSClient {
	return &MockIMDSClient{}
}

func (m *MockIMDSClient) GetVMUniqueID(ctx context.Context) (string, error) {
	if ctx.Value(SimulateError) != nil {
		return "", imds.ErrUnexpectedStatusCode
	}

	return "55b8499d-9b42-4f85-843f-24ff69f4a643", nil
}
