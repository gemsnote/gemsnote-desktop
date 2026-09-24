package webapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gemsnote/gemsnote/models"
)

func TestBackgroundAvatarUsesCapturedAccountAndNoSecondLogin(t *testing.T) {
	e := newTestEnv(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/user/info":
			if r.URL.Query().Get("token") != "captured-token" {
				t.Error("missing captured token")
			}
			w.Write([]byte(`{"UserId":"user1","Logo":"/avatar.png"}`))
		case "/avatar.png":
			w.Header().Set("Content-Type", "image/png")
			w.Write([]byte("avatar bytes"))
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	user := models.User{ID: "user1", Host: server.URL, Token: "captured-token"}
	if err := e.db.InsertUser(&user); err != nil {
		t.Fatal(err)
	}
	// Global config/current account must not reroute this captured task.
	e.db.SetConfig("host", "http://127.0.0.1:1")
	e.db.SetCurrentUser("another-user")
	if err := refreshAccountAvatar(context.Background(), user, e.db, e.handler.Files); err != nil {
		t.Fatal(err)
	}
	logo, err := e.db.GetConfig("logo:user1")
	if err != nil {
		t.Fatal(err)
	}
	img, err := e.db.GetImage(strings.TrimPrefix(logo, "/api2/file/getImage?fileId="))
	if err != nil || img == nil || img.UserID != user.ID {
		t.Fatalf("avatar saved to wrong account: %+v %v", img, err)
	}
	if body, err := os.ReadFile(img.Path); err != nil || string(body) != "avatar bytes" {
		t.Fatalf("wrong avatar body %q %v", body, err)
	}
}
