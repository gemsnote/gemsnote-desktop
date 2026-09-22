package db

import (
	"testing"

	"github.com/gemsnote/gemsnote/models"
)

func TestResetAccountCacheIsAccountScoped(t *testing.T) {
	d, err := NewInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	for _, id := range []string{"user-a", "user-b"} {
		if err := d.InsertUser(&models.User{ID: id, Username: id, Host: "https://example.test", IsActive: id == "user-a", LastSyncUsn: 12, NotebookUsn: 11, NoteUsn: 10, TagUsn: 9}); err != nil {
			t.Fatal(err)
		}
		if err := d.InsertNotebook(&models.Notebook{ID: id + "-book", NotebookID: id + "-book", UserID: id, Title: id}); err != nil {
			t.Fatal(err)
		}
		if err := d.InsertNote(&models.Note{ID: id + "-note", NoteID: id + "-note", NotebookID: id + "-book", UserID: id, Title: id, Content: id}); err != nil {
			t.Fatal(err)
		}
		if _, err := d.db.Exec(`INSERT INTO note_histories(note_id,content) VALUES(?,?)`, id+"-note", id); err != nil {
			t.Fatal(err)
		}
		if _, err := d.db.Exec(`INSERT INTO tags(_id,tag,user_id) VALUES(?,?,?)`, id+"-tag", id, id); err != nil {
			t.Fatal(err)
		}
		if _, err := d.db.Exec(`INSERT INTO shared_accounts(account_id,server_url,remote_user_id) VALUES(?,?,?)`, SharedAccountID("https://example.test", id), "https://example.test", id); err != nil {
			t.Fatal(err)
		}
		if _, err := d.db.Exec(`INSERT INTO shared_notes(account_id,server_note_id,owner_user_id,generation) VALUES(?,?,?,1)`, SharedAccountID("https://example.test", id), id+"-remote", id); err != nil {
			t.Fatal(err)
		}
	}
	if err := d.ResetAccountCache("user-a", SharedAccountID("https://example.test", "user-a")); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ table, field string }{
		{"notes", "user_id"}, {"notebooks", "user_id"}, {"tags", "user_id"},
		{"note_histories", "note_id"}, {"shared_accounts", "account_id"}, {"shared_notes", "account_id"},
	} {
		keyA, keyB := "user-a", "user-b"
		switch tc.field {
		case "note_id":
			keyA, keyB = "user-a-note", "user-b-note"
		case "account_id":
			keyA, keyB = SharedAccountID("https://example.test", "user-a"), SharedAccountID("https://example.test", "user-b")
		}
		for key, want := range map[string]int{keyA: 0, keyB: 1} {
			var got int
			if err := d.db.QueryRow(`SELECT COUNT(*) FROM `+tc.table+` WHERE `+tc.field+` = ?`, key).Scan(&got); err != nil || got != want {
				t.Fatalf("%s %s count=%d want=%d err=%v", tc.table, key, got, want, err)
			}
		}
	}
	for id, want := range map[string]int64{"user-a": -1, "user-b": 12} {
		var usn int64
		if err := d.db.QueryRow(`SELECT last_sync_usn FROM users WHERE _id = ?`, id).Scan(&usn); err != nil || usn != want {
			t.Fatalf("%s last sync usn=%d want=%d err=%v", id, usn, want, err)
		}
	}
	if err := d.ResetAccountCache("user-b", SharedAccountID("https://example.test", "user-b")); err == nil {
		t.Fatal("inactive account reset must be rejected")
	}
	if err := d.ResetAccountCache("user-a", SharedAccountID("https://example.test", "user-b")); err == nil {
		t.Fatal("mismatched shared account reset must be rejected")
	}
}
