//go:build linux

package main

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"regexp"
)

const (
	suiteName           = "windows-npm-lite"
	roleLabel           = "acn-npm-lite-role"
	tcpProtocol         = "TCP"
	windowsOS           = "windows"
	windowsArchitecture = "amd64"
	windowsBuild        = "10.0.20348"
)

var errContract = errors.New("windows-lite: invalid input contract")

//go:embed config.json cases.v1.json
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

func loadCorpus() (corpus, error) {
	config, err := runfiles.ReadFile("config.json")
	if err != nil {
		return corpus{}, fmt.Errorf("read config: %w", err)
	}
	if configErr := validateConfig(config); configErr != nil {
		return corpus{}, configErr
	}
	data, err := runfiles.ReadFile("cases.v1.json")
	if err != nil {
		return corpus{}, fmt.Errorf("read cases: %w", err)
	}
	return parseCorpus(data)
}

func parseCorpus(data []byte) (corpus, error) {
	var out corpus
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&out); err != nil {
		return out, fmt.Errorf("decoding corpus: %w", err)
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return out, fmt.Errorf("%w: trailing corpus content", errContract)
	}
	if out.SchemaVersion != 1 || out.Suite != suiteName || out.PodLabelKey != roleLabel || len(out.Cases) != len(requiredCases) {
		return out, fmt.Errorf("%w: unsupported corpus", errContract)
	}
	for i, entry := range out.Cases {
		if entry.Name != requiredCases[i] || len(entry.Probes) != 2 {
			return out, fmt.Errorf("%w: unexpected corpus case at index %d", errContract, i)
		}
		if entry.Rejection != noRejection && entry.Rejection != unsupportedSelector && entry.Rejection != unsupportedPort {
			return out, fmt.Errorf("%w: unknown rejection for %s", errContract, entry.Name)
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

var imagePattern = regexp.MustCompile(`^[a-z0-9.-]+(?::[0-9]+)?/[a-z0-9]+(?:[._/-][a-z0-9]+)*(?::[A-Za-z0-9_][A-Za-z0-9_.-]{0,127})?@sha256:([a-f0-9]{64})$`)

type imageIdentity struct {
	Reference string
	Digest    string
}

func parseImage(reference string) (imageIdentity, error) {
	match := imagePattern.FindStringSubmatch(reference)
	if len(match) != 2 {
		return imageIdentity{}, fmt.Errorf("%w: expected a complete immutable child reference", errContract)
	}
	return imageIdentity{reference, "sha256:" + match[1]}, nil
}
