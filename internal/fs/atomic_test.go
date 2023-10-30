package fs_test

import (
	"bufio"
	"os"
	"testing"

	"github.com/Azure/azure-container-networking/internal/fs"
	"github.com/stretchr/testify/require"
)

func TestAtomicWriterFileExists(t *testing.T) {
	file := "testdata/data.txt"
	w, err := fs.NewAtomicWriter(file)
	require.NoError(t, err, "error creating atomic writer")

	// atomic writer should copy existing file with -old suffix
	_, err = os.Stat(file)
	require.NoError(t, err, "error stating existing file")
	_, err = os.Stat(file + "-old")
	require.NoError(t, err, "error stating old file")

	data := []byte("some test data")
	_, err = w.Write(data)
	require.NoError(t, err, "error writing with atomic writer")

	err = w.Close()
	require.NoError(t, err, "error closing atomic writer")

	dataFile, err := os.Open(file)
	require.NoError(t, err, "error opening testdata file")

	line, _, err := bufio.NewReader(dataFile).ReadLine()
	require.NoError(t, err, "error reading written file")

	require.Equal(t, data, line, "testdata doesn't match expected")
}

func TestAtomicWriterNewFile(t *testing.T) {
	file := "testdata/newdata.txt"

	// if the file exists before running this test, remove it
	err := os.Remove(file)
	require.NoError(t, ignoreDoesNotExistError(err), "error removing file")

	w, err := fs.NewAtomicWriter(file)
	require.NoError(t, err, "error creating atomic writer")

	data := []byte("some test data")
	_, err = w.Write(data)
	require.NoError(t, err, "error writing with atomic writer")

	err = w.Close()
	require.NoError(t, err, "error closing atomic writer")

	dataFile, err := os.Open(file)
	require.NoError(t, err, "error opening testdata file")

	line, _, err := bufio.NewReader(dataFile).ReadLine()
	require.NoError(t, err, "error reading written file")

	require.Equal(t, data, line, "testdata doesn't match expected")
}

func ignoreDoesNotExistError(err error) error {
	if os.IsNotExist(err) {
		return nil
	}

	return err
}
