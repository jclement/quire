// Image-description backfill: the companion to describing on upload.
// Screenshots pasted before vision was switched on still carry their filename
// as alt text; this walks the vault, describes each such image once, and
// rewrites only that alt text. Every other byte of the note stays as it was,
// and so does its modified time — a backfill is maintenance, not an edit.
//
// It runs in the background after startup (see main.go) and is idempotent by
// construction: a described image no longer has filename-shaped alt text, so
// the next run finds nothing to do and costs nothing.
package service

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/jclement/quire/internal/vault"
	"github.com/jclement/quire/internal/vision"
)

// backfillDescribeTimeout is looser than the upload path's describeTimeout:
// nobody is watching a placeholder here, so a slow model may take its time.
const backfillDescribeTimeout = 60 * time.Second

// backfillRetryDelays are the pauses before retrying a rate-limited or
// failed-upstream call. A variable so tests need not wait them out.
var backfillRetryDelays = []time.Duration{5 * time.Second, 30 * time.Second}

// errTooLargeToDescribe marks an image the vision API would reject for size.
var errTooLargeToDescribe = errors.New("image too large to describe")

// imageRefRe matches ![alt](dest) and ![alt](dest "title"), with dest either
// bare or <angle-bracketed> — how CommonMark spells a path with spaces.
var imageRefRe = regexp.MustCompile(`!\[([^\]\n]*)\]\(\s*(<[^>\n]+>|[^\s)]+)(?:\s+"[^"\n]*")?\s*\)`)

// filenameAltRe recognises alt text that is only a file name: what an upload
// writes ("screen shot.png"), what Obsidian writes ("Pasted image
// 20240101.png"). Prose someone typed rarely ends in ".png", so this is the
// line between "never described" and "someone wrote this; leave it alone".
var filenameAltRe = regexp.MustCompile(`(?i)^[^/\\]*\.(png|jpe?g|gif|webp|svg|heic|avif)$`)

// codeFenceRe opens or closes a fenced block — the same rule markdown.Scan
// uses, so the backfill and the index agree about what is code.
var codeFenceRe = regexp.MustCompile("^\\s*(```|~~~)")

// BackfillReport is what one backfill run did, for the log line after it.
type BackfillReport struct {
	// Found counts undescribed image references across the vault.
	Found int
	// Described counts distinct images the model described.
	Described int
	// Failed counts distinct images the model could not describe this run;
	// they keep their filename and are retried on the next start.
	Failed int
	// Skipped counts distinct images too large to send.
	Skipped int
	// Documents counts notes rewritten.
	Documents int
}

// imageRef is one undescribed image reference inside a note.
type imageRef struct {
	// AltStart and AltEnd are byte offsets of the alt text in the raw file
	// (equal for ![](…)), so a rewrite replaces exactly those bytes.
	AltStart, AltEnd int
	// Target is the vault-relative path of the image the reference names.
	Target string
}

// BackfillImageDescriptions describes every image in the vault whose alt
// text is still empty or a bare filename, rewriting those alt texts in place.
// It stops early, with ctx's error, when ctx ends.
func (s *Service) BackfillImageDescriptions(ctx context.Context) (BackfillReport, error) {
	var report BackfillReport
	if s.Vision == nil {
		return report, fmt.Errorf("%w: vision is not configured", ErrValidation)
	}
	plan, err := s.planBackfill()
	if err != nil {
		return report, err
	}
	for _, refs := range plan {
		report.Found += len(refs)
	}
	if report.Found == 0 {
		return report, nil
	}
	slog.Info("backfilling image descriptions", "images", report.Found, "documents", len(plan))

	described := map[string]string{} // target → alt text
	settled := map[string]bool{}     // targets already tried this run
	docs := make([]string, 0, len(plan))
	for doc := range plan {
		docs = append(docs, doc)
	}
	sort.Strings(docs)

	for _, doc := range docs {
		// Describe then rewrite one note at a time, so a run interrupted by
		// a restart keeps everything it had already paid for.
		for _, ref := range plan[doc] {
			if settled[ref.Target] {
				continue
			}
			if err := ctx.Err(); err != nil {
				return report, err
			}
			settled[ref.Target] = true
			alt, err := s.describeForBackfill(ctx, ref.Target)
			switch {
			case ctx.Err() != nil:
				return report, ctx.Err()
			case errors.Is(err, errTooLargeToDescribe):
				report.Skipped++
			case err != nil || alt == "":
				report.Failed++
				slog.Warn("backfill: describing image", "path", ref.Target, "err", err)
			default:
				described[ref.Target] = alt
				report.Described++
			}
		}
		changed, err := s.applyDescriptions(doc, described)
		if err != nil {
			slog.Warn("backfill: rewriting note", "path", doc, "err", err)
			continue
		}
		if changed {
			report.Documents++
		}
	}
	return report, nil
}

// planBackfill finds the undescribed image references in every note.
func (s *Service) planBackfill() (map[string][]imageRef, error) {
	plan := map[string][]imageRef{}
	err := s.Vault.WalkMarkdown(func(rel string, _ fs.FileInfo) error {
		f, err := s.Vault.Read(rel)
		if err != nil {
			slog.Warn("backfill: reading note", "path", rel, "err", err)
			return nil
		}
		if refs := findUndescribedImages(rel, f.Raw, s.Vault.Exists); len(refs) > 0 {
			plan[rel] = refs
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walking the vault: %w", err)
	}
	return plan, nil
}

// describeForBackfill describes one image, retrying what is worth retrying.
func (s *Service) describeForBackfill(ctx context.Context, target string) (string, error) {
	f, err := s.Vault.Read(target)
	if err != nil {
		return "", err
	}
	if len(f.Raw) > vision.MaxBytes {
		return "", errTooLargeToDescribe
	}
	mimeType := vision.MIMEType(path.Ext(target))
	for attempt := 0; ; attempt++ {
		callCtx, cancel := context.WithTimeout(ctx, backfillDescribeTimeout)
		alt, err := s.Vision.Describe(callCtx, f.Raw, mimeType)
		cancel()
		var retryable vision.RetryableError
		if err == nil || !errors.As(err, &retryable) || attempt >= len(backfillRetryDelays) {
			return alt, err
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(backfillRetryDelays[attempt]):
		}
	}
}

// applyDescriptions rewrites a note's undescribed alt texts from described,
// re-reading the note first so descriptions land on whatever is on disk now
// rather than on the snapshot planning saw. A save that races the rewrite is
// met by one fresh re-read; a note that keeps changing is left for next time.
func (s *Service) applyDescriptions(doc string, described map[string]string) (bool, error) {
	for attempt := 0; attempt < 2; attempt++ {
		f, err := s.Vault.Read(doc)
		if err != nil {
			return false, err
		}
		refs := findUndescribedImages(doc, f.Raw, s.Vault.Exists)
		content := string(f.Raw)
		changed := false
		// Right to left, so earlier offsets survive later replacements.
		for i := len(refs) - 1; i >= 0; i-- {
			alt, ok := described[refs[i].Target]
			if !ok {
				continue
			}
			content = content[:refs[i].AltStart] + alt + content[refs[i].AltEnd:]
			changed = true
		}
		if !changed {
			return false, nil
		}
		_, err = s.Vault.WriteKeepingModTime(doc, []byte(content), f.SHA256, f.ModTime)
		if errors.Is(err, vault.ErrConflict) {
			continue
		}
		if err != nil {
			return false, err
		}
		if _, err := s.Index.IndexFile(doc); err != nil {
			return true, fmt.Errorf("indexing after backfill: %w", err)
		}
		return true, nil
	}
	return false, fmt.Errorf("%s kept changing during the backfill; will retry on next start", doc)
}

// findUndescribedImages returns the image references in a note whose alt text
// is empty or a bare filename and whose target is an existing image a vision
// API can read. Frontmatter, fenced blocks and inline code are skipped: a
// reference quoted as an example is not an image in the note.
func findUndescribedImages(docPath string, raw []byte, exists func(string) bool) []imageRef {
	_, body, _ := vault.SplitFrontmatter(raw)
	offset := len(raw) - len(body)

	var refs []imageRef
	inFence := false
	for _, line := range strings.SplitAfter(string(body), "\n") {
		lineStart := offset
		offset += len(line)
		if codeFenceRe.MatchString(line) {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		spans := inlineCodeSpans(line)
		for _, m := range imageRefRe.FindAllStringSubmatchIndex(line, -1) {
			if insideAny(m[0], spans) {
				continue
			}
			alt := strings.TrimSpace(line[m[2]:m[3]])
			if alt != "" && !filenameAltRe.MatchString(alt) {
				continue
			}
			target := resolveImageTarget(docPath, line[m[4]:m[5]], exists)
			if target == "" {
				continue
			}
			refs = append(refs, imageRef{AltStart: lineStart + m[2], AltEnd: lineStart + m[3], Target: target})
		}
	}
	return refs
}

// resolveImageTarget turns a reference's destination into the vault path of
// a describable image, or "" when it names anything else. quire writes
// vault-relative paths; a vault imported from elsewhere may use paths
// relative to the note, so that is tried second.
func resolveImageTarget(docPath, dest string, exists func(string) bool) string {
	d := strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(dest), "<"), ">")
	if strings.Contains(d, "://") || strings.HasPrefix(d, "data:") || strings.HasPrefix(d, "#") {
		return ""
	}
	if i := strings.IndexAny(d, "?#"); i >= 0 {
		d = d[:i]
	}
	if unescaped, err := url.PathUnescape(d); err == nil {
		d = unescaped
	}
	d = strings.TrimPrefix(d, "/")
	if !vision.Describable(path.Ext(d)) {
		return ""
	}
	for _, candidate := range []string{d, path.Join(path.Dir(docPath), d)} {
		if vault.ValidatePath(candidate) == nil && exists(candidate) {
			return candidate
		}
	}
	return ""
}

// inlineCodeSpans returns the [start, end) byte ranges of `code` spans on a
// line. A run of n backticks closes only on another run of exactly n; an
// unmatched run is literal text.
func inlineCodeSpans(line string) [][2]int {
	var spans [][2]int
	for i := 0; i < len(line); {
		if line[i] != '`' {
			i++
			continue
		}
		run := runOfBackticks(line, i)
		closeAt := -1
		for j := i + run; j < len(line); {
			if line[j] != '`' {
				j++
				continue
			}
			other := runOfBackticks(line, j)
			if other == run {
				closeAt = j + other
				break
			}
			j += other
		}
		if closeAt < 0 {
			i += run
			continue
		}
		spans = append(spans, [2]int{i, closeAt})
		i = closeAt
	}
	return spans
}

func runOfBackticks(line string, at int) int {
	n := 0
	for at+n < len(line) && line[at+n] == '`' {
		n++
	}
	return n
}

func insideAny(pos int, spans [][2]int) bool {
	for _, span := range spans {
		if pos >= span[0] && pos < span[1] {
			return true
		}
	}
	return false
}
