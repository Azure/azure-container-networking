package state

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"
)

const repositoryRoot = "../../.."

type handoffTransition struct {
	Job                  string `json:"job"`
	DependsOn            string `json:"dependsOn"`
	BaselineTransition   string `json:"baselineTransition"`
	Name                 string `json:"transition"`
	Action               string `json:"action"`
	Backend              string `json:"backend"`
	CNI                  string `json:"cni"`
	ManageEndpointState  string `json:"manageEndpointState"`
	InitializeFromCNI    string `json:"initializeFromCNI"`
	EnableStateMigration string `json:"enableStateMigration"`
	StateRelation        string `json:"stateRelation"`
	BootRelation         string `json:"bootRelation"`
	PodRelation          string `json:"podRelation"`
	ExternalFault        bool   `json:"externalFault"`
}

func TestOwnershipHandoffTemplateContract(t *testing.T) {
	transitions := readHandoffTransitions(t)
	require.Len(t, transitions, 18)

	for index, transition := range transitions {
		number := 11 + index
		require.True(t, strings.HasPrefix(transition.Name, strconv.Itoa(number)+"-"))
		if index == 0 {
			require.Equal(t, "final_same_boot_restart", transition.DependsOn)
			require.Equal(t, "10-final-same-boot-restart", transition.BaselineTransition)
		} else {
			require.Equal(t, transitions[index-1].Job, transition.DependsOn)
			require.Equal(t, transitions[index-1].Name, transition.BaselineTransition)
		}

		if transition.Backend == "bolt" {
			require.Equal(t, "true", transition.ManageEndpointState,
				"supported Bolt runtime must always be CNS-owned")
			if transition.EnableStateMigration == "true" {
				require.Equal(t, "true", transition.InitializeFromCNI,
					"Bolt import requires the stateful CNI source")
			}
		}
		if transition.CNI == "stateless" {
			require.Equal(t, "true", transition.ManageEndpointState)
			require.Equal(t, "false", transition.InitializeFromCNI)
		}
	}

	require.Equal(t, "configure-state-import", transitions[1].Action)
	require.Equal(t, "json", transitions[1].Backend)
	require.Equal(t, "cniv2", transitions[1].CNI)
	require.Equal(t, "true", transitions[1].EnableStateMigration)
	require.Equal(t, "stateless", transitions[2].CNI)
	require.Equal(t, "bolt", transitions[3].Backend)

	for _, index := range []int{5, 11} {
		reverse := transitions[index]
		require.Equal(t, "rollback-json-and-clear-endpoints", reverse.Action)
		require.Equal(t, "json", reverse.Backend)
		require.Equal(t, "cniv2", reverse.CNI)
		require.Equal(t, "false", reverse.ManageEndpointState)
		require.Equal(t, "exact", reverse.PodRelation)
		require.Equal(t, "same", reverse.BootRelation)
	}

	require.Equal(t, "configure-state-import", transitions[7].Action)
	require.Equal(t, "bolt", transitions[7].Backend)
	require.Equal(t, "cniv2", transitions[7].CNI)
	require.Equal(t, "true", transitions[7].InitializeFromCNI)
	require.Equal(t, "true", transitions[7].EnableStateMigration)
	require.Equal(t, "configure-state", transitions[8].Action)
	require.Equal(t, "false", transitions[8].InitializeFromCNI)
	require.Equal(t, "stateless", transitions[9].CNI)

	require.Equal(t, "configure-state-import", transitions[13].Action)
	require.Equal(t, "true", transitions[13].InitializeFromCNI)
	require.Equal(t, "stateless", transitions[14].CNI)
	require.Equal(t, "external-fault", transitions[15].Action)
	require.True(t, transitions[15].ExternalFault)
	require.Equal(t, "same-boot-restart", transitions[16].Action)
	require.Equal(t, "node-reboot", transitions[17].Action)
	require.Equal(t, "identity", transitions[17].PodRelation)

	for _, transition := range transitions[15:] {
		require.Equal(t, "bolt", transition.Backend)
		require.Equal(t, "true", transition.ManageEndpointState,
			"handoff must remain CNS-owned after churn begins")
	}
}

func TestOwnershipHandoffPipelineContract(t *testing.T) {
	raw := readFile(t, pipelinePath("pipeline.yaml"))
	text := string(raw)
	for _, expected := range []string{
		"pr: none",
		"trigger: none",
		`default: "1.34"`,
		`ACN_VERSION: "r22-$(Build.BuildId)"`,
		`CNI_VERSION: "r22-$(Build.BuildId)"`,
		`CNS_VERSION: "r22-$(Build.BuildId)"`,
		`AZURE_IPAM_VERSION: "r22-$(Build.BuildId)"`,
		"$(Build.SourceVersion)",
		"../../containers/container-template.yaml",
		"../../containers/manifest-template.yaml",
	} {
		require.Contains(t, text, expected)
	}
	for _, forbidden := range []string{
		"enableFaultInjectionHooks",
		"CNI-managed Bolt",
	} {
		require.NotContains(t, text, forbidden)
	}

	var document struct {
		Stages []struct {
			Template   string         `json:"template"`
			Parameters map[string]any `json:"parameters"`
		} `json:"stages"`
	}
	require.NoError(t, yaml.Unmarshal(raw, &document))

	lanes := map[string]struct{}{}
	handoffLanes := []string{}
	for _, stage := range document.Stages {
		if stage.Template != "lane.stage.yaml" {
			continue
		}
		name, ok := stage.Parameters["name"].(string)
		require.True(t, ok)
		lanes[name] = struct{}{}
		require.Equal(t, "false", stage.Parameters["enableStateMigration"])
		if stage.Parameters["enableOwnershipHandoff"] == true {
			handoffLanes = append(handoffLanes, name)
		}
	}
	require.Len(t, lanes, 5)
	require.Equal(t, []string{"windows_podsubnet"}, handoffLanes)
}

func TestMigrationTemplateSafetyContract(t *testing.T) {
	lane := string(readFile(t, pipelinePath("lane.stage.yaml")))
	transition := string(readFile(t, pipelinePath("transition.steps.yaml")))
	capture := string(readFile(t, pipelinePath("capture-transition.steps.yaml")))
	install := string(readFile(t, pipelinePath("install-components.steps.yaml")))

	for _, expected := range []string{
		`KUBECONFIG: "$(Agent.TempDirectory)/kubeconfig-$(Build.BuildId)-$(System.JobId)"`,
		"condition: always()",
		"deleteResources: \"true\"",
		"handoff_reboot",
		"podRelation: identity",
	} {
		require.Contains(t, lane, expected)
	}
	for _, expected := range []string{
		".EnableBoltStateStore = $enableBolt",
		".EnablePersistentStateDebug = $enableDebug",
		".EnablePersistentStateFaults = false",
		"rollback-json-and-clear-endpoints",
		"external-pod-deletion",
		"kubectl delete pod",
		"bash patch-kubeclusterconfig.sh",
		`grep -Fxq "All nodes patched successfully"`,
		"Cannot stop service",
		"service cannot accept control messages",
		"Start-Service hns -ErrorAction Stop",
		"VALIDATE_SUMMARY_PATH",
		"VALIDATE_STATE_BACKEND",
		"persistent-state/snapshot",
	} {
		require.Contains(t, transition, expected)
	}
	require.Less(t,
		strings.Index(transition, "bash patch-kubeclusterconfig.sh"),
		strings.Index(transition, "az vmss restart"),
	)
	require.Less(t,
		strings.Index(transition, "displayName: Validate strict"),
		strings.Index(transition, "template: capture-transition.steps.yaml"),
	)
	for _, forbidden := range []string{
		"|| true",
		"migration-fault-injection-template",
		"faultinjection",
		"CNS_TEST_FAULT",
		"pkill",
		"killall",
		"retryCountOnTaskFailure",
	} {
		require.NotContains(t, transition, forbidden)
	}

	require.Contains(t, capture, "Validate strict persistent state metadata")
	require.Contains(t, capture, "persistent-state/status")
	require.Contains(t, capture, "persistent-state/snapshot")
	require.Contains(t, capture, "continueOnError: true")
	require.Contains(t, capture, "one or more best-effort transition evidence captures failed")
	require.NotContains(t, capture, "retryCountOnTaskFailure")

	for _, expected := range []string{
		".EnableBoltStateStore = false",
		".EnablePersistentStateDebug = false",
		".EnablePersistentStateFaults = false",
		`.StateStoreBackend = "json"`,
	} {
		require.Contains(t, install, expected)
	}
}

func TestMigrationTemplatesParseAndResolve(t *testing.T) {
	files, err := filepath.Glob(pipelinePath("*.yaml"))
	require.NoError(t, err)
	require.Len(t, files, 6)
	localFiles := make(map[string]string, len(files))
	for _, path := range files {
		localFiles[filepath.Base(path)] = path
	}
	for _, sourcePath := range files {
		t.Run(filepath.Base(sourcePath), func(t *testing.T) {
			raw := readFile(t, sourcePath)
			var source any
			require.NoError(t, yaml.Unmarshal(raw, &source))
			for _, reference := range collectTemplateReferences(source) {
				targetPath, local := localFiles[filepath.Base(reference.Template)]
				if !local {
					continue
				}
				requireTemplateParameters(t, targetPath, reference.Parameters)
			}
		})
	}
}

type templateReference struct {
	Template   string
	Parameters map[string]any
}

func collectTemplateReferences(value any) []templateReference {
	var references []templateReference
	switch typed := value.(type) {
	case map[string]any:
		template, hasTemplate := typed["template"].(string)
		if hasTemplate {
			parameters, _ := typed["parameters"].(map[string]any)
			references = append(references, templateReference{
				Template:   template,
				Parameters: parameters,
			})
		}
		for _, nested := range typed {
			references = append(references, collectTemplateReferences(nested)...)
		}
	case []any:
		for _, nested := range typed {
			references = append(references, collectTemplateReferences(nested)...)
		}
	}
	return references
}

func requireTemplateParameters(t *testing.T, targetPath string, passed map[string]any) {
	t.Helper()
	var target struct {
		Parameters []map[string]any `json:"parameters"`
	}
	require.NoError(t, yaml.Unmarshal(readFile(t, targetPath), &target))
	declared := make(map[string]bool, len(target.Parameters))
	for _, parameter := range target.Parameters {
		name, ok := parameter["name"].(string)
		require.True(t, ok, "parameter name in %s", targetPath)
		_, optional := parameter["default"]
		declared[name] = optional
	}
	for name := range passed {
		_, ok := declared[name]
		require.True(t, ok, "unexpected parameter %q for %s", name, targetPath)
	}
	for name, optional := range declared {
		if optional {
			continue
		}
		_, ok := passed[name]
		require.True(t, ok, "required parameter %q is missing for %s", name, targetPath)
	}
}

func TestCompareStateMigrationSummariesPodIdentity(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bash validation runs on Linux pipeline agents")
	}

	tests := []struct {
		name          string
		candidatePods string
		wantErr       bool
	}{
		{
			name: "IP churn preserves identity",
			candidatePods: `[{
				"namespace":"load-test","name":"pod-1","nodeName":"node-1",
				"phase":"Running","podIPs":["10.0.0.5"]
			}]`,
		},
		{
			name: "pod movement fails",
			candidatePods: `[{
				"namespace":"load-test","name":"pod-1","nodeName":"node-2",
				"phase":"Running","podIPs":["10.0.0.5"]
			}]`,
			wantErr: true,
		},
		{
			name: "phase change fails",
			candidatePods: `[{
				"namespace":"load-test","name":"pod-1","nodeName":"node-1",
				"phase":"Pending","podIPs":["10.0.0.5"]
			}]`,
			wantErr: true,
		},
		{
			name: "name change fails",
			candidatePods: `[{
				"namespace":"load-test","name":"pod-2","nodeName":"node-1",
				"phase":"Running","podIPs":["10.0.0.5"]
			}]`,
			wantErr: true,
		},
		{
			name:          "count reduction fails",
			candidatePods: `[]`,
			wantErr:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newCompareFixture(t)
			fixture.write("candidate-pods.json", tt.candidatePods)
			output, err := fixture.run("bolt", "none", "none", "identity")
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err, output)
		})
	}
}

func TestCompareStateMigrationSummariesRejectsInvalidState(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bash validation runs on Linux pipeline agents")
	}

	tests := []struct {
		name      string
		candidate string
		relation  string
	}{
		{name: "malformed", candidate: `{`, relation: "none"},
		{name: "empty checks", candidate: `{"stateBackend":"json","checks":[]}`, relation: "none"},
		{
			name: "malformed IP",
			candidate: `{"stateBackend":"json","checks":[{
				"checkName":"state","nodeName":"node-1","livePodCount":1,
				"expected":[{"podID":"pod-1","ip":"not-an-ip"}],
				"actual":[{"podID":"pod-1","ip":"not-an-ip"}]
			}]}`,
			relation: "none",
		},
		{
			name: "unknown field",
			candidate: `{"stateBackend":"json","unexpected":true,"checks":[{
				"checkName":"state","nodeName":"node-1","livePodCount":1,
				"expected":[{"podID":"pod-1","ip":"10.0.0.2"}],
				"actual":[{"podID":"pod-1","ip":"10.0.0.2"}]
			}]}`,
			relation: "none",
		},
		{
			name: "duplicate check",
			candidate: `{"stateBackend":"json","checks":[
				{"checkName":"state","nodeName":"node-1","livePodCount":1,"expected":[{"podID":"pod-1","ip":"10.0.0.2"}],"actual":[{"podID":"pod-1","ip":"10.0.0.2"}]},
				{"checkName":"state","nodeName":"node-1","livePodCount":1,"expected":[{"podID":"pod-1","ip":"10.0.0.2"}],"actual":[{"podID":"pod-1","ip":"10.0.0.2"}]}
			]}`,
			relation: "none",
		},
		{
			name: "duplicate identity",
			candidate: `{"stateBackend":"json","checks":[{
				"checkName":"state","nodeName":"node-1","livePodCount":1,
				"expected":[{"podID":"pod-1","ip":"10.0.0.2"},{"podID":"pod-1","ip":"10.0.0.2"}],
				"actual":[{"podID":"pod-1","ip":"10.0.0.2"},{"podID":"pod-1","ip":"10.0.0.2"}]
			}]}`,
			relation: "none",
		},
		{
			name: "reduced exact results",
			candidate: `{"stateBackend":"json","checks":[{
				"checkName":"state","nodeName":"node-1","livePodCount":1,
				"expected":[{"podID":"pod-1","ip":"10.0.0.2"}],
				"actual":[{"podID":"pod-1","ip":"10.0.0.2"}]
			}]}`,
			relation: "exact",
		},
		{
			name: "reduced changed results",
			candidate: `{"stateBackend":"json","checks":[{
				"checkName":"state","nodeName":"node-1","livePodCount":2,
				"expected":[{"podID":"pod-1","ip":"10.0.0.2"},{"podID":"pod-2","ip":"10.0.0.3"}],
				"actual":[{"podID":"pod-1","ip":"10.0.0.2"},{"podID":"pod-2","ip":"10.0.0.3"}]
			}]}`,
			relation: "changed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newCompareFixture(t)
			fixture.write("candidate-summary.json", tt.candidate)
			output, err := fixture.run("json", tt.relation, "none", "exact")
			require.Error(t, err, output)
		})
	}
}

func TestCompareStateMigrationSummariesBootRelations(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bash validation runs on Linux pipeline agents")
	}

	tests := []struct {
		name          string
		candidateBoot string
		relation      string
		wantErr       bool
	}{
		{name: "same boot", candidateBoot: "boot-1", relation: "same"},
		{name: "changed boot", candidateBoot: "boot-2", relation: "changed"},
		{name: "unchanged rejected", candidateBoot: "boot-1", relation: "changed", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newCompareFixture(t)
			fixture.write("candidate-node-boots.json", `[{"nodeName":"node-1","bootID":"`+tt.candidateBoot+`"}]`)
			output, err := fixture.run("bolt", "none", tt.relation, "exact")
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err, output)
		})
	}
}

func readHandoffTransitions(t *testing.T) []handoffTransition {
	t.Helper()
	raw := readFile(t, pipelinePath("handoff.jobs.yaml"))
	var document struct {
		Parameters []struct {
			Name    string              `json:"name"`
			Default []handoffTransition `json:"default"`
		} `json:"parameters"`
	}
	require.NoError(t, yaml.Unmarshal(raw, &document))
	for _, parameter := range document.Parameters {
		if parameter.Name == "transitions" {
			return parameter.Default
		}
	}
	t.Fatal("transitions parameter is missing")
	return nil
}

func pipelinePath(name string) string {
	return filepath.Join(repositoryRoot, ".pipelines", "cni", "state-migration", name)
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	return raw
}

type compareFixture struct {
	t            *testing.T
	baselineDir  string
	candidateDir string
}

func newCompareFixture(t *testing.T) compareFixture {
	t.Helper()
	dir, err := os.MkdirTemp(".", ".r22-fixture-")
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, os.RemoveAll(dir))
	})
	baselineDir := filepath.Join(dir, "baseline")
	candidateDir := filepath.Join(dir, "candidate")
	require.NoError(t, os.Mkdir(baselineDir, 0o700))
	require.NoError(t, os.Mkdir(candidateDir, 0o700))
	fixture := compareFixture{
		t:            t,
		baselineDir:  baselineDir,
		candidateDir: candidateDir,
	}
	fixture.write("baseline-summary.json", `{"stateBackend":"bolt","checks":[
		{"checkName":"state","nodeName":"node-1","livePodCount":1,"expected":[{"podID":"pod-1","ip":"10.0.0.2"}],"actual":[{"podID":"pod-1","ip":"10.0.0.2"}]},
		{"checkName":"cache","nodeName":"node-1","livePodCount":1,"expected":[{"podID":"pod-1","ip":"10.0.0.2"}],"actual":[{"podID":"pod-1","ip":"10.0.0.2"}]}
	]}`)
	fixture.write("candidate-summary.json", string(readFile(t, fixture.path("baseline-summary.json"))))
	fixture.write("baseline-pods.json", `[{
		"namespace":"load-test","name":"pod-1","nodeName":"node-1",
		"phase":"Running","podIPs":["10.0.0.4"]
	}]`)
	fixture.write("candidate-pods.json", string(readFile(t, fixture.path("baseline-pods.json"))))
	fixture.write("baseline-node-boots.json", `[{"nodeName":"node-1","bootID":"boot-1"}]`)
	fixture.write("candidate-node-boots.json", `[{"nodeName":"node-1","bootID":"boot-1"}]`)
	require.NoError(t, os.Mkdir(filepath.Join(baselineDir, "persistent-debug"), 0o700))
	require.NoError(t, os.Mkdir(filepath.Join(candidateDir, "persistent-debug"), 0o700))
	metadata := `{
		"nodeName":"node-1",
		"status":{"backend":"bbolt","authority":"bolt","schemaVersion":1,"generation":2,"bootPresent":true,"storagePresent":true,"databaseBytes":4096,"invariantStatus":"healthy"},
		"snapshot":{"Metadata":{"authority":"bolt","schemaVersion":1,"generation":2,"bootID":"boot-1"}}
	}`
	fixture.write(filepath.Join("baseline-metadata", "node-1.json"), metadata)
	fixture.write(filepath.Join("candidate-metadata", "node-1.json"), metadata)
	return fixture
}

func (f compareFixture) write(name, contents string) {
	f.t.Helper()
	require.NoError(f.t, os.WriteFile(f.path(name), []byte(contents), 0o600))
}

func (f compareFixture) path(name string) string {
	switch {
	case strings.HasPrefix(name, "baseline-metadata/"):
		return filepath.Join(f.baselineDir, "persistent-debug", strings.TrimPrefix(name, "baseline-metadata/"))
	case strings.HasPrefix(name, "candidate-metadata/"):
		return filepath.Join(f.candidateDir, "persistent-debug", strings.TrimPrefix(name, "candidate-metadata/"))
	case strings.HasPrefix(name, "baseline-"):
		return filepath.Join(f.baselineDir, strings.TrimPrefix(name, "baseline-"))
	case strings.HasPrefix(name, "candidate-"):
		return filepath.Join(f.candidateDir, strings.TrimPrefix(name, "candidate-"))
	default:
		f.t.Fatalf("fixture path %q has no baseline or candidate prefix", name)
		return ""
	}
}

func (f compareFixture) run(backend, stateRelation, bootRelation, podRelation string) (string, error) {
	f.t.Helper()
	script := filepath.Join(repositoryRoot, "hack", "scripts", "compare-state-migration-summaries.sh")
	command := exec.Command(
		"bash",
		script,
		filepath.Join(f.baselineDir, "summary.json"),
		filepath.Join(f.candidateDir, "summary.json"),
		backend,
		"bolt",
		"1",
		stateRelation,
		bootRelation,
		podRelation,
		filepath.Join(f.baselineDir, "pods.json"),
		filepath.Join(f.candidateDir, "pods.json"),
		filepath.Join(f.baselineDir, "persistent-debug"),
		filepath.Join(f.candidateDir, "persistent-debug"),
	)
	output, err := command.CombinedOutput()
	return string(output), err
}
