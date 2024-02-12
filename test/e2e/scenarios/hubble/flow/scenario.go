package flow

import "github.com/Azure/azure-container-networking/test/e2e/framework/types"

// todo: once AMA is rolled out
func ValidateAMATargets() *types.Scenario {
	name := "Validate that flow metrics are present in the prometheus endpoint"
	steps := []*types.StepWrapper{
		{
			Step: &ValidateHubbleFlowMetric{
				LocalPort: "9090",
			},
		},
	}
	return types.NewScenario(name, steps...)
}
