// Package fingerprint derives a stable identifier for a class of failure by
// normalizing run-specific noise (timestamps, IDs, IPs, pod suffixes) out of
// the failure signal, so the same underlying failure hashes identically across
// runs. This drives recurrence detection and idempotent reporting.
package fingerprint

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"

	"github.com/Azure/azure-container-networking/tools/failure-agent/internal/model"
)

// topErrorLinesUsed is how many error lines feed the fingerprint signal.
const topErrorLinesUsed = 3

// hashLen is the number of hex characters kept from the digest.
const hashLen = 12

// Substitution patterns are applied in order: more specific tokens first so
// generic digit scrubbing does not corrupt them.
var (
	reTimestamp = regexp.MustCompile(`\d{4}-\d{2}-\d{2}[t ]\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:z|[+-]\d{2}:?\d{2})?`)
	reGUID      = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)
	reIPv4      = regexp.MustCompile(`\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\b`)
	// rePodFull matches a deployment pod's generated tail: a replicaset hash
	// (8-10 chars) plus a 5-char random suffix, e.g. "-7f9c8b6d4-xz12k".
	rePodFull = regexp.MustCompile(`-[a-z0-9]{8,10}-[a-z0-9]{5}\b`)
	// rePodRand matches a standalone 5-char generated suffix; only collapsed
	// when it contains a digit (see Normalize) to avoid scrubbing real words.
	rePodRand = regexp.MustCompile(`-[a-z0-9]{5}\b`)
	reHex     = regexp.MustCompile(`\b[0-9a-f]{7,}\b`)
	reNumber  = regexp.MustCompile(`\d+`)
	reSpace   = regexp.MustCompile(`\s+`)
)

// Normalize strips run-specific noise from a single line and lowercases it.
func Normalize(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = reTimestamp.ReplaceAllString(s, "<ts>")
	s = reGUID.ReplaceAllString(s, "<guid>")
	s = reIPv4.ReplaceAllString(s, "<ip>")
	s = rePodFull.ReplaceAllString(s, "-<pod>")
	s = rePodRand.ReplaceAllStringFunc(s, func(m string) string {
		if strings.ContainsAny(m, "0123456789") {
			return "-<pod>"
		}
		return m
	})
	s = reHex.ReplaceAllString(s, "<hex>")
	s = reNumber.ReplaceAllString(s, "<n>")
	s = reSpace.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

// Compute builds the normalized signal and its stable short hash.
func Compute(rc model.RunContext, ev model.Evidence) model.Fingerprint {
	parts := []string{
		Normalize(rc.PipelineName),
		Normalize(rc.StageName),
		Normalize(rc.JobName),
		Normalize(rc.ClusterType),
		strings.ToLower(strings.TrimSpace(rc.OS)),
		strings.ToLower(strings.TrimSpace(rc.CNI)),
	}

	n := topErrorLinesUsed
	if len(ev.TopErrorLines) < n {
		n = len(ev.TopErrorLines)
	}
	for _, line := range ev.TopErrorLines[:n] {
		parts = append(parts, Normalize(line))
	}

	signal := strings.Join(parts, "|")
	sum := sha256.Sum256([]byte(signal))
	return model.Fingerprint{
		Hash:             hex.EncodeToString(sum[:])[:hashLen],
		NormalizedSignal: signal,
	}
}
