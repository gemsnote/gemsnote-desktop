package webapi

import (
	"net/http"
	"strings"

	"github.com/gemsnote/gemsnote/db"
)

func exportFilename(title string, markdown bool) string {
	name := strings.TrimSpace(strings.Map(func(r rune) rune {
		if r < 32 || strings.ContainsRune(`<>:"/\|?*`, r) {
			return '_'
		}
		return r
	}, title))
	name = strings.TrimRight(name, ". ")
	if name == "" {
		name = "note"
	}
	if markdown {
		return name + ".md"
	}
	return name + ".html"
}

func (h *Handler) exportNote(w http.ResponseWriter, r *http.Request) {
	user := h.requireUser(w)
	if user == nil {
		return
	}
	if h.OnExportNote == nil {
		h.fail(w, "exportUnavailable")
		return
	}
	var title, content string
	var markdown bool
	noteID := h.form(r, "noteId")
	if note := h.ownedNote(user.ID, noteID); note != nil {
		title, content, markdown = note.Title, normalizeContent(note.Content), note.IsMarkdown
	} else if shared, err := h.DB.GetSharedNote(db.SharedAccountID(user.Host, user.ID), noteID); err == nil && shared != nil && (shared.CacheState == "ready" || shared.CacheState == "stale") {
		title, content, markdown = shared.Title, normalizeContent(shared.Content), shared.IsMarkdown
	} else {
		h.fail(w, "noAuth")
		return
	}
	saved, err := h.OnExportNote(exportFilename(title, markdown), content, r.Header.Get("Accept-Language"))
	if err != nil {
		h.fail(w, "exportSaveFailed")
		return
	}
	h.writeJSON(w, map[string]any{"Ok": true, "Cancelled": !saved})
}
