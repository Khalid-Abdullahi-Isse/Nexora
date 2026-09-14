# Redis integration

The existing five services share one Redis 8 container (`redis:6379`, DB 0).
Auth owns accounts; the existing User Service owns profiles/follows. No service
was created or removed. All five consume Redis through the existing distributed
rate limiter. Post, Chat, Notification and User cache boundaries now receive the
same initialized client. Auth has no active cache operations; its unused cache
placeholder is not instantiated. WebSocket, cache, session and queue features
remain at their existing implementation stage.

## Configuration and behavior

- `REDIS_ADDR=redis:6379` inside Docker; `localhost:6379` outside Docker.
- Existing `redis://` / `rediss://` URL support selects credentials and database,
  for example `redis://default:<password>@redis:6379/0`. Supply real credentials
  through deployment secrets; never commit them. The local Redis has no password.
- No competing `REDIS_HOST`, `REDIS_PASSWORD`, or `REDIS_DB` convention was added.
- `shared/redisconn` contains the former rate-limiter client factory. The old
  `ratelimit.Client` API delegates to it for compatibility. Existing go-redis/v9
  dependencies are retained; no module dependency changes or tidy were needed.
- Every server calls `Connect` once, with a five-second startup context and real
  PING. Pool/socket waits are bounded to three seconds; the existing limiter
  retains its configured request deadline (default 200ms). Invalid configuration
  and failed PING stop startup. No credential URLs are logged.
- Redis remains required at startup even if rate limiting is disabled. Runtime
  general-request fail-open and registration fail-closed policies are preserved.
- `/health` retains `status` and `service`, adds a real one-second Redis check,
  and returns 503 with sanitized status when a dependency fails. Auth/User also
  check their initialized SQL pools. Post/Chat/Notification do not claim a working
  PostgreSQL connection: their pre-existing database layers are placeholders.
- SIGINT/SIGTERM drains HTTP requests for up to five seconds, then closes pools.
- All five Compose services wait for healthy Redis. Its existing persistent
  volume and loopback-only published port are preserved; restart is unless-stopped.
- Existing atomic limiter keys remain `ratelimit:<service>:<policy>:<identity-hash>`
  with policy-specific TTLs. No cache contents or permanent data were introduced.

The connection configuration and SET/GET check follow the official
[go-redis connection documentation](https://redis.io/docs/latest/develop/clients/go/connect/).

## Repeat verification

```sh
make check
docker compose -f docker/docker-compose.yml up -d --build
./scripts/verify-redis.sh
```

Each image supports `/app/server --check-redis`: loads that service's environment,
connects, PINGs, writes a unique `<service>:health:<random>` key with a 30-second
TTL, reads it back, verifies TTL, deletes it and closes the pool. It does not
open SQL or serve HTTP. These checks run explicitly, never on each request.
The script also verifies each normal health endpoint, exercises its real global
limiter via an unknown route (no business writes), inspects keys with SCAN/TTL,
and checks that a separate process exits unsuccessfully for unreachable Redis.
The script assumes the default Compose ports, enabled limiter and DB 0.

Unit tests cover environment/URL configuration, invalid address/database values,
credential redaction, failed startup, live Redis SET/GET/TTL/cleanup, healthy and
failed Redis/SQL health responses. Existing limiter tests exercise atomic counts,
concurrency, expiration, policy behavior and route wiring. Tests require the
existing `redis-server` host executable. The optional PostgreSQL ownership test
still requires `AUTH_TEST_DATABASE_URL` and otherwise skips.

## Files changed for this task

- `.env.example`
- `docker/docker-compose.yml`
- `RATE_LIMITING.md`
- `REDIS_INTEGRATION.md`
- `scripts/verify-redis.sh`
- `shared/ratelimit/middleware.go`
- `shared/redisconn/client.go`
- `shared/redisconn/lifecycle.go`
- `shared/redisconn/client_test.go`
- `shared/server/server.go`
- `services/auth-service/cmd/server/main.go`
- `services/auth-service/internal/controller/http/controller.go`
- `services/user-service/cmd/server/main.go`
- `services/user-service/dependencies.go`
- `services/user-service/internal/controller/http/controller.go`
- `services/post-service/cmd/server/main.go`
- `services/post-service/internal/controller/http/controller.go`
- `services/chat-service/cmd/server/main.go`
- `services/chat-service/internal/controller/http/controller.go`
- `services/notification-service/cmd/server/main.go`
- `services/notification-service/internal/controller/http/controller.go`

Other pre-existing/concurrent rate-limit test changes were preserved.

## Executed verification — 2026-09-10

Final rebuilt Compose images were started successfully and tested using the
script above. All five startup logs confirm successful Redis connection via
`redis:6379`. PostgreSQL, Redis and all five services report Docker `healthy`.
Direct `redis-cli PING` returned `PONG`.

| Service | Redis required/use | Connection | PING | SET/GET/TTL | HTTP health |
| --- | --- | --- | --- | --- | --- |
| auth-service | Yes: registration/general rate limits | PASS | PASS | PASS | PASS |
| user-service | Yes: general rate limits | PASS | PASS | PASS | PASS |
| post-service | Yes: general rate limits | PASS | PASS | PASS | PASS |
| chat-service | Yes: general rate limits | PASS | PASS | PASS | PASS |
| notification-service | Yes: general rate limits | PASS | PASS | PASS | PASS |

All probes observed a 30-second TTL and cleaned up successfully (subsequent SCAN
found no probe keys). Real HTTP requests generated independently namespaced
limiter keys; SCAN and TTL verified bounded lifetime. Separate processes in each
service container, configured with `REDIS_ADDR=127.0.0.1:1`, exited nonzero with
`Redis startup PING failed` / `connection refused`, as expected. Normal server
logs contain successful startup connections and all health responses remain OK.
Auth/User health responses additionally report PostgreSQL connected.

`make check` passed: go vet, go test and go build across all six Go modules.
`go test -race ./shared/redisconn/... ./shared/ratelimit/...` passed. Formatting,
`git diff --check`, shell syntax and Compose configuration validation passed.
The optional PostgreSQL ownership integration test was not enabled for this task.

Problems fixed: missing startup PING, nil Redis cache-boundary clients, missing
Redis dependencies in three Compose services, health endpoints that only checked
HTTP availability, absent signal-aware shutdown, and acceptance of negative Redis
database numbers. Existing client construction was consolidated, not duplicated.

The host Docker configuration references an unavailable
`docker-credential-secretservice`. Public image builds succeeded using an empty
temporary `DOCKER_CONFIG=/tmp/redis-docker-config`; the user's Docker settings
were not changed. Future builds need the same override or repair of that helper.
There are no unresolved Redis integration failures. The pre-existing placeholder
business/database/WebSocket features remain outside this task.
