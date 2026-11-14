package db

import (
	"fmt"
	"github.com/gemsnote/gemsnote/models"
	"strings"
	"testing"
)

func TestNoteListPagesExcludeBodiesAndKeepFilters(t *testing.T) {
	d, err := NewInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if err = d.InsertUser(&models.User{ID: "u", Username: "u"}); err != nil {
		t.Fatal(err)
	}
	if err = d.InsertNotebook(&models.Notebook{ID: "b", NotebookID: "b", UserID: "u"}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 1001; i++ {
		n := &models.Note{ID: fmt.Sprint(i), NoteID: fmt.Sprintf("n%04d", i), UserID: "u", NotebookID: "b", Title: fmt.Sprintf("Title %04d", i), Content: strings.Repeat("large body ", 1000), Tags: []string{"tag"}, IsStar: i == 1000}
		if err = d.InsertNote(n); err != nil {
			t.Fatal(err)
		}
	}
	seen := map[string]bool{}
	for page := 1; page <= 11; page++ {
		list, err := d.NoteListPage("u", "b", "", "", "Title", false, false, page, 100)
		if err != nil {
			t.Fatal(err)
		}
		for _, n := range list {
			if n.Content != "" || seen[n.NoteID] {
				t.Fatalf("body loaded or duplicate: %s", n.NoteID)
			}
			seen[n.NoteID] = true
		}
	}
	if len(seen) != 1001 {
		t.Fatalf("missing notes: %d", len(seen))
	}
	for _, filter := range []struct {
		book, key, tag, user string
		star                 bool
		want                 int
	}{{"b", "large body", "tag", "u", true, 1}, {"wrong", "", "", "u", false, 0}, {"b", "", "absent", "u", false, 0}, {"b", "", "", "other", false, 0}} {
		list, err := d.NoteListPage(filter.user, filter.book, filter.key, filter.tag, "Title", false, filter.star, 1, 100)
		if err != nil || len(list) != filter.want {
			t.Fatalf("filter result=%d want=%d err=%v", len(list), filter.want, err)
		}
	}
	n, err := d.GetNote("n1000")
	if err != nil || n == nil || n.Content == "" {
		t.Fatal("list read damaged stored body")
	}
}
