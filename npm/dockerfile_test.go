package npm

import (
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// Keep the signed and unsigned NPM image contracts in sync.

const (
	unsignedLinuxDockerfile   = "linux.Dockerfile"
	unsignedWindowsDockerfile = "windows.Dockerfile"
	signedDockerfile          = "../.pipelines/build/dockerfiles/npm.Dockerfile"

	// minGoBuilderVersion is the MS Go toolchain floor required for the Go/stdlib
	// CVE remediation. Asserting it — not just that both Dockerfiles match — makes
	// a synchronized rollback of both files to an older, vulnerable Go tag fail the
	// test. Raise this when a newer toolchain is required to clear a CVE.
	minGoBuilderVersion = "1.26.9"
)

var requiredLinuxPackages = []string{
	"SymCrypt",
	"SymCrypt-OpenSSL",
	"bash",
	"ca-certificates",
	"grep",
	"ipset",
	"iptables",
	"openssl-libs",
}

var (
	goBuilderRe  = regexp.MustCompile(`mcr\.microsoft\.com/oss/go/microsoft/golang:(\S+?)\s+AS builder`)
	servercoreRe = regexp.MustCompile(`mcr\.microsoft\.com/windows/servercore:ltsc2022@(sha256:[0-9a-f]{64})`)
	linuxBaseRe  = regexp.MustCompile(`(?im)^FROM (?:--platform=\S+ )?(mcr\.microsoft\.com/azurelinux/base/core:3\.0@sha256:[0-9a-f]{64}) AS linux\s*$`)
	tdnfBlockRe  = regexp.MustCompile(`(?s)tdnf install -y(.*?)&& tdnf clean all`)
)

func readDockerfile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return string(b)
}

// TestNPMDockerfileGoBuilderPinned verifies both NPM builder images use the same
// pinned MS Go toolchain, so the Linux and Windows npm binaries are built with an
// identical (CVE-patched) compiler.
func TestNPMDockerfileGoBuilderPinned(t *testing.T) {
	linux := goBuilderRe.FindStringSubmatch(readDockerfile(t, unsignedLinuxDockerfile))
	windows := goBuilderRe.FindStringSubmatch(readDockerfile(t, unsignedWindowsDockerfile))
	if linux == nil {
		t.Fatalf("%s: no MS Go builder image found", unsignedLinuxDockerfile)
	}
	if windows == nil {
		t.Fatalf("%s: no MS Go builder image found", unsignedWindowsDockerfile)
	}
	if !strings.HasSuffix(linux[1], "-azurelinux3.0") {
		t.Errorf("%s: expected an Azure Linux 3 builder, got %s", unsignedLinuxDockerfile, linux[1])
	}
	if strings.TrimSuffix(linux[1], "-azurelinux3.0") != windows[1] {
		t.Errorf("Go builder drift: %s uses %s but %s uses %s",
			unsignedLinuxDockerfile, linux[1], unsignedWindowsDockerfile, windows[1])
	}
	for _, f := range []struct{ name, version string }{
		{unsignedLinuxDockerfile, linux[1]},
		{unsignedWindowsDockerfile, windows[1]},
	} {
		if compareGoVersions(f.version, minGoBuilderVersion) < 0 {
			t.Errorf("%s pins Go builder %s, want >= %s (CVE remediation floor)", f.name, f.version, minGoBuilderVersion)
		}
	}
}

// compareGoVersions compares dotted numeric versions (e.g. "1.26.7"). It returns
// -1 if a < b, 0 if equal, and 1 if a > b. Non-numeric suffixes are ignored.
func compareGoVersions(a, b string) int {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(as) || i < len(bs); i++ {
		var ai, bi int
		if i < len(as) {
			ai = leadingInt(as[i])
		}
		if i < len(bs) {
			bi = leadingInt(bs[i])
		}
		if ai != bi {
			if ai < bi {
				return -1
			}
			return 1
		}
	}
	return 0
}

func leadingInt(s string) int {
	end := 0
	for end < len(s) && s[end] >= '0' && s[end] <= '9' {
		end++
	}
	n, _ := strconv.Atoi(s[:end])
	return n
}

// TestNPMDockerfileWindowsBasePinned verifies the signed and unsigned Windows
// images both pin the Server Core base by digest and pin the same digest. The
// digest value itself is not asserted: Dependabot refreshes it in both
// Dockerfiles on every Server Core security release and cannot update a
// constant in this test.
func TestNPMDockerfileWindowsBasePinned(t *testing.T) {
	unsigned := servercoreRe.FindStringSubmatch(readDockerfile(t, unsignedWindowsDockerfile))
	signed := servercoreRe.FindStringSubmatch(readDockerfile(t, signedDockerfile))
	if unsigned == nil {
		t.Fatalf("%s: no pinned servercore ltsc2022 digest found", unsignedWindowsDockerfile)
	}
	if signed == nil {
		t.Fatalf("%s: no pinned servercore ltsc2022 digest found", signedDockerfile)
	}
	if unsigned[1] != signed[1] {
		t.Errorf("Windows base drift: %s pins %s but %s pins %s",
			unsignedWindowsDockerfile, unsigned[1], signedDockerfile, signed[1])
	}
}

func TestNPMDockerfileLinuxRuntime(t *testing.T) {
	var base string
	for _, df := range []string{unsignedLinuxDockerfile, signedDockerfile} {
		content := readDockerfile(t, df)
		image := linuxBaseRe.FindStringSubmatch(content)
		switch {
		case image == nil:
			t.Errorf("%s: missing digest-pinned Azure Linux 3 runtime", df)
		case base == "":
			base = image[1]
		case image[1] != base:
			t.Errorf("%s: Linux base drift: got %s, want %s", df, image[1], base)
		}
		for _, obsolete := range []string{"ubuntu", "apt-get"} {
			if strings.Contains(strings.ToLower(content), obsolete) {
				t.Errorf("%s: obsolete Ubuntu configuration %q", df, obsolete)
			}
		}
		if !strings.Contains(content, `ENTRYPOINT ["/usr/bin/azure-npm", "start"]`) {
			t.Errorf("%s: missing NPM entrypoint", df)
		}
		block := tdnfBlockRe.FindStringSubmatch(content)
		if block == nil {
			t.Errorf("%s: missing tdnf install and cache cleanup", df)
			continue
		}
		packages := strings.Fields(strings.ReplaceAll(block[1], `\`, ""))
		slices.Sort(packages)
		if !slices.Equal(packages, requiredLinuxPackages) {
			t.Errorf("%s: got runtime packages %v, want %v without version pins", df, packages, requiredLinuxPackages)
		}
	}
}

func TestNPMDockerfileLinuxCrypto(t *testing.T) {
	content := readDockerfile(t, unsignedLinuxDockerfile)
	for _, setting := range []string{"GOEXPERIMENT=ms_nocgo_opensslcrypto", "CGO_ENABLED=0"} {
		if !strings.Contains(content, setting) {
			t.Errorf("%s: missing %s", unsignedLinuxDockerfile, setting)
		}
	}
	if strings.Contains(content, "MS_GO_NOSYSTEMCRYPTO") {
		t.Errorf("%s: Linux container build must use system crypto", unsignedLinuxDockerfile)
	}
}
