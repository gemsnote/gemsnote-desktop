package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gemsnote/gemsnote/models"
)

func TestNoteUploadsUseDedicatedLongTimeout(t *testing.T) {
	client := NewClient()
	if got := client.client.GetClient().Timeout; got != defaultRequestTimeout {
		t.Fatalf("default timeout = %v, want %v", got, defaultRequestTimeout)
	}
	if got := client.uploadClient.GetClient().Timeout; got != noteUploadTimeout {
		t.Fatalf("note upload timeout = %v, want %v", got, noteUploadTimeout)
	}
	if got := client.contentClient.GetClient().Timeout; got != noteContentTimeout {
		t.Fatalf("note content timeout = %v, want %v", got, noteContentTimeout)
	}
}

func TestGetNoteContentRetriesTransientServerFailure(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls < 3 {
			http.Error(w, "temporary", http.StatusGatewayTimeout)
			return
		}
		w.Write([]byte(`{"Ok":true,"NoteId":"note1","Content":"body"}`))
	}))
	defer server.Close()
	client := NewClient()
	client.SetHost(server.URL)
	content, err := client.GetNoteContent("note1")
	if err != nil || content != "body" || calls != 3 {
		t.Fatalf("content=%q calls=%d err=%v", content, calls, err)
	}
}

func TestClientErrorsRedactToken(t *testing.T) {
	client := NewClient()
	client.SetToken("secret-token")
	err := client.redactError(errors.New(`Post "https://example.test/api2/note/addNote?token=secret-token": timeout`))
	if strings.Contains(err.Error(), "secret-token") || !strings.Contains(err.Error(), "[redacted]") {
		t.Fatalf("token was not redacted: %v", err)
	}
}

func TestAddNoteSendsStableIDAndOriginalTimes(t *testing.T) {
	created := time.Date(2020, 2, 3, 4, 5, 6, 0, time.UTC)
	updated := created.Add(2 * time.Hour)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		for key, want := range map[string]string{
			"ClientNoteId": "507f1f77bcf86cd799439011",
			"CreatedTime":  created.Format(time.RFC3339Nano),
			"UpdatedTime":  updated.Format(time.RFC3339Nano),
		} {
			if got := r.Form.Get(key); got != want {
				t.Errorf("%s = %q, want %q", key, got, want)
			}
		}
		w.Write([]byte(`{"NoteId":"507f1f77bcf86cd799439011","NotebookId":"507f191e810c19729de860ea","Usn":1}`))
	}))
	defer server.Close()

	client := NewClient()
	client.SetHost(server.URL)
	_, err := client.AddNote(&models.Note{
		NoteID: "507f1f77bcf86cd799439011", NotebookID: "507f191e810c19729de860ea",
		Title: "original", CreatedTime: &created, UpdatedTime: &updated,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestLastSyncStateTimeAcceptsStringAndNumber(t *testing.T) {
	for _, tc := range []struct {
		body string
		want string
	}{
		{`{"Ok":true,"LastSyncUsn":7,"LastSyncTime":"2026-09-13T00:00:00Z"}`, "2026-09-13T00:00:00Z"},
		{`{"Ok":true,"LastSyncUsn":7,"LastSyncTime":1694567890}`, "1694567890"},
	} {
		var state LastSyncStateResponse
		if err := json.Unmarshal([]byte(tc.body), &state); err != nil {
			t.Fatalf("unmarshal %s: %v", tc.body, err)
		}
		if state.LastSyncTime != tc.want || state.LastSyncUsn != 7 || !state.Ok {
			t.Fatalf("state = %+v, want time %q", state, tc.want)
		}
	}
}

func TestCheckAPIResponseRejectsFailedServerResponse(t *testing.T) {
	if err := checkAPIResponse(&APIResponse{Ok: false, Msg: "conflict"}, "update note"); err == nil {
		t.Fatal("expected failed API response to be returned as an error")
	}
	if err := checkAPIResponse(&APIResponse{Ok: true}, "update note"); err != nil {
		t.Fatalf("unexpected error for successful API response: %v", err)
	}
}

func TestSyncListRejectsErrorEnvelope(t *testing.T) {
	if err := checkSyncListResponse(200, []byte(`{"Ok":false,"Msg":"NOTLOGIN"}`)); err == nil {
		t.Fatal("authentication error must not be treated as an empty sync page")
	}
	if err := checkSyncListResponse(200, []byte(`[]`)); err != nil {
		t.Fatalf("empty sync page should be valid: %v", err)
	}
}

func TestDecodeNoteResponseSupportsLeanoteDirectAndWrappedResponses(t *testing.T) {
	for _, body := range []string{
		`{"NoteId":"507f1f77bcf86cd799439011","Title":"direct","Usn":4}`,
		`{"Ok":true,"Note":{"NoteId":"507f1f77bcf86cd799439011","Title":"wrapped","Usn":5}}`,
	} {
		note, err := decodeNoteResponse([]byte(body), "update note")
		if err != nil || note == nil || note.NoteID == "" {
			t.Fatalf("decode %s: note=%+v err=%v", body, note, err)
		}
	}
	if _, err := decodeNoteResponse([]byte(`{"Ok":false,"Msg":"conflict"}`), "update note"); err == nil {
		t.Fatal("expected wrapped failure to be returned as an error")
	}
}

func TestDecodeNotebookResponseSupportsDirectResponse(t *testing.T) {
	notebook, err := decodeNotebookResponse([]byte(`{"NotebookId":"507f1f77bcf86cd799439011","Title":"direct"}`), "update notebook")
	if err != nil || notebook == nil || notebook.NotebookID == "" {
		t.Fatalf("decode notebook: notebook=%+v err=%v", notebook, err)
	}
}

func TestFlattenFormDataUsesLeanoteFieldNames(t *testing.T) {
	data := map[string]interface{}{
		"NoteId": "note-1",
		"Tags":   []string{"one", "two"},
		"Files":  []map[string]interface{}{{"LocalFileId": "file-1", "HasBody": true}},
	}
	form := flattenFormData(data)
	for key, want := range map[string]string{
		"NoteId":                "note-1",
		"Tags[0]":               "one",
		"Tags[1]":               "two",
		"Files[0][LocalFileId]": "file-1",
		"Files[0][HasBody]":     "true",
	} {
		if form[key] != want {
			t.Fatalf("form[%q] = %q, want %q (all=%v)", key, form[key], want, form)
		}
	}
}
