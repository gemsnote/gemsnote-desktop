package db

import "fmt"

// ResetAccountCache removes only one account's cached content. The user row
// and its authenticated session remain so a fresh server snapshot can be
// downloaded without asking the user to log in again.
func (d *Database) ResetAccountCache(userID, accountID string) error {
	if userID == "" || accountID == "" {
		return fmt.Errorf("missing account identity")
	}
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var host string
	if err := tx.QueryRow(`SELECT host FROM users WHERE _id = ? AND is_active = 1`, userID).Scan(&host); err != nil {
		return err
	}
	if accountID != SharedAccountID(host, userID) {
		return fmt.Errorf("shared account identity mismatch")
	}
	for _, statement := range []string{
		`DELETE FROM note_histories WHERE note_id IN (SELECT note_id FROM notes WHERE user_id = ?)`,
		`DELETE FROM attachs WHERE user_id = ?`,
		`DELETE FROM images WHERE user_id = ?`,
		`DELETE FROM notes WHERE user_id = ?`,
		`DELETE FROM notebooks WHERE user_id = ?`,
		`DELETE FROM tags WHERE user_id = ?`,
	} {
		if _, err := tx.Exec(statement, userID); err != nil {
			return err
		}
	}
	for _, table := range []string{"shared_files", "shared_download_jobs", "shared_snapshot_items", "shared_notes", "shared_notebooks", "shared_sync_state", "shared_accounts"} {
		if _, err := tx.Exec(`DELETE FROM `+table+` WHERE account_id = ?`, accountID); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`UPDATE users SET last_sync_usn = -1, notebook_usn = -1, note_usn = -1, tag_usn = -1, last_sync_time = NULL WHERE _id = ?`, userID); err != nil {
		return err
	}
	return tx.Commit()
}
