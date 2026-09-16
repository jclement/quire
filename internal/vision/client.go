// Vision: asking an OpenAI-compatible /chat/completions endpoint to describe
// an image, so a pasted screenshot arrives carrying real alt text instead of
// its filename. Entirely opt-in — nothing here runs unless QUIRE_VISION_MODEL
// is set, because it sends image bytes to a third party.
//
// The credential and endpoint are the ones semantic search already uses; only
// the model is separate, and deliberately so: QUIRE_OPENAI_BASE_URL is
// documented as any compatible server, and plenty of them (Ollama, LiteLLM)
// serve embeddings without serving vision. Keying this off the embeddings key
// alone would turn "semantic search works" into "every paste hits a 404".
package vision

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// DefaultBaseURL is OpenAI's, matching the embeddings client's default; any
// compatible server works via QUIRE_OPENAI_BASE_URL.
const DefaultBaseURL = "https://api.openai.com/v1"

// MaxBytes is the largest image worth sending. Attachments may be 50MB, but
// a screenshot is rarely over a few hundred KB, and the APIs reject very
// large payloads anyway — better to skip describing than to spend a slow
// request discovering that.
const MaxBytes = 10 << 20

// maxTokens caps the reply. Alt text is a sentence or two; without a cap a
// model will happily narrate a screenshot for a paragraph.
const maxTokens = 160

// maxAltLen truncates a reply that ignored the instruction to be brief.
const maxAltLen = 400

// describePrompt is the product, really: the difference between "image.png"
// and something worth finding six months later. It asks for the meaning of a
// screenshot (which app, which numbers, which error) rather than a literal
// inventory of pixels, and forbids the preamble models like to open with.
const describePrompt = `Write alt text for this image, to be stored in a personal notes app.

One or two plain sentences, no preamble, no markdown, no quotes around it.
Start directly with the description — never "This image shows".
If it is a screenshot of software, say which app or page it is and what the
important labels, numbers, or error text say.
If readable text carries the meaning, include the parts that matter.`

// Client calls an OpenAI-compatible chat-completions endpoint.
type Client struct {
	BaseURL string
	APIKey  string
	Model   string
	HTTP    *http.Client
}

// NewClient applies defaults for anything empty. Callers construct this only
// when a model is configured; an empty Model is not a usable client.
func NewClient(baseURL, apiKey, model string) *Client {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		APIKey:  apiKey,
		Model:   model,
		// Generous: a vision call is slower than an embedding, and the
		// caller bounds it more tightly with a context deadline.
		HTTP: &http.Client{Timeout: 60 * time.Second},
	}
}

// Describable reports whether an extension is a raster format the vision
// APIs accept. Deliberately narrower than the app's isImageExt: .svg is
// markup rather than pixels, and .heic/.avif are not accepted, so sending
// either just buys a slow error on a path where the user is waiting.
func Describable(ext string) bool {
	switch strings.ToLower(ext) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp":
		return true
	}
	return false
}

// MIMEType is the media type for a Describable extension, "" otherwise.
func MIMEType(ext string) string {
	switch strings.ToLower(ext) {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	}
	return ""
}

// RetryableError marks a failure worth trying again later (rate limit,
// server trouble, network) as opposed to one that will never succeed.
type RetryableError struct{ Err error }

func (e RetryableError) Error() string { return e.Err.Error() }
func (e RetryableError) Unwrap() error { return e.Err }

type imageURL struct {
	URL string `json:"url"`
}

type contentPart struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *imageURL `json:"image_url,omitempty"`
}

type chatMessage struct {
	Role    string        `json:"role"`
	Content []contentPart `json:"content"`
}

type chatRequest struct {
	Model     string        `json:"model"`
	Messages  []chatMessage `json:"messages"`
	MaxTokens int           `json:"max_tokens,omitempty"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// Describe returns one or two sentences of alt text for an image, already
// safe to place inside a markdown image reference. An empty string with a
// nil error means the model had nothing to say, which callers treat the same
// as a failure: keep the filename.
func (c *Client) Describe(ctx context.Context, data []byte, mimeType string) (string, error) {
	if len(data) == 0 {
		return "", fmt.Errorf("vision: empty image")
	}
	if len(data) > MaxBytes {
		return "", fmt.Errorf("vision: image is %dMB, over the %dMB limit", len(data)>>20, MaxBytes>>20)
	}
	if mimeType == "" {
		return "", fmt.Errorf("vision: unknown image type")
	}

	// A data URL rather than a hosted URL: the vault is private and usually
	// not reachable from the internet, so there is no address we could hand
	// the API that it could actually fetch.
	dataURL := "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(data)
	body, err := json.Marshal(chatRequest{
		Model:     c.Model,
		MaxTokens: maxTokens,
		Messages: []chatMessage{{
			Role: "user",
			Content: []contentPart{
				{Type: "text", Text: describePrompt},
				{Type: "image_url", ImageURL: &imageURL{URL: dataURL}},
			},
		}},
	})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	res, err := c.HTTP.Do(req)
	if err != nil {
		return "", RetryableError{fmt.Errorf("vision request: %w", err)}
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return "", RetryableError{err}
	}
	if res.StatusCode == http.StatusTooManyRequests || res.StatusCode >= 500 {
		return "", RetryableError{fmt.Errorf("vision: HTTP %d: %s", res.StatusCode, firstLine(raw))}
	}
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("vision: HTTP %d: %s", res.StatusCode, firstLine(raw))
	}
	var parsed chatResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", fmt.Errorf("vision: bad JSON: %w", err)
	}
	if parsed.Error != nil {
		return "", fmt.Errorf("vision: %s", parsed.Error.Message)
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("vision: no choices in response")
	}
	return SanitizeAlt(parsed.Choices[0].Message.Content), nil
}

// SanitizeAlt makes a model's reply safe to sit inside `![...](path)`.
// Square brackets would close the alt early and a newline would break the
// reference outright, so both are neutralised rather than escaped — alt text
// is prose, and a backslash in it reads worse than a parenthesis.
func SanitizeAlt(s string) string {
	s = strings.TrimSpace(s)
	// Models like to wrap a one-line answer in quotes despite being asked not to.
	if len(s) >= 2 && strings.HasPrefix(s, `"`) && strings.HasSuffix(s, `"`) {
		s = strings.TrimSpace(s[1 : len(s)-1])
	}
	s = strings.NewReplacer(
		"[", "(",
		"]", ")",
		"\r", " ",
		"\n", " ",
		"\t", " ",
	).Replace(s)
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > maxAltLen {
		cut := s[:maxAltLen]
		// Back off to the last space so the cut does not land mid-word.
		// Space is ASCII, so cutting there is also rune-safe; ToValidUTF8
		// covers the case of a single word longer than the whole budget.
		if i := strings.LastIndexByte(cut, ' '); i > maxAltLen/2 {
			cut = cut[:i]
		}
		s = strings.TrimSpace(strings.ToValidUTF8(cut, "")) + "…"
	}
	return s
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
