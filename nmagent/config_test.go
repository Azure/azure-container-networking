package nmagent_test

import (
	"testing"

	"github.com/Azure/azure-container-networking/nmagent"
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
