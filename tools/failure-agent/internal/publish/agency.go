package publish

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
)

const (
	// AgencyProdBaseURL is the production 1ES Agency Job API root.
	AgencyProdBaseURL = "https://copilotswe.app.prod.gitops.startclean.microsoft.com/api/agency"
	// AgencyPPEBaseURL is the pre-production Agency Job API root.
	AgencyPPEBaseURL = "https://copilotswe.app.ppe.gitops.startclean.microsoft.com/api/agency"
	// AgencyProdScope is the AAD token scope for the production Agency resource.
	AgencyProdScope = "api://81bbac67-d541-4a6d-a48b-b1c0f9a57888/.default"
	// AgencyPPEScope is the AAD token scope for the PPE Agency resource.
	AgencyPPEScope = "api://99d8628b-dd58-4123-b33c-7e82ddf37571/.default"

	// AgentTypeCompany references an agent or plugin published at the company level.
	AgentTypeCompany = "Company"
	// RepoTypeADO is the polymorphic discriminator for an Azure DevOps repository.
	RepoTypeADO = "ado"

	agencyMaxResponseBytes = 1 << 20
)

// ADORepository targets an Azure DevOps-hosted repository for a v2 job.
type ADORepository struct {
	Type         string `json:"$type"`
	Organization string `json:"organization"`
	Project      string `json:"project"`
	Repository   string `json:"repository"`
	Branch       string `json:"branch,omitempty"`
}

// JobContext carries the repository the job operates on.
type JobContext struct {
	Repository ADORepository `json:"repository"`
}

// AgentRef selects the custom agent that drives a job.
type AgentRef struct {
	Name       string `json:"name"`
	Type       string `json:"type,omitempty"`
	PluginName string `json:"pluginName,omitempty"`
}

// PluginRef loads an additional plugin alongside the agent.
type PluginRef struct {
	Name string `json:"name"`
	Type string `json:"type,omitempty"`
}

// V2JobRequest is the Agency Job API v2 GenericJobRequest body.
type V2JobRequest struct {
	Prompt      string            `json:"prompt,omitempty"`
	Context     JobContext        `json:"context"`
	CustomAgent *AgentRef         `json:"customAgent,omitempty"`
	Plugins     []PluginRef       `json:"plugins,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// JobResponse is the subset of the Agency job payload the agent consumes.
type JobResponse struct {
	JobID       string `json:"jobId"`
	OperationID string `json:"operationId,omitempty"`
	State       string `json:"state,omitempty"`
	Result      string `json:"result,omitempty"`
	Reason      string `json:"reason,omitempty"`
}

// TokenProvider issues an AAD bearer token for the Agency resource.
type TokenProvider interface {
	Token(ctx context.Context) (string, error)
}

// StaticToken is a TokenProvider that returns a pre-acquired token, used for the
// FAA_AGENCY_TOKEN override and in tests.
type StaticToken string

// Token returns the static token, erroring only when it is empty.
func (s StaticToken) Token(context.Context) (string, error) {
	if s == "" {
		return "", errors.New("static agency token is empty")
	}
	return string(s), nil
}

// credentialToken adapts an azcore.TokenCredential to the TokenProvider interface.
type credentialToken struct {
	cred  azcore.TokenCredential
	scope string
}

// Token acquires a token for the configured scope from the wrapped credential.
func (c credentialToken) Token(ctx context.Context) (string, error) {
	tk, err := c.cred.GetToken(ctx, policy.TokenRequestOptions{Scopes: []string{c.scope}})
	if err != nil {
		return "", fmt.Errorf("acquiring agency token: %w", err)
	}
	return tk.Token, nil
}

// NewDefaultTokenProvider returns a TokenProvider backed by DefaultAzureCredential
// (managed identity in-pipeline, Azure CLI locally) for the given scope.
func NewDefaultTokenProvider(scope string) (TokenProvider, error) {
	if scope == "" {
		return nil, errors.New("an agency token scope is required")
	}
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, fmt.Errorf("building default azure credential: %w", err)
	}
	return credentialToken{cred: cred, scope: scope}, nil
}

// AgencyConfig configures an AgencyClient.
type AgencyConfig struct {
	BaseURL    string // defaults to AgencyProdBaseURL
	Token      TokenProvider
	HTTPClient *http.Client
}

// AgencyClient calls the 1ES Agency Job API.
type AgencyClient struct {
	httpClient *http.Client
	baseURL    string
	token      TokenProvider
}

// NewAgencyClient validates cfg and returns a ready client.
func NewAgencyClient(cfg AgencyConfig) (*AgencyClient, error) {
	if cfg.Token == nil {
		return nil, errors.New("an agency token provider is required")
	}
	base := cfg.BaseURL
	if base == "" {
		base = AgencyProdBaseURL
	}
	hc := cfg.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: requestTimeout}
	}
	return &AgencyClient{
		httpClient: hc,
		baseURL:    strings.TrimRight(base, "/"),
		token:      cfg.Token,
	}, nil
}

// CreateV2Job dispatches a v2 coding job via POST /v2/jobs.
func (c *AgencyClient) CreateV2Job(ctx context.Context, req V2JobRequest) (JobResponse, error) {
	return c.postJob(ctx, "/v2/jobs", req)
}

// GetJob fetches the status of a previously created job.
func (c *AgencyClient) GetJob(ctx context.Context, jobID string) (JobResponse, error) {
	if jobID == "" {
		return JobResponse{}, errors.New("a job id is required")
	}
	req, err := c.newRequest(ctx, http.MethodGet, "/jobs/"+jobID, nil)
	if err != nil {
		return JobResponse{}, err
	}
	return c.do(req)
}

func (c *AgencyClient) postJob(ctx context.Context, path string, body V2JobRequest) (JobResponse, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return JobResponse{}, fmt.Errorf("encoding job request: %w", err)
	}
	req, err := c.newRequest(ctx, http.MethodPost, path, payload)
	if err != nil {
		return JobResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	return c.do(req)
}

func (c *AgencyClient) newRequest(ctx context.Context, method, path string, body []byte) (*http.Request, error) {
	tok, err := c.token.Token(ctx)
	if err != nil {
		return nil, err
	}
	var rdr io.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, rdr)
	if err != nil {
		return nil, fmt.Errorf("building agency request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Accept", "application/json")
	return req, nil
}

func (c *AgencyClient) do(req *http.Request) (JobResponse, error) {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return JobResponse{}, fmt.Errorf("calling agency api: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, agencyMaxResponseBytes))
	if err != nil {
		return JobResponse{}, fmt.Errorf("reading agency response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return JobResponse{}, fmt.Errorf("agency api %s: %s", resp.Status, strings.TrimSpace(string(data)))
	}

	var jr JobResponse
	if err := json.Unmarshal(data, &jr); err != nil {
		return JobResponse{}, fmt.Errorf("decoding agency response: %w", err)
	}
	return jr, nil
}
