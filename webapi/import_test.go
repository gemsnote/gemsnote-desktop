package webapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gemsnote/gemsnote/utils"
)

// The shared SPA imports using the existing bridge, with no remote server.
// Verify that ZIP-style nesting and Markdown bodies persist as uploadable data.
func TestOfflineMarkdownImport(t *testing.T) {
	e := newTestEnv(t)
	userID, existing := e.login(t)
	root, folder, child, noteID := utils.ObjectId(), utils.ObjectId(), utils.ObjectId(), utils.ObjectId()
	for _, book := range []struct{ id, parent, title string }{{root, existing, "导入资料"}, {folder, root, "Folder"}, {child, folder, "子目录"}} {
		var result struct{ Ok bool }
		e.postJSON(t, "/api2/notebook/addNotebook", url.Values{"notebookId": {book.id}, "parentNotebookId": {book.parent}, "title": {book.title}}, &result)
		if !result.Ok {
			t.Fatalf("create imported notebook %q failed", book.title)
		}
	}
	content := "# 你好\n\n**Markdown** 正文\n"
	payload, err := json.Marshal(map[string]any{"noteId": noteID, "notebookId": child, "title": "我的笔记", "content": content, "tags": "", "isNew": true, "isMarkdown": true})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api2/save", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.handler.ServeHTTP(rec, req)
	var result struct {
		Note struct {
			NoteID string `json:"NoteId"`
		}
		Content string
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil || result.Note.NoteID != noteID || result.Content != content {
		t.Fatalf("import note response: %s (%v)", rec.Body, err)
	}
	note, err := e.db.GetNote(noteID)
	if err != nil || note == nil || note.UserID != userID || note.NotebookID != child || !note.IsMarkdown || !note.IsDirty || !note.LocalIsNew || note.Content != content {
		t.Fatalf("imported local note: %+v (%v)", note, err)
	}
	for id, parent := range map[string]string{root: existing, folder: root, child: folder} {
		book, err := e.db.GetNotebook(id)
		if err != nil || book == nil || book.ParentNotebookID != parent || !book.IsDirty || !book.LocalIsNew {
			t.Fatalf("imported local notebook: %+v (%v)", book, err)
		}
	}
	if count, err := e.db.CountNotes(child); err != nil || count != 1 {
		t.Fatalf("imported notebook count = %d, %v", count, err)
	}
	if pending, err := e.db.HasPendingChanges(userID); err != nil || !pending {
		t.Fatalf("imported data must be pending upload: %v, %v", pending, err)
	}
	// A retry with the same ID cannot overwrite an already imported note.
	retry := httptest.NewRequest(http.MethodPost, "/api2/save", bytes.NewReader(payload))
	retry.Header.Set("Content-Type", "application/json")
	retryRec := httptest.NewRecorder()
	e.handler.ServeHTTP(retryRec, retry)
	if !bytes.Contains(retryRec.Body.Bytes(), []byte("conflict")) {
		t.Fatalf("duplicate import must fail safely: %s", retryRec.Body)
	}
}
