// Work-item links: the setting behind rendering "AB#2433" and "#2433" as
// links to the owner's tracker. The rendering itself is the SPA's (see
// web/src/lib/workItems.ts); the server stores, validates and tells agents.
package service

import (
	"fmt"
	"strings"

	"github.com/jclement/quire/internal/settings"
)

// WorkItems returns the configured link template ("" when off or when
// there is no settings store).
func (s *Service) WorkItems() WorkItemSettings {
	if s.Settings == nil {
		return WorkItemSettings{}
	}
	cfg, err := s.Settings.Load()
	if err != nil {
		return WorkItemSettings{}
	}
	return WorkItemSettings{URLTemplate: cfg.WorkItemURL}
}

// SetWorkItemURL stores the link template; "" turns the links off.
func (s *Service) SetWorkItemURL(template string) error {
	if s.Settings == nil {
		return fmt.Errorf("%w: settings are not available", ErrValidation)
	}
	template = strings.TrimSpace(template)
	if err := settings.ValidateWorkItemURL(template); err != nil {
		return fmt.Errorf("%w: %v", ErrValidation, err)
	}
	cfg, err := s.Settings.Load()
	if err != nil {
		return err
	}
	cfg.WorkItemURL = template
	return s.Settings.Save(cfg)
}
