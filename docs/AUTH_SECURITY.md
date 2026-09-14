# Authentication and authorization

This implements the architecture approved after [the security review](../SECURITY_REVIEW.md).
Auth owns users, roles, permissions, password verification, signing and refresh
sessions. Every other service verifies access JWTs locally with public keys.

## Running locally

For a **new local environment**, run:

```sh
python scripts/setup-security-dev.py
docker compose --env-file .env -f docker/docker-compose.yml up -d --build
```

The setup script creates random local passwords and a 3072-bit RSA key pair in
ignored `.env`/`.secrets` files. It refuses to overwrite an existing environment or
key file and prints no secrets. Compose is explicitly for development: ports bind
to loopback and cookies permit local HTTP. Do not reuse this Compose configuration
as a production ingress.

Existing databases/volumes are preserved. PostgreSQL initialization scripts run
only on **empty** volumes. Generating new `.env` values does not rotate an existing
database password. For an existing installation, retain access with the existing
migration identity, provision the four runtime roles with your secret manager,
import verified legacy migration history when required, then apply forward
migrations. Review `docker/postgres-init.sh` for the role names and provisioning
logic. Never remove a volume to make migration or authentication work.

Migrations require a separate DDL-capable identity (`DATABASE_URL` for host CLI,
or the migration container's PostgreSQL credentials). The application `.env`
default user is `app_auth`, which intentionally cannot migrate the database.
See [migration instructions](MIGRATIONS.md). The deleted user runtime stays deleted;
its unchanged historical SQL lives in `docker/migrations/legacy-user` so existing
identity imports and checksums continue to work.

A fully disposable check that does not use project databases or Docker volumes:

```sh
python scripts/test-security.py
```

This starts temporary PostgreSQL and Redis processes, applies migrations, runs
race/integration tests and vet, builds and starts all four services, exercises HTTP
authentication, and checks log redaction. It requires Go, PostgreSQL binaries and
Redis. It stops its processes and removes its own data on exit. After building images,
`python scripts/test-security-docker.py` also verifies an isolated Docker startup,
non-root runtimes and key mount separation using only newly created test volumes.

## Endpoint contract

All paths below start with `/api/v1/auth`.

| Method/path | Authentication and behavior |
| --- | --- |
| POST `/register` | Email/password only; 201 safe account DTO; 409 duplicate retained for compatibility. Unknown JSON fields rejected. No privileged fields accepted. |
| POST `/login` | Generic credential failure; active accounts only; returns access/CSRF tokens and sets refresh cookie. Requires trusted Origin and `X-CSRF-Protection: 1`. |
| POST `/csrf` | Trusted Origin, custom protection header and refresh cookie; recovers the session-bound CSRF value after a page reload. No token issuance. |
| POST `/refresh` | Refresh cookie, trusted Origin, `X-CSRF-Protection: 1`, and session-bound `X-CSRF-Token`. Rotates refresh credentials. Access JWT may be expired. |
| POST `/logout` | Same cookie/CSRF requirements; revokes its whole family and clears cookie; no access JWT required. |
| POST `/logout-all` | Bearer JWT plus `sessions.manage-own`; revokes every family for the subject. |
| GET `/me` | Bearer JWT plus `accounts.read-own`; returns only the token subject's account. |
| GET `/sessions` | Bearer JWT plus `sessions.manage-own`; up to 100 active, unexpired families belonging to the subject. |
| DELETE `/sessions/:id` | Same permission; query includes both family ID and subject. Foreign/unknown family is 404. |
| POST `/change-password` | Bearer JWT plus `accounts.read-own`; requires `current_password` and `new_password`, then revokes all refresh families. |
| PUT `/admin/users/:id/roles/:roleID` | Admin role, `admin.users.manage`, original login within five minutes, and a repeated current DB permission/status check; audits role assignment. |
| DELETE `/admin/users/:id/roles/:roleID` | Same controls; audits role removal. |

Successful login/refresh JSON:

```json
{"success":true,"data":{"access_token":"<JWT>","expires_in":900,"csrf_token":"<session-bound CSRF value>"}}
```

The refresh credential is **only** in an HttpOnly cookie, never in JSON, URLs or
logs. Production cookie: `__Secure-refresh`, Secure, HttpOnly, host-only,
SameSite=Lax, Path=/api/v1/auth. Local insecure mode uses a different name,
`dev-refresh`. Logout clears the same path and attributes. Native clients can use
the same explicit cookie/header contract; no alternate JSON refresh-token mode is
silently enabled.

CSRF tokens are SHA-256 of a domain-separated prefix plus the random refresh
credential. This binds the header to the current session generation without
storing an additional secret. Origin validation and a required custom header
prevent login CSRF. Refresh/logout additionally compare the session-bound token in
constant time. CORS only grants configured exact origins; wildcard credentials are
never used. Even a same-site hostile subdomain must be explicitly allowed.

The browser should keep the access/CSRF values in memory, send Bearer access
credentials, and use `credentials: "include"` for auth. On one 401, perform a
single-flight refresh across concurrent requests/tabs, update both access and CSRF
values, and retry the original request **once**. If refresh fails, clear local
state and require login. On page reload, first POST `/api/v1/auth/csrf` with the refresh
cookie, trusted Origin and protection header to recover the CSRF value, then
refresh normally. This bootstrap does not rotate or authorize a session by itself.
No frontend exists in this repository, so that client
coordination is a documented contract, not a tested browser implementation.

Implemented auth and rate-limit errors have `success:false` and
`error:{code,message}`. Unmatched routes retain Gin's default 404 response. Use 400 invalid
input, 401 invalid authentication, 403 missing capability/CSRF, 404 inaccessible
private object, 409 registration conflict, 429 throttled, 503 auth limiter
unavailable, and 500 unexpected failure. Requests are limited to 16 KiB JSON, one
object, known DTO fields. HTTP header/body/write/idle timeouts are bounded and request work has a 10-second
context deadline.

## Access JWTs and key lifecycle

Access tokens use RS256, `typ=at+jwt`, a configured `kid`, and exactly 900 seconds
between `iat` and `exp`. Required claims: UUID `sub`, UUID `jti`, session-family
`sid`, original `auth_time`, nonempty `roles`, `permissions`, issuer and audience.
No email or credential material is included. The existing multiple-role model is
preserved. Four service audiences are explicitly issued; each verifier requires
its own service's audience. A user token is not an internal service identity.

The verifier pins the signing algorithm and known public key IDs, rejects key URLs,
wrong token types, malformed/duplicate Bearer headers, missing/invalid claims,
future issuance/`nbf`, and expired tokens. Expiry uses **zero leeway**. It places a
typed principal in Gin and request context; repositories take that principal, not
an owner supplied in a payload. Auth performs a signing/verification self-check at
startup so mismatched keys fail before serving traffic.

Only auth receives `JWT_PRIVATE_KEY_FILE`. All services receive
`JWT_PUBLIC_KEYS_FILE`, a JSON map of key ID to PEM public key. No HS256 flow existed
in the original project; the unused `JWT_SECRET` was removed without adding an
insecure compatibility fallback.

For rotation: distribute the new public key to all verifiers, restart/reload those
processes, switch auth's key and `JWT_KEY_ID`, wait at least 15 minutes after the
last token signed by the old key, then remove the old public key. Files are loaded
at startup; there is no automatic JWKS network request on API calls.

## Refresh sessions and revocation

Refresh tokens use 32 bytes from crypto/rand, base64url encoded. PostgreSQL stores
only their SHA-256 hash. Families default to an **absolute seven-day** lifetime;
rotation does not extend that deadline. Expired or revoked families cannot rotate.
Consumed generations retain their hashes so replay remains detectable.

All session mutations lock the user first. Refresh then locks the family and
rereads the token under lock before checking state. It consumes the old generation,
inserts a replacement, signs the JWT, writes the audit event, and commits before
returning credentials. A partial unique index permits one unconsumed/unrevoked
generation per family. Signing or database failure rolls back issuance.

Reusing a consumed token revokes the entire family and records
`REFRESH_TOKEN_REUSE_DETECTED`. That transaction commits before returning 401.
Two simultaneous refresh attempts can produce one successful response, followed
by revocation of that response's replacement when the second attempt is detected.
There is no replay grace window. A lost response can require re-login.

Logout, logout-all, password changes and inactive status revoke refresh families.
Existing access JWTs remain valid until their 15-minute expiry; no denylist or
per-request auth call was introduced. Ordinary permission changes take effect on
the next JWT issuance. Administrative mutations additionally check current DB
permissions/status, so stale admin tokens cannot change roles after demotion.
Refreshing never resets `auth_time`; sensitive admin actions require recent login.

## Roles, resource ownership and business API boundaries

The existing users/roles/permissions/join tables remain authoritative. Registration
assigns only `user` transactionally. Existing accounts receive baseline `user`
capabilities; none receive admin automatically. The old Admin struct is a type
alias to User, not a new credential table. Initial admin assignment must be an
operator-reviewed SQL operation with an audit entry; there is no public bootstrap
endpoint or default admin password.

`user`: accounts.read-own, sessions.manage-own, posts.read-own, posts.create,
posts.update-own, posts.delete-own, chats.member, notifications.manage-own.
`admin`: admin.users.manage, system.audit.read. Audit read is reserved; no audit
HTTP endpoint is exposed. Admin has no implicit private-content access. Ordinary
accounts holding both roles retain their own-resource permissions.

Post/chat/notification HTTP business routes and WebSockets remain unimplemented.
Their existing API groups now attach the verifier; new handlers must attach
operation-specific permission checks. Database connections are real and health
checks now cover PostgreSQL as well as Redis.

New private repository APIs and PostgreSQL tests enforce:

- Post/comment read/update/delete: object ID **and** authenticated author; no
  ownership field in write signatures. Creating a comment/like requires access to
  the parent post. These methods conservatively treat posts as private. A public
  social feed needs a separately reviewed visibility contract.
- Chat: active membership under a conversation lock; message mutations additionally
  require sender ownership. Membership removal takes the same lock. A global admin
  is not a conversation admin. Removed members cannot read or write messages.
- Notifications: recipient predicates on reads/updates/deletes and delivery reads;
  preference upsert derives the user ID solely from the principal. Notification
  creation/delivery is not exposed to arbitrary user clients.
- Sessions: family queries include the current subject; foreign IDs return 404.

These are repository-level protections tested against PostgreSQL, not claims that
unimplemented CRUD HTTP routes have been end-to-end tested. There is still no
internal service API. If introduced, give it distinct workload identity, issuer,
audience and verification policy (or deployment-provided mTLS), never just a user
JWT or a client-supplied service header.

## Rate limits, audit and deployment

Redis policies: registration 3/10 minutes/IP; login 10/minute/IP plus
20/10 minutes/normalized account; refresh 30/minute/IP plus 5/minute/refresh
credential; global 100/minute/IP. Changing password also uses the login IP budget
and a per-subject budget. Credential hashes in keys are hashed again by the
limiter. Authentication policies fail closed with 503 on Redis outage and 429 when
exceeded. Counters have TTLs and use atomic Lua. No permanent account lockout.
Refresh's secondary limiter is **per credential**, not per family; family replay
is enforced transactionally in PostgreSQL. This avoids an extra DB lookup solely
for rate-limit identity.

Audit events include REGISTERED, LOGIN_SUCCESS, LOGIN_FAILED, TOKEN_REFRESHED,
REFRESH_TOKEN_REUSE_DETECTED, LOGOUT, LOGOUT_ALL, SESSION_REVOKED,
PASSWORD_CHANGED, ROLE_CREATED/ASSIGNED/REMOVED, PERMISSION_CREATED/ASSIGNED and
ACCOUNT_STATUS_CHANGED. Actor/target/resource IDs and timestamps are recorded;
request metadata is bounded where available. Privileged/session events are in the
same transaction as their state changes. Runtime auth can insert audit records
but cannot update/delete them. Audit retention/export is an operator responsibility.
Retain consumed token records at least through family expiry; schedule bounded
cleanup afterward and retain security audit events according to your policy.

HTTP logs contain request ID, route template, status and duration. They omit raw
URLs, query strings, headers, cookies, bodies, panic values and SQL parameters.
GORM logging is disabled centrally, avoiding credential-hash SQL leakage.

Local runtime PostgreSQL roles are `app_auth`, `app_post`, `app_chat`,
`app_notification`, restricted to their service tables. The migration identity is
separate. Credentials are generated locally and not committed. Runtime containers
are non-root; only auth mounts a private signing key. Local Redis requires a
password and is loopback-published. Production still needs service-specific Redis
ACLs, network policy and certificate provisioning.

Production configuration fails closed unless strong DB/Redis secrets, a
non-superuser DB identity, `POSTGRES_SSLMODE=verify-full`, authenticated `rediss://`,
secure cookies, enabled limits and `TLS_TERMINATED=true` are supplied. The latter is
an operator assertion, not automatic proof of TLS. Deploy behind HTTPS ingress;
do not publish databases, Redis or internal services. Terminate TLS at a managed
load balancer/ingress, restrict trusted proxy CIDRs, and set HSTS there only when
HTTPS is guaranteed. Mount your database/Redis CA trust where needed. Exact
production infrastructure is outside this repository and was not deployed.

Registration's explicit 409 remains an account-enumeration risk; a uniform
email-verification flow is a separate API/product change. Legacy imported inactive
accounts still require a verified password-setup flow. MFA, email changes, account
deletion, recovery and public content visibility are not silently implemented.
