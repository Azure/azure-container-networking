package azuremonitor

import (
	k8s "github.com/Azure/azure-container-networking/test/e2e/framework/kubernetes"
	"github.com/Azure/azure-container-networking/test/e2e/framework/types"
)

// todo: once AMA is rolled out
func ValidateAMATargets() *types.Scenario {
	steps := []*types.StepWrapper{
		{
			Step: &k8s.PortForward{
				Namespace:     "kube-system",
				LabelSelector: "k8s-app=cilium",
				LocalPort:     "9965",
				RemotePort:    "9965",
			},
			Opts: &types.StepOptions{
				RunInBackgroundWithID: "validate-ama-targets",
			},
		},
		{
			Step: &VerifyPrometheusMetrics{
				Address: "http://localhost:9090",
			},
		},
		{
			Step: &types.Stop{
				BackgroundID: "validate-ama-targets",
			},
		},
	}

	return types.NewScenario(
		"Validate that drop metrics are present in the prometheus endpoint",
		steps...,
	)
}
