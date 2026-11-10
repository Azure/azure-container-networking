package configuration

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIPAMUnixSocketEnabled(t *testing.T) {
	for _, tt := range []struct {
		name       string
		configured bool
		override   string
		set        bool
		want       bool
		wantErr    bool
	}{
		{name: "disabled in file"},
		{name: "enabled in file", configured: true, want: true},
		{name: "enable for pod", override: "true", set: true, want: true},
		{name: "disable for pod", configured: true, override: "false", set: true},
		{name: "invalid override", override: "invalid", set: true, wantErr: true},
		{name: "empty override", set: true, wantErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(EnvIPAMUnixSocket, tt.override)
			if !tt.set {
				require.NoError(t, os.Unsetenv(EnvIPAMUnixSocket))
			}
			enabled, err := IPAMUnixSocketEnabled(tt.configured)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, enabled)
		})
	}
}

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

func TestInfraVNETCIDRs(t *testing.T) {
	_, err := InfraVNETCIDRs()
	require.Error(t, err)
	require.ErrorIs(t, err, ErrInfraVNETCIDRsUnset)
	os.Setenv(EnvInfraVNETCIDRs, "test")
	cidr, err := InfraVNETCIDRs()
	assert.NoError(t, err)
	assert.Equal(t, "test", cidr)
}
