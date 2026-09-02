// Area inheritance: a document without an area: of its own files under the
// area of what it is attached to. A person takes their company's; a project
// its company's, else its first person's; a note or meeting its first
// person's, else its company's, else its project's. Companies are roots,
// daily notes belong to every area, templates to none. Explicit always
// wins, and the chain follows inherited areas too (note → person →
// company), so filing a company files everyone in it.
//
// It is recomputed for the whole vault after every index change: a few
// thousand rows of JSON and a map lookup per link is milliseconds, and it
// means no dependency bookkeeping — the answer is always derived from the
// current state of every file.
package index

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/jclement/quire/internal/vault"
)

// inheritKeys is the frontmatter keys consulted, in order, per type.
var inheritKeys = map[vault.DocType][]string{
	vault.TypePerson:  {"company"},
	vault.TypeProject: {"company", "people", "owner"},
	vault.TypeMeeting: {"people", "company", "project"},
	vault.TypeNote:    {"people", "company", "project"},
}

// maxInheritDepth bounds the walk; a real chain is two hops.
const maxInheritDepth = 4

type areaDoc struct {
	docType  vault.DocType
	explicit string
	targets  map[string][]string // key → resolved paths, in order
}

// PropagateAreas recomputes every document's effective area and provenance
// from explicit values and entity links, writing only the rows that change.
func (ix *Index) PropagateAreas() error {
	names, err := ix.loadNames()
	if err != nil {
		return err
	}
	rows, err := ix.DB.Query("SELECT path, type, area_explicit, frontmatter_json, area, area_from FROM documents")
	if err != nil {
		return fmt.Errorf("loading documents for area propagation: %w", err)
	}
	docs := map[string]areaDoc{}
	current := map[string][2]string{}
	for rows.Next() {
		var p, typ, explicit, fmJSON, area, from string
		if err := rows.Scan(&p, &typ, &explicit, &fmJSON, &area, &from); err != nil {
			rows.Close()
			return err
		}
		docType := vault.DocType(typ)
		d := areaDoc{docType: docType, explicit: explicit, targets: map[string][]string{}}
		if keys, ok := inheritKeys[docType]; ok && explicit == "" {
			var fm map[string]any
			_ = json.Unmarshal([]byte(fmJSON), &fm)
			for _, key := range keys {
				for _, raw := range stringList(fm[key]) {
					if target := linkTarget(stripWikilink(raw)); target != "" {
						if resolved, ok := names[target]; ok {
							d.targets[key] = append(d.targets[key], resolved)
						}
					}
				}
			}
		}
		docs[p] = d
		current[p] = [2]string{area, from}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	memo := map[string][2]string{}
	var resolve func(p string, depth int) (string, string)
	resolve = func(p string, depth int) (string, string) {
		if got, ok := memo[p]; ok {
			return got[0], got[1]
		}
		d, ok := docs[p]
		if !ok {
			return "", ""
		}
		if d.explicit != "" || depth >= maxInheritDepth {
			memo[p] = [2]string{d.explicit, ""}
			return d.explicit, ""
		}
		memo[p] = [2]string{"", ""} // cycle guard while descending
		for _, key := range inheritKeys[d.docType] {
			for _, target := range d.targets[key] {
				if area, _ := resolve(target, depth+1); area != "" {
					memo[p] = [2]string{area, target}
					return area, target
				}
			}
		}
		return "", ""
	}

	tx, err := ix.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	changed := 0
	for p := range docs {
		area, from := resolve(p, 0)
		if current[p] == [2]string{area, from} {
			continue
		}
		if _, err := tx.Exec("UPDATE documents SET area = ?, area_from = ? WHERE path = ?", area, from, p); err != nil {
			return fmt.Errorf("updating area of %s: %w", p, err)
		}
		changed++
	}
	if changed == 0 {
		return nil
	}
	return tx.Commit()
}

// loadNames is the docnames table as one map: normalised name → path
// (the lowest path wins ties, matching ResolveLink).
func (ix *Index) loadNames() (map[string]string, error) {
	rows, err := ix.DB.Query("SELECT name, MIN(path) FROM docnames GROUP BY name")
	if err != nil {
		return nil, fmt.Errorf("loading docnames: %w", err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var name string
		var p sql.NullString
		if err := rows.Scan(&name, &p); err != nil {
			return nil, err
		}
		if p.Valid {
			out[name] = p.String
		}
	}
	return out, rows.Err()
}
