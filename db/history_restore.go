package db

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/gemsnote/gemsnote/models"
	"github.com/gemsnote/gemsnote/utils"
)

// RestoreLocalHistory updates the note and attachment membership in one SQLite
// transaction. Historical bytes are retained separately from active records.
func (d *Database) RestoreLocalHistory(userID, noteID string, historyID int64) error {
	n, err := d.GetNote(noteID)
	if err != nil {
		return err
	}
	if n == nil || n.UserID != userID || n.IsTrash || n.LocalIsDelete {
		return fmt.Errorf("noAuth")
	}
	histories, err := d.GetNoteHistories(noteID)
	if err != nil {
		return err
	}
	for _, h := range histories {
		if h.ID != historyID {
			continue
		}
		if !h.FilesComplete || h.IsMarkdown == nil {
			return fmt.Errorf("historyFilesUnknown")
		}
		seen := map[string]bool{}
		for _, a := range h.Attachments {
			if a == nil || a.NoteID != noteID || a.UserID != userID || a.FileID == "" || seen[a.FileID] {
				return fmt.Errorf("historyFilesInvalid")
			}
			seen[a.FileID] = true
			stat, e := os.Stat(a.Path)
			if e != nil || !stat.Mode().IsRegular() {
				return fmt.Errorf("historyFileMissing")
			}
		}
		current, err := d.GetAttachsByNote(noteID)
		if err != nil {
			return err
		}
		backup, err := json.Marshal(current)
		if err != nil {
			return err
		}
		tx, err := d.db.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback()
		now := time.Now().Unix()
		result, err := tx.Exec(`UPDATE notes SET content=?,is_markdown=?,is_dirty=1,content_is_dirty=1,updated_time=?,local_edited_time=? WHERE note_id=? AND user_id=? AND content=? AND is_markdown=? AND usn=? AND is_trash=0 AND local_is_delete=0`, h.Content, *h.IsMarkdown, now, now, noteID, userID, n.Content, n.IsMarkdown, n.Usn)
		if err != nil {
			return err
		}
		count, _ := result.RowsAffected()
		if count != 1 {
			return fmt.Errorf("conflict")
		}
		if _, err = tx.Exec(`INSERT INTO note_histories(note_id,content,updated_time,is_markdown,attachments_json,files_complete) VALUES(?,?,?,?,?,?)`, noteID, n.Content, now, n.IsMarkdown, string(backup), n.ServerNoteID == ""); err != nil {
			return err
		}
		if _, err = tx.Exec(`DELETE FROM attachs WHERE note_id=? AND user_id=?`, noteID, userID); err != nil {
			return err
		}
		for _, a := range h.Attachments {
			if _, err = tx.Exec(`INSERT INTO attachs(_id,file_id,server_file_id,note_id,user_id,title,type,path,is_attach,is_dirty,created_time) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, a.ID, a.FileID, "", noteID, userID, a.Title, a.Type, a.Path, a.IsAttach, true, utils.TimeToUnix(a.CreatedTime)); err != nil {
				return err
			}
		}
		return tx.Commit()
	}
	return fmt.Errorf("historyNotFound")
}

// ReplaceSyncedAttachments is called only for a confirmed complete remote file
// list. Keep historical bytes and reject a concurrently edited local note.
func (d *Database) ReplaceSyncedAttachments(userID, noteID string, usn int64, files []*models.Attach) error {
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var current int64
	var dirty bool
	if err = tx.QueryRow(`SELECT usn,is_dirty FROM notes WHERE user_id=? AND note_id=?`, userID, noteID).Scan(&current, &dirty); err != nil {
		return err
	}
	if current != usn || dirty {
		return fmt.Errorf("historyPendingChanges")
	}
	var pending int
	if err = tx.QueryRow(`SELECT COUNT(*) FROM attachs WHERE note_id=? AND is_dirty=1`, noteID).Scan(&pending); err != nil {
		return err
	}
	if pending != 0 {
		return fmt.Errorf("historyPendingChanges")
	}
	if _, err = tx.Exec(`DELETE FROM attachs WHERE note_id=? AND user_id=?`, noteID, userID); err != nil {
		return err
	}
	for _, a := range files {
		if a.NoteID != noteID || a.UserID != userID {
			return fmt.Errorf("noAuth")
		}
		if _, err = tx.Exec(`INSERT INTO attachs(_id,file_id,server_file_id,note_id,user_id,title,type,path,is_attach,is_dirty,created_time) VALUES(?,?,?,?,?,?,?,?,?,0,?)`, a.ID, a.FileID, a.ServerFileID, noteID, userID, a.Title, a.Type, a.Path, true, utils.TimeToUnix(a.CreatedTime)); err != nil {
			return err
		}
	}
	return tx.Commit()
}
