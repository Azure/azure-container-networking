package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseCIDRs(t *testing.T) {
	// Test valid IPv4 CIDR
	cidr := "192.168.0.0/16"
	expectedV4 := []string{"192.168.0.0/16"}
	expectedV6 := []string{}
	resultV4, resultV6, err := ParseCIDRs(cidr)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	assert.Equal(t, expectedV4, resultV4)
	assert.Equal(t, expectedV6, resultV6)

	// Test valid IPv6 CIDR
	cidr = "2001:db8::/32"
	expectedV4 = []string{}
	expectedV6 = []string{"2001:db8::/32"}
	resultV4, resultV6, err = ParseCIDRs(cidr)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	assert.Equal(t, expectedV4, resultV4)
	assert.Equal(t, expectedV6, resultV6)

	// Test multiple valid CIDRs
	cidrs := "192.168.0.0/16,10.0.0.0/8,2001:db8::/32"
	expectedV4 = []string{"192.168.0.0/16", "10.0.0.0/8"}
	expectedV6 = []string{"2001:db8::/32"}
	resultV4, resultV6, err = ParseCIDRs(cidrs)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	assert.Equal(t, expectedV4, resultV4)
	assert.Equal(t, expectedV6, resultV6)

	// Test invalid CIDR
	cidr = "192.168.0.0/33"
	_, _, err = ParseCIDRs(cidr)
	if err == nil {
		t.Errorf("Expected error but got nil")
	}

	// Test invalid CIDRs
	cidrs = "192.168.0.0/33,10.0.0.0/8,2001:db8::/33"
	_, _, err = ParseCIDRs(cidrs)
	if err == nil {
		t.Errorf("Expected error but got nil")
	}
}
