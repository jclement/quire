// Agent guidance: house rules the vault owner writes once and every MCP
// client is told about. Stored as an ordinary vault document (AGENTS.md at
// the vault root) rather than as app state, because that is the whole
// premise of quire — it is editable in the app, in vim, greppable, and
// versioned by the vault's git repo like everything else.
package service

import (
	"errors"
	"strings"

	"github.com/jclement/quire/internal/vault"
)

// GuidancePath is where the owner's agent guidance lives. The name matches
// the convention agents already look for in a repository.
const GuidancePath = "AGENTS.md"

// AgentGuidance returns the owner's guidance, or "" when none is written.
func (s *Service) AgentGuidance() string {
	f, err := s.Vault.Read(GuidancePath)
	if err != nil {
		if !errors.Is(err, vault.ErrNotFound) {
			// A read failure here must never break MCP; agents just get the
			// built-in instructions.
			return ""
		}
		return ""
	}
	_, body, hasFrontmatter := vault.SplitFrontmatter(f.Raw)
	if !hasFrontmatter {
		body = f.Raw
	}
	return strings.TrimSpace(string(body))
}

// SetAgentGuidance writes the guidance document, creating it if needed.
// Empty text deletes it, so "no guidance" leaves no confusing empty file.
func (s *Service) SetAgentGuidance(text string) (Document, error) {
	text = strings.TrimSpace(text)
	existing, err := s.Vault.Read(GuidancePath)
	exists := err == nil

	if text == "" {
		if exists {
			if err := s.DeleteDocument(GuidancePath); err != nil {
				return Document{}, err
			}
		}
		return Document{}, nil
	}

	base := ""
	if exists {
		base = existing.SHA256
	}
	return s.UpdateDocument(GuidancePath, text+"\n", base)
}
