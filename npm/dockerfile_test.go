package npm

import (
	"os"
	"regexp"
	"sort"
	"testing"
)

// These tests guard the NPM image CVE-hardening pins so the signed (release) and
// unsigned Dockerfiles cannot silently drift apart. Drift between the two paths
// is how CVE fixes get lost: a bump in one file that is not mirrored in the
// other ships an unpatched image.

const (
	unsignedLinuxDockerfile   = "linux.Dockerfile"
	unsignedWindowsDockerfile = "windows.Dockerfile"
	signedDockerfile          = "../.pipelines/build/dockerfiles/npm.Dockerfile"

	// expectedWindowsBaseDigest is the patched Windows Server Core LTSC2022 base
	// (build 10.0.20348.5622). Asserting the exact digest — not just that the two
	// Dockerfiles agree — makes a synchronized rollback of both files to an older,
	// vulnerable Server Core digest fail the test. Bump this when the base is
	// intentionally refreshed to a newer patched digest.
	expectedWindowsBaseDigest = "sha256:76cf422c98ca437b308374d0498280541fa42ac7061bb44015a6c8b70cf4db6a"
)

var (
	goBuilderRe  = regexp.MustCompile(`mcr\.microsoft\.com/oss/go/microsoft/golang:(\S+?)\s+AS builder`)
	servercoreRe = regexp.MustCompile(`mcr\.microsoft\.com/windows/servercore:ltsc2022@(sha256:[0-9a-f]{64})`)
	aptBlockRe   = regexp.MustCompile(`(?s)apt-get install -y(.*?)apt-get autoremove`)
	aptPinRe     = regexp.MustCompile(`([A-Za-z0-9][A-Za-z0-9.+-]*)=([0-9][A-Za-z0-9.:+~-]*)`)
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
	if linux[1] != windows[1] {
		t.Errorf("Go builder drift: %s uses %s but %s uses %s",
			unsignedLinuxDockerfile, linux[1], unsignedWindowsDockerfile, windows[1])
	}
}

// TestNPMDockerfileWindowsBasePinned verifies the signed and unsigned Windows
// images both pin the expected patched Server Core base digest. Asserting the
// exact digest (not only that the two files match) detects a synchronized
// rollback of both files to an older, vulnerable base.
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
	for _, f := range []struct{ name, digest string }{
		{unsignedWindowsDockerfile, unsigned[1]},
		{signedDockerfile, signed[1]},
	} {
		if f.digest != expectedWindowsBaseDigest {
			t.Errorf("%s pins servercore %s, want patched digest %s", f.name, f.digest, expectedWindowsBaseDigest)
		}
	}
}

// TestNPMDockerfileLinuxPackagePinsInSync verifies the pinned Ubuntu CVE package
// versions are identical in the signed and unsigned Linux images.
func TestNPMDockerfileLinuxPackagePinsInSync(t *testing.T) {
	unsigned := aptPins(readDockerfile(t, unsignedLinuxDockerfile))
	signed := aptPins(readDockerfile(t, signedDockerfile))

	if len(unsigned) == 0 {
		t.Fatalf("%s: expected pinned apt packages, found none", unsignedLinuxDockerfile)
	}

	for pkg, ver := range unsigned {
		sv, ok := signed[pkg]
		switch {
		case !ok:
			t.Errorf("package %s pinned in %s (%s) but missing from %s", pkg, unsignedLinuxDockerfile, ver, signedDockerfile)
		case sv != ver:
			t.Errorf("package %s version drift: %s pins %s but %s pins %s", pkg, unsignedLinuxDockerfile, ver, signedDockerfile, sv)
		}
	}
	for pkg, ver := range signed {
		if _, ok := unsigned[pkg]; !ok {
			t.Errorf("package %s pinned in %s (%s) but missing from %s", pkg, signedDockerfile, ver, unsignedLinuxDockerfile)
		}
	}

	pkgs := make([]string, 0, len(unsigned))
	for pkg := range unsigned {
		pkgs = append(pkgs, pkg)
	}
	sort.Strings(pkgs)
	t.Logf("verified %d pinned Ubuntu packages in sync: %v", len(pkgs), pkgs)
}

func aptPins(dockerfile string) map[string]string {
	pins := map[string]string{}
	block := aptBlockRe.FindStringSubmatch(dockerfile)
	if block == nil {
		return pins
	}
	for _, m := range aptPinRe.FindAllStringSubmatch(block[1], -1) {
		pins[m[1]] = m[2]
	}
	return pins
}
