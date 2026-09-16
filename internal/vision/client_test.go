package vision

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// onePixelPNG is a real PNG, so the bytes travelling to the fake endpoint are
// an image rather than a placeholder.
var onePixelPNG = []byte{
	0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a,
	0x00, 0x00, 0x00, 0x0d, 'I', 'H', 'D', 'R',
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4, 0x89,
}

func TestDescribeSendsADataURLAndReturnsTheReply(t *testing.T) {
	var got chatRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path = %q, want /chat/completions", r.URL.Path)
		}
		if auth := r.Header.Get("Authorization"); auth != "Bearer test-key" {
			t.Errorf("Authorization = %q", auth)
		}
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatalf("request body: %v", err)
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"A Grafana panel with p99 latency spiking to 2.4s at 14:00."}}]}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "test-key", "test-vision")
	out, err := c.Describe(context.Background(), onePixelPNG, "image/png")
	if err != nil {
		t.Fatalf("Describe: %v", err)
	}
	if want := "A Grafana panel with p99 latency spiking to 2.4s at 14:00."; out != want {
		t.Errorf("got %q, want %q", out, want)
	}

	if got.Model != "test-vision" {
		t.Errorf("model = %q", got.Model)
	}
	if len(got.Messages) != 1 || len(got.Messages[0].Content) != 2 {
		t.Fatalf("unexpected message shape: %+v", got.Messages)
	}
	img := got.Messages[0].Content[1]
	if img.Type != "image_url" || img.ImageURL == nil {
		t.Fatalf("second part is not an image: %+v", img)
	}
	if !strings.HasPrefix(img.ImageURL.URL, "data:image/png;base64,") {
		t.Errorf("image URL = %q, want a png data URL", img.ImageURL.URL)
	}
}

func TestDescribeClassifiesFailures(t *testing.T) {
	tests := []struct {
		name      string
		status    int
		body      string
		retryable bool
	}{
		{"rate limited", http.StatusTooManyRequests, `{"error":{"message":"slow down"}}`, true},
		{"server error", http.StatusBadGateway, "upstream is unhappy", true},
		{"bad request", http.StatusBadRequest, `{"error":{"message":"no such model"}}`, false},
		{"model refuses images", http.StatusNotFound, "not found", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()

			c := NewClient(srv.URL, "k", "m")
			_, err := c.Describe(context.Background(), onePixelPNG, "image/png")
			if err == nil {
				t.Fatal("want an error")
			}
			_, isRetryable := err.(RetryableError)
			if isRetryable != tt.retryable {
				t.Errorf("retryable = %v, want %v (err %v)", isRetryable, tt.retryable, err)
			}
		})
	}
}

func TestDescribeRejectsWhatItCannotSend(t *testing.T) {
	// No server: none of these may reach the network.
	c := NewClient("http://127.0.0.1:1", "k", "m")
	tests := []struct {
		name string
		data []byte
		mime string
	}{
		{"empty", nil, "image/png"},
		{"unknown type", onePixelPNG, ""},
		{"too large", make([]byte, MaxBytes+1), "image/png"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := c.Describe(context.Background(), tt.data, tt.mime); err == nil {
				t.Fatal("want an error")
			}
		})
	}
}

func TestSanitizeAltKeepsTheReferenceIntact(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain", "A login screen.", "A login screen."},
		{"brackets would close the alt early", "A chart of [revenue] by month", "A chart of (revenue) by month"},
		{"newlines break the reference", "Line one\nline two", "Line one line two"},
		{"collapses whitespace", "  too    much   space  ", "too much space"},
		{"strips the quotes models add", `"A dashboard."`, "A dashboard."},
		{"empty stays empty", "   ", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SanitizeAlt(tt.in); got != tt.want {
				t.Errorf("SanitizeAlt(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestSanitizeAltTruncatesRunaway(t *testing.T) {
	got := SanitizeAlt(strings.Repeat("word ", 300))
	if len(got) > maxAltLen+8 {
		t.Errorf("length %d, want <= %d", len(got), maxAltLen+8)
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("want an ellipsis on a truncated description, got %q", got[len(got)-10:])
	}
}

func TestDescribableIsNarrowerThanTheAppsImageSet(t *testing.T) {
	for _, ext := range []string{".png", ".jpg", ".jpeg", ".gif", ".webp", ".PNG"} {
		if !Describable(ext) {
			t.Errorf("Describable(%q) = false, want true", ext)
		}
		if MIMEType(ext) == "" {
			t.Errorf("MIMEType(%q) = \"\"", ext)
		}
	}
	// The app stores these as images, but no vision API takes them.
	for _, ext := range []string{".svg", ".heic", ".avif", ".pdf", ""} {
		if Describable(ext) {
			t.Errorf("Describable(%q) = true, want false", ext)
		}
	}
}
