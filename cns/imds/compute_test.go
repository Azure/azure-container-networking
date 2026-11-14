package imds

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestComputeMetadataReturnsResourceAndIncarnation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != imdsComputePath || r.Header.Get("Metadata") != "true" {
			t.Errorf("unexpected IMDS request: %s %v", r.URL, r.Header)
		}
		_, _ = w.Write([]byte(`{"resourceId":"/subscriptions/sub/resourceGroups/rg/providers/Microsoft.Compute/virtualMachines/vm","vmId":"incarnation"}`))
	}))
	defer server.Close()
	got, err := NewClient(Endpoint(server.URL), RetryAttempts(1)).GetComputeMetadata(context.Background())
	if err != nil || got.VMID != "incarnation" || got.ResourceID == "" {
		t.Fatalf("compute identity = %+v, %v", got, err)
	}
}

func TestComputeMetadataRejectsIncompleteIdentity(t *testing.T) {
	for _, response := range []string{`{"vmId":"vm"}`, `{"resourceId":"vm"}`, `{"resourceId":12,"vmId":"vm"}`} {
		t.Run(response, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(response)) }))
			defer server.Close()
			if _, err := NewClient(Endpoint(server.URL), RetryAttempts(1)).GetComputeMetadata(context.Background()); err == nil {
				t.Fatal("incomplete guest identity accepted")
			}
		})
	}
}
