package webapi

import (
	"encoding/base64"
	"net/http"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// The shared frontend converts the displayed note or notebook into Markdown, RTF or ZIP.
// It supplies bytes, never an output path; the native save dialog owns that choice.
func (h *Handler) exportFile(w http.ResponseWriter, r *http.Request) {
	if h.requireUser(w) == nil {
		return
	}
	if h.OnExportNote == nil {
		h.fail(w, "exportUnavailable")
		return
	}
	filename := h.form(r, "filename")
	ext := filepath.Ext(filename)
	if (ext != ".md" && ext != ".rtf" && ext != ".zip") || len(filename) > 255 || filename == ext || strings.ContainsAny(filename, "<>:\"/\\|?*\x00\r\n") || strings.HasPrefix(filename, ".") {
		h.fail(w, "invalidExportFile")
		return
	}
	content, err := base64.StdEncoding.DecodeString(h.form(r, "data"))
	if err != nil || len(content) > 128*1024*1024 || (ext == ".md" && !utf8.Valid(content)) {
		h.fail(w, "invalidExportFile")
		return
	}
	saved, err := h.OnExportNote(filename, string(content), r.Header.Get("Accept-Language"))
	if err != nil {
		h.fail(w, "exportSaveFailed")
		return
	}
	h.writeJSON(w, map[string]any{"Ok": true, "Cancelled": !saved})
}
