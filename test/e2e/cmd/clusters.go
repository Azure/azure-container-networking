package main

import (
	"fmt"
	"os/exec"
	"os/user"
	"strconv"
	"strings"
	"time"

	"github.com/Azure/azure-container-networking/test/e2e/framework/azure"
	"github.com/Azure/azure-container-networking/test/e2e/framework/types"
	"github.com/spf13/cobra"
)

func newClusterCmd() *cobra.Command {
	clusterCmd := &cobra.Command{
		Use:   "cluster",
		Short: "deploys a cluster",
		CompletionOptions: cobra.CompletionOptions{
			DisableDefaultCmd: true,
		},
	}

	clusterCmd.AddCommand(newBYOCiliumCmd())

	return clusterCmd
}

func newBYOCiliumCmd() *cobra.Command {
	byocilium := &cobra.Command{
		Use:   "byocilium",
		Short: "deploys a BYO Cilium Cluster",
		RunE: func(cmd *cobra.Command, args []string) error {
			job := types.NewJob("deploy BYO Cilium Cluster")

			sub, err := GetCurrentAzCLISubscriptionID()
			if err != nil {
				return fmt.Errorf("failed to get subscription id: %w", err)
			}

			curuser, _ := user.Current()
			clusterName := curuser.Username + "-byocilium-" + strconv.FormatInt(time.Now().Unix(), 10)

			job.AddStep(&azure.CreateResourceGroup{
				SubscriptionID:    sub,
				ResourceGroupName: clusterName,
				Location:          "westus2",
			}, nil)

			job.AddStep(&azure.CreateVNet{
				VnetName:         "testvnet",
				VnetAddressSpace: "10.0.0.0/9",
			}, nil)

			job.AddStep(&azure.CreateSubnet{
				SubnetName:         "testsubnet",
				SubnetAddressSpace: "10.0.0.0/12",
			}, nil)

			job.AddStep(&azure.CreateBYOCiliumCluster{
				ClusterName:  clusterName,
				PodCidr:      "10.128.0.0/9",
				DNSServiceIP: "192.168.0.10",
				ServiceCidr:  "192.168.0.0/28",
			}, nil)

			if err := job.Run(); err != nil {
				return err // nolint // wrapping this error is noise, Cobra will handle
			}

			fmt.Printf("\nto get the kubeconfig for this cluster, run:\n\n\taz aks get-credentials --resource-group %s --name %s\n\n", clusterName, clusterName)

			return nil
		},
	}

	return byocilium
}

func GetCurrentAzCLISubscriptionID() (string, error) {
	// this function requires Azure CLI to be installed, as even the Azure SDK for Go makes a call to it when using the Azure CLI credential type:
	// https://github.com/Azure/azure-sdk-for-go/blob/0cda95c7a7e55361d9602a7c8a141eec584f75cc/sdk/azidentity/azure_cli_credential.go#L116

	cmd := exec.Command("az", "account", "show", "--query=id", "-o", "tsv")
	output, err := cmd.Output()
	if err != nil {
		return "", err //nolint // wrapping this error is noise, caller will handle
	}
	return strings.TrimSpace(string(output)), nil
}
