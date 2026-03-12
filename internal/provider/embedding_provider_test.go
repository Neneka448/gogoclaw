package provider

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Neneka448/gogoclaw/internal/config"
)

func TestEmbeddingVectorUnmarshalJSONSupportsNumericAndBase64Payloads(t *testing.T) {
	t.Run("numeric", func(t *testing.T) {
		var vector EmbeddingVector
		if err := json.Unmarshal([]byte(`[1,2.5,3]`), &vector); err != nil {
			t.Fatalf("json.Unmarshal() error = %v", err)
		}
		if len(vector.Values) != 3 || vector.Values[1] != 2.5 {
			t.Fatalf("vector.Values = %#v, want [1 2.5 3]", vector.Values)
		}
	})

	t.Run("base64", func(t *testing.T) {
		var vector EmbeddingVector
		if err := json.Unmarshal([]byte(`"YWJj"`), &vector); err != nil {
			t.Fatalf("json.Unmarshal() error = %v", err)
		}
		if vector.Encoded != "YWJj" {
			t.Fatalf("vector.Encoded = %q, want YWJj", vector.Encoded)
		}
	})
}

func TestNewEmbeddingProviderRejectsUnsupportedProvider(t *testing.T) {
	_, err := NewEmbeddingProvider(&config.ProviderConfig{Name: "unknown"})
	if err == nil {
		t.Fatal("NewEmbeddingProvider() error = nil, want unsupported provider error")
	}
}

func TestVoyageAITextEmbeddings(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/embeddings" {
			t.Fatalf("path = %q, want /embeddings", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer voyage-token" {
			t.Fatalf("Authorization = %q, want Bearer voyage-token", got)
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("io.ReadAll() error = %v", err)
		}

		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("json.Unmarshal() error = %v", err)
		}
		if payload["model"] != "voyage-4" {
			t.Fatalf("payload model = %#v, want voyage-4", payload["model"])
		}
		input, ok := payload["input"].([]any)
		if !ok || len(input) != 2 {
			t.Fatalf("payload input = %#v, want two strings", payload["input"])
		}

		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"object":"list","data":[{"object":"embedding","embedding":[0.1,0.2],"index":0}],"model":"voyage-4","usage":{"total_tokens":8}}`))
	}))
	defer server.Close()

	embeddingProvider, err := NewEmbeddingProvider(&config.ProviderConfig{
		Name:    "voyageai",
		BaseURL: server.URL,
		Auth:    config.AuthConfig{Token: "voyage-token"},
	})
	if err != nil {
		t.Fatalf("NewEmbeddingProvider() error = %v", err)
	}

	response, err := embeddingProvider.TextEmbeddings(TextEmbeddingParams{
		Model: "voyage-4",
		Input: []string{"Sample text 1", "Sample text 2"},
	})
	if err != nil {
		t.Fatalf("TextEmbeddings() error = %v", err)
	}
	if response.Model != "voyage-4" {
		t.Fatalf("response.Model = %q, want voyage-4", response.Model)
	}
	if len(response.Data) != 1 || len(response.Data[0].Embedding.Values) != 2 {
		t.Fatalf("response.Data = %#v, want one embedding with two values", response.Data)
	}
}

func TestVoyageAIMultimodalEmbeddings(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/multimodalembeddings" {
			t.Fatalf("path = %q, want /multimodalembeddings", r.URL.Path)
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("io.ReadAll() error = %v", err)
		}

		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("json.Unmarshal() error = %v", err)
		}
		inputs, ok := payload["inputs"].([]any)
		if !ok || len(inputs) != 1 {
			t.Fatalf("payload inputs = %#v, want one input", payload["inputs"])
		}
		if payload["output_dimension"] != float64(1024) {
			t.Fatalf("payload output_dimension = %#v, want 1024", payload["output_dimension"])
		}

		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"object":"list","data":[{"object":"embedding","embedding":"ZW1iZWRkaW5n","index":0}],"model":"voyage-multimodal-3.5","usage":{"text_tokens":5,"image_pixels":2000000,"total_tokens":12}}`))
	}))
	defer server.Close()

	embeddingProvider, err := NewEmbeddingProvider(&config.ProviderConfig{
		Name:    "voyageai",
		BaseURL: server.URL,
		Auth:    config.AuthConfig{Token: "voyage-token"},
	})
	if err != nil {
		t.Fatalf("NewEmbeddingProvider() error = %v", err)
	}

	response, err := embeddingProvider.MultimodalEmbeddings(MultimodalEmbeddingParams{
		Model:           "voyage-multimodal-3.5",
		OutputDimension: intPtr(1024),
		Inputs: []MultimodalEmbeddingInput{{
			Content: []MultimodalContentPart{{
				Type: MultimodalContentTypeText,
				Text: "This is a banana.",
			}, {
				Type:     MultimodalContentTypeImageURL,
				ImageURL: "https://example.com/banana.jpg",
			}},
		}},
	})
	if err != nil {
		t.Fatalf("MultimodalEmbeddings() error = %v", err)
	}
	if response.Model != "voyage-multimodal-3.5" {
		t.Fatalf("response.Model = %q, want voyage-multimodal-3.5", response.Model)
	}
	if len(response.Data) != 1 || response.Data[0].Embedding.Encoded != "ZW1iZWRkaW5n" {
		t.Fatalf("response.Data = %#v, want one base64 embedding", response.Data)
	}
	if response.Usage.ImagePixels != 2000000 {
		t.Fatalf("response.Usage.ImagePixels = %d, want 2000000", response.Usage.ImagePixels)
	}
}

func intPtr(value int) *int {
	return &value
}
