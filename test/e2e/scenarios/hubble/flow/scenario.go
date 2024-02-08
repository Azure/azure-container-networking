package flow

import "github.com/Azure/azure-container-networking/test/e2e/framework/types"

// todo: once AMA is rolled out
func ValidateAMATargets() *types.Scenario {
	return &types.Scenario{
		Steps: []*types.StepWrapper{
			{
				Step: &ValidateHubbleFlowMetric{
					LocalPort: "9090",
				},
			},
		},
	}
}
