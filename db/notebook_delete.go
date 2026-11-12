package db

import (
	"fmt"
	"time"
)

func (d *Database) DeleteNotebookWithNotes(userID, notebookID string, deleteNotes bool) error {
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var children int
	if err = tx.QueryRow(`SELECT COUNT(*) FROM notebooks WHERE user_id = ? AND parent_notebook_id = ? AND local_is_delete = 0`, userID, notebookID).Scan(&children); err != nil {
		return err
	}
	if children > 0 {
		return fmt.Errorf("笔记本下有子笔记本")
	}
	var notes int
	if err = tx.QueryRow(`SELECT COUNT(*) FROM notes WHERE user_id = ? AND notebook_id = ? AND is_trash = 0 AND local_is_delete = 0`, userID, notebookID).Scan(&notes); err != nil {
		return err
	}
	if notes > 0 && !deleteNotes {
		return fmt.Errorf("notebookHasNotes")
	}
	if deleteNotes {
		if _, err = tx.Exec(`UPDATE notes SET is_trash = 1, is_dirty = 1, local_is_delete = CASE WHEN local_is_new = 1 THEN 1 ELSE local_is_delete END, local_edited_time = ? WHERE user_id = ? AND notebook_id = ? AND is_trash = 0 AND local_is_delete = 0`, time.Now().Unix(), userID, notebookID); err != nil {
			return err
		}
		if _, err = tx.Exec(`INSERT OR IGNORE INTO notebook_delete_options (notebook_id) VALUES (?)`, notebookID); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(`UPDATE notebooks SET is_dirty = 1, local_is_delete = 1, updated_time = ? WHERE user_id = ? AND notebook_id = ?`, time.Now().Unix(), userID, notebookID); err != nil {
		return err
	}
	return tx.Commit()
}

func (d *Database) NotebookDeleteNotes(notebookID string) bool {
	var count int
	_ = d.db.QueryRow(`SELECT COUNT(*) FROM notebook_delete_options WHERE notebook_id = ?`, notebookID).Scan(&count)
	return count > 0
}
