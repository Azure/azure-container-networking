package command

import "testing"

func TestValidateAllowsReadOnlyVerbs(t *testing.T) {
	cases := [][]string{
		{"kubectl", "get", "pods", "-A"},
		{"kubectl", "describe", "node", "aks-node-1"},
		{"kubectl", "logs", "pod/azure-cns-abc", "-n", "kube-system"},
		{"kubectl", "events", "-n", "kube-system"},
		{"kubectl", "get", "pods", "-o", "jsonpath={.items}"},
	}
	for _, argv := range cases {
		if err := Validate(argv); err != nil {
			t.Errorf("Validate(%v) = %v, want nil", argv, err)
		}
	}
}

func TestValidateDeniesMutatingAndInteractive(t *testing.T) {
	cases := [][]string{
		{"kubectl", "apply", "-f", "x.yaml"},
		{"kubectl", "patch", "deploy", "x"},
		{"kubectl", "delete", "pod", "x"},
		{"kubectl", "exec", "-it", "pod", "--", "sh"},
		{"kubectl", "port-forward", "pod", "8080:80"},
		{"kubectl", "cp", "pod:/a", "/b"},
		{"kubectl", "scale", "deploy", "x", "--replicas=0"},
		{"kubectl", "rollout", "restart", "deploy", "x"},
	}
	for _, argv := range cases {
		if err := Validate(argv); err == nil {
			t.Errorf("Validate(%v) = nil, want denial", argv)
		}
	}
}

func TestValidateDeniesUnlistedVerb(t *testing.T) {
	if err := Validate([]string{"kubectl", "top", "pods"}); err == nil {
		t.Error("expected unlisted verb to be denied")
	}
}

func TestValidateDeniesStreamingFlags(t *testing.T) {
	cases := [][]string{
		{"kubectl", "logs", "pod/x", "-f"},
		{"kubectl", "logs", "pod/x", "--follow"},
		{"kubectl", "get", "pods", "-w"},
		{"kubectl", "get", "pods", "--watch=true"},
	}
	for _, argv := range cases {
		if err := Validate(argv); err == nil {
			t.Errorf("Validate(%v) = nil, want denial of streaming flag", argv)
		}
	}
}

func TestValidateRejectsMalformed(t *testing.T) {
	cases := [][]string{
		nil,
		{"helm", "install", "x"},
		{"kubectl"},
		{"kubectl", "-n", "kube-system"},
	}
	for _, argv := range cases {
		if err := Validate(argv); err == nil {
			t.Errorf("Validate(%v) = nil, want error", argv)
		}
	}
}
