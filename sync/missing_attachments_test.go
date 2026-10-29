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

func TestMissingAttachmentDoesNotStopNoteSyncOrDropRemoteReference(t *testing.T) {
	d, err := db.NewInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(d.InsertUser(&models.User{ID: "u", Username: "u", IsActive: true}))
	d.SetCurrentUser("u")
	must(d.InsertNotebook(&models.Notebook{ID: "b", NotebookID: "b", ServerNotebookID: "b", UserID: "u"}))
	notes := []*models.Note{
		{NoteID: "first", NotebookID: "b", UserID: "u", Title: "with missing file", Content: "first body", Usn: 1, Files: []*models.FileRef{{FileID: "missing", Title: "ELK.xml", IsAttach: true}}},
		{NoteID: "second", NotebookID: "b", UserID: "u", Title: "next note", Content: "second body", Usn: 2, Files: []*models.FileRef{}},
	}
	missing := true
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/note/getSyncNotes":
			json.NewEncoder(w).Encode(notes)
		case "/api2/note/getNoteContent":
			json.NewEncoder(w).Encode(map[string]any{"Ok": true, "Content": r.URL.Query().Get("noteId") + " body"})
		case "/api2/file/getAttach":
			if missing {
				http.Error(w, "fileNotFound", http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Disposition", `attachment; filename="ELK.xml"`)
			w.Write([]byte("restored"))
		default:
			t.Errorf("unexpected request: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := api.NewClient()
	client.SetHost(server.URL)
	s := NewSyncService(d, client)
	s.files.SetDataDir(t.TempDir())
	must(s.syncNotes(0, models.NewSyncInfo()))
	first, _ := d.GetNoteByServerID("first")
	second, _ := d.GetNoteByServerID("second")
	if first == nil || second == nil || first.Content != "first body" || second.Content != "second body" {
		t.Fatalf("notes did not continue: %+v %+v", first, second)
	}
	files, err := d.GetAttachsByNote(first.NoteID)
	must(err)
	if len(files) != 1 || files[0].ServerFileID != "missing" || files[0].Path != "" || files[0].IsDirty {
		t.Fatalf("missing placeholder: %+v", files)
	}
	refs, bodies := s.getNoteFilesForUpload(first)
	if len(refs) != 1 || refs[0].FileID != "missing" || refs[0].HasBody || len(bodies) != 0 {
		t.Fatalf("placeholder must retain the reference without uploading empty bytes: %+v %+v", refs, bodies)
	}
	if pending, err := d.HasPendingChanges("u"); err != nil || pending {
		t.Fatalf("placeholder counts as dirty: %v %v", pending, err)
	}
	localID := files[0].FileID
	missing = false
	must(s.syncAttachmentSet(notes[0]))
	files, err = d.GetAttachsByNote(first.NoteID)
	must(err)
	if len(files) != 1 || files[0].Path == "" || files[0].FileID != localID {
		t.Fatalf("retry did not restore the placeholder: %+v", files)
	}
}
