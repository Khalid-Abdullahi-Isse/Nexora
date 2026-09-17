# API Gateway

Kong OSS 3.9.1 is the single proxy for Nexora. It runs DB-less, with no new database,
Kong Consumers, or ingress controller. Auth still owns accounts, RS256 JWTs, refresh
sessions and roles; Go services still validate tokens, permissions and ownership.

```text
Postman / frontend → localhost:8000 → Kong
                                     ├─ auth-service:8001 → PostgreSQL / Redis
                                     ├─ post-service:8003 → PostgreSQL / Redis
                                     ├─ chat-service:8004 → PostgreSQL / Redis
                                     └─ notification-service:8005 → PostgreSQL / Redis
```

## Existing architecture and route map

The existing chart is `deployments/helm/social-media-backend`. Its release is
`social-media` in `social-media` on `kind-social-media`. The separate
`social-media` namespace is not changed. All service names below resolve in Kong's
own namespace; no ClusterIP addresses are stored in configuration.

| Public request | Internal destination | Path forwarded |
| --- | --- | --- |
| `/api/v1/auth/*` | auth-service:8001 | unchanged |
| `/api/v1/posts` and `/api/v1/posts/*` | post-service:8003 | unchanged |
| `/api/v1/users/:userId/posts` | post-service:8003 | unchanged |
| `/api/v1/chats/*` | chat-service:8004 | unchanged |
| `/api/v1/notifications/*` | notification-service:8005 | unchanged |
| `/api/v1/auth/health` or `/health` | auth-service:8001 | `/health` |
| `/api/v1/posts/health` | post-service:8003 | `/health` |
| `/api/v1/chats/health` | chat-service:8004 | `/health` |
| `/api/v1/notifications/health` | notification-service:8005 | `/health` |
| `/api/v1/posts/ready` | post-service:8003 | `/ready` |
| `/api/v1/chats/ready` | chat-service:8004 | `/ready` |
| `/api/v1/notifications/ready` | notification-service:8005 | `/ready` |

API routes have `strip_path: false` and no upstream path. Exact health aliases use
`strip_path: true` and upstream `/health`. Regex boundaries prevent `/postsXYZ`
matching `/posts`. Sensitive POST auth routes take precedence over the general route.

Chat exposes authenticated conversations, message history/send/read/delete and
`/api/v1/chats/ws`. Notifications expose listing, unread count, marking read,
deletion and `/api/v1/notifications/ws`. Kong forwards WebSocket upgrades along
with Authorization and subprotocol headers. See [Chat](CHAT_SERVICE.md) and
[Notifications](NOTIFICATION_SERVICE.md) for contracts. Posts include authenticated
likes and comments. Identity lives at `/api/v1/auth/me`; there is no user service.
Health and readiness aliases take precedence over API resource paths. Auth has
no separate readiness endpoint.

## Kubernetes: deploy and expose

From the repository root, the existing development script builds the current Go
sources, imports images, reuses credentials and persistent data, runs migrations
and upgrades the existing Helm release:

```bash
./scripts/start-dev.sh
```

For configuration-only changes after images and credentials are available:

```bash
helm upgrade --install nexora deployments/helm/social-media-backend \
  --kube-context kind-social-media --namespace social-media \
  -f deployments/helm/social-media-backend/values-dev.yaml --timeout 10m
```

Startup creates one managed background forward on `http://localhost:8000`.
Stop it with `./scripts/stop-dev.sh`. The compatibility command
`./scripts/kong-port-forward.sh` can restart forwarding without rebuilding.
Do not run Docker Kong on port 8000 at the same time. Re-run startup or the
forward helper after a Kong pod rollout ends its forward.

```bash
kubectl --context kind-social-media get nodes
kubectl --context kind-social-media get pods -A
kubectl --context kind-social-media -n social-media get pods,svc
kubectl --context kind-social-media -n social-media logs deployment/kong-gateway
kubectl --context kind-social-media -n social-media rollout status deployment/kong-gateway
curl http://localhost:8000/api/v1/auth/health
```

Kong startup/readiness uses `/status/ready`; liveness uses `/status` on internal
8100. These checks do not depend on application databases or upstream health.
The proxy Service exposes only 8000 and 8443 internally. Admin is disabled
in both base and development values; no Admin Service exists. Only proxy port
8000 is forwarded to the host.

## Docker Compose

Restore your existing `.env` and `.secrets/private.pem` / `.secrets/public-keys.json`
first. For a **fresh** local setup, follow the existing security setup instructions
in the main README. Never regenerate database credentials for existing volumes.

```bash
python3 scripts/render-kong-compose.py --env-file .env
docker compose --env-file .env -f docker/docker-compose.yml config --quiet
docker compose --env-file .env -f docker/docker-compose.yml up -d --build
docker compose --env-file .env -f docker/docker-compose.yml ps
curl http://localhost:8000/api/v1/auth/health
```

Kong listens on loopback host ports 8000 (HTTP), 8443 (development HTTPS), and
8001 (development Admin). Auth's old host port 8001 now belongs to Kong Admin.
Microservices no longer publish host ports. Existing PostgreSQL/Redis debug ports
remain loopback-only. All four applications retain `social-network` for database
access and join `gateway-network` with Kong. Docker DNS resolves their service names.
The dedicated gateway subnet is `172.30.80.0/24`, with Kong at `.2`; if that subnet
conflicts with local routing, change its IPAM, Kong address and `TRUSTED_PROXIES`
together. The existing database network is not recreated.

`docker/kong/kong.yml` is generated from the same Helm configuration. Regenerate
it when routes or origins change; `--check` detects drift. `.env` origins must
match the generated file. Default origins are localhost ports 3000 and 3001.

## Postman: exact requests

Import `postman/collections/kong-gateway.postman_collection.json`; it uses one
`base_url` variable and saves access/CSRF tokens after login and refresh. The
Postman cookie jar stores the refresh cookie automatically.

1. `GET http://localhost:8000/api/v1/auth/health`
2. `POST http://localhost:8000/api/v1/auth/register`
3. `POST http://localhost:8000/api/v1/auth/login`

For register/login use these headers and JSON (there is **no name field**):

```http
Content-Type: application/json
Origin: http://localhost:3000
X-CSRF-Protection: 1
```

```json
{"email":"test@example.com","password":"StrongPassword123!"}
```

Use a new email if it already exists. Login returns `data.access_token` and
`data.csrf_token`, and sets an HttpOnly `dev-refresh` cookie. The refresh credential
is intentionally absent from JSON. Store tokens in Postman with:

```javascript
const data = pm.response.json().data;
pm.collectionVariables.set('access_token', data.access_token);
pm.collectionVariables.set('csrf_token', data.csrf_token);
```

Then:

```http
GET http://localhost:8000/api/v1/auth/me
Authorization: Bearer <data.access_token>
```

```http
GET http://localhost:8000/api/v1/posts
```

```http
POST http://localhost:8000/api/v1/posts
Authorization: Bearer <data.access_token>
Content-Type: application/json

{"content":"Hello through Kong"}
```

Refresh/logout: POST `/api/v1/auth/refresh` or `/api/v1/auth/logout`, using the
cookie jar plus `Origin`, `X-CSRF-Protection: 1`, and `X-CSRF-Token: <data.csrf_token>`.
After refresh replace both tokens with the new response values. Logout returns 204.
The collections cover all 41 mounted routes, including health/readiness, session and admin APIs, post interactions, chat, notifications and WebSocket references. See [Postman setup](../postman/README.md) for variables and folder prerequisites.

## Security and operational limits

- Kong forwards Authorization unchanged. No duplicate user database or JWT system.
- CORS uses the same `config.ALLOWED_ORIGINS` as the Go services. Go's origin/CSRF
  validation remains active. Kong handles browser preflights. Production values
  use an explicit HTTPS frontend domain placeholder that must be replaced.
- Gateway baseline: 100 requests/minute/IP. Sensitive POST login/register/refresh:
  10/minute/IP, shared across these three routes. A more specific Kong rate-limit
  plugin **replaces** the global plugin on these requests; limits are not additive.
  `local` counters are per Kong instance and reset on restart. Keep one replica for
  predictable local limits; plan shared Redis counters before horizontal scaling.
- Existing Redis-backed Go limits remain, including registration 3/10 minutes and
  account/session limits. Gateway limits are abuse protection; Go limits are
  application protection. Kong hides its quota headers to avoid conflicting with
  Go's rate-limit headers. In this Kong version that also hides Retry-After on
  gateway-generated 429 responses; wait until the next minute window. Go-generated
  429 responses retain their existing Retry-After behavior.
- Docker Go middleware trusts only Kong's fixed gateway IP. Development Kubernetes
  trusts the inspected node pod CIDR `10.244.0.0/24`. This assumes a trusted local
  cluster. Production must set `config.TRUSTED_PROXIES` to actual proxy source
  ranges, constrain backend access with an enforcing CNI/network policy, and set
  `kong.trustedIPs` only if a trusted external proxy sits before Kong. Never trust
  `0.0.0.0/0`; leaving Go trust empty conservatively groups clients by Kong pod IP.
  Port-forward also groups requests under its local source address.
- Requests are bounded to 16 KiB at both Kong and Go; Kong's header buffers are
  bounded to `4 8k`. Retrying upstream requests is disabled to avoid duplicate writes.
- Correlation plugin supplies `X-Request-ID`. Go preserves canonical UUIDs and
  replaces invalid IDs. IDs are diagnostics, never identity/authorization input.
- JSON access logs contain method, path **without query string**, status, upstream
  address, duration and request ID. Headers, cookies, bodies and tokens are not
  logged by the access format. Error logs use warn; standard Nginx error messages
  may include request URLs, so never put credentials in URL paths or query strings.
- HTTPS 8443 is enabled. Development uses Kong's generated self-signed certificate.
  Set `kong.tls.existingSecret` to a `kubernetes.io/tls` Secret for real certificates,
  or terminate TLS at the existing external Ingress. Production must enforce HTTPS
  at that edge and configure the real DNS, certificate and trusted proxy ranges.
  Direct TLS Secret rotations require a Kong rollout. Do not publicly expose Admin.

## Helm, Ingress and ArgoCD

All resources belong to the existing Helm chart; there are no manual gateway
manifests or Kong ingress controller. Configuration checksum changes roll the pod.
`kong.enabled=false` restores the legacy Ingress routing behavior.

No Ingress/controller was deployed in the inspected cluster. The chart already
has an optional **nginx** Ingress. When enabled, its sole `/` backend is now Kong,
so it provides edge TLS while Kong owns application routing. No live Ingress was
deleted. Before using those staging/production values, supply an installed ingress
class, real hostname, TLS Secret and HTTPS redirect settings for that controller.

Existing ArgoCD Application files already reference this chart and environment
values. Commit/push these changes through your normal workflow to make them
available to ArgoCD. Do not install a second release into `social-media` or let
Helm and ArgoCD independently reconcile that namespace. The local verification
uses the existing Helm release; it does not change live ArgoCD ownership or push Git.

Validation:

```bash
python3 scripts/validate-deployment.py
python3 scripts/render-kong-compose.py --check
helm lint deployments/helm/social-media-backend
helm template social-media deployments/helm/social-media-backend \
  -n social-media -f deployments/helm/social-media-backend/values-dev.yaml
python3 scripts/verify-kong.py
```

The smoke script creates a uniquely named test account and a test post, deletes its
post and logs out; the test account remains because no account-delete API exists.
Run it only against a development environment.

References: [Kong DB-less mode](https://developer.konghq.com/gateway/db-less-mode/),
[rate-limit policy behavior](https://developer.konghq.com/plugins/rate-limiting/),
[health probes](https://developer.konghq.com/gateway/traffic-control/health-check-probes/).

## Verification on this workspace (2026-09-16)

- Upgraded existing Helm release `social-media`, revision 5, in `social-media`.
  Built current Go sources as `social-{auth,post,chat,notification,migrate}:kong-dev`;
  all application pods and Kong became Ready. The migration Job completed.
- Parsed the generated declaration with the actual `kong:3.9.1` image.
- Helm lint/render and semantic checks passed for default, dev, staging and
  production, in both Helm and ArgoCD modes. Compose configuration validated with
  temporary placeholder environment values (no credentials printed or changed).
- Started the actual Compose Kong service in an isolated one-off container, checked
  it healthy and removed that test container. A full Compose backend startup was
  not attempted: local `.env` and Compose JWT keys are missing. Existing Redis and
  all database volumes were preserved.
- `go test` passed across `shared` and all four service modules. Database integration
  tests that require external test configuration retain their existing skip behavior;
  the live gateway smoke test exercised the deployed database and Redis.
- `scripts/verify-kong.py` passed through the single Kubernetes port-forward:
  four health routes, register, login, JWT identity, public listing, authenticated
  post create/read/update/delete, user posts, refresh and logout.
- Missing JWT returned 401; untrusted Origin returned 403. Prefix collision returned
  404. Oversized request body returned 413; oversized header returned 400.
- Confirmed trusted-origin preflights (localhost 3000 and 3001), rejected CORS grants
  for an untrusted domain, both gateway rate limits returning 429, and inability
  to bypass IP limits by changing client-supplied X-Forwarded-For.
- Confirmed the same X-Request-ID in the response, Kong access log and Go service
  log. Direct request from inside Kong to the auth Service's namespace-qualified
  DNS name returned 200. HTTPS proxy health returned 200 with development certificate
  verification disabled (`curl -k`); this is not production certificate validation.
- The smoke test removed its post and logged out. Test account
  `kong-smoke-83eb4b2ae3cd@example.com` remains; no account-delete API exists.

Remaining deployment work outside local kind: restore Compose credentials if
using Compose; commit/push the chart through the normal GitOps workflow; supply real
production origins, image references, certificates and ingress/proxy trust settings.
Deploy current backend images and the updated gateway declaration to use the new chat, notification and readiness routes. The historical verification above predates these additions.
