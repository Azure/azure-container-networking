// Package signatures loads the repo-committed catalog of known failure
// patterns and matches them against collected evidence. A match grounds the
// classifier with a deterministic, human-curated category and recommendation.
package signatures

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/Azure/azure-container-networking/tools/failure-agent/internal/model"
	"gopkg.in/yaml.v3"
)

// validCategories constrains the category field of a signature.
var validCategories = map[model.FailureCategory]bool{
	model.CategoryPRRegression:          true,
	model.CategoryClusterBringupFailure: true,
	model.CategoryPipelineInfraConfig:   true,
	model.CategoryKnownFlake:            true,
	model.CategoryUnknownNeedsHuman:     true,
}

// rawSignature is the on-disk YAML shape.
type rawSignature struct {
	ID             string   `yaml:"id"`
	Category       string   `yaml:"category"`
	Description    string   `yaml:"description"`
	Owner          string   `yaml:"owner"`
	Recommendation string   `yaml:"recommendation"`
	Confidence     float64  `yaml:"confidence"`
	AnyOf          []string `yaml:"anyOf"`
}

type rawCatalog struct {
	Signatures []rawSignature `yaml:"signatures"`
}

type compiledSignature struct {
	meta     model.SignatureMatch
	patterns []*regexp.Regexp
	raw      []string
}

// Set is a compiled, ready-to-match catalog of signatures.
type Set struct {
	sigs []compiledSignature
}

// LoadFile reads and compiles the signature catalog at path.
func LoadFile(path string) (*Set, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening signatures file: %w", err)
	}
	defer f.Close()
	return Load(f)
}

// Load reads and compiles a signature catalog from r, validating each entry.
func Load(r io.Reader) (*Set, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("reading signatures: %w", err)
	}

	var catalog rawCatalog
	if err := yaml.Unmarshal(data, &catalog); err != nil {
		return nil, fmt.Errorf("parsing signatures yaml: %w", err)
	}

	set := &Set{}
	ids := map[string]bool{}
	for i, rs := range catalog.Signatures {
		if rs.ID == "" {
			return nil, fmt.Errorf("signature %d: missing id", i)
		}
		if ids[rs.ID] {
			return nil, fmt.Errorf("signature %q: duplicate id", rs.ID)
		}
		ids[rs.ID] = true

		cat := model.FailureCategory(rs.Category)
		if !validCategories[cat] {
			return nil, fmt.Errorf("signature %q: invalid category %q", rs.ID, rs.Category)
		}
		if rs.Confidence < 0 || rs.Confidence > 1 {
			return nil, fmt.Errorf("signature %q: confidence %v out of range [0,1]", rs.ID, rs.Confidence)
		}
		if len(rs.AnyOf) == 0 {
			return nil, fmt.Errorf("signature %q: no anyOf patterns", rs.ID)
		}

		cs := compiledSignature{
			meta: model.SignatureMatch{
				ID:             rs.ID,
				Category:       cat,
				Description:    rs.Description,
				Owner:          rs.Owner,
				Recommendation: rs.Recommendation,
				Confidence:     rs.Confidence,
			},
		}
		for _, pat := range rs.AnyOf {
			re, compErr := regexp.Compile(pat)
			if compErr != nil {
				return nil, fmt.Errorf("signature %q: bad pattern %q: %w", rs.ID, pat, compErr)
			}
			cs.patterns = append(cs.patterns, re)
			cs.raw = append(cs.raw, pat)
		}
		set.sigs = append(set.sigs, cs)
	}
	return set, nil
}

// Match returns all signatures whose patterns match the evidence, sorted by
// descending confidence. The searchable text combines error lines, excerpts,
// and the failing stage/job names.
func (s *Set) Match(rc model.RunContext, ev model.Evidence) []model.SignatureMatch {
	text := searchableText(rc, ev)

	var matches []model.SignatureMatch
	for _, cs := range s.sigs {
		for i, re := range cs.patterns {
			if re.MatchString(text) {
				m := cs.meta
				m.MatchedOn = cs.raw[i]
				matches = append(matches, m)
				break
			}
		}
	}

	sort.SliceStable(matches, func(i, j int) bool {
		return matches[i].Confidence > matches[j].Confidence
	})
	return matches
}

func searchableText(rc model.RunContext, ev model.Evidence) string {
	var b strings.Builder
	b.WriteString(rc.StageName)
	b.WriteByte('\n')
	b.WriteString(rc.JobName)
	b.WriteByte('\n')
	for _, l := range ev.TopErrorLines {
		b.WriteString(l)
		b.WriteByte('\n')
	}
	for _, e := range ev.Excerpts {
		b.WriteString(e)
		b.WriteByte('\n')
	}
	return b.String()
}
