package fakes

import (
	"io"
	"io/fs"
	"os"
	"testing/fstest"

	testingutils "github.com/Azure/azure-container-networking/test/utils"
)

type FakeIptOperationShim struct {
	iptablesSaveState []testingutils.TestCmd
	m                 fstest.MapFS
	fo                fs.File
}

func NewFakeIptOperationShim() *FakeIptOperationShim {
	return &FakeIptOperationShim{
		m: fstest.MapFS{
			"iptables-test.conf": &fstest.MapFile{
				Data: []byte("test-iptables-command"),
			},
			"a": &fstest.MapFile{Data: []byte("text")},
		},
	}
}

func (f *FakeIptOperationShim) LoadExistingIPTablesState([]testingutils.TestCmd) {

}

func (f *FakeIptOperationShim) GrabIptablesLocks() (*os.File, error) {
	return &os.File{}, nil
}

func (f *FakeIptOperationShim) SaveConfigFile(configFile string) (io.Writer, error) {
	return nil, nil
}

func (f *FakeIptOperationShim) OpenConfigFile(configFile string) (io.Reader, error) {
	fo, err := f.m.Open("iptables-test.conf")
	f.fo = fo
	return f.fo, err
}

func (f *FakeIptOperationShim) CloseConfigFile() error {
	f.fo.Close()
	return nil
}
