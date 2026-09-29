// The work-item link template: where "AB#2433" and "#2433" in notes point.
// Azure DevOps, GitHub issues, Jira — anything with one URL per number. The
// app only ever links; it never fetches an item (that needs a PAT and is a
// separate feature, see DESIGN.md).
package settings

import (
	"fmt"
	"net/url"
	"strings"
)

// WorkItemPlaceholder is replaced by the item number.
const WorkItemPlaceholder = "{id}"

// ValidateWorkItemURL accepts "" (the feature off) or an http(s) URL with
// an {id} placeholder. Errors are written for the person typing it.
func ValidateWorkItemURL(template string) error {
	if template == "" {
		return nil
	}
	if !strings.Contains(template, WorkItemPlaceholder) {
		return fmt.Errorf("the work item URL needs %s where the number goes, e.g. https://dev.azure.com/org/project/_workitems/edit/{id}", WorkItemPlaceholder)
	}
	parsed, err := url.Parse(strings.ReplaceAll(template, WorkItemPlaceholder, "1"))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return fmt.Errorf("the work item URL must start with https:// (got %q)", template)
	}
	return nil
}
