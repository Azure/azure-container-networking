package publish

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCreateV2JobSendsExpectedRequest(t *testing.T) {
	var (
		gotMethod string
		gotPath   string
		gotAuth   string
		gotType   string
		gotBody   V2JobRequest
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotType = r.Header.Get("Content-Type")
		data, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(data, &gotBody)
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"jobId":"job-123","state":"Queued","result":"Pending"}`))
	}))
	defer srv.Close()

	client, err := NewAgencyClient(AgencyConfig{BaseURL: srv.URL, Token: StaticToken("tok-abc")})
	if err != nil {
		t.Fatalf("NewAgencyClient: %v", err)
	}

	req := V2JobRequest{
		Prompt: "fix the build",
		Context: JobContext{Repository: ADORepository{
			Type: RepoTypeADO, Organization: "msazure", Project: "proj", Repository: "acn", Branch: "master",
		}},
		CustomAgent: &AgentRef{Name: "coding-agent", Type: AgentTypeCompany, PluginName: "agency-coding"},
		Plugins:     []PluginRef{{Name: "agency-coding", Type: AgentTypeCompany}},
	}
	resp, err := client.CreateV2Job(context.Background(), req)
	if err != nil {
		t.Fatalf("CreateV2Job: %v", err)
	}

	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/v2/jobs" {
		t.Errorf("path = %q, want /v2/jobs", gotPath)
	}
	if gotAuth != "Bearer tok-abc" {
		t.Errorf("auth = %q, want Bearer tok-abc", gotAuth)
	}
	if gotType != "application/json" {
		t.Errorf("content-type = %q, want application/json", gotType)
	}
	if gotBody.Context.Repository.Type != "ado" {
		t.Errorf("repository $type = %q, want ado", gotBody.Context.Repository.Type)
	}
	if gotBody.CustomAgent == nil || gotBody.CustomAgent.PluginName != "agency-coding" {
		t.Errorf("customAgent = %+v, want pluginName agency-coding", gotBody.CustomAgent)
	}
	if resp.JobID != "job-123" || resp.State != "Queued" {
		t.Errorf("resp = %+v, want jobId job-123 / state Queued", resp)
	}
}

func TestCreateV2JobPropagatesErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"detail":"bad request"}`))
	}))
	defer srv.Close()

	client, err := NewAgencyClient(AgencyConfig{BaseURL: srv.URL, Token: StaticToken("tok")})
	if err != nil {
		t.Fatalf("NewAgencyClient: %v", err)
	}
	_, err = client.CreateV2Job(context.Background(), V2JobRequest{})
	if err == nil {
		t.Fatal("expected error on 400 response")
	}
	if !strings.Contains(err.Error(), "bad request") {
		t.Errorf("error = %v, want it to include response body", err)
	}
}

func TestValidateV2JobUsesValidatePath(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`{"state":"Valid"}`))
	}))
	defer srv.Close()

	client, err := NewAgencyClient(AgencyConfig{BaseURL: srv.URL, Token: StaticToken("t")})
	if err != nil {
		t.Fatalf("NewAgencyClient: %v", err)
	}
	if _, err := client.ValidateV2Job(context.Background(), V2JobRequest{}); err != nil {
		t.Fatalf("ValidateV2Job: %v", err)
	}
	if gotPath != "/v2/jobs/validate" {
		t.Errorf("path = %q, want /v2/jobs/validate", gotPath)
	}
}

func TestNewAgencyClientRequiresToken(t *testing.T) {
	if _, err := NewAgencyClient(AgencyConfig{}); err == nil {
		t.Fatal("expected error when token provider is nil")
	}
}

func TestStaticTokenEmpty(t *testing.T) {
	if _, err := StaticToken("").Token(context.Background()); err == nil {
		t.Fatal("expected error for empty static token")
	}
	tok, err := StaticToken("x").Token(context.Background())
	if err != nil || tok != "x" {
		t.Fatalf("Token = %q, %v; want x, nil", tok, err)
	}
}

func TestGetJobRequiresID(t *testing.T) {
	client, _ := NewAgencyClient(AgencyConfig{BaseURL: "http://example", Token: StaticToken("t")})
	if _, err := client.GetJob(context.Background(), ""); err == nil {
		t.Fatal("expected error for empty job id")
	}
}
