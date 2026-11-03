package webapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gemsnote/gemsnote/models"
	"github.com/gemsnote/gemsnote/utils"
)

// Both creation and modification times must reach the shared frontend unchanged;
// its ascending/descending menu cannot sort correctly using only modification time.
func TestNotesPreserveCreationTimesForSharedFrontendSorting(t *testing.T) {
	e := newTestEnv(t)
	userID, notebookID := e.login(t)
	older := time.Date(2020, 1, 1, 1, 0, 0, 0, time.UTC)
	newer := older.Add(time.Hour)
	for _, note := range []*models.Note{
		{ID: utils.ObjectId(), NoteID: utils.ObjectId(), UserID: userID, NotebookID: notebookID, Title: "Older", CreatedTime: &older, UpdatedTime: &newer},
		{ID: utils.ObjectId(), NoteID: utils.ObjectId(), UserID: userID, NotebookID: notebookID, Title: "Newer", CreatedTime: &newer, UpdatedTime: &older},
	} {
		if err := e.db.InsertNote(note); err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest(http.MethodPost, "/api2/notes", strings.NewReader(`{"sort":"CreatedTime"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.handler.ServeHTTP(rec, req)
	var notes []struct {
		Title       string
		CreatedTime time.Time
		UpdatedTime time.Time
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("notes status=%d body=%s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &notes); err != nil {
		t.Fatal(err)
	}
	if len(notes) != 2 || notes[0].Title != "Newer" || notes[1].Title != "Older" {
		t.Fatalf("creation order lost: %+v", notes)
	}
	if !notes[0].CreatedTime.Equal(newer) || !notes[0].UpdatedTime.Equal(older) ||
		!notes[1].CreatedTime.Equal(older) || !notes[1].UpdatedTime.Equal(newer) {
		t.Fatalf("creation/modification times changed: %+v", notes)
	}
}
