# Social Media Backend

Go microservices backend for a social media application.

Open this directory in your Go editor. `go.work` includes all four service modules
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

All four servers use shared Redis rate limiting, with stricter registration
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

## Start everything locally

Prerequisites: Docker, kind, kubectl, Helm, Python 3 with PyYAML, OpenSSL and
`flock` (Linux). Run from this backend directory:

```bash
chmod +x scripts/start-dev.sh
./scripts/start-dev.sh
```

This reuses or creates the `social-media` kind cluster, builds and loads the five
application/migration images, reuses or creates Secrets, upgrades Helm release
`nexora` in namespace `social-media`, waits for all workloads, prints their status,
and starts one background Kong forward on **http://localhost:8000**.
Repeated starts reuse the owned forward. An occupied port belonging to another
process produces a clear error; the script never kills unrelated processes.

```bash
kubectl --context kind-social-media get pods -n social-media
kubectl --context kind-social-media get svc -n social-media
helm --kube-context kind-social-media list -n social-media
./scripts/stop-dev.sh
```

Stop only closes the forward created by these scripts. Kubernetes and databases
keep running. PostgreSQL/Redis and every microservice remain internal ClusterIP
Services; only Kong is forwarded. Admin API is disabled.

Expected workloads: auth, post, chat, notification (two replicas), Kong,
PostgreSQL and Redis. The migration Job should be `Completed`.

Import the existing Postman collections and select **Local** or **Development**:
`base_url=http://localhost:8000`, `ws_base_url=ws://localhost:8000`.

- `POST {{base_url}}/api/v1/auth/register`
- `POST {{base_url}}/api/v1/auth/login`
- `GET {{base_url}}/api/v1/posts`
- `GET {{base_url}}/api/v1/notifications` (Bearer access token)
- `ws://localhost:8000/api/v1/chats/ws` (Bearer access token)
- `ws://localhost:8000/api/v1/notifications/ws` (Bearer access token)

Login/register JSON: `{"email":"you@example.com","password":"your-strong-password"}`.
Use `Content-Type: application/json`, `Origin: http://localhost:3000`, and
`X-CSRF-Protection: 1` for mutations; refresh/logout also require the CSRF token.

```bash
python3 scripts/validate-deployment.py
python3 scripts/verify-kong.py
```

The smoke test creates one test account and removes its temporary post.
See [deployment details](docs/deployment.md) for credentials, retained older
environments, GitOps ownership and troubleshooting, and [API Gateway](docs/API_GATEWAY.md)
for routing/security details. Local development uses manual Helm; ArgoCD is an
alternative owner and must not manage the same namespace concurrently.

Chat service REST/WebSocket API, configuration, security and deployment:
[Chat service guide](docs/CHAT_SERVICE.md).

## Realtime notifications

See [Notification Service](docs/NOTIFICATION_SERVICE.md) for architecture, authenticated WebSockets, producer outboxes, local testing, Postman, migrations and ArgoCD deployment.
