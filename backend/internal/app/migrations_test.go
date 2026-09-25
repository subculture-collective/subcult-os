package app

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestLoadMigrationsValidatesSequence(t *testing.T) {
	tests := []struct {
		name    string
		files   fstest.MapFS
		wantErr string
	}{
		{
			name: "ordered",
			files: fstest.MapFS{
				"schema.sql":                     &fstest.MapFile{Data: []byte(`select 1`)},
				"migrations/README.md":           &fstest.MapFile{Data: []byte("docs")},
				"migrations/000002_add_name.sql": &fstest.MapFile{Data: []byte(`select 2`)},
			},
		},
		{
			name: "gap",
			files: fstest.MapFS{
				"schema.sql":                     &fstest.MapFile{Data: []byte(`select 1`)},
				"migrations/000003_skip_two.sql": &fstest.MapFile{Data: []byte(`select 3`)},
			},
			wantErr: "got version 3, want 2",
		},
		{
			name: "invalid filename",
			files: fstest.MapFS{
				"schema.sql":                 &fstest.MapFile{Data: []byte(`select 1`)},
				"migrations/not_ordered.sql": &fstest.MapFile{Data: []byte(`select 2`)},
			},
			wantErr: "invalid migration filename",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			migrations, err := loadMigrations(tt.files)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("loadMigrations() error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(migrations) != 2 || migrations[0].Version != 1 || migrations[1].Version != 2 {
				t.Fatalf("unexpected migrations: %#v", migrations)
			}
			if migrations[0].Checksum == "" || migrations[1].Checksum == "" {
				t.Fatal("expected migration checksums")
			}
		})
	}
}

func TestRunMigrationsTracksReplayAndVersion(t *testing.T) {
	pool := newMigrationTestPool(t)
	ctx := t.Context()

	if err := RunMigrations(ctx, pool); err != nil {
		t.Fatal(err)
	}
	if err := RunMigrations(ctx, pool); err != nil {
		t.Fatalf("replay migrations: %v", err)
	}

	version, err := CurrentSchemaVersion(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	if version != minimumSchemaVersion {
		t.Fatalf("schema version = %d, want %d", version, minimumSchemaVersion)
	}
	var count int
	if err := pool.QueryRow(ctx, `select count(*) from schema_migrations`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != minimumSchemaVersion {
		t.Fatalf("migration ledger rows = %d, want %d", count, minimumSchemaVersion)
	}
}

func TestRunMigrationsRejectsChangedAppliedMigration(t *testing.T) {
	pool := newMigrationTestPool(t)
	ctx := t.Context()
	if err := RunMigrations(ctx, pool); err != nil {
		t.Fatal(err)
	}

	changed := []migration{newMigration(1, "initial", []byte(`select 1`))}
	err := runMigrations(ctx, pool, changed)
	if err == nil || !strings.Contains(err.Error(), "changed after application") {
		t.Fatalf("runMigrations() error = %v, want changed migration rejection", err)
	}
}

func TestRunMigrationsRejectsDatabaseAheadOfBinary(t *testing.T) {
	pool := newMigrationTestPool(t)
	ctx := t.Context()
	if err := RunMigrations(ctx, pool); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		insert into schema_migrations (version, name, checksum)
		values ($1, 'future', $2)
	`, minimumSchemaVersion+1, strings.Repeat("0", 64)); err != nil {
		t.Fatal(err)
	}

	err := RunMigrations(ctx, pool)
	if err == nil || !strings.Contains(err.Error(), "does not match binary version") {
		t.Fatalf("RunMigrations() error = %v, want database-ahead rejection", err)
	}
}

func TestRunMigrationsRollsBackFailedVersion(t *testing.T) {
	pool := newMigrationTestPool(t)
	ctx := t.Context()
	migrations, err := loadMigrations(migrationFS)
	if err != nil {
		t.Fatal(err)
	}
	failedVersion := len(migrations) + 1
	migrations = append(migrations, newMigration(failedVersion, "forced_failure", []byte(`
		create table migration_failure_probe (id integer primary key);
		select * from migration_table_that_does_not_exist;
	`)))

	err = runMigrations(ctx, pool, migrations)
	if err == nil || !strings.Contains(err.Error(), fmt.Sprintf("apply migration %d forced_failure", failedVersion)) {
		t.Fatalf("runMigrations() error = %v, want forced migration failure", err)
	}

	var tableExists bool
	if err := pool.QueryRow(ctx, `select to_regclass('migration_failure_probe') is not null`).Scan(&tableExists); err != nil {
		t.Fatal(err)
	}
	if tableExists {
		t.Fatal("failed migration left its table behind")
	}
	var ledgerExists bool
	if err := pool.QueryRow(ctx, `select to_regclass('schema_migrations') is not null`).Scan(&ledgerExists); err != nil {
		t.Fatal(err)
	}
	if ledgerExists {
		t.Fatal("failed initial transaction left its migration ledger behind")
	}
}

func TestStaffingParticipantRequirementsMigrationUpgradesVersionTwentyFixture(t *testing.T) {
	ctx := t.Context()
	pool := newMigrationTestPool(t)
	migrations, err := loadMigrations(migrationFS)
	if err != nil {
		t.Fatal(err)
	}
	if len(migrations) < 21 {
		t.Fatalf("migrations=%d, want version 21", len(migrations))
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `create table schema_migrations(version integer primary key,name text not null,checksum char(64) not null,applied_at timestamptz not null default now())`); err != nil {
		t.Fatal(err)
	}
	for _, migration := range migrations[:20] {
		if _, err := tx.Exec(ctx, migration.SQL); err != nil {
			t.Fatalf("apply historical migration %d: %v", migration.Version, err)
		}
		if _, err := tx.Exec(ctx, `insert into schema_migrations(version,name,checksum) values($1,$2,$3)`, migration.Version, migration.Name, migration.Checksum); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := RunMigrations(ctx, pool); err != nil {
		t.Fatalf("upgrade version-20 fixture: %v", err)
	}
	var isNullable, defaultValue string
	if err := pool.QueryRow(ctx, `
		select is_nullable, column_default
		from information_schema.columns
		where table_schema = current_schema()
		  and table_name = 'event_staffing_items'
		  and column_name = 'participant_requirements'
	`).Scan(&isNullable, &defaultValue); err != nil {
		t.Fatal(err)
	}
	if isNullable != "NO" || defaultValue != "''::text" {
		t.Fatalf("participant requirements column nullable=%q default=%q, want NO and empty text", isNullable, defaultValue)
	}
}

func TestRunMigrationsSerializesConcurrentRunners(t *testing.T) {
	pool := newMigrationTestPool(t)
	ctx := t.Context()

	var wait sync.WaitGroup
	errorsFound := make(chan error, 2)
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			errorsFound <- RunMigrations(ctx, pool)
		}()
	}
	wait.Wait()
	close(errorsFound)
	for err := range errorsFound {
		if err != nil {
			t.Fatal(err)
		}
	}

	var count int
	if err := pool.QueryRow(ctx, `select count(*) from schema_migrations`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != minimumSchemaVersion {
		t.Fatalf("migration ledger rows = %d, want %d", count, minimumSchemaVersion)
	}
}

func TestIdentityFoundationRejectsPopulatedPrototypeAccounts(t *testing.T) {
	pool := newMigrationTestPool(t)
	ctx := t.Context()

	initial := newMigration(1, "initial", mustReadMigrationFile(t, "schema.sql"))
	if _, err := pool.Exec(ctx, initial.SQL); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		create table schema_migrations (
			version integer primary key check (version > 0),
			name text not null,
			checksum char(64) not null,
			applied_at timestamptz not null default now()
		)
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		insert into schema_migrations (version, name, checksum) values ($1, $2, $3)
	`, initial.Version, initial.Name, initial.Checksum); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		insert into people (email, password_hash)
		values ('retained@example.test', 'bcrypt$placeholder')
	`); err != nil {
		t.Fatal(err)
	}

	err := RunMigrations(ctx, pool)
	if err == nil || !strings.Contains(err.Error(), "inventory retained accounts before migration") {
		t.Fatalf("RunMigrations() error = %v, want retained-account inventory gate", err)
	}

	version, err := CurrentSchemaVersion(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	if version != 1 {
		t.Fatalf("schema version = %d, want failed migration to preserve version 1", version)
	}
	var email string
	if err := pool.QueryRow(ctx, `select email from people`).Scan(&email); err != nil {
		t.Fatal(err)
	}
	if email != "retained@example.test" {
		t.Fatalf("retained account changed to %q", email)
	}
}

func TestIdentityFoundationCreatesCanonicalTables(t *testing.T) {
	pool := newMigrationTestPool(t)
	ctx := t.Context()
	if err := RunMigrations(ctx, pool); err != nil {
		t.Fatal(err)
	}

	for _, table := range []string{"email_identities", "identity_challenges", "identity_sessions", "did_links", "auth_audit_events", "atproto_oauth_requests", "atproto_oauth_sessions"} {
		var exists bool
		if err := pool.QueryRow(ctx, `select to_regclass($1) is not null`, table).Scan(&exists); err != nil {
			t.Fatal(err)
		}
		if !exists {
			t.Fatalf("expected %s table", table)
		}
	}
}

func TestPrototypeSessionRemovalRejectsPopulatedTable(t *testing.T) {
	pool := newMigrationTestPool(t)
	ctx := t.Context()
	migrations, err := loadMigrations(migrationFS)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, migrations[0].SQL); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		create table schema_migrations (
			version integer primary key check (version > 0),
			name text not null,
			checksum char(64) not null,
			applied_at timestamptz not null default now()
		)
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `insert into schema_migrations (version, name, checksum) values ($1, $2, $3)`, migrations[0].Version, migrations[0].Name, migrations[0].Checksum); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, migrations[1].SQL); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `insert into schema_migrations (version, name, checksum) values ($1, $2, $3)`, migrations[1].Version, migrations[1].Name, migrations[1].Checksum); err != nil {
		t.Fatal(err)
	}
	var personID string
	if err := pool.QueryRow(ctx, `
		insert into people (email, password_hash) values ('prototype-session@example.test', 'bcrypt$placeholder')
		returning id
	`).Scan(&personID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `insert into sessions (person_id, token_hash, expires_at) values ($1, 'prototype-token', now() + interval '1 hour')`, personID); err != nil {
		t.Fatal(err)
	}

	err = RunMigrations(ctx, pool)
	if err == nil || !strings.Contains(err.Error(), "prototype sessions require inventory before removal") {
		t.Fatalf("RunMigrations() error = %v, want prototype-session inventory gate", err)
	}
	var version int
	if versionErr := pool.QueryRow(ctx, `select coalesce(max(version), 0) from schema_migrations`).Scan(&version); versionErr != nil {
		t.Fatal(versionErr)
	}
	if version != 2 {
		t.Fatalf("schema version = %d, want failed migration to preserve version 2", version)
	}
}

func mustReadMigrationFile(t *testing.T, name string) []byte {
	t.Helper()
	body, err := migrationFS.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func newMigrationTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set TEST_DATABASE_URL to run migration integration tests")
	}

	ctx := t.Context()
	basePool, err := OpenDB(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("migration_test_%d", time.Now().UnixNano())
	identifier := pgx.Identifier{schema}.Sanitize()
	if _, err := basePool.Exec(ctx, "create schema "+identifier); err != nil {
		basePool.Close()
		t.Fatal(err)
	}

	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		_, _ = basePool.Exec(context.Background(), "drop schema "+identifier+" cascade")
		basePool.Close()
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		_, _ = basePool.Exec(context.Background(), "drop schema "+identifier+" cascade")
		basePool.Close()
		t.Fatal(err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		_, _ = basePool.Exec(context.Background(), "drop schema "+identifier+" cascade")
		basePool.Close()
		t.Fatal(err)
	}

	t.Cleanup(func() {
		pool.Close()
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := basePool.Exec(cleanupCtx, "drop schema "+identifier+" cascade"); err != nil {
			t.Errorf("drop migration test schema: %v", err)
		}
		basePool.Close()
	})
	return pool
}
