package db

import (
	"github.com/gemsnote/gemsnote/models"
	"time"
)

// Keep the requested import time separate from local edit times across restarts
// and upload retries. It is used only when creating a new remote note.
func (d *Database) InsertImportedNote(note *models.Note) error {
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = d.InsertNoteTx(tx, note); err != nil {
		return err
	}
	if note.ImportedUpdatedTime != nil {
		if _, err = tx.Exec(`INSERT OR REPLACE INTO note_import_times (note_id, created_time, updated_time) VALUES (?, ?, ?)`, note.NoteID, note.CreatedTime.Format(time.RFC3339Nano), note.ImportedUpdatedTime.Format(time.RFC3339Nano)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (d *Database) ImportedNoteTime(noteID string) *time.Time {
	return d.importedTime(noteID, "updated_time")
}

func (d *Database) ImportedNoteCreatedTime(noteID string) *time.Time {
	return d.importedTime(noteID, "created_time")
}

func (d *Database) importedTime(noteID, column string) *time.Time {
	var value string
	if err := d.db.QueryRow(`SELECT `+column+` FROM note_import_times WHERE note_id = ?`, noteID).Scan(&value); err != nil {
		return nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return nil
	}
	return &parsed
}
