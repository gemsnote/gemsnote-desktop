package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gemsnote/gemsnote/db"
	"github.com/gemsnote/gemsnote/models"
)

func TestResetSyncDiscardsOnlyCurrentAccountCacheWithoutUpload(t *testing.T) {
	const userID = "507f1f77bcf86cd799439011"
	const otherID = "507f1f77bcf86cd799439012"
	uploads := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/user/getSyncState":
			w.Write([]byte(`{"Ok":true,"LastSyncUsn":3,"LastSyncTime":0}`))
		case "/api2/notebook/getSyncNotebooks", "/api2/note/getSyncNotes", "/api2/tag/getSyncTags":
			w.Write([]byte(`[]`))
		default:
			uploads++
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	database, err := db.NewInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	for _, id := range []string{userID, otherID} {
		if err := database.InsertUser(&models.User{ID: id, Username: id, Host: server.URL, Token: "token", IsActive: id == userID}); err != nil {
			t.Fatal(err)
		}
		if err := database.InsertNotebook(&models.Notebook{ID: id + "-book", NotebookID: id + "-book", UserID: id, Title: id, IsDirty: true, LocalIsNew: true}); err != nil {
			t.Fatal(err)
		}
		if err := database.InsertNote(&models.Note{ID: id + "-note", NoteID: id + "-note", NotebookID: id + "-book", UserID: id, Title: id, Content: "offline", IsDirty: true, LocalIsNew: true}); err != nil {
			t.Fatal(err)
		}
	}
	app := NewApp(database)
	app.files.SetDataDir(t.TempDir())
	currentFile := filepath.Join(app.files.GetUserDir(userID), "offline.txt")
	otherFile := filepath.Join(app.files.GetUserDir(otherID), "keep.txt")
	for _, file := range []string{currentFile, otherFile} {
		if err := os.MkdirAll(filepath.Dir(file), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte("cache"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	result := app.ResetSync()
	if result["Ok"] != true {
		t.Fatalf("reset sync failed: %+v", result)
	}
	if uploads != 0 {
		t.Fatalf("reset sync uploaded %d local edits", uploads)
	}
	if note, _ := database.GetNote(userID + "-note"); note != nil {
		t.Fatalf("current user's offline note survived reset: %+v", note)
	}
	if note, _ := database.GetNote(otherID + "-note"); note == nil {
		t.Fatal("another account's note was removed")
	}
	if _, err := os.Stat(currentFile); !os.IsNotExist(err) {
		t.Fatalf("current user's file survived reset: %v", err)
	}
	if _, err := os.Stat(otherFile); err != nil {
		t.Fatalf("another account's file was removed: %v", err)
	}
}
