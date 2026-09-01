// Attachment upload and vault-file serving.
package api

import (
	"mime"
	"net/http"
	"path"
	"strings"
)

func (s *Server) handleUploadAttachment(w http.ResponseWriter, r *http.Request) {
	// 50MB + form overhead; the service enforces the real limit.
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid multipart form")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", `missing "file" form field`)
		return
	}
	defer file.Close()

	att, err := s.Service.SaveAttachment(header.Filename, file)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusCreated, att)
}

func (s *Server) handleServeFile(w http.ResponseWriter, r *http.Request) {
	rel := r.PathValue("path")
	if strings.HasSuffix(rel, ".md") {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "markdown is served by the documents API")
		return
	}
	data, err := s.Service.ReadAttachment(rel)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	ctype := mime.TypeByExtension(path.Ext(rel))
	if ctype == "" || strings.HasPrefix(ctype, "text/html") {
		// Never let an uploaded file execute as a page in our origin
		// (SVG/HTML can carry script).
		ctype = "application/octet-stream"
	}
	if strings.HasSuffix(rel, ".svg") {
		ctype = "image/svg+xml"
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'")
	}
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("Cache-Control", "private, max-age=3600")
	_, _ = w.Write(data)
}
