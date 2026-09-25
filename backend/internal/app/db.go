package app

import (
	"context"
	"crypto/sha256"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	migrationLockID      int64 = 0x53554243554c54
	minimumSchemaVersion       = 21
)

var migrationFilename = regexp.MustCompile(`^(\d{6})_([a-z0-9_]+)\.sql$`)

//go:embed schema.sql migrations
var migrationFS embed.FS

type migration struct {
	Version  int
	Name     string
	SQL      string
	Checksum string
}

func OpenDB(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	if databaseURL == "" {
		return nil, nil
	}
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}

// RunMigrations applies every embedded migration exactly once in version order.
// The current clean schema is migration 1; subsequent migrations live in the
// migrations directory and use the NNNNNN_name.sql filename convention.
func RunMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	if pool == nil {
		return nil
	}
	migrations, err := loadMigrations(migrationFS)
	if err != nil {
		return err
	}
	return runMigrations(ctx, pool, migrations)
}

// CurrentSchemaVersion returns the greatest successfully applied migration.
func CurrentSchemaVersion(ctx context.Context, pool *pgxpool.Pool) (int, error) {
	if pool == nil {
		return 0, nil
	}
	var version int
	err := pool.QueryRow(ctx, `
		select coalesce(max(version), 0)
		from schema_migrations
	`).Scan(&version)
	if err != nil {
		return 0, fmt.Errorf("read schema version: %w", err)
	}
	return version, nil
}

func loadMigrations(source fs.FS) ([]migration, error) {
	initialSQL, err := fs.ReadFile(source, "schema.sql")
	if err != nil {
		return nil, fmt.Errorf("read initial schema: %w", err)
	}
	migrations := []migration{newMigration(1, "initial", initialSQL)}

	entries, err := fs.ReadDir(source, "migrations")
	if err != nil {
		return nil, fmt.Errorf("read migration directory: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".sql" {
			continue
		}
		matches := migrationFilename.FindStringSubmatch(entry.Name())
		if matches == nil {
			return nil, fmt.Errorf("invalid migration filename %q", entry.Name())
		}
		version, err := strconv.Atoi(matches[1])
		if err != nil {
			return nil, fmt.Errorf("parse migration version %q: %w", entry.Name(), err)
		}
		body, err := fs.ReadFile(source, "migrations/"+entry.Name())
		if err != nil {
			return nil, fmt.Errorf("read migration %q: %w", entry.Name(), err)
		}
		migrations = append(migrations, newMigration(version, matches[2], body))
	}

	sort.Slice(migrations, func(i, j int) bool { return migrations[i].Version < migrations[j].Version })
	for index, item := range migrations {
		expected := index + 1
		if item.Version != expected {
			return nil, fmt.Errorf("migration sequence: got version %d, want %d", item.Version, expected)
		}
	}
	return migrations, nil
}

func newMigration(version int, name string, body []byte) migration {
	return migration{
		Version:  version,
		Name:     name,
		SQL:      string(body),
		Checksum: fmt.Sprintf("%x", sha256.Sum256(body)),
	}
}

func runMigrations(ctx context.Context, pool *pgxpool.Pool, migrations []migration) error {
	if err := validateMigrations(migrations); err != nil {
		return err
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin migrations: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `select pg_advisory_xact_lock($1)`, migrationLockID); err != nil {
		return fmt.Errorf("lock migrations: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		create table if not exists schema_migrations (
			version integer primary key check (version > 0),
			name text not null,
			checksum char(64) not null,
			applied_at timestamptz not null default now()
		)
	`); err != nil {
		return fmt.Errorf("create migration ledger: %w", err)
	}

	for _, item := range migrations {
		var storedName, storedChecksum string
		err := tx.QueryRow(ctx, `
			select name, checksum
			from schema_migrations
			where version = $1
		`, item.Version).Scan(&storedName, &storedChecksum)
		switch {
		case err == nil:
			if storedName != item.Name || storedChecksum != item.Checksum {
				return fmt.Errorf("migration %d changed after application", item.Version)
			}
			continue
		case !errors.Is(err, pgx.ErrNoRows):
			return fmt.Errorf("read migration %d: %w", item.Version, err)
		}

		if _, err := tx.Exec(ctx, item.SQL); err != nil {
			return fmt.Errorf("apply migration %d %s: %w", item.Version, item.Name, err)
		}
		if _, err := tx.Exec(ctx, `
			insert into schema_migrations (version, name, checksum)
			values ($1, $2, $3)
		`, item.Version, item.Name, item.Checksum); err != nil {
			return fmt.Errorf("record migration %d: %w", item.Version, err)
		}
	}

	version := 0
	if err := tx.QueryRow(ctx, `select coalesce(max(version), 0) from schema_migrations`).Scan(&version); err != nil {
		return fmt.Errorf("verify schema version: %w", err)
	}
	expectedVersion := migrations[len(migrations)-1].Version
	if version != expectedVersion {
		return fmt.Errorf("database schema version %d does not match binary version %d", version, expectedVersion)
	}
	if version < minimumSchemaVersion {
		return fmt.Errorf("schema version %d is below minimum %d", version, minimumSchemaVersion)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit migrations: %w", err)
	}
	return nil
}

func validateMigrations(migrations []migration) error {
	if len(migrations) == 0 {
		return errors.New("no migrations configured")
	}
	for index, item := range migrations {
		expected := index + 1
		if item.Version != expected {
			return fmt.Errorf("migration sequence: got version %d, want %d", item.Version, expected)
		}
		if item.Name == "" || item.SQL == "" || item.Checksum == "" {
			return fmt.Errorf("migration %d is incomplete", item.Version)
		}
	}
	return nil
}
