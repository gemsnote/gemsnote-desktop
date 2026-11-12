package webapi

import (
	"github.com/gemsnote/gemsnote/models"
	"github.com/gemsnote/gemsnote/utils"
	"net/url"
	"testing"
)

func TestDeleteNotebookWithNotesRequiresExplicitOptIn(t *testing.T) {
	e := newTestEnv(t)
	user, book := e.login(t)
	id := utils.ObjectId()
	if err := e.db.InsertNote(&models.Note{ID: id, NoteID: id, UserID: user, NotebookID: book, Title: "Body", Content: "keep", ServerNoteID: id, Usn: 1}); err != nil {
		t.Fatal(err)
	}
	var result struct {
		Ok  bool
		Msg string
	}
	e.postJSON(t, "/api2/notebook/deleteNotebook", url.Values{"notebookId": {book}}, &result)
	if result.Ok {
		t.Fatal("non-empty notebook deleted without opt-in")
	}
	note, _ := e.db.GetNote(id)
	if note.IsTrash {
		t.Fatal("note trashed without opt-in")
	}
	e.postJSON(t, "/api2/notebook/deleteNotebook", url.Values{"notebookId": {book}, "deleteNotes": {"true"}}, &result)
	if !result.Ok {
		t.Fatal(result.Msg)
	}
	note, _ = e.db.GetNote(id)
	if !note.IsTrash || !note.IsDirty || note.Content != "keep" || !e.db.NotebookDeleteNotes(book) {
		t.Fatalf("incorrect local deletion: %+v", note)
	}
}
