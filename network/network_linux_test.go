package network

import (
	"errors"
	"strings"
	"testing"

	"github.com/Azure/azure-container-networking/platform"
)

func TestAddDNSServersWithErr(t *testing.T) {
	nm := &networkManager{
		plClient: platform.NewMockExecClient(true),
	}
	_, err := nm.addDNSServers("invalid", []string{""})
	if err == nil {
		t.Errorf("Expected error")
	}
}

func TestAddDNSServers(t *testing.T) {
	testCases := []struct {
		desc      string
		osversion string
		ifName    string
		servers   []string
		cmd       string
		err       error
	}{
		{
			desc:      "empty server list",
			osversion: Ubuntu22,
			ifName:    "azure0",
			servers:   []string{},
			cmd:       "",
			err:       nil,
		},
		{
			desc:      "one server address",
			osversion: Ubuntu22,
			ifName:    "azure0",
			servers:   []string{"8.8.8.8"},
			cmd:       "resolvectl dns azure0 8.8.8.8",
			err:       nil,
		},
		{
			desc:      "multiple server addresses",
			osversion: Ubuntu22,
			ifName:    "azure0",
			servers:   []string{"1.1.1.1", "bing.com"},
			cmd:       "resolvectl dns azure0 1.1.1.1 bing.com",
			err:       nil,
		},
		{
			desc:      "empty server list",
			osversion: "16.0.0",
			ifName:    "azure0",
			servers:   []string{},
			cmd:       "",
			err:       nil,
		},
		{
			desc:      "one server address",
			osversion: "16.0.0",
			ifName:    "azure0",
			servers:   []string{"8.8.8.8"},
			cmd:       "systemd-resolve --interface azure0 --set-dns 8.8.8.8",
			err:       nil,
		},
		{
			desc:      "multiple server addresses",
			osversion: "16.0.0",
			ifName:    "azure0",
			servers:   []string{"1.1.1.1", "bing.com"},
			cmd:       "systemd-resolve --interface azure0 --set-dns 1.1.1.1 --set-dns bing.com",
			err:       nil,
		},
	}
	for _, tc := range testCases {
		t.Log("Running: ", tc.desc, " for ", tc.osversion)

		m := platform.NewMockExecClient(false)
		m.SetExecCommand(func(cmd string) (string, error) {
			return tc.osversion, nil
		})
		nm := &networkManager{
			plClient: m,
		}
		cmd, err := nm.addDNSServers(tc.ifName, tc.servers)

		if !errors.Is(err, tc.err) {
			t.Errorf("Expected error: %v, got: %v", tc.err, err)
		}
		if strings.Compare(cmd, tc.cmd) != 0 {
			t.Errorf("Expected cmd: %v, got: %v", tc.cmd, cmd)
		}
	}
}
