// Package policy applies deterministic, human-owned rules on top of a
// classification: confidence banding, the advisory retention decision, and the
// recommended next-action wording. These rules live in Go (not the LLM) so they
// are auditable and stable.
package policy

import (
	"fmt"

	"github.com/Azure/azure-container-networking/tools/failure-agent/internal/model"
)

const (
	highThreshold   = 0.8
	mediumThreshold = 0.5
)

// Band buckets a numeric confidence into high/medium/low.
func Band(confidence float64) model.ConfidenceBand {
	switch {
	case confidence >= highThreshold:
		return model.BandHigh
	case confidence >= mediumThreshold:
		return model.BandMedium
	default:
		return model.BandLow
	}
}

// Retention returns the advisory retention decision. Low-confidence or
// unclassified failures are worth keeping briefly for human inspection;
// everything else can follow the normal teardown.
func Retention(category model.FailureCategory, confidence float64) model.RetentionDecision {
	if category == model.CategoryUnknownNeedsHuman || Band(confidence) == model.BandLow {
		return model.RetentionRetainTTL
	}
	return model.RetentionDelete
}

// RecommendedAction composes the human-facing next step. It prefers a matched
// signature's recommendation, falls back to a category default, and appends a
// retention note when retention is advised.
func RecommendedAction(category model.FailureCategory, matches []model.SignatureMatch, retention model.RetentionDecision) string {
	action := categoryDefault(category)
	if len(matches) > 0 && matches[0].Recommendation != "" {
		action = matches[0].Recommendation
	}
	if retention == model.RetentionRetainTTL {
		action = fmt.Sprintf("%s Consider retaining the cluster briefly (retain_ttl) to inspect live state before teardown.", action)
	}
	return action
}

func categoryDefault(category model.FailureCategory) string {
	switch category {
	case model.CategoryPRRegression:
		return "Likely caused by the change under test; review the diff against the failing component and re-run after a fix."
	case model.CategoryClusterBringupFailure:
		return "Cluster bring-up failed; inspect CNS/CNI daemonset logs and node/NNC status for the affected nodes."
	case model.CategoryPipelineInfraConfig:
		return "Likely a pipeline/infra/config issue rather than a product bug; verify agent, credentials, quota, and connectivity, then re-run."
	case model.CategoryKnownFlake:
		return "Recognized intermittent failure; re-run the failed stage and escalate if it recurs with the same fingerprint."
	default:
		return "Unable to classify automatically; route to a human for triage using the attached evidence."
	}
}
