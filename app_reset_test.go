package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

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
		case "/api2/user/info":
			w.Write([]byte(`{"UserId":"507f1f77bcf86cd799439011","Logo":""}`))
		case "/api2/notebook/getSyncNotebooks", "/api2/note/getSyncNotesWithContent", "/api2/tag/getSyncTags":
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
	defer app.sync.StopBackgroundDownloads()
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
	if initial := app.InitialSync(); initial["Ok"] != false || initial["Msg"] != "confirmationRequired" {
		t.Fatalf("automatic initial sync must not delete an existing cache: %v", initial)
	}
	if note, _ := database.GetNote(userID + "-note"); note == nil || note.Content != "offline" {
		t.Fatal("unconfirmed reset altered the cached note")
	}
	if _, err := os.Stat(currentFile); err != nil {
		t.Fatalf("unconfirmed reset removed the cached file: %v", err)
	}
	if progress := app.GetSyncProgress(); progress.Running {
		t.Fatalf("rejected reset left progress running: %+v", progress)
	}
	result := app.ResetSync()
	if result["Ok"] != true {
		t.Fatalf("reset sync failed: %+v", result)
	}
	if progress := app.GetSyncProgress(); progress.Running || progress.Stage != "done" || progress.Mode != "reset" {
		t.Fatalf("completed reset has incorrect progress: %+v", progress)
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

func TestInitialSyncProgressWhilePreflightIsPending(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	var once, unblock sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api2/user/getSyncState" {
			once.Do(func() { close(started); <-release })
			w.Write([]byte(`{"LastSyncUsn":0,"LastSyncTime":0}`))
			return
		}
		w.Write([]byte(`[]`))
	}))
	defer server.Close()
	defer unblock.Do(func() { close(release) })
	database, err := db.NewInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.InsertUser(&models.User{ID: "507f1f77bcf86cd799439011", Username: "tester", Host: server.URL, Token: "test", IsActive: true}); err != nil {
		t.Fatal(err)
	}
	app := NewApp(database)
	defer app.sync.StopBackgroundDownloads()
	app.files.SetDataDir(t.TempDir())
	result := make(chan map[string]interface{}, 1)
	go func() { result <- app.InitialSync() }()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("initial sync did not reach preflight")
	}
	progress := app.GetSyncProgress()
	if !progress.Running || progress.Mode != "reset" || progress.Stage != "checking" {
		t.Errorf("pending preflight must be visible before any Wails event: %+v", progress)
	}
	unblock.Do(func() { close(release) })
	select {
	case got := <-result:
		if got["Ok"] != true {
			t.Fatalf("fresh account failed to sync: %+v", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("initial sync did not finish")
	}
	if progress := app.GetSyncProgress(); progress.Running || progress.Stage != "done" {
		t.Fatalf("finished sync has stale progress: %+v", progress)
	}
}
