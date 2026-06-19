package database

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type Migration struct {
	Version int
	Name    string
	Path    string
	SQL     string
}

type MigrationStore interface {
	Ensure(ctx context.Context) error
	AppliedVersions(ctx context.Context) (map[int]bool, error)
	Apply(ctx context.Context, migration Migration) error
}

type Runner struct {
	store MigrationStore
}

func NewRunner(store MigrationStore) *Runner {
	return &Runner{store: store}
}

func (r *Runner) Up(ctx context.Context, migrations []Migration) ([]Migration, error) {
	if err := r.store.Ensure(ctx); err != nil {
		return nil, err
	}

	applied, err := r.store.AppliedVersions(ctx)
	if err != nil {
		return nil, err
	}

	var ran []Migration
	for _, migration := range migrations {
		if applied[migration.Version] {
			continue
		}
		if err := r.store.Apply(ctx, migration); err != nil {
			return ran, fmt.Errorf("apply migration %06d_%s: %w", migration.Version, migration.Name, err)
		}
		ran = append(ran, migration)
	}

	return ran, nil
}

var migrationFilePattern = regexp.MustCompile(`^([0-9]+)_([a-zA-Z0-9_-]+)\.sql$`)

func LoadMigrations(fsys fs.FS) ([]Migration, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, err
	}

	var migrations []Migration
	seen := make(map[int]string)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		match := migrationFilePattern.FindStringSubmatch(name)
		if match == nil {
			continue
		}

		version, err := strconv.Atoi(match[1])
		if err != nil {
			return nil, fmt.Errorf("parse migration version %q: %w", name, err)
		}
		if previous, exists := seen[version]; exists {
			return nil, fmt.Errorf("duplicate migration version %d in %s and %s", version, previous, name)
		}
		seen[version] = name

		content, err := fs.ReadFile(fsys, name)
		if err != nil {
			return nil, err
		}

		sqlText := strings.TrimSpace(string(content))
		if sqlText == "" {
			return nil, fmt.Errorf("migration %s is empty", name)
		}

		migrations = append(migrations, Migration{
			Version: version,
			Name:    match[2],
			Path:    filepath.ToSlash(name),
			SQL:     sqlText,
		})
	}

	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})

	return migrations, nil
}

type SQLStore struct {
	db *sql.DB
}

func NewSQLStore(db *sql.DB) *SQLStore {
	return &SQLStore{db: db}
}

func (s *SQLStore) Ensure(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS schema_migrations (
	version INTEGER PRIMARY KEY,
	name TEXT NOT NULL,
	applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
)`)
	return err
}

func (s *SQLStore) AppliedVersions(ctx context.Context) (map[int]bool, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	versions := make(map[int]bool)
	for rows.Next() {
		var version int
		if err := rows.Scan(&version); err != nil {
			return nil, err
		}
		versions[version] = true
	}
	return versions, rows.Err()
}

func (s *SQLStore) Apply(ctx context.Context, migration Migration) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, migration.SQL); err != nil {
		return err
	}
	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO schema_migrations (version, name) VALUES ($1, $2)`,
		migration.Version,
		migration.Name,
	); err != nil {
		return err
	}

	return tx.Commit()
}
