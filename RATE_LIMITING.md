# Rate limiting

Current implementation and defaults are documented in
[Authentication security](docs/AUTH_SECURITY.md#rate-limits-audit-and-deployment).

The shared Redis limiter uses atomic fixed windows, expiration, namespaced hashed
identity keys, bounded Redis timeouts and per-service budgets. Trusted proxies
default to none. Authentication policies fail closed; the general traffic policy
fails open. Rejected attempts do not extend counter expiry. Health endpoints bypass
rate limiting. All replicas must use identical policy settings and the same Redis
DB. `RATE_LIMIT_<POLICY>_REQUESTS` and `RATE_LIMIT_<POLICY>_WINDOW` configure
GLOBAL, REGISTER, LOGIN, LOGIN_ACCOUNT, REFRESH, REFRESH_ACCOUNT and WEBSOCKET.
WEBSOCKET remains a reusable policy; no WebSocket endpoint is implemented.

Responses use the common error envelope. 429 includes Retry-After; an unavailable
auth limiter returns 503. IP policies expose RateLimit-Limit/Remaining/Reset.
Account/credential sublimits can further restrict an accepted IP request. Never
pass client-controlled identity headers as an authenticated principal.

Run `go test ./shared/ratelimit/...` and the disposable security harness for atomic
counter, expiry, trusted-proxy, outage, deadline and live login/refresh checks.
Redis processes in tests bind isolated Unix sockets; the end-to-end harness uses a
password-protected loopback Redis on a temporary port. No project Redis data is used.
