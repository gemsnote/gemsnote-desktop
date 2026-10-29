package webapi

import (
	"encoding/json"
	"net/http"

	"github.com/gemsnote/gemsnote/db"
	"github.com/gemsnote/gemsnote/models"
)

type sharingRecipient struct {
	UserId   string
	Username string
	NoteIds  []string
}
type sharingNavigation struct {
	Recipients   []sharingRecipient
	Placements   map[string]string
	PlacedShares []map[string]any
}

// Stored IDs are remote IDs. Translate only at the local bridge boundary and
// include received notes only if present in the published permission cache.
func (h *Handler) cachedSharing(user *models.User) sharingNavigation {
	result := sharingNavigation{Recipients: []sharingRecipient{}, Placements: map[string]string{}, PlacedShares: []map[string]any{}}
	accountID := db.SharedAccountID(user.Host, user.ID)
	raw, _ := h.DB.GetConfig("sharing:" + accountID)
	var cached sharingNavigation
	if json.Unmarshal([]byte(raw), &cached) != nil {
		return result
	}
	for _, recipient := range cached.Recipients {
		ids := []string{}
		for _, serverID := range recipient.NoteIds {
			note, _ := h.DB.GetNoteByServerID(serverID)
			if note != nil && note.UserID == user.ID && !note.LocalIsDelete && !note.IsTrash {
				ids = append(ids, note.NoteID)
			}
		}
		recipient.NoteIds = ids
		if len(ids) > 0 {
			result.Recipients = append(result.Recipients, recipient)
		}
	}
	for noteID, serverBook := range cached.Placements {
		note, _ := h.DB.GetSharedNote(accountID, noteID)
		book, _ := h.DB.GetNotebookByServerID(serverBook)
		if note == nil || book == nil || book.UserID != user.ID || book.LocalIsDelete || book.IsTrash {
			continue
		}
		result.Placements[noteID] = book.NotebookID
		result.PlacedShares = append(result.PlacedShares, map[string]any{
			"NoteId": note.NoteID, "NotebookId": book.NotebookID, "UserId": note.OwnerUserID, "Title": note.Title, "Desc": note.Desc, "Tags": note.Tags, "IsMarkdown": note.IsMarkdown, "IsShared": true, "Perm": 0, "CreatedTime": note.CreatedTime, "UpdatedTime": note.UpdatedTime,
		})
	}
	return result
}

func (h *Handler) moveSharedNote(w http.ResponseWriter, r *http.Request) {
	user := h.requireUser(w)
	if user == nil {
		return
	}
	accountID := db.SharedAccountID(user.Host, user.ID)
	noteID := h.form(r, "noteId")
	note, _ := h.DB.GetSharedNote(accountID, noteID)
	target := h.ownedNotebook(user.ID, h.form(r, "notebookId"))
	if note == nil || target == nil {
		h.fail(w, "invalidNotebook")
		return
	}
	if target.ServerNotebookID == "" || target.LocalIsNew {
		h.fail(w, "notebookNotSynced")
		return
	}
	if h.Proxy == nil || !h.Proxy.ensureSession() {
		h.fail(w, "offline")
		return
	}
	data, _, status, err := h.Proxy.callJSON(http.MethodPost, "/api2/shared/move", map[string]string{"noteId": noteID, "notebookId": target.ServerNotebookID})
	if err != nil {
		h.fail(w, "offline")
		return
	}
	ok, msg, parsed := parseRe(data)
	if status < 200 || status >= 300 || !parsed || !ok {
		if msg == "" {
			msg = "moveFailed"
		}
		h.fail(w, msg)
		return
	}
	raw, _ := h.DB.GetConfig("sharing:" + accountID)
	var nav sharingNavigation
	_ = json.Unmarshal([]byte(raw), &nav)
	if nav.Placements == nil {
		nav.Placements = map[string]string{}
	}
	nav.Placements[noteID] = target.ServerNotebookID
	encoded, _ := json.Marshal(nav)
	if err := h.DB.SetConfig("sharing:"+accountID, string(encoded)); err != nil {
		h.fail(w, err.Error())
		return
	}
	h.writeJSON(w, map[string]any{"Ok": true})
}
