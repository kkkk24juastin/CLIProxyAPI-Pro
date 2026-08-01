package storage

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
)

func TestDatabaseOwnsLifecycleAndDomainRepositories(t *testing.T) {
	database, err := OpenSQLite(filepath.Join(t.TempDir(), "nested", "pro.db"))
	if err != nil {
		t.Fatalf("OpenSQLite() error = %v", err)
	}
	if database.Usage().Domain() != DomainUsage || database.Quota().Domain() != DomainQuota || database.RoutingState().Domain() != DomainRoutingState {
		t.Fatal("domain repositories do not preserve ownership")
	}
	if err := database.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if database.SQL() != nil {
		t.Fatal("SQL() remained available after Close")
	}
}

func TestApplySchemaIsIdempotentForAdditiveMigrations(t *testing.T) {
	database, err := OpenSQLite(filepath.Join(t.TempDir(), "pro.db"))
	if err != nil {
		t.Fatalf("OpenSQLite() error = %v", err)
	}
	defer database.Close()
	schema := Schema{
		Create: []string{`create table if not exists sample (id integer primary key)`},
		Alter:  []string{`alter table sample add column value text`},
		Seed:   []string{`insert or ignore into sample(id, value) values (1, 'ok')`},
	}
	if err := ApplySchema(context.Background(), database.SQL(), schema); err != nil {
		t.Fatalf("first ApplySchema() error = %v", err)
	}
	if err := ApplySchema(context.Background(), database.SQL(), schema); err != nil {
		t.Fatalf("second ApplySchema() error = %v", err)
	}
}

func TestRepositoryTransactionRollsBackOnFailure(t *testing.T) {
	database, err := OpenSQLite(filepath.Join(t.TempDir(), "pro.db"))
	if err != nil {
		t.Fatalf("OpenSQLite() error = %v", err)
	}
	defer database.Close()
	if _, err := database.SQL().Exec(`create table sample (value text)`); err != nil {
		t.Fatalf("create table error = %v", err)
	}
	wantErr := errors.New("stop")
	err = database.Settings().Transaction(context.Background(), func(tx *sql.Tx) error {
		if _, insertErr := tx.Exec(`insert into sample(value) values ('temporary')`); insertErr != nil {
			return insertErr
		}
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("Transaction() error = %v, want %v", err, wantErr)
	}
	var count int
	if err := database.SQL().QueryRow(`select count(*) from sample`).Scan(&count); err != nil {
		t.Fatalf("count rows error = %v", err)
	}
	if count != 0 {
		t.Fatalf("row count = %d, want rollback", count)
	}
}
