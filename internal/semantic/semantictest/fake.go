// A stand-in for OpenAI's /embeddings endpoint, for tests at every layer
// (the semantic package, the API, and — as a port of the same idea —
// web/e2e/fake-openai.ts for Playwright). Vectors are a bag of words
// hashed into buckets: texts sharing words come out similar, which is all
// "semantic" needs to mean to test a pipeline.
package semantictest

import (
	"encoding/json"
	"hash/fnv"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
)

var wordRE = regexp.MustCompile(`[a-z0-9]+`)

// Vector is the fake embedding of text at the requested width.
func Vector(text string, dimensions int) []float32 {
	v := make([]float32, dimensions)
	for _, w := range wordRE.FindAllString(strings.ToLower(text), -1) {
		h := fnv.New32a()
		h.Write([]byte(w))
		v[h.Sum32()%uint32(dimensions)] += 1
	}
	return v
}

// Server serves POST /v1/embeddings, requires "Bearer test-key", counts
// calls, and answers the first failFirst calls with a 429.
func Server(t *testing.T, failFirst int) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embeddings" || r.Method != "POST" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":{"message":"bad key"}}`))
			return
		}
		calls.Add(1)
		if int(attempts.Add(1)) <= failFirst {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":{"message":"slow down"}}`))
			return
		}
		var req struct {
			Input      []string `json:"input"`
			Dimensions int      `json:"dimensions"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Dimensions <= 0 {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		type datum struct {
			Index     int       `json:"index"`
			Embedding []float32 `json:"embedding"`
		}
		out := struct {
			Data []datum `json:"data"`
		}{}
		// Reverse order on purpose: clients must honour `index`.
		for i := len(req.Input) - 1; i >= 0; i-- {
			out.Data = append(out.Data, datum{Index: i, Embedding: Vector(req.Input[i], req.Dimensions)})
		}
		_ = json.NewEncoder(w).Encode(out)
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}
