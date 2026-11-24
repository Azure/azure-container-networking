//go:build linux

//nolint:goconst // Keep wire fixtures independent of executor constants.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/netip"
	"reflect"
	"strings"
	"testing"
)

func marshalTest(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func decodeTest[T any](t *testing.T, data []byte) T {
	t.Helper()
	var value T
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func testImage(t *testing.T) imageIdentity {
	t.Helper()
	reference := "example.invalid/dalec/synthetic/azure-npm:test@sha256:" + strings.Repeat("a", 64)
	image, err := parseImage(reference)
	if err != nil {
		t.Fatal(err)
	}
	return image
}

func TestCorpusRejectsMissingWork(t *testing.T) {
	raw, err := runfiles.ReadFile("cases.v1.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name string
		edit func(*corpus)
	}{
		{"empty", func(c *corpus) { c.Cases = nil }},
		{"missing case", func(c *corpus) { c.Cases = c.Cases[:9] }},
		{"duplicate case", func(c *corpus) { c.Cases[1] = c.Cases[0] }},
		{"missing probe", func(c *corpus) { c.Cases[0].Probes = c.Cases[0].Probes[:1] }},
	} {
		t.Run(test.name, func(t *testing.T) {
			value := decodeTest[corpus](t, raw)
			test.edit(&value)
			if _, err := parseCorpus(marshalTest(t, value)); err == nil {
				t.Fatal("accepted incomplete conformance work")
			}
		})
	}
	if _, err := parseCorpus(append(bytes.Clone(raw), []byte("{}")...)); err == nil {
		t.Fatal("accepted trailing corpus content")
	}
}

func TestCorpusRejectionCodes(t *testing.T) {
	raw, err := runfiles.ReadFile("cases.v1.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parseCorpus(raw); err != nil {
		t.Fatalf("rejected valid embedded corpus: %v", err)
	}
	for _, test := range []struct {
		name string
		edit func(*corpus)
	}{
		{"unknown code", func(c *corpus) { c.Cases[4].Rejection = "unknown" }},
		{"unknown code with message", func(c *corpus) {
			c.Cases[4].Rejection = "unknown"
			c.RejectionMessages["unknown"] = "unknown rejection message"
		}},
		{"missing selector message", func(c *corpus) { delete(c.RejectionMessages, "unsupported-peer-selector") }},
		{"empty named port message", func(c *corpus) { c.RejectionMessages["unsupported-named-port"] = "" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			value := decodeTest[corpus](t, raw)
			test.edit(&value)
			if _, err := parseCorpus(marshalTest(t, value)); !errors.Is(err, errContract) {
				t.Fatalf("expected invalid rejection contract, got %v", err)
			}
		})
	}
}

func TestCLIRequiresOnlyDeploymentInputs(t *testing.T) {
	image := testImage(t)
	args := []string{"--kubeconfig", "private-kubeconfig", "--daemonset", "azure-npm-win", "--npm-image", image.Reference}
	input, err := parseOptions(args)
	if err != nil || input.kubeconfig != "private-kubeconfig" || input.daemonSet != "azure-npm-win" ||
		input.image != image.Reference || input.npmNamespace != "kube-system" {
		t.Fatalf("three-input CLI: %+v, %v", input, err)
	}
	input, err = parseOptions(append(args, "--npm-namespace", "custom-npm"))
	if err != nil || input.npmNamespace != "custom-npm" {
		t.Fatalf("namespace override: %+v, %v", input, err)
	}
}

func TestPolicyRewritesOnlyDeclaredIPBlocks(t *testing.T) {
	cases, err := loadCorpus()
	if err != nil {
		t.Fatal(err)
	}
	addresses := map[role]netip.Addr{
		subject: netip.MustParseAddr("10.10.0.10"), allowedPeer: netip.MustParseAddr("10.10.1.11"),
		deniedPeer: netip.MustParseAddr("10.10.1.12"),
	}
	for _, entry := range cases.Cases {
		t.Run(entry.Name, func(t *testing.T) {
			original := bytes.Clone(entry.Policy)
			actual, err := policyFor(entry, "unique-namespace", cases.PeerCIDRs, addresses)
			if err != nil {
				t.Fatal(err)
			}
			expected := decodeTest[map[string]any](t, original)
			expected["metadata"].(map[string]any)["namespace"] = "unique-namespace"
			spec := expected["spec"].(map[string]any)
			for _, direction := range []string{"ingress", "egress"} {
				rules, _ := spec[direction].([]any)
				for _, rule := range rules {
					for _, peersKey := range []string{"from", "to"} {
						peers, _ := rule.(map[string]any)[peersKey].([]any)
						for _, peer := range peers {
							block, exists := peer.(map[string]any)["ipBlock"].(map[string]any)
							if !exists {
								continue
							}
							if block["cidr"] == "192.0.2.10/32" {
								block["cidr"] = "10.10.1.11/32"
							}
							if exceptions, ok := block["except"].([]any); ok {
								if len(exceptions) != 1 || exceptions[0] != "192.0.2.11/32" {
									t.Fatalf("unexpected exception: %v", exceptions)
								}
								block["except"] = []any{"10.10.1.12/32"}
							}
						}
					}
				}
			}
			got := decodeTest[map[string]any](t, marshalTest(t, actual))
			if !reflect.DeepEqual(expected, got) {
				t.Fatalf("unexpected policy change\nwant %s\ngot  %s", marshalTest(t, expected), marshalTest(t, got))
			}
			if !bytes.Equal(entry.Policy, original) {
				t.Fatal("mutated the embedded policy")
			}
		})
	}
}

func TestCandidateReferenceIsImmutableAndPreserved(t *testing.T) {
	image := testImage(t)
	for _, reference := range []string{
		"example.invalid/azure-npm:latest", "azure-npm@" + image.Digest,
		image.Reference + "\n", strings.TrimSuffix(image.Reference, "a"),
	} {
		if _, err := parseImage(reference); err == nil {
			t.Fatalf("accepted invalid candidate reference %q", reference)
		}
	}
	for _, reference := range []string{
		"example.invalid/azure-npm@" + image.Digest,
		"example.invalid:5000/dalec/azure-npm:tag@" + image.Digest,
	} {
		actual, err := parseImage(reference)
		if err != nil || actual.Reference != reference || actual.Digest != image.Digest {
			t.Fatalf("changed immutable reference: %+v, %v", actual, err)
		}
	}
	if image.Reference != "example.invalid/dalec/synthetic/azure-npm:test@"+image.Digest {
		t.Fatal("rewrote the complete tag-plus-digest reference")
	}
}
