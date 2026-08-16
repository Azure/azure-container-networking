// Copyright 2017 Microsoft. All rights reserved.
// MIT License

//go:build windows
// +build windows

package network

import (
	"fmt"
	"os"
	"testing"
)

func TestIBVFDismountEnabled(t *testing.T) {
	tests := []struct {
		name  string
		value string
		set   bool
		want  bool
	}{
		{name: "unset defaults to no-op", set: false, want: false},
		{name: "empty is no-op", value: "", set: true, want: false},
		{name: "zero is no-op", value: "0", set: true, want: false},
		{name: "false is no-op", value: "false", set: true, want: false},
		{name: "one enables", value: "1", set: true, want: true},
		{name: "true enables", value: "true", set: true, want: true},
		{name: "mixed case true enables", value: "TrUe", set: true, want: true},
		{name: "padded true enables", value: "  true  ", set: true, want: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			os.Unsetenv(enableIBVFDismountEnvVar)
			if tc.set {
				t.Setenv(enableIBVFDismountEnvVar, tc.value)
			}
			if got := ibVFDismountEnabled(); got != tc.want {
				t.Fatalf("ibVFDismountEnabled()=%v, want %v (env=%q set=%v)", got, tc.want, tc.value, tc.set)
			}
		})
	}
}

func TestSignalNamedEvent(t *testing.T) {
	// Use a session-local (Local\) unique name so the test does not require the
	// SeCreateGlobalPrivilege that the production Global\ event needs.
	eventName := fmt.Sprintf(`Local\AzureCNI-IB-Test-%d`, os.Getpid())

	if err := signalNamedEvent(eventName); err != nil {
		t.Fatalf("signalNamedEvent(%q) unexpected error: %v", eventName, err)
	}

	// Signalling again must succeed (event already exists, handle is reopened).
	if err := signalNamedEvent(eventName); err != nil {
		t.Fatalf("signalNamedEvent(%q) second call unexpected error: %v", eventName, err)
	}
}

func TestSignalIBWorkDoneUsesGlobalName(t *testing.T) {
	if ibReadyEventName != `Global\AzureCNI-IB-Ready` {
		t.Fatalf("ibReadyEventName changed unexpectedly: %q", ibReadyEventName)
	}
}
