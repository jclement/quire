package mcp

import (
	"bytes"
	"context"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// onePixelPNG is a real PNG header, so what travels through the tool result
// is image bytes rather than a placeholder.
var onePixelPNG = []byte{
	0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a,
	0x00, 0x00, 0x00, 0x0d, 'I', 'H', 'D', 'R',
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4, 0x89,
}

func TestReadAttachmentHandsBackTheImage(t *testing.T) {
	session, svc := connectWithService(t)
	att, err := svc.SaveAttachment(context.Background(), "screen shot.png", bytes.NewReader(onePixelPNG))
	if err != nil {
		t.Fatal(err)
	}

	res, err := session.CallTool(context.Background(), &sdk.CallToolParams{
		Name: "read_attachment", Arguments: map[string]any{"path": att.Path},
	})
	if err != nil {
		t.Fatalf("read_attachment: %v", err)
	}
	if res.IsError {
		t.Fatalf("tool error: %+v", res.Content)
	}
	if len(res.Content) != 1 {
		t.Fatalf("want one content block, got %d", len(res.Content))
	}
	img, ok := res.Content[0].(*sdk.ImageContent)
	if !ok {
		t.Fatalf("content is %T, want *sdk.ImageContent", res.Content[0])
	}
	if img.MIMEType != "image/png" {
		t.Errorf("mime = %q, want image/png", img.MIMEType)
	}
	if !bytes.Equal(img.Data, onePixelPNG) {
		t.Errorf("image bytes did not survive the round trip")
	}
}

func TestReadAttachmentRefusesWhatItCannotShow(t *testing.T) {
	session, svc := connectWithService(t)
	notes, err := svc.SaveAttachment(context.Background(), "notes.txt", bytes.NewReader([]byte("plain text")))
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		path string
	}{
		{"not an image", notes.Path},
		{"no such file", "attachments/2026/09/missing-abc123.png"},
		// The path comes from an agent, so the vault's traversal guard is
		// part of this tool's contract, not someone else's problem.
		{"escaping the vault", "../../etc/passwd"},
		{"escaping with an image name", "../../../tmp/evil.png"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := session.CallTool(context.Background(), &sdk.CallToolParams{
				Name: "read_attachment", Arguments: map[string]any{"path": tt.path},
			})
			if err != nil {
				return // A protocol-level refusal is a refusal.
			}
			if !res.IsError {
				t.Fatalf("want a refusal for %q, got %+v", tt.path, res.Content)
			}
		})
	}
}
