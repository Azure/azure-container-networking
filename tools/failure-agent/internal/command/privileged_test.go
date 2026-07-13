package command

import "testing"

func TestValidatePrivilegedAllowsDebugAndExec(t *testing.T) {
	tests := []struct {
		name string
		argv []string
	}{
		{"debug node", []string{"kubectl", "debug", "node/aks-node-1", "--image=mcr.microsoft.com/cbl-mariner/busybox:2.0", "--", "cat", "/var/log/azure-vnet.log"}},
		{"exec pod", []string{"kubectl", "exec", "-n", "kube-system", "azure-cns-xyz", "--", "cat", "/var/log/azure-vnet.log"}},
		{"get still allowed", []string{"kubectl", "get", "nodes"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidatePrivileged(tt.argv); err != nil {
				t.Errorf("expected privileged validation to pass for %v, got %v", tt.argv, err)
			}
		})
	}
}

func TestValidatePrivilegedDeniesMutating(t *testing.T) {
	tests := []struct {
		name string
		argv []string
	}{
		{"delete", []string{"kubectl", "delete", "pod", "foo"}},
		{"apply", []string{"kubectl", "apply", "-f", "foo.yaml"}},
		{"debug with watch", []string{"kubectl", "debug", "node/aks-node-1", "--watch"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidatePrivileged(tt.argv); err == nil {
				t.Errorf("expected privileged validation to reject %v", tt.argv)
			}
		})
	}
}
