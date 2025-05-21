package iptables

import (
	"errors"
	"testing"

	"github.com/Azure/azure-container-networking/platform"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type validationCase struct {
	cmd   string
	doErr bool
}

var (
	errMockPlatform    = errors.New("mock pl error")
	errExtraneousCalls = errors.New("function called too many times")
)

// GenerateValidateFunc takes in a slice of expected calls and intended responses for each time the returned function is called
// For example, if expectedCmds has one validationCase, the first call of the func returned will check the command
// passed in matches the first validationCase's command (fails test if not), and return an error if the first validationCase has doErr as true
// The second call will use the second validation case in the slice to check against the cmd passed in and so on
// If we call this function more times than the number of elements in expectedCmds, errExtraneousCalls is returned
func GenerateValidationFunc(t *testing.T, expectedCmds []validationCase) func(cmd string) (string, error) {
	curr := 0

	ret := func(cmd string) (string, error) {
		if curr >= len(expectedCmds) {
			return "", errExtraneousCalls
		}
		expected := expectedCmds[curr]
		curr++

		require.Equal(t, expected.cmd, cmd, "command run does not match expected")

		if expected.doErr {
			return "", errMockPlatform
		}
		return "", nil
	}

	return ret
}

func TestGenerateValidationFunc(t *testing.T) {
	mockPL := platform.NewMockExecClient(false)
	fn := GenerateValidationFunc(t, []validationCase{
		{
			cmd:   "echo hello",
			doErr: true,
		},
	})
	mockPL.SetExecRawCommand(fn)

	_, err := mockPL.ExecuteRawCommand("echo hello")
	require.Error(t, err)

	_, err = mockPL.ExecuteRawCommand("echo hello")
	require.ErrorIs(t, err, errExtraneousCalls)
}

func TestRunCmd(t *testing.T) {
	mockPL := platform.NewMockExecClient(false)
	client := &Client{
		pl: mockPL,
	}
	mockPL.SetExecRawCommand(
		GenerateValidationFunc(t, []validationCase{
			{
				cmd:   "iptables -w 60 -L",
				doErr: false,
			},
		}),
	)

	err := client.RunCmd(V4, "-L")
	require.NoError(t, err)
}

func TestCreateChain(t *testing.T) {
	mockPL := platform.NewMockExecClient(false)
	client := &Client{
		pl: mockPL,
	}
	mockPL.SetExecRawCommand(
		GenerateValidationFunc(t, []validationCase{
			{
				cmd:   "iptables -w 60 -t filter -nL AZURECNIINPUT",
				doErr: true,
			},
			{
				cmd:   "iptables -w 60 -t filter -N AZURECNIINPUT",
				doErr: false,
			},
		}),
	)

	err := client.CreateChain(V4, Filter, CNIInputChain)
	require.NoError(t, err)
}

func TestInsertIptableRule(t *testing.T) {
	mockPL := platform.NewMockExecClient(false)
	client := &Client{
		pl: mockPL,
	}

	mockPL.SetExecRawCommand(
		GenerateValidationFunc(t, []validationCase{
			// iptables succeeds
			{
				cmd:   "iptables -w 60 -t filter -C AZURECNIINPUT -p tcp --dport 70 -j ACCEPT",
				doErr: true,
			},
			{
				cmd:   "iptables -w 60 -t filter -I AZURECNIINPUT 1 -p tcp --dport 70 -j ACCEPT",
				doErr: false,
			},
			{
				cmd:   "iptables -w 60 -t filter -C AZURECNIINPUT -p tcp --dport 70 -j ACCEPT",
				doErr: false,
			},
			// iptables fails silently
			{
				cmd:   "iptables -w 60 -t filter -C AZURECNIINPUT -p tcp --dport 80 -j ACCEPT",
				doErr: true,
			},
			{
				cmd:   "iptables -w 60 -t filter -I AZURECNIINPUT 1 -p tcp --dport 80 -j ACCEPT",
				doErr: false,
			},
			{
				cmd:   "iptables -w 60 -t filter -C AZURECNIINPUT -p tcp --dport 80 -j ACCEPT",
				doErr: true,
			},
			// iptables finds rule already
			{
				cmd:   "iptables -w 60 -t filter -C AZURECNIINPUT -p tcp --dport 90 -j ACCEPT",
				doErr: false,
			},
		}),
	)
	// iptables succeeds
	err := client.InsertIptableRule(V4, Filter, CNIInputChain, "-p tcp --dport 70", Accept)
	require.NoError(t, err)
	// iptables fails silently
	err = client.InsertIptableRule(V4, Filter, CNIInputChain, "-p tcp --dport 80", Accept)
	require.ErrorIs(t, err, errCouldNotValidateRuleExists)
	// iptables finds rule already
	err = client.InsertIptableRule(V4, Filter, CNIInputChain, "-p tcp --dport 90", Accept)
	require.NoError(t, err)
}

func TestAppendIptableRule(t *testing.T) {
	mockPL := platform.NewMockExecClient(false)
	client := &Client{
		pl: mockPL,
	}
	mockPL.SetExecRawCommand(
		GenerateValidationFunc(t, []validationCase{
			// iptables succeeds
			{
				cmd:   "iptables -w 60 -t filter -C AZURECNIINPUT -p tcp --dport 70 -j ACCEPT",
				doErr: true,
			},
			{
				cmd:   "iptables -w 60 -t filter -A AZURECNIINPUT -p tcp --dport 70 -j ACCEPT",
				doErr: false,
			},
			{
				cmd:   "iptables -w 60 -t filter -C AZURECNIINPUT -p tcp --dport 70 -j ACCEPT",
				doErr: false,
			},
			// iptables fails silently
			{
				cmd:   "iptables -w 60 -t filter -C AZURECNIINPUT -p tcp --dport 80 -j ACCEPT",
				doErr: true,
			},
			{
				cmd:   "iptables -w 60 -t filter -A AZURECNIINPUT -p tcp --dport 80 -j ACCEPT",
				doErr: false,
			},
			{
				cmd:   "iptables -w 60 -t filter -C AZURECNIINPUT -p tcp --dport 80 -j ACCEPT",
				doErr: true,
			},
			// iptables finds rule already
			{
				cmd:   "iptables -w 60 -t filter -C AZURECNIINPUT -p tcp --dport 90 -j ACCEPT",
				doErr: false,
			},
		}),
	)
	// iptables succeeds
	err := client.AppendIptableRule(V4, Filter, CNIInputChain, "-p tcp --dport 70", Accept)
	require.NoError(t, err)
	// iptables fails silently
	err = client.AppendIptableRule(V4, Filter, CNIInputChain, "-p tcp --dport 80", Accept)
	require.ErrorIs(t, errCouldNotValidateRuleExists, err)
	// iptables finds rule already
	err = client.AppendIptableRule(V4, Filter, CNIInputChain, "-p tcp --dport 90", Accept)
	require.NoError(t, err)
}

func TestDeleteIptableRule(t *testing.T) {
	mockPL := platform.NewMockExecClient(false)
	client := &Client{
		pl: mockPL,
	}
	mockPL.SetExecRawCommand(
		GenerateValidationFunc(t, []validationCase{
			{
				cmd:   "iptables -w 60 -t filter -D AZURECNIINPUT -p tcp --dport 80 -j ACCEPT",
				doErr: false,
			},
		}),
	)

	err := client.DeleteIptableRule(V4, Filter, CNIInputChain, "-p tcp --dport 80", Accept)
	require.NoError(t, err)
}

func TestChainExists(t *testing.T) {
	mockPL := platform.NewMockExecClient(false)
	client := &Client{
		pl: mockPL,
	}
	mockPL.SetExecRawCommand(
		GenerateValidationFunc(t, []validationCase{
			{
				cmd:   "iptables -w 60 -t filter -nL AZURECNIINPUT",
				doErr: true,
			},
		}),
	)

	result := client.ChainExists(V4, Filter, CNIInputChain)
	assert.False(t, result)
}

func TestRuleExists(t *testing.T) {
	mockPL := platform.NewMockExecClient(false)
	client := &Client{
		pl: mockPL,
	}
	mockPL.SetExecRawCommand(
		GenerateValidationFunc(t, []validationCase{
			{
				cmd:   "iptables -w 60 -t filter -C AZURECNIINPUT -p tcp --dport 80 -j ACCEPT",
				doErr: true,
			},
		}),
	)

	result := client.RuleExists(V4, Filter, CNIInputChain, "-p tcp --dport 80", Accept)
	assert.False(t, result)
}
