// Queries the calendar feature needs: finding the person behind an
// invitation's email address. Kept out of queries.go because nothing else
// asks it, and it mirrors PeopleWithBirthdays — the index narrows to the
// people who declare the field; the service does the matching.
package index

import "fmt"

// PeopleWithEmail returns person documents whose frontmatter has an
// email: key, as a string or a list (the service reads both shapes).
func (ix *Index) PeopleWithEmail() ([]DocRow, error) {
	rows, err := ix.DB.Query(docSelect + `
		WHERE d.type = 'person'
		  AND json_extract(d.frontmatter_json, '$.email') IS NOT NULL
		ORDER BY d.path`)
	if err != nil {
		return nil, fmt.Errorf("people with email: %w", err)
	}
	defer rows.Close()
	return collectDocs(rows)
}
