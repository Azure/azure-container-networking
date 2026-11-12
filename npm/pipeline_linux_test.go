package npm

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"
)

func TestNPMPipelineValidatesBuiltImage(t *testing.T) {
	var pipeline struct {
		Steps []struct {
			Name        string `json:"name"`
			DisplayName string `json:"displayName"`
			Script      string `json:"script"`
		} `json:"steps"`
	}
	content := readDockerfile(t, "../.pipelines/containers/container-template.yaml")
	if err := yaml.Unmarshal([]byte(content), &pipeline); err != nil {
		t.Fatal(err)
	}
	var build, validate string
	for _, step := range pipeline.Steps {
		if step.Name == "image_build" {
			build = step.Script
		}
		if step.DisplayName == "Validate NPM Linux image runtime" {
			validate = step.Script
		}
	}
	if build == "" || validate == "" {
		t.Fatal("NPM image build or validation step is missing")
	}
	replacer := strings.NewReplacer(
		"${{ parameters.name }}", "npm",
		"${{ parameters.os }}", "linux",
		"${{ parameters.arch }}", "amd64",
	)

	const (
		image          = "registry.invalid/azure-npm:linux-amd64-refresh-test"
		buildahBuild   = "buildah build " + image + "\n"
		buildahPush    = "buildah push " + image + "\n"
		dockerLoad     = "docker load-build " + image + "\n"
		dockerPush     = "docker push-build " + image + "\n"
		pullCall       = "docker pull " + image + "\n"
		validationCall = "bash hack/scripts/test_npm_image.sh " + image + "\n"
	)
	for _, tt := range []struct {
		name             string
		buildahAvailable bool
		builder          string
		action           string
		pullExit         int
		validationExit   int
		wantFailure      bool
		wantCalls        string
	}{
		{name: "Buildah publishes", buildahAvailable: true, wantCalls: buildahBuild + buildahPush + pullCall + validationCall},
		{name: "Docker fallback loads locally", wantCalls: dockerLoad + validationCall},
		{name: "explicit Docker loads locally", buildahAvailable: true, builder: "docker", wantCalls: dockerLoad + validationCall},
		{name: "Docker push override", action: "--push --provenance=false", wantCalls: dockerPush + pullCall + validationCall},
		{
			name: "pull failure stops validation", buildahAvailable: true, pullExit: 7, wantFailure: true,
			wantCalls: buildahBuild + buildahPush + pullCall,
		},
		{name: "local helper failure", validationExit: 9, wantFailure: true, wantCalls: dockerLoad + validationCall},
		{
			name: "published helper failure", buildahAvailable: true, validationExit: 9, wantFailure: true,
			wantCalls: buildahBuild + buildahPush + pullCall + validationCall,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			trace := filepath.Join(t.TempDir(), "commands")
			if err := os.WriteFile(trace, nil, 0o600); err != nil {
				t.Fatal(err)
			}
			// Run both CI steps and the real Makefile, replacing only external image operations.
			cmd := exec.CommandContext(t.Context(), "bash", "-s")
			cmd.Dir = ".."
			cmd.Stdin = strings.NewReader(`
set -euo pipefail
unset CONTAINER_BUILDER BUILDX_ACTION MAKEFLAGS MFLAGS GNUMAKEFLAGS
if [[ -n "$TEST_BUILDER" ]]; then export CONTAINER_BUILDER="$TEST_BUILDER"; fi
if [[ -n "$TEST_ACTION" ]]; then export BUILDX_ACTION="$TEST_ACTION"; fi
trace() { printf '%s\n' "$*" >>"$TEST_TRACE"; }
which() {
    if [[ "$1" == buildah ]]; then
        [[ "$TEST_BUILDAH" == true ]] || return 1
        printf '/test/buildah\n'
    else
        command -v "$1"
    fi
}
record_build() {
    local event="$1"
    shift
    while (( $# )); do
        if [[ "$1" == -t ]]; then trace "$event $2"; return; fi
        shift
    done
    return 64
}
buildah() {
    case "$1" in
        bud) record_build "buildah build" "$@" ;;
        push) trace "buildah push $2" ;;
        *) return 64 ;;
    esac
}
docker() {
    case "$1 $2" in
        "buildx create") ;;
        "buildx build")
            if [[ " $* " == *" --push "* ]]; then
                record_build "docker push-build" "$@"
            elif [[ " $* " == *" --load "* ]]; then
                record_build "docker load-build" "$@"
            else
                return 64
            fi ;;
        "pull "*) trace "docker pull $2"; return "$PULL_EXIT" ;;
        *) return 64 ;;
    esac
}
bash() { trace "bash $*"; return "$VALIDATION_EXIT"; }
df() { :; }
export -f trace which record_build buildah docker bash df
(` + replacer.Replace(build) + "\n)\n" + replacer.Replace(validate))
			cmd.Env = append(os.Environ(),
				"TEST_TRACE="+trace, "TEST_BUILDAH="+strconv.FormatBool(tt.buildahAvailable),
				"TEST_BUILDER="+tt.builder, "TEST_ACTION="+tt.action,
				"PULL_EXIT="+strconv.Itoa(tt.pullExit), "VALIDATION_EXIT="+strconv.Itoa(tt.validationExit),
				"IMAGE_REGISTRY=registry.invalid", "NPM_VERSION=refresh-test", "GOOS=linux", "GOARCH=amd64",
			)
			output, err := cmd.CombinedOutput()
			if err != nil {
				var exitErr *exec.ExitError
				if !errors.As(err, &exitErr) {
					t.Fatal(err)
				}
			}
			if (err != nil) != tt.wantFailure {
				t.Errorf("pipeline error %v, want failure %t; output: %s", err, tt.wantFailure, output)
			}
			calls, err := os.ReadFile(trace)
			if err != nil {
				t.Fatal(err)
			}
			if string(calls) != tt.wantCalls {
				t.Errorf("commands %q, want %q; output: %s", calls, tt.wantCalls, output)
			}
		})
	}
}
