package webapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gemsnote/gemsnote/models"
	"github.com/gemsnote/gemsnote/utils"
)

func TestSavePreservesNoteFormat(t *testing.T) {
	for _, markdown := range []bool{true, false} {
		name := "rich_text"
		if markdown {
			name = "markdown"
		}
		t.Run(name, func(t *testing.T) {
			e := newTestEnv(t)
			userID, notebookID := e.login(t)
			noteID := utils.ObjectId()
			if err := e.db.InsertNote(&models.Note{ID: utils.ObjectId(), NoteID: noteID, NotebookID: notebookID, UserID: userID, Title: "original", Content: "# original", IsMarkdown: markdown}); err != nil {
				t.Fatal(err)
			}
			save := func(payload map[string]any, wantFormat bool, wantContent string) {
				t.Helper()
				payload["noteId"] = noteID
				body, err := json.Marshal(payload)
				if err != nil {
					t.Fatal(err)
				}
				req := httptest.NewRequest(http.MethodPost, "/api2/save", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				rec := httptest.NewRecorder()
				e.handler.ServeHTTP(rec, req)
				var doc struct {
					Note struct {
						NoteID     string `json:"NoteId"`
						IsMarkdown bool
					}
					Content string
				}
				if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil || doc.Note.NoteID != noteID || doc.Note.IsMarkdown != wantFormat || doc.Content != wantContent {
					t.Fatalf("save response: %s (%v)", rec.Body, err)
				}
				note, err := e.db.GetNote(noteID)
				if err != nil || note == nil || note.IsMarkdown != wantFormat || note.Content != wantContent || !note.IsDirty {
					t.Fatalf("persisted format/body mismatch: %+v (%v)", note, err)
				}
			}
			// Missing format preserves the persisted type, including title-only edits.
			save(map[string]any{"title": "edited", "content": "# edited"}, markdown, "# edited")
			save(map[string]any{"title": "renamed", "content": "# edited", "tags": "tag"}, markdown, "# edited")
			// Explicit booleans can still restore either historical format.
			save(map[string]any{"title": "rich", "content": "<p>rich</p>", "isMarkdown": false}, false, "<p>rich</p>")
			save(map[string]any{"title": "markdown", "content": "# markdown", "isMarkdown": true}, true, "# markdown")
		})
	}
}
