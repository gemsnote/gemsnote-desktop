package webapi

import (
	"encoding/json"
	"github.com/gemsnote/gemsnote/models"
	"github.com/gemsnote/gemsnote/utils"
	"net/url"
	"testing"
)

func TestSaveTagsUpdatesBootstrapAndIndex(t *testing.T) {
	e := newTestEnv(t)
	userID, bookID := e.login(t)
	noteID := utils.ObjectId()
	_, body := e.post(t, "/api2/save", url.Values{"noteId": {noteID}, "notebookId": {bookID}, "title": {"note"}, "tags": {"new"}, "isNew": {"true"}})
	var doc struct{ Note struct{ NoteId string } }
	if err := json.Unmarshal(body, &doc); err != nil || doc.Note.NoteId != noteID {
		t.Fatalf("save failed: %s", body)
	}
	tag, err := e.db.GetTag(userID, "new")
	if err != nil || tag == nil {
		t.Fatalf("tag index missing: %v %v", tag, err)
	}
	// Existing notes may have no tag-index entry, including in offline caches.
	otherUserID := utils.ObjectId()
	if err := e.db.InsertUser(&models.User{ID: otherUserID, Username: "other", Email: "other@gemsnote.test"}); err != nil {
		t.Fatal(err)
	}
	for _, note := range []*models.Note{
		{NoteID: utils.ObjectId(), UserID: userID, NotebookID: bookID, Tags: []string{"recovered", "recovered", "new", ""}},
		{NoteID: utils.ObjectId(), UserID: userID, NotebookID: bookID, Tags: []string{"trash"}, IsTrash: true},
		{NoteID: utils.ObjectId(), UserID: userID, NotebookID: bookID, Tags: []string{"deleted"}, LocalIsDelete: true},
		{NoteID: utils.ObjectId(), UserID: otherUserID, NotebookID: bookID, Tags: []string{"other-user"}},
	} {
		note.ID = utils.ObjectId()
		if err := e.db.InsertNote(note); err != nil {
			t.Fatal(err)
		}
	}
	assertCounts := func(want map[string]int) {
		t.Helper()
		_, body := e.get(t, "/api2/bootstrap")
		var result struct{ Tags []models.Tag }
		if err := json.Unmarshal(body, &result); err != nil {
			t.Fatal(err)
		}
		if len(result.Tags) != len(want) {
			t.Fatalf("unexpected tags: %s", body)
		}
		for _, tag := range result.Tags {
			if want[tag.Tag] != tag.Count {
				t.Fatalf("unexpected count: %+v", tag)
			}
		}
	}
	assertCounts(map[string]int{"new": 2, "recovered": 1})
	e.post(t, "/api2/save", url.Values{"noteId": {noteID}, "title": {"note"}, "tags": {"replacement"}})
	assertCounts(map[string]int{"new": 1, "recovered": 1, "replacement": 1})
}
