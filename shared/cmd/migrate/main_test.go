package main

import (
	"crypto/sha256"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestMigrationValidation(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "000001_initial.up.sql"), "SELECT 1;")
	if _, err := inspect(dir); err == nil {
		t.Fatal("accepted missing down")
	}
	write(t, filepath.Join(dir, "000001_initial.down.sql"), "SELECT 1;")
	files, err := inspect(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err = create(dir, "post", "../escape", files); err == nil {
		t.Fatal("accepted path traversal")
	}
	if err = create(dir, "post", "", files); err == nil {
		t.Fatal("accepted missing name")
	}
	if err = create(dir, "post", "add_field", files); err != nil {
		t.Fatal(err)
	}
	if _, err = inspect(dir); err != nil {
		t.Fatal(err)
	}
	if err = create(dir, "post", "add_field", files); err == nil {
		t.Fatal("overwrote existing migration")
	}
	write(t, filepath.Join(dir, "000004_gap.up.sql"), "SELECT 1;")
	write(t, filepath.Join(dir, "000004_gap.down.sql"), "SELECT 1;")
	if _, err = inspect(dir); err == nil {
		t.Fatal("accepted sequence gap")
	}
}

func TestDatabaseConfiguration(t *testing.T) {
	t.Setenv("ENV_FILE", filepath.Join(t.TempDir(), ".env"))
	write(t, os.Getenv("ENV_FILE"), "POSTGRES_HOST=ignored\n")
	t.Setenv("DATABASE_URL", "")
	t.Setenv("POSTGRES_HOST", "localhost")
	t.Setenv("POSTGRES_PORT", "15432")
	t.Setenv("POSTGRES_USER", "test")
	t.Setenv("POSTGRES_PASSWORD", "p@ss:/?#")
	dsn, err := databaseURL()
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	password, _ := u.User.Password()
	if password != "p@ss:/?#" || u.Host != "localhost:15432" {
		t.Fatal("credentials or port changed")
	}
	t.Setenv("DATABASE_URL", "postgres://u:secret@remote:5432/test?sslmode=require&search_path=wrong")
	dsn, err = databaseURL()
	if err != nil {
		t.Fatal(err)
	}
	u, _ = url.Parse(dsn)
	if u.Host != "remote:5432" || u.Query().Get("sslmode") != "require" || u.Query().Get("search_path") != "public" {
		t.Fatal("URL override incorrect")
	}
	t.Setenv("DATABASE_URL", "postgres://u:secret@remote/test?x-migrations-table=shared")
	if _, err = databaseURL(); err == nil {
		t.Fatal("accepted history override")
	}
	t.Setenv("DATABASE_URL", "postgres://u:secret%@remote/test")
	if _, err = databaseURL(); err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatal("invalid URL accepted or secret exposed")
	}
}

func TestGlobalDownRequiresConfirmation(t *testing.T) {
	t.Setenv("CONFIRM", "")
	if err := run(t.TempDir(), []string{"all", "down"}); err == nil || !strings.Contains(err.Error(), "CONFIRM=yes") {
		t.Fatal(err)
	}
}

// Integration tests create and drop only their own uniquely named databases.
// Supply an admin URL to an isolated PostgreSQL instance with CREATEDB privileges.
func TestPostgresWorkflow(t *testing.T) {
	adminURL := os.Getenv("MIGRATION_TEST_DATABASE_URL")
	if adminURL == "" {
		t.Skip("set MIGRATION_TEST_DATABASE_URL to an isolated PostgreSQL instance")
	}
	sourceRoot := os.Getenv("MIGRATION_SOURCE_ROOT")
	if sourceRoot == "" {
		sourceRoot = filepath.Join("..", "..", "..")
	}
	root := t.TempDir()
	for _, s := range services {
		source := migrationDir(sourceRoot, s)
		files, err := inspect(source)
		if err != nil {
			t.Fatal(err)
		}
		// Pin the original seven migrations as the transition regression fixture;
		// future application migrations should not change these test scenarios.
		baseline := 1
		if s == "auth" || s == "user" {
			baseline = 2
		}
		if len(files) < baseline {
			t.Fatal("missing historical migration baseline")
		}
		for _, f := range files[:baseline] {
			for _, p := range []string{f.up, f.down} {
				b, err := os.ReadFile(p)
				if err != nil {
					t.Fatal(err)
				}
				write(t, filepath.Join(migrationDir(root, s), filepath.Base(p)), string(b))
			}
		}
	}
	admin, err := sql.Open("postgres", adminURL)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	newDB := func(label string) *sql.DB {
		name := fmt.Sprintf("migration_test_%s_%d", label, time.Now().UnixNano())
		if _, err := admin.Exec("CREATE DATABASE " + name); err != nil {
			t.Fatal(err)
		}
		u, err := url.Parse(adminURL)
		if err != nil {
			t.Fatal(err)
		}
		u.Path = "/" + name
		t.Setenv("DATABASE_URL", u.String())
		db, err := sql.Open("postgres", u.String())
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			db.Close()
			cleanup, err := sql.Open("postgres", adminURL)
			if err != nil {
				t.Error(err)
				return
			}
			defer cleanup.Close()
			if _, err = cleanup.Exec("DROP DATABASE " + name + " WITH (FORCE)"); err != nil {
				t.Error(err)
			}
		})
		return db
	}
	t.Setenv("ENV_FILE", filepath.Join(t.TempDir(), ".env"))
	write(t, os.Getenv("ENV_FILE"), "")
	mustRun := func(s, a string) {
		t.Helper()
		if err := run(root, []string{s, a}); err != nil {
			t.Fatal(err)
		}
	}
	version := func(db *sql.DB, s string, want int, dirtyWant bool) {
		t.Helper()
		var v int
		var dirty bool
		if err := db.QueryRow("SELECT version,dirty FROM public.schema_migrations_"+s).Scan(&v, &dirty); err != nil {
			t.Fatal(err)
		}
		if v != want || dirty != dirtyWant {
			t.Fatalf("%s version=%d dirty=%v", s, v, dirty)
		}
	}
	db := newDB("fresh")
	if err := run(root, []string{"auth", "up"}); err == nil {
		t.Fatal("auth skipped user prerequisite")
	}
	mustRun("all", "up")
	mustRun("all", "up")
	mustRun("all", "status")
	for _, s := range services {
		want := 1
		if s == "auth" || s == "user" {
			want = 2
		}
		version(db, s, want, false)
	}
	if err := run(root, []string{"auth", "down"}); err == nil {
		t.Fatal("allowed irreversible rollback")
	}
	version(db, "auth", 2, false)
	t.Setenv("CONFIRM", "yes")
	if err := run(root, []string{"all", "down"}); err == nil {
		t.Fatal("global down bypassed auth refusal")
	}
	version(db, "post", 1, false)
	t.Setenv("name", "add_probe")
	mustRun("post", "create")
	if err := run(root, []string{"post", "up"}); err == nil {
		t.Fatal("applied empty template")
	}
	dir := filepath.Join(root, "services", "post-service", "migrations")
	write(t, filepath.Join(dir, "000002_add_probe.up.sql"), "BEGIN; CREATE TABLE migration_probe(id integer PRIMARY KEY); COMMIT;")
	write(t, filepath.Join(dir, "000002_add_probe.down.sql"), "BEGIN; DROP TABLE migration_probe; COMMIT;")
	mustRun("post", "up")
	version(db, "post", 2, false)
	mustRun("post", "down")
	version(db, "post", 1, false)
	mustRun("post", "up")
	write(t, filepath.Join(dir, "000003_failure.up.sql"), "BEGIN; CREATE TABLE should_rollback(id integer); SELECT missing_column; COMMIT;")
	write(t, filepath.Join(dir, "000003_failure.down.sql"), "DROP TABLE IF EXISTS should_rollback;")
	if err := run(root, []string{"post", "up"}); err == nil {
		t.Fatal("ignored SQL failure")
	}
	version(db, "post", 3, true)
	var absent bool
	if err := db.QueryRow("SELECT to_regclass('public.should_rollback') IS NULL").Scan(&absent); err != nil || !absent {
		t.Fatal("failed SQL was not rolled back", err)
	}
	if err := run(root, []string{"post", "status"}); err == nil {
		t.Fatal("dirty status succeeded")
	}
	if err := run(root, []string{"post", "up"}); err == nil {
		t.Fatal("dirty database accepted")
	}
	// Restore only this temporary fixture to the real historical migrations.
	for _, n := range []string{"000002_add_probe.up.sql", "000002_add_probe.down.sql", "000003_failure.up.sql", "000003_failure.down.sql"} {
		if err := os.Remove(filepath.Join(dir, n)); err != nil {
			t.Fatal(err)
		}
	}
	legacy := newDB("legacy")
	if _, err := legacy.Exec(`CREATE TABLE public.schema_migrations(service text,version text,checksum text,applied_at timestamptz DEFAULT now(),PRIMARY KEY(service,version))`); err != nil {
		t.Fatal(err)
	}
	for _, s := range services {
		files, err := inspect(migrationDir(root, s))
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range files {
			b, err := os.ReadFile(f.up)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = legacy.Exec(string(b)); err != nil {
				t.Fatal(err)
			}
			if _, err = legacy.Exec(`INSERT INTO public.schema_migrations(service,version,checksum) VALUES($1,$2,$3)`, s+"-service", filepath.Base(f.up), fmt.Sprintf("%x", sha256.Sum256(b))); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := run(root, []string{"user", "up"}); err == nil {
		t.Fatal("replayed legacy SQL")
	}
	if _, err = legacy.Exec(`UPDATE public.schema_migrations SET checksum='bad' WHERE service='post-service'`); err != nil {
		t.Fatal(err)
	}
	if err := run(root, []string{"post", "import-legacy"}); err == nil {
		t.Fatal("accepted checksum mismatch")
	}
	files, _ := inspect(dir)
	b, _ := os.ReadFile(files[0].up)
	if _, err = legacy.Exec(`UPDATE public.schema_migrations SET checksum=$1 WHERE service='post-service'`, fmt.Sprintf("%x", sha256.Sum256(b))); err != nil {
		t.Fatal(err)
	}
	if _, err = legacy.Exec(`INSERT INTO public.posts(id,author_user_id,content) VALUES(gen_random_uuid(),gen_random_uuid(),'preserve me')`); err != nil {
		t.Fatal(err)
	}
	mustRun("all", "import-legacy")
	mustRun("all", "up")
	if err := run(root, []string{"post", "import-legacy"}); err == nil {
		t.Fatal("overwrote new history")
	}
	var count int
	if err = legacy.QueryRow(`SELECT count(*) FROM posts WHERE content='preserve me'`).Scan(&count); err != nil || count != 1 {
		t.Fatal("legacy data changed", err)
	}
	mustRun("post", "down")
	mustRun("post", "up")
	version(legacy, "post", 1, false)
}
