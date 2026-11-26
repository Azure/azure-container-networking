package npm

import (
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"
)

func TestNPMPipelineRefreshesImage(t *testing.T) {
	var pipeline struct {
		Steps []struct {
			DisplayName string `json:"displayName"`
			Script      string `json:"script"`
		} `json:"steps"`
	}
	content := readDockerfile(t, "../.pipelines/containers/container-template.yaml")
	if err := yaml.Unmarshal([]byte(content), &pipeline); err != nil {
		t.Fatal(err)
	}
	var script string
	for _, step := range pipeline.Steps {
		if step.DisplayName == "Validate NPM Linux image runtime" {
			script = step.Script
		}
	}
	if script == "" {
		t.Fatal("NPM image validation step is missing")
	}
	script = strings.NewReplacer(
		"${{ parameters.os }}", "linux",
		"${{ parameters.arch }}", "amd64",
	).Replace(script)

	const (
		image          = "registry.invalid/azure-npm:linux-amd64-test"
		pullCall       = "docker pull " + image + "\n"
		validationCall = "bash hack/scripts/test_npm_image.sh " + image + "\n"
	)
	for _, tt := range []struct {
		name           string
		pullExit       int
		validationExit int
		wantExit       int
		wantCalls      string
	}{
		{name: "pull before validation", wantCalls: pullCall + validationCall},
		{name: "pull failure stops validation", pullExit: 7, wantExit: 7, wantCalls: pullCall},
		{name: "validation failure propagates", validationExit: 9, wantExit: 9, wantCalls: pullCall + validationCall},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// Execute the actual CI step without accessing Docker or a registry.
			cmd := exec.CommandContext(t.Context(), "bash", "-s")
			cmd.Stdin = strings.NewReader(`
make() { printf '%s\n' "$TEST_IMAGE"; }
docker() { printf 'docker %s\n' "$*"; return "$PULL_EXIT"; }
bash() { printf 'bash %s\n' "$*"; return "$VALIDATION_EXIT"; }
` + script)
			cmd.Env = append(os.Environ(),
				"TEST_IMAGE="+image,
				"PULL_EXIT="+strconv.Itoa(tt.pullExit),
				"VALIDATION_EXIT="+strconv.Itoa(tt.validationExit),
			)
			output, err := cmd.CombinedOutput()
			exitCode := 0
			if err != nil {
				var exitErr *exec.ExitError
				if !errors.As(err, &exitErr) {
					t.Fatal(err)
				}
				exitCode = exitErr.ExitCode()
			}
			if exitCode != tt.wantExit {
				t.Errorf("exit code %d, want %d; output: %s", exitCode, tt.wantExit, output)
			}
			if string(output) != tt.wantCalls {
				t.Errorf("commands %q, want %q", output, tt.wantCalls)
			}
		})
	}
}
