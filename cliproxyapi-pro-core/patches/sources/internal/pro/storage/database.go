// Package storage owns the shared Pro SQLite connection and exposes explicit
// domain repositories. Business modules share one database without sharing
// lifecycle or transaction implementation details.
package storage

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

type Domain string

const (
	DomainUsage           Domain = "usage"
	DomainModelPrice      Domain = "model_price"
	DomainQuota           Domain = "quota"
	DomainSettings        Domain = "settings"
	DomainRoutingState    Domain = "routing_state"
	DomainInspectionState Domain = "inspection_state"
)

type Database struct {
	mu     sync.RWMutex
	shared *sharedDatabase
	closed bool
}

type sharedDatabase struct {
	db   *sql.DB
	path string
	refs int
}

var sharedDatabases = struct {
	sync.Mutex
	items map[string]*sharedDatabase
}{items: make(map[string]*sharedDatabase)}

func OpenSQLite(path string) (*Database, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("sqlite path is required")
	}
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	path = filepath.Clean(absolutePath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	sharedDatabases.Lock()
	if existing := sharedDatabases.items[path]; existing != nil {
		existing.refs++
		sharedDatabases.Unlock()
		return &Database{shared: existing}, nil
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		sharedDatabases.Unlock()
		return nil, err
	}
	db.SetMaxOpenConns(1)
	for _, statement := range []string{
		`pragma foreign_keys = ON`,
		`pragma busy_timeout = 5000`,
	} {
		if _, err = db.Exec(statement); err != nil {
			_ = db.Close()
			sharedDatabases.Unlock()
			return nil, err
		}
	}
	var foreignKeys int
	if err = db.QueryRow(`pragma foreign_keys`).Scan(&foreignKeys); err != nil || foreignKeys != 1 {
		_ = db.Close()
		sharedDatabases.Unlock()
		if err != nil {
			return nil, err
		}
		return nil, errors.New("sqlite foreign key enforcement is unavailable")
	}
	shared := &sharedDatabase{db: db, path: path, refs: 1}
	sharedDatabases.items[path] = shared
	sharedDatabases.Unlock()
	return &Database{shared: shared}, nil
}

func (d *Database) SQL() *sql.DB {
	if d == nil {
		return nil
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.closed || d.shared == nil {
		return nil
	}
	return d.shared.db
}

func (d *Database) Close() error {
	if d == nil {
		return nil
	}
	d.mu.Lock()
	if d.closed || d.shared == nil {
		d.mu.Unlock()
		return nil
	}
	shared := d.shared
	d.closed = true
	d.shared = nil
	d.mu.Unlock()
	sharedDatabases.Lock()
	shared.refs--
	if shared.refs > 0 {
		sharedDatabases.Unlock()
		return nil
	}
	delete(sharedDatabases.items, shared.path)
	sharedDatabases.Unlock()
	return shared.db.Close()
}

func (d *Database) Repository(domain Domain) Repository {
	return Repository{database: d, domain: domain}
}

func (d *Database) Usage() Repository           { return d.Repository(DomainUsage) }
func (d *Database) ModelPrice() Repository      { return d.Repository(DomainModelPrice) }
func (d *Database) Quota() Repository           { return d.Repository(DomainQuota) }
func (d *Database) Settings() Repository        { return d.Repository(DomainSettings) }
func (d *Database) RoutingState() Repository    { return d.Repository(DomainRoutingState) }
func (d *Database) InspectionState() Repository { return d.Repository(DomainInspectionState) }

// SameConnection reports whether two lifecycle leases share the same SQLite
// connection. It is intentionally diagnostic-only; callers must not infer
// domain ownership from it.
func (d *Database) SameConnection(other *Database) bool {
	if d == nil || other == nil {
		return false
	}
	d.mu.RLock()
	left := d.shared
	leftClosed := d.closed
	d.mu.RUnlock()
	other.mu.RLock()
	right := other.shared
	rightClosed := other.closed
	other.mu.RUnlock()
	return !leftClosed && !rightClosed && left != nil && left == right
}

type Repository struct {
	database *Database
	domain   Domain
}

func (r Repository) Domain() Domain { return r.domain }

func (r Repository) SQL() *sql.DB {
	if r.database == nil {
		return nil
	}
	return r.database.SQL()
}

func (r Repository) Transaction(ctx context.Context, run func(*sql.Tx) error) error {
	if run == nil {
		return nil
	}
	db := r.SQL()
	if db == nil {
		return sql.ErrConnDone
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := run(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// RetryBusy centralizes the retry contract used by state and quota writes.
func RetryBusy(ctx context.Context, operation func() error) error {
	if operation == nil {
		return nil
	}
	var err error
	for attempt := 0; attempt < 5; attempt++ {
		if err = operation(); err == nil || !IsBusy(err) {
			return err
		}
		delay := time.Duration(attempt+1) * 100 * time.Millisecond
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
	return err
}

func IsBusy(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "database is locked") ||
		strings.Contains(message, "database table is locked") ||
		strings.Contains(message, "sqlite_busy") ||
		strings.Contains(message, "sqlite_locked")
}
