package db

import (
	"database/sql"
	"github.com/gemsnote/gemsnote/models"
	"path/filepath"
	"testing"
)

func TestOptionalTrashNotebookMigratesOldCacheWithoutLosingChildren(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	raw, err := sql.Open("sqlite", path+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	d := &Database{db: raw}
	schema, _ := migrationsFS.ReadFile("migrations.sql")
	if _, err = raw.Exec(string(schema)); err != nil {
		t.Fatal(err)
	}
	if _, err = raw.Exec(`ALTER TABLE notes ADD COLUMN local_edited_time INTEGER; PRAGMA user_version=5`); err != nil {
		t.Fatal(err)
	}
	if err = d.InsertUser(&models.User{ID: "u", Username: "u"}); err != nil {
		t.Fatal(err)
	}
	if err = d.InsertNotebook(&models.Notebook{ID: "b", NotebookID: "b", UserID: "u"}); err != nil {
		t.Fatal(err)
	}
	if err = d.InsertNote(&models.Note{ID: "n", NoteID: "n", UserID: "u", NotebookID: "b", Content: "body", IsTrash: true}); err != nil {
		t.Fatal(err)
	}
	if _, err = raw.Exec(`INSERT INTO note_import_times VALUES ('n','2020','2021')`); err != nil {
		t.Fatal(err)
	}
	raw.Close()
	d, err = New(path)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	var count int
	if err = d.db.QueryRow(`SELECT COUNT(*) FROM note_import_times WHERE note_id='n'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("child records lost: %d %v", count, err)
	}
	if err = d.ApplyRemoteNotebookDelete("b"); err != nil {
		t.Fatal(err)
	}
	note, err := d.GetNote("n")
	if err != nil || note == nil || note.NotebookID != "" || note.Content != "body" || !note.IsTrash {
		t.Fatalf("invalid detached trash: %+v %v", note, err)
	}
	book, err := d.GetNotebook("b")
	if err != nil || book != nil {
		t.Fatalf("deleted book retained: %+v %v", book, err)
	}
	if err = d.SetNoteTrash("n", false); err == nil {
		t.Fatal("restored without a notebook")
	}
	if _, err = d.db.Exec(`UPDATE notes SET is_trash=0 WHERE note_id='n'`); err == nil {
		t.Fatal("non-trash note accepted without notebook")
	}
	var fk int
	if err = d.db.QueryRow(`PRAGMA foreign_keys`).Scan(&fk); err != nil || fk != 1 {
		t.Fatalf("foreign keys disabled: %d %v", fk, err)
	}
	if _, err = d.db.Exec(`UPDATE notes SET server_note_id='remote-n' WHERE note_id='n'; INSERT INTO note_histories(note_id,content) VALUES('n','old body')`); err != nil {
		t.Fatal(err)
	}
	d.SetCurrentUser("u")
	if err = d.DeleteNoteForce("n"); err != nil {
		t.Fatal(err)
	}
	if note, err = d.GetNote("n"); err != nil || note != nil {
		t.Fatalf("permanently deleted note retained: %+v %v", note, err)
	}

}
