package cni

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteObjectToFile(t *testing.T) {
	baseDir := t.TempDir()
	name := filepath.Join(baseDir, "testdata", "test")

	err := os.MkdirAll(filepath.Dir(name), 0o755)
	require.NoError(t, err)
	defer os.RemoveAll(filepath.Join(baseDir, "testdata"))

	_, err = os.Stat(name)
	require.ErrorIs(t, err, os.ErrNotExist)

	// create empty file
	f, err := os.Create(name)
	require.NoError(t, err)
	f.Close() // required on windows
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
