package common

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

const invalidConfigText = "invalid"

func TestCiliumPoolConfigRejectsInvalidInput(t *testing.T) {
	for _, test := range []struct{ name, data, want string }{
		{"malformed", `{`, "decoding"},
		{"unknown", `{"unused":true}`, "unknown field"},
		{"missing", `{}`, invalidConfigText},
		{"trailing", `{} {}`, "trailing"},
		{"relative", `{"nodeName":"n","stateDir":"relative","interface":"eth0","nicID":"nic","subnet":"10.0.0.0/24","preAllocate":8}`, invalidConfigText},
		{"ephemeral", `{"nodeName":"n","stateDir":"/var/run/pool","interface":"eth0","nicID":"nic","subnet":"10.0.0.0/24","preAllocate":8}`, "durable"},
		{"no reserve", `{"nodeName":"n","stateDir":"/var/lib/pool","interface":"eth0","nicID":"nic","subnet":"10.0.0.0/24"}`, invalidConfigText},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			require.NoError(t, os.WriteFile(path, []byte(test.data), 0o600))
			_, err := ReadCiliumPoolConfig(path)
			require.ErrorContains(t, err, test.want)
		})
	}
	cfg, err := ReadCiliumPoolConfig(filepath.Join(t.TempDir(), "missing.json"))
	require.NoError(t, err)
	require.Nil(t, cfg)
	cfg = &CiliumPoolConfig{StateDir: "/var/lib/azure-vnet-cilium"}
	require.Equal(t, "/var/lib/azure-vnet-cilium/azure-vnet-ipam.json", cfg.StatePath("azure-vnet-ipam"))
	require.Equal(t, "/var/lib/azure-vnet-cilium/locks/azure-vnet-ipam.lock", cfg.LockPath("azure-vnet-ipam"))
}
