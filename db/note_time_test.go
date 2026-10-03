package db

import (
	"github.com/gemsnote/gemsnote/models"
	"testing"
	"time"
)

func TestLocalEditAndServerConfirmationTimes(t *testing.T) {
	d, err := NewInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if err = d.InsertUser(&models.User{ID: "u", Username: "u"}); err != nil {
		t.Fatal(err)
	}
	if err = d.InsertNotebook(&models.Notebook{ID: "b", NotebookID: "b", UserID: "u", Title: "book"}); err != nil {
		t.Fatal(err)
	}
	old := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	n := &models.Note{ID: "n", NoteID: "n", ServerNoteID: "n", NotebookID: "b", UserID: "u", Content: "before", UpdatedTime: &old, Usn: 1}
	if err = d.InsertNote(n); err != nil {
		t.Fatal(err)
	}
	n.Content = "after"
	n.IsDirty = true
	n.ContentIsDirty = true
	if err = d.UpdateNote(n); err != nil {
		t.Fatal(err)
	}
	got, err := d.GetNote("n")
	if err != nil {
		t.Fatal(err)
	}
	if !got.UpdatedTime.Equal(old) || got.LocalEditedTime == nil || got.LocalEditedTime.Before(time.Now().Add(-time.Minute)) {
		t.Fatalf("time sources mixed: %+v", got)
	}
	localTime := *got.LocalEditedTime
	confirmed := old.Add(time.Hour)
	got.UpdatedTime = &confirmed
	got.Usn = 2
	if err = d.UpdateNoteAfterSync(got, false); err != nil {
		t.Fatal(err)
	}
	got, err = d.GetNote("n")
	if err != nil {
		t.Fatal(err)
	}
	if got.IsDirty || !got.UpdatedTime.Equal(confirmed) || !got.LocalEditedTime.Equal(localTime) {
		t.Fatalf("confirmation lost edit provenance: %+v", got)
	}
	// Simulate a v2 database and verify the upgrade does not discard dirty work.
	if _, err = d.db.Exec(`ALTER TABLE notes DROP COLUMN local_edited_time; UPDATE notes SET is_dirty=1; PRAGMA user_version=2;`); err != nil {
		t.Fatal(err)
	}
	if err = d.migrate(); err != nil {
		t.Fatal(err)
	}
	got, err = d.GetNote("n")
	if err != nil {
		t.Fatal(err)
	}
	if got.Content != "after" || !got.IsDirty || got.LocalEditedTime == nil {
		t.Fatalf("upgrade damaged note: %+v", got)
	}
}
