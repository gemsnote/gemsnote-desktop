package db

import (
	"context"
	"fmt"
	"strings"
)

// SQLite requires rebuilding a table to remove NOT NULL. Keep foreign keys
// disabled only on this startup connection while preserving dependent rows.
func (d *Database) migrateOptionalTrashNotebook() error {
	conn, err := d.db.Conn(context.Background())
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err := conn.ExecContext(context.Background(), `PRAGMA foreign_keys = OFF`); err != nil {
		return err
	}
	defer conn.ExecContext(context.Background(), `PRAGMA foreign_keys = ON`)
	tx, err := conn.BeginTx(context.Background(), nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var schema string
	if err = tx.QueryRow(`SELECT sql FROM sqlite_master WHERE type='table' AND name='notes'`).Scan(&schema); err != nil {
		return err
	}
	rows, err := tx.Query(`SELECT sql FROM sqlite_master WHERE tbl_name='notes' AND type IN ('index','trigger') AND sql IS NOT NULL`)
	if err != nil {
		return err
	}
	var indexes []string
	for rows.Next() {
		var s string
		if err = rows.Scan(&s); err != nil {
			rows.Close()
			return err
		}
		indexes = append(indexes, s)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	schema = "CREATE TABLE notes_optional " + schema[strings.Index(schema, "("):]
	schema = strings.Replace(schema, "notebook_id TEXT NOT NULL", "notebook_id TEXT CHECK(notebook_id IS NOT NULL OR is_trash = 1)", 1)
	if _, err = tx.Exec(schema); err != nil {
		return err
	}
	for _, q := range []string{
		`INSERT INTO notes_optional SELECT * FROM notes`,
		`DROP TABLE notes`,
		`ALTER TABLE notes_optional RENAME TO notes`,
	} {
		if _, err = tx.Exec(q); err != nil {
			return err
		}
	}
	for _, q := range indexes {
		if _, err = tx.Exec(q); err != nil {
			return err
		}
	}
	// Clean up tombstones previously retained solely for trashed notes.
	if _, err = tx.Exec(`UPDATE notes SET notebook_id=NULL WHERE is_trash=1 AND notebook_id IN (SELECT notebook_id FROM notebooks WHERE local_is_delete=1)`); err != nil {
		return err
	}
	if _, err = tx.Exec(`DELETE FROM notebooks WHERE local_is_delete=1 AND is_dirty=0 AND NOT EXISTS (SELECT 1 FROM notes WHERE notes.notebook_id=notebooks.notebook_id)`); err != nil {
		return err
	}
	check, err := tx.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		return err
	}
	invalid := check.Next()
	err = check.Err()
	check.Close()
	if err != nil {
		return err
	}
	if invalid {
		return fmt.Errorf("foreign key validation failed during trash notebook migration")
	}
	if _, err = tx.Exec(`PRAGMA user_version=6`); err != nil {
		return err
	}
	return tx.Commit()
}
