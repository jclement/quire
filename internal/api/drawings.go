// Excalidraw drawings: create an empty one (the server picks the path), and
// save a scene + render back into it. Reading goes through the files API.
package api

import (
	"encoding/json"
	"net/http"
)

func (s *Server) handleCreateDrawing(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Title string `json:"title"`
	}
	if r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON body")
			return
		}
	}
	d, err := s.Service.CreateDrawing(body.Title)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusCreated, d)
}

func (s *Server) handleSaveDrawing(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Scene json.RawMessage `json:"scene"`
		SVG   string          `json:"svg"`
	}
	// The scene can carry pasted images as data URIs; cap the body at the
	// attachment limit plus a little slack for the JSON around it.
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2*maxAttachmentBody)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON body: expected {scene, svg}")
		return
	}
	d, err := s.Service.SaveDrawing(r.PathValue("path"), body.Scene, []byte(body.SVG))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusOK, d)
}

// maxAttachmentBody mirrors the service's 50MB attachment cap.
const maxAttachmentBody = 50 << 20
