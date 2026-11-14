package db

import (
	"github.com/gemsnote/gemsnote/models"
	"strings"
)

// NoteListPage reads metadata only. Paging never materializes note bodies.
func (d *Database) NoteListPage(userID, notebookID, key, tag, order string, trash, starred bool, page, size int) ([]*models.Note, error) {
	clauses := []string{"user_id=?", "COALESCE(local_is_delete,0)=0", "is_trash=?"}
	args := []any{userID, trash}
	if notebookID != "" {
		clauses = append(clauses, "notebook_id=?")
		args = append(args, notebookID)
	}
	if key != "" {
		clauses = append(clauses, "(instr(lower(title),lower(?))>0 OR instr(lower(content),lower(?))>0)")
		args = append(args, key, key)
	}
	if tag != "" {
		clauses = append(clauses, "EXISTS(SELECT 1 FROM json_each(notes.tags) WHERE value=?)")
		args = append(args, tag)
	}
	if starred {
		clauses = append(clauses, "is_star=1")
	}
	sortSQL := "COALESCE(CASE WHEN is_dirty=1 THEN local_edited_time END,updated_time) DESC, note_id"
	if order == "Title" {
		sortSQL = "lower(title), note_id"
	} else if order == "CreatedTime" {
		sortSQL = "created_time DESC, note_id"
	}
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 100
	}
	args = append(args, size, (page-1)*size)
	rows, err := d.db.Query(`SELECT _id, note_id, server_note_id, notebook_id, user_id,
			title, '' AS content, desc, abstract, img_src, tags,
			is_markdown, is_trash, is_blog, is_star, usn,
			is_dirty, content_is_dirty, local_is_new, local_is_delete, init_sync,
			conflict_note_id, conflict_time, conflict_fixed, err,
			created_time, updated_time, local_edited_time
		FROM notes WHERE `+strings.Join(clauses, " AND ")+` ORDER BY `+sortSQL+` LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return d.scanNotes(rows)
}
