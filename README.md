# Social Media Backend

Go microservices backend for a social media application.

Open this directory in your Go editor. `go.work` includes all five service modules
and the shared module so tools can resolve them together.

Run `make check` from this directory to vet, test, and build every module. The
PostgreSQL integration test requires `AUTH_TEST_DATABASE_URL`; without it, that
test is skipped. Individual service modules remain independently buildable.

Start Docker with `docker compose -f docker/docker-compose.yml up -d --build`
(or `make docker-up`). PostgreSQL must become healthy, then the `migrate` job
applies pending migrations before application startup. A successful migration
container shows `Exited (0)`; that is expected.

See [Docker database initialization](docker/migrations/README.md) for migration
tracking, existing databases, connection settings, and verification results.

Services:

- Auth Service
- User Service
- Post Service
- Chat Service
- Notification Service

Future technologies:

- Go
- Gin
- PostgreSQL
- Redis
- WebSockets
- Docker
- Kubernetes

## Rate Limiting

All five servers use shared Redis rate limiting, with stricter registration
limits and health-check exemptions. See [Rate Limiting](RATE_LIMITING.md) for
policies, environment variables, Docker/Kubernetes guidance and isolated tests.
