package policy

import (
	"strings"
	"testing"

	"github.com/Azure/azure-container-networking/tools/failure-agent/internal/model"
)

func TestBand(t *testing.T) {
	cases := []struct {
		conf float64
		want model.ConfidenceBand
	}{
		{0.95, model.BandHigh},
		{0.8, model.BandHigh},
		{0.79, model.BandMedium},
		{0.5, model.BandMedium},
		{0.49, model.BandLow},
		{0, model.BandLow},
	}
	for _, c := range cases {
		if got := Band(c.conf); got != c.want {
			t.Errorf("Band(%v) = %s, want %s", c.conf, got, c.want)
		}
	}
}

func TestRetention(t *testing.T) {
	if got := Retention(model.CategoryPRRegression, 0.9); got != model.RetentionDelete {
		t.Errorf("high-confidence regression should delete, got %s", got)
	}
	if got := Retention(model.CategoryPRRegression, 0.3); got != model.RetentionRetainTTL {
		t.Errorf("low-confidence should retain, got %s", got)
	}
	if got := Retention(model.CategoryUnknownNeedsHuman, 0.9); got != model.RetentionRetainTTL {
		t.Errorf("unknown should retain regardless of confidence, got %s", got)
	}
}

func TestRecommendedActionPrefersSignature(t *testing.T) {
	matches := []model.SignatureMatch{{Recommendation: "check the image tag exists"}}
	got := RecommendedAction(model.CategoryClusterBringupFailure, matches, model.RetentionDelete)
	if !strings.Contains(got, "check the image tag exists") {
		t.Errorf("expected signature recommendation, got %q", got)
	}
}

func TestRecommendedActionAppendsRetentionNote(t *testing.T) {
	got := RecommendedAction(model.CategoryUnknownNeedsHuman, nil, model.RetentionRetainTTL)
	if !strings.Contains(got, "retain_ttl") {
		t.Errorf("expected retention note, got %q", got)
	}
}
