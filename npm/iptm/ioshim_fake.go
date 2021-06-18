package iptm

import (
	"io"
	"io/fs"
	"testing/fstest"

	testingutils "github.com/Azure/azure-container-networking/test/utils"
)

type fakeIptOperationShim struct {
	configname string
	m          fstest.MapFS
	fo         fs.File
}

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

func (f *fakeIptOperationShim) SetTestData(configname, configdata string) {
	f.configname = configname
	f.m[configname] = &fstest.MapFile{
		Data: []byte(configdata),
	}
}

func (f *fakeIptOperationShim) LoadExistingIPTablesState([]testingutils.TestCmd) {
}

func (f *fakeIptOperationShim) lockIptables() error {
	return nil
}

func (f *fakeIptOperationShim) unlockIptables() error {
	return nil
}

func (f *fakeIptOperationShim) createConfigFile(configFile string) (io.Writer, error) {
	return nil, nil
}

func (f *fakeIptOperationShim) openConfigFile(configFile string) (io.Reader, error) {
	fo, err := f.m.Open(configFile)
	f.fo = fo
	return f.fo, err
}

func (f *fakeIptOperationShim) closeConfigFile() error {
	if f.fo != nil {
		return f.fo.Close()
	}
	return nil
}
