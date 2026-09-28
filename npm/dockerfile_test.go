package npm

import (
	"os"
	"regexp"
	"strconv"
	"strings"
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

	// minGoBuilderVersion is the MS Go toolchain floor required for the Go/stdlib
	// CVE remediation. Asserting it — not just that both Dockerfiles match — makes
	// a synchronized rollback of both files to an older, vulnerable Go tag fail the
	// test. Raise this when a newer toolchain is required to clear a CVE.
	minGoBuilderVersion = "1.26.7"
)

// requiredLinuxPins is the authoritative set of Ubuntu packages that must be
// pinned — at exactly these patched versions — in BOTH the signed and unsigned
// Linux images. Asserting the full set with exact versions (rather than only
// that the two Dockerfiles agree) makes the test fail on a *synchronized*
// regression: dropping the same pin from both files, or rolling the same
// version back in both, no longer slips through. Update this map together with
// npm/linux.Dockerfile and .pipelines/build/dockerfiles/npm.Dockerfile whenever
// pins are refreshed for a new Ubuntu security release.
var requiredLinuxPins = map[string]string{
	"gpgv":           "2.4.4-2ubuntu17.6",
	"libc-bin":       "2.39-0ubuntu8.9",
	"libc6":          "2.39-0ubuntu8.9",
	"libtasn1-6":     "4.19.0-3ubuntu0.24.04.2",
	"dpkg":           "1.22.6ubuntu6.6",
	"libcap2":        "1:2.66-5ubuntu2.4",
	"libgcrypt20":    "1.10.3-2ubuntu0.2",
	"libgnutls30t64": "3.8.3-1.1ubuntu3.6",
	"libsystemd0":    "255.4-1ubuntu8.17",
	"libudev1":       "255.4-1ubuntu8.17",
	"liblzma5":       "5.6.1+really5.4.5-1ubuntu0.3",
	"sed":            "4.9-2ubuntu0.24.04.1",
	"gzip":           "1.12-1ubuntu3.2",
	"libncursesw6":   "6.4+20240113-1ubuntu2.2",
	"libtinfo6":      "6.4+20240113-1ubuntu2.2",
	"libpam-modules": "1.5.3-5ubuntu5.7",
	"perl-base":      "5.38.2-3.2ubuntu0.6",
	"tar":            "1.35+dfsg-3ubuntu0.4",
	"util-linux":     "2.39.3-9ubuntu6.6",
	"mount":          "2.39.3-9ubuntu6.6",
	"bsdutils":       "1:2.39.3-9ubuntu6.6",
	"libblkid1":      "2.39.3-9ubuntu6.6",
	"libmount1":      "2.39.3-9ubuntu6.6",
	"libsmartcols1":  "2.39.3-9ubuntu6.6",
	"libuuid1":       "2.39.3-9ubuntu6.6",
	"coreutils":      "9.4-3ubuntu6.3",
	"diffutils":      "1:3.10-1ubuntu0.1",
	"libattr1":       "1:2.5.2-1ubuntu0.1",
	"libbz2-1.0":     "1.0.8-5.1ubuntu0.1",
	"libp11-kit0":    "0.25.3-4ubuntu2.2",
	"zlib1g":         "1:1.3.dfsg-3.1ubuntu2.2",
}

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

// TestNPMDockerfileLinuxPackagePinsInSync verifies that both the signed and
// unsigned Linux images pin the complete set of required Ubuntu CVE packages at
// exactly their patched versions. Because it checks against requiredLinuxPins
// (not just that the two files agree), it fails on a synchronized regression —
// the same pin removed from, or rolled back in, both Dockerfiles.
func TestNPMDockerfileLinuxPackagePinsInSync(t *testing.T) {
	for _, df := range []string{unsignedLinuxDockerfile, signedDockerfile} {
		got := aptPins(readDockerfile(t, df))
		if len(got) == 0 {
			t.Errorf("%s: expected pinned apt packages, found none", df)
			continue
		}
		// Every required package must be pinned at exactly the patched version.
		for pkg, want := range requiredLinuxPins {
			switch v, ok := got[pkg]; {
			case !ok:
				t.Errorf("%s: required CVE pin for %q is missing (want %s)", df, pkg, want)
			case v != want:
				t.Errorf("%s: %q pinned at %s, want patched version %s", df, pkg, v, want)
			}
		}
		// Reject unexpected pins so requiredLinuxPins is kept in lockstep with the
		// Dockerfiles when a new package is pinned.
		for pkg, v := range got {
			if _, ok := requiredLinuxPins[pkg]; !ok {
				t.Errorf("%s: pin %q=%s not in requiredLinuxPins; add it to the test when adding a pin", df, pkg, v)
			}
		}
	}
	t.Logf("verified %d required Ubuntu CVE pins present at patched versions in both images", len(requiredLinuxPins))
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
