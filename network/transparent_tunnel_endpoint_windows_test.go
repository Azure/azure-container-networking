// Copyright 2017 Microsoft. All rights reserved.
// MIT License

//go:build windows
// +build windows

package network

import (
	"errors"
	"net"
	"strings"
	"testing"

	"github.com/Azure/azure-container-networking/platform"
)

var (
	errObjectAlreadyExists = errors.New("the object already exists")
	errNetworkUnreachable  = errors.New("network unreachable")
)

func TestIPv4Gateway(t *testing.T) {
	tests := []struct {
		name    string
		subnets []SubnetInfo
		want    net.IP
	}{
		{
			name:    "single IPv4 subnet",
			subnets: []SubnetInfo{{Gateway: net.ParseIP("10.0.0.1")}},
			want:    net.ParseIP("10.0.0.1"),
		},
		{
			name: "IPv6 first then IPv4",
			subnets: []SubnetInfo{
				{Gateway: net.ParseIP("fd00::1")},
				{Gateway: net.ParseIP("10.0.0.1")},
			},
			want: net.ParseIP("10.0.0.1"),
		},
		{
			name:    "empty subnets",
			subnets: nil,
			want:    nil,
		},
		{
			name:    "nil gateway in subnet",
			subnets: []SubnetInfo{{Gateway: nil}},
			want:    nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ipv4Gateway(tt.subnets)
			if !got.Equal(tt.want) {
				t.Errorf("ipv4Gateway() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAddTransparentTunnelRoutes(t *testing.T) {
	tests := []struct {
		name        string
		ipAddresses []net.IPNet
		gateway     net.IP
		execFn      func(cmd string, args ...string) (string, error)
		wantCmds    []string
		wantNoCmds  bool
		wantErr     bool
		wantErrMsg  string
	}{
		{
			name: "single IPv4 pod",
			ipAddresses: []net.IPNet{
				{IP: net.ParseIP("10.0.0.5"), Mask: net.CIDRMask(16, 32)},
			},
			gateway:  net.ParseIP("10.0.0.1"),
			wantCmds: []string{"route add 10.0.0.5 mask 255.255.255.255 10.0.0.1"},
		},
		{
			name: "multiple IPv4 pods",
			ipAddresses: []net.IPNet{
				{IP: net.ParseIP("10.0.0.5"), Mask: net.CIDRMask(16, 32)},
				{IP: net.ParseIP("10.0.0.6"), Mask: net.CIDRMask(16, 32)},
			},
			gateway: net.ParseIP("10.0.0.1"),
			wantCmds: []string{
				"route add 10.0.0.5 mask 255.255.255.255 10.0.0.1",
				"route add 10.0.0.6 mask 255.255.255.255 10.0.0.1",
			},
		},
		{
			name: "skip IPv6 addresses",
			ipAddresses: []net.IPNet{
				{IP: net.ParseIP("10.0.0.5"), Mask: net.CIDRMask(16, 32)},
				{IP: net.ParseIP("fd00::5"), Mask: net.CIDRMask(64, 128)},
			},
			gateway:  net.ParseIP("10.0.0.1"),
			wantCmds: []string{"route add 10.0.0.5 mask 255.255.255.255 10.0.0.1"},
		},
		{
			name:        "nil gateway returns error",
			ipAddresses: []net.IPNet{{IP: net.ParseIP("10.0.0.5"), Mask: net.CIDRMask(16, 32)}},
			gateway:     nil,
			wantNoCmds:  true,
			wantErr:     true,
			wantErrMsg:  "gateway is nil",
		},
		{
			name:        "unspecified gateway returns error",
			ipAddresses: []net.IPNet{{IP: net.ParseIP("10.0.0.5"), Mask: net.CIDRMask(16, 32)}},
			gateway:     net.IPv4zero,
			wantNoCmds:  true,
			wantErr:     true,
			wantErrMsg:  "gateway is nil or unspecified",
		},
		{
			name: "duplicate route already exists is tolerated",
			ipAddresses: []net.IPNet{
				{IP: net.ParseIP("10.0.0.5"), Mask: net.CIDRMask(16, 32)},
			},
			gateway: net.ParseIP("10.0.0.1"),
			execFn: func(_ string, _ ...string) (string, error) {
				return "", errObjectAlreadyExists
			},
			wantCmds: []string{"route add 10.0.0.5 mask 255.255.255.255 10.0.0.1"},
		},
		{
			name: "real failure returns error and rolls back",
			ipAddresses: []net.IPNet{
				{IP: net.ParseIP("10.0.0.5"), Mask: net.CIDRMask(16, 32)},
				{IP: net.ParseIP("10.0.0.6"), Mask: net.CIDRMask(16, 32)},
			},
			gateway: net.ParseIP("10.0.0.1"),
			execFn: func() func(string, ...string) (string, error) {
				callCount := 0
				return func(_ string, args ...string) (string, error) {
					callCount++
					if callCount == 1 {
						return "", nil // first route succeeds
					}
					if len(args) > 0 && args[0] == "delete" {
						return "", nil // rollback succeeds
					}
					return "", errNetworkUnreachable // second route fails
				}
			}(),
			// route add .5 (success), route add .6 (fail), route delete .5 (rollback)
			wantCmds: []string{
				"route add 10.0.0.5 mask 255.255.255.255 10.0.0.1",
				"route add 10.0.0.6 mask 255.255.255.255 10.0.0.1",
				"route delete 10.0.0.5 mask 255.255.255.255",
			},
			wantErr:    true,
			wantErrMsg: "network unreachable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var executedCmds []string
			mockPlc := platform.NewMockExecClient(false)
			recordCmd := func(cmd string, args ...string) string {
				full := cmd + " " + strings.Join(args, " ")
				executedCmds = append(executedCmds, full)
				return full
			}
			if tt.execFn != nil {
				mockPlc.SetExecCommand(func(cmd string, args ...string) (string, error) {
					recordCmd(cmd, args...)
					return tt.execFn(cmd, args...)
				})
			} else {
				mockPlc.SetExecCommand(func(cmd string, args ...string) (string, error) {
					recordCmd(cmd, args...)
					return "", nil
				})
			}

			err := addTransparentTunnelRoutes(mockPlc, tt.ipAddresses, tt.gateway)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.wantErrMsg != "" && !strings.Contains(err.Error(), tt.wantErrMsg) {
					t.Errorf("error %q should contain %q", err.Error(), tt.wantErrMsg)
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.wantNoCmds {
				if len(executedCmds) != 0 {
					t.Errorf("expected no commands, got %v", executedCmds)
				}
				return
			}

			if len(executedCmds) != len(tt.wantCmds) {
				t.Fatalf("expected %d commands, got %d: %v", len(tt.wantCmds), len(executedCmds), executedCmds)
			}
			for i, want := range tt.wantCmds {
				if executedCmds[i] != want {
					t.Errorf("cmd[%d] = %q, want %q", i, executedCmds[i], want)
				}
			}
		})
	}
}

func TestDeleteTransparentTunnelRoutes(t *testing.T) {
	tests := []struct {
		name        string
		ipAddresses []net.IPNet
		wantCmds    []string
	}{
		{
			name: "single IPv4 pod cleanup",
			ipAddresses: []net.IPNet{
				{IP: net.ParseIP("10.0.0.5"), Mask: net.CIDRMask(16, 32)},
			},
			wantCmds: []string{"route delete 10.0.0.5 mask 255.255.255.255"},
		},
		{
			name: "skip IPv6 on delete",
			ipAddresses: []net.IPNet{
				{IP: net.ParseIP("10.0.0.5"), Mask: net.CIDRMask(16, 32)},
				{IP: net.ParseIP("fd00::5"), Mask: net.CIDRMask(64, 128)},
			},
			wantCmds: []string{"route delete 10.0.0.5 mask 255.255.255.255"},
		},
		{
			name:        "empty IP list does nothing",
			ipAddresses: []net.IPNet{},
			wantCmds:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var executedCmds []string
			mockPlc := platform.NewMockExecClient(false)
			mockPlc.SetExecCommand(func(cmd string, args ...string) (string, error) {
				executedCmds = append(executedCmds, cmd+" "+strings.Join(args, " "))
				return "", nil
			})

			deleteTransparentTunnelRoutes(mockPlc, tt.ipAddresses)

			if len(tt.wantCmds) == 0 {
				if len(executedCmds) != 0 {
					t.Errorf("expected no commands, got %v", executedCmds)
				}
				return
			}

			if len(executedCmds) != len(tt.wantCmds) {
				t.Fatalf("expected %d commands, got %d: %v", len(tt.wantCmds), len(executedCmds), executedCmds)
			}
			for i, want := range tt.wantCmds {
				if executedCmds[i] != want {
					t.Errorf("cmd[%d] = %q, want %q", i, executedCmds[i], want)
				}
			}
		})
	}
}
