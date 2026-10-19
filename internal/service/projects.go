// Project status: the five states a project moves through, which the
// Projects board shows as its columns. They live in frontmatter as
// `status:`, one lowercase word, so a project file says where it stands to
// anyone reading it — vim, grep, an agent.
//
// Older vaults used another vocabulary (DESIGN.md once listed active,
// waiting, someday, completed, archived), and agents and humans type
// synonyms. Those are read as the nearest status rather than rewritten:
// files change only when someone sets a status, and then the canonical word
// is what is written.
//
// Closing a project records when: `completed:` or `cancelled:` with the
// date, the way a task gets ✅ or ❌. That is what lets the board keep its
// Complete column to what finished recently without guessing from mtime.
package service

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// The statuses, in board order.
const (
	ProjectPlanned   = "planned"
	ProjectActive    = "active"
	ProjectHold      = "hold"
	ProjectComplete  = "complete"
	ProjectCancelled = "cancelled"
)

// ProjectStatuses is every status, in the order the board shows them.
var ProjectStatuses = []string{ProjectPlanned, ProjectActive, ProjectHold, ProjectComplete, ProjectCancelled}

// projectStatusAliases maps what people (and older vaults) write onto a
// status. Keys are lowercased with spaces, dashes and underscores removed.
var projectStatusAliases = map[string]string{
	"planned": ProjectPlanned, "planning": ProjectPlanned, "someday": ProjectPlanned,
	"backlog": ProjectPlanned, "idea": ProjectPlanned, "proposed": ProjectPlanned,
	"todo": ProjectPlanned, "notstarted": ProjectPlanned,
	"active": ProjectActive, "inprogress": ProjectActive, "doing": ProjectActive,
	"started": ProjectActive, "ongoing": ProjectActive, "open": ProjectActive,
	"hold": ProjectHold, "onhold": ProjectHold, "waiting": ProjectHold,
	"paused": ProjectHold, "blocked": ProjectHold, "parked": ProjectHold,
	"complete": ProjectComplete, "completed": ProjectComplete, "done": ProjectComplete,
	"finished": ProjectComplete, "shipped": ProjectComplete, "closed": ProjectComplete,
	"archived":  ProjectComplete,
	"cancelled": ProjectCancelled, "canceled": ProjectCancelled, "dropped": ProjectCancelled,
	"abandoned": ProjectCancelled, "killed": ProjectCancelled,
}

// NormalizeProjectStatus reads a frontmatter status as one of
// ProjectStatuses; ok is false when it names none of them.
func NormalizeProjectStatus(raw string) (status string, ok bool) {
	key := strings.ToLower(strings.TrimSpace(raw))
	key = strings.NewReplacer(" ", "", "-", "", "_", "").Replace(key)
	status, ok = projectStatusAliases[key]
	return status, ok
}

// projectMeta derives a project's board fields from its frontmatter; nil
// for any other type.
func projectMeta(docType string, fm map[string]any) *ProjectMeta {
	if docType != "project" {
		return nil
	}
	status, ok := NormalizeProjectStatus(fmt.Sprint(fm["status"]))
	if fm["status"] == nil || !ok {
		status = ProjectActive
	}
	meta := &ProjectMeta{Status: status, Due: dayValue(fm["due"]), Company: linkName(fm["company"])}
	switch status {
	case ProjectComplete:
		meta.Closed = dayValue(fm["completed"])
	case ProjectCancelled:
		meta.Closed = dayValue(fm["cancelled"])
	}
	switch rank := fm["rank"].(type) {
	case float64:
		meta.Rank = &rank
	case int:
		r := float64(rank)
		meta.Rank = &r
	}
	return meta
}

// projectMetaFromJSON is projectMeta for an index row, whose frontmatter is
// stored as JSON.
func projectMetaFromJSON(docType string, raw json.RawMessage) *ProjectMeta {
	if docType != "project" {
		return nil
	}
	var fm map[string]any
	_ = json.Unmarshal(raw, &fm)
	return projectMeta(docType, fm)
}

// dayValue is a date-ish frontmatter value as YYYY-MM-DD. YAML reads
// a bare 2026-10-18 as a timestamp, and the index stores that as RFC 3339.
func dayValue(v any) string {
	switch d := v.(type) {
	case time.Time:
		return d.Format("2006-01-02")
	case string:
		d = strings.TrimSpace(d)
		if len(d) >= 10 {
			if _, err := time.Parse("2006-01-02", d[:10]); err == nil {
				return d[:10]
			}
		}
	}
	return ""
}

// linkName is the display name inside a wikilink value: "[[Acme|ACME]]" is
// "ACME", a bare "Acme" is itself.
func linkName(v any) string {
	s, ok := v.(string)
	if !ok {
		return ""
	}
	s = strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(s), "[["), "]]")
	if _, display, found := strings.Cut(s, "|"); found {
		return strings.TrimSpace(display)
	}
	return strings.TrimSpace(s)
}

// projectStatusChanges turns a status being set on a project into the full
// set of frontmatter edits: the canonical word, and the closing date that
// goes with it. A status that does not change leaves the dates alone, and
// any date the caller set explicitly wins.
func (s *Service) projectStatusChanges(current map[string]any, values map[string]any) (map[string]any, error) {
	raw, ok := values["status"]
	if !ok || raw == nil {
		return values, nil
	}
	next, ok := NormalizeProjectStatus(fmt.Sprint(raw))
	if !ok {
		return nil, fmt.Errorf("%w: project status %q is not one of %s", ErrValidation, fmt.Sprint(raw), strings.Join(ProjectStatuses, ", "))
	}
	out := make(map[string]any, len(values)+2)
	for key, value := range values {
		out[key] = value
	}
	out["status"] = next
	if previous := projectMeta("project", current); previous.Status == next && current["status"] != nil {
		return out, nil
	}
	stamp := func(key string, value any) {
		if _, set := values[key]; !set {
			out[key] = value
		}
	}
	switch next {
	case ProjectComplete:
		stamp("completed", s.today())
		stamp("cancelled", nil)
	case ProjectCancelled:
		stamp("cancelled", s.today())
		stamp("completed", nil)
	default:
		stamp("completed", nil)
		stamp("cancelled", nil)
	}
	return out, nil
}
