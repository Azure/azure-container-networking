package npm

import (
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
)

const (
	unsignedLinuxDockerfile = "linux.Dockerfile"
	signedDockerfile        = "../.pipelines/build/dockerfiles/npm.Dockerfile"
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
	linuxBuilderRe = regexp.MustCompile(`(?m)^FROM mcr\.microsoft\.com/oss/go/microsoft/golang:\d+\.\d+\.\d+(?:-\d+)?-azurelinux3\.0 AS builder\s*$`)
	linuxBaseRe    = regexp.MustCompile(`(?im)^FROM (?:--platform=\S+ )?(mcr\.microsoft\.com/azurelinux/base/core:3\.0@sha256:[0-9a-f]{64}) AS linux\s*$`)
	tdnfBlockRe    = regexp.MustCompile(`(?s)tdnf install -y(.*?)&& tdnf clean all`)
)

func readDockerfile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return string(b)
}

func TestNPMDockerfileLinuxBuilder(t *testing.T) {
	if !linuxBuilderRe.MatchString(readDockerfile(t, unsignedLinuxDockerfile)) {
		t.Errorf("%s: missing patch-pinned Azure Linux 3 MS Go builder", unsignedLinuxDockerfile)
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
	for _, path := range []string{unsignedLinuxDockerfile, "../.pipelines/build/scripts/npm.sh"} {
		content := readDockerfile(t, path)
		for _, setting := range []string{"GOEXPERIMENT=ms_nocgo_opensslcrypto", "CGO_ENABLED=0"} {
			if !strings.Contains(content, setting) {
				t.Errorf("%s: missing %s", path, setting)
			}
		}
	}
	if strings.Contains(readDockerfile(t, unsignedLinuxDockerfile), "MS_GO_NOSYSTEMCRYPTO") {
		t.Errorf("%s: Linux container build must use system crypto", unsignedLinuxDockerfile)
	}
}
