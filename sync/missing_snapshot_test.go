package sync

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gemsnote/gemsnote/api"
	"github.com/gemsnote/gemsnote/db"
	"github.com/gemsnote/gemsnote/models"
)

func TestSnapshotMissingAttachmentKeepsPlaceholder(t *testing.T) {
	d, err := db.NewInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	for _, err := range []error{d.InsertUser(&models.User{ID: "u", Username: "u"}), d.InsertNotebook(&models.Notebook{ID: "b", NotebookID: "b", UserID: "u"}), d.InsertNote(&models.Note{ID: "n", NoteID: "n", NotebookID: "b", UserID: "u"})} {
		if err != nil {
			t.Fatal(err)
		}
	}
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	client := api.NewClient()
	client.SetHost(server.URL)
	s := NewSyncService(d, client)
	batch := &snapshotDownloads{}
	s.downloadSnapshotFiles(context.Background(), client, "u", map[string]snapshotFile{"attach:f:n": {id: "f", noteID: "n", title: "ELK.xml", attachment: true}}, t.TempDir(), t.TempDir(), batch)
	files, err := d.GetAttachsByNote("n")
	if err != nil || len(files) != 1 || files[0].ServerFileID != "f" || files[0].Path != "" || files[0].IsDirty {
		t.Fatalf("snapshot lost missing attachment: %+v %v", files, err)
	}
}
