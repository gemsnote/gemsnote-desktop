package sync

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gemsnote/gemsnote/api"
	"github.com/gemsnote/gemsnote/db"
	"github.com/gemsnote/gemsnote/models"
)

func TestRestoredAttachmentSetsSurviveEmptyBodyAndIncrementalSync(t *testing.T) {
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
	must(d.InsertNotebook(&models.Notebook{ID: "b", NotebookID: "b", UserID: "u"}))
	n := &models.Note{ID: "n", NoteID: "n", ServerNoteID: "remote", NotebookID: "b", UserID: "u", Content: "", Usn: 1}
	must(d.InsertNote(n))
	path := filepath.Join(t.TempDir(), "original")
	must(os.WriteFile(path, []byte("bytes"), 0600))
	must(d.InsertAttach(&models.Attach{ID: "clean", FileID: "clean", ServerFileID: "remote-file", NoteID: "n", UserID: "u", Path: path, IsAttach: true}))
	must(d.InsertAttach(&models.Attach{ID: "restored", FileID: "restored", NoteID: "n", UserID: "u", Path: path, IsAttach: true, IsDirty: true}))
	client := api.NewClient()
	svc := NewSyncService(d, client)
	svc.files.SetDataDir(t.TempDir())
	files, data := svc.getNoteFilesForUpload(n)
	if len(files) != 2 || len(data) != 1 {
		t.Fatalf("files=%+v data=%+v", files, data)
	}
	must(d.DeleteAttach("restored"))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("fileId") == "missing" {
			w.Write([]byte("No Such File"))
			return
		}
		w.Header().Set("Content-Disposition", `attachment; filename="restored.txt"`)
		w.Write([]byte("historical"))
	}))
	defer server.Close()
	client.SetHost(server.URL)
	client.SetToken("token")
	remote := &models.Note{NoteID: "remote", Usn: 1, Files: []*models.FileRef{{FileID: "new-file", Title: "restored.txt", IsAttach: true}}}
	must(svc.syncAttachmentSet(remote))
	active, err := d.GetAttachsByNote("n")
	must(err)
	if len(active) != 1 || active[0].ServerFileID != "new-file" {
		t.Fatal(active)
	}
	bytes, err := os.ReadFile(active[0].Path)
	must(err)
	if string(bytes) != "historical" {
		t.Fatal(string(bytes))
	}
	remote.Files = []*models.FileRef{{FileID: "missing", IsAttach: true}}
	if err = svc.syncAttachmentSet(remote); err == nil || !strings.Contains(err.Error(), "note remote") {
		t.Fatalf("missing attachment error must identify the note: %v", err)
	}
	active, err = d.GetAttachsByNote("n")
	must(err)
	if len(active) != 1 {
		t.Fatal("lost active set on error")
	}
	remote.Files = []*models.FileRef{}
	must(svc.syncAttachmentSet(remote))
	active, err = d.GetAttachsByNote("n")
	must(err)
	if len(active) != 0 {
		t.Fatal("empty set not applied")
	}
	if _, err = os.Stat(path); err != nil {
		t.Fatal("historical original deleted")
	}
}
