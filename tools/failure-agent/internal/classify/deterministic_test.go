package classify

import (
	"testing"

	"github.com/Azure/azure-container-networking/tools/failure-agent/internal/model"
)

func TestDeterministicUsesTopSignature(t *testing.T) {
	ev := model.Evidence{TopErrorLines: []string{"ImagePullBackOff azure-cns"}}
	matches := []model.SignatureMatch{
		{ID: "image-pull", Category: model.CategoryClusterBringupFailure, Confidence: 0.75, Owner: "acn-cni", Description: "image pull failed"},
	}

	c := Deterministic(model.RunContext{}, ev, matches)

	if c.Category != model.CategoryClusterBringupFailure {
		t.Errorf("category: got %s", c.Category)
	}
	if c.Confidence != 0.75 {
		t.Errorf("confidence: got %v", c.Confidence)
	}
	if c.RecommendedOwner != "acn-cni" {
		t.Errorf("owner: got %q", c.RecommendedOwner)
	}
	if c.Source != "deterministic" {
		t.Errorf("source: got %q", c.Source)
	}
}

func TestDeterministicUnknownWhenNoMatch(t *testing.T) {
	ev := model.Evidence{TopErrorLines: []string{"some unrecognized error"}}
	c := Deterministic(model.RunContext{}, ev, nil)

	if c.Category != model.CategoryUnknownNeedsHuman {
		t.Errorf("expected unknown category, got %s", c.Category)
	}
	if c.Confidence != 0 {
		t.Errorf("expected zero confidence, got %v", c.Confidence)
	}
	if len(c.TopEvidence) != 1 {
		t.Errorf("expected evidence carried through, got %v", c.TopEvidence)
	}
}
