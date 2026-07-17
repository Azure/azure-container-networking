package state

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"
)

func TestMigrationFaultInjectionTemplateContract(t *testing.T) {
	path := filepath.Join("..", "..", "..", ".pipelines", "cni", "load-test-templates", "migration-fault-injection-template.yaml")
	raw, err := os.ReadFile(path)
	require.NoError(t, err)

	var document struct {
		Parameters map[string]any   `json:"parameters"`
		Steps      []map[string]any `json:"steps"`
	}
	require.NoError(t, yaml.Unmarshal(raw, &document))

	for _, parameter := range []string{
		"clusterName",
		"os",
		"cni",
		"scenario",
		"scaleReplicas",
		"timeoutMinutes",
		"testTimeoutMinutes",
		"taskTimeoutMinutes",
		"runID",
		"artifactName",
		"workloadImage",
	} {
		require.Contains(t, document.Parameters, parameter)
	}
	require.Contains(t, document.Parameters["runID"], "$(System.JobId)")
	require.Contains(t, document.Parameters["artifactName"], "$(System.JobId)")

	var inlineScript string
	var publishAlways bool
	for _, step := range document.Steps {
		if inputs, ok := step["inputs"].(map[string]any); ok {
			if script, ok := inputs["inlineScript"].(string); ok {
				inlineScript = script
			}
		}
		if step["task"] == "PublishPipelineArtifact@1" && step["condition"] == "always()" {
			publishAlways = true
		}
	}
	require.NotEmpty(t, inlineScript)
	require.True(t, publishAlways)

	for _, value := range []string{
		"MIGRATION_FAULT_SCENARIO",
		"MIGRATION_FAULT_OS",
		"MIGRATION_FAULT_CNI",
		"MIGRATION_FAULT_RUN_ID",
		"MIGRATION_FAULT_ARTIFACT_DIR",
		"VALIDATE_STATE_BACKEND=bolt",
		"export KUBECONFIG=",
		"-test-kubeconfig=\"$KUBECONFIG\"",
		"./test/integration/state",
		"tee \"$artifactDir/go-test.log\"",
	} {
		require.Contains(t, inlineScript, value)
	}
	for _, forbidden := range []string{"killall", "pkill", "rollout restart"} {
		require.NotContains(t, strings.ToLower(inlineScript), forbidden)
	}
}
