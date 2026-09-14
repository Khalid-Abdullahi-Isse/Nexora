# Security Review

Review date: 2026-09-12. Historical pre-implementation review. The user subsequently approved the architecture; see [implementation results](SECURITY_IMPLEMENTATION.md) for changes and current validation.

**Concurrent-edit notice:** At the final status check, changes made outside this review appeared in `Makefile`, `shared/go.mod`, `shared/go.sum`, and migration tooling; `docker/migrations/run.sh`, which was read earlier, had been deleted. The new Makefile references a Go migration command and shared dependencies now include golang-migrate/lib/pq. Findings and passing checks below apply to the files as inspected/tested, not these later replacements. Re-review the completed migration changes and re-run affected checks before implementation. This report does not certify the moving working tree as fully reviewed.

## Scope and evidence limits

Reviewed the current working tree: all four present services' routes, controllers, service boundaries, database models and implemented queries, authentication migrations, shared configuration/server/Redis/rate limiting, Dockerfiles, Compose, migration runner, existing tests, and project documentation. Existing uncommitted edits and user-service deletions were preserved. This report is the only file added by this review.

The repository is a registration implementation plus service scaffolding, not a functioning JWT authentication system. Findings below distinguish current weaknesses, conditional deployment exposure, and missing capabilities. No remotely exploitable JWT bypass, HTTP admin escalation, or HTTP IDOR was demonstrated. Production networking, deployed images, external ingress, cloud secrets, and live production logs were not available as evidence. Historical claims in OWNERSHIP.md are not current test results.

Secret inspection covered current text files and 128 unique blobs across both locally reachable Git commits. Patterns covered private-key markers, common provider tokens, and credential settings. Development credentials occur in historical configuration/documentation and current configuration. No private-key or provider-token pattern matched. No local `.env` exists; only `.env.example` is tracked. This is a bounded pattern scan, not proof that every possible secret is absent; inaccessible remote history, unreachable objects, and external secret stores were not inspected. No Kubernetes manifests or CI workflows were found in the project.

## Current authentication and authorization flow

| Area | Inspected behavior |
| --- | --- |
| Registration | `POST /api/v1/auth/register` binds email/password, validates and normalizes email, checks duplicates, generates a UUID, hashes with bcrypt, inserts an active account, returns a safe DTO. |
| Passwords | bcrypt default cost; service enforces 8–72 bytes. No password verification/login path. Legacy imports are inactive with an unusable password marker. |
| JWT | No signing, parsing, validation, claims, or authentication middleware. `JWT_SECRET` is unused configuration; JWT dependency imports in other services are placeholders. There is no implemented HS256 flow to migrate. |
| Refresh/logout | Session table/model has hash, expiry and revocation fields. No token generator, session operations, rotation, reuse detection, logout, or logout-all. No raw refresh-token storage implementation exists. |
| Roles/permissions | Existing many-to-many tables and internal services; permission query checks active account status. No default role assignment, protected admin route, or authorization middleware. |
| Ownership | Post/comment authors, conversation members, message senders, notification recipients exist as schema fields. Their repositories have no CRUD operations. There are no protected HTTP resource routes to exercise. |
| Other services | Post/chat/notification expose only `/health`; API groups are empty and WebSockets are unimplemented. Their database wrappers receive `nil` connections. |
| Frontend/transport | No frontend implementation, token cookies, CORS policy, CSRF defenses, or TLS ingress in this repository. Absence of CORS does not mean wildcard credentialed CORS. |
| Rate limiting | Shared atomic Redis fixed windows; registration defaults to 3 requests/10 minutes/IP, global to 100/minute/IP. Registration fails closed with 503 on Redis failure; global fails open. Actual health handlers are exempt. |

## Critical Findings

No confirmed critical application vulnerability in the inspected executable route set. This is not a production readiness approval. H1 would permit complete database compromise if an attacker can reach the published port with the configured credentials; internet reachability was not tested.

## High Findings

### H1 — Database exposure and shared privileged credentials

- **Severity:** High; deployment exposure is conditional on host/network reachability.
- **Affected file:** `docker/docker-compose.yml:8`, `:29`, `:34`; `shared/Envfolder/EnvLoader.go`.
- **Affected function:** Compose PostgreSQL/service configuration; `envfolder.Load`.
- **Problem:** PostgreSQL is published as `15432:5432` on all host interfaces with a hard-coded development password. Every application receives the same PostgreSQL superuser credentials. Configuration also defaults to those credentials without production checks.
- **Attack scenario:** A host-network attacker connects using the development credentials, or one compromised service uses shared database credentials to access auth tables.
- **Impact:** Read/write access to identities, password hashes, role assignments, and all business data; asymmetric JWT signing alone would not fix this database trust boundary.
- **Recommended fix:** Bind development database access to loopback; omit production host publication. Use independently provisioned runtime roles restricted to each service's tables and a separate migration identity. Require non-placeholder production secrets; rotate credentials in any deployment that used these values.

### H2 — Plaintext credential transport is the only provided deployment path

- **Severity:** High when used outside isolated development.
- **Affected file:** `shared/server/server.go:18`, `services/auth-service/cmd/server/main.go`, `docker/docker-compose.yml`.
- **Affected function:** `server.Run`, auth `run`, Compose port configuration.
- **Problem:** HTTP listeners are published on all host interfaces; no TLS termination is configured. Auth defaults PostgreSQL to `sslmode=disable`.
- **Attack scenario:** A network observer captures registration passwords on an untrusted network; future login/token traffic would have the same exposure.
- **Impact:** Credential compromise. This review does not establish that an external production proxy is absent, only that none is supplied/documented here.
- **Recommended fix:** Explicit HTTPS ingress, private application ports, narrow trusted proxies, production transport checks, and certificate-verified database TLS across untrusted network boundaries. Retain an explicit loopback development mode.

### H3 — Authentication/session boundary is missing

- **Severity:** High production-readiness gap; not a demonstrated bypass of an existing endpoint.
- **Affected file:** `services/auth-service/internal/controller/http/routes.go:5`, `internal/service/identity.go`, `internal/database/postgres/session.go`; other services' `internal/controller/http/routes.go`.
- **Affected function:** All `RegisterRoutes` functions; missing login/token/session operations.
- **Problem:** JWT and refresh controls requested in the brief are not implemented. Empty route groups do not enforce identity on future endpoints.
- **Attack scenario:** A future CRUD route is added to an existing unprotected group and trusted client IDs become its effective identity source.
- **Impact:** Potential unauthorized access once business routes are introduced; currently login, refresh and logout cannot function at all.
- **Recommended fix:** Shared strict local JWT verifier, authentication on protected groups, and transactional auth session implementation before exposing business routes.

### H4 — Authorization and ownership are not enforceable at current service boundaries

- **Severity:** High design gap; no reachable admin/IDOR exploit demonstrated.
- **Affected file:** Auth `internal/service/{role_service.go,permission_service.go,user_service.go}`, `internal/database/postgres/access.go`; post/chat/notification `internal/service/service.go` and repositories.
- **Affected function:** `AssignRoleToUser`, `RemoveRoleFromUser`, `AssignPermissionToRole`, `SetStatus`; future resource operations.
- **Problem:** Privileged service methods accept targets without an acting principal; no authorization or audit is performed there. Business interfaces are empty and have no owner-scoped query contracts.
- **Attack scenario:** Exposing these methods through a future handler without explicit actor checks enables privilege escalation or cross-user access.
- **Impact:** Missing defense in depth; role database support must not be mistaken for enforced RBAC.
- **Recommended fix:** Explicit authorized service operations for privileged changes, trusted bootstrap kept separate, and owner/membership predicates in every resource repository method. No blanket admin ownership bypass.

## Medium Findings

### M1 — SQL logs can disclose credential material

- **Severity:** Medium, conditional on an error or slow sensitive query.
- **Affected file:** `services/auth-service/cmd/server/main.go:63`; `internal/database/postgres/user_operations.go:15`.
- **Affected function:** `run`, `CreateUser`.
- **Problem:** `gorm.Config{}` uses the default logger. The installed logger defaults to warning-level slow/error SQL logging without parameter redaction; registration INSERT parameters include password hashes and email.
- **Attack scenario:** A failed/slow INSERT emits its interpolated SQL to logs, making credential hashes available to log readers.
- **Impact:** Credential-hash and personal-data disclosure. This is a source-confirmed logging path, not a captured production leak.
- **Recommended fix:** Explicit parameterized/redacted SQL logging and tests with sentinel secrets on error/slow paths. Sanitize startup errors and request/recovery logs; never log token headers, cookie values, bodies, or URL query secrets. [GORM logging documentation](https://gorm.io/docs/logger.html) documents default error/slow-query logging and parameter suppression.

### M2 — Registration reveals account existence

- **Severity:** Medium.
- **Affected file:** `services/auth-service/internal/controller/http/controller.go:26`; `internal/service/user_service.go:34`.
- **Affected function:** `Register`, `CreateUser`.
- **Problem:** Existing email produces explicit `409 EMAIL_EXISTS`; new email produces 201. The duplicate pre-check also changes timing.
- **Attack scenario:** Distributed registration attempts discover registered addresses despite the per-IP limiter.
- **Impact:** Account enumeration and targeting.
- **Recommended fix:** Generic login errors and comparable password-verification work for missing accounts. Registration enumeration resistance requires a deliberate API/product decision, typically a uniform verification-email flow. Merely changing the error text while retaining distinguishable status/body does not solve it. Preserve current 409 compatibility until that contract change is approved; document the residual risk.

### M3 — Request bodies and body-read time are unbounded

- **Severity:** Medium.
- **Affected file:** `shared/server/server.go:18`; auth `internal/controller/http/controller.go:26`.
- **Affected function:** `server.Run`, `Register`.
- **Problem:** Only `ReadHeaderTimeout` is set. No body-size limit or body-read deadline is imposed; DTO validation happens after decoding.
- **Attack scenario:** Distributed clients send oversized JSON strings or trickle bodies to occupy memory/connections. Per-IP rate limits do not bound an accepted request's resource use.
- **Impact:** Availability loss.
- **Recommended fix:** Small auth body limits before binding, bounded HTTP timeouts, exact JSON shape validation and one-object parsing. Use separate lifecycle limits for any future WebSocket connections.

### M4 — Security audit trail is absent

- **Severity:** Medium.
- **Affected file:** Auth `internal/service/{user_service.go,role_service.go,permission_service.go}`; router logging setup.
- **Affected function:** `CreateUser`, `SetStatus`, role/permission mutations.
- **Problem:** Access logs are not structured security events; sensitive mutations have no actor/target/action records.
- **Attack scenario:** Unauthorized role changes or later token replay cannot be reconstructed reliably.
- **Impact:** Reduced detection and incident investigation capability.
- **Recommended fix:** Structured, bounded audit metadata; transactional audit insertion for session and privileged mutations, explicit security events for failed authentication, and restricted retention/access. Never store credentials in event metadata.

### M5 — Redis isolation relies on network access alone

- **Severity:** Medium within the provided development topology.
- **Affected file:** `docker/redis.conf`, `docker/docker-compose.yml`, `shared/redisconn/client.go`.
- **Affected function:** Redis deployment configuration; `redisconn.Client`.
- **Problem:** No Redis ACL/password or TLS is configured in Compose. Redis shares the application network. Host publication correctly uses loopback.
- **Attack scenario:** A process with permitted Redis connectivity tampers with or removes rate-limit keys; successful access depends on runtime Redis protection/network configuration.
- **Impact:** Throttling integrity/availability risk. Sessions are not currently stored there.
- **Recommended fix:** Production network isolation and least-privilege ACLs scoped to service key prefixes; TLS when crossing untrusted networks. Preserve TTLs, bounded operations and supported `rediss://` configuration. No production `KEYS *` use was found.

## Low Findings

### L1 — Secret fields lack model-level JSON exclusion

- **Severity:** Low, latent defense-in-depth issue.
- **Affected file:** Auth `internal/database/postgres/user.go:20`, `session.go:12`.
- **Affected function:** Future model serialization.
- **Problem:** Persistence `PasswordHash` and `TokenHash` fields lack `json:"-"`; the current domain types and response DTO already hide credentials correctly.
- **Attack scenario:** A future handler accidentally serializes a persistence model.
- **Impact:** Possible future hash disclosure; current registration response is protected.
- **Recommended fix:** Add JSON exclusions to persistence types as well and keep dedicated response DTOs.

### L2 — Missing response hardening and inconsistent error envelopes

- **Severity:** Low.
- **Affected file:** All HTTP routers; `shared/ratelimit/middleware.go`; auth HTTP responses.
- **Affected function:** `NewRouter`, `Limiter.Middleware`, `Register`.
- **Problem:** No `nosniff` or sensitive-response `no-store`; auth and rate-limit errors use different envelopes. Unknown routes use Gin defaults.
- **Attack scenario:** Sensitive responses can be retained by clients/intermediaries; inconsistent errors encourage fragile future refresh clients.
- **Impact:** Defense-in-depth and client correctness risk.
- **Recommended fix:** Central error envelope, explicit 400/401/403/404/409/429/500 semantics, no-store on auth responses, nosniff, and HSTS only at guaranteed HTTPS ingress. No irrelevant browser headers required.

### L3 — Containers use root by default

- **Severity:** Low.
- **Affected file:** `docker/*-service.Dockerfile`.
- **Affected function:** Runtime image configuration.
- **Problem:** No runtime `USER` is declared.
- **Attack scenario:** An application compromise starts with root privileges inside its container.
- **Impact:** Increased post-compromise capability; no container escape is claimed.
- **Recommended fix:** Non-root runtime users, read-only mounted verification keys, and minimal write permissions/capabilities appropriate to each service.

## Existing Good Security Controls

- bcrypt hashing with an explicit 72-byte upper limit avoids silent password truncation; service validation catches multibyte passwords too.
- Dedicated registration input accepts no role, status, owner, or hash fields. UUID and active status are assigned by the backend. Unknown JSON fields are ignored, not mass-assigned.
- Domain credential fields have `json:"-"`; safe HTTP DTO and sanitized database failure responses have tests.
- Email normalization and a case-insensitive database unique index protect against duplicate registration races; duplicate constraint errors map to 409.
- Auth queries use placeholders and fixed SQL fragments. No request-derived SQL interpolation, unsafe `Raw()`, or unscoped full-table update was found in implemented operations.
- Existing role/permission joins are reusable; inactive/suspended accounts fail the internal permission query.
- Legacy imports preserve identities, reject conflicts, and remain inactive without usable passwords. The migration runner inspected earlier used a transaction, advisory lock and checksums; its subsequent deletion means these controls must be reassessed in the replacement.
- Redis rate limits use atomic Lua, TTLs, hashed identity keys, bounded deadlines, safe error messages and explicit failure policies. Tests use isolated Redis processes.
- Trusted proxies default to none; configuration rejects trusting all addresses. Forwarded client IDs do not become trusted rate-limit identity.
- Redis host port is loopback-only; Redis client supports credentialed/TLS URLs. `.env` is ignored.

## Recommended Architecture

### Identity, signing and verification

Preserve registration and existing auth-owned roles/permissions/users. Add login, refresh, logout and logout-all under `/api/v1/auth`. Reject inactive, suspended and legacy-password-unset accounts during login and refresh. Use a generic login error and a dummy bcrypt comparison for unknown accounts. Retain bcrypt initially; benchmark cost before increasing it. Do not add a duplicate Admin identity table.

Use RS256 with a securely generated private key mounted only into auth and a configured public-key set in every verifier. Access tokens live exactly 15 minutes and carry `sub`, `roles` (preserving the existing multiple-role model), necessary permissions, `iss`, `aud`, `iat`, `exp`, `jti`, and optionally `sid`/`auth_time` when needed. Exclude email and credential material. Backend data alone supplies roles/permissions.

The shared verifier must strictly parse a single Bearer credential, pin RS256, select only configured `kid` values, verify signature and require valid subject/issuer/audience/issued-at/expiry, validate `nbf` if present, reject impossible lifetimes/future issuance, and bound token size/claim structure. Require the current service's configured audience; ignore token-provided key URLs. Save a typed principal in Gin context and pass it explicitly to service methods. These checks follow [RFC 8725](https://www.rfc-editor.org/rfc/rfc8725.html).

There is no HS256 implementation to preserve. Remove unused shared `JWT_SECRET` after introducing explicit key configuration; do not add a compatibility fallback accepting symmetric tokens. If an external deployed issuer exists, inventory it before rollout. For RS256 key rotation, distribute the new public key first, switch auth's signing `kid`, retain the old public key for the maximum access lifetime plus clock allowance, then retire it.

No Auth Service call on ordinary protected requests. Role/status revocation and logout affect already issued JWTs only when they expire, up to 15 minutes plus any configured validation leeway. Use zero expiry leeway if a hard 15-minute boundary is required. Immediate revocation is a separate availability/cache tradeoff; no default denylist.

### Refresh sessions and races

Use 32 random bytes from `crypto/rand`, base64url encoded. Store only SHA-256 of that high-entropy refresh value in PostgreSQL. Default absolute session-family lifetime is seven days, configurable; rotation does not extend the family's absolute lifetime. Retain consumed token hashes until family expiry so reuse remains detectable.

Extend existing sessions to represent token generations with family ID, replacement ID and consumption time, and add a family record with user, creation/expiry/revocation metadata. Use a unique token hash and a constraint preventing multiple active generations per family. Keep device/IP/user-agent optional, length-bounded, and subject to retention rules.

Login creates family and first generation transactionally. Refresh validates input, locks the user then family then token row in a consistent order, verifies active user/family/token/expiry, marks the old generation consumed, inserts its replacement and audit event, and commits before returning credentials. Sign before committing so signing failure rolls back the transition. A reused consumed generation revokes the entire family and records the event in a transaction that **commits even though the HTTP response is 401**. Do not roll back replay revocation by returning the authentication error directly from the transaction callback.

The same lock order must serialize refresh with logout-all, password/status changes and session creation as needed. Concurrent use of one refresh token must never mint two active descendants. Under strict replay policy, a second concurrent request revokes the family, including the first request's replacement. The frontend must single-flight refresh across requests/tabs and retry an API request only once. A lost refresh response can require re-login; no replay grace window is proposed. Family replay handling follows the rotation principle in [RFC 9700](https://www.rfc-editor.org/rfc/rfc9700.html).

Logout accepts the current refresh credential, revokes its family and clears its cookie, including when the access JWT has expired. Logout-all requires a valid access principal and revokes all that user's families. Both leave existing access JWTs to expire. Sensitive account changes require password verification or a trusted recent `auth_time`; refreshing an access JWT does not reset the original authentication time.

### Browser transport and perimeter

Default proposal: serve frontend and auth through the same HTTPS site. Return access tokens in JSON for in-memory use; deliver refresh tokens only in a host-only Secure, HttpOnly, SameSite=Lax cookie scoped to auth endpoints. Clear using identical cookie path/domain attributes. Do not also expose the browser refresh value in JSON. Local HTTP cookie exceptions must be explicit development configuration.

Enforce trusted Origin checks and CSRF tokens on cookie-authorized state changes, including refresh/logout, and prevent login CSRF. Support cross-origin credentials only through an exact configured allowlist; a cross-site frontend requiring SameSite=None needs a deliberate transport configuration. No wildcard credentialed CORS. Native clients, if later needed, get an explicit transport contract; never token query parameters. No frontend code exists here to implement silent refresh yet.

Add separate Redis policies for login, register and refresh, including IP and normalized account signals for login, and IP/family signals for refresh. Avoid permanent account lockouts and avoid raw email/token values in keys/logs. Bound error-path work and apply throttling before expensive verification. Sensitive auth limits fail closed with sanitized 503 on backend failure, and 429 with Retry-After when exceeded.

### Roles, permissions and ownership

Reuse existing role and permission tables. Seed a `user` role transactionally for new accounts; never promote existing identities automatically. Seed known permission codes and define explicit admin capabilities. Preserve conversation member/admin roles separately from global application admin. Server-side bootstrap uses an operator-only path, not a public registration field.

| Resource | Proposed enforcement |
| --- | --- |
| Account/self sessions | Subject matches account owner; session lookup/delete includes `user_id = principal.sub`. Explicit separate audited admin routes if approved. |
| Posts | Writes scoped by `id` and `author_user_id`; create author from principal. Public vs private reading needs a documented product policy before adding read endpoints. |
| Comments | Update/delete scoped to author; creation verifies access to parent post and parent comment belongs to that post. Moderation is an explicit capability. |
| Likes | Acting user always comes from principal; parent post must be accessible; delete includes both post and acting user. |
| Conversations | Active membership (`left_at IS NULL`) required for private reads; membership changes require conversation-specific authority. |
| Messages | Active conversation membership plus sender ownership for own-message mutation; create sender from principal. Membership and write checks must remain atomic against removal races. |
| Notifications/preferences | Scope by recipient user. Delivery records inherit recipient authorization through their parent notification. Users cannot create arbitrary internal notifications for other recipients. |

Repositories should expose operation-specific methods such as `GetOwnedPost` or `ListMessagesForMember`, not generic unscoped retrieval for private routes. Permission checks do not replace owner/membership predicates. Reject ownership/role/security-field manipulation in update DTOs; return 404 for private resources that do not match the actor, 403 for missing endpoint permission and 401 for invalid identity. Pagination, IDs, payload size and nested resource relationships need explicit validation.

No internal HTTP calls are currently implemented. Keep internal-only endpoints unexposed; if introduced, use separately issued, audience-bound short-lived service credentials with a distinct token type/issuer/key boundary. A user JWT must never pass service-identity verification. Prefer deployment-provided mTLS/workload identity when that infrastructure exists, without building a service mesh for local development.

## Implementation Plan

Implementation should begin only after approval of this report's architecture. The following is a proposed change map, not completed changes.

| Files/modules | Why / security issue | Proposed change | Expected result |
| --- | --- | --- | --- |
| `go.work`, `Makefile`, Compose, migration runner, README | Deleted user-service references block builds/migrations | Reconcile the removed runtime without resurrecting user changes; preserve required historical profile migration artifacts in a documented archive | Stable build and data-preserving migration path |
| Auth `internal/database/postgres/Admin.go` | Current incomplete untracked file blocks parsing | Resolve unfinished model with user intent; use existing users/roles rather than a second admin identity | Auth builds; one identity model |
| `shared/authn/*`, `shared/authz/*`, shared module dependencies | Missing cryptographic/authorization boundary | One verifier, typed principal, role/permission middleware and security tests | Consistent local verification |
| Auth `internal/config/config.go`, `cmd/server/main.go`; shared EnvLoader; other service mains/config | Shared unused JWT secret, weak production defaults | Private/public key separation, issuer/audience config, redacted SQL logger, explicit production checks | Least-privilege signing and safer startup |
| Auth HTTP controller/request/response/routes, `dependencies.go`, service files | Missing auth operations | Login/refresh/logout/logout-all, strict DTOs, cookies/CSRF, active-account checks | Working session lifecycle preserving register API |
| Auth `internal/database/postgres/session.go`, new session operations/family model; new numbered migrations | Missing atomic rotation/replay protection | Session generations/families, indexes/constraints, transactions and consistent locks | Single-use refresh and race-safe revocation |
| Auth existing role/permission/user services and `access.go`; new seed migration | Unenforced privileges | Default user assignment, authorized actor-aware operations, explicit admin permissions | Backend RBAC/PBAC without blanket bypass |
| Auth audit service/repository and new migration | No security audit records | Structured append-only events, transaction integration and restricted retention/access | Traceable sensitive actions |
| `shared/ratelimit/*`, `shared/server/*`, HTTP routers | Per-IP-only limits, unbounded body handling, missing headers | Auth-specific policies, body/time bounds, safe logging/errors/headers | Bounded abuse and consistent errors |
| Post/chat/notification routes/services/repositories/mains | Empty interfaces and nil DBs | Attach verification to protected groups now; implement owner-scoped operations with real DB wiring when adding agreed business routes | No new unprotected CRUD; scaffold groups alone are not tested IDOR coverage |
| `.env.example`, `.gitignore`, Dockerfiles/Compose and deployment docs | Hard-coded credentials and broad exposure | Placeholder-only examples, key exclusions, secret mounts, least-privilege DB/Redis identities, non-root containers, private production ports and TLS contract | Safer deployment boundary |
| Existing tests, new auth/ownership integration tests and Postman documentation | Missing security regression coverage | Execute matrix below, update docs to actual supported endpoints | Reviewable evidence and accurate client contract |

Proposed environment settings: signing key file and active key ID (auth only), public key-set file, JWT issuer, per-service audience, refresh family TTL, allowed origins, cookie/CSRF settings, explicit production mode and auth rate-limit policies. Exact names belong in implementation documentation. Access TTL remains 15 minutes. Existing DB/Redis settings remain but lose insecure production fallbacks. No environment variables or schemas were changed during review.

Before implementing business APIs, resolve post visibility, chat membership/moderation rules, administrator access to private data, and whether deleted user-service functionality is intentionally retired. The secure default is no admin access to private content and no resurrection of the deleted runtime. A full new CRUD product is not silently implied by securing the existing scaffolds.

## Validation results and required acceptance tests

### Checks performed

| Check | Result |
| --- | --- |
| `go version` | Installed Go reports `go1.27.0 linux/amd64`. |
| Root `go test ./...` | Failed: `go.work` references missing `services/user-service/go.mod`. Root also has no `go.mod`; use explicit workspace module patterns once repaired. |
| Per-module `GOWORK=off go test ./...` and `go vet ./...` | Both passed in shared, post, chat and notification modules. |
| Auth `GOWORK=off go test ./...` | Full module failed on `Admin.go:2` (`expected 'package', found 'type'`). Controller and service package tests passed within that invocation. |
| Auth `GOWORK=off go vet ./...` | Failed on the same parser error. |
| Read-only `gofmt -l` over Go files | Parser error for `Admin.go`; no other formatting paths reported. No formatting write performed before approval. |
| `docker compose -f docker/docker-compose.yml config --quiet` | Passed syntax/configuration validation. Does not establish build/startup success or safety. |
| Redis integration | Existing shared tests passed using isolated Unix-socket Redis processes, including atomic limits, expiry, outage behavior, proxy handling and connection/probe tests. |
| PostgreSQL integration | Not executed successfully: auth package cannot compile. Its existing test additionally requires `AUTH_TEST_DATABASE_URL`; it tests account/role operations, not A-vs-B HTTP IDOR. |
| Full service deployment/manual login | Not run: source build/migration blockers and missing endpoints. No existing deployment or database was modified. |

After implementation run formatting, vet, tests and race tests on explicit module patterns. Run a dedicated isolated PostgreSQL/Redis stack, migrations and service startup; capture logs with synthetic sentinel credentials and verify redaction. Never infer database readiness from other services' current Redis-only health responses.

Required tests:

1. JWT: missing/malformed/duplicate Bearer credentials; expired/future/missing claims; wrong signature/algorithm/issuer/audience/key ID; forged role; valid token; exact 15-minute issuance.
2. Refresh: valid/expired/malformed/unknown/revoked tokens; hash-only persistence; rotation; consumed-token reuse revoking family; concurrent refresh; refresh versus logout/logout-all/status/password changes; signing/storage failure rollback; replay revocation survives 401.
3. Authorization: normal user/admin boundaries; absent permissions; multiple roles; manipulated frontend role ignored/rejected; conversation admin does not imply system admin; system admin has no implicit private-content bypass.
4. Ownership: two users and resources per implemented type. Test own and foreign GET/PATCH/PUT/DELETE; forged owner on POST/update; nested-resource mismatches; guessed UUIDs; inaccessible list results; inactive conversation membership. Test directly at repository/service boundaries too. Absent endpoints must be marked unimplemented, not passed.
5. Browser/session behavior: Secure/HttpOnly/SameSite/path, CSRF/Origin rejection, cookie clearing, CORS allowlist, no-store, one refresh/retry, failed refresh clears client state. Frontend verification requires an actual frontend implementation.
6. Abuse and audit: login/register/refresh policy thresholds, distributed counters, account/IP combinations, fail-closed behavior, body/time limits, generic login errors, redacted success/error/panic logs, and actor/target audit records.

### Manual verification status

All 18 requested end-to-end scenarios remain **not verified** against a running, source-matched system. Registration and limiter behavior have automated partial evidence only. Login, access token acceptance/expiry, refresh, reuse, logout, logout-all, admin enforcement, cross-user operations, owner manipulation, forged tokens/roles, login/refresh throttling and credential-free startup logs require the implementation and isolated runtime checks above.

## Review-phase completion status

- **Security changes completed / critical and high vulnerabilities fixed:** None; this phase intentionally performed analysis only.
- **Architecture:** Proposed above; not implemented or approved yet.
- **Remaining risks:** All findings remain; deployment exposure is conditional, missing capabilities are explicitly identified, and live production security is unverified.
- **Future improvements:** Verified email/password recovery for inactive legacy accounts, optional device/session management, breach-password screening, MFA/step-up, signing-key rotation automation, dependency vulnerability scanning and production ingress verification after the core architecture is established.
- **Tests passed / failed:** See exact baseline results above; no new security tests were claimed.
- **Files changed by this review:** `SECURITY_REVIEW.md` only. Other working-tree changes either predated this review or appeared concurrently, as noted above.
- **Database changes / environment variables added:** None.
- **Next approval:** Approve or amend the proposed architecture and implementation scope. This gate comes from the user's explicit instruction to complete the review before implementing the approved architecture.
