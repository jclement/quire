// Settings → Work items: the URL template "AB#2433" and "#2433" link to.
package api

import "net/http"

func (s *Server) handleGetWorkItems(w http.ResponseWriter, _ *http.Request) {
	writeData(w, http.StatusOK, s.Service.WorkItems())
}

// handleSetWorkItems: body {url_template}; "" turns the links off.
func (s *Server) handleSetWorkItems(w http.ResponseWriter, r *http.Request) {
	var body struct {
		URLTemplate string `json:"url_template"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	if err := s.Service.SetWorkItemURL(body.URLTemplate); err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusOK, s.Service.WorkItems())
}
