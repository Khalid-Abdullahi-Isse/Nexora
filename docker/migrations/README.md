# Docker migrations

The Compose migration job now uses the same pinned golang-migrate Go runner as the
root Makefile. See [Database migrations](../../docs/MIGRATIONS.md) for commands,
connection settings, rollback safety, and legacy history import.

Existing databases using the previous psql runner must explicitly import verified
history before the new deployment job can apply migrations. The old ledger remains
an audit record; do not run the old runner after switching.

```sh
docker compose -f docker/docker-compose.yml run --build --rm migrate all import-legacy
docker compose -f docker/docker-compose.yml run --build --rm migrate
```

Skip import-legacy for an empty database. No volumes need deletion.
