package sync

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	stdsync "sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gemsnote/gemsnote/api"
	"github.com/gemsnote/gemsnote/db"
	"github.com/gemsnote/gemsnote/models"
)

func TestFreshSync220NotesFinishesBeforeBackgroundMedia(t *testing.T) {
	const count = 220
	const userID = "507f1f77bcf86cd799439011"
	const bookID = "507f1f77bcf86cd799439012"
	fileID := func(i int) string { return fmt.Sprintf("%024x", i+1000) }
	noteID := func(i int) string { return fmt.Sprintf("%024x", i+2000) }
	release := make(chan struct{})
	var unblock stdsync.Once
	var active, maximum, singleBodies atomic.Int32
	var countsMu stdsync.Mutex
	counts := make(map[string]int)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/user/getSyncState":
			json.NewEncoder(w).Encode(map[string]any{"LastSyncUsn": count, "LastSyncTime": 0})
		case "/api2/notebook/getSyncNotebooks":
			json.NewEncoder(w).Encode([]map[string]any{{"NotebookId": bookID, "UserId": userID, "Title": "Book", "Usn": 1}})
		case "/api2/tag/getSyncTags":
			w.Write([]byte(`[]`))
		case "/api2/note/getSyncNotesWithContent":
			after, _ := strconv.Atoi(r.URL.Query().Get("afterUsn"))
			if after < 0 {
				after = 0
			}
			limit, _ := strconv.Atoi(r.URL.Query().Get("maxEntry"))
			notes := make([]map[string]any, 0)
			for i := after; i < count && i < after+limit; i++ {
				id := fileID(i % 12)
				content := strings.Repeat("Synthetic note正文 <p>test</p>\n", 2000) + fmt.Sprintf(`<img src="/api2/file/getImage?fileId=%s"><img src="/api2/file/getImage?fileId=%s">`, id, id)
				files := []map[string]any{{"FileId": id, "IsAttach": false}}
				if i < 2 {
					files = append(files, map[string]any{"FileId": fileID(20 + i), "IsAttach": true, "Title": "same-name.txt"})
				}
				notes = append(notes, map[string]any{"NoteId": noteID(i), "NotebookId": bookID, "UserId": userID, "Content": content, "Usn": i + 1, "Files": files})
			}
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Content-Encoding", "gzip")
			gz := gzip.NewWriter(w)
			json.NewEncoder(gz).Encode(notes)
			gz.Close()
		case "/api2/note/getNoteContent":
			singleBodies.Add(1)
			t.Error("snapshot must not fetch individual bodies")
			w.Write([]byte(`{}`))
		case "/api2/file/getImage", "/api2/file/getAttach":
			id := r.URL.Query().Get("fileId")
			countsMu.Lock()
			counts[id]++
			countsMu.Unlock()
			n := active.Add(1)
			defer active.Add(-1)
			for old := maximum.Load(); n > old; old = maximum.Load() {
				if maximum.CompareAndSwap(old, n) {
					break
				}
			}
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
			if strings.HasSuffix(r.URL.Path, "getImage") {
				w.Header().Set("Content-Type", "image/png")
			} else {
				w.Header().Set("Content-Disposition", `attachment; filename="same-name.txt"`)
			}
			w.Write([]byte(id))
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	defer unblock.Do(func() { close(release) })
	database, err := db.New(filepath.Join(t.TempDir(), "cache.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err = database.InsertUser(&models.User{ID: userID, Username: "test", Host: server.URL, Token: "test", IsActive: true}); err != nil {
		t.Fatal(err)
	}
	database.SetCurrentUser(userID)
	syncer := NewSyncService(database, api.NewClient())
	syncer.files.SetDataDir(t.TempDir())
	defer syncer.StopBackgroundDownloads()
	started := time.Now()
	result := make(chan error, 1)
	go func() { _, err := syncer.FreshSync(); result <- err }()
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	// This is a deadlock guard, not a throughput assertion: the race detector
	// substantially slows JSON/SQLite work for the multi-megabyte fixture.
	case <-time.After(60 * time.Second):
		t.Fatal("note sync waited for blocked media")
	}
	t.Logf("220 synthetic notes committed while media blocked: %s", time.Since(started))
	if n, err := database.CountVisibleNotes(userID); err != nil || n != count {
		t.Fatalf("cached notes=%d err=%v", n, err)
	}
	if syncer.IsSyncing() || singleBodies.Load() != 0 {
		t.Fatal("note sync did not finish independently")
	}
	syncer.downloadMu.Lock()
	batch := syncer.downloads
	syncer.downloadMu.Unlock()
	if batch == nil {
		t.Fatal("missing background job")
	}
	waited := make(chan struct{})
	go func() { syncer.WaitForBackgroundImage(context.Background(), fileID(0)); close(waited) }()
	select {
	case <-waited:
		t.Fatal("pending image returned before being downloaded")
	case <-time.After(50 * time.Millisecond):
	}
	// Changing the foreground API host must not redirect queued background work.
	syncer.api.SetHost("http://127.0.0.1:1")
	unblock.Do(func() { close(release) })
	select {
	case <-batch.done:
	case <-time.After(10 * time.Second):
		t.Fatal("background jobs failed to complete")
	}
	<-waited
	if got := maximum.Load(); got < 2 || got > snapshotDownloadWorkers {
		t.Fatalf("download concurrency=%d", got)
	}
	countsMu.Lock()
	defer countsMu.Unlock()
	if len(counts) != 14 {
		t.Fatalf("unique media requests=%d, want 14", len(counts))
	}
	for id, n := range counts {
		if n != 1 {
			t.Errorf("file %s downloaded %d times", id, n)
		}
	}
	for i := 0; i < 12; i++ {
		img, err := database.GetImage(fileID(i))
		if err != nil || img == nil || img.UserID != userID {
			t.Fatalf("missing image %d: %+v %v", i, img, err)
		}
	}
	paths := map[string]bool{}
	for i := 0; i < 2; i++ {
		att, err := database.GetAttachByServerID(fileID(20 + i))
		if err != nil || att == nil || att.NoteID != noteID(i) || att.Title != "same-name.txt" {
			t.Fatalf("attachment mismatch: %+v %v", att, err)
		}
		if paths[att.Path] {
			t.Fatal("same-name attachments overwritten")
		}
		paths[att.Path] = true
		if body, err := os.ReadFile(att.Path); err != nil || string(body) != fileID(20+i) {
			t.Fatal("wrong attachment body")
		}
	}
}

func TestStopBackgroundDownloadsCancelsNetworkAndWrites(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(started); <-r.Context().Done() }))
	defer server.Close()
	database, err := db.NewInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	syncer := NewSyncService(database, api.NewClient())
	syncer.files.SetDataDir(t.TempDir())
	defer syncer.StopBackgroundDownloads()
	id := "507f1f77bcf86cd799439013"
	syncer.snapshotFiles = map[string]snapshotFile{"image:" + id: {id: id}}
	syncer.startSnapshotDownloads(&models.User{ID: "user", Host: server.URL, Token: "secret"})
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("download did not start")
	}
	done := make(chan struct{})
	go func() { syncer.StopBackgroundDownloads(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("cancel waited for the network timeout")
	}
	if img, err := database.GetImage(id); err != nil || img != nil {
		t.Fatalf("canceled download wrote cache: %+v %v", img, err)
	}
}
