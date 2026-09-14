// Command migrate runs explicit, service-owned SQL migrations. It never runs in a server.
package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	envfolder "github.com/Khalid-Abdullahi-Isse/social-media-backend/shared/Envfolder"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/lib/pq"
)

var services = []string{"user", "auth", "post", "chat", "notification"}
var filenameRE = regexp.MustCompile(`^([0-9]{6})_([a-z][a-z0-9_]*?)\.(up|down)\.sql$`)
var nameRE = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

type migrationFile struct {
	version  int
	up, down string
}

func main() {
	root := flag.String("root", ".", "backend directory")
	flag.Parse()
	if err := run(*root, flag.Args()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(root string, args []string) error {
	if len(args) != 2 {
		return errors.New("usage: migrate [-root DIR] SERVICE|all up|down|status|create|import-legacy")
	}
	service, action := args[0], args[1]
	if action != "up" && action != "down" && action != "status" && action != "create" && action != "import-legacy" {
		return fmt.Errorf("unknown action %q", action)
	}
	selected := []string{service}
	if service == "all" {
		if action == "create" {
			return errors.New("select one service to create a migration")
		}
		selected = append([]string(nil), services...)
		if action == "down" {
			if os.Getenv("CONFIRM") != "yes" {
				return errors.New("global rollback requires make migrate-down CONFIRM=yes; this rolls back ONE migration per service and may delete data")
			}
			// Auth's historical import rollback is deliberately irreversible. Try it first
			// so this known refusal cannot leave other services partially rolled back.
			selected = []string{"auth", "notification", "chat", "post", "user"}
		}
	} else {
		valid := false
		for _, s := range services {
			if s == service {
				valid = true
			}
		}
		if !valid {
			return fmt.Errorf("unknown service %q", service)
		}
	}
	// Validate every directory before changing any database.
	files := map[string][]migrationFile{}
	for _, s := range selected {
		dir := migrationDir(root, s)
		list, err := inspect(dir)
		if err != nil {
			return err
		}
		files[s] = list
	}
	if action == "create" {
		return create(migrationDir(root, service), service, os.Getenv("name"), files[service])
	}
	dsn, err := databaseURL()
	if err != nil {
		return err
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return errors.New("invalid PostgreSQL connection configuration")
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	conn, err := db.Conn(ctx)
	if err != nil {
		return errors.New("cannot connect to PostgreSQL; check DATABASE_URL or POSTGRES_* settings")
	}
	defer conn.Close()
	// Also coordinates with the old Docker runner during the history transition.
	if _, err = conn.ExecContext(ctx, `SELECT pg_advisory_lock(735091284)`); err != nil {
		return fmt.Errorf("acquire migration lock: %w", err)
	}
	defer conn.ExecContext(context.Background(), `SELECT pg_advisory_unlock(735091284)`)
	for _, s := range selected {
		fmt.Printf("%s: %s\n", s, action)
		if err := execute(conn, root, s, action, files[s]); err != nil {
			return fmt.Errorf("%s: %w", s, err)
		}
	}
	return nil
}

func databaseURL() (string, error) {
	cfg, err := envfolder.Load()
	if err != nil {
		return "", err
	}
	raw := os.Getenv("DATABASE_URL")
	var u *url.URL
	if raw != "" {
		u, err = url.Parse(raw)
		if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || u.Host == "" {
			return "", errors.New("DATABASE_URL must be a valid postgres:// URL")
		}
	} else {
		u = &url.URL{Scheme: "postgres", User: url.UserPassword(cfg.PostgresUser, cfg.PostgresPassword), Host: net.JoinHostPort(cfg.PostgresHost, cfg.PostgresPort), Path: "/" + cfg.PostgresDB}
	}
	q := u.Query()
	for k := range q {
		if strings.HasPrefix(k, "x-") {
			return "", errors.New("DATABASE_URL must not contain migrate x-* options; service history is managed by this runner")
		}
	}
	if q.Get("sslmode") == "" {
		mode := os.Getenv("POSTGRES_SSLMODE")
		if mode == "" {
			mode = "disable"
		}
		q.Set("sslmode", mode)
	}
	q.Set("search_path", "public")
	q.Set("connect_timeout", "5")
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func inspect(dir string) ([]migrationFile, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read service migrations: %w", err)
	}
	byVersion := map[int]*migrationFile{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		match := filenameRE.FindStringSubmatch(entry.Name())
		if match == nil {
			return nil, fmt.Errorf("invalid sequential migration filename: %s", entry.Name())
		}
		v, _ := strconv.Atoi(match[1])
		if v == 0 {
			return nil, errors.New("migration versions start at 000001")
		}
		f := byVersion[v]
		if f == nil {
			f = &migrationFile{version: v}
			byVersion[v] = f
		}
		path := filepath.Join(dir, entry.Name())
		if match[3] == "up" {
			if f.up != "" {
				return nil, fmt.Errorf("duplicate version %d", v)
			}
			f.up = path
		} else {
			if f.down != "" {
				return nil, fmt.Errorf("duplicate version %d", v)
			}
			f.down = path
		}
	}
	result := make([]migrationFile, 0, len(byVersion))
	for v := 1; v <= len(byVersion); v++ {
		f := byVersion[v]
		if f == nil || f.up == "" || f.down == "" {
			return nil, fmt.Errorf("%s: migration %06d must have a matching up/down pair, with no sequence gaps", dir, v)
		}
		if strings.TrimSuffix(f.up, ".up.sql") != strings.TrimSuffix(f.down, ".down.sql") {
			return nil, fmt.Errorf("migration %d filenames do not match", v)
		}
		result = append(result, *f)
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("no migrations in %s", dir)
	}
	return result, nil
}

func create(dir, service, name string, files []migrationFile) error {
	if !nameRE.MatchString(name) {
		return fmt.Errorf("Usage: make migration-%s name=add_example (lowercase letters, digits, underscores)", service)
	}
	version := len(files) + 1
	if version > 999999 {
		return errors.New("six-digit migration sequence exhausted")
	}
	paths := []string{}
	for _, direction := range []string{"up", "down"} {
		path := filepath.Join(dir, fmt.Sprintf("%06d_%s.%s.sql", version, name, direction))
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
		if err != nil {
			for _, p := range paths {
				os.Remove(p)
			}
			return err
		}
		paths = append(paths, path)
		_, err = f.WriteString("BEGIN;\n\n-- TODO: write and review the " + direction + " migration before applying.\n\nCOMMIT;\n")
		closeErr := f.Close()
		if err == nil {
			err = closeErr
		}
		if err != nil {
			for _, p := range paths {
				os.Remove(p)
			}
			return err
		}
	}
	for _, p := range paths {
		fmt.Println(p)
	}
	return nil
}

func execute(conn *sql.Conn, root, service, action string, files []migrationFile) error {
	ctx := context.Background()
	table := "schema_migrations_" + service
	var legacy, current bool
	if err := conn.QueryRowContext(ctx, `SELECT to_regclass('public.schema_migrations') IS NOT NULL, to_regclass($1) IS NOT NULL`, "public."+table).Scan(&legacy, &current); err != nil {
		return err
	}
	if action == "import-legacy" {
		if current {
			return errors.New("service history already exists; refusing to overwrite it")
		}
		if !legacy {
			return errors.New("no legacy history exists; use up for a fresh database")
		}
		return importLegacy(conn, service, table, files)
	}
	if legacy && !current {
		return errors.New("legacy history detected; stop the old runner, back up the database, then run make migrate-" + service + "-import-legacy")
	}
	if action == "status" && !current {
		fmt.Println("unversioned (no history table)")
		return nil
	}
	if action == "down" && !current {
		return errors.New("no migration history; nothing to roll back")
	}
	// Auth's historical account import reads user-owned legacy profiles. Do not
	// allow a standalone auth-up to silently skip that transfer on a fresh DB.
	if service == "auth" && action == "up" {
		var version int
		if current {
			err := conn.QueryRowContext(ctx, `SELECT COALESCE(MAX(version),0) FROM public.schema_migrations_auth`).Scan(&version)
			if err != nil {
				return err
			}
		}
		if version < 2 {
			var userHistory bool
			if err := conn.QueryRowContext(ctx, `SELECT to_regclass('public.schema_migrations_user') IS NOT NULL`).Scan(&userHistory); err != nil {
				return err
			}
			if !userHistory {
				return errors.New("run make migrate-user-up first; auth migration 000002 depends on user migration 000002")
			}
			var ready bool
			if err := conn.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM public.schema_migrations_user WHERE version >= 2 AND NOT dirty)`).Scan(&ready); err != nil {
				return err
			}
			if !ready {
				return errors.New("run make migrate-user-up first; user migration 000002 must be clean")
			}
		}
	}
	// Catch unedited templates rather than recording empty schema changes.
	if action == "up" {
		for _, f := range files {
			b, err := os.ReadFile(f.up)
			if err != nil {
				return err
			}
			if strings.Contains(string(b), "-- TODO: write and review") {
				return fmt.Errorf("edit migration before applying: %s", f.up)
			}
		}
	}
	driver, err := postgres.WithConnection(ctx, conn, &postgres.Config{SchemaName: "public", MigrationsTable: table})
	if err != nil {
		return err
	}
	dir, err := filepath.Abs(migrationDir(root, service))
	if err != nil {
		return err
	}
	source := (&url.URL{Scheme: "file", Path: dir}).String()
	m, err := migrate.NewWithDatabaseInstance(source, "postgres", driver)
	if err != nil {
		return err
	}
	// The connection is shared across services and closed by run, not by m.Close.
	if action == "status" {
		v, dirty, e := m.Version()
		if errors.Is(e, migrate.ErrNilVersion) {
			fmt.Println("unversioned")
			return nil
		}
		if e != nil {
			return e
		}
		fmt.Printf("version=%d dirty=%t\n", v, dirty)
		if dirty {
			return errors.New("dirty migration: inspect and repair before proceeding")
		}
		return nil
	}
	if action == "up" {
		err = m.Up()
	} else {
		v, _, e := m.Version()
		if e != nil {
			return e
		}
		// Avoid marking history dirty for the known intentionally irreversible import.
		if service == "auth" && v == 2 {
			return errors.New("auth migration 000002 is intentionally irreversible; an operator-reviewed identity recovery plan is required")
		}
		if v > 0 && int(v) <= len(files) {
			b, e := os.ReadFile(files[v-1].down)
			if e != nil {
				return e
			}
			if strings.Contains(string(b), "-- TODO: write and review") {
				return errors.New("edit the down migration before rollback")
			}
		}
		err = m.Steps(-1)
	}
	if errors.Is(err, migrate.ErrNoChange) {
		fmt.Println("no change")
		return nil
	}
	return err
}

// Import only a checksum-verified contiguous prefix, atomically. No application SQL
// runs and the original audit ledger remains intact. Never infer state from tables.
func importLegacy(conn *sql.Conn, service, table string, files []migrationFile) error {
	ctx := context.Background()
	rows, err := conn.QueryContext(ctx, `SELECT version,checksum FROM public.schema_migrations WHERE service=$1 ORDER BY version`, service+"-service")
	if err != nil {
		return err
	}
	count := 0
	for rows.Next() {
		var name, checksum string
		if err = rows.Scan(&name, &checksum); err != nil {
			rows.Close()
			return err
		}
		if count >= len(files) || name != filepath.Base(files[count].up) {
			rows.Close()
			return errors.New("legacy history is not a contiguous prefix of local migrations")
		}
		b, e := os.ReadFile(files[count].up)
		if e != nil {
			rows.Close()
			return e
		}
		if fmt.Sprintf("%x", sha256.Sum256(b)) != checksum {
			rows.Close()
			return fmt.Errorf("legacy checksum mismatch: %s", name)
		}
		count++
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if count == 0 {
		return errors.New("no legacy entries for this service; verify and baseline manually")
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// table is derived exclusively from the fixed service allowlist.
	if _, err = tx.ExecContext(ctx, `CREATE TABLE public.`+table+` (version bigint NOT NULL PRIMARY KEY, dirty boolean NOT NULL)`); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO public.`+table+` (version,dirty) VALUES ($1,false)`, count); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	fmt.Printf("imported verified legacy version %d; original ledger retained\n", count)
	return nil
}

// The retired user runtime is not rebuilt; its immutable schema history is retained.
func migrationDir(root, service string) string {
	if service == "user" {
		return filepath.Join(root, "docker", "migrations", "legacy-user")
	}
	return filepath.Join(root, "services", service+"-service", "migrations")
}
