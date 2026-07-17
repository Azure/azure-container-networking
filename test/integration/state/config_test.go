package state

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestLoadFaultConfig(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		wantErr string
	}{
		{
			name: "valid defaults",
			env: map[string]string{
				envFaultRunID:       "build-123",
				envFaultArtifactDir: "artifacts",
				envValidateBackend:  "bolt",
			},
		},
		{
			name: "specific scenario",
			env: map[string]string{
				envFaultScenario:       string(scenarioEndpointPatch),
				envFaultOS:             "windows",
				envFaultCNI:            "stateless",
				envFaultRunID:          "build-123",
				envFaultArtifactDir:    "artifacts",
				envFaultScaleReplicas:  "32",
				envFaultTimeoutMinutes: "60",
				envValidateBackend:     "bolt",
			},
		},
		{
			name: "unsupported scenario",
			env: map[string]string{
				envFaultScenario:    "unknown",
				envFaultRunID:       "build-123",
				envFaultArtifactDir: "artifacts",
				envValidateBackend:  "bolt",
			},
			wantErr: "unsupported migration fault scenario",
		},
		{
			name: "unsupported OS and CNI combination",
			env: map[string]string{
				envFaultOS:          "windows",
				envFaultCNI:         "cilium",
				envFaultRunID:       "build-123",
				envFaultArtifactDir: "artifacts",
				envValidateBackend:  "bolt",
			},
			wantErr: "unsupported migration fault CNI",
		},
		{
			name: "non bolt backend",
			env: map[string]string{
				envFaultRunID:       "build-123",
				envFaultArtifactDir: "artifacts",
				envValidateBackend:  "json",
			},
			wantErr: "VALIDATE_STATE_BACKEND must be bolt",
		},
		{
			name: "unsafe scale",
			env: map[string]string{
				envFaultRunID:         "build-123",
				envFaultArtifactDir:   "artifacts",
				envFaultScaleReplicas: "1",
				envValidateBackend:    "bolt",
			},
			wantErr: "MIGRATION_FAULT_SCALE_REPLICAS",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := loadFaultConfig(func(key string) string {
				return tt.env[key]
			})
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.NotEmpty(t, cfg.RunID)
			require.NotEmpty(t, cfg.ArtifactDir)
			if tt.name == "specific scenario" {
				require.Equal(t, int32(32), cfg.ScaleReplicas)
				require.Equal(t, 60*time.Minute, cfg.Timeout)
				require.Equal(t, "windows", cfg.OS)
			}
		})
	}
}

func TestScenarioContract(t *testing.T) {
	cfg := faultConfig{Scenario: scenarioAll}
	scenarios, err := cfg.scenarios()
	require.NoError(t, err)
	require.Equal(t, []scenario{
		scenarioAddBeforeEndpointCommit,
		scenarioDeleteAfterIntentCommit,
		scenarioEndpointPatch,
		scenarioRestartDuringScale,
	}, scenarios)
	require.Equal(t, faultPointAddBeforeEndpoint, faultPointForScenario(scenarioRestartDuringScale))
	require.Equal(t, faultPointDeleteAfterIntent, faultPointForScenario(scenarioDeleteAfterIntentCommit))
	require.Equal(t, faultPointPatchBeforeEndpoint, faultPointForScenario(scenarioEndpointPatch))
}

func TestSanitizeResourceName(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		maxLength int
		want      string
	}{
		{name: "normalizes", value: "Build_123/PATCH", maxLength: 63, want: "build-123-patch"},
		{name: "removes non ASCII", value: "Büild", maxLength: 63, want: "b-ild"},
		{name: "empty", value: "---", maxLength: 63, want: "run"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, sanitizeResourceName(tt.value, tt.maxLength))
		})
	}

	first := sanitizeResourceName("a very long build identifier with repeated content a very long build identifier", 32)
	second := sanitizeResourceName("a very long build identifier with repeated content a very long build identifier", 32)
	require.Equal(t, first, second)
	require.Len(t, first, 32)
}

func TestContainerFaultEnvRoundTrip(t *testing.T) {
	container := corev1.Container{
		Env: []corev1.EnvVar{{Name: "EXISTING", Value: "value"}},
	}
	backup := setContainerEnv(&container, faultTokenEnv, "token")
	require.False(t, backup.Present)
	require.Contains(t, container.Env, corev1.EnvVar{Name: faultTokenEnv, Value: "token"})
	require.True(t, restoreContainerEnv(&container, faultTokenEnv, "token", backup))
	require.Equal(t, []corev1.EnvVar{{Name: "EXISTING", Value: "value"}}, container.Env)

	container.Env = append(container.Env, corev1.EnvVar{Name: faultTokenEnv, Value: "original"})
	backup = setContainerEnv(&container, faultTokenEnv, "replacement")
	require.True(t, backup.Present)
	require.True(t, restoreContainerEnv(&container, faultTokenEnv, "replacement", backup))
	require.Contains(t, container.Env, corev1.EnvVar{Name: faultTokenEnv, Value: "original"})
}

func TestValidateMigrationCNSConfig(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantErr string
	}{
		{
			name: "valid",
			raw:  `{"StateStoreBackend":"bolt","ManageEndpointState":true}`,
		},
		{
			name:    "wrong backend",
			raw:     `{"StateStoreBackend":"json","ManageEndpointState":true}`,
			wantErr: "state store backend must be bolt",
		},
		{
			name:    "endpoint state disabled",
			raw:     `{"StateStoreBackend":"bolt","ManageEndpointState":false}`,
			wantErr: "managed endpoint state must be enabled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateMigrationCNSConfig([]byte(tt.raw))
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestSelectCNSTarget(t *testing.T) {
	now := metav1.Now()
	pods := []corev1.Pod{
		readyPod("cns-b", "node-b"),
		readyPod("cns-a", "node-a"),
		{
			ObjectMeta: metav1.ObjectMeta{Name: "deleting", DeletionTimestamp: &now},
			Spec:       corev1.PodSpec{NodeName: "node-a"},
			Status:     readyPod("ignored", "node-a").Status,
		},
	}

	target, err := selectCNSTarget(pods, "")
	require.NoError(t, err)
	require.Equal(t, "cns-a", target.Name)

	target, err = selectCNSTarget(pods, "node-b")
	require.NoError(t, err)
	require.Equal(t, "cns-b", target.Name)

	_, err = selectCNSTarget(pods, "node-c")
	require.Error(t, err)
}

func TestExactPodDeleteOptions(t *testing.T) {
	uid := types.UID("pod-uid")
	options := exactPodDeleteOptions(uid)
	require.NotNil(t, options.Preconditions)
	require.Equal(t, uid, *options.Preconditions.UID)
	require.Zero(t, *options.GracePeriodSeconds)

	workloadOptions := workloadPodDeleteOptions(uid)
	require.Equal(t, uid, *workloadOptions.Preconditions.UID)
	require.Nil(t, workloadOptions.GracePeriodSeconds)
}

func readyPod(name, node string) corev1.Pod {
	return corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec:       corev1.PodSpec{NodeName: node},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{{
				Type:   corev1.PodReady,
				Status: corev1.ConditionTrue,
			}},
		},
	}
}
