package signatures

import (
	"strings"
	"testing"

	"github.com/Azure/azure-container-networking/tools/failure-agent/internal/model"
)

const validCatalog = `
signatures:
  - id: image-pull
    category: cluster_bringup_failure
    description: image pull failed
    owner: acn-cni
    recommendation: check the image tag
    confidence: 0.75
    anyOf:
      - "ImagePullBackOff"
  - id: low-conf-flake
    category: known_flake
    description: timeout flake
    confidence: 0.5
    anyOf:
      - "context deadline exceeded"
`

func TestLoadAndMatchSortedByConfidence(t *testing.T) {
	set, err := Load(strings.NewReader(validCatalog))
	if err != nil {
		t.Fatalf("unexpected load error: %v", err)
	}

	ev := model.Evidence{TopErrorLines: []string{
		"pod azure-cns ImagePullBackOff",
		"test failed: context deadline exceeded",
	}}
	matches := set.Match(model.RunContext{}, ev)

	if len(matches) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(matches))
	}
	if matches[0].ID != "image-pull" {
		t.Errorf("expected highest-confidence match first, got %q", matches[0].ID)
	}
	if matches[0].MatchedOn != "ImagePullBackOff" {
		t.Errorf("expected MatchedOn recorded, got %q", matches[0].MatchedOn)
	}
}

func TestLoadRejectsInvalidCategory(t *testing.T) {
	_, err := Load(strings.NewReader(`
signatures:
  - id: bad
    category: not_a_category
    confidence: 0.5
    anyOf: ["x"]
`))
	if err == nil {
		t.Fatal("expected error for invalid category")
	}
}

func TestLoadRejectsDuplicateID(t *testing.T) {
	_, err := Load(strings.NewReader(`
signatures:
  - id: dup
    category: known_flake
    confidence: 0.5
    anyOf: ["a"]
  - id: dup
    category: known_flake
    confidence: 0.5
    anyOf: ["b"]
`))
	if err == nil {
		t.Fatal("expected error for duplicate id")
	}
}

func TestLoadRejectsBadRegexAndMissingPatterns(t *testing.T) {
	if _, err := Load(strings.NewReader(`
signatures:
  - id: badre
    category: known_flake
    confidence: 0.5
    anyOf: ["("]
`)); err == nil {
		t.Fatal("expected error for bad regex")
	}

	if _, err := Load(strings.NewReader(`
signatures:
  - id: empty
    category: known_flake
    confidence: 0.5
    anyOf: []
`)); err == nil {
		t.Fatal("expected error for missing patterns")
	}
}

func TestLoadRejectsOutOfRangeConfidence(t *testing.T) {
	if _, err := Load(strings.NewReader(`
signatures:
  - id: over
    category: known_flake
    confidence: 1.5
    anyOf: ["x"]
`)); err == nil {
		t.Fatal("expected error for confidence out of range")
	}
}
