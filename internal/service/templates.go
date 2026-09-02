// Templates: the shape a new document starts from. They are ordinary
// markdown under templates/ — editable in the app or in vim like anything
// else — with two conventions:
//
//   - templates/<type>.md is that type's default and applies whenever a
//     document of that type is created without a body (templates/daily.md
//     shapes every new daily note);
//   - any other file declares `for: <type>` in frontmatter and is offered
//     by name in the New dialog and to agents.
//
// A template's remaining frontmatter is copied into the new document (so a
// decision template can carry `tags: [decision]`), and {{title}}, {{date}},
// {{time}} and {{datetime}} expand in both frontmatter and body.
package service

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jclement/quire/internal/vault"
)

// templateKeys are the template's own metadata, not copied to new documents.
var templateKeys = map[string]bool{"for": true, "description": true, "type": true}

// Templates lists every template, defaults first, then by name.
func (s *Service) Templates() ([]TemplateInfo, error) {
	rows, err := s.Index.ListDocuments(string(vault.TypeTemplate), "", "", 200)
	if err != nil {
		return nil, err
	}
	out := make([]TemplateInfo, 0, len(rows))
	for _, r := range rows {
		f, err := s.Vault.Read(r.Path)
		if err != nil {
			continue
		}
		out = append(out, templateInfo(r.Path, vault.ParseFrontmatter(f.Raw)))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Default != out[j].Default {
			return out[i].Default
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

func templateInfo(path string, fm map[string]any) TemplateInfo {
	name := strings.TrimSuffix(strings.TrimPrefix(path, "templates/"), ".md")
	info := TemplateInfo{Path: path, Name: name}
	if v, ok := fm["description"].(string); ok {
		info.Description = v
	}
	if v, ok := fm["for"].(string); ok && v != "" {
		info.For = strings.ToLower(strings.TrimSpace(v))
	} else if isDocType(name) {
		// templates/meeting.md is the meeting default.
		info.For, info.Default = name, true
	} else {
		info.For = string(vault.TypeNote)
	}
	return info
}

func isDocType(s string) bool {
	switch vault.DocType(s) {
	case vault.TypeNote, vault.TypePerson, vault.TypeCompany, vault.TypeProject, vault.TypeMeeting, vault.TypeDaily:
		return true
	}
	return false
}

// resolveTemplate finds the template to apply: the named one (by path or
// name) if given, else the type's default if it exists, else nothing.
func (s *Service) resolveTemplate(docType vault.DocType, name string) (string, bool) {
	if name != "" {
		candidates := []string{name, "templates/" + name, "templates/" + name + ".md"}
		for _, c := range candidates {
			if strings.HasPrefix(c, "templates/") && s.Vault.Exists(c) {
				return c, true
			}
		}
		return "", false
	}
	def := "templates/" + string(docType) + ".md"
	return def, s.Vault.Exists(def)
}

// renderTemplate expands placeholders and separates the template's own
// metadata from what the new document inherits. Returns (frontmatter to
// seed, body). `at` is the moment the placeholders describe: now for a
// fresh document, the note's own day for a daily note — a journal entry
// for last Tuesday must say last Tuesday.
func (s *Service) renderTemplate(path, title string, at time.Time) ([][2]string, string, error) {
	f, err := s.Vault.Read(path)
	if err != nil {
		return nil, "", fmt.Errorf("template %s: %w", path, err)
	}
	now := at
	expand := strings.NewReplacer(
		"{{title}}", title,
		"{{date}}", now.Format("2006-01-02"),
		"{{time}}", now.Format("15:04"),
		"{{datetime}}", now.Format("2006-01-02T15:04"),
	)
	fmRaw, body, _ := vault.SplitFrontmatter(f.Raw)
	var seed [][2]string
	if len(fmRaw) > 0 {
		for _, kv := range vault.FrontmatterPairs(fmRaw) {
			if templateKeys[kv[0]] {
				continue
			}
			seed = append(seed, [2]string{kv[0], expand.Replace(kv[1])})
		}
	}
	return seed, expand.Replace(string(body)), nil
}
