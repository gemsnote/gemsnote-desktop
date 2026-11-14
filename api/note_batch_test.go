package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNoteContentBatchesSplitOnBudgetAndPreserveEmptyBody(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "POST" || r.URL.Path != "/api2/note/getNoteContents" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		var p struct {
			IDs []string `json:"noteIds"`
		}
		json.NewDecoder(r.Body).Decode(&p)
		if len(p.IDs) > 1 {
			w.WriteHeader(413)
			return
		}
		json.NewEncoder(w).Encode([]map[string]any{{"NoteId": p.IDs[0], "Content": "", "Usn": 3}})
	}))
	defer server.Close()
	client := NewClient()
	client.SetHost(server.URL)
	notes, err := client.GetNoteContents([]string{"a", "b"})
	if err != nil || len(notes) != 2 || calls != 3 {
		t.Fatalf("notes=%v calls=%d err=%v", notes, calls, err)
	}
	for _, n := range notes {
		if !n.ContentPresent || n.Content != "" || n.Usn != 3 {
			t.Fatalf("empty body lost: %+v", n)
		}
	}
}
func TestNoteContentBatchRejectsIncompleteOrForeignResponse(t *testing.T) {
	for _, body := range []string{`[]`, `[{"NoteId":"other","Content":"x"}]`, `[{"NoteId":"a"}]`} {
		t.Run(body, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(body)) }))
			defer s.Close()
			c := NewClient()
			c.SetHost(s.URL)
			if _, err := c.GetNoteContents([]string{"a"}); err == nil {
				t.Fatal("accepted incomplete/foreign body")
			}
		})
	}
}
func TestNoteContentBatchHonorsCancellation(t *testing.T) {
	c := NewClient()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c.SetContext(ctx)
	if _, err := c.GetNoteContents([]string{"a"}); err != context.Canceled {
		t.Fatalf("canceled read: %v", err)
	}
}
