package classify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/azure"
	"github.com/openai/openai-go/v3/shared"
)

// AzureClient is a ChatCompleter backed by an Azure OpenAI deployment.
type AzureClient struct {
	client     openai.Client
	deployment string
}

// NewAzureClient builds a ChatCompleter for the given Azure OpenAI endpoint and
// deployment using an API key for authentication.
func NewAzureClient(endpoint, deployment, apiVersion, apiKey string) (*AzureClient, error) {
	if endpoint == "" {
		return nil, errors.New("azure openai endpoint is required")
	}
	if deployment == "" {
		return nil, errors.New("azure openai deployment is required")
	}
	if apiVersion == "" {
		return nil, errors.New("azure openai api version is required")
	}
	if apiKey == "" {
		return nil, errors.New("azure openai api key is required")
	}

	client := openai.NewClient(
		azure.WithEndpoint(endpoint, apiVersion),
		azure.WithAPIKey(apiKey),
	)
	return &AzureClient{client: client, deployment: deployment}, nil
}

// Complete sends the system and user prompts and returns the model's response,
// constraining output to schema when one is provided.
func (a *AzureClient) Complete(ctx context.Context, system, user string, schema *Schema) (string, error) {
	params := openai.ChatCompletionNewParams{
		Model: shared.ChatModel(a.deployment),
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(system),
			openai.UserMessage(user),
		},
	}

	if schema != nil {
		var def map[string]any
		if err := json.Unmarshal(schema.Definition, &def); err != nil {
			return "", fmt.Errorf("decoding response schema: %w", err)
		}
		params.ResponseFormat = openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONSchema: &shared.ResponseFormatJSONSchemaParam{
				JSONSchema: shared.ResponseFormatJSONSchemaJSONSchemaParam{
					Name:   schema.Name,
					Schema: def,
					Strict: openai.Bool(true),
				},
			},
		}
	}

	resp, err := a.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return "", fmt.Errorf("azure openai chat completion: %w", err)
	}
	if len(resp.Choices) == 0 {
		return "", errors.New("azure openai returned no choices")
	}
	return resp.Choices[0].Message.Content, nil
}
