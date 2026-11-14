package webapi

import (
	"bytes"
	"encoding/json"
	"github.com/gemsnote/gemsnote/models"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNotebookReorderPreservesArrayOrderAndRejectsOtherParents(t *testing.T) {
	e := newTestEnv(t)
	user, first := e.login(t)
	for _, nb := range []*models.Notebook{{ID: "z", NotebookID: "z-book", UserID: user, Title: "Z"}, {ID: "a", NotebookID: "a-book", UserID: user, Title: "A"}, {ID: "child", NotebookID: "child", UserID: user, ParentNotebookID: first, Title: "Child"}} {
		if err := e.db.InsertNotebook(nb); err != nil {
			t.Fatal(err)
		}
	}
	send := func(ids []string) bool {
		data, _ := json.Marshal(map[string]any{"notebookIds": ids})
		req := httptest.NewRequest(http.MethodPost, "/api2/notebook/reorder", bytes.NewReader(data))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		e.handler.ServeHTTP(rec, req)
		var result struct{ Ok bool }
		if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result.Ok
	}
	if !send([]string{"z-book", first, "a-book"}) {
		t.Fatal("reorder failed")
	}
	for i, id := range []string{"z-book", first, "a-book"} {
		nb, _ := e.db.GetNotebook(id)
		if nb.Seq != i || !nb.IsDirty {
			t.Fatalf("order was sorted by ID: %+v", nb)
		}
	}
	for _, ids := range [][]string{{"a-book", "child"}, {"a-book", "a-book"}, {"a-book", "missing"}} {
		if send(ids) {
			t.Fatalf("invalid order accepted: %v", ids)
		}
	}
	nb, _ := e.db.GetNotebook("a-book")
	if nb.Seq != 2 {
		t.Fatal("failed order partially committed")
	}
}
