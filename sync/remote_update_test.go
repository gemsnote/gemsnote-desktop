package sync

import (
	"encoding/json"
	"github.com/gemsnote/gemsnote/api"
	"github.com/gemsnote/gemsnote/db"
	"github.com/gemsnote/gemsnote/models"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRemoteBodyUpdateAndFullRepair(t *testing.T) {
	for _, tc := range []struct {
		name, body               string
		usn                      int64
		repair, fail, editDuring bool
	}{
		{"web edit", "new web body", 2, false, false, false},
		{"web clears body", "", 2, false, false, false},
		{"full repairs equal usn", "repaired", 1, true, false, false},
		{"download fails", "", 2, false, true, false},
		{"concurrent local edit", "web body", 2, false, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			database, err := db.NewInMemory()
			if err != nil {
				t.Fatal(err)
			}
			defer database.Close()
			if err = database.InsertUser(&models.User{ID: "u", Username: "u", IsActive: true}); err != nil {
				t.Fatal(err)
			}
			database.SetCurrentUser("u")
			if err = database.InsertNotebook(&models.Notebook{ID: "b", NotebookID: "b", ServerNotebookID: "remote-b", UserID: "u", Title: "book"}); err != nil {
				t.Fatal(err)
			}
			old := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
			latest := old.Add(time.Hour)
			if err = database.InsertNote(&models.Note{ID: "n", NoteID: "local-n", ServerNoteID: "remote-n", NotebookID: "b", UserID: "u", Title: "old", Content: "old body", Usn: 1, UpdatedTime: &old}); err != nil {
				t.Fatal(err)
			}
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.URL.Path != "/api2/note/getNoteContent" || r.URL.Query().Get("noteId") != "remote-n" {
					t.Errorf("wrong content request: %s", r.URL)
				}
				if tc.fail {
					http.Error(w, "denied", 403)
					return
				}
				if tc.editDuring {
					n, _ := database.GetNote("local-n")
					n.Content = "offline edit"
					n.IsDirty = true
					n.ContentIsDirty = true
					if err := database.UpdateNote(n); err != nil {
						t.Error(err)
					}
				}
				json.NewEncoder(w).Encode(map[string]any{"Content": tc.body})
			}))
			defer server.Close()
			client := api.NewClient()
			client.SetHost(server.URL)
			info := models.NewSyncInfo()
			err = NewSyncService(database, client).processNoteSync(&models.Note{NoteID: "remote-n", NotebookID: "remote-b", Title: "web title", Usn: tc.usn, UpdatedTime: &latest}, info, tc.repair)
			got, readErr := database.GetNote("local-n")
			if readErr != nil {
				t.Fatal(readErr)
			}
			if tc.fail || tc.editDuring {
				if err == nil || got.Usn != 1 || !got.UpdatedTime.Equal(old) {
					t.Fatalf("failed download acknowledged: %+v %v", got, err)
				}
				want := "old body"
				if tc.editDuring {
					want = "offline edit"
				}
				if got.Content != want {
					t.Fatalf("local body lost: %+v", got)
				}
			} else if err != nil || got.Content != tc.body || got.Usn != tc.usn || !got.UpdatedTime.Equal(latest) || len(info.Note.Updates) != 1 {
				t.Fatalf("remote update missing: %+v %v updates=%v", got, err, info.Note.Updates)
			}
			if calls != 1 {
				t.Fatalf("content requests=%d", calls)
			}
		})
	}
}
