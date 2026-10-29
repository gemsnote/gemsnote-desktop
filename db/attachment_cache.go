package db

import "fmt"

// Update only cached bytes, never attachment membership or pending uploads.
func (d *Database) SetAttachmentCache(userID, fileID, serverID, path string) error {
	result, err := d.db.Exec(`UPDATE attachs SET path=? WHERE user_id=? AND file_id=? AND server_file_id=? AND is_dirty=0`, path, userID, fileID, serverID)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return fmt.Errorf("attachmentChanged")
	}
	return nil
}
