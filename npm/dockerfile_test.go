package npm

import (
	"os"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	expectedNPMGoImage      = "mcr.microsoft.com/oss/go/microsoft/golang:1.26.5@sha256:ea712a1aaf80306c19ff842ba0bfcb9ad360afd8143e70044e0d0bd6d6899887"
	expectedServerCoreImage = "mcr.microsoft.com/windows/servercore:ltsc2022@sha256:6b43c814ed2a800563083ce3193e5f1951d4d6a18fd2879ff45173851db82bd5"
)

func TestNPMDockerfileImagePins(t *testing.T) {
	linuxDockerfile := readDockerfile(t, "linux.Dockerfile")
	unsignedDockerfile := readDockerfile(t, "windows.Dockerfile")
	signedDockerfile := readDockerfile(t, "../.pipelines/build/dockerfiles/npm.Dockerfile")

	require.Contains(t, linuxDockerfile, expectedNPMGoImage)
	require.Contains(t, unsignedDockerfile, expectedNPMGoImage)
	require.Equal(t, expectedServerCoreImage, serverCoreImage(t, unsignedDockerfile))
	require.Equal(t, expectedServerCoreImage, serverCoreImage(t, signedDockerfile))
}

func readDockerfile(t *testing.T, path string) string {
	t.Helper()

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(content)
}

func serverCoreImage(t *testing.T, dockerfile string) string {
	t.Helper()

	match := regexp.MustCompile(`mcr\.microsoft\.com/windows/servercore:ltsc2022@sha256:[a-f0-9]+`).FindString(dockerfile)
	require.NotEmpty(t, match)
	return match
}
