package sharedsync

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gemsnote/gemsnote/api"
	"github.com/gemsnote/gemsnote/db"
	"github.com/gemsnote/gemsnote/models"
	"github.com/gemsnote/gemsnote/service"
)

func TestSuspendCancelsRequestsBeforeAccountSwitch(t *testing.T) {
	started := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { started <- struct{}{}; <-r.Context().Done() }))
	defer server.Close()
	database, err := db.NewInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.InsertUser(&models.User{ID: testUser, Host: server.URL, Token: "test-token", IsActive: true}); err != nil {
		t.Fatal(err)
	}
	svc := New(database, api.NewClient(), service.NewFileService(database))
	defer svc.SuspendAndWait()
	finished := make(chan error, 1)
	go func() { finished <- svc.SyncOnce() }()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("shared sync did not start")
	}
	stopped := make(chan struct{}, 1)
	go func() { svc.SuspendAndWait(); stopped <- struct{}{} }()
	select {
	case <-stopped:
	case <-time.After(3 * time.Second):
		t.Fatal("shared sync did not cancel promptly")
	}
	if err := <-finished; err == nil {
		t.Fatal("canceled shared snapshot reported success")
	}
	if err := svc.SyncOnce(); err != nil {
		t.Fatal(err)
	} // suspended means no further requests
	select {
	case <-started:
		t.Fatal("suspended shared sync reached old server")
	default:
	}
}
