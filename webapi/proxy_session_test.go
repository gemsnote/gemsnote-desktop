package webapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"

	"github.com/gemsnote/gemsnote/db"
	"github.com/gemsnote/gemsnote/models"
	"github.com/gemsnote/gemsnote/service"
	"github.com/gemsnote/gemsnote/utils"
)

func api2LoginResponse(userID string) string {
	return fmt.Sprintf(`{"Ok":true,"Token":"test-token","User":{"UserId":%q,"Username":"tester","Email":"tester@example.test","Logo":""},"Server":{"Name":"gemsnote","Version":"1.0.0","MinVersion":""}}`, userID)
}

func TestSharedNotebooksRestoresAndRenewsSession(t *testing.T) {
	var logins atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/auth/login":
			w.Write([]byte(api2LoginResponse("admin")))
		case "/api2/auth/session":
			logins.Add(1)
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "valid", Path: "/"})
			w.Write([]byte(`{"Ok":true}`))
		case "/api2/system/version":
			w.Write([]byte(`{"server":"gemsnote","version":"1.0.0","min_version":""}`))
		case "/api2/web/bootstrap":
			cookie, err := r.Cookie("session")
			if err != nil || cookie.Value != "valid" {
				w.Write([]byte(`{"Ok":true,"User":null}`))
				return
			}
			w.Write([]byte(`{"Ok":true,"User":{"UserId":"admin"},"SharedNotebooks":{},"IsAdmin":true}`))
		}
	}))
	defer server.Close()

	database, err := db.NewInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	database.SetConfig("host", server.URL)
	database.SetConfig("proxy:email", "admin")
	database.SetConfig("proxy:pwd", "secret")
	proxy := NewServerProxy(database, service.NewFileService(database))

	_, admin := proxy.SharedNotebooks(nil)
	if !admin || logins.Load() != 1 {
		t.Fatalf("initial session: admin=%v logins=%d", admin, logins.Load())
	}
	proxy.client.Jar, _ = newEmptyCookieJar()
	_, admin = proxy.SharedNotebooks(nil)
	if !admin || logins.Load() != 2 {
		t.Fatalf("renewed session: admin=%v logins=%d", admin, logins.Load())
	}
}

func TestAccountGroupsUsesBrowserSession(t *testing.T) {
	var tokenLogins, sessionLogins atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/auth/login":
			tokenLogins.Add(1)
			w.Write([]byte(api2LoginResponse("user1")))
		case "/api2/auth/session":
			sessionLogins.Add(1)
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "valid", Path: "/"})
			w.Write([]byte(`{"Ok":true}`))
		case "/api2/system/version":
			w.Write([]byte(`{"server":"gemsnote","version":"1.0.0"}`))
		case "/api2/groups":
			if cookie, err := r.Cookie("session"); err != nil || cookie.Value != "valid" {
				w.Write([]byte(`{"Ok":false,"Msg":"NOTLOGIN"}`))
				return
			}
			w.Write([]byte(`[]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	e := newTestEnv(t)
	e.db.SetConfig("host", server.URL)
	e.db.SetConfig("proxy:email", "tester")
	e.db.SetConfig("proxy:pwd", "secret")
	e.handler.Proxy = NewServerProxy(e.db, e.handler.Files)
	_, body := e.get(t, "/api2/groups")
	if string(body) != "[]" || tokenLogins.Load() != 1 || sessionLogins.Load() != 1 {
		t.Fatalf("account groups response=%s token logins=%d session logins=%d", body, tokenLogins.Load(), sessionLogins.Load())
	}
}

func TestAccountUpdateUsesBrowserSessionAndPreservesJSON(t *testing.T) {
	var sessionLogins atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/auth/login":
			w.Write([]byte(api2LoginResponse("user1")))
		case "/api2/auth/session":
			sessionLogins.Add(1)
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "valid", Path: "/"})
			w.Write([]byte(`{"Ok":true}`))
		case "/api2/system/version":
			w.Write([]byte(`{"server":"gemsnote","version":"1.0.0"}`))
		case "/api2/user/updateUsername":
			if cookie, err := r.Cookie("session"); err != nil || cookie.Value != "valid" {
				w.Write([]byte(`{"Ok":false,"Msg":"NOTLOGIN"}`))
				return
			}
			var payload struct {
				Username string
				Count    int
			}
			if r.Header.Get("Content-Type") != "application/json" || json.NewDecoder(r.Body).Decode(&payload) != nil || payload.Username != "renamed" || payload.Count != 2 {
				w.Write([]byte(`{"Ok":false,"Msg":"badRequest"}`))
				return
			}
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"Ok":true}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	e := newTestEnv(t)
	e.db.SetConfig("host", server.URL)
	e.db.SetConfig("proxy:email", "tester")
	e.db.SetConfig("proxy:pwd", "secret")
	e.handler.Proxy = NewServerProxy(e.db, e.handler.Files)
	req := httptest.NewRequest(http.MethodPost, "/api2/user/updateUsername", bytes.NewBufferString(`{"username":"renamed","count":2}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated || rec.Body.String() != `{"Ok":true}` || sessionLogins.Load() != 1 {
		t.Fatalf("account JSON forward: status=%d body=%s sessions=%d", rec.Code, rec.Body.String(), sessionLogins.Load())
	}
}

func TestUpdatePasswordCachesOnlyConfirmedRemoteChange(t *testing.T) {
	e := newTestEnv(t)
	userID, _ := e.login(t)
	var allow atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/auth/login":
			w.Write([]byte(api2LoginResponse(userID)))
		case "/api2/auth/session":
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "valid", Path: "/"})
			w.Write([]byte(`{"Ok":true}`))
		case "/api2/system/version":
			w.Write([]byte(`{"server":"gemsnote","version":"1.0.0"}`))
		case "/api2/user/updatePwd":
			if _, err := r.Cookie("session"); err != nil {
				w.Write([]byte(`{"Ok":false,"Msg":"NOTLOGIN"}`))
				return
			}
			if !allow.Load() {
				w.Write([]byte(`{"Ok":false,"Msg":"wrongPassword"}`))
				return
			}
			w.Write([]byte(`{"Ok":true}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	e.db.SetConfig("host", server.URL)
	e.db.SetConfig("proxy:email", "tester")
	e.db.SetConfig("proxy:pwd", "secret")
	e.handler.Proxy = NewServerProxy(e.db, e.handler.Files)
	_, body := e.post(t, "/api2/user/updatePwd", url.Values{"oldPwd": {"wrong"}, "pwd": {"new-secret"}})
	if !bytes.Contains(body, []byte("wrongPassword")) {
		t.Fatalf("rejected password response: %s", body)
	}
	user, _ := e.db.GetUser(userID)
	if user.Pwd != utils.MD5WithSalt("secret", userID) {
		t.Fatalf("local password changed after remote rejection")
	}
	allow.Store(true)
	_, body = e.post(t, "/api2/user/updatePwd", url.Values{"oldPwd": {"secret"}, "pwd": {"new-secret"}})
	if !bytes.Contains(body, []byte(`"Ok":true`)) {
		t.Fatalf("accepted password response: %s", body)
	}
	user, _ = e.db.GetUser(userID)
	if user.Pwd != utils.MD5WithSalt("new-secret", userID) {
		t.Fatalf("local password not updated after remote acceptance")
	}
}

func TestAvatarUploadForwardsAjaxHeader(t *testing.T) {
	e := newTestEnv(t)
	userID, _ := e.login(t)
	var receivedUpload atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/auth/login":
			w.Write([]byte(api2LoginResponse(userID)))
		case "/api2/auth/session":
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "valid", Path: "/"})
			w.Write([]byte(`{"Ok":true}`))
		case "/api2/system/version":
			w.Write([]byte(`{"server":"gemsnote","version":"1.0.0"}`))
		case "/api2/avatar":
			if r.Header.Get("X-Requested-With") != "XMLHttpRequest" {
				w.Write([]byte(`{"Ok":false,"Msg":"invalidRequest"}`))
				return
			}
			if cookie, err := r.Cookie("session"); err != nil || cookie.Value != "valid" {
				w.Write([]byte(`{"Ok":false,"Msg":"NOTLOGIN"}`))
				return
			}
			if file, _, err := r.FormFile("file"); err == nil {
				file.Close()
				receivedUpload.Store(true)
			}
			w.Write([]byte(`{"Ok":true,"Id":"0123456789abcdef01234567"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	e.db.SetConfig("host", server.URL)
	e.db.SetConfig("proxy:email", "tester")
	e.db.SetConfig("proxy:pwd", "secret")
	e.handler.Proxy = NewServerProxy(e.db, e.handler.Files)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "avatar.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("avatar image")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api2/avatar", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	e.handler.ServeHTTP(rec, req)
	var result struct {
		Ok bool
		Id string
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if !receivedUpload.Load() || !result.Ok || result.Id == "" || result.Id == "0123456789abcdef01234567" {
		t.Fatalf("avatar upload was not forwarded successfully: %s", rec.Body.String())
	}
	logo, _ := e.db.GetConfig("logo:" + userID)
	if logo != "/api2/file/getImage?fileId="+result.Id {
		t.Fatalf("uploaded avatar is not available from the local image route: %q", logo)
	}
}

func TestProfileAvatarUsesTokenAndCachesImage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/auth/login":
			w.Write([]byte(api2LoginResponse("user1")))
		case "/api2/system/version":
			w.Write([]byte(`{"server":"gemsnote","version":"1.0.0"}`))
		case "/api2/user/info":
			if r.URL.Query().Get("token") != "test-token" {
				w.Write([]byte(`{"Ok":false,"Msg":"NOTLOGIN"}`))
				return
			}
			w.Write([]byte(`{"UserId":"user1","Username":"tester","Logo":"public/upload/avatar.jpeg"}`))
		case "/public/upload/avatar.jpeg":
			w.Header().Set("Content-Type", "image/jpeg")
			w.Write([]byte("image bytes"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	database, err := db.NewInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.InsertUser(&models.User{ID: "user1", Username: "tester", Host: server.URL, IsActive: true}); err != nil {
		t.Fatal(err)
	}
	database.SetCurrentUser("user1")
	database.SetConfig("host", server.URL)
	files := service.NewFileService(database)
	files.SetDataDir(t.TempDir())
	proxy := NewServerProxy(database, files)
	if ok, msg := proxy.LoginServer("tester", "secret"); !ok {
		t.Fatalf("login failed: %s", msg)
	}
	if !proxy.RefreshUserProfile() {
		t.Fatal("profile refresh failed")
	}
	logo, _ := database.GetConfig("logo:user1")
	if logo == "" || logo == "public/upload/avatar.jpeg" {
		t.Fatalf("avatar not cached locally: %q", logo)
	}
}

func TestRemoteLoginReportsWhetherAccountCacheExists(t *testing.T) {
	for _, tc := range []struct {
		name      string
		withCache bool
		savedHost string
	}{
		{name: "fresh account", withCache: false},
		{name: "cached account", withCache: true},
		{name: "cache with missing saved host", withCache: true, savedHost: "missing"},
		{name: "cache with changed server address", withCache: true, savedHost: "https://old-address.example"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			authCalls, profileCalls := 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api2/auth/login":
					authCalls++
					w.Write([]byte(api2LoginResponse("user1")))
				case "/api2/system/version":
					w.Write([]byte(`{"server":"gemsnote","version":"1.0.0"}`))
				case "/api2/user/info":
					profileCalls++
					w.Write([]byte(`{"UserId":"user1","Username":"tester"}`))
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			e := newTestEnv(t)
			if tc.withCache {
				host := server.URL
				if tc.savedHost != "" {
					host = tc.savedHost
				}
				if host == "missing" {
					host = ""
				}
				if err := e.db.InsertUser(&models.User{ID: "user1", Username: "previous-name", Host: host}); err != nil {
					t.Fatal(err)
				}
				if err := e.db.InsertNotebook(&models.Notebook{ID: "book1", NotebookID: "book1", UserID: "user1", Title: "Cached"}); err != nil {
					t.Fatal(err)
				}
			}
			proxy := NewServerProxy(e.db, e.handler.Files)
			proxy.SetHost(server.URL)
			e.handler.Proxy = proxy
			var hookCache bool
			var sessionLive bool
			e.handler.OnSessionChanged = func(live bool) { sessionLive = live }
			e.handler.OnLogin = func(hasLocalCache bool) (any, error) {
				hookCache = hasLocalCache
				return map[string]any{"SyncChoiceRequired": hasLocalCache}, nil
			}
			var result map[string]any
			e.postJSON(t, "/api2/auth/session", url.Values{"email": {"tester"}, "pwd": {"secret"}}, &result)
			if result["Ok"] != true || !sessionLive || hookCache != tc.withCache || (result["SyncChoiceRequired"] == true) != tc.withCache {
				t.Fatalf("login cache decision mismatch: result=%v sessionLive=%v hookCache=%v", result, sessionLive, hookCache)
			}
			if authCalls != 1 || profileCalls != 0 {
				t.Fatalf("login performed redundant requests: auth=%d profile=%d", authCalls, profileCalls)
			}
			if paused, err := e.db.GetConfig("sync:paused:user1"); err != nil || paused != "true" {
				t.Fatalf("auto sync must remain paused until the user decides: paused=%s err=%v", paused, err)
			}
		})
	}
}

func TestLoginCacheReadFailureDoesNotStartSync(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api2/auth/login" {
			t.Errorf("unexpected login-time request %s", r.URL.Path)
		}
		w.Write([]byte(api2LoginResponse("user1")))
	}))
	defer server.Close()
	e := newTestEnv(t)
	e.handler.Proxy = NewServerProxy(e.db, e.handler.Files)
	e.handler.Proxy.SetHost(server.URL)
	tx, err := e.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	// Simulate a cache/schema read error only inside the test's memory DB.
	if _, err := tx.Exec("DROP TABLE images"); err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	e.handler.OnLogin = func(bool) (any, error) {
		t.Error("failed cache detection must not call the initial-sync hook")
		return nil, nil
	}
	var result map[string]any
	e.postJSON(t, "/api2/auth/session", url.Values{"email": {"tester"}, "pwd": {"secret"}}, &result)
	if result["Ok"] != false || result["Msg"] == "" {
		t.Fatalf("cache read failure was ignored: %v", result)
	}
	if user, err := e.db.GetActiveUser(); err != nil || user != nil {
		t.Fatalf("failed login exposed an active account: %+v, %v", user, err)
	}
}

func TestProfileRefreshAcceptsDefaultOrMissingAvatar(t *testing.T) {
	for _, avatar := range []string{"/images/blog/default_avatar.png", "/missing/avatar.png"} {
		t.Run(avatar, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api2/auth/login":
					w.Write([]byte(api2LoginResponse("user1")))
				case "/api2/system/version":
					w.Write([]byte(`{"server":"gemsnote","version":"1.0.0"}`))
				case "/api2/user/info":
					w.Write([]byte(`{"UserId":"user1","Username":"tester","Logo":"` + avatar + `"}`))
				case "/images/blog/default_avatar.png":
					w.Header().Set("Content-Type", "image/png")
					w.Write([]byte("image bytes"))
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()

			database, err := db.NewInMemory()
			if err != nil {
				t.Fatal(err)
			}
			defer database.Close()
			if err := database.InsertUser(&models.User{ID: "user1", Username: "tester", Host: server.URL, IsActive: true}); err != nil {
				t.Fatal(err)
			}
			database.SetCurrentUser("user1")
			database.SetConfig("host", server.URL)
			files := service.NewFileService(database)
			files.SetDataDir(t.TempDir())
			proxy := NewServerProxy(database, files)
			if ok, msg := proxy.LoginServer("tester", "secret"); !ok {
				t.Fatalf("login failed: %s", msg)
			}
			if !proxy.RefreshUserProfile() {
				t.Fatal("valid profile was rejected because its avatar could not be cached")
			}
		})
	}
}

func newEmptyCookieJar() (http.CookieJar, error) {
	return cookiejar.New(nil)
}
