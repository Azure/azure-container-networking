package cniconflist_test

import (
	"bytes"
	"os"
	"testing"

	"github.com/Azure/azure-container-networking/cns/cniconflist"
	"github.com/stretchr/testify/assert"
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
	assert.NoError(t, err)

	fixtureBytes, err := os.ReadFile(fixture)
	assert.NoError(t, err)

	// remove newlines and carriage returns in case these UTs are running on Windows
	assert.Equal(t, removeNewLines(fixtureBytes), removeNewLines(buffer.Bytes()))
}

func TestGenerateDualStackOverlayConflist(t *testing.T) {
	fixture := "testdata/fixtures/azure-linux-swift-dualstack-overlay.conflist"

	buffer := new(bytes.Buffer)
	g := cniconflist.DualStackOverlayGenerator{Writer: &bufferWriteCloser{buffer}}
	err := g.Generate()
	assert.NoError(t, err)

	fixtureBytes, err := os.ReadFile(fixture)
	assert.NoError(t, err)

	// remove newlines and carriage returns in case these UTs are running on Windows
	assert.Equal(t, removeNewLines(fixtureBytes), removeNewLines(buffer.Bytes()))
}

func TestGenerateOverlayConflist(t *testing.T) {
	fixture := "testdata/fixtures/azure-linux-swift-overlay.conflist"

	buffer := new(bytes.Buffer)
	g := cniconflist.OverlayGenerator{Writer: &bufferWriteCloser{buffer}}
	err := g.Generate()
	assert.NoError(t, err)

	fixtureBytes, err := os.ReadFile(fixture)
	assert.NoError(t, err)

	// remove newlines and carriage returns in case these UTs are running on Windows
	assert.Equal(t, removeNewLines(fixtureBytes), removeNewLines(buffer.Bytes()))
}

func TestGenerateCiliumConflist(t *testing.T) {
	fixture := "testdata/fixtures/cilium.conflist"

	buffer := new(bytes.Buffer)
	g := cniconflist.CiliumGenerator{Writer: &bufferWriteCloser{buffer}}
	err := g.Generate()
	assert.NoError(t, err)

	fixtureBytes, err := os.ReadFile(fixture)
	assert.NoError(t, err)

	// remove newlines and carriage returns in case these UTs are running on Windows
	assert.Equal(t, removeNewLines(fixtureBytes), removeNewLines(buffer.Bytes()))
}

func TestGenerateSWIFTConflist(t *testing.T) {
	fixture := "testdata/fixtures/azure-linux-swift.conflist"

	buffer := new(bytes.Buffer)
	g := cniconflist.SWIFTGenerator{Writer: &bufferWriteCloser{buffer}}
	err := g.Generate()
	assert.NoError(t, err)

	fixtureBytes, err := os.ReadFile(fixture)
	assert.NoError(t, err)

	// remove newlines and carriage returns in case these UTs are running on Windows
	assert.Equal(t, removeNewLines(fixtureBytes), removeNewLines(buffer.Bytes()))
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
