package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/Neneka448/gogoclaw/internal/config"
)

type voyageAIEmbeddingProvider struct {
	client  *http.Client
	baseURL string
	token   string
	timeout time.Duration
}

type voyageTextEmbeddingRequest struct {
	Input           []string `json:"input"`
	Model           string   `json:"model"`
	InputType       string   `json:"input_type,omitempty"`
	Truncation      *bool    `json:"truncation,omitempty"`
	OutputDimension *int     `json:"output_dimension,omitempty"`
	OutputDType     string   `json:"output_dtype,omitempty"`
	EncodingFormat  string   `json:"encoding_format,omitempty"`
}

type voyageMultimodalEmbeddingRequest struct {
	Inputs          []MultimodalEmbeddingInput `json:"inputs"`
	Model           string                     `json:"model"`
	InputType       string                     `json:"input_type,omitempty"`
	Truncation      *bool                      `json:"truncation,omitempty"`
	OutputEncoding  string                     `json:"output_encoding,omitempty"`
	OutputDimension *int                       `json:"output_dimension,omitempty"`
}

func newVoyageAIEmbeddingProvider(providerConfig *config.ProviderConfig) (EmbeddingProvider, error) {
	baseURL, err := resolveProviderBaseURL(providerConfig)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(baseURL) == "" {
		return nil, fmt.Errorf("resolve voyageai base url: empty base url")
	}

	timeout := providerTimeout(providerConfig.Timeout)

	return &voyageAIEmbeddingProvider{
		client:  &http.Client{Timeout: timeout},
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   providerConfig.Auth.Token,
		timeout: timeout,
	}, nil
}

func (provider *voyageAIEmbeddingProvider) TextEmbeddings(params TextEmbeddingParams) (*EmbeddingResponse, error) {
	request := voyageTextEmbeddingRequest{
		Input:           cloneStrings(params.Input),
		Model:           params.Model,
		InputType:       string(params.InputType),
		Truncation:      params.Truncation,
		OutputDimension: params.OutputDimension,
		OutputDType:     params.OutputDType,
		EncodingFormat:  params.EncodingFormat,
	}

	return provider.post("embeddings", request)
}

func (provider *voyageAIEmbeddingProvider) MultimodalEmbeddings(params MultimodalEmbeddingParams) (*EmbeddingResponse, error) {
	request := voyageMultimodalEmbeddingRequest{
		Inputs:          cloneMultimodalInputs(params.Inputs),
		Model:           params.Model,
		InputType:       string(params.InputType),
		Truncation:      params.Truncation,
		OutputDimension: params.OutputDimension,
		OutputEncoding:  params.OutputEncoding,
	}

	return provider.post("multimodalembeddings", request)
}

func (provider *voyageAIEmbeddingProvider) post(endpoint string, payload any) (*EmbeddingResponse, error) {
	encodedBody, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), provider.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, joinURL(provider.baseURL, endpoint), bytes.NewReader(encodedBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+provider.token)
	req.Header.Set("content-type", "application/json")

	resp, err := provider.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("voyageai embeddings request failed: %s", strings.TrimSpace(string(body)))
	}

	var response EmbeddingResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, err
	}

	return &response, nil
}

func joinURL(baseURL string, endpoint string) string {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return strings.TrimRight(baseURL, "/") + "/" + strings.TrimLeft(endpoint, "/")
	}
	parsed.Path = path.Join(parsed.Path, endpoint)
	return parsed.String()
}

func cloneStrings(values []string) []string {
	cloned := make([]string, len(values))
	copy(cloned, values)
	return cloned
}

func cloneMultimodalInputs(inputs []MultimodalEmbeddingInput) []MultimodalEmbeddingInput {
	cloned := make([]MultimodalEmbeddingInput, 0, len(inputs))
	for _, input := range inputs {
		copiedContent := make([]MultimodalContentPart, len(input.Content))
		copy(copiedContent, input.Content)
		cloned = append(cloned, MultimodalEmbeddingInput{Content: copiedContent})
	}
	return cloned
}
