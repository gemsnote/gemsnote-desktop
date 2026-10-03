package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestContentPresenceDependsOnEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api2/note/getNote" {
			w.Write([]byte(`{"NoteId":"n","Content":"","Usn":2}`))
			return
		}
		w.Write([]byte(`[{"NoteId":"n","Content":"","Usn":2}]`))
	}))
	defer server.Close()
	c := NewClient()
	c.SetHost(server.URL)
	metadata, err := c.GetSyncNotes(0, 10)
	if err != nil || len(metadata) != 1 {
		t.Fatalf("metadata: %v %v", metadata, err)
	}
	if metadata[0].ContentPresent {
		t.Fatal("metadata placeholder marked as body")
	}
	note, err := c.GetNote("n")
	if err != nil || note == nil {
		t.Fatalf("note: %v", err)
	}
	if note.ContentPresent {
		t.Fatal("single-note metadata placeholder marked as body")
	}
	snapshot, err := c.GetSyncNotesWithContent(0, 10)
	if err != nil || len(snapshot) != 1 {
		t.Fatalf("snapshot: %v %v", snapshot, err)
	}
	if !snapshot[0].ContentPresent || snapshot[0].Content != "" {
		t.Fatal("valid empty snapshot body lost")
	}
}
