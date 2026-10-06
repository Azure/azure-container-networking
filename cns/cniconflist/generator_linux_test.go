package cniconflist_test

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"github.com/Azure/azure-container-networking/cns/cniconflist"
	"github.com/stretchr/testify/require"
)

type bufferWriteCloser struct {
	*bytes.Buffer
}

func (b *bufferWriteCloser) Close() error {
	return nil
}

func TestGenerateV4OverlayConflist(t *testing.T) {
	fixture := "testdata/fixtures/azure-linux-swift-v4overlay.conflist"

	buffer := new(bytes.Buffer)
	g := cniconflist.V4OverlayGenerator{Writer: &bufferWriteCloser{buffer}}
	err := g.Generate()
	require.NoError(t, err)

	fixtureBytes, err := os.ReadFile(fixture)
	require.NoError(t, err)

	// remove newlines and carriage returns in case these UTs are running on Windows
	require.Equal(t, removeNewLines(fixtureBytes), removeNewLines(buffer.Bytes()))
}

func TestGenerateDualStackOverlayConflist(t *testing.T) {
	fixture := "testdata/fixtures/azure-linux-swift-dualstack-overlay.conflist"

	buffer := new(bytes.Buffer)
	g := cniconflist.DualStackOverlayGenerator{Writer: &bufferWriteCloser{buffer}}
	err := g.Generate()
	require.NoError(t, err)

	fixtureBytes, err := os.ReadFile(fixture)
	require.NoError(t, err)

	// remove newlines and carriage returns in case these UTs are running on Windows
	require.Equal(t, removeNewLines(fixtureBytes), removeNewLines(buffer.Bytes()))
}

func TestGenerateOverlayConflist(t *testing.T) {
	fixture := "testdata/fixtures/azure-linux-swift-overlay.conflist"

	buffer := new(bytes.Buffer)
	g := cniconflist.OverlayGenerator{Writer: &bufferWriteCloser{buffer}}
	err := g.Generate()
	require.NoError(t, err)

	fixtureBytes, err := os.ReadFile(fixture)
	require.NoError(t, err)

	// remove newlines and carriage returns in case these UTs are running on Windows
	require.Equal(t, removeNewLines(fixtureBytes), removeNewLines(buffer.Bytes()))
}

func TestGenerateCiliumConflist(t *testing.T) {
	fixture := "testdata/fixtures/cilium.conflist"

	buffer := new(bytes.Buffer)
	g := cniconflist.CiliumGenerator{Writer: &bufferWriteCloser{buffer}}
	err := g.Generate()
	require.NoError(t, err)

	fixtureBytes, err := os.ReadFile(fixture)
	require.NoError(t, err)

	// remove newlines and carriage returns in case these UTs are running on Windows
	require.Equal(t, removeNewLines(fixtureBytes), removeNewLines(buffer.Bytes()))
}

func TestGenerateSWIFTConflist(t *testing.T) {
	fixture := "testdata/fixtures/azure-linux-swift.conflist"

	buffer := new(bytes.Buffer)
	g := cniconflist.SWIFTGenerator{Writer: &bufferWriteCloser{buffer}}
	err := g.Generate()
	require.NoError(t, err)

	fixtureBytes, err := os.ReadFile(fixture)
	require.NoError(t, err)

	// remove newlines and carriage returns in case these UTs are running on Windows
	require.Equal(t, removeNewLines(fixtureBytes), removeNewLines(buffer.Bytes()))
}

func TestGenerateAzurecniCiliumConflist(t *testing.T) {
	fixture := "testdata/fixtures/azure-chained-cilium.conflist"

	buffer := new(bytes.Buffer)
	g := cniconflist.AzureCNIChainedCiliumGenerator{Writer: &bufferWriteCloser{buffer}}
	err := g.Generate()
	require.NoError(t, err)

	fixtureBytes, err := os.ReadFile(fixture)
	require.NoError(t, err)

	// remove newlines and carriage returns in case these UTs are running on Windows
	require.Equal(t, removeNewLines(fixtureBytes), removeNewLines(buffer.Bytes()))
}

func TestGeneratorsEmitCNSSocketPath(t *testing.T) {
	const socketPath = "/var/run/azure-cns/cns.sock"
	tests := []struct {
		name string
		new  func(*bufferWriteCloser) interface{ Generate() error }
	}{
		{name: "v4 overlay", new: func(writer *bufferWriteCloser) interface{ Generate() error } {
			return &cniconflist.V4OverlayGenerator{Writer: writer, CNSSocketPath: socketPath}
		}},
		{name: "dual-stack overlay", new: func(writer *bufferWriteCloser) interface{ Generate() error } {
			return &cniconflist.DualStackOverlayGenerator{Writer: writer, CNSSocketPath: socketPath}
		}},
		{name: "overlay", new: func(writer *bufferWriteCloser) interface{ Generate() error } {
			return &cniconflist.OverlayGenerator{Writer: writer, CNSSocketPath: socketPath}
		}},
		{name: "swift", new: func(writer *bufferWriteCloser) interface{ Generate() error } {
			return &cniconflist.SWIFTGenerator{Writer: writer, CNSSocketPath: socketPath}
		}},
		{name: "azure cni chained cilium", new: func(writer *bufferWriteCloser) interface{ Generate() error } {
			return &cniconflist.AzureCNIChainedCiliumGenerator{Writer: writer, CNSSocketPath: socketPath}
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buffer := new(bytes.Buffer)
			require.NoError(t, tt.new(&bufferWriteCloser{buffer}).Generate())

			var conflist struct {
				Plugins []struct {
					CNSSocketPath string `json:"cnsSocketPath"`
				} `json:"plugins"`
			}
			require.NoError(t, json.Unmarshal(buffer.Bytes(), &conflist))
			require.Equal(t, socketPath, conflist.Plugins[0].CNSSocketPath)
		})
	}
}

// removeNewLines will remove the newlines and carriage returns from the byte slice
func removeNewLines(b []byte) []byte {
	var bb []byte //nolint:prealloc // can't prealloc since we don't know how many bytes will get removed

	for _, bs := range b {
		if bs == byte('\n') || bs == byte('\r') {
			continue
		}

		bb = append(bb, bs)
	}

	return bb
}
