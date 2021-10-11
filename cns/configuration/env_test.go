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
