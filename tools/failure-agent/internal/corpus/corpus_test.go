package corpus

import (
	"context"
	"errors"
	"strings"
	"testing"

	"go.uber.org/zap"
)

// fakeBlob is an in-memory blobAPI: names lists the container, and blobs maps a
// name to its bytes or a download error.
type fakeBlob struct {
	names   []string
	blobs   map[string][]byte
	errs    map[string]error
	listErr error
}

func (f *fakeBlob) list(context.Context) ([]string, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.names, nil
}

func (f *fakeBlob) download(_ context.Context, name string) ([]byte, error) {
	if err := f.errs[name]; err != nil {
		return nil, err
	}
	return f.blobs[name], nil
}

func newClient(api blobAPI) *Client {
	return &Client{api: api, logger: zap.NewNop()}
}

func TestFetchFiltersMarkdownAndSkipsBadBlobs(t *testing.T) {
	oversized := make([]byte, maxBlobBytes+1)
	for i := range oversized {
		oversized[i] = 'a'
	}
	api := &fakeBlob{
		names: []string{
			"123/report.md",
			"456/report.md",
			"789/incident.json", // not markdown -> skipped
			"999/report.md",     // download error -> skipped
			"111/report.md",     // oversized -> skipped
			"222/report.md",     // empty -> skipped
		},
		blobs: map[string][]byte{
			"123/report.md": []byte("# Report 123\nCNS restarted"),
			"456/report.md": []byte("# Report 456\nnode NotReady"),
			"111/report.md": oversized,
			"222/report.md": []byte("   \n  "),
		},
		errs: map[string]error{
			"999/report.md": errors.New("boom"),
		},
	}

	recs, err := newClient(api).Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(recs) != 2 {
		t.Fatalf("want 2 records, got %d: %+v", len(recs), recs)
	}
	got := map[string]string{}
	for _, r := range recs {
		got[r.Path] = r.Markdown
	}
	if _, ok := got["123/report.md"]; !ok {
		t.Errorf("missing 123/report.md: %+v", got)
	}
	if strings.Contains(got["123/report.md"], "  ") && strings.HasPrefix(got["123/report.md"], " ") {
		t.Errorf("markdown not trimmed: %q", got["123/report.md"])
	}
	if _, ok := got["789/incident.json"]; ok {
		t.Errorf("non-markdown blob should be skipped")
	}
}

func TestFetchListErrorPropagates(t *testing.T) {
	api := &fakeBlob{listErr: errors.New("list failed")}
	if _, err := newClient(api).Fetch(context.Background()); err == nil {
		t.Fatal("expected error when list fails")
	}
}

func TestSelectRelevantRanksByKeyword(t *testing.T) {
	recs := []Record{
		{Path: "a", Markdown: "nothing relevant here"},
		{Path: "b", Markdown: "CNS restarted because the node rebooted"},
		{Path: "c", Markdown: "cns pod restart loop after upgrade"},
	}
	got := SelectRelevant(recs, "why did CNS restart?", 2)
	if len(got) != 2 {
		t.Fatalf("want 2, got %d", len(got))
	}
	// b and c both mention cns/restart; a does not, so it must be excluded.
	for _, r := range got {
		if r.Path == "a" {
			t.Errorf("irrelevant record a should not be selected: %+v", got)
		}
	}
}

func TestSelectRelevantNoMatchesStillReturnsBounded(t *testing.T) {
	recs := []Record{
		{Path: "a", Markdown: "alpha"},
		{Path: "b", Markdown: "beta"},
		{Path: "c", Markdown: "gamma"},
	}
	got := SelectRelevant(recs, "unrelated question xyz", 2)
	if len(got) != 2 {
		t.Fatalf("want 2 (bounded even with no matches), got %d", len(got))
	}
}

func TestSelectRelevantDefaultLimit(t *testing.T) {
	recs := make([]Record, 40)
	for i := range recs {
		recs[i] = Record{Path: string(rune('a' + i%26)), Markdown: "cns restart"}
	}
	got := SelectRelevant(recs, "cns restart", 0)
	if len(got) != defaultLimit {
		t.Fatalf("want default limit %d, got %d", defaultLimit, len(got))
	}
}
