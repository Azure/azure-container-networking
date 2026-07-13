package store

import (
	"context"
	"strings"
	"testing"
)

func seedIncidents(t *testing.T, s *Store) {
	t.Helper()
	ctx := context.Background()
	// "abc" recurs 3 times -> flakiness signal.
	for i := 0; i < 3; i++ {
		if _, err := s.CreateIncident(ctx, Incident{
			Fingerprint: "abc", Pipeline: "ACN PR", Category: "cluster_bringup_failure",
			Summary: "CNS not ready", Status: StatusNew,
		}); err != nil {
			t.Fatalf("seed abc: %v", err)
		}
	}
	// "xyz" occurs once -> below threshold.
	if _, err := s.CreateIncident(ctx, Incident{
		Fingerprint: "xyz", Pipeline: "ACN Nightly", Category: "known_flake", Status: StatusNew,
	}); err != nil {
		t.Fatalf("seed xyz: %v", err)
	}
}

func TestFlakinessSurfacesRecurringFingerprint(t *testing.T) {
	s := newTestStore(t)
	seedIncidents(t, s)

	rep, err := s.Flakiness(context.Background(), 10)
	if err != nil {
		t.Fatalf("flakiness: %v", err)
	}
	if rep.TotalIncidents != 4 {
		t.Errorf("total: got %d, want 4", rep.TotalIncidents)
	}
	if len(rep.TopFingerprints) != 1 {
		t.Fatalf("expected only the recurring fingerprint, got %d", len(rep.TopFingerprints))
	}
	top := rep.TopFingerprints[0]
	if top.Fingerprint != "abc" || top.Occurrences != 3 {
		t.Errorf("top fingerprint: got %s x%d, want abc x3", top.Fingerprint, top.Occurrences)
	}
	if top.Category != "cluster_bringup_failure" {
		t.Errorf("category: got %s", top.Category)
	}
}

func TestFlakinessHotspots(t *testing.T) {
	s := newTestStore(t)
	seedIncidents(t, s)

	rep, err := s.Flakiness(context.Background(), 10)
	if err != nil {
		t.Fatalf("flakiness: %v", err)
	}
	if len(rep.CategoryHotspots) == 0 || rep.CategoryHotspots[0].Label != "cluster_bringup_failure" {
		t.Errorf("expected cluster_bringup_failure as top category, got %+v", rep.CategoryHotspots)
	}
	if len(rep.PipelineHotspots) == 0 || rep.PipelineHotspots[0].Label != "ACN PR" {
		t.Errorf("expected ACN PR as top pipeline, got %+v", rep.PipelineHotspots)
	}
}

func TestRenderFlakinessMarkdown(t *testing.T) {
	s := newTestStore(t)
	seedIncidents(t, s)

	rep, _ := s.Flakiness(context.Background(), 10)
	md := RenderFlakiness(rep)

	for _, want := range []string{"Flakiness Insights", "Recurring failures", "abc", "Category hotspots"} {
		if !strings.Contains(md, want) {
			t.Errorf("expected %q in rendered report", want)
		}
	}
}

func TestRenderFlakinessEmpty(t *testing.T) {
	s := newTestStore(t)
	rep, _ := s.Flakiness(context.Background(), 10)
	md := RenderFlakiness(rep)
	if !strings.Contains(md, "No fingerprint has recurred") {
		t.Error("expected empty-state message for recurring failures")
	}
}
