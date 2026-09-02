// The embedding pipeline: keep every document's chunk vectors in index.db
// (table `embeddings`, created here, rebuilt for free after a reindex) and
// in memory, refresh them as the index reports changes, and answer
// similarity queries by brute-force dot product over the cache. Brute force
// is the right call at this scale: 20k chunks × 512 floats is 40MB and a
// query scans it in a few milliseconds; an ANN index would be complexity
// spent on a problem a personal vault doesn't have.
package semantic

import (
	"context"
	"database/sql"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/jclement/quire/internal/index"
	"github.com/jclement/quire/internal/vault"
)

const schema = `
CREATE TABLE IF NOT EXISTS embeddings (
	path        TEXT NOT NULL,
	chunk       INTEGER NOT NULL,
	heading     TEXT NOT NULL DEFAULT '',
	fingerprint TEXT NOT NULL,
	vector      BLOB NOT NULL,
	PRIMARY KEY (path, chunk)
);`

// Hit is one document matched by a query, with its best passage.
type Hit struct {
	Path    string
	Heading string
	Score   float32
}

// Status is what Settings shows.
type Status struct {
	Enabled   bool   `json:"enabled"`
	Model     string `json:"model"`
	Documents int    `json:"documents"`
	Pending   int    `json:"pending"`
	// LastError is the most recent embedding failure, "" when healthy.
	LastError string `json:"last_error"`
}

type entry struct {
	path    string
	chunk   int
	heading string
	vec     []float32
}

// Embedder keeps documents embedded and answers similarity queries.
type Embedder struct {
	db     *sql.DB
	vault  *vault.Vault
	client *Client

	mu      sync.RWMutex
	entries []entry
	byPath  map[string][]int // path → indexes into entries

	queueMu   sync.Mutex
	queued    map[string]bool
	queue     chan string
	lastError string
}

// retryDelay between attempts when the endpoint is rate-limiting or down.
const retryDelay = 15 * time.Second

// Start opens the table, loads the cache, queues every document whose
// embeddings are missing or stale, and runs the worker until ctx ends.
func Start(ctx context.Context, db *sql.DB, v *vault.Vault, client *Client) (*Embedder, error) {
	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("creating embeddings table: %w", err)
	}
	e := &Embedder{
		db: db, vault: v, client: client,
		byPath: map[string][]int{},
		queued: map[string]bool{},
		queue:  make(chan string, 4096),
	}
	if err := e.load(); err != nil {
		return nil, err
	}
	if err := e.sweep(); err != nil {
		return nil, err
	}
	go e.run(ctx)
	return e, nil
}

// Notify is the index hook: an upsert queues a refresh, a delete drops rows.
func (e *Embedder) Notify(ev index.Event) {
	if ev.Action == "delete" {
		e.remove(ev.Path)
		return
	}
	e.enqueue(ev.Path)
}

// Status reports counts for the Settings page.
func (e *Embedder) Status() Status {
	e.mu.RLock()
	docs := len(e.byPath)
	e.mu.RUnlock()
	e.queueMu.Lock()
	pending := len(e.queued)
	lastError := e.lastError
	e.queueMu.Unlock()
	return Status{Enabled: true, Model: e.client.Model, Documents: docs, Pending: pending, LastError: lastError}
}

// Search embeds the query and returns the closest documents, best passage
// per document, highest score first.
func (e *Embedder) Search(ctx context.Context, query string, limit int) ([]Hit, error) {
	vecs, err := e.client.Embed(ctx, []string{query})
	if err != nil {
		return nil, err
	}
	return e.nearest(vecs[0], "", limit), nil
}

// minRelatedScore is the cosine similarity below which two documents are
// not shown as related. Without a floor the rail always fills its slots,
// and a note that says only "test" is "related" to whatever is least
// unlike it. 0.4 sits well above the 0.1–0.3 that unrelated notes score
// with text-embedding-3 while keeping genuinely overlapping ones.
const minRelatedScore = 0.4

// Related ranks other documents by similarity to this one — no API call,
// the document's own vectors are already here. Nil when it has none yet,
// and only documents above minRelatedScore are returned.
func (e *Embedder) Related(path string, limit int) []Hit {
	e.mu.RLock()
	idx := e.byPath[path]
	if len(idx) == 0 {
		e.mu.RUnlock()
		return nil
	}
	// The document's centroid, renormalised.
	sum := make([]float32, len(e.entries[idx[0]].vec))
	for _, i := range idx {
		for j, x := range e.entries[i].vec {
			sum[j] += x
		}
	}
	e.mu.RUnlock()
	hits := e.nearest(normalize(sum), path, limit)
	kept := hits[:0]
	for _, h := range hits {
		if h.Score >= minRelatedScore {
			kept = append(kept, h)
		}
	}
	return kept
}

func (e *Embedder) nearest(q []float32, exclude string, limit int) []Hit {
	if limit <= 0 {
		limit = 20
	}
	best := map[string]Hit{}
	e.mu.RLock()
	for _, en := range e.entries {
		if en.path == exclude || len(en.vec) != len(q) {
			continue
		}
		score := dot(q, en.vec)
		if cur, ok := best[en.path]; !ok || score > cur.Score {
			best[en.path] = Hit{Path: en.path, Heading: en.heading, Score: score}
		}
	}
	e.mu.RUnlock()
	hits := make([]Hit, 0, len(best))
	for _, h := range best {
		hits = append(hits, h)
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].Score != hits[j].Score {
			return hits[i].Score > hits[j].Score
		}
		return hits[i].Path < hits[j].Path
	})
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return hits
}

// ---- pipeline ----

func (e *Embedder) enqueue(path string) {
	e.queueMu.Lock()
	already := e.queued[path]
	e.queued[path] = true
	e.queueMu.Unlock()
	if already {
		return
	}
	select {
	case e.queue <- path:
	default:
		// Queue full (a huge import): the startup sweep will catch it next
		// boot, and the watcher's own retry of a changed file re-queues it.
		e.queueMu.Lock()
		delete(e.queued, path)
		e.queueMu.Unlock()
		slog.Warn("embedding queue full; skipping", "path", path)
	}
}

func (e *Embedder) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case path := <-e.queue:
			// Drain what else is waiting so one request carries several docs.
			paths := []string{path}
		drain:
			for len(paths) < maxBatch {
				select {
				case p := <-e.queue:
					paths = append(paths, p)
				default:
					break drain
				}
			}
			e.process(ctx, paths)
		}
	}
}

// process embeds the stale chunks of a batch of documents. A retryable
// failure re-queues the whole batch after a pause; a permanent one is
// logged once per batch and those documents stay un-embedded (findable by
// full-text search still, and retried on their next change).
func (e *Embedder) process(ctx context.Context, paths []string) {
	type pending struct {
		path  string
		chunk int
		c     Chunk
		fp    string
	}
	var work []pending
	for _, p := range paths {
		f, err := e.vault.Read(p)
		if err != nil {
			e.remove(p) // gone between the event and now
			e.finish(p)
			continue
		}
		title := titleOf(p, f.Raw)
		chunks := Chunks(title, f.Raw)
		existing := e.fingerprints(p)
		for i, c := range chunks {
			fp := Fingerprint(e.client.Model, c.Text)
			if existing[i] == fp {
				continue
			}
			work = append(work, pending{p, i, c, fp})
		}
		// Chunks beyond the new count are stale (the note got shorter).
		if _, err := e.db.Exec("DELETE FROM embeddings WHERE path = ? AND chunk >= ?", p, len(chunks)); err != nil {
			slog.Warn("trimming embeddings", "path", p, "err", err)
		}
	}
	for start := 0; start < len(work); start += maxBatch {
		end := min(start+maxBatch, len(work))
		batch := work[start:end]
		inputs := make([]string, len(batch))
		for i, w := range batch {
			inputs[i] = w.c.Text
		}
		vecs, err := e.client.Embed(ctx, inputs)
		if err != nil {
			e.setError(err)
			var retryable RetryableError
			if errors.As(err, &retryable) && ctx.Err() == nil {
				slog.Warn("embedding batch failed; will retry", "err", err, "in", retryDelay)
				go func() {
					select {
					case <-ctx.Done():
					case <-time.After(retryDelay):
						for _, p := range paths {
							e.finish(p)
							e.enqueue(p)
						}
					}
				}()
				return
			}
			slog.Error("embedding batch failed", "err", err)
			for _, p := range paths {
				e.finish(p)
			}
			return
		}
		for i, w := range batch {
			if err := e.store(w.path, w.chunk, w.c.Heading, w.fp, vecs[i]); err != nil {
				slog.Warn("storing embedding", "path", w.path, "err", err)
			}
		}
	}
	e.setError(nil)
	for _, p := range paths {
		e.reloadPath(p)
		e.finish(p)
	}
}

func (e *Embedder) finish(path string) {
	e.queueMu.Lock()
	delete(e.queued, path)
	e.queueMu.Unlock()
}

func (e *Embedder) setError(err error) {
	e.queueMu.Lock()
	if err == nil {
		e.lastError = ""
	} else {
		e.lastError = err.Error()
	}
	e.queueMu.Unlock()
}

// fingerprints returns chunk index → fingerprint for what is stored.
func (e *Embedder) fingerprints(path string) map[int]string {
	out := map[int]string{}
	rows, err := e.db.Query("SELECT chunk, fingerprint FROM embeddings WHERE path = ?", path)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var i int
		var fp string
		if rows.Scan(&i, &fp) == nil {
			out[i] = fp
		}
	}
	return out
}

func (e *Embedder) store(path string, chunk int, heading, fp string, vec []float32) error {
	_, err := e.db.Exec(`INSERT INTO embeddings (path, chunk, heading, fingerprint, vector) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(path, chunk) DO UPDATE SET heading = excluded.heading, fingerprint = excluded.fingerprint, vector = excluded.vector`,
		path, chunk, heading, fp, encode(vec))
	return err
}

func (e *Embedder) remove(path string) {
	if _, err := e.db.Exec("DELETE FROM embeddings WHERE path = ?", path); err != nil {
		slog.Warn("removing embeddings", "path", path, "err", err)
	}
	e.reloadPath(path)
}

// load fills the cache from the table.
func (e *Embedder) load() error {
	rows, err := e.db.Query("SELECT path, chunk, heading, vector FROM embeddings ORDER BY path, chunk")
	if err != nil {
		return err
	}
	defer rows.Close()
	var entries []entry
	byPath := map[string][]int{}
	for rows.Next() {
		var en entry
		var blob []byte
		if err := rows.Scan(&en.path, &en.chunk, &en.heading, &blob); err != nil {
			return err
		}
		en.vec = decode(blob)
		byPath[en.path] = append(byPath[en.path], len(entries))
		entries = append(entries, en)
	}
	e.mu.Lock()
	e.entries, e.byPath = entries, byPath
	e.mu.Unlock()
	return rows.Err()
}

// reloadPath replaces one document's cache entries from the table.
func (e *Embedder) reloadPath(path string) {
	rows, err := e.db.Query("SELECT chunk, heading, vector FROM embeddings WHERE path = ? ORDER BY chunk", path)
	if err != nil {
		return
	}
	defer rows.Close()
	var fresh []entry
	for rows.Next() {
		var en entry
		var blob []byte
		if rows.Scan(&en.chunk, &en.heading, &blob) == nil {
			en.path = path
			en.vec = decode(blob)
			fresh = append(fresh, en)
		}
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	kept := e.entries[:0:0]
	for _, en := range e.entries {
		if en.path != path {
			kept = append(kept, en)
		}
	}
	kept = append(kept, fresh...)
	byPath := make(map[string][]int, len(e.byPath))
	for i, en := range kept {
		byPath[en.path] = append(byPath[en.path], i)
	}
	e.entries, e.byPath = kept, byPath
}

// sweep drops embeddings for documents no longer indexed and queues every
// indexed document — process() skips chunks whose fingerprint is current,
// so a warm start costs one table read per document and no API calls.
func (e *Embedder) sweep() error {
	if _, err := e.db.Exec("DELETE FROM embeddings WHERE path NOT IN (SELECT path FROM documents)"); err != nil {
		return err
	}
	rows, err := e.db.Query("SELECT path FROM documents")
	if err != nil {
		return err
	}
	defer rows.Close()
	var paths []string
	for rows.Next() {
		var p string
		if rows.Scan(&p) == nil {
			paths = append(paths, p)
		}
	}
	if err := e.load(); err != nil {
		return err
	}
	for _, p := range paths {
		e.enqueue(p)
	}
	return nil
}

// ---- vectors ----

func dot(a, b []float32) float32 {
	var s float32
	for i := range a {
		s += a[i] * b[i]
	}
	return s
}

func normalize(v []float32) []float32 {
	var s float64
	for _, x := range v {
		s += float64(x) * float64(x)
	}
	if s == 0 {
		return v
	}
	n := float32(1 / math.Sqrt(s))
	out := make([]float32, len(v))
	for i, x := range v {
		out[i] = x * n
	}
	return out
}

func encode(v []float32) []byte {
	b := make([]byte, 4*len(v))
	for i, x := range v {
		binary.LittleEndian.PutUint32(b[4*i:], math.Float32bits(x))
	}
	return b
}

func decode(b []byte) []float32 {
	v := make([]float32, len(b)/4)
	for i := range v {
		v[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[4*i:]))
	}
	return v
}

// titleOf is the indexer's notion of a title, approximated: frontmatter
// title, else the first H1, else the filename stem.
func titleOf(rel string, raw []byte) string {
	fm := vault.ParseFrontmatter(raw)
	if t, ok := fm["title"].(string); ok && t != "" {
		return t
	}
	body := string(stripFrontmatter(raw))
	for _, line := range splitLines(body) {
		if len(line) > 2 && line[0] == '#' && line[1] == ' ' {
			return line[2:]
		}
	}
	return stem(rel)
}
