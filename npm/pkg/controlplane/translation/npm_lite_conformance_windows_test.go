package translation

import (
	"bytes"
	"encoding/json"
	"net/netip"
	"os"
	"path/filepath"
	"testing"

	npmconfig "github.com/Azure/azure-container-networking/npm/config"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type liteRejection string

const (
	liteAccepted        liteRejection = "none"
	liteUnsupportedPeer liteRejection = "unsupported-peer-selector"
	liteUnsupportedPort liteRejection = "unsupported-named-port"
)

type liteRole string

const (
	liteSubject liteRole = "subject"
	liteAllowed liteRole = "allowed-peer"
	liteDenied  liteRole = "denied-peer"
)

type liteConnectivity string

const (
	liteAllow liteConnectivity = "allow"
	liteDeny  liteConnectivity = "deny"
)

type liteProbe struct {
	From     liteRole         `json:"from"`
	To       liteRole         `json:"to"`
	Protocol corev1.Protocol  `json:"protocol"`
	Port     uint16           `json:"port"`
	Expect   liteConnectivity `json:"expect"`
}

type liteConformanceCase struct {
	Name      string                     `json:"name"`
	Policy    networkingv1.NetworkPolicy `json:"policy"`
	Rejection liteRejection              `json:"rejection"`
	Probes    []liteProbe                `json:"probes"`
}

type liteConformanceSuite struct {
	SchemaVersion     int                      `json:"schemaVersion"`
	Suite             string                   `json:"suite"`
	PodLabelKey       string                   `json:"podLabelKey"`
	PeerCIDRs         map[liteRole]string      `json:"peerCIDRs"`
	RejectionMessages map[liteRejection]string `json:"rejectionMessages"`
	Cases             []liteConformanceCase    `json:"cases"`
}

func readLiteConformanceFile(t *testing.T, name string) []byte {
	t.Helper()
	path := filepath.Join("..", "..", "..", "..", "test", "npm", "windows-lite", name)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.True(t, json.Valid(data), "%s must contain exactly one JSON value", name)
	return data
}

func TestWindowsNPMLiteConformanceConfig(t *testing.T) {
	data := readLiteConformanceFile(t, "config.json")
	var required struct {
		Toggles map[string]bool `json:"Toggles"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	require.NoError(t, decoder.Decode(&required))
	require.Equal(t, map[string]bool{
		"EnableV2NPM":        true,
		"EnableNPMLite":      true,
		"EnableHTTPDebugAPI": false,
		"EnablePprof":        false,
	}, required.Toggles)

	config := npmconfig.DefaultConfig
	require.NoError(t, json.Unmarshal(data, &config)) //nolint:musttag // NPM's existing toggle fields use their Go names as JSON keys.
	require.True(t, config.Toggles.EnableV2NPM)
	require.True(t, config.Toggles.EnableNPMLite)
	require.False(t, config.Toggles.EnableHTTPDebugAPI)
	require.False(t, config.Toggles.EnablePprof)
}

func TestWindowsNPMLiteConformanceCorpus(t *testing.T) {
	var suite liteConformanceSuite
	decoder := json.NewDecoder(bytes.NewReader(readLiteConformanceFile(t, "cases.v1.json")))
	decoder.DisallowUnknownFields()
	require.NoError(t, decoder.Decode(&suite))
	require.Equal(t, 1, suite.SchemaVersion)
	require.Equal(t, "windows-npm-lite", suite.Suite)
	require.Equal(t, "acn-npm-lite-role", suite.PodLabelKey)
	require.Equal(t, map[liteRejection]string{
		liteUnsupportedPeer: ErrUnsupportedNonCIDR.Error(),
		liteUnsupportedPort: ErrUnsupportedNamedPort.Error(),
	}, suite.RejectionMessages)
	require.Len(t, suite.PeerCIDRs, 2)
	for _, role := range []liteRole{liteAllowed, liteDenied} {
		prefix, err := netip.ParsePrefix(suite.PeerCIDRs[role])
		require.NoError(t, err)
		require.True(t, prefix.Addr().Is4())
		require.Equal(t, 32, prefix.Bits())
	}
	require.NotEqual(t, suite.PeerCIDRs[liteAllowed], suite.PeerCIDRs[liteDenied])

	names := make([]string, 0, len(suite.Cases))
	for _, testCase := range suite.Cases {
		names = append(names, testCase.Name)
		t.Run(testCase.Name, func(t *testing.T) {
			policy := testCase.Policy.DeepCopy()
			require.Equal(t, "networking.k8s.io/v1", policy.APIVersion)
			require.Equal(t, "NetworkPolicy", policy.Kind)
			require.Equal(t, testCase.Name, policy.Name)
			require.Empty(t, policy.Namespace)
			require.Equal(t, metav1.LabelSelector{
				MatchLabels: map[string]string{suite.PodLabelKey: string(liteSubject)},
			}, policy.Spec.PodSelector)
			require.Len(t, policy.Spec.PolicyTypes, 1)
			direction := policy.Spec.PolicyTypes[0]
			require.Contains(t, []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress, networkingv1.PolicyTypeEgress,
			}, direction)
			require.Len(t, testCase.Probes, 2)
			peers := make([]liteRole, 0, len(testCase.Probes))
			for _, probe := range testCase.Probes {
				require.Equal(t, corev1.ProtocolTCP, probe.Protocol)
				require.Equal(t, uint16(8080), probe.Port)
				peer := probe.From
				if direction == networkingv1.PolicyTypeIngress {
					require.Equal(t, liteSubject, probe.To)
				} else {
					require.Equal(t, liteSubject, probe.From)
					peer = probe.To
				}
				peers = append(peers, peer)
				expected := liteAllow
				if testCase.Rejection == liteAccepted && peer == liteDenied {
					expected = liteDeny
				}
				require.Equal(t, expected, probe.Expect)
			}
			require.ElementsMatch(t, []liteRole{liteAllowed, liteDenied}, peers)

			policy.Namespace = "npm-lite-conformance"
			translated, err := TranslatePolicy(policy, true)
			switch testCase.Rejection {
			case liteAccepted:
				require.NoError(t, err)
				require.NotNil(t, translated)
				require.NotEmpty(t, translated.ACLs)
				require.Empty(t, translated.RuleIPSets)
			case liteUnsupportedPeer:
				require.ErrorIs(t, err, ErrUnsupportedNonCIDR)
				require.Nil(t, translated)
			case liteUnsupportedPort:
				require.ErrorIs(t, err, ErrUnsupportedNamedPort)
				require.Nil(t, translated)
			default:
				t.Fatalf("unknown rejection %q", testCase.Rejection)
			}
		})
	}
	require.ElementsMatch(t, []string{
		"ingress-cidr", "egress-cidr",
		"ingress-cidr-except", "egress-cidr-except",
		"ingress-pod-peer-rejected", "egress-pod-peer-rejected",
		"ingress-namespace-peer-rejected", "egress-namespace-peer-rejected",
		"ingress-named-port-rejected", "egress-named-port-rejected",
	}, names)
}
