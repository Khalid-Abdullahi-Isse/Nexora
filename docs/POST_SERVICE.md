# Post Service REST API

## Architecture and identity

The service uses the existing Gin → service → PostgreSQL repository layout, GORM,
shared `dbconn.Open` pool (20 open/5 idle connections), shared environment loader,
shared RS256 JWT verifier, Redis rate limiter, HTTP request limits/timeouts, and
signal-aware server shutdown. PostgreSQL remains the **one existing `social_media`
database**, with runtime role `app_post`. No new database or PostgreSQL container.

Both users and posts use UUIDs. The existing database owner column remains
`posts.author_user_id`; JSON exposes it as `user_id`. Auth owns `users`, roles and
sessions. Following the repository migration policy, no cross-service foreign key
is added; the post service never writes Auth tables. Reads are now public, as
requested; older private repository helpers remain available to their existing callers.

JWT signature, expiry, issuer, audience `post-service`, key ID and claim structure
are checked by `shared/authn`. The business layer obtains the principal only from
the trusted request context and requires `posts.create`, `posts.update-own` or
`posts.delete-own`. Ownership mismatches return 403, including for administrators.
Mutation SQL also matches both post ID and owner ID, with parameterized arguments.
Missing posts return 404. Unknown JSON fields (including `user_id`) are rejected.

## API

| Method | Route | Authentication | Success |
|---|---|---|---|
| POST | `/api/v1/posts` | JWT + create permission | 201 |
| GET | `/api/v1/posts` | Public | 200 |
| GET | `/api/v1/posts/:id` | Public | 200 |
| GET | `/api/v1/users/:userId/posts` | Public | 200 |
| PATCH | `/api/v1/posts/:id` | JWT + update permission + owner | 200 |
| DELETE | `/api/v1/posts/:id` | JWT + delete permission + owner | 204 |
| GET | `/health` | Public | 200 (liveness) |
| GET | `/ready` | Public | 200 or 503 (PostgreSQL + Redis) |

Create body: `{"content":"My first post","image_url":"https://example.com/a.jpg"}`.
Image is optional. Content is trimmed, nonblank, at most 10,000 Unicode code points;
NUL is rejected. The existing shared HTTP middleware also caps JSON bodies at 16 KiB.
Image URLs must be HTTP(S), include a hostname, omit credentials, and be at most
2,048 bytes. URLs are stored only; this service does not fetch images.

PATCH supports omitted fields (preserved) and `image_url: null` (clear image).
`content: null`, empty objects and blank content are invalid. IDs must be nonzero
canonical UUIDs. Feed defaults to `page=1&limit=20`, maximum limit 100. Invalid or
overflowing pagination is rejected. Database LIMIT/OFFSET and deterministic
`created_at DESC, id DESC` ordering apply to both feed routes. A separate COUNT
provides totals; concurrent writes can change totals between count and fetch.

Errors use `{"success":false,"error":{"code":"...","message":"..."}}`.
Internal failures are sanitized; logs include request ID, error type and PostgreSQL
SQLSTATE/table/constraint where available, without SQL values or database secrets.

## Migration and configuration

`000003_post_rest_api.up.sql` adds nullable `image_url` and two indexes supporting
stable global/per-author feeds. It preserves historical migration files and data.
Repeated up SQL is safe; the existing versioned migration runner applies it once.
The down migration removes the new indexes and column and loses image URLs; it is
not part of startup or normal deployment. No migration was committed during testing.

Required Post Service environment values: `POSTGRES_HOST`, `POSTGRES_PORT`,
`POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB`, `POSTGRES_SSLMODE`.
`POST_SERVICE_PORT` defaults to 8003. Existing JWT, Redis, CORS and rate-limit
configuration is still required. Compose and Helm already provide these values.
Inside Docker/Kubernetes the database host is `postgres`, not localhost.

Apply pending migrations with the existing DDL-capable migration identity before
rolling out the new service. Runtime `app_post` intentionally cannot run DDL.
The migration image must also be rebuilt, since it packages the SQL for Kubernetes.

## Verification commands

Run from the repository root:

```bash
make check
# Explicit module patterns are required: there is no root go.mod.
go test ./shared/... ./services/auth-service/... ./services/post-service/... ./services/chat-service/... ./services/notification-service/...
go build ./shared/... ./services/auth-service/... ./services/post-service/... ./services/chat-service/... ./services/notification-service/...
go test -race ./services/post-service/...
python scripts/validate-deployment.py
node postman/validate-post.mjs
```

Database integration test (uses an existing database; all schema/test data is
rolled back). Set `POST_TEST_DATABASE_URL` through your secret environment to a
DDL-capable identity against an existing migrated database, then:

```bash
go test ./services/post-service/internal/database/postgres -run TestPostRepository -count=1 -v
```

Do not point the older `MIGRATION_TEST_DATABASE_URL` suite at this workflow: that
suite creates disposable databases. Without a test URL, PostgreSQL integration
tests explicitly skip; ordinary Go tests do not prove database connectivity.

Compose requires a configured root `.env` and existing `.secrets` key files (the
checkout currently has no root `.env`). Follow the existing security setup docs to
configure those; do not overwrite existing credentials or reset volumes.

```bash
docker compose --env-file .env -f docker/docker-compose.yml config --quiet
docker compose --env-file .env -f docker/docker-compose.yml build post-service migrate
docker compose --env-file .env -f docker/docker-compose.yml run --rm migrate
docker compose --env-file .env -f docker/docker-compose.yml up -d post-service
docker compose --env-file .env -f docker/docker-compose.yml logs -f post-service
```

For Kubernetes deployment use the existing Helm/Argo CD process in
[deployment.md](deployment.md), building both Post and migration images. This
implementation has not upgraded the running cluster; its current Post image still
has the previous API. The chart now routes `/api/v1/users` to Post (Auth account
routes remain under `/api/v1/auth`), uses `/ready` for Post readiness and `/health`
for liveness, and retains ClusterIP, resource limits and secret references.

```bash
kubectl --context kind-social-media get pods -n social-media-dev
kubectl --context kind-social-media get svc -n social-media-dev
kubectl --context kind-social-media logs -n social-media-dev deployment/post-service
kubectl --context kind-social-media -n social-media-dev port-forward svc/post-service 8003:8003
```

## Postman

After deploying the new image and migration, use **http://localhost:8003** with the
port-forward above (or with Compose). Import
`postman/collections/post-service.postman_collection.json`. Set `access_token` from
Auth login. Run sequentially: create saves `post_id` and `user_id`; the collection
then reads, updates, validates owner-injection rejection, and deletes that post.
For a manual ownership test, create as User A, then use User B's access token on
PATCH and DELETE of the same UUID: both must return 403.

Quick URLs: `http://localhost:8003/health`, `http://localhost:8003/ready`,
`http://localhost:8003/api/v1/posts?page=1&limit=20`.

## Validation results

- Full workspace vet, tests and builds (`make check`): passed.
- Post Service tests with race detector: passed.
- Existing PostgreSQL repository integration and repeated migration SQL inside a
  rollback-only transaction: passed; no committed schema or data changes.
- Docker image build, container startup with `app_post` against existing Kubernetes
  PostgreSQL/Redis, health/readiness, and SIGTERM shutdown with exit 0: passed.
- Compose configuration using temporary validation-only environment values: passed.
  Full Compose startup was not run because the root `.env` is absent.
- Helm lint/render/semantic checks: all four environments in Helm and Argo CD modes
  passed. Existing Auth `/health` reports both dependencies connected.
- Focused Postman collection validation: passed (8 routes, 13 requests).
- Legacy `node postman/validate.mjs`: fails on an unsupported Auth `protected` group.
  Confirmed the identical failure on unmodified HEAD; its old collection predates
  existing Auth APIs. The focused Post collection has its own validation command.
- Deployment of the new image and migration remains an operator rollout step.

## Files created

```text
docs/POST_SERVICE.md
postman/collections/post-service.postman_collection.json
postman/validate-post.mjs
services/post-service/internal/config/config_test.go
services/post-service/internal/controller/http/posts.go
services/post-service/internal/controller/http/posts_test.go
services/post-service/internal/database/postgres/posts_api.go
services/post-service/internal/database/postgres/posts_api_test.go
services/post-service/migrations/000003_post_rest_api.down.sql
services/post-service/migrations/000003_post_rest_api.up.sql
```

## Files modified

```text
deployments/helm/social-media-backend/templates/_helpers.tpl
deployments/helm/social-media-backend/values.yaml
docker/docker-compose.yml
scripts/validate-deployment.py
services/post-service/cmd/server/main.go
services/post-service/internal/config/config.go
services/post-service/internal/controller/http/controller.go
services/post-service/internal/controller/http/ratelimit_test.go
services/post-service/internal/controller/http/request.go
services/post-service/internal/controller/http/routes.go
services/post-service/internal/database/postgres/database.go
services/post-service/internal/database/postgres/post.go
services/post-service/internal/models/models.go
services/post-service/internal/service/service.go
shared/ratelimit/middleware.go
```
