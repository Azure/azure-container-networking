package network

import (
	"net"
	"testing"

	"github.com/Azure/azure-container-networking/platform"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ipsetSetNotFoundStderr reproduces what platform.ExecClient returns when
// `ipset del` targets a set that does not exist. ExecuteCommand folds stderr
// into the error, so the message arrives on the error and not on stdout.
const ipsetSetNotFoundStderr = "exit status 1:ipset v7.15: The set with the given name does not exist"

func TestIsSetNotFoundErr(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil error is not a missing set", err: nil, want: false},
		{
			name: "ipset missing-set message is detected",
			err:  errors.New(ipsetSetNotFoundStderr),
			want: true,
		},
		{
			name: "wrapped missing-set message is detected",
			err:  errors.Wrap(errors.New(ipsetSetNotFoundStderr), "ipset del azure-tt-local-pods 10.224.0.46 -exist"),
			want: true,
		},
		{
			name: "unrelated ipset failure is not treated as a missing set",
			err:  errors.New("exit status 1:ipset v7.15: Kernel error received: ipset protocol error"),
			want: false,
		},
		{
			name: "permission failure is not treated as a missing set",
			err:  errors.New("exit status 1:ipset v7.15: Operation not permitted"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isSetNotFoundErr(tt.err))
		})
	}
}

func TestDefaultIpsetClientDel(t *testing.T) {
	tests := []struct {
		name      string
		execErr   error
		wantErr   bool
		errSubstr string
	}{
		{
			name:    "successful delete returns nil",
			execErr: nil,
			wantErr: false,
		},
		{
			name: "missing set is treated as success",
			// `-exist` does not cover a missing set, so the command fails. The
			// entry cannot be present if the set is gone, so DEL must not error
			// or CNI DEL would be retried forever after a node reboot.
			execErr: errors.New(ipsetSetNotFoundStderr),
			wantErr: false,
		},
		{
			name:      "other ipset failures are still returned",
			execErr:   errors.New("exit status 1:ipset v7.15: Operation not permitted"),
			wantErr:   true,
			errSubstr: "Operation not permitted",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotArgs []string
			exec := platform.NewMockExecClient(false)
			exec.SetExecCommand(func(_ string, args ...string) (string, error) {
				gotArgs = args
				return "", tt.execErr
			})

			client := newDefaultTransparentTunnelIpsetClient(exec)
			err := client.Del("azure-tt-local-pods", "10.224.0.46")

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errSubstr)
			} else {
				require.NoError(t, err)
			}

			assert.Equal(t,
				[]string{ipsetOpDel, "azure-tt-local-pods", "10.224.0.46", "-exist"},
				gotArgs,
				"delete should pass -exist so a missing entry is also a no-op")
		})
	}
}

// A missing set must not stall cleanup of the remaining pod IPs or of the
// downstream iptables rule, so the whole delete path is exercised end to end.
func TestDeleteTransparentTunnelRulesToleratesMissingIpset(t *testing.T) {
	exec := platform.NewMockExecClient(false)
	exec.SetExecCommand(func(_ string, args ...string) (string, error) {
		if len(args) > 0 && args[0] == ipsetOpDel {
			return "", errors.New(ipsetSetNotFoundStderr)
		}
		return "", nil
	})

	iptMock := &transparentTunnelMockIPTablesClient{}
	client := &TransparentTunnelEndpointClient{
		ipsetClient:    newDefaultTransparentTunnelIpsetClient(exec),
		iptablesClient: iptMock,
	}

	ep := &endpoint{
		Id:         "test-endpoint",
		HostIfName: testHostVethName,
		IPAddresses: []net.IPNet{
			{IP: net.IPv4(10, 224, 0, 46), Mask: net.CIDRMask(32, 32)},
		},
	}

	err := client.deleteTransparentTunnelRules(ep)
	require.NoError(t, err, "a missing ipset must not fail CNI DEL")

	assert.NotEmpty(t, iptMock.deleteCalls,
		"cleanup must continue to the fwmark rule after the ipset is found missing")
}

func TestDefaultIpsetClientCreateAndAdd(t *testing.T) {
	t.Run("create passes -exist so an existing set is a no-op", func(t *testing.T) {
		var gotArgs []string
		exec := platform.NewMockExecClient(false)
		exec.SetExecCommand(func(_ string, args ...string) (string, error) {
			gotArgs = args
			return "", nil
		})

		client := newDefaultTransparentTunnelIpsetClient(exec)
		require.NoError(t, client.Create("azure-tt-local-pods", "hash:ip"))
		assert.Equal(t, []string{"create", "azure-tt-local-pods", "hash:ip", "-exist"}, gotArgs)
	})

	t.Run("add surfaces failures", func(t *testing.T) {
		exec := platform.NewMockExecClient(false)
		exec.SetExecCommand(func(_ string, _ ...string) (string, error) {
			return "", errors.New("exit status 1:ipset v7.15: Operation not permitted")
		})

		client := newDefaultTransparentTunnelIpsetClient(exec)
		err := client.Add("azure-tt-local-pods", "10.224.0.46")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Operation not permitted")
	})
}
