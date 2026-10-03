package sync

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gemsnote/gemsnote/api"
	"github.com/gemsnote/gemsnote/db"
	"github.com/gemsnote/gemsnote/models"
)

// The real server serializes ApiNote.Content even on the metadata endpoint.
// Test the complete HTTP decode -> sync -> SQLite path, not a hand-built Note.
func TestSyncIgnoresMetadataContentPlaceholder(t *testing.T) {
	for _, tc := range []struct {
		name        string
		full, fresh bool
		body        string
	}{
		{"incremental web edit", false, false, "web body"},
		{"full repairs same usn", true, false, "repaired web body"},
		{"new note", false, true, "new web note"},
		{"real empty body", false, false, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			contentCalls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api2/user/getSyncState":
					w.Write([]byte(`{"LastSyncUsn":2,"LastSyncTime":0}`))
				case "/api2/notebook/getSyncNotebooks":
					w.Write([]byte(`[{"NotebookId":"remote-book","UserId":"u","Title":"Book","Usn":1}]`))
				case "/api2/note/getSyncNotes":
					w.Write([]byte(`[{"NoteId":"remote-note","NotebookId":"remote-book","UserId":"u","Title":"Web title","Content":"","Usn":2}]`))
				case "/api2/note/getNoteContent":
					contentCalls++
					if r.URL.Query().Get("noteId") != "remote-note" {
						t.Errorf("wrong remote ID: %s", r.URL)
					}
					json.NewEncoder(w).Encode(map[string]any{"Content": tc.body})
				case "/api2/tag/getSyncTags":
					w.Write([]byte(`[]`))
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			database, err := db.NewInMemory()
			if err != nil {
				t.Fatal(err)
			}
			defer database.Close()
			if err = database.InsertUser(&models.User{ID: "u", Username: "u", Host: server.URL, Token: "test", IsActive: true}); err != nil {
				t.Fatal(err)
			}
			database.SetCurrentUser("u")
			if err = database.UpdateUserSyncState("u", map[string]int64{"last_sync_usn": 1}); err != nil {
				t.Fatal(err)
			}
			if err = database.InsertNotebook(&models.Notebook{ID: "b", NotebookID: "local-book", ServerNotebookID: "remote-book", UserID: "u", Title: "Book", Usn: 1}); err != nil {
				t.Fatal(err)
			}
			if !tc.fresh {
				usn := int64(1)
				if tc.full {
					usn = 2
				}
				if err = database.InsertNote(&models.Note{ID: "n", NoteID: "local-note", ServerNoteID: "remote-note", NotebookID: "local-book", UserID: "u", Title: "Old title", Content: "old cached body", Usn: usn}); err != nil {
					t.Fatal(err)
				}
			}
			svc := NewSyncService(database, api.NewClient())
			if tc.full {
				_, err = svc.FullSync()
			} else {
				_, err = svc.IncrSync()
			}
			if err != nil {
				t.Fatal(err)
			}
			got, err := database.GetNoteByServerID("remote-note")
			if err != nil || got == nil {
				t.Fatalf("missing note: %v", err)
			}
			if contentCalls != 1 || got.Content != tc.body || got.Title != "Web title" || got.Usn != 2 {
				t.Fatalf("calls=%d content=%q title=%q usn=%d", contentCalls, got.Content, got.Title, got.Usn)
			}
		})
	}
}
