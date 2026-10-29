package webapi

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"testing"
)

func TestNativeExportFile(t *testing.T) {
	e := newTestEnv(t)
	e.login(t)
	for _, name := range []string{"珠玑.md", "富文本.rtf", "笔记本.zip"} {
		for _, cancel := range []bool{false, true} {
			called := 0
			body := "---\ntitle: 珠玑\n---\n正文\x00"
			e.handler.OnExportNote = func(filename, content, language string) (bool, error) {
				called++
				if filename != name || content != body {
					t.Errorf("export changed bytes: %q %q", filename, content)
				}
				return !cancel, nil
			}
			var result struct{ Ok, Cancelled bool }
			e.postJSON(t, "/api2/desktop/exportFile", url.Values{"filename": {name}, "data": {base64.StdEncoding.EncodeToString([]byte(body))}}, &result)
			if !result.Ok || result.Cancelled != cancel || called != 1 {
				t.Fatalf("result %+v calls %d", result, called)
			}
		}
	}
	e.handler.OnExportNote = func(string, string, string) (bool, error) { t.Fatal("invalid export opened dialog"); return false, nil }
	for _, name := range []string{"../x.md", "/tmp/x.zip", "bad.exe", ".md", "x\\y.zip"} {
		var result struct {
			Ok  bool
			Msg string
		}
		e.postJSON(t, "/api2/desktop/exportFile", url.Values{"filename": {name}, "data": {"eA=="}}, &result)
		if result.Ok || result.Msg != "invalidExportFile" {
			t.Fatalf("bad filename accepted: %s %+v", name, result)
		}
	}
	e.handler.OnExportNote = func(string, string, string) (bool, error) { return false, fmt.Errorf("disk failure") }
	var result struct {
		Ok  bool
		Msg string
	}
	e.postJSON(t, "/api2/desktop/exportFile", url.Values{"filename": {"note.md"}, "data": {"eA=="}}, &result)
	if result.Ok || result.Msg != "exportSaveFailed" {
		t.Fatalf("error hidden: %+v", result)
	}
}

func TestNativeExportFileRequiresLogin(t *testing.T) {
	e := newTestEnv(t)
	e.handler.OnExportNote = func(string, string, string) (bool, error) {
		t.Fatal("anonymous export opened dialog")
		return false, nil
	}
	var result struct {
		Ok  bool
		Msg string
	}
	e.postJSON(t, "/api2/desktop/exportFile", url.Values{"filename": {"note.md"}, "data": {"eA=="}}, &result)
	if result.Ok || result.Msg != "NOTLOGIN" {
		t.Fatalf("anonymous export: %+v", result)
	}
}
