//go:build linux

//nolint:goconst // Keep wire fixtures independent of executor constants.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/netip"
	"reflect"
	"slices"
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

func testImage(t *testing.T) (identity imageIdentity, manifestBytes, configBytes []byte) {
	t.Helper()
	config := []byte(`{"os":"windows","architecture":"amd64","os.version":"10.0.20348.4400"}`)
	manifest := marshalTest(t, map[string]any{
		"schemaVersion": 2, "mediaType": "application/vnd.oci.image.manifest.v1+json",
		"config": map[string]any{"digest": "sha256:" + digest(config), "size": len(config)},
		"layers": []any{map[string]any{"digest": "sha256:" + strings.Repeat("a", 64), "size": 1}},
	})
	reference := "example.invalid/dalec/synthetic/azure-npm:test@sha256:" + digest(manifest)
	image, err := parseImage(reference, "10.0.20348.4400", manifest, config)
	if err != nil {
		t.Fatal(err)
	}
	return image, manifest, config
}

func TestCorpusAndRunnerPins(t *testing.T) {
	cases, err := loadCorpus()
	if err != nil {
		t.Fatal(err)
	}
	if len(cases.Cases) != 10 || cases.Suite != suiteName {
		t.Fatalf("unexpected corpus: %+v", cases)
	}
	for i, entry := range cases.Cases {
		if entry.Name != requiredCases[i] {
			t.Fatalf("case %d: %s", i, entry.Name)
		}
	}
	config, err := runfiles.ReadFile("config.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = parseConfig(config); err != nil {
		t.Fatal(err)
	}

	entries, err := runfiles.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".go") && !strings.HasSuffix(entry.Name(), "_test.go") {
			names = append(names, entry.Name())
		}
	}
	slices.Sort(names)
	sum := sha256.New()
	for _, name := range names {
		data, readErr := runfiles.ReadFile(name)
		if readErr != nil {
			t.Fatal(readErr)
		}
		_, _ = sum.Write(append([]byte(name), 0))
		_, _ = sum.Write(append(data, 0))
	}
	got, err := runnerHash()
	if err != nil || got != hex.EncodeToString(sum.Sum(nil)) {
		t.Fatalf("source-bundle framing: %s, %v", got, err)
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

func TestCandidateManifestChain(t *testing.T) {
	image, manifest, config := testImage(t)
	tests := []struct {
		name string
		edit func(*imageIdentity, *[]byte, *[]byte)
	}{
		{"mutable reference", func(i *imageIdentity, _, _ *[]byte) { i.Reference = "example.invalid/azure-npm:latest" }},
		{"manifest bytes", func(_ *imageIdentity, m, _ *[]byte) { *m = append(*m, '\n') }},
		{"config bytes", func(_ *imageIdentity, _, c *[]byte) { *c = append(*c, '\n') }},
		{"wrong OS revision", func(i *imageIdentity, _, _ *[]byte) { i.OSVersion = "10.0.20348.4401" }},
		{"unsupported Windows build", func(i *imageIdentity, _, _ *[]byte) { i.OSVersion = "10.0.17763.5000" }},
		{"index", func(i *imageIdentity, m, _ *[]byte) {
			object := decodeTest[map[string]any](t, *m)
			object["mediaType"] = "application/vnd.oci.image.index.v1+json"
			object["manifests"] = []any{map[string]string{"digest": image.Digest}}
			*m = marshalTest(t, object)
			i.Reference = "example.invalid/azure-npm@sha256:" + digest(*m)
		}},
		{"config size", func(i *imageIdentity, m, _ *[]byte) {
			object := decodeTest[map[string]any](t, *m)
			object["config"].(map[string]any)["size"] = len(config) + 1
			*m = marshalTest(t, object)
			i.Reference = "example.invalid/azure-npm@sha256:" + digest(*m)
		}},
		{"Linux child", func(i *imageIdentity, m, c *[]byte) {
			*c = bytes.Replace(*c, []byte(`"windows"`), []byte(`"linux"`), 1)
			object := decodeTest[map[string]any](t, *m)
			object["config"] = map[string]any{"digest": "sha256:" + digest(*c), "size": len(*c)}
			*m = marshalTest(t, object)
			i.Reference = "example.invalid/azure-npm@sha256:" + digest(*m)
		}},
		{"arm64 child", func(i *imageIdentity, m, c *[]byte) {
			*c = bytes.Replace(*c, []byte(`"amd64"`), []byte(`"arm64"`), 1)
			object := decodeTest[map[string]any](t, *m)
			object["config"] = map[string]any{"digest": "sha256:" + digest(*c), "size": len(*c)}
			*m = marshalTest(t, object)
			i.Reference = "example.invalid/azure-npm@sha256:" + digest(*m)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate, rawManifest, rawConfig := image, bytes.Clone(manifest), bytes.Clone(config)
			test.edit(&candidate, &rawManifest, &rawConfig)
			if _, err := parseImage(candidate.Reference, candidate.OSVersion, rawManifest, rawConfig); err == nil {
				t.Fatal("accepted invalid candidate metadata")
			}
		})
	}
	if image.Reference != "example.invalid/dalec/synthetic/azure-npm:test@"+image.Digest {
		t.Fatal("rewrote the complete tag-plus-digest reference")
	}
}
