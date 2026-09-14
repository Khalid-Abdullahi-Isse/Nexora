# Database migrations

Changing a Go struct does NOT automatically change PostgreSQL. Every service owns
explicit sequential SQL in `services/<service>-service/migrations/`. The retired
user runtime's immutable history is archived in `docker/migrations/legacy-user/`. Server startup
never runs schema synchronization or down migrations. GORM remains a query layer;
there is no AutoMigrate.

## Architecture and commands

The existing architecture is **one PostgreSQL database (`social_media`), with all
application tables in `public`**, not five databases or five schemas. Keep service
ownership despite the shared namespace. The runner in `shared/cmd/migrate` uses the
pinned `github.com/golang-migrate/migrate/v4` PostgreSQL and file drivers. Go builds
it via Make; no separately installed `migrate` CLI or `migration/migrate.go` is needed.

| Command | Operation |
| --- | --- |
| `make migration-auth name=add_refresh_tokens` | Create the next numbered up/down pair |
| `make migrate-auth-up` | Apply pending auth migrations |
| `make migrate-auth-down` | Roll back exactly ONE auth migration |
| `make migrate-auth-status` | Show auth version and dirty state |
| `make migrate-up` | Apply user → auth → post → chat → notification, stopping on error |
| `make migrate-status` | Show all service versions; dirty state returns an error |
| `make migrate-down CONFIRM=yes` | Roll back ONE migration per service; may delete data |
| `make migrate-auth-import-legacy` | Explicitly import verified old auth history |
| `make migrate-import-legacy` | Import verified old history for every service |

Replace `auth` with `user`, `post`, `chat`, or `notification` for every per-service
command. Global commands execute sequentially, including under `make -j`. Do not
request contradictory targets (up and down) together. A database advisory lock
serializes runner operations and coordinates with the retired runner's lock.
Each service has its own `public.schema_migrations_<service>` table containing
version and dirty state. Never share one golang-migrate history table across services.

Auth migration 000002 is a historical one-time import from user profiles, so user
migration 000002 must precede it. Standalone auth-up checks that prerequisite.
The historical auth-000002 down SQL intentionally raises an exception to prevent
identity loss. The runner refuses it before changing history. Global down attempts
auth first for this reason, then notification, chat, post, user. It is not an atomic
rollback of all services; successful earlier steps remain if a later step fails.
Prefer explicit per-service down in an isolated database, after reviewing its SQL.

## Connections

The runner uses the existing environment loader: nearest `.env`, or `ENV_FILE`,
with exported environment variables taking precedence. `DATABASE_URL` overrides
`POSTGRES_HOST`, `POSTGRES_PORT`, `POSTGRES_USER`, `POSTGRES_PASSWORD`, and
`POSTGRES_DB` for migration operations only. `POSTGRES_SSLMODE` supplies sslmode
when absent from the URL. The runner fixes search_path to public and owns history
configuration; migrate-specific `x-*` URL options are rejected.

Use a private `DATABASE_URL` with a DDL-capable migration identity and your actual
credentials (properly percent-encoded). Never put real credentials in documentation
or shell history. The generated local `.env` configures the application as
`app_auth`, which intentionally cannot perform DDL. Compose passes a separate
migration identity. Existing databases retain their existing passwords until an
operator rotates them; generating `.env` does not change stored PostgreSQL roles.
Applications use POSTGRES_*; DATABASE_URL does not reconfigure them.

## Daily workflow

1. Update the Go model/domain type and relevant persistence code.
2. Run `make migration-auth name=add_refresh_tokens`.
3. Edit BOTH generated SQL files. Auth currently has versions through 000004 (secure sessions and runtime grants);
   the next version is 000005. Other active services have runtime grants at 000002.
   Do not edit already-applied historical SQL.
4. Review the SQL, then run `make migrate-auth-up` (use `make migrate-up` to
   initialize all services on an empty database).
5. Run `make migrate-status`, then start the service and run `make check`.
6. To undo the newest reversible change in a test environment, run
   `make migrate-auth-down`, then reapply with `make migrate-auth-up`.

Creation validates a lowercase SQL filename name, paired six-digit sequential
versions, and no gaps or duplicate versions. Generated files contain BEGIN/COMMIT
and a TODO marker; applying unedited templates is rejected. Remove that marker when
you write the real SQL. Refresh token generations now extend `sessions`; `session_families` records their
absolute expiry and revocation. Do not introduce a duplicate session system.

## Existing databases: transition from the old Docker runner

The removed `docker/migrations/run.sh` used psql and a custom
`public.schema_migrations(service, version, checksum, applied_at)` ledger. That is
not golang-migrate's table format. It is retained as an audit record and never
used as a golang-migrate version table.

Before the first new runner deployment:

1. Back up the database and stop any deployment using the old migration runner.
   Never run the old image again after the transition: its ledger will no longer
   record new changes or rollbacks.
2. Verify that this checkout contains the original applied SQL files.
3. Run `make migrate-import-legacy` against that database (or inside Docker:
   `docker compose -f docker/docker-compose.yml run --build --rm migrate all import-legacy`).
4. Run `make migrate-status`, then `make migrate-up`.

Import checks every recorded filename and SHA-256 checksum against a contiguous
prefix of each service's local files. It creates only that service's history table,
atomically recording the verified version without executing application SQL.
It refuses checksum mismatches, gaps, unknown migrations, missing service history,
and overwriting an existing new history table. If a multi-service import stops,
fix the cause and import only the remaining services individually. A service already
imported should not be re-imported. The normal up/down commands refuse legacy
history without that service's new history table, rather than reapply old SQL.

A database with application tables but no trustworthy ledger requires an operator
schema AND data-migration audit against a fresh database before a manual baseline.
Do not infer completion from table existence. No automatic force or repair command
is exposed. golang-migrate does not check historical file checksums after this
one-time import; protect applied files through review and source control.

## Docker deployment

```sh
docker compose -f docker/docker-compose.yml up -d postgres
docker compose -f docker/docker-compose.yml run --build --rm migrate
make docker-up
```

The migration image uses the same Go runner and read-only per-service SQL mounts.
It connects to `postgres:5432` and only applies up migrations as the existing
explicit Compose deployment job. Applications wait for successful completion.
`Exited (0)` is normal for that job. Later deployments must rerun the job explicitly
with `run --build --rm migrate`; do not assume a previously exited container reruns.
No volumes need deletion. `make docker-clean` remains an existing destructive
volume-removal command and is not part of the migration workflow.

## Safety and recovery

- Never edit an applied production migration. Add a new numbered pair instead.
- Every up must have a reviewed down. Some data transformations cannot be reversed
  safely: document a recovery plan rather than pretending rollback restores data.
- Review table/column drops and data loss explicitly. A down that drops a column
  cannot recover its values. Backups and staged deployment remain necessary.
- Use transactions where PostgreSQL supports them. The existing SQL is sent as one
  statement batch per file; generated files have explicit BEGIN/COMMIT. The default
  PostgreSQL driver does not split migration files. Concurrent-index operations
  require a separately reviewed nontransactional migration strategy.
- Specify indexes, unique/check constraints, foreign keys and ON DELETE behavior
  explicitly. Use IDs without cross-service foreign keys for externally owned data.
- For NOT NULL columns on populated tables, add nullable, backfill, validate, then
  enforce in later migrations. Prefer compatible multi-stage production changes.
- Never place secrets in SQL and never run destructive down at application startup.
- A failed SQL migration can leave golang-migrate's version marked dirty even when
  PostgreSQL rolls back the SQL. Inspect actual schema/data, restore or repair with
  an operator-reviewed plan, and only then correct the version. Never blindly force
  a version or modify a historical file to bypass a failure.

## Inspection findings and scope

All inspected tables have UUID primary/composite keys, appropriate internal foreign
keys with ON DELETE clauses, and explicit indexes/constraints. No invalid PostgreSQL
syntax or direct cross-service foreign keys were found. Historical SQL is unchanged.
Potential domain issues require separate decisions:

- Comments can reference a parent comment from a different post; the self-parent
  check does not prevent longer cycles.
- Chat does not enforce sender membership or uniqueness of a direct conversation
  between the same two users. These may belong in application logic.
- Notification preference/delivery Go types omit timestamps present in SQL.
- `updated_at` defaults do not update themselves; persistence code must maintain them.
- Auth's legacy import intentionally accesses public.profiles once. Its data rollback
  is intentionally blocked. Profile email still has a legacy uniqueness constraint.
- Shared public tables and shared database credentials provide logical ownership,
  not database privilege isolation. pgcrypto is shared and is not dropped by down.
- Post, chat, notification still initialize placeholder nil PostgreSQL clients;
  connecting them is business implementation outside this migration change.

Kubernetes deployment is now owned by `deployments/helm/social-media-backend/`.
The former raw manifests and empty scaffolding have been replaced by Helm.
See [deployment instructions](deployment.md) for packaged SQL and Helm/ArgoCD migration Jobs.
The accidental root `}` file was inspected (zero bytes) and removed.
Auth types already live alongside their service logic; remaining model files group
small related types. No model reorganization was needed. The existing untracked Admin.go lacked a
package declaration and time import; these were added and it was renamed admin.go
without changing its fields, so auth builds.

## Verification and outstanding working-tree issue

Validation used an isolated PostgreSQL 17 container with temporary storage; no real
application database or persistent volume was migrated or rolled back.

- gofmt and git diff --check passed for the changes.
- Shared, auth, post, chat, notification passed independent vet/test/build.
- Migration tests passed: configuration/credential handling, sequential pairs,
  creation validation, global-down guard, fresh SQL, repeat up, status, rollback and
  reapply, transaction failure and dirty state, legacy checksum validation/import,
  preservation of existing data, and refusal to overwrite imported history.
- Auth's PostgreSQL ownership integration test passed on the disposable database.
- Compose configuration and the migration Docker image build passed. The image
  applied a new test migration and host Make rolled it back using the same history.
- The host has a broken Docker credential-helper setting. Image build succeeded
  with a temporary empty DOCKER_CONFIG; the user's Docker configuration was untouched.

**Outstanding:** user-service was already entirely deleted from the working tree
before this task. That deletion is preserved pending the user's restoration choice.
Consequently root `make check`, global migration commands, user commands, and full
Compose deployment cannot succeed in the current checkout. A temporary copy with
user-service restored from HEAD passed all five modules through `make check`, and
all five original service SQL histories passed PostgreSQL tests. This temporary
verification does not mean user-service has been restored in the actual project.

Run the regression tests after resolving that deletion:

```sh
GOWORK=off go test -C shared ./cmd/migrate
# An ISOLATED PostgreSQL admin URL with CREATEDB rights; tests create/drop only
# uniquely named migration_test_* databases. No tests run against your app DB.
MIGRATION_TEST_DATABASE_URL='postgres://USER:PASSWORD@localhost:PORT/postgres?sslmode=disable' \
  GOWORK=off go test -C shared ./cmd/migrate -v
```

## Security migrations

Auth 000003 adds session families/generation state and audit records, invalidates
unused historical session records, and seeds baseline roles/permissions. It does
not overwrite password hashes or promote admins. Its down migration intentionally
refuses to erase security state. Auth 000004 and each other active service's 000002
grant table privileges to pre-provisioned `app_*` identities when those roles exist.
Provision roles before applying those grants; for an already-applied installation,
review and explicitly reapply the grants if provisioning happens later. Passwords
are never embedded in migrations. A down migration removing grants stops runtime
access; it is not a general application rollback strategy.

The old user-service runtime is not restored. The runner resolves `user` history
from the archive so the original migration names/content and legacy checksums stay
intact. The migration integration test exercises the historical prefix; the security
harness additionally applies and tests all current migrations on an isolated DB.
