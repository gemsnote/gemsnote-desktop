package sync

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gemsnote/gemsnote/api"
	"github.com/gemsnote/gemsnote/db"
	"github.com/gemsnote/gemsnote/models"
)

func TestIncrementalSyncUploadsDirtyDataBeforePulling(t *testing.T) {
	var order []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/client/notebook/add":
			order = append(order, "push")
			w.Write([]byte(`{"NotebookId":"remote-book","Title":"Local","Usn":1}`))
		case "/api2/user/getSyncState":
			w.Write([]byte(`{"LastSyncUsn":1,"LastSyncTime":0}`))
		case "/api2/notebook/getSyncNotebooks":
			order = append(order, "pull")
			w.Write([]byte(`[]`))
		case "/api2/note/getSyncNotes", "/api2/tag/getSyncTags":
			w.Write([]byte(`[]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	database, err := db.NewInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.InsertUser(&models.User{ID: "user1", Username: "tester", Host: server.URL, Token: "token", IsActive: true}); err != nil {
		t.Fatal(err)
	}
	database.SetCurrentUser("user1")
	if err := database.InsertNotebook(&models.Notebook{ID: "book", NotebookID: "local-book", UserID: "user1", Title: "Local", IsDirty: true, LocalIsNew: true}); err != nil {
		t.Fatal(err)
	}

	syncer := NewSyncService(database, api.NewClient())
	syncer.SetProgressCallback(func(progress models.SyncProgress) {
		t.Errorf("incremental sync must not open the full-sync modal: %+v", progress)
	})
	if _, err := syncer.IncrSync(); err != nil {
		t.Fatal(err)
	}
	if len(order) < 2 || order[0] != "push" || order[1] != "pull" {
		t.Fatalf("sync order = %v, want push before pull", order)
	}
}

func TestFreshSyncUsesPagedContentSnapshot(t *testing.T) {
	contentCalls, singleContentCalls := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/user/getSyncState":
			w.Write([]byte(`{"Ok":true,"LastSyncUsn":2,"LastSyncTime":0}`))
		case "/api2/notebook/getSyncNotebooks":
			w.Write([]byte(`[{"NotebookId":"remote-book","UserId":"user1","Title":"Book","Usn":1}]`))
		case "/api2/note/getSyncNotesWithContent":
			contentCalls++
			if got := r.URL.Query().Get("maxEntry"); got != "20" {
				t.Errorf("snapshot maxEntry=%q, want 20", got)
			}
			var notes []map[string]any
			start, end := 0, 20
			if r.URL.Query().Get("afterUsn") != "-1" {
				start, end = 20, 21
			}
			for i := start; i < end; i++ {
				id := fmt.Sprintf("remote-note-%d", i)
				if i == 0 {
					id = "remote-note"
				}
				notes = append(notes, map[string]any{"NoteId": id, "NotebookId": "remote-book", "UserId": "user1", "Title": "Note", "Content": "snapshot body", "Usn": i + 2})
			}
			json.NewEncoder(w).Encode(notes)
		case "/api2/note/getNoteContent":
			singleContentCalls++
			w.Write([]byte(`{"Ok":true,"Content":"unexpected"}`))
		case "/api2/tag/getSyncTags":
			w.Write([]byte(`[]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	database, err := db.NewInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.InsertUser(&models.User{ID: "user1", Username: "tester", Host: server.URL, Token: "token", IsActive: true}); err != nil {
		t.Fatal(err)
	}
	database.SetCurrentUser("user1")
	syncer := NewSyncService(database, api.NewClient())
	var progressEvents []models.SyncProgress
	syncer.SetProgressCallback(func(progress models.SyncProgress) { progressEvents = append(progressEvents, progress) })
	if _, err := syncer.FreshSync(); err != nil {
		t.Fatal(err)
	}
	note, err := database.GetNoteByServerID("remote-note")
	if err != nil || note == nil || note.Content != "snapshot body" || note.InitSync {
		t.Fatalf("snapshot note not cached: note=%+v err=%v", note, err)
	}
	if contentCalls != 2 || singleContentCalls != 0 {
		t.Fatalf("snapshot calls=%d single content calls=%d", contentCalls, singleContentCalls)
	}
	completed := 0
	for _, progress := range progressEvents {
		if progress.Stage == "notes" && progress.Current > 0 {
			completed++
			if progress.Current != completed || progress.Total != 0 {
				t.Fatalf("per-note progress must span pages without an invented total: %+v", progress)
			}
		}
		if progress.Stage != "done" && progress.Percent >= 100 {
			t.Fatalf("sync reached 100%% before completion: %+v", progress)
		}
	}
	if completed != 21 || progressEvents[len(progressEvents)-1].Stage != "done" {
		t.Fatalf("incomplete progress: %d notes, events=%+v", completed, progressEvents)
	}
}

func TestFullSyncRepushesMissingDesktopNote(t *testing.T) {
	addCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/user/getSyncState":
			w.Write([]byte(`{"LastSyncUsn":10,"LastSyncTime":0}`))
		case "/api2/notebook/getSyncNotebooks":
			json.NewEncoder(w).Encode([]map[string]any{{"NotebookId": "remote-book", "UserId": "user1", "Title": "Book", "Usn": 1}})
		case "/api2/note/getSyncNotes", "/api2/tag/getSyncTags":
			w.Write([]byte(`[]`))
		case "/api2/note/updateNote":
			w.Write([]byte(`{"Ok":false,"Msg":"noteIdNotExists"}`))
		case "/api2/note/addNote":
			addCalls++
			if err := r.ParseForm(); err != nil || r.Form.Get("NotebookId") != "remote-book" {
				t.Errorf("wrong notebook sent to server: %v %v", r.Form, err)
			}
			w.Write([]byte(`{"NoteId":"remote-new","NotebookId":"remote-book","Usn":11,"Title":"Offline"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	database, err := db.NewInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.InsertUser(&models.User{ID: "user1", Username: "tester", Host: server.URL, Token: "token", IsActive: true}); err != nil {
		t.Fatal(err)
	}
	database.SetCurrentUser("user1")
	if err := database.InsertNotebook(&models.Notebook{ID: "b", NotebookID: "local-book", ServerNotebookID: "remote-book", UserID: "user1", Usn: 1}); err != nil {
		t.Fatal(err)
	}
	if err := database.InsertNote(&models.Note{ID: "n", NoteID: "local-note", ServerNoteID: "gone-note", NotebookID: "local-book", UserID: "user1", Title: "Offline", Content: "body", Usn: 9}); err != nil {
		t.Fatal(err)
	}
	if err := NewSyncService(database, api.NewClient()).ForceFullSync(); err != nil {
		t.Fatal(err)
	}
	note, err := database.GetNote("local-note")
	if err != nil || note.ServerNoteID != "remote-new" || note.IsDirty || addCalls != 1 {
		t.Fatalf("note not repushed: note=%+v calls=%d err=%v", note, addCalls, err)
	}
}

func TestIncrementalSyncResolvesUploadConflict(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/note/updateNote":
			w.Write([]byte(`{"Ok":false,"Msg":"conflict"}`))
		case "/api2/note/getNote":
			w.Write([]byte(`{"NoteId":"remote-note","NotebookId":"remote-book","UserId":"user1","Title":"Remote title","Usn":8}`))
		case "/api2/note/getNoteContent":
			w.Write([]byte(`{"Ok":true,"NoteId":"remote-note","Content":"remote body"}`))
		case "/api2/note/addNote":
			w.Write([]byte(`{"NoteId":"conflict-copy","NotebookId":"remote-book","UserId":"user1","Title":"Local title","Usn":9}`))
		case "/api2/user/getSyncState":
			w.Write([]byte(`{"LastSyncUsn":8,"LastSyncTime":0}`))
		case "/api2/notebook/getSyncNotebooks", "/api2/note/getSyncNotes", "/api2/tag/getSyncTags":
			w.Write([]byte(`[]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	database, err := db.NewInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.InsertUser(&models.User{ID: "user1", Username: "tester", Host: server.URL, Token: "token", IsActive: true}); err != nil {
		t.Fatal(err)
	}
	database.SetCurrentUser("user1")
	if err := database.InsertNotebook(&models.Notebook{ID: "b", NotebookID: "local-book", ServerNotebookID: "remote-book", UserID: "user1"}); err != nil {
		t.Fatal(err)
	}
	if err := database.InsertNote(&models.Note{ID: "n", NoteID: "local-note", ServerNoteID: "remote-note", NotebookID: "local-book", UserID: "user1", Title: "Local title", Content: "local body", Usn: 7, IsDirty: true, ContentIsDirty: true}); err != nil {
		t.Fatal(err)
	}

	info, err := NewSyncService(database, api.NewClient()).IncrSync()
	if err != nil {
		t.Fatal(err)
	}
	if len(info.Note.Conflicts) != 1 {
		t.Fatalf("conflicts = %d, want 1", len(info.Note.Conflicts))
	}
	original, _ := database.GetNote("local-note")
	if original == nil || original.IsDirty || original.Usn != 8 || original.Title != "Remote title" || original.Content != "remote body" {
		t.Fatalf("original note was not refreshed from server: %+v", original)
	}
	copyNote := info.Note.Conflicts[0].ConflictCopy
	if copyNote == nil || copyNote.Content != "local body" {
		t.Fatalf("local edit was not preserved as conflict copy: %+v", copyNote)
	}
	uploadedCopy, _ := database.GetNote(copyNote.NoteID)
	if uploadedCopy == nil || uploadedCopy.IsDirty || uploadedCopy.LocalIsNew || uploadedCopy.ServerNoteID != "conflict-copy" {
		t.Fatalf("conflict copy was not uploaded: %+v", uploadedCopy)
	}
}

func TestUploadConflictUsesServerMetadataWhenBodyMatches(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/note/updateNote":
			w.Write([]byte(`{"Ok":false,"Msg":"conflict"}`))
		case "/api2/note/getNote":
			w.Write([]byte(`{"NoteId":"remote-note","NotebookId":"remote-book-b","UserId":"user1","Title":"Server title","Tags":["server"],"IsStar":true,"Usn":8}`))
		case "/api2/note/getNoteContent":
			w.Write([]byte(`{"Ok":true,"NoteId":"remote-note","Content":"same body"}`))
		case "/api2/user/getSyncState":
			w.Write([]byte(`{"LastSyncUsn":8,"LastSyncTime":0}`))
		case "/api2/notebook/getSyncNotebooks", "/api2/note/getSyncNotes", "/api2/tag/getSyncTags":
			w.Write([]byte(`[]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	database, err := db.NewInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.InsertUser(&models.User{ID: "user1", Username: "tester", Host: server.URL, Token: "token", IsActive: true}); err != nil {
		t.Fatal(err)
	}
	database.SetCurrentUser("user1")
	for _, id := range []string{"a", "b"} {
		if err := database.InsertNotebook(&models.Notebook{ID: id, NotebookID: "local-book-" + id, ServerNotebookID: "remote-book-" + id, UserID: "user1", Title: id}); err != nil {
			t.Fatal(err)
		}
	}
	if err := database.InsertNote(&models.Note{ID: "n", NoteID: "local-note", ServerNoteID: "remote-note", NotebookID: "local-book-a", UserID: "user1", Title: "Local title", Tags: []string{"local"}, Content: "same body", Usn: 7, IsDirty: true}); err != nil {
		t.Fatal(err)
	}
	info, err := NewSyncService(database, api.NewClient()).IncrSync()
	if err != nil {
		t.Fatal(err)
	}
	if len(info.Note.Conflicts) != 0 {
		t.Fatalf("metadata-only conflict made a copy: %+v", info.Note.Conflicts)
	}
	note, err := database.GetNote("local-note")
	if err != nil || note == nil || note.IsDirty || note.Title != "Server title" || note.NotebookID != "local-book-b" || !note.IsStar || len(note.Tags) != 1 || note.Tags[0] != "server" {
		t.Fatalf("server metadata did not win: note=%+v err=%v", note, err)
	}
}

func TestSameUSNNoteMoveUpdatesLocalNotebook(t *testing.T) {
	database, err := db.NewInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.InsertUser(&models.User{ID: "user1", Username: "tester"}); err != nil {
		t.Fatal(err)
	}
	for _, book := range []*models.Notebook{
		{ID: "a", NotebookID: "local-a", ServerNotebookID: "remote-a", UserID: "user1"},
		{ID: "b", NotebookID: "local-b", ServerNotebookID: "remote-b", UserID: "user1"},
	} {
		if err := database.InsertNotebook(book); err != nil {
			t.Fatal(err)
		}
	}
	if err := database.InsertNote(&models.Note{ID: "n", NoteID: "local-note", ServerNoteID: "remote-note", NotebookID: "local-a", UserID: "user1", Content: "cached", Usn: 12}); err != nil {
		t.Fatal(err)
	}
	svc := NewSyncService(database, api.NewClient())
	if err := svc.processNoteSync(&models.Note{NoteID: "remote-note", NotebookID: "remote-b", Usn: 12}, models.NewSyncInfo()); err != nil {
		t.Fatal(err)
	}
	note, err := database.GetNote("local-note")
	if err != nil || note.NotebookID != "local-b" {
		t.Fatalf("notebook move not applied: note=%+v err=%v", note, err)
	}
}

func TestSameUSNRootNotebookClearsStaleParent(t *testing.T) {
	database, err := db.NewInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.InsertUser(&models.User{ID: "user1", Username: "tester"}); err != nil {
		t.Fatal(err)
	}
	if err := database.InsertNotebook(&models.Notebook{ID: "n", NotebookID: "local-root", ServerNotebookID: "remote-root", ParentNotebookID: "local-root", UserID: "user1", Title: "Root", Usn: 7}); err != nil {
		t.Fatal(err)
	}
	if err := NewSyncService(database, api.NewClient()).processNotebookSync(&models.Notebook{NotebookID: "remote-root", ParentNotebookID: "", UserID: "user1", Title: "Root", Usn: 7}, models.NewSyncInfo()); err != nil {
		t.Fatal(err)
	}
	nb, err := database.GetNotebook("local-root")
	if err != nil || nb == nil || nb.ParentNotebookID != "" {
		t.Fatalf("stale root parent retained: %+v %v", nb, err)
	}
}

func TestFullSyncMergesChangedServerDatabase(t *testing.T) {
	addedNotebook, addedNote := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/user/getSyncState":
			w.Write([]byte(`{"LastSyncUsn":3,"LastSyncTime":0}`))
		case "/api2/notebook/getSyncNotebooks":
			w.Write([]byte(`[{"NotebookId":"remote-book","UserId":"user1","Title":"Remote","Usn":1}]`))
		case "/api2/note/getSyncNotes":
			w.Write([]byte(`[{"NoteId":"remote-only","NotebookId":"remote-book","UserId":"user1","Title":"Remote note","Usn":2},{"NoteId":"old-note","IsDeleted":true,"Usn":3}]`))
		case "/api2/note/getNoteContent":
			w.Write([]byte(`{"Content":"remote body"}`))
		case "/api2/tag/getSyncTags":
			w.Write([]byte(`[]`))
		case "/api2/client/notebook/update":
			w.Write([]byte(`{"Ok":false,"Msg":"notebookIdNotExists"}`))
		case "/api2/note/updateNote":
			w.Write([]byte(`{"Ok":false,"Msg":"noteIdNotExists"}`))
		case "/api2/client/notebook/add":
			addedNotebook++
			w.Write([]byte(`{"NotebookId":"new-book","Title":"Offline","Usn":4}`))
		case "/api2/note/addNote":
			addedNote++
			if err := r.ParseForm(); err != nil || r.Form.Get("NotebookId") != "new-book" {
				t.Errorf("note sent to wrong notebook: %v %v", r.Form, err)
			}
			json.NewEncoder(w).Encode(map[string]any{"NoteId": fmt.Sprintf("new-note-%d", addedNote), "NotebookId": "new-book", "Usn": 4 + addedNote})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	database, err := db.NewInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.InsertUser(&models.User{ID: "user1", Username: "tester", Host: server.URL, Token: "token", IsActive: true}); err != nil {
		t.Fatal(err)
	}
	database.SetCurrentUser("user1")
	if err := database.InsertNotebook(&models.Notebook{ID: "b", NotebookID: "old-book", ServerNotebookID: "old-book", UserID: "user1", Title: "Offline"}); err != nil {
		t.Fatal(err)
	}
	if err := database.InsertNote(&models.Note{ID: "old", NoteID: "old-note", ServerNoteID: "old-note", NotebookID: "old-book", UserID: "user1", Title: "Old cache"}); err != nil {
		t.Fatal(err)
	}
	if err := database.InsertNote(&models.Note{ID: "local", NoteID: "local-note", ServerNoteID: "missing-note", NotebookID: "old-book", UserID: "user1", Title: "Local work", Content: "body"}); err != nil {
		t.Fatal(err)
	}
	if err := NewSyncService(database, api.NewClient()).ForceFullSync(); err != nil {
		t.Fatal(err)
	}
	if addedNotebook != 1 || addedNote != 2 {
		t.Fatalf("uploads: notebooks=%d notes=%d", addedNotebook, addedNote)
	}
	oldNote, err := database.GetNote("old-note")
	if err != nil || oldNote == nil || oldNote.ServerNoteID == "old-note" || oldNote.IsDirty || oldNote.LocalIsDelete {
		t.Fatalf("old cache not merged: %+v %v", oldNote, err)
	}
	localNote, err := database.GetNote("local-note")
	if err != nil || localNote == nil || localNote.ServerNoteID == "missing-note" || localNote.IsDirty || localNote.LocalIsDelete {
		t.Fatalf("local note not uploaded: %+v %v", localNote, err)
	}
	remoteNote, err := database.GetNoteByServerID("remote-only")
	if err != nil || remoteNote == nil || remoteNote.Content != "remote body" {
		t.Fatalf("remote-only note not downloaded: %+v %v", remoteNote, err)
	}
}

func TestIncrementalSyncDetectsServerRollback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/user/getSyncState":
			w.Write([]byte(`{"LastSyncUsn":3,"LastSyncTime":0}`))
		case "/api2/notebook/getSyncNotebooks", "/api2/note/getSyncNotes", "/api2/tag/getSyncTags":
			w.Write([]byte(`[]`))
		case "/api2/client/notebook/update":
			w.Write([]byte(`{"Ok":false,"Msg":"notebookIdNotExists"}`))
		case "/api2/note/updateNote":
			w.Write([]byte(`{"Ok":false,"Msg":"noteIdNotExists"}`))
		case "/api2/client/notebook/add":
			w.Write([]byte(`{"NotebookId":"new-book","Usn":4}`))
		case "/api2/note/addNote":
			w.Write([]byte(`{"NoteId":"new-note","NotebookId":"new-book","Usn":5}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	database, err := db.NewInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.InsertUser(&models.User{ID: "user1", Username: "tester", Host: server.URL, Token: "token", IsActive: true}); err != nil {
		t.Fatal(err)
	}
	database.SetCurrentUser("user1")
	if err := database.InsertNotebook(&models.Notebook{ID: "b", NotebookID: "old-book", ServerNotebookID: "old-book", UserID: "user1"}); err != nil {
		t.Fatal(err)
	}
	if err := database.InsertNote(&models.Note{ID: "n", NoteID: "old-note", ServerNoteID: "old-note", NotebookID: "old-book", UserID: "user1"}); err != nil {
		t.Fatal(err)
	}
	if err := database.UpdateUserSyncState("user1", map[string]int64{"last_sync_usn": 10}); err != nil {
		t.Fatal(err)
	}
	if _, err := NewSyncService(database, api.NewClient()).IncrSync(); err != nil {
		t.Fatal(err)
	}
	note, err := database.GetNote("old-note")
	if err != nil || note == nil || note.ServerNoteID != "new-note" || note.IsDirty || note.LocalIsDelete {
		t.Fatalf("rollback did not merge old cache: %+v %v", note, err)
	}
}

func TestFullSyncUploadsMissingNotebookTreeParentFirst(t *testing.T) {
	var order []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/user/getSyncState":
			w.Write([]byte(`{"LastSyncUsn":1,"LastSyncTime":0}`))
		case "/api2/notebook/getSyncNotebooks", "/api2/note/getSyncNotes", "/api2/tag/getSyncTags":
			w.Write([]byte(`[]`))
		case "/api2/client/notebook/update":
			w.Write([]byte(`{"Ok":false,"Msg":"notebookIdNotExists"}`))
		case "/api2/client/notebook/add":
			_ = r.ParseForm()
			order = append(order, r.Form.Get("title"))
			if r.Form.Get("title") == "Parent" {
				w.Write([]byte(`{"NotebookId":"new-parent","Title":"Parent","Usn":2}`))
			} else {
				if r.Form.Get("parentNotebookId") != "new-parent" {
					t.Errorf("child parent ID = %q", r.Form.Get("parentNotebookId"))
				}
				w.Write([]byte(`{"NotebookId":"new-child","ParentNotebookId":"new-parent","Title":"Child","Usn":3}`))
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	database, err := db.NewInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.InsertUser(&models.User{ID: "user1", Username: "tester", Host: server.URL, Token: "token", IsActive: true}); err != nil {
		t.Fatal(err)
	}
	database.SetCurrentUser("user1")
	for _, nb := range []*models.Notebook{
		{ID: "c", NotebookID: "old-child", ServerNotebookID: "old-child", ParentNotebookID: "old-parent", UserID: "user1", Title: "Child"},
		{ID: "p", NotebookID: "old-parent", ServerNotebookID: "old-parent", UserID: "user1", Title: "Parent"},
	} {
		if err := database.InsertNotebook(nb); err != nil {
			t.Fatal(err)
		}
	}
	if err := NewSyncService(database, api.NewClient()).ForceFullSync(); err != nil {
		t.Fatal(err)
	}
	if len(order) != 2 || order[0] != "Parent" || order[1] != "Child" {
		t.Fatalf("upload order: %v", order)
	}
	child, err := database.GetNotebook("old-child")
	if err != nil || child == nil || child.ServerNotebookID != "new-child" || child.ParentNotebookID != "old-parent" || child.IsDirty {
		t.Fatalf("child mapping: %+v %v", child, err)
	}
}
