package sync

import (
	"fmt"
	"os"
	"time"

	"github.com/gemsnote/gemsnote/models"
	"github.com/gemsnote/gemsnote/utils"
)

// Metadata APIs include the complete file set. Incremental sync must apply
// that set as well as the body, including an explicitly empty attachment set.
func (s *SyncService) syncAttachmentSet(remote *models.Note) error {
	remoteID := remote.NoteID
	if remote.ServerNoteID != "" {
		remoteID = remote.ServerNoteID
	}
	local, err := s.db.GetNoteByServerID(remoteID)
	if err != nil {
		return err
	}
	if local == nil {
		return fmt.Errorf("missing local note")
	}
	current, err := s.db.GetAttachsByNote(local.NoteID)
	if err != nil {
		return err
	}
	byServer := map[string]*models.Attach{}
	for _, a := range current {
		if a.IsDirty {
			return fmt.Errorf("pending local attachment")
		}
		byServer[a.ServerFileID] = a
	}
	files := []*models.Attach{}
	for _, ref := range remote.Files {
		if ref == nil || !ref.IsAttach {
			continue
		}
		if a := byServer[ref.FileID]; a != nil && a.Path != "" {
			if stat, e := os.Stat(a.Path); e == nil && stat.Mode().IsRegular() {
				copy := *a
				copy.Title = ref.Title
				copy.Type = ref.Type
				files = append(files, &copy)
				continue
			}
		}
		dir := s.files.GetUserAttachDir(local.UserID)
		if err = os.MkdirAll(dir, 0700); err != nil {
			return err
		}
		path, title, e := s.api.GetAttach(ref.FileID, dir)
		if e != nil {
			return e
		}
		if ref.Title != "" {
			title = ref.Title
		}
		now := time.Now()
		files = append(files, &models.Attach{ID: utils.ObjectId(), FileID: utils.ObjectId(), ServerFileID: ref.FileID, NoteID: local.NoteID, UserID: local.UserID, Title: title, Type: ref.Type, Path: path, IsAttach: true, CreatedTime: &now})
	}
	// A local edit during download must not be overwritten.
	return s.db.ReplaceSyncedAttachments(local.UserID, local.NoteID, remote.Usn, files)
}
