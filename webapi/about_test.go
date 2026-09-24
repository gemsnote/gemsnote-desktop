package webapi

import (
	"encoding/json"
	"net/http"
	"runtime"
	"testing"
)

func TestDesktopAboutReturnsLocalBuildWithoutLogin(t *testing.T) {
	e := newTestEnv(t)
	e.handler.Version = "2.3.4-test"
	status, body := e.get(t, "/api2/desktop/about")
	var info map[string]string
	if status != http.StatusOK || json.Unmarshal(body, &info) != nil {
		t.Fatalf("about must return JSON rather than the SPA: %d %s", status, body)
	}
	for key, want := range map[string]string{
		"Name": "Gemsnote", "Version": "2.3.4-test",
		"Platform": runtime.GOOS, "Arch": runtime.GOARCH, "Runtime": runtime.Version(),
	} {
		if info[key] != want {
			t.Errorf("%s = %q, want %q", key, info[key], want)
		}
	}
}
