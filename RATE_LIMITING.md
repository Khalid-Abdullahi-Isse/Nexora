# Rate Limiting

All five server entrypoints install `shared/ratelimit` before registering routes.
Redis stores counts shared by every replica of a service. There is no in-memory
fallback. This protects application requests and registration attempts; it is
not a replacement for network DDoS protection.

## Architecture inspected

- auth-service: PostgreSQL-backed `POST /api/v1/auth/register`, `GET /health`.
- user-service: PostgreSQL initialization, `GET /health`; profiles/follows are not routed.
- post-service: `GET /health`; application/database boundaries are placeholders.
- chat-service: `GET /health`; WebSocket controller, hub, client and message placeholders.
- notification-service: `GET /health`; WebSocket placeholders.

No login, reset, OTP, upload, search, admin, authenticated API, JWT middleware,
WebSocket upgrade/message loop, authenticated internal HTTP calls, metrics
platform, Kubernetes resource, or ingress exists in this checkout. Dependencies
and data models alone do not implement these features. No routes were invented.

## Active policies

| Policy | Default | Identity | Redis failure |
|---|---|---|---|
| General requests, including unknown routes | 100/minute | IP per service | Allow, throttled log |
| POST /api/v1/auth/register | 3/10 minutes | IP in auth-service | Generic 503 |
| GET /health | Exempt | None | Independent of Redis |
| WebSocket middleware configuration (not yet routed) | 10/minute | IP when attached | Generic 503 |

Registration replaces the general policy, and counts every attempt, including
invalid requests. Existing success and validation responses are unchanged.

`ratelimit:<service>:<policy>:<SHA256(identity)>` uses separate policy/identity
namespaces (`ip:` and `user:`). Keys contain no raw IPs, account identifiers or
passwords. SHA256 is pseudonymization, not encryption. An atomic Lua script
checks and increments the counter and sets a millisecond TTL on creation. The
window starts with the first request. Rejections neither increment nor extend
it. Redis TTL provides the shared clock. Fixed windows can allow boundary
bursts; these are not sliding-window guarantees. Each policy requires one Lua
execution (an initial NOSCRIPT may require a second command).

## Configuration

The existing Envfolder loader loads `.env` first; the rate-limit loader then
validates these environment values before a server starts:

| Variable | Default |
|---|---|
| RATE_LIMIT_ENABLED | true |
| RATE_LIMIT_GLOBAL_REQUESTS / RATE_LIMIT_GLOBAL_WINDOW | 100 / 1m |
| RATE_LIMIT_REGISTER_REQUESTS / RATE_LIMIT_REGISTER_WINDOW | 3 / 10m |
| RATE_LIMIT_WEBSOCKET_REQUESTS / RATE_LIMIT_WEBSOCKET_WINDOW | 10 / 1m |
| RATE_LIMIT_REDIS_TIMEOUT | 200ms |
| TRUSTED_PROXIES | empty |
| REDIS_ADDR | Existing shared value; localhost:6379 fallback |

Requests must be 1–1,000,000,000; windows 1ms–24h; timeout 1ms–5s. Invalid
settings stop startup. Boolean parsing uses Go's `strconv.ParseBool`.
`REDIS_ADDR` also accepts `redis://` and `rediss://` URLs for ACL credentials,
database selection and TLS. Keep credential URLs in Secrets, not ConfigMaps.
Timeouts cover dialing, socket operations, pool waits and request context;
automatic command retries are disabled. Redis outages never produce raw errors
in HTTP responses or logs. Outage logs are limited to once per 30 seconds per
process using the existing standard logger. There is no new metrics platform.

Keep configuration identical across replicas. Restart services after changing
settings. Existing windows retain their original TTL until they expire; use a
coordinated rollout and allow the longest previous window to expire.

## Docker and Kubernetes

Compose uses the existing single `redis:6379` instance on `social-network`.
Redis's host port is now loopback-only (`127.0.0.1:6379`), so host development
still works without exposing Redis to external interfaces. Compose forwards all
limiter variables. Existing Dockerfiles already copy the shared module.

No Kubernetes manifests exist, so none were fabricated. For deployment, set
`REDIS_ADDR` to the shared internal Redis Service DNS (never pod localhost),
put non-secret settings above in a ConfigMap and credentials in a Secret, and
allow application pods to reach Redis using NetworkPolicy. Redis must not have
a public Service. Use a provisioned highly available Redis with capacity
monitoring, appropriate ACL/TLS, and a no-eviction policy for limiter state.
Eviction, restart, or failover loss can reset quotas. TTL bounds key lifetime,
not total key cardinality: distributed source traffic still requires capacity
planning and edge protection.

Gin trusts no proxies by default. Configure only actual ingress peer addresses
in `TRUSTED_PROXIES`, have ingress sanitize forwarded headers, and restrict
service access to that ingress. Never use all-address CIDRs or broad shared
networks containing untrusted clients. Incorrect proxy settings may aggregate
all clients under a proxy IP. There is no custom-header internal bypass.
Ingress rate-limit rules depend on the controller chosen later; its local
replica counters cannot replace Redis account/action policies.

## Future routes

Attach `limiter.Middleware(rateConfig.WebSocket, ratelimit.IP)` directly before
the WebSocket upgrade handler; upgrade headers must never exempt an HTTP route.
For authenticated routes, run authentication first, then supply an Identity
function returning `"user:" + verifiedID` from the trusted authentication
context. Never use a user-ID request header. `Limiter.Check` is reusable in a
future message loop without sleeps. Explicit route policies are needed for
login (5/min/IP plus 10/15min/account), resets, OTP, admins and writes when
those endpoints exist. Normalize account identity consistently with auth and
use a keyed hash if protection against offline guessing of account identifiers
is required. Do not include a password. No account-body parsing or hypothetical
login policy is active today. If stacking middleware, the last successful
policy supplies response headers; prefer a single selected route policy where
possible and document compound quotas explicitly.

## Responses

A depleted quota returns HTTP 429:

```json
{"status":429,"error":"Too Many Requests","message":"Rate limit exceeded. Please try again later."}
```

`RateLimit-Limit`, `RateLimit-Remaining`, and `RateLimit-Reset` describe the
selected policy. Reset is seconds until expiry, rounded up (not epoch time).
429 also supplies `Retry-After`. On registration Redis failure, HTTP 503 uses
`{"status":503,"error":"Service Unavailable","message":"Please try again later."}`
and `Retry-After: 1`. Fail-open responses omit unknown quota metadata.

## Validation

Install `redis-server` and `curl` on the test host, then run:

```sh
make check
go test -race ./shared/ratelimit/...
go test -v ./shared/ratelimit/... -run TestCurlSmoke
```

This workspace has six modules and no root go.mod; `make check` runs vet, test
and build across all six with explicit patterns. Tests start isolated Redis
processes on temporary Unix sockets with persistence disabled, never using
production Redis. They cover concurrent replicas, exact quota boundaries,
expiry and TTL, independent IPs/users, forwarded-header spoofing, health
exemption, stricter registration, WebSocket pre-upgrade middleware, outage
behavior, configuration validation and real curl calls. WebSocket/user tests
use test handlers because real handlers/auth do not exist yet. Curl performs
101 API calls (last 429) and four registration-fixture attempts (last 429),
without creating accounts. The existing PostgreSQL ownership integration test
requires `AUTH_TEST_DATABASE_URL` and otherwise skips.

Implementation references: [Redis atomic counter guidance](https://redis.io/docs/latest/commands/incr/)
and [Gin trusted proxy configuration](https://gin-gonic.com/en/docs/server-config/trusted-proxies/).
