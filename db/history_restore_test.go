package db

import (
	"github.com/gemsnote/gemsnote/models"
	"os"
	"path/filepath"
	"testing"
)

func TestLocalHistoryRestoresFormatAndFiles(t *testing.T) {
	d, err := NewInMemory()
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
	must(d.InsertUser(&models.User{ID: "u", Username: "u"}))
	must(d.InsertNotebook(&models.Notebook{ID: "b", NotebookID: "b", UserID: "u"}))
	n := &models.Note{ID: "n", NoteID: "n", UserID: "u", NotebookID: "b", Content: "historical", IsMarkdown: true}
	must(d.InsertNote(n))
	path := filepath.Join(t.TempDir(), "attachment")
	must(os.WriteFile(path, []byte("bytes"), 0600))
	must(d.InsertAttach(&models.Attach{ID: "a", FileID: "a", NoteID: "n", UserID: "u", Path: path, Title: "original", IsAttach: true}))
	must(d.AddNoteHistory("n", n.Content))
	histories, err := d.GetNoteHistories("n")
	must(err)
	if len(histories) != 1 || !histories[0].FilesComplete {
		t.Fatal(histories)
	}
	id := histories[0].ID
	n.Content = "current"
	n.IsMarkdown = false
	must(d.UpdateNote(n))
	must(d.DeleteAttach("a"))
	if err := d.RestoreLocalHistory("other", "n", id); err == nil {
		t.Fatal("cross-account restore accepted")
	}
	must(d.RestoreLocalHistory("u", "n", id))
	restored, err := d.GetNote("n")
	must(err)
	if restored.Content != "historical" || !restored.IsMarkdown || !restored.IsDirty {
		t.Fatalf("%+v", restored)
	}
	files, err := d.GetAttachsByNote("n")
	must(err)
	if len(files) != 1 || files[0].Path != path || !files[0].IsDirty {
		t.Fatal(files)
	}
	must(os.Remove(path))
	restored.Content = "keep"
	must(d.UpdateNote(restored))
	if err := d.RestoreLocalHistory("u", "n", id); err == nil {
		t.Fatal("missing file accepted")
	}
	unchanged, err := d.GetNote("n")
	must(err)
	if unchanged.Content != "keep" {
		t.Fatal("partial overwrite")
	}
	// Legacy versions retain an unknown manifest rather than an empty set.
	must(d.InsertNoteHistory(&models.NoteHistory{NoteID: "n", Content: "legacy"}))
	histories, err = d.GetNoteHistories("n")
	must(err)
	for _, h := range histories {
		if h.Content == "legacy" {
			if h.FilesComplete || h.IsMarkdown != nil {
				t.Fatal("legacy upgraded to complete")
			}
			if err := d.RestoreLocalHistory("u", "n", h.ID); err == nil {
				t.Fatal("legacy full restore accepted")
			}
		}
	}
}

func TestHistorySchemaUpgradeKeepsLegacyRows(t *testing.T) {
	d, err := NewInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	for _, statement := range []string{
		`ALTER TABLE note_histories DROP COLUMN is_markdown`,
		`ALTER TABLE note_histories DROP COLUMN attachments_json`,
		`ALTER TABLE note_histories DROP COLUMN files_complete`,
		`PRAGMA user_version=3`,
	} {
		if _, err = d.db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	if err = d.migrate(); err != nil {
		t.Fatal(err)
	}
	var version int
	if err = d.db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil || version != 4 {
		t.Fatalf("%d %v", version, err)
	}
	rows, err := d.db.Query(`SELECT is_markdown,attachments_json FROM note_histories`)
	if err != nil {
		t.Fatal(err)
	}
	rows.Close()
}

func TestRemoteCacheHistoryDoesNotClaimCompleteFileSet(t *testing.T) {
	d, err := NewInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	for _, err := range []error{
		d.InsertUser(&models.User{ID: "u", Username: "u"}),
		d.InsertNotebook(&models.Notebook{ID: "b", NotebookID: "b", UserID: "u"}),
		d.InsertNote(&models.Note{ID: "n", NoteID: "n", ServerNoteID: "remote", UserID: "u", NotebookID: "b", Content: "body", IsMarkdown: true}),
	} {
		if err != nil {
			t.Fatal(err)
		}
	}
	if err = d.AddNoteHistory("n", "body"); err != nil {
		t.Fatal(err)
	}
	list, err := d.GetNoteHistories("n")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].FilesComplete || list[0].IsMarkdown == nil || !*list[0].IsMarkdown {
		t.Fatal(list)
	}
}
