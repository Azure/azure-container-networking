package middlewares

import (
	"context"
	"testing"

	"github.com/Azure/azure-container-networking/crd/multitenancy/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// Shared test fixtures; constants keep the repeated literals from tripping goconst.
const (
	testNode       = "node1"
	testSubnetName = "mySubnet"
	canonMAC01     = "aa:bb:cc:dd:ee:01"
	canonMAC02     = "aa:bb:cc:dd:ee:02"
	canonMAC03     = "aa:bb:cc:dd:ee:03"
)

// GetMTPNCResourceSliceInfoByMAC must node-scope, compute capacity from DRA state, and tolerate a
// not-ready MTPNC (empty Spec network/subnet) without erroring — empty values flow
// through as-is.
func TestGetMTPNCResourceSliceInfoByMAC(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}

	mtpncs := []v1alpha1.MultitenantPodNetworkConfig{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "ready-dra", Namespace: "ns"},
			Spec: v1alpha1.MultitenantPodNetworkConfigSpec{
				NetworkID:        "net-a",
				SubnetGUID:       "guid-a",
				SubnetResourceID: "/subscriptions/x/subnets/subA",
				ResourceClaims:   []string{"claim-a"}, // scheduled with DRA
			},
			Status: v1alpha1.MultitenantPodNetworkConfigStatus{
				NodeName:       testNode,
				InterfaceInfos: []v1alpha1.InterfaceInfo{{MacAddress: "aa:bb:cc:dd:ee:0a"}},
			},
		},
		{
			// Not-ready: has a MAC but empty Spec network/subnet and no DRA claims.
			ObjectMeta: metav1.ObjectMeta{Name: "partial", Namespace: "ns"},
			Status: v1alpha1.MultitenantPodNetworkConfigStatus{
				NodeName:       testNode,
				InterfaceInfos: []v1alpha1.InterfaceInfo{{MacAddress: "aa:bb:cc:dd:ee:0b"}},
			},
		},
		{
			// On a different node — must be excluded.
			ObjectMeta: metav1.ObjectMeta{Name: "othernode", Namespace: "ns"},
			Spec:       v1alpha1.MultitenantPodNetworkConfigSpec{NetworkID: "net-c"},
			Status: v1alpha1.MultitenantPodNetworkConfigStatus{
				NodeName:       "node2",
				InterfaceInfos: []v1alpha1.InterfaceInfo{{MacAddress: "aa:bb:cc:dd:ee:0c"}},
			},
		},
	}

	cli := fake.NewClientBuilder().WithScheme(scheme).
		WithLists(&v1alpha1.MultitenantPodNetworkConfigList{Items: mtpncs}).Build()
	mw := &K8sSWIFTv2Middleware{Cli: cli, NodeName: testNode}

	got, err := mw.GetMTPNCResourceSliceInfoByMAC(context.Background())
	if err != nil {
		t.Fatalf("GetMTPNCResourceSliceInfoByMAC: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d entries, want 2: %+v", len(got), got)
	}

	ready := got["aa:bb:cc:dd:ee:0a"]
	if ready == nil {
		t.Fatal("ready MTPNC MAC missing")
	}
	if ready.NetworkID != "net-a" || ready.SubnetGUID != "guid-a" || ready.SubnetName != "subA" || ready.Capacity != 1 {
		t.Errorf("ready entry = %+v, want net-a/guid-a/subA/cap 1", *ready)
	}

	// The not-ready MTPNC must be included with empty network/subnet and zero capacity,
	// and must not have caused an error.
	partial := got["aa:bb:cc:dd:ee:0b"]
	if partial == nil {
		t.Fatal("partial MTPNC MAC missing")
	}
	if partial.NetworkID != "" || partial.SubnetGUID != "" || partial.SubnetName != "" || partial.Capacity != 0 {
		t.Errorf("partial entry = %+v, want all-empty/cap 0", *partial)
	}

	if _, ok := got["aa:bb:cc:dd:ee:0c"]; ok {
		t.Error("MTPNC on another node should be excluded")
	}
}

func TestSubnetNameFromResourceID(t *testing.T) {
	tests := []struct {
		name       string
		resourceID string
		want       string
	}{
		{name: "trailing subnet name", resourceID: "/subscriptions/x/subnets/mySubnet", want: testSubnetName},
		{name: "no slashes returns input", resourceID: testSubnetName, want: testSubnetName},
		{name: "empty returns empty", resourceID: "", want: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := subnetNameFromResourceID(tc.resourceID); got != tc.want {
				t.Errorf("subnetNameFromResourceID(%q) = %q, want %q", tc.resourceID, got, tc.want)
			}
		})
	}
}

func TestCanonicalMAC(t *testing.T) {
	tests := []struct {
		name   string
		raw    string
		want   string
		wantOK bool
	}{
		{name: "uppercase colon form", raw: "AA:BB:CC:DD:EE:01", want: canonMAC01, wantOK: true},
		{name: "hyphen form", raw: "aa-bb-cc-dd-ee-02", want: canonMAC02, wantOK: true},
		{name: "already canonical", raw: canonMAC03, want: canonMAC03, wantOK: true},
		{name: "empty is invalid", raw: "", want: "", wantOK: false},
		{name: "garbage is invalid", raw: "not-a-mac", want: "", wantOK: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := canonicalMAC(tc.raw)
			if ok != tc.wantOK || got != tc.want {
				t.Errorf("canonicalMAC(%q) = (%q, %v), want (%q, %v)", tc.raw, got, ok, tc.want, tc.wantOK)
			}
		})
	}
}

func TestMTPNCMACs(t *testing.T) {
	tests := []struct {
		name  string
		mtpnc v1alpha1.MultitenantPodNetworkConfig
		want  []string
	}{
		{
			name: "reads MACs from InterfaceInfos",
			mtpnc: v1alpha1.MultitenantPodNetworkConfig{
				Status: v1alpha1.MultitenantPodNetworkConfigStatus{
					InterfaceInfos: []v1alpha1.InterfaceInfo{
						{MacAddress: canonMAC01},
						{MacAddress: canonMAC02},
					},
				},
			},
			want: []string{canonMAC01, canonMAC02},
		},
		{
			name: "skips empty MACs in InterfaceInfos",
			mtpnc: v1alpha1.MultitenantPodNetworkConfig{
				Status: v1alpha1.MultitenantPodNetworkConfigStatus{
					InterfaceInfos: []v1alpha1.InterfaceInfo{
						{MacAddress: ""},
						{MacAddress: canonMAC03},
					},
				},
			},
			want: []string{canonMAC03},
		},
		{
			name: "falls back to deprecated Status.MacAddress",
			mtpnc: v1alpha1.MultitenantPodNetworkConfig{
				Status: v1alpha1.MultitenantPodNetworkConfigStatus{MacAddress: "aa:bb:cc:dd:ee:04"},
			},
			want: []string{"aa:bb:cc:dd:ee:04"},
		},
		{
			name:  "no MACs returns nil",
			mtpnc: v1alpha1.MultitenantPodNetworkConfig{},
			want:  nil,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := mtpncMACs(&tc.mtpnc)
			if len(got) != len(tc.want) {
				t.Fatalf("mtpncMACs() = %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("mtpncMACs()[%d] = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}
