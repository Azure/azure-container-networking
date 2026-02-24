package subcmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	k8syaml "sigs.k8s.io/yaml"

	"github.com/azure/aksmigrate/pkg/policy"
	"github.com/azure/aksmigrate/pkg/types"
	"github.com/azure/aksmigrate/pkg/utils"
)

// NewTranslateCmd returns the "translate" subcommand.
func NewTranslateCmd() *cobra.Command {
	var (
		kubeconfig string
		inputDir   string
		outputDir  string
		k8sVersion string
	)

	cmd := &cobra.Command{
		Use:   "translate",
		Short: "Patch NetworkPolicies and generate CiliumNetworkPolicies",
		Long: `Patches existing Kubernetes NetworkPolicies and generates supplementary
CiliumNetworkPolicies to maintain behavioral equivalence after migrating
from Azure NPM to Cilium.`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			var resources *types.ClusterResources
			var err error

			switch {
			case inputDir != "":
				resources, err = utils.LoadFromDirectory(inputDir)
				if err != nil {
					return fmt.Errorf("failed to load from directory %s: %w", inputDir, err)
				}
			case kubeconfig != "":
				client, _, kerr := utils.NewKubeClient(kubeconfig)
				if kerr != nil {
					return fmt.Errorf("failed to create kube client: %w", kerr)
				}
				resources, err = utils.LoadFromCluster(context.Background(), client)
				if err != nil {
					return fmt.Errorf("failed to load from cluster: %w", err)
				}
			default:
				client, _, kerr := utils.NewKubeClient("")
				if kerr != nil {
					return fmt.Errorf("failed to create kube client with default kubeconfig: %w", kerr)
				}
				resources, err = utils.LoadFromCluster(context.Background(), client)
				if err != nil {
					return fmt.Errorf("failed to load from cluster: %w", err)
				}
			}

			translator := policy.NewTranslator(resources, k8sVersion)
			output := translator.Translate()

			patchedDir := filepath.Join(outputDir, "patched")
			ciliumDir := filepath.Join(outputDir, "cilium")

			if err := os.MkdirAll(patchedDir, 0755); err != nil {
				return fmt.Errorf("failed to create patched directory %s: %w", patchedDir, err)
			}
			if err := os.MkdirAll(ciliumDir, 0755); err != nil {
				return fmt.Errorf("failed to create cilium directory %s: %w", ciliumDir, err)
			}

			for _, pp := range output.PatchedPolicies {
				data, err := k8syaml.Marshal(pp.Patched)
				if err != nil {
					return fmt.Errorf("failed to marshal patched policy %s/%s: %w", pp.Patched.Namespace, pp.Patched.Name, err)
				}

				filename := fmt.Sprintf("%s-%s.yaml", pp.Patched.Namespace, pp.Patched.Name)
				filePath := filepath.Join(patchedDir, filename)
				if err := os.WriteFile(filePath, data, 0644); err != nil {
					return fmt.Errorf("failed to write patched policy to %s: %w", filePath, err)
				}
				fmt.Printf("Wrote patched policy: %s\n", filePath)
			}

			for _, cp := range output.CiliumPolicies {
				yamlContent := policy.RenderCiliumPolicyYAML(cp)

				filename := fmt.Sprintf("%s-%s.yaml", cp.Namespace, cp.Name)
				filePath := filepath.Join(ciliumDir, filename)
				if err := os.WriteFile(filePath, []byte(yamlContent), 0644); err != nil {
					return fmt.Errorf("failed to write cilium policy to %s: %w", filePath, err)
				}
				fmt.Printf("Wrote CiliumNetworkPolicy: %s\n", filePath)
			}

			utils.PrintTranslationSummary(output)

			return nil
		},
	}

	cmd.Flags().StringVar(&kubeconfig, "kubeconfig", "", "path to kubeconfig file")
	cmd.Flags().StringVar(&inputDir, "input-dir", "", "path to directory containing Kubernetes YAML manifests")
	cmd.Flags().StringVar(&outputDir, "output-dir", "./cilium-patches", "output directory for generated YAML files")
	cmd.Flags().StringVar(&k8sVersion, "k8s-version", "1.29", "target Kubernetes version (determines Cilium version)")

	return cmd
}
