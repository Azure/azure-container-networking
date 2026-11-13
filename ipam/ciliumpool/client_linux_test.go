package ciliumpool

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	jsonpatch "gopkg.in/evanphx/json-patch.v4"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

type testAPI struct {
	mu                sync.Mutex
	node              *unstructured.Unstructured
	conflicts         int
	loseResponse      bool
	invalidConflict   bool
	replaceOnConflict bool
}

func testAllocator(t *testing.T) (*Allocator, *testAPI) {
	t.Helper()
	api := &testAPI{node: testNode(t, newState())}
	api.node.SetOwnerReferences([]metav1.OwnerReference{{APIVersion: "v1", Kind: "Node", Name: testNodeName, UID: testNodeUID}})
	annotations := api.node.GetAnnotations()
	annotations[vmAnnotation] = "vm"
	annotations[consumerAnnotation] = PluginName
	api.node.SetAnnotations(annotations)
	if err := unstructured.SetNestedField(api.node.Object, int64(2), specField, ipamField, "pre-allocate"); err != nil {
		t.Fatal(err)
	}
	if err := unstructured.SetNestedField(api.node.Object, "instance", specField, "instance-id"); err != nil {
		t.Fatal(err)
	}
	if err := unstructured.SetNestedSlice(api.node.Object, []any{map[string]any{
		"id": testNIC, "ip": testPrimary, macField: testMAC, gatewayField: "10.0.0.1", subnetField: map[string]any{"id": subnetField, cidrField: testSubnet}, "addresses": []any{
			map[string]any{"ip": testIP, stateField: stateSucceeded},
			map[string]any{"ip": testOtherIP, stateField: stateSucceeded},
		},
	}}, statusField, testNetwork, "interfaces"); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		api.mu.Lock()
		defer api.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		wantPath := "/apis/cilium.io/v2/ciliumnodes/node"
		if r.Method == http.MethodPatch {
			wantPath += "/status"
		}
		if r.URL.Path != wantPath {
			t.Errorf("unexpected resource endpoint: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodPatch {
			old, err := api.node.MarshalJSON()
			if err != nil {
				t.Error(err)
				return
			}
			data, err := io.ReadAll(r.Body)
			if err != nil {
				t.Error(err)
				return
			}
			patch, err := jsonpatch.DecodePatch(data)
			if err != nil {
				t.Error(err)
				return
			}
			var ops []patchOperation
			if err := json.Unmarshal(data, &ops); err != nil {
				t.Error(err)
				return
			}
			for _, op := range ops {
				if op.Op != "test" && !strings.HasPrefix(op.Path, "/status") {
					t.Errorf("consumer attempted non-status write: %+v", op)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
			}
			updated, applyErr := patch.Apply(old)
			if api.conflicts > 0 {
				api.conflicts--
				rv, _ := strconv.Atoi(api.node.GetResourceVersion())
				api.node.SetResourceVersion(strconv.Itoa(rv + 1))
				if api.replaceOnConflict {
					api.node.SetUID("replacement")
				}
				_ = unstructured.SetNestedField(api.node.Object, "operator changed status", statusField, "diagnostic")
				applyErr = errTestConflict
			}
			if applyErr != nil {
				code, reason := http.StatusConflict, metav1.StatusReasonConflict
				if api.invalidConflict {
					code, reason = http.StatusUnprocessableEntity, metav1.StatusReasonInvalid
				}
				w.WriteHeader(code)
				_ = json.NewEncoder(w).Encode(metav1.Status{
					TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Status"},
					Status:   "Failure", Reason: reason, Code: int32(code),
				})
				return
			}
			if err := api.node.UnmarshalJSON(updated); err != nil {
				t.Error(err)
				return
			}
			rv, _ := strconv.Atoi(api.node.GetResourceVersion())
			api.node.SetResourceVersion(strconv.Itoa(rv + 1))
			if api.loseResponse {
				api.loseResponse = false
				http.Error(w, "response lost after commit", http.StatusInternalServerError)
				return
			}
		} else if r.Method != http.MethodGet {
			t.Errorf("unexpected HTTP method: %s", r.Method)
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		_ = json.NewEncoder(w).Encode(api.node.Object)
	}))
	t.Cleanup(server.Close)
	client, err := dynamic.NewForConfig(&rest.Config{Host: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	allocator := &Allocator{
		Nodes:    client.Resource(schema.GroupVersionResource{Group: "cilium.io", Version: "v2", Resource: "ciliumnodes"}),
		NodeName: testNodeName, Binding: Binding{NodeUID: testNodeUID, VMID: "vm", Resource: testNIC, Subnet: testSubnet, NodeName: testNodeName, InstanceID: "instance", Primary: testPrimary, MAC: testMAC},
		BootID: testBoot, PreAllocate: 2, Supply: map[string]bool{testIP: true, testOtherIP: true},
	}
	return allocator, api
}

func TestAPIClaimsRetryConflictAndPreserveOperatorStatus(t *testing.T) {
	a, api := testAllocator(t)
	api.conflicts = 1
	owner := Owner{Network: testNetwork, Sandbox: testSandbox, Interface: testIfName}
	ip, err := a.Add(context.Background(), owner)
	if err != nil || ip != testIP {
		t.Fatalf("Add = %q, %v", ip, err)
	}
	api.mu.Lock()
	defer api.mu.Unlock()
	state, err := readState(api.node, a.Binding)
	if err != nil || state.Reservations[ip].Owner != owner {
		t.Fatalf("reservation not committed: %+v, %v", state, err)
	}
	diagnostic, _, _ := unstructured.NestedString(api.node.Object, statusField, "diagnostic")
	if diagnostic != "operator changed status" {
		t.Fatal("operator status lost")
	}
}

func TestAPILostResponseReplaysWithoutSecondReservation(t *testing.T) {
	a, api := testAllocator(t)
	api.loseResponse = true
	owner := Owner{Network: testNetwork, Sandbox: testSandbox, Interface: testIfName}
	if _, err := a.Add(context.Background(), owner); err == nil {
		t.Fatal("lost response reported success")
	}
	ip, err := a.Add(context.Background(), owner)
	if err != nil || ip != testIP {
		t.Fatalf("replay = %q, %v", ip, err)
	}
	api.mu.Lock()
	defer api.mu.Unlock()
	state, err := readState(api.node, a.Binding)
	if err != nil || len(state.Reservations) != 1 {
		t.Fatalf("duplicate allocation: %+v, %v", state, err)
	}
}

func TestAPIConcurrentClaimsAreUnique(t *testing.T) {
	a, _ := testAllocator(t)
	results := make(chan string, 2)
	var wg sync.WaitGroup
	for _, sandbox := range []string{"a", "b"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ip, err := a.Add(context.Background(), Owner{Network: testNetwork, Sandbox: sandbox, Interface: testIfName})
			if err != nil {
				t.Error(err)
				return
			}
			results <- ip
		}()
	}
	wg.Wait()
	close(results)
	seen := map[string]bool{}
	for ip := range results {
		if seen[ip] {
			t.Fatalf("duplicate IP %s", ip)
		}
		seen[ip] = true
	}
	if len(seen) != 2 {
		t.Fatalf("only %d unique claims", len(seen))
	}
}
