package nmagent_test

import (
	"testing"

	"github.com/Azure/azure-container-networking/nmagent"
	"github.com/google/go-cmp/cmp"
)

func TestConfig(t *testing.T) {
	configTests := []struct {
		name     string
		config   nmagent.Config
		expValid bool
	}{
		{
			"empty",
			nmagent.Config{},
			false,
		},
		{
			"missing port",
			nmagent.Config{
				Host: "localhost",
			},
			false,
		},
		{
			"missing host",
			nmagent.Config{
				Port: 12345,
			},
			false,
		},
		{
			"complete",
			nmagent.Config{
				Host: "localhost",
				Port: 12345,
			},
			true,
		},
	}

	for _, test := range configTests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			err := test.config.Validate()
			if err != nil && test.expValid {
				t.Fatal("expected config to be valid but wasnt: err:", err)
			}

			if err == nil && !test.expValid {
				t.Fatal("expected config to be invalid but wasn't")
			}
		})
	}
}

func TestNMAgentConfig(t *testing.T) {
	tests := []struct {
		name         string
		wireserverIP string
		exp          nmagent.Config
		shouldErr    bool
	}{
		{
			"empty",
			"",
			nmagent.Config{
				Host: "168.63.129.16",
				Port: 80,
			},
			false,
		},
		{
			"ip",
			"127.0.0.1",
			nmagent.Config{
				Host: "127.0.0.1",
				Port: 80,
			},
			false,
		},
		{
			"ipport",
			"127.0.0.1:8080",
			nmagent.Config{
				Host: "127.0.0.1",
				Port: 8080,
			},
			false,
		},
		{
			"scheme",
			"http://127.0.0.1:8080",
			nmagent.Config{
				Host: "127.0.0.1",
				Port: 8080,
			},
			false,
		},
		{
			"invalid URL",
			"a string containing \"http\" with an invalid character: \x7F",
			nmagent.Config{},
			true,
		},
		{
			"invalid host:port",
			"way:too:many:colons",
			nmagent.Config{},
			true,
		},
		{
			"too big for a uint16 port",
			"127.0.0.1:4815162342",
			nmagent.Config{},
			true,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			got, err := nmagent.NewConfig(test.wireserverIP)
			if err != nil && !test.shouldErr {
				t.Fatal("unexpected error fetching nmagent config: err:", err)
			}

			if err == nil && test.shouldErr {
				t.Fatal("expected error fetching nmagent config but received none")
			}

			if !cmp.Equal(got, test.exp) {
				t.Error("received config differs from expectation: diff:", cmp.Diff(got, test.exp))
			}
		})
	}
}
