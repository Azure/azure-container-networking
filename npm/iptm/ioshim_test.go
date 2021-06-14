package iptm

import (
	"bufio"
	"io"
	"io/fs"
	"os"
	"strings"
	"testing"
	"testing/fstest"

	testingutils "github.com/Azure/azure-container-networking/test/utils"
	"github.com/stretchr/testify/require"
)

const (
	testFileName = "iptables-test.conf"
)

var (
	testIPTablesData = `Chain INPUT (policy ACCEPT)
target     prot opt source               destination         

Chain FORWARD (policy ACCEPT)
target     prot opt source               destination         

Chain OUTPUT (policy ACCEPT)
target     prot opt source               destination 
`
)

type fakeIptOperationShim struct {
	m  fstest.MapFS
	fo fs.File
}

// TODO: we can use this method down the road for testing iptables restore
func NewFakeIptOperationShim() *fakeIptOperationShim {
	return &fakeIptOperationShim{
		m: fstest.MapFS{

			testFileName: &fstest.MapFile{
				Data: []byte(testIPTablesData),
			},
		},
	}
}

func (f *fakeIptOperationShim) LoadExistingIPTablesState([]testingutils.TestCmd) {
}

func (f *fakeIptOperationShim) GrabIptablesLocks() (*os.File, error) {
	return &os.File{}, nil
}

func (f *fakeIptOperationShim) SaveConfigFile(configFile string) (io.Writer, error) {
	return nil, nil
}

func (f *fakeIptOperationShim) OpenConfigFile(configFile string) (io.Reader, error) {
	fo, err := f.m.Open(testFileName)
	f.fo = fo
	return f.fo, err
}

func (f *fakeIptOperationShim) CloseConfigFile() error {
	f.fo.Close()
	return nil
}

func TestFakeIOShim(t *testing.T) {
	fake := NewFakeIptOperationShim()
	f, err := fake.OpenConfigFile(testFileName)
	require.NoError(t, err)

	s := bufio.NewScanner(f)
	res := ""
	for s.Scan() {
		res += s.Text()
	}

	require.Equal(t, strings.Replace(testIPTablesData, "\n", "", -1), res)
}
