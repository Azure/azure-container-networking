package wireserver

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateGatewayIP(t *testing.T) {
	tests := []struct {
		name    string
		cidr    string
		want    net.IP
		wantErr bool
	}{
		{
			name: "base case",
			cidr: "10.0.0.0/8",
			want: net.IPv4(10, 0, 0, 1),
		},
		{
			name: "nonzero start",
			cidr: "10.177.233.128/27",
			want: net.IPv4(10, 177, 233, 129),
		},
		{
			name:    "invalid",
			cidr:    "test",
			wantErr: true,
		},
		{
			name:    "no available",
			cidr:    "255.255.255.255/32",
			wantErr: true,
		},
		{
			name: "max IPv4",
			cidr: "255.255.255.255/31",
			want: net.IPv4(255, 255, 255, 255),
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			got, err := calculateGatewayIP(tt.cidr)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.Truef(t, tt.want.Equal(got), "want %s, got %s", tt.want.String(), got.String())
		})
	}
}
