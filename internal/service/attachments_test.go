package service

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/jclement/quire/internal/vision"
)

var testPNG = []byte{
	0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a,
	0x00, 0x00, 0x00, 0x0d, 'I', 'H', 'D', 'R',
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4, 0x89,
}

// stubVision serves one canned description and counts how often it is asked.
func stubVision(t *testing.T, s *Service, reply string, status int) *atomic.Int32 {
	t.Helper()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		if status != http.StatusOK {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"error":{"message":"nope"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"` + reply + `"}}]}`))
	}))
	t.Cleanup(srv.Close)
	s.Vision = vision.NewClient(srv.URL, "test-key", "test-vision")
	return &calls
}

func TestSaveAttachmentDescribesAnImage(t *testing.T) {
	s := newTestService(t)
	stubVision(t, s, "A login screen with a red validation error.", http.StatusOK)

	att, err := s.SaveAttachment(context.Background(), "screen shot.png", bytes.NewReader(testPNG))
	if err != nil {
		t.Fatal(err)
	}
	want := "![A login screen with a red validation error.](" + att.Path + ")"
	if att.Markdown != want {
		t.Errorf("markdown = %q, want %q", att.Markdown, want)
	}
	if !s.VisionEnabled() {
		t.Error("VisionEnabled() = false with a client set")
	}
}

func TestSaveAttachmentKeepsTheFilenameWithoutVision(t *testing.T) {
	s := newTestService(t)

	att, err := s.SaveAttachment(context.Background(), "screen shot.png", bytes.NewReader(testPNG))
	if err != nil {
		t.Fatal(err)
	}
	if want := "![screen shot.png](" + att.Path + ")"; att.Markdown != want {
		t.Errorf("markdown = %q, want %q", att.Markdown, want)
	}
	if s.VisionEnabled() {
		t.Error("VisionEnabled() = true with no client")
	}
}

// A failing endpoint must cost the description, never the upload: the file is
// already on disk and the person is watching a placeholder.
func TestSaveAttachmentSurvivesAFailingModel(t *testing.T) {
	s := newTestService(t)
	calls := stubVision(t, s, "", http.StatusInternalServerError)

	att, err := s.SaveAttachment(context.Background(), "screen shot.png", bytes.NewReader(testPNG))
	if err != nil {
		t.Fatalf("upload failed because describing did: %v", err)
	}
	if want := "![screen shot.png](" + att.Path + ")"; att.Markdown != want {
		t.Errorf("markdown = %q, want the filename %q", att.Markdown, want)
	}
	if calls.Load() == 0 {
		t.Error("the vision endpoint was never called")
	}
	if _, err := s.ReadAttachment(att.Path); err != nil {
		t.Errorf("the file should still be on disk: %v", err)
	}
}

// .svg is stored as an image but no vision API accepts it, so it must not
// even be attempted.
func TestSaveAttachmentSkipsFormatsVisionCannotRead(t *testing.T) {
	s := newTestService(t)
	calls := stubVision(t, s, "should never be used", http.StatusOK)

	att, err := s.SaveAttachment(context.Background(), "diagram.svg", strings.NewReader("<svg/>"))
	if err != nil {
		t.Fatal(err)
	}
	if want := "![diagram.svg](" + att.Path + ")"; att.Markdown != want {
		t.Errorf("markdown = %q, want %q", att.Markdown, want)
	}
	if n := calls.Load(); n != 0 {
		t.Errorf("vision was called %d times for an svg", n)
	}
}

// A non-image keeps the plain link form, described or not.
func TestSaveAttachmentLeavesNonImagesAlone(t *testing.T) {
	s := newTestService(t)
	calls := stubVision(t, s, "should never be used", http.StatusOK)

	att, err := s.SaveAttachment(context.Background(), "report.pdf", strings.NewReader("%PDF-1.4"))
	if err != nil {
		t.Fatal(err)
	}
	if want := "[report.pdf](" + att.Path + ")"; att.Markdown != want {
		t.Errorf("markdown = %q, want %q", att.Markdown, want)
	}
	if n := calls.Load(); n != 0 {
		t.Errorf("vision was called %d times for a pdf", n)
	}
}
