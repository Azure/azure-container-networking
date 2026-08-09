// Package corpus reads historical FAA analysis reports from an Azure Blob
// Storage container. It backs the failure-agent "history" ask mode: given a
// container assumed to hold past report.md files, it downloads them so the
// answerer can ground a build-independent question in prior failures.
//
// This iteration is read-only; archival/upload of new analyses is intentionally
// out of scope.
package corpus

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"go.uber.org/zap"
)

// maxBlobBytes caps a single report download so one oversized blob cannot
// exhaust memory or overflow the prompt.
const maxBlobBytes = 1 << 20 // 1 MiB

// defaultLimit bounds how many reports are fed to the model when the caller does
// not specify a limit.
const defaultLimit = 15

// Record is one historical report pulled from the corpus. Path is the blob name,
// used as the citation label in the answer.
type Record struct {
	Path     string
	Markdown string
}

// blobAPI is the minimal slice of the Azure Blob API the corpus needs. It lets
// tests supply a fake without a real storage account or emulator.
type blobAPI interface {
	list(ctx context.Context) ([]string, error)
	download(ctx context.Context, name string) ([]byte, error)
}

// Client reads historical FAA reports from a single blob container.
type Client struct {
	api    blobAPI
	logger *zap.Logger
}

// NewClient builds a corpus Client from a storage account connection string and
// container name.
func NewClient(connectionString, container string, logger *zap.Logger) (*Client, error) {
	if connectionString == "" {
		return nil, errors.New("storage connection string is required")
	}
	if container == "" {
		return nil, errors.New("storage container is required")
	}
	c, err := azblob.NewClientFromConnectionString(connectionString, nil)
	if err != nil {
		return nil, fmt.Errorf("azblob client: %w", err)
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Client{api: &azBlob{client: c, container: container}, logger: logger}, nil
}

// Fetch lists the container and downloads every markdown report into a Record. A
// blob that fails to download, is oversized, or is empty is skipped with a
// warning so one bad object never fails the whole history answer.
func (c *Client) Fetch(ctx context.Context) ([]Record, error) {
	names, err := c.api.list(ctx)
	if err != nil {
		return nil, err
	}
	var records []Record
	for _, name := range names {
		if !isMarkdown(name) {
			continue
		}
		b, err := c.api.download(ctx, name)
		if err != nil {
			c.logger.Warn("skipping unreadable corpus blob", zap.String("blob", name), zap.Error(err))
			continue
		}
		if len(b) > maxBlobBytes {
			c.logger.Warn("skipping oversized corpus blob", zap.String("blob", name), zap.Int("bytes", len(b)))
			continue
		}
		text := strings.TrimSpace(string(b))
		if text == "" {
			continue
		}
		records = append(records, Record{Path: name, Markdown: text})
	}
	return records, nil
}

// SelectRelevant ranks records by keyword overlap with the question and returns
// at most limit of them, most relevant first. It always returns a bounded,
// non-empty-when-possible set: with no keyword matches it still returns the
// first records so the model can answer (or state there is no precedent).
func SelectRelevant(records []Record, question string, limit int) []Record {
	if limit <= 0 {
		limit = defaultLimit
	}
	terms := keywords(question)

	type scored struct {
		rec   Record
		score int
		idx   int
	}
	ranked := make([]scored, len(records))
	for i, r := range records {
		ranked[i] = scored{rec: r, score: score(r.Markdown, terms), idx: i}
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].score != ranked[j].score {
			return ranked[i].score > ranked[j].score
		}
		return ranked[i].idx < ranked[j].idx
	})

	out := make([]Record, 0, limit)
	for _, s := range ranked {
		if len(out) >= limit {
			break
		}
		out = append(out, s.rec)
	}
	return out
}

func isMarkdown(name string) bool {
	return strings.HasSuffix(strings.ToLower(name), ".md")
}

// keywords lowercases the question and returns its distinct words longer than
// two characters, which is a good-enough relevance signal for the MVP.
func keywords(question string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, w := range strings.FieldsFunc(strings.ToLower(question), func(r rune) bool {
		return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9')
	}) {
		if len(w) <= 2 {
			continue
		}
		if _, ok := seen[w]; ok {
			continue
		}
		seen[w] = struct{}{}
		out = append(out, w)
	}
	return out
}

// score counts how many question terms appear in the report text.
func score(markdown string, terms []string) int {
	if len(terms) == 0 {
		return 0
	}
	lower := strings.ToLower(markdown)
	n := 0
	for _, t := range terms {
		if strings.Contains(lower, t) {
			n++
		}
	}
	return n
}

// azBlob is the production blobAPI backed by an azblob container client.
type azBlob struct {
	client    *azblob.Client
	container string
}

func (a *azBlob) list(ctx context.Context) ([]string, error) {
	var names []string
	pager := a.client.NewListBlobsFlatPager(a.container, nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("listing blobs: %w", err)
		}
		for _, b := range page.Segment.BlobItems {
			if b.Name != nil {
				names = append(names, *b.Name)
			}
		}
	}
	return names, nil
}

func (a *azBlob) download(ctx context.Context, name string) ([]byte, error) {
	resp, err := a.client.DownloadStream(ctx, a.container, name, nil)
	if err != nil {
		return nil, fmt.Errorf("downloading %s: %w", name, err)
	}
	defer func() { _ = resp.Body.Close() }()
	return io.ReadAll(io.LimitReader(resp.Body, maxBlobBytes+1))
}
