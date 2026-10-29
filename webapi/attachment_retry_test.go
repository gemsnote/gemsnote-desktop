package webapi

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gemsnote/gemsnote/models"
)

func TestMissingAttachmentPlaceholderAndRetry(t *testing.T) {
	e := newTestEnv(t)
	userID, bookID := e.login(t)
	noteID := "note"
	if err := e.db.InsertNote(&models.Note{ID: noteID, NoteID: noteID, UserID: userID, NotebookID: bookID}); err != nil {
		t.Fatal(err)
	}
	if err := e.db.InsertAttach(&models.Attach{ID: "a", FileID: "local", ServerFileID: "remote", NoteID: noteID, UserID: userID, Title: "ELK.xml", IsAttach: true}); err != nil {
		t.Fatal(err)
	}
	missing := true
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api2/file/getAttach" || r.URL.Query().Get("fileId") != "remote" {
			t.Errorf("wrong server ID: %s", r.URL.Path)
		}
		if missing {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Disposition", `attachment; filename="ELK.xml"`)
		w.Write([]byte("original"))
	}))
	defer server.Close()
	e.db.UpdateUserHost(userID, server.URL)
	e.db.UpdateUserToken(userID, "token")
	var listing struct {
		List []struct {
			AttachID   string `json:"AttachId"`
			CacheState string
		}
	}
	e.postJSON(t, "/api2/attachments", url.Values{"noteId": {noteID}}, &listing)
	if len(listing.List) != 1 || listing.List[0].CacheState != "missing" {
		t.Fatalf("missing placeholder not exposed: %+v", listing)
	}
	var result struct {
		Ok  bool
		Msg string
	}
	e.postJSON(t, "/api2/desktop/attachments/retry", url.Values{"attachId": {"local"}}, &result)
	if result.Ok || result.Msg != "attachmentMissing" {
		t.Fatalf("retry missing: %+v", result)
	}
	missing = false
	e.postJSON(t, "/api2/desktop/attachments/retry", url.Values{"attachId": {"local"}}, &result)
	if !result.Ok {
		t.Fatalf("retry restored: %+v", result)
	}
	e.postJSON(t, "/api2/attachments", url.Values{"noteId": {noteID}}, &listing)
	if listing.List[0].CacheState != "ready" {
		t.Fatalf("restored cache not exposed: %+v", listing)
	}
	if pending, err := e.db.HasPendingChanges(userID); err != nil || pending {
		t.Fatalf("retry marked note dirty: %v %v", pending, err)
	}
}
