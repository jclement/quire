// Email settings: what is configured (SMTP, digest recipient and time) and
// a test send, so a misconfigured relay is found from Settings rather than
// by waiting for a morning digest that never comes.
package api

import (
	"net/http"

	"github.com/jclement/quire/internal/service"
)

// EmailHooks is what main wires in when SMTP is configured.
type EmailHooks struct {
	Status   func() service.EmailStatus
	SendTest func() error
}

func (s *Server) handleEmailStatus(w http.ResponseWriter, _ *http.Request) {
	if s.Email == nil {
		writeData(w, http.StatusOK, service.EmailStatus{})
		return
	}
	writeData(w, http.StatusOK, s.Email.Status())
}

func (s *Server) handleEmailTest(w http.ResponseWriter, _ *http.Request) {
	if s.Email == nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "email is not configured (set QUIRE_SMTP_HOST, QUIRE_SMTP_FROM and QUIRE_DIGEST_TO)")
		return
	}
	if err := s.Email.SendTest(); err != nil {
		writeError(w, http.StatusBadGateway, "INTERNAL_ERROR", "sending failed: "+err.Error())
		return
	}
	writeData(w, http.StatusOK, map[string]bool{"sent": true})
}
