// Work items in the agent's instructions: when the owner has told quire
// where "AB#2433" points, agents are told too, so they write references the
// app will link and read "#2433" as a ticket rather than a tag.
package mcp

import "github.com/jclement/quire/internal/service"

// workItemInstructions is an extra working rule, "" when the feature is off.
func workItemInstructions(svc *service.Service) string {
	template := svc.WorkItems().URLTemplate
	if template == "" {
		return ""
	}
	return "\n- Work items are written AB#1234 or a bare #1234 (never a tag) and live at\n  " +
		template + " with {id} the number. Refer to them that way."
}
