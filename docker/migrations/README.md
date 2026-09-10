# Docker database initialization

Run from the backend directory:

```sh
docker compose -f docker/docker-compose.yml up -d --build
docker compose -f docker/docker-compose.yml logs migrate
docker compose -f docker/docker-compose.yml ps -a
```

Startup order is PostgreSQL healthy → migration job succeeds → applications start.
Compose uses `service_healthy` and `service_completed_successfully` as described in
[Docker's startup-order documentation](https://docs.docker.com/compose/how-tos/startup-order/).
Redis-dependent services also wait for Redis health. The migration container is a
one-time job and should finish with `Exited (0)`.

`run.sh` applies every service's `*.up.sql` files using PostgreSQL's own client.
The order is user, auth, post, chat, notification; filenames within each service
are ordered lexically. User's legacy schema precedes auth's account import.

`public.schema_migrations` records service, filename, SHA-256 checksum, and time.
Applied files are verified and skipped. Add a new migration rather than editing an
applied file. All pending migrations and history updates run in one transaction,
protected by a database advisory lock. SQL errors roll back the run and return a
nonzero exit code. New migrations must support transactional execution. Down
migrations never run automatically.

To apply new forward migrations explicitly:

```sh
docker compose -f docker/docker-compose.yml run --rm migrate
```

PostgreSQL data remains in `social-media-backend_postgres_data`; Redis uses its
existing volume. This workflow does not require deleting either volume.

## Connections

- Containers: PostgreSQL `postgres:5432`, database `social_media`.
- Host tools: PostgreSQL `localhost:15432` (host 5432 is occupied).
- Containers: Redis `redis:6379`; host tools: `localhost:6379`.
- Auth and user open real PostgreSQL connections. Post, chat, and notification
  currently retain placeholder database clients; their tables are initialized,
  but their health endpoints do not prove application database operations work.

## Existing databases

This local database had four manually applied auth/user migrations and no migration
history. Before baselining them, its eight table definitions were compared against
a fresh database created from the migration files; they matched exactly. Only then
were their filenames/checksums recorded. Existing rows were preserved, and the three
remaining migrations were applied normally.

Other pre-existing installations without migration history require the same explicit
schema and data-migration verification before recording a baseline. The runner does
not guess that a migration succeeded just because a table exists. Fresh databases
need no baseline; all seven current migrations run automatically.

## Verification performed

- Compose configuration validated successfully.
- Initialized an empty temporary PostgreSQL 17 instance: seven migrations, seventeen
  application tables plus the migration history table.
- Re-ran migrations: all applied files verified and skipped.
- Injected a failing migration: nonzero exit, created test table and history rolled back.
- Injected a checksum mismatch in the temporary database: migration run rejected.
- Ran normal Compose startup on the existing database: migration job exited 0.
- Compared data-dump checksums for all eight original tables: unchanged.
- Auth PostgreSQL integration test passed against the initialized Docker database.
- All five service health endpoints returned 200; PostgreSQL and Redis are healthy.
- Removed the temporary test container, which used memory-backed storage. Existing
  PostgreSQL/Redis containers and volumes were preserved.
