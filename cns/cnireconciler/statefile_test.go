package cnireconciler

import (
	"os"
	"path"
	"testing"

	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteObjectToFile(t *testing.T) {
	name := "testdata/test"
	err := os.MkdirAll(path.Dir(name), 0666)
	require.NoError(t, err)

	_, err = os.Stat(name)
	require.ErrorIs(t, err, os.ErrNotExist)

	// create empty file
	_, err = os.Create(name)
	require.NoError(t, err)
	defer os.Remove(name)

	// check it's empty
	fi, _ := os.Stat(name)
	assert.Equal(t, fi.Size(), int64(0))

	// populate
	require.NoError(t, writeObjectToFile(name))

	// read
	b, err := os.ReadFile(name)
	require.NoError(t, err)
	assert.Equal(t, string(b), "{}")
}

func init() {
	logger.InitLogger("testlogs", 0, 0, "./")
}
