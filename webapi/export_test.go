package webapi

import (
	"fmt"
	"net/url"
	"testing"

	"github.com/gemsnote/gemsnote/models"
)

func TestNativeExportUsesAuthorizedCachedNoteAndHandlesCancel(t *testing.T) {
	e := newTestEnv(t)
	user, book := e.login(t)
	for _, md := range []bool{true, false} {
		id := fmt.Sprint(md)
		body := "# 原文\n"
		extension := ".md"
		if !md {
			body = "<p>原文</p>"
			extension = ".html"
		}
		if err := e.db.InsertNote(&models.Note{ID: id, NoteID: id, UserID: user, NotebookID: book, Title: "目录/笔记", Content: body, IsMarkdown: md}); err != nil {
			t.Fatal(err)
		}
		for _, cancel := range []bool{false, true} {
			called := 0
			e.handler.OnExportNote = func(filename, content, language string) (bool, error) {
				called++
				if filename != "目录_笔记"+extension || content != body {
					t.Errorf("unsafe or wrong export: %q %q", filename, content)
				}
				return !cancel, nil
			}
			var result struct{ Ok, Cancelled bool }
			e.postJSON(t, "/api2/desktop/export", url.Values{"noteId": {id}, "content": {"untrusted body"}, "path": {"/untrusted/path"}}, &result)
			if !result.Ok || result.Cancelled != cancel || called != 1 {
				t.Fatalf("export result: %+v calls=%d", result, called)
			}
		}
	}
	e.handler.OnExportNote = func(string, string, string) (bool, error) {
		t.Fatal("unauthorized export opened dialog")
		return false, nil
	}
	var denied struct {
		Ok  bool
		Msg string
	}
	e.postJSON(t, "/api2/desktop/export", url.Values{"noteId": {"not-owned"}}, &denied)
	if denied.Ok || denied.Msg != "noAuth" {
		t.Fatalf("unauthorized export: %+v", denied)
	}
	e.handler.OnExportNote = func(string, string, string) (bool, error) { return false, fmt.Errorf("disk failure") }
	e.postJSON(t, "/api2/desktop/export", url.Values{"noteId": {"true"}}, &denied)
	if denied.Ok || denied.Msg != "exportSaveFailed" {
		t.Fatalf("failed export hidden: %+v", denied)
	}
}
