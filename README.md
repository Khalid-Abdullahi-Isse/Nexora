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


## API Gateway

Kong is the single entry point for the backend. Auth and each microservice retain
JWT validation and business authorization. Kong adds routing, CORS, request limits,
correlation IDs, access logs and TLS support without another database.

```text
Postman → localhost:8000 → Kong → auth-service:8001
                              → post-service:8003
                              → chat-service:8004
                              → notification-service:8005
                                   ↓
                              PostgreSQL / Redis
```

Deploy with `./scripts/kind-deploy.sh`, then expose the whole backend with:

```bash
kubectl --context kind-social-media -n social-media-dev \
  port-forward svc/kong-gateway 8000:8000
```

Test `GET http://localhost:8000/api/v1/auth/health`.
See [API Gateway](docs/API_GATEWAY.md) for Docker startup, Helm/ArgoCD configuration,
route mappings, security, health checks and exact Postman requests. Import the
[gateway Postman collection](postman/collections/kong-gateway.postman_collection.json).
Chat and notification business APIs remain unimplemented; their health routes work.
Older direct-service examples in this README are for standalone debugging; use the
gateway collection and port 8000 for normal Compose/Kubernetes access.

Chat service REST/WebSocket API, configuration, security and deployment:
[Chat service guide](docs/CHAT_SERVICE.md).
