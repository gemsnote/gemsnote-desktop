package db

import "database/sql"

type NoteSyncState struct {
	NoteID, NotebookID        string
	Usn                       int64
	NeedsBody, Dirty, Deleted bool
}

func (d *Database) GetNoteSyncState(remoteID string) (*NoteSyncState, error) {
	query, args := `SELECT note_id,COALESCE(notebook_id,''),usn,(init_sync=1 OR content IS NULL),is_dirty,local_is_delete FROM notes WHERE server_note_id=?`, []any{remoteID}
	if user := d.GetCurrentUserID(); user != "" {
		query += ` AND user_id=?`
		args = append(args, user)
	}
	n := &NoteSyncState{}
	err := d.db.QueryRow(query, args...).Scan(&n.NoteID, &n.NotebookID, &n.Usn, &n.NeedsBody, &n.Dirty, &n.Deleted)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return n, err
}
