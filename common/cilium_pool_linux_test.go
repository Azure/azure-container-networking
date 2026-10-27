package common

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
			if err := os.WriteFile(path, []byte(test.data), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := ReadCiliumPoolConfig(path)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Errorf("ReadCiliumPoolConfig: got %v, want error containing %q", err, test.want)
			}
		})
	}
	cfg, err := ReadCiliumPoolConfig(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg != nil {
		t.Errorf("missing configuration: got %+v, want nil", cfg)
	}
	cfg = &CiliumPoolConfig{StateDir: "/var/lib/azure-vnet-cilium"}
	if got, want := cfg.StatePath("azure-vnet-ipam"), "/var/lib/azure-vnet-cilium/azure-vnet-ipam.json"; got != want {
		t.Errorf("StatePath = %q, want %q", got, want)
	}
	if got, want := cfg.LockPath("azure-vnet-ipam"), "/var/lib/azure-vnet-cilium/locks/azure-vnet-ipam.lock"; got != want {
		t.Errorf("LockPath = %q, want %q", got, want)
	}
}
