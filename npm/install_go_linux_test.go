package npm

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestNPMGoImageSelection(t *testing.T) {
	const (
		linuxOS         = "linux"
		windowsOS       = "windows"
		npmComponent    = "npm"
		linuxDockerfile = "npm/linux.Dockerfile"
		linuxImage      = "mcr.microsoft.com/oss/go/microsoft/golang:linux-test"
		windowsImage    = "mcr.microsoft.com/oss/go/microsoft/golang:windows-test"
		override        = "mcr.microsoft.com/oss/go/microsoft/golang:override-test"
	)
	script := strings.ReplaceAll(readDockerfile(t, "../.pipelines/build/scripts/install-go.sh"), "\r\n", "\n")

	tests := []struct {
		name        string
		component   string
		os          string
		dockerfile  string
		content     string
		override    string
		wantImage   string
		wantFailure bool
	}{
		{
			name: linuxOS, component: npmComponent, os: linuxOS,
			dockerfile: linuxDockerfile,
			content:    "FROM " + linuxImage + " AS builder\n", wantImage: linuxImage,
		},
		{
			name: windowsOS, component: npmComponent, os: windowsOS,
			dockerfile: "npm/windows.Dockerfile",
			content:    "FROM --platform=linux/amd64 " + windowsImage + " AS builder\n", wantImage: windowsImage,
		},
		{
			name: "default OS", component: npmComponent,
			dockerfile: linuxDockerfile,
			content:    "FROM " + linuxImage + " AS builder\n", wantImage: linuxImage,
		},
		{
			name: "missing Linux Dockerfile", component: npmComponent, os: linuxOS, wantFailure: true,
		},
		{
			name: "missing Windows Dockerfile", component: npmComponent, os: windowsOS, wantFailure: true,
		},
		{
			name: "missing Go builder", component: npmComponent, os: linuxOS,
			dockerfile: linuxDockerfile, content: "FROM scratch\n", wantFailure: true,
		},
		{
			name: "explicit override", component: npmComponent, os: linuxOS,
			override: override, wantImage: override,
		},
		{
			name: "non-NPM Dockerfile", component: "cns", os: linuxOS,
			dockerfile: "cns/Dockerfile",
			content:    "FROM --platform=linux/amd64 " + linuxImage + " AS builder\n", wantImage: linuxImage,
		},
		{
			name: "non-NPM fallback", component: "cns", os: linuxOS,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			if tt.dockerfile != "" {
				path := filepath.Join(root, tt.dockerfile)
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(tt.content), 0o600); err != nil {
					t.Fatal(err)
				}
			}

			// Stop at the first installation command without changing the host or pulling an image.
			cmd := exec.CommandContext(t.Context(), "bash", "-c", `
sudo() { printf '%s\n' "$MSFT_GO_IMAGE" "$DEFAULT_IMAGE"; exit 42; }
crane() { exit 43; }
source /dev/stdin
`)
			cmd.Stdin = strings.NewReader(script)
			cmd.Env = append(os.Environ(), "REPO_ROOT="+root, "name="+tt.component, "OS="+tt.os, "MSFT_GO_IMAGE="+tt.override)
			output, err := cmd.Output()
			var exitErr *exec.ExitError
			if !errors.As(err, &exitErr) {
				t.Fatalf("expected the installer to stop before modifying the host, got %v", err)
			}
			if tt.wantFailure {
				if exitErr.ExitCode() != 1 || len(output) != 0 {
					t.Fatalf("missing NPM compiler reached installation: exit %d, output %q, stderr %s", exitErr.ExitCode(), output, exitErr.Stderr)
				}
				for _, message := range []string{"could not resolve the golang image", "npm/" + tt.os + ".Dockerfile", "refusing to fall back"} {
					if !strings.Contains(string(exitErr.Stderr), message) {
						t.Errorf("missing diagnostic %q in %s", message, exitErr.Stderr)
					}
				}
				return
			}
			if exitErr.ExitCode() != 42 {
				t.Fatalf("compiler selection failed: %s", exitErr.Stderr)
			}
			images := strings.Fields(string(output))
			if len(images) != 2 {
				t.Fatalf("expected selected and fallback images, got %q", output)
			}
			want := tt.wantImage
			if want == "" {
				want = images[1]
			}
			if images[0] != want {
				t.Errorf("selected %q, want %q", images[0], want)
			}
		})
	}
}
