package provider

import (
	"encoding/json"
	"fmt"

	"github.com/Neneka448/gogoclaw/internal/config"
)

type EmbeddingInputType string

const (
	EmbeddingInputTypeQuery    EmbeddingInputType = "query"
	EmbeddingInputTypeDocument EmbeddingInputType = "document"
)

type MultimodalContentType string

const (
	MultimodalContentTypeText        MultimodalContentType = "text"
	MultimodalContentTypeImageURL    MultimodalContentType = "image_url"
	MultimodalContentTypeImageBase64 MultimodalContentType = "image_base64"
	MultimodalContentTypeVideoURL    MultimodalContentType = "video_url"
	MultimodalContentTypeVideoBase64 MultimodalContentType = "video_base64"
)

type TextEmbeddingParams struct {
	Model           string
	Input           []string
	InputType       EmbeddingInputType
	Truncation      *bool
	OutputDimension *int
	OutputDType     string
	EncodingFormat  string
}

type MultimodalEmbeddingParams struct {
	Model           string
	Inputs          []MultimodalEmbeddingInput
	InputType       EmbeddingInputType
	Truncation      *bool
	OutputEncoding  string
	OutputDimension *int
}

type MultimodalEmbeddingInput struct {
	Content []MultimodalContentPart `json:"content"`
}

type MultimodalContentPart struct {
	Type        MultimodalContentType `json:"type"`
	Text        string                `json:"text,omitempty"`
	ImageURL    string                `json:"image_url,omitempty"`
	ImageBase64 string                `json:"image_base64,omitempty"`
	VideoURL    string                `json:"video_url,omitempty"`
	VideoBase64 string                `json:"video_base64,omitempty"`
}

type EmbeddingResponse struct {
	Object string              `json:"object"`
	Data   []EmbeddingData     `json:"data"`
	Model  string              `json:"model"`
	Usage  EmbeddingUsageStats `json:"usage"`
}

type EmbeddingData struct {
	Object    string          `json:"object"`
	Embedding EmbeddingVector `json:"embedding"`
	Index     int             `json:"index"`
}

type EmbeddingVector struct {
	Values  []float64
	Encoded string
	Raw     json.RawMessage
}

func (vector *EmbeddingVector) UnmarshalJSON(data []byte) error {
	vector.Raw = append(vector.Raw[:0], data...)

	var encoded string
	if err := json.Unmarshal(data, &encoded); err == nil {
		vector.Encoded = encoded
		vector.Values = nil
		return nil
	}

	var values []float64
	if err := json.Unmarshal(data, &values); err == nil {
		vector.Values = values
		vector.Encoded = ""
		return nil
	}

	return fmt.Errorf("unsupported embedding format: %s", string(data))
}

type EmbeddingUsageStats struct {
	TotalTokens int `json:"total_tokens,omitempty"`
	TextTokens  int `json:"text_tokens,omitempty"`
	ImagePixels int `json:"image_pixels,omitempty"`
	VideoPixels int `json:"video_pixels,omitempty"`
}

type EmbeddingProvider interface {
	TextEmbeddings(params TextEmbeddingParams) (*EmbeddingResponse, error)
	MultimodalEmbeddings(params MultimodalEmbeddingParams) (*EmbeddingResponse, error)
}

func NewEmbeddingProvider(providerConfig *config.ProviderConfig) (EmbeddingProvider, error) {
	if providerConfig == nil {
		return nil, fmt.Errorf("embedding provider config is nil")
	}

	switch providerConfig.Name {
	case "voyageai":
		return newVoyageAIEmbeddingProvider(providerConfig)
	default:
		return nil, fmt.Errorf("unsupported embedding provider: %s", providerConfig.Name)
	}
}
