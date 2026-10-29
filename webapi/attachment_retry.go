package webapi

import (
	"net/http"
	"os"

	"github.com/gemsnote/gemsnote/api"
)

func (h *Handler) retryAttachment(w http.ResponseWriter, r *http.Request) {
	user := h.requireUser(w)
	if user == nil {
		return
	}
	attachment := h.ownedAttach(user.ID, h.form(r, "attachId"))
	if attachment == nil || h.ownedNote(user.ID, attachment.NoteID) == nil {
		h.fail(w, "noAuth")
		return
	}
	if attachment.IsDirty || attachment.ServerFileID == "" || user.Host == "" || user.Token == "" {
		h.fail(w, "attachmentUnavailable")
		return
	}
	dir := h.Files.GetUserAttachDir(user.ID)
	if err := os.MkdirAll(dir, 0700); err != nil {
		h.fail(w, err.Error())
		return
	}
	client := api.NewClient()
	client.SetHost(user.Host)
	client.SetToken(user.Token)
	path, _, err := client.GetAttachContext(r.Context(), attachment.ServerFileID, dir)
	if err != nil {
		if api.IsAttachmentMissing(err) {
			h.fail(w, "attachmentMissing")
		} else {
			h.fail(w, err.Error())
		}
		return
	}
	// A logout/account switch or deletion during the request must not attach
	// downloaded bytes to a different account or resurrect a deleted record.
	active := h.activeUser()
	if active == nil || active.ID != user.ID || active.Host != user.Host {
		os.Remove(path)
		h.fail(w, "NOTLOGIN")
		return
	}
	if err := h.DB.SetAttachmentCache(user.ID, attachment.FileID, attachment.ServerFileID, path); err != nil {
		os.Remove(path)
		h.fail(w, err.Error())
		return
	}
	h.ok(w)
}
