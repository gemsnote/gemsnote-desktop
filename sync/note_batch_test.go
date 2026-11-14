package sync

import (
	"encoding/json"
	"fmt"
	"github.com/gemsnote/gemsnote/api"
	"github.com/gemsnote/gemsnote/db"
	"github.com/gemsnote/gemsnote/models"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

func TestMetadataCursorDoesNotSkipAheadOrAcknowledgeFailedBody(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprint(fail), func(t *testing.T) {
			d, err := db.NewInMemory()
			if err != nil {
				t.Fatal(err)
			}
			defer d.Close()
			if err = d.InsertUser(&models.User{ID: "u", IsActive: true}); err != nil {
				t.Fatal(err)
			}
			d.SetCurrentUser("u")
			if err = d.InsertNotebook(&models.Notebook{ID: "b", NotebookID: "b", ServerNotebookID: "remote-b", UserID: "u"}); err != nil {
				t.Fatal(err)
			}
			cursors := []int{}
			calls := 0
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api2/note/getSyncNotes" {
					after, _ := strconv.Atoi(r.URL.Query().Get("afterUsn"))
					cursors = append(cursors, after)
					if r.URL.Query().Get("maxEntry") != "500" {
						t.Error("metadata page is not 500")
					}
					notes := []map[string]any{}
					for i := after + 1; i <= 501 && i <= after+500; i++ {
						notes = append(notes, map[string]any{"NoteId": fmt.Sprint(i), "NotebookId": "remote-b", "Usn": i})
					}
					json.NewEncoder(w).Encode(notes)
					return
				}
				if r.URL.Path != "/api2/note/getNoteContents" {
					t.Errorf("unexpected %s", r.URL.Path)
					http.NotFound(w, r)
					return
				}
				calls++
				if fail && calls == 2 {
					http.Error(w, "fixture failure", 403)
					return
				}
				var p struct {
					IDs []string `json:"noteIds"`
				}
				json.NewDecoder(r.Body).Decode(&p)
				if len(p.IDs) > 20 {
					t.Error("body page is not bounded")
				}
				notes := []map[string]any{}
				for _, id := range p.IDs {
					usn, _ := strconv.Atoi(id)
					if id == "1" {
						usn = 600
					}
					notes = append(notes, map[string]any{"NoteId": id, "NotebookId": "remote-b", "Content": "body-" + id, "Usn": usn})
				}
				json.NewEncoder(w).Encode(notes)
			}))
			defer s.Close()
			c := api.NewClient()
			c.SetHost(s.URL)
			err = NewSyncService(d, c).syncNotes(0, models.NewSyncInfo())
			u, _ := d.GetActiveUser()
			if fail {
				if err == nil || u.NoteUsn != 0 {
					t.Fatalf("failed body acknowledged: user=%+v err=%v", u, err)
				}
				if n, _ := d.GetNoteByServerID("1"); n == nil || n.Content != "body-1" {
					t.Fatal("completed chunk not saved")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if fmt.Sprint(cursors) != "[0 500]" {
				t.Fatalf("cursor skipped records: %v", cursors)
			}
			if n, _ := d.GetNoteByServerID("501"); n == nil || n.Content != "body-501" {
				t.Fatal("later metadata page lost")
			}
		})
	}
}

func TestFullSyncSkipsSameVersionCachedBodyIncludingEmpty(t *testing.T) {
	for _, body := range []string{"cached body", ""} {
		t.Run(body, func(t *testing.T) {
			d, err := db.NewInMemory()
			if err != nil {
				t.Fatal(err)
			}
			defer d.Close()
			d.InsertUser(&models.User{ID: "u", IsActive: true})
			d.SetCurrentUser("u")
			d.InsertNotebook(&models.Notebook{ID: "b", NotebookID: "b", ServerNotebookID: "remote-b", UserID: "u"})
			if err = d.InsertNote(&models.Note{ID: "n", NoteID: "n", ServerNoteID: "remote-n", NotebookID: "b", UserID: "u", Content: body, Usn: 1}); err != nil {
				t.Fatal(err)
			}
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api2/note/getSyncNotes" {
					t.Errorf("unchanged note caused body download: %s", r.URL.Path)
					http.NotFound(w, r)
					return
				}
				w.Write([]byte(`[{"NoteId":"remote-n","NotebookId":"remote-b","Content":"","Usn":1}]`))
			}))
			defer s.Close()
			c := api.NewClient()
			c.SetHost(s.URL)
			if err = NewSyncService(d, c).syncNotes(-1, models.NewSyncInfo()); err != nil {
				t.Fatal(err)
			}
			n, _ := d.GetNote("n")
			if n == nil || n.Content != body {
				t.Fatal("cached body changed")
			}
		})
	}
}
