package webapi

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/gemsnote/gemsnote/db"
	"github.com/gemsnote/gemsnote/models"
	"github.com/gemsnote/gemsnote/utils"
)

func TestSharedPlacementUsesRemoteIDsAndKeepsSourceReadOnly(t *testing.T) {
	e := newTestEnv(t)
	userID, bookID := e.login(t)
	user, _ := e.db.GetActiveUser()
	// Exercise unequal local/remote IDs, not just the usual equal-ID case.
	book, _ := e.db.GetNotebook(bookID)
	book.ServerNotebookID = utils.ObjectId()
	book.LocalIsNew = false
	if err := e.db.InsertNotebook(book); err != nil {
		t.Fatal(err)
	}
	account := db.SharedAccountID(user.Host, userID)
	noteID, owner, sourceBook := utils.ObjectId(), utils.ObjectId(), utils.ObjectId()
	shared := &models.SharedNote{NoteID: noteID, NotebookID: sourceBook, OwnerUserID: owner, Title: "Received", IsMarkdown: true, TargetContentVersion: "v1"}
	if err := e.db.PublishSharedSnapshot(account, []models.SharedSnapshotItem{{Kind: "note", Note: shared}}, 1); err != nil {
		t.Fatal(err)
	}
	proxy := NewServerProxy(e.db, e.handler.Files)
	proxy.sessionOk = true
	e.handler.Proxy = proxy
	e.db.SetConfig("host", "https://example.test")
	calls := 0
	proxy.client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Path != "/api2/shared/move" || r.Method != "POST" || r.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var payload map[string]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload["noteId"] != noteID || payload["notebookId"] != book.ServerNotebookID {
			t.Fatalf("wrong remote IDs: %v", payload)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"Ok":true}`))}, nil
	})}
	var result map[string]any
	e.postJSON(t, "/api2/shared/move", url.Values{"noteId": {noteID}, "notebookId": {bookID}}, &result)
	if result["Ok"] != true || calls != 1 {
		t.Fatalf("move failed: %v calls=%d", result, calls)
	}
	nav := e.handler.cachedSharing(user)
	if nav.Placements[noteID] != bookID || len(nav.PlacedShares) != 1 {
		t.Fatalf("placement not exposed: %+v", nav)
	}
	cached, _ := e.db.GetSharedNote(account, noteID)
	if cached.NotebookID != sourceBook {
		t.Fatal("source notebook changed")
	}
	if nav.PlacedShares[0]["IsMarkdown"] != true {
		t.Fatal("shared format lost")
	}
	// Normal writes remain forbidden even after a personal placement.
	e.postJSON(t, "/api2/note/moveNote", url.Values{"noteIds[0]": {noteID}, "notebookId": {bookID}}, &result)
	if result["Ok"] != false {
		t.Fatal("source note move allowed")
	}
	if err := e.db.RevokeSharedNote(account, noteID); err != nil {
		t.Fatal(err)
	}
	nav = e.handler.cachedSharing(user)
	if len(nav.PlacedShares) != 0 || len(nav.Placements) != 0 {
		t.Fatal("revoked note leaked through personal placement")
	}
}

func TestNoteListIncludesMarkdownFormat(t *testing.T) {
	h := &Handler{}
	for _, md := range []bool{true, false} {
		item := h.noteListItem(&models.Note{NoteID: "note", IsMarkdown: md})
		if value, ok := item["IsMarkdown"]; !ok || value != md {
			t.Fatalf("lost format: %#v", item)
		}
	}
}

func TestOutgoingSharingMapsLocalNoteIDs(t *testing.T) {
	e := newTestEnv(t)
	uid, bid := e.login(t)
	user, _ := e.db.GetActiveUser()
	localID, remoteID := utils.ObjectId(), utils.ObjectId()
	if err := e.db.InsertNote(&models.Note{ID: utils.ObjectId(), NoteID: localID, ServerNoteID: remoteID, UserID: uid, NotebookID: bid, Title: "Owned"}); err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(sharingNavigation{Recipients: []sharingRecipient{{UserId: "reader", Username: "Reader", NoteIds: []string{remoteID}}}})
	e.db.SetConfig("sharing:"+db.SharedAccountID(user.Host, uid), string(encoded))
	nav := e.handler.cachedSharing(user)
	if len(nav.Recipients) != 1 || len(nav.Recipients[0].NoteIds) != 1 || nav.Recipients[0].NoteIds[0] != localID {
		t.Fatalf("outgoing IDs not mapped: %+v", nav)
	}
	e.db.SetConfig("host", "https://example.test")
	proxy := NewServerProxy(e.db, e.handler.Files)
	proxy.sessionOk = true
	e.handler.Proxy = proxy
	proxy.client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if r.Form.Get("noteId") != remoteID {
			t.Fatalf("forwarded local ID: %v", r.Form)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"Ok":true,"Users":[]}`))}, nil
	})}
	var response map[string]any
	e.postJSON(t, "/api2/web/shareMembers", url.Values{"noteId": {localID}}, &response)
	if response["Ok"] != true {
		t.Fatalf("members failed: %v", response)
	}
}
