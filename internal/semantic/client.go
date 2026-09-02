// Semantic search: notes and queries turned into embedding vectors by an
// OpenAI-compatible /embeddings endpoint, compared by cosine similarity.
// Entirely opt-in — nothing here runs unless QUIRE_OPENAI_API_KEY is set,
// because it sends note text to a third party. This file is the HTTP
// client; embedder.go is the pipeline around it.
package semantic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// DefaultBaseURL is OpenAI's; any compatible server (Ollama, LiteLLM, …)
// works via QUIRE_OPENAI_BASE_URL.
const DefaultBaseURL = "https://api.openai.com/v1"

// DefaultModel is cheap and good; Dimensions below shrinks its 1536-wide
// vectors with Matryoshka truncation, which the model is trained for.
const DefaultModel = "text-embedding-3-small"

// Dimensions per vector. 512 floats keeps 20k chunks around 40MB in memory
// and loses very little ranking quality against the full width.
const Dimensions = 512

// maxBatch inputs per request — well under OpenAI's 2048, and small enough
// that one failed request doesn't waste much.
const maxBatch = 32

// Client calls an OpenAI-compatible embeddings endpoint.
type Client struct {
	BaseURL string
	APIKey  string
	Model   string
	HTTP    *http.Client
}

// NewClient applies defaults for anything empty.
func NewClient(baseURL, apiKey, model string) *Client {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	if model == "" {
		model = DefaultModel
	}
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		APIKey:  apiKey,
		Model:   model,
		HTTP:    &http.Client{Timeout: 60 * time.Second},
	}
}

type embedRequest struct {
	Model      string   `json:"model"`
	Input      []string `json:"input"`
	Dimensions int      `json:"dimensions,omitempty"`
}

type embedResponse struct {
	Data []struct {
		Index     int       `json:"index"`
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// RetryableError marks a failure worth trying again later (rate limit,
// server trouble, network) as opposed to one that will never succeed.
type RetryableError struct{ Err error }

func (e RetryableError) Error() string { return e.Err.Error() }
func (e RetryableError) Unwrap() error { return e.Err }

// Embed returns one unit-length vector per input, in order.
func (c *Client) Embed(ctx context.Context, inputs []string) ([][]float32, error) {
	if len(inputs) == 0 {
		return nil, nil
	}
	body, err := json.Marshal(embedRequest{Model: c.Model, Input: inputs, Dimensions: Dimensions})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", c.BaseURL+"/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, RetryableError{fmt.Errorf("embeddings request: %w", err)}
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 64<<20))
	if err != nil {
		return nil, RetryableError{err}
	}
	if res.StatusCode == http.StatusTooManyRequests || res.StatusCode >= 500 {
		return nil, RetryableError{fmt.Errorf("embeddings: HTTP %d: %s", res.StatusCode, firstLine(raw))}
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("embeddings: HTTP %d: %s", res.StatusCode, firstLine(raw))
	}
	var parsed embedResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("embeddings: bad JSON: %w", err)
	}
	if parsed.Error != nil {
		return nil, fmt.Errorf("embeddings: %s", parsed.Error.Message)
	}
	if len(parsed.Data) != len(inputs) {
		return nil, fmt.Errorf("embeddings: got %d vectors for %d inputs", len(parsed.Data), len(inputs))
	}
	out := make([][]float32, len(inputs))
	for _, d := range parsed.Data {
		if d.Index < 0 || d.Index >= len(inputs) {
			return nil, fmt.Errorf("embeddings: vector index %d out of range", d.Index)
		}
		out[d.Index] = normalize(d.Embedding)
	}
	return out, nil
}

func firstLine(b []byte) string {
	s := strings.TrimSpace(string(b))
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 200 {
		s = s[:200] + "…"
	}
	return s
}
