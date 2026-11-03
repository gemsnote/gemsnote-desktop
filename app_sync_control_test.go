package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gemsnote/gemsnote/db"
	"github.com/gemsnote/gemsnote/models"
)

func awaitSync[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case result := <-ch:
		return result
	case <-time.After(3 * time.Second):
		t.Fatal("operation did not stop promptly")
		var zero T
		return zero
	}
}

func TestCancelFullAndResetSyncDuringNetworkWait(t *testing.T) {
	for _, mode := range []string{"full", "reset"} {
		t.Run(mode, func(t *testing.T) {
			started, stopped := make(chan struct{}, 1), make(chan struct{}, 1)
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				started <- struct{}{}
				<-r.Context().Done()
				stopped <- struct{}{}
			}))
			defer server.Close()
			database, err := db.NewInMemory()
			if err != nil {
				t.Fatal(err)
			}
			defer database.Close()
			const id = "507f1f77bcf86cd799439011"
			if err := database.InsertUser(&models.User{ID: id, Host: server.URL, Token: "test-token", IsActive: true}); err != nil {
				t.Fatal(err)
			}
			if err := database.InsertNotebook(&models.Notebook{ID: "book", NotebookID: "book", ServerNotebookID: "507f1f77bcf86cd799439022", UserID: id}); err != nil {
				t.Fatal(err)
			}
			// A canceled reset preflight must not delete even a clean cached note.
			if err := database.InsertNote(&models.Note{ID: "cached", NoteID: "cached", NotebookID: "book", ServerNoteID: "507f1f77bcf86cd799439033", UserID: id, Content: "keep"}); err != nil {
				t.Fatal(err)
			}
			app := NewApp(database)
			defer app.stopSessionSync()
			app.files.SetDataDir(t.TempDir())
			done := make(chan map[string]interface{}, 1)
			go func() {
				if mode == "full" {
					done <- app.FullSyncForce()
				} else {
					done <- app.ResetSync()
				}
			}()
			awaitSync(t, started)
			progress := app.GetSyncProgress()
			if !progress.Running || progress.RunID == 0 {
				t.Fatalf("missing running operation: %+v", progress)
			}
			if got := app.CancelSync(progress.RunID + 1); got["Cancelled"] != false {
				t.Fatalf("stale cancel affected current run: %v", got)
			}
			canceled := make(chan map[string]interface{}, 1)
			go func() { canceled <- app.CancelSync(progress.RunID) }()
			if got := awaitSync(t, canceled); got["Cancelled"] != true {
				t.Fatal(got)
			}
			awaitSync(t, stopped)
			if result := awaitSync(t, done); result["Ok"] != true || result["Cancelled"] != true {
				t.Fatalf("cancel treated as error/success: %v", result)
			}
			if app.GetSyncProgress().Running {
				t.Fatal("progress still running after cancel")
			}
			if calls.Load() != 1 {
				t.Fatalf("canceled request retried: %d", calls.Load())
			}
			if note, _ := database.GetNote("cached"); note == nil || note.Content != "keep" {
				t.Fatal("preflight cancellation removed cached note")
			}
			if !app.IsAutoSyncPaused() {
				t.Fatal("canceled sync can immediately restart automatically")
			}
		})
	}
}

func TestSessionSwitchCancelsAndJoinsOldServerRequests(t *testing.T) {
	started := make(chan struct{}, 1)
	var oldCalls, newCalls atomic.Int32
	old := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		oldCalls.Add(1)
		started <- struct{}{}
		<-r.Context().Done()
	}))
	defer old.Close()
	next := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		newCalls.Add(1)
		if r.Header.Get("Authorization") != "Bearer new-token" {
			t.Error("old credentials sent to new server")
		}
		switch r.URL.Path {
		case "/api2/user/getSyncState":
			w.Write([]byte(`{"LastSyncUsn":0}`))
		default:
			w.Write([]byte(`[]`))
		}
	}))
	defer next.Close()
	database, err := db.NewInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	const id = "507f1f77bcf86cd799439011"
	if err := database.InsertUser(&models.User{ID: id, Host: old.URL, Token: "old-token", IsActive: true}); err != nil {
		t.Fatal(err)
	}
	app := NewApp(database)
	defer app.stopSessionSync()
	app.sync.BackgroundProfileRefresh = nil
	app.SetSessionLive(true)
	done := make(chan map[string]interface{}, 1)
	go func() { done <- app.FullSyncForce() }()
	awaitSync(t, started)
	stopped := make(chan struct{}, 1)
	go func() { app.SetSessionLive(false); stopped <- struct{}{} }()
	awaitSync(t, stopped)
	if result := awaitSync(t, done); result["Cancelled"] != true {
		t.Fatal(result)
	}
	if _, err := app.beginSyncOperation("full"); err != context.Canceled {
		t.Fatalf("sync started in logged-out session: %v", err)
	}
	// Same user ID on a different host reproduces a restored server account.
	if err := database.UpdateUserHost(id, next.URL); err != nil {
		t.Fatal(err)
	}
	if err := database.UpdateUserToken(id, "new-token"); err != nil {
		t.Fatal(err)
	}
	app.SetSessionLive(true)
	if result := app.FullSyncForce(); result["Ok"] != true || result["Cancelled"] == true {
		t.Fatal(result)
	}
	if oldCalls.Load() != 1 || newCalls.Load() < 4 {
		t.Fatalf("wrong server: old=%d new=%d", oldCalls.Load(), newCalls.Load())
	}
}

func TestCancelUploadPreservesDirtyNote(t *testing.T) {
	started := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api2/note/updateNote" {
			t.Errorf("unexpected request before upload: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		_, _ = io.Copy(io.Discard, r.Body)
		started <- struct{}{}
		<-r.Context().Done()
	}))
	defer server.Close()
	database, err := db.NewInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	const id = "507f1f77bcf86cd799439011"
	if err := database.InsertUser(&models.User{ID: id, Host: server.URL, Token: "test-token", IsActive: true}); err != nil {
		t.Fatal(err)
	}
	if err := database.InsertNotebook(&models.Notebook{ID: "book", NotebookID: "book", ServerNotebookID: "507f1f77bcf86cd799439022", UserID: id}); err != nil {
		t.Fatal(err)
	}
	if err := database.InsertNote(&models.Note{ID: "note", NoteID: "note", ServerNoteID: "507f1f77bcf86cd799439033", NotebookID: "book", UserID: id, Content: "unsent edits", IsDirty: true, ContentIsDirty: true, Usn: 3}); err != nil {
		t.Fatal(err)
	}
	app := NewApp(database)
	defer app.stopSessionSync()
	finished := make(chan map[string]interface{}, 1)
	go func() { finished <- app.FullSyncForce() }()
	awaitSync(t, started)
	canceled := make(chan map[string]interface{}, 1)
	go func() { canceled <- app.CancelSync(app.GetSyncProgress().RunID) }()
	awaitSync(t, canceled)
	if result := awaitSync(t, finished); result["Cancelled"] != true {
		t.Fatal(result)
	}
	note, err := database.GetNote("note")
	if err != nil || note == nil || !note.IsDirty || !note.ContentIsDirty || note.Content != "unsent edits" {
		t.Fatal("canceled upload discarded or marked local edits as synchronized")
	}
}
