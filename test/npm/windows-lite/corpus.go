//go:build linux

package main

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"regexp"
	"slices"
	"strings"
)

const (
	suiteName           = "windows-npm-lite"
	corpusSourceVersion = "9d3089f42f7e06c30041eb0388f10a9c071cbd3d"
	configHash          = "dad5356aa853bc80be93771717fac7db8a40b2270e6f38a154bd18ceff254163"
	casesHash           = "aecd2d2a5423d8a6a50cf1156a8df6370915ee4f839f7473d97ea8d87932de90"
	roleLabel           = "acn-npm-lite-role"
	tcpProtocol         = "TCP"
	windowsOS           = "windows"
	windowsArchitecture = "amd64"
	windowsBuild        = "10.0.20348"
)

var errContract = errors.New("windows-lite: invalid input contract")

//go:embed config.json cases.v1.json *.go
var runfiles embed.FS

type role string

const (
	subject     role = "subject"
	allowedPeer role = "allowed-peer"
	deniedPeer  role = "denied-peer"
)

type verdict string

const (
	allow verdict = "allow"
	deny  verdict = "deny"
)

type rejection string

const (
	noRejection         rejection = "none"
	unsupportedSelector rejection = "unsupported-peer-selector"
	unsupportedPort     rejection = "unsupported-named-port"
)

var requiredCases = [...]string{
	"ingress-cidr", "egress-cidr", "ingress-cidr-except", "egress-cidr-except",
	"ingress-pod-peer-rejected", "egress-pod-peer-rejected",
	"ingress-namespace-peer-rejected", "egress-namespace-peer-rejected",
	"ingress-named-port-rejected", "egress-named-port-rejected",
}

type probe struct {
	From     role    `json:"from"`
	To       role    `json:"to"`
	Protocol string  `json:"protocol"`
	Port     uint16  `json:"port"`
	Expect   verdict `json:"expect"`
}

type conformanceCase struct {
	Name      string          `json:"name"`
	Policy    json.RawMessage `json:"policy"`
	Rejection rejection       `json:"rejection"`
	Probes    []probe         `json:"probes"`
}

type corpus struct {
	SchemaVersion     int                   `json:"schemaVersion"`
	Suite             string                `json:"suite"`
	PodLabelKey       string                `json:"podLabelKey"`
	PeerCIDRs         map[role]netip.Prefix `json:"peerCIDRs"`
	RejectionMessages map[rejection]string  `json:"rejectionMessages"`
	Cases             []conformanceCase     `json:"cases"`
}

type toggles struct {
	EnableV2NPM        bool `json:"EnableV2NPM"`
	EnableNPMLite      bool `json:"EnableNPMLite"`
	EnableHTTPDebugAPI bool `json:"EnableHTTPDebugAPI"`
	EnablePprof        bool `json:"EnablePprof"`
}

func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func loadCorpus() (corpus, error) {
	var out corpus
	for name, expected := range map[string]string{"config.json": configHash, "cases.v1.json": casesHash} {
		data, err := runfiles.ReadFile(name)
		if err != nil {
			return out, fmt.Errorf("read corpus %s: %w", name, err)
		}
		if digest(data) != expected {
			return out, fmt.Errorf("%w: corpus content pin mismatch for %s", errContract, name)
		}
	}
	data, err := runfiles.ReadFile("cases.v1.json")
	if err != nil {
		return out, fmt.Errorf("read cases: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&out); err != nil {
		return out, fmt.Errorf("decoding corpus: %w", err)
	}
	if out.SchemaVersion != 1 || out.Suite != suiteName || out.PodLabelKey != roleLabel || len(out.Cases) != len(requiredCases) {
		return out, fmt.Errorf("%w: unsupported corpus", errContract)
	}
	for i, entry := range out.Cases {
		if entry.Name != requiredCases[i] || len(entry.Probes) != 2 {
			return out, fmt.Errorf("%w: unexpected corpus case at index %d", errContract, i)
		}
		if entry.Rejection != noRejection && out.RejectionMessages[entry.Rejection] == "" {
			return out, fmt.Errorf("%w: unknown rejection for %s", errContract, entry.Name)
		}
		for _, p := range entry.Probes {
			if p.Protocol != tcpProtocol || p.Port != 8080 || (p.Expect != allow && p.Expect != deny) ||
				!validRole(p.From) || !validRole(p.To) {
				return out, fmt.Errorf("%w: unsupported probe in %s", errContract, entry.Name)
			}
		}
	}
	return out, nil
}

func validRole(value role) bool {
	return value == subject || value == allowedPeer || value == deniedPeer
}

func runnerHash() (string, error) {
	entries, err := runfiles.ReadDir(".")
	if err != nil {
		return "", fmt.Errorf("list runner source: %w", err)
	}
	var names []string
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".go") && !strings.HasSuffix(entry.Name(), "_test.go") {
			names = append(names, entry.Name())
		}
	}
	slices.Sort(names)
	var contents bytes.Buffer
	for _, name := range names {
		data, err := runfiles.ReadFile(name)
		if err != nil {
			return "", fmt.Errorf("read runner source %s: %w", name, err)
		}
		contents.WriteString(name)
		contents.WriteByte(0)
		contents.Write(data)
		contents.WriteByte(0)
	}
	return digest(contents.Bytes()), nil
}

type policyDocument struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Metadata   struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace,omitempty"`
	} `json:"metadata"`
	Spec struct {
		PodSelector json.RawMessage `json:"podSelector"`
		PolicyTypes []string        `json:"policyTypes"`
		Ingress     []policyRule    `json:"ingress,omitempty"`
		Egress      []policyRule    `json:"egress,omitempty"`
	} `json:"spec"`
}

type policyRule struct {
	From  []policyPeer    `json:"from,omitempty"`
	To    []policyPeer    `json:"to,omitempty"`
	Ports json.RawMessage `json:"ports"`
}

type policyPeer struct {
	IPBlock *struct {
		CIDR   netip.Prefix   `json:"cidr"`
		Except []netip.Prefix `json:"except,omitempty"`
	} `json:"ipBlock,omitempty"`
	PodSelector       json.RawMessage `json:"podSelector,omitempty"`
	NamespaceSelector json.RawMessage `json:"namespaceSelector,omitempty"`
}

func policyFor(entry conformanceCase, namespace string, sentinels map[role]netip.Prefix, addresses map[role]netip.Addr) (policyDocument, error) {
	var policy policyDocument
	if err := json.Unmarshal(entry.Policy, &policy); err != nil {
		return policy, fmt.Errorf("decode policy: %w", err)
	}
	replacements := make(map[netip.Prefix]netip.Prefix, len(sentinels))
	for key, sentinel := range sentinels {
		address := addresses[key]
		if !address.Is4() || address.IsUnspecified() {
			return policy, fmt.Errorf("%w: missing IPv4 endpoint for %s", errEvidence, key)
		}
		replacements[sentinel] = netip.PrefixFrom(address, 32)
	}
	policy.Metadata.Namespace = namespace
	for _, rules := range [][]policyRule{policy.Spec.Ingress, policy.Spec.Egress} {
		for _, rule := range rules {
			for _, peers := range [][]policyPeer{rule.From, rule.To} {
				for _, peer := range peers {
					if peer.IPBlock == nil {
						continue
					}
					if replacement, ok := replacements[peer.IPBlock.CIDR]; ok {
						peer.IPBlock.CIDR = replacement
					}
					for i, prefix := range peer.IPBlock.Except {
						if replacement, ok := replacements[prefix]; ok {
							peer.IPBlock.Except[i] = replacement
						}
					}
				}
			}
		}
	}
	return policy, nil
}

var (
	imagePattern     = regexp.MustCompile(`^[a-z0-9.-]+(?::[0-9]+)?/[a-z0-9]+(?:[._/-][a-z0-9]+)*(?::[A-Za-z0-9_][A-Za-z0-9_.-]{0,127})?@sha256:([a-f0-9]{64})$`)
	sourcePattern    = regexp.MustCompile(`^[a-f0-9]{40}$`)
	osVersionPattern = regexp.MustCompile(`^10\.0\.20348\.\d+$`)
)

type imageIdentity struct {
	Reference    string
	Digest       string
	ConfigDigest string
	OSVersion    string
}

func parseImage(reference, osVersion string, manifestBytes, configBytes []byte) (imageIdentity, error) {
	var out imageIdentity
	match := imagePattern.FindStringSubmatch(reference)
	if len(match) != 2 || !osVersionPattern.MatchString(osVersion) {
		return out, fmt.Errorf("%w: expected a complete immutable Windows2022 child reference and OS version", errContract)
	}
	if digest(manifestBytes) != match[1] {
		return out, fmt.Errorf("%w: candidate manifest digest mismatch", errContract)
	}
	var manifest struct {
		SchemaVersion int    `json:"schemaVersion"`
		MediaType     string `json:"mediaType"`
		Config        struct {
			Digest string `json:"digest"`
			Size   int    `json:"size"`
		} `json:"config"`
		Layers    []json.RawMessage `json:"layers"`
		Manifests []json.RawMessage `json:"manifests"`
	}
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return out, fmt.Errorf("decoding image manifest: %w", err)
	}
	if manifest.SchemaVersion != 2 || len(manifest.Manifests) != 0 || len(manifest.Layers) == 0 ||
		(manifest.MediaType != "application/vnd.oci.image.manifest.v1+json" &&
			manifest.MediaType != "application/vnd.docker.distribution.manifest.v2+json") {
		return out, fmt.Errorf("%w: candidate must be a container-image child manifest, not an index", errContract)
	}
	if manifest.Config.Digest != "sha256:"+digest(configBytes) || manifest.Config.Size != len(configBytes) {
		return out, fmt.Errorf("%w: candidate config digest or size mismatch", errContract)
	}
	var config struct {
		OS           string `json:"os"`
		Architecture string `json:"architecture"`
		OSVersion    string `json:"os.version"`
	}
	if err := json.Unmarshal(configBytes, &config); err != nil {
		return out, fmt.Errorf("decoding image config: %w", err)
	}
	if config.OS != windowsOS || config.Architecture != windowsArchitecture || config.OSVersion != osVersion {
		return out, fmt.Errorf("%w: candidate config is not the requested windows/amd64 OS version", errContract)
	}
	return imageIdentity{reference, "sha256:" + match[1], manifest.Config.Digest, config.OSVersion}, nil
}
