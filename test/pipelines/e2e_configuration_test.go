package pipelines

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

func readPipeline(t *testing.T, path string) *yaml.Node {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", ".pipelines", filepath.FromSlash(path)))
	require.NoError(t, err)
	var pipeline yaml.Node
	require.NoError(t, yaml.Unmarshal(data, &pipeline), path)
	require.Len(t, pipeline.Content, 1)
	return pipeline.Content[0]
}

func field(t *testing.T, node *yaml.Node, path ...string) *yaml.Node {
	t.Helper()
	for _, name := range path {
		require.Equal(t, yaml.MappingNode, node.Kind)
		var child *yaml.Node
		for i := 0; i < len(node.Content); i += 2 {
			if node.Content[i].Value == name {
				child = node.Content[i+1]
				break
			}
		}
		require.NotNil(t, child, "missing YAML field %q", name)
		node = child
	}
	return node
}

func findNodes(root *yaml.Node, key, value string) []*yaml.Node {
	nodes := []*yaml.Node{root}
	var matches []*yaml.Node
	for len(nodes) > 0 {
		node := nodes[0]
		nodes = nodes[1:]
		if node.Kind == yaml.MappingNode {
			for i := 0; i < len(node.Content); i += 2 {
				if node.Content[i].Value == key && node.Content[i+1].Value == value {
					matches = append(matches, node)
				}
			}
		}
		nodes = append(nodes, node.Content...)
	}
	return matches
}

func findNode(t *testing.T, root *yaml.Node, key, value string) *yaml.Node {
	t.Helper()
	matches := findNodes(root, key, value)
	require.Len(t, matches, 1, "pipeline node %s=%q", key, value)
	return matches[0]
}

func parameterMap(t *testing.T, node *yaml.Node) map[string]any {
	t.Helper()
	var parameters map[string]any
	require.NoError(t, node.Decode(&parameters))
	return parameters
}

func resolveParameters(t *testing.T, node *yaml.Node, parent map[string]any) map[string]any {
	t.Helper()
	parameters := parameterMap(t, field(t, node, "parameters"))
	for name, value := range parameters {
		if text, ok := value.(string); ok {
			parameters[name] = expandParameters(t, text, parent)
		}
	}
	return parameters
}

func bindParameters(t *testing.T, defaults, supplied map[string]any) map[string]any {
	t.Helper()
	parameters := maps.Clone(defaults)
	for name, value := range supplied {
		require.Contains(t, defaults, name, "undeclared template parameter")
		parameters[name] = value
	}
	return parameters
}

func typedDefaults(t *testing.T, node *yaml.Node) map[string]any {
	t.Helper()
	var declarations []struct {
		Name    string `yaml:"name"`
		Default any    `yaml:"default"`
	}
	require.NoError(t, node.Decode(&declarations))
	defaults := make(map[string]any, len(declarations))
	for _, declaration := range declarations {
		defaults[declaration.Name] = declaration.Default
	}
	return defaults
}

// These templates use literal parameter substitutions, not a full ADO expression evaluator.
func expandParameters(t *testing.T, text string, parameters map[string]any) string {
	t.Helper()
	for name, value := range parameters {
		text = strings.ReplaceAll(text, "${{ parameters."+name+" }}", fmt.Sprint(value))
	}
	require.NotContains(t, text, "${{ parameters.", "unresolved template parameter")
	return text
}

func TestSwiftInstallationParameters(t *testing.T) {
	run := readPipeline(t, "run-pipeline.yaml")
	stages := readPipeline(t, "singletenancy/aks-swift/e2e.stages.yaml")
	steps := readPipeline(t, "singletenancy/aks-swift/e2e.steps.yaml")
	network := readPipeline(t, "templates/network-install.yaml")
	defaults := parameterMap(t, field(t, stages, "parameters"))
	require.Equal(t, "cniv2", defaults["cniType"])
	require.Equal(t, "INSTALL_AZURE_VNET", defaults["installFlag"])

	install := findNode(t, stages, "template", "../../templates/network-install.yaml")
	tests := findNode(t, stages, "template", "e2e.steps.yaml")
	networkScript := field(t, findNode(t, network, "displayName", "Install ACN Networking"), "inputs", "inlineScript").Value
	scenarios := map[string]struct{ cniType, installFlag string }{
		"aks_swift_e2e":           {"stateless", "INSTALL_AZURE_VNET_STATELESS_SWIFT"},
		"aks_swift_vnetscale_e2e": {"cniv2", "INSTALL_AZURE_VNET"},
	}
	callers := findNodes(run, "template", "singletenancy/aks-swift/e2e.stages.yaml")
	require.Len(t, callers, len(scenarios))
	for _, caller := range callers {
		parameters := bindParameters(t, defaults, parameterMap(t, field(t, caller, "parameters")))
		name, ok := parameters["name"].(string)
		require.True(t, ok)
		expected, ok := scenarios[name]
		require.True(t, ok, "unexpected Swift scenario %q", name)
		t.Run(name, func(t *testing.T) {
			provisioning := resolveParameters(t, install, parameters)
			require.Equal(t, expected.installFlag, provisioning["installFlag"])
			require.Equal(t, "linux", provisioning["os"])
			networkParameters := bindParameters(t, typedDefaults(t, field(t, network, "parameters")), provisioning)
			script := expandParameters(t, networkScript, networkParameters)
			require.Contains(t, script, expected.installFlag+"=true")
			require.Contains(t, script, "INSTALL_CNS=true")
			require.Contains(t, script, "CNS_ONLY=true")

			forwarded := resolveParameters(t, tests, parameters)
			testParameters := bindParameters(t, parameterMap(t, field(t, steps, "parameters")), forwarded)
			load := expandParameters(t, field(t, findNode(t, steps, "name", "aksswifte2e"), "script").Value, testParameters)
			require.Contains(t, load, "CNI_TYPE="+expected.cniType)
			require.Contains(t, load, expected.installFlag+"=true")
			require.Contains(t, load, "VALIDATE_STATEFILE=true")
			restartScript := field(t, findNode(t, steps, "displayName", "Validate Node Restart"), "inputs", "inlineScript").Value
			restart := expandParameters(t, restartScript, testParameters)
			require.Contains(t, restart, "RESTART_CASE=true CNI_TYPE="+expected.cniType)
		})
	}
}

func TestWindowsStatelessOverlayParameters(t *testing.T) {
	const directory = "singletenancy/azure-cni-overlay-stateless/"
	run := readPipeline(t, "run-pipeline.yaml")
	stages := readPipeline(t, directory+"azure-cni-overlay-stateless-e2e.stages.yaml")
	steps := readPipeline(t, directory+"azure-cni-overlay-stateless-e2e.steps.yaml")
	defaults := parameterMap(t, field(t, stages, "parameters"))
	require.Equal(t, "windows", defaults["os"])
	caller := findNode(t, run, "template", directory+"azure-cni-overlay-stateless-e2e.stages.yaml")
	parameters := bindParameters(t, defaults, parameterMap(t, field(t, caller, "parameters")))

	install := findNode(t, stages, "template", "../../templates/network-install.yaml")
	provisioning := resolveParameters(t, install, parameters)
	require.Equal(t, "windows", provisioning["os"])
	require.Equal(t, "INSTALL_AZURE_VNET_STATELESS", provisioning["installFlag"])

	tests := findNode(t, stages, "template", "azure-cni-overlay-stateless-e2e.steps.yaml")
	forwarded := resolveParameters(t, tests, parameters)
	testDefaults := parameterMap(t, field(t, steps, "parameters"))
	require.Equal(t, "windows", testDefaults["os"])
	testParameters := bindParameters(t, testDefaults, forwarded)
	loadScript := field(t, findNode(t, steps, "name", "WindowsOverlayControlPlaneScaleTests"), "script").Value
	load := expandParameters(t, loadScript, testParameters)
	for _, argument := range []string{
		"OS_TYPE=windows", "CNI_TYPE=stateless", "VALIDATE_STATEFILE=true",
		"VALIDATE_V4OVERLAY=true", "INSTALL_AZURE_VNET_STATELESS=true",
	} {
		require.Contains(t, load, argument)
	}
	require.NotContains(t, load, "OS_TYPE=linux")

	restartScript := field(t, findNode(t, steps, "displayName", "Validate Node Restart"), "inputs", "inlineScript").Value
	restart := expandParameters(t, restartScript, testParameters)
	require.True(t, strings.HasPrefix(strings.TrimSpace(restart), "set -e\n"))
	require.Contains(t, restart, "make -C ./hack/aks set-kubeconf")
	require.Contains(t, restart, "make -C ./hack/aks azcfg")
	require.Contains(t, restart, "make test-validate-state OS_TYPE=windows RESTART_CASE=true CNI_TYPE=stateless")
	require.NotContains(t, restart, "cd test/integration/load")
}

func TestManifoldCandidateParameters(t *testing.T) {
	pipeline := readPipeline(t, "multitenancy/swiftv2-manifold-e2e.stages.yaml")
	trigger := field(t, findNode(t, pipeline, "task", "TriggerBuild@3"), "inputs")
	require.Equal(t, "391699", field(t, trigger, "buildDefinition").Value)
	require.Equal(t, "refs/heads/master", field(t, trigger, "branchToUse").Value)
	var parameters map[string]any
	require.NoError(t, yaml.Unmarshal([]byte("{"+field(t, trigger, "templateParameters").Value+"}"), &parameters))
	require.Equal(t, map[string]any{
		"westus2":              false,
		"centraluseuap":        false,
		"australiaeast":        true,
		"runMode":              "regular",
		"updateComponents":     true,
		"useAcnPublic":         true,
		"cnscniversion":        "$(CNS_VERSION)",
		"cnscniversionwindows": "$(CNS_VERSION)",
		"cnscniImagePrefix":    "$(IMAGE_REPO_PATH_REF)",
		"npmversion":           "$(NPM_VERSION)",
	}, parameters)
}
