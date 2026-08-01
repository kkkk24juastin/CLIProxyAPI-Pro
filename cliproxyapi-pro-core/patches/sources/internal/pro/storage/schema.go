package storage

import (
	"context"
	"database/sql"
	"strings"
)

// Schema describes idempotent creation, additive migration and seed phases.
// It is intentionally data-driven so the compatibility facade can retain the
// historical SQL text while storage owns execution order and error semantics.
type Schema struct {
	Create []string
	Alter  []string
	Seed   []string
}

func ApplySchema(ctx context.Context, db *sql.DB, schema Schema) error {
	if db == nil {
		return sql.ErrConnDone
	}
	for _, statement := range schema.Create {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	for _, statement := range schema.Alter {
		if _, err := db.ExecContext(ctx, statement); err != nil && !isDuplicateColumn(err) {
			return err
		}
	}
	for _, statement := range schema.Seed {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func isDuplicateColumn(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "duplicate column name")
}
