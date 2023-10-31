package configuration

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNodeName(t *testing.T) {
	_, err := NodeName()
	require.Error(t, err)
	require.ErrorIs(t, err, ErrNodeNameUnset)
	os.Setenv(EnvNodeName, "test")
	name, err := NodeName()
	assert.NoError(t, err)
	assert.Equal(t, "test", name)
}

func TestPodCIDRs(t *testing.T) {
	_, err := PodCIDRs()
	require.Error(t, err)
	require.ErrorIs(t, err, ErrPodCIDRsUnset)
	os.Setenv(EnvPodCIDRs, "test")
	cidr, err := PodCIDRs()
	assert.NoError(t, err)
	assert.Equal(t, "test", cidr)
}

func TestServiceCIDRs(t *testing.T) {
	_, err := ServiceCIDRs()
	require.Error(t, err)
	require.ErrorIs(t, err, ErrServiceCIDRsUnset)
	os.Setenv(EnvServiceCIDRs, "test")
	cidr, err := ServiceCIDRs()
	assert.NoError(t, err)
	assert.Equal(t, "test", cidr)
}
