# Security implementation results

Implemented after approval of [the pre-implementation review](SECURITY_REVIEW.md).
This report describes the current source and isolated test evidence, not a security
certification of an external production deployment. Existing unrelated edits and
the removed user-service runtime were preserved.

## Security Changes Completed

- Auth login, refresh, CSRF bootstrap after browser reload, logout, logout-all, own-session list/revocation, own-account
  lookup and password changes, preserving registration's existing path/response.
- RS256 access JWTs with strict shared local verification and 15-minute expiry.
- Seven-day absolute refresh families, hash-only storage, atomic rotation, replay
  family revocation and race-safe logout/password revocation.
- Backend role/permission checks, recent authentication for admin mutations,
  repeated current-database admin checks and security audit records.
- Owner/membership-scoped repositories for posts/comments/likes, chats/messages,
  notifications/preferences/deliveries, plus cross-user PostgreSQL tests.
- Strict DTO decoding, bounded requests/timeouts, safe logging, exact CORS origins,
  cookie/CSRF controls, auth-specific Redis rate limits and production config checks.
- Loopback development ports, generated local secrets, separate database runtime
  identities, non-root images, and private signing-key isolation.
- Workspace/migration repair: keep the retired user runtime deleted and archive its
  immutable SQL so legacy identity history is retained.

## Critical Vulnerabilities Fixed

No confirmed critical application exploit existed in the reviewed route set, so
none is claimed as fixed. The original PostgreSQL exposure could have enabled
complete database compromise if reachable; its source deployment configuration is
now loopback-only with generated secrets. Existing external deployments still need
credential rotation and network verification.

## High-Risk Vulnerabilities Fixed

The missing authentication/session boundary is implemented. Shared-secret token
forgery is avoided by asymmetric signing and public-only verifier services. New
private queries include ownership/membership predicates, and privilege mutations
require backend authorization. PostgreSQL runtime identities have service-specific
grants. This does not imply that previously absent business HTTP endpoints have
been implemented or penetration-tested.

## Authentication Architecture

Auth verifies bcrypt passwords and active account status, creates a refresh family,
issues a 900-second JWT and sends a refresh cookie. Services verify JWTs locally,
requiring signature/RS256, configured key ID, token type, subject, issuer, their own
audience, timestamps, original authentication time and session identifier. No
private key or per-request Auth Service validation is needed outside auth.

JWT signing was previously absent; unused `JWT_SECRET` configuration was removed.
There is no HS256 fallback. Auth checks its public/private configuration at startup.
See [the key rotation and API contract](docs/AUTH_SECURITY.md).

## Authorization Architecture

Authentication sets a typed principal in Gin/request context. Role/permission
middleware guards operations separately. Auth administrative service and repository
methods repeat authorization, require login within five minutes, lock actor/target
users in a consistent order and verify current database authority. A stale admin
JWT cannot mutate roles after the admin role is removed. No blanket admin override.

## Ownership / IDOR Protection

Auth's `/me` derives identity from the token; session queries/revocation include
both family ID and token subject. Foreign session deletion returns 404. Owned
repository methods derive author/sender/recipient from the principal and expose no
owner-changing input fields. Cross-user tests exercise reads, writes, deletion,
parent access and membership removal. Chat membership-sensitive operations share a
conversation lock. Admin status alone grants no private-content access.

Post/chat/notification business HTTP endpoints and WebSockets remain scaffolds.
Their groups attach authentication, and their repositories now have tested private
operations. Public feed visibility, group creation/moderation, business controllers
and comprehensive HTTP CRUD matrices require their actual product contracts.

## Refresh Token Architecture

32 random bytes, base64url transport; only SHA-256 hashes stored. Existing sessions
are extended with family/consumption/replacement metadata. User/family/token locks
and a unique active-generation index serialize refresh. Signing/audit/write failures
roll back issuance. Reuse revokes the entire family in a transaction committed before
401. Concurrent refresh creates at most one replacement, which is subsequently
revoked when the second attempt triggers strict replay handling.

Logout, logout-all and password changes revoke refresh state. Access JWTs naturally
expire within 15 minutes. Original authentication time survives refresh. There is
no token denylist, replay grace window or rolling extension of the seven-day limit.
A lost response can require login. The absent frontend must single-flight refresh
and retry original API calls only once.

## Role and Permission Model

Existing users/roles/permissions/join tables are reused. Registration assigns
`user`; no frontend privileged fields are accepted. Existing users receive baseline
user capabilities, never automatic admin promotion. The old Admin type is an alias
to User. Conversation roles remain separate from global roles.

`user` owns account/session/post capabilities, active chat membership access and
own notifications. `admin` has explicit user-management and reserved audit-read
permissions; no audit HTTP endpoint is added. Bootstrap admin assignment remains an
operator action, not a public endpoint/default credential.

## Remaining Security Risks

- Registration still returns distinguishable 409 for existing email, preserving
  its contract. Eliminating enumeration requires a uniform verification flow.
- Already issued access tokens survive logout/status/permission changes until expiry;
  current DB checks narrow this window for administrative mutations.
- Concurrent local Kubernetes/Kind files appeared during the final pass. A read-only
  spot check found development settings and private-key secret mounts; this work did
  not deploy or fully validate that separate configuration. Its ignored private key
  has mode 0600. It is not included in the Docker or runtime validation claims.
- No actual frontend, production HTTPS ingress, cloud network policy, production
  Redis ACL deployment or automated key rotation was available for verification.
  `TLS_TERMINATED=true` is an operator assertion, not proof of HTTPS.
- The shipped Compose file is development-only. Existing databases need explicit
  runtime-role provisioning and credential transition; their volumes/passwords were
  not changed by these tests.
- Password recovery/MFA/email verification/account deletion and inactive legacy
  account onboarding are not implemented. Internal service endpoints/workload
  identity are not invented where none existed.
- Audit/token-history retention and cleanup require an operator schedule. Retain
  consumed hashes through family expiry; restrict audit access and export/retain it
  according to policy.
- Dependency versions were resolved and builds tested; no claim of a complete
  vulnerability-database scan or production penetration test is made.

## Recommended Future Improvements

Implement the frontend refresh coordinator, verified email/recovery and legacy
onboarding, deployment-managed TLS/workload identity and Redis ACLs, key rotation
automation, audit export/retention, and business-specific visibility policies before
publishing additional CRUD APIs. Add HTTP ownership tests alongside each new route.

## Tests Passed

- Per-module `go fmt ./...`; shared/auth/post/chat/notification modules build.
- `go test -race -count=1` across all five modules against isolated PostgreSQL/Redis,
  including migration-history integration and current security migrations.
- `go vet` across all modules.
- JWT matrix: absent/malformed/duplicate Bearer, wrong signature/algorithm including
  unsigned tokens, wrong issuer/audience/key, missing claims, future/expired claims,
  valid identity and role/permission combinations.
- Refresh: malformed/unknown/expired/revoked tokens, rotation, replay family audit,
  concurrent reuse, refresh versus logout-all, signing-failure rollback, logout,
  logout-all and password-change revocation.
- PostgreSQL ownership matrices and HTTP session/role manipulation tests; secure
  cookie attributes, login CSRF, wrong Origin/CSRF, strict payloads, stale admin denial.
- Live CSRF bootstrap after reload, including rejection of a hostile Origin.
- Live isolated service registration/login/access/rotation/reuse/logout/logout-all,
  forged token denial, admin denial, login/refresh 429 responses and log redaction.
- Runtime database-grant checks: other services cannot read/update auth tables;
  auth cannot delete audit records.
- Compose configuration and all four service images plus migration image build.

- Final disposable Docker startup passed: fresh PostgreSQL role provisioning,
  migrations, four healthy services, registration/login, non-root users,
  public/private key mount separation and log redaction. Only the test project's
  new containers/network/volumes were removed afterward.

## Tests Failed

No unresolved failure in the latest completed isolated security run. An initial
expiry fixture violated the database's expiry-after-creation constraint; the fixture
was corrected to create valid expired state and the entire suite passed afterward.
The pre-review build blockers were fixed. Missing frontend/production tests are
unverified, not counted as passed.

## Database Changes

Auth 000003 adds `session_families`, generation fields/indexes on `sessions` and
`security_audit`, revokes unused historical session records, and seeds baseline
roles/permissions. It preserves users/passwords and does not promote admins. Its
down migration refuses automatic deletion of security history. Auth 000004 and each
other active service's 000002 grant privileges to pre-provisioned `app_*` roles.
Historical user SQL is archived unchanged; no applied SQL checksum was rewritten.
Only disposable test databases/volumes were migrated during validation.

## Environment Variables Added

`JWT_PRIVATE_KEY_FILE` (auth only), `JWT_PUBLIC_KEYS_FILE`, `JWT_KEY_ID`,
`JWT_ISSUER`, `REFRESH_TOKEN_TTL`, `ALLOWED_ORIGINS`, `AUTH_COOKIE_INSECURE`,
`TLS_TERMINATED`, login/account/refresh/credential rate-limit policy variables,
`POSTGRES_ADMIN_PASSWORD`, per-service database passwords, `REDIS_PASSWORD`,
`LOCAL_UID`/`LOCAL_GID`. Tests use `RESOURCE_TEST_DATABASE_URL` alongside existing
auth/migration test URLs. The unused `JWT_SECRET` was removed. Examples contain
placeholders; setup generates ignored local values without printing them.

## Manual Verification Results

The live HTTP checks are automated scripts exercising real running services, not
claims of a human browser session.

| Requested scenario | Evidence |
| --- | --- |
| Successful login/access | Live isolated HTTP, including safe DTO/cookie transport |
| 15-minute expiry | Live claim interval 900 seconds plus verifier clock-boundary test; no 15-minute wall-clock wait |
| Refresh/new tokens/old token invalid | Live HTTP and PostgreSQL transaction tests |
| Continue without password | Live refresh followed by protected request; frontend UX not present |
| Logout/logout-all | Live HTTP plus family revocation and race tests |
| Normal user denied admin API | Live HTTP and integration tests |
| User A denied User B resources | Auth session HTTP plus post/chat/notification repository matrices |
| URL manipulation | Auth session UUID HTTP test; other business HTTP routes do not exist |
| JSON owner/role manipulation | Strict registration DTO HTTP test and owner-free repository write signatures |
| Forged role/JWT/expired JWT | Cryptographic unit matrix, live forged JWT and RBAC tests |
| Login/refresh throttling | Live 429 responses plus isolated Redis atomic-limit tests |
| No sensitive logs | Live application logs scanned for generated passwords, raw access/refresh and key/hash markers |

## Files Changed

The following manifest covers files changed for this security implementation.
Pre-existing user-service deletions, unrelated Postman exports and other earlier
working-tree edits are not attributed to this work. Each row supplies FILE, WHY,
SECURITY ISSUE, CHANGE and EXPECTED RESULT; all changes remain uncommitted.

| FILE | WHY | SECURITY ISSUE | CHANGE | EXPECTED RESULT |
| --- | --- | --- | --- | --- |
| `go.work` | Repair active workspace | Deleted runtime blocked validation | Remove deleted module from build patterns; retain migration tooling | Four active services build |
| `Makefile` | Repair active workspace | Deleted runtime blocked validation | Remove deleted module from build patterns; retain migration tooling | Four active services build |
| `go.work.sum` | Resolve shared verification/database code | Missing required dependencies | Use existing pinned JWT/GORM libraries; update checksums | Independent reproducible module builds |
| `shared/go.mod` | Resolve shared verification/database code | Missing required dependencies | Use existing pinned JWT/GORM libraries; update checksums | Independent reproducible module builds |
| `shared/go.sum` | Resolve shared verification/database code | Missing required dependencies | Use existing pinned JWT/GORM libraries; update checksums | Independent reproducible module builds |
| `services/auth-service/go.mod` | Resolve shared verification/database code | Missing required dependencies | Use existing pinned JWT/GORM libraries; update checksums | Independent reproducible module builds |
| `services/auth-service/go.sum` | Resolve shared verification/database code | Missing required dependencies | Use existing pinned JWT/GORM libraries; update checksums | Independent reproducible module builds |
| `shared/authn/authentication.go` | Centralize identity verification | Missing JWT boundary | Pinned RSA validation, typed principal, RBAC/PBAC/recent-auth middleware | Local trusted identity and explicit authorization |
| `shared/authn/authentication_test.go` | Test cryptographic failures | Forgery/claim regression risk | Positive and negative JWT/header/RBAC matrix | Invalid tokens and permissions denied |
| `shared/httpsecurity/http.go` | Bound and sanitize HTTP | Unbounded input, CSRF/CORS/logging gaps | Strict JSON, origins, deadlines, safe request logs and tests | Bounded requests without credential logs |
| `shared/httpsecurity/http_test.go` | Bound and sanitize HTTP | Unbounded input, CSRF/CORS/logging gaps | Strict JSON, origins, deadlines, safe request logs and tests | Bounded requests without credential logs |
| `shared/dbconn/postgres.go` | Share safe connections | Sensitive SQL logging and nil DBs | Bounded pool with disabled SQL logs | Real service DB connections without DSN leaks |
| `shared/ownership/ownership.go` | Share ownership error semantics | Untrusted IDs/permissions | Validate principal/capability/IDs and sanitize DB results | Consistent private-resource denial |
| `shared/Envfolder/EnvLoader.go` | Fail closed in production | Insecure defaults and boolean bypass | Production secret/TLS/limiter checks; remove unused secret | Misconfigured production process refuses startup |
| `shared/Envfolder/security_test.go` | Fail closed in production | Insecure defaults and boolean bypass | Production secret/TLS/limiter checks; remove unused secret | Misconfigured production process refuses startup |
| `shared/ratelimit/config.go` | Protect authentication work | Brute force and refresh abuse | Login/refresh policies and common error envelope | 429 throttling; auth Redis failures return 503 |
| `shared/ratelimit/middleware.go` | Protect authentication work | Brute force and refresh abuse | Login/refresh policies and common error envelope | 429 throttling; auth Redis failures return 503 |
| `shared/redisconn/client.go` | Keep TLS verification enabled | Redis skip_verify URL bypass | Reject disabled certificate verification and test it | Encrypted Redis also authenticates the server |
| `shared/redisconn/client_test.go` | Keep TLS verification enabled | Redis skip_verify URL bypass | Reject disabled certificate verification and test it | Encrypted Redis also authenticates the server |
| `shared/server/server.go` | Bound HTTP connection lifetime | Slow client availability risk | Header/read/write/idle limits and graceful shutdown | Bounded connection handling |
| `shared/cmd/security-keys/main.go` | Provision secrets safely | Hard-coded keys/passwords | Generate RSA/random secrets with exclusive private files | No committed or printed secrets |
| `scripts/setup-security-dev.py` | Provision secrets safely | Hard-coded keys/passwords | Generate RSA/random secrets with exclusive private files | No committed or printed secrets |
| `shared/cmd/migrate/main.go` | Preserve migration history | Deleted user runtime broke dependency history | Resolve archived user SQL; retain legacy-prefix integration tests | Migration compatibility without restoring runtime |
| `shared/cmd/migrate/main_test.go` | Preserve migration history | Deleted user runtime broke dependency history | Resolve archived user SQL; retain legacy-prefix integration tests | Migration compatibility without restoring runtime |
| `docker/migrations/legacy-user/000001_create_user_schema.down.sql` | Retain immutable schema history | Lost identity-import prerequisite | Archive exact historical migration contents | Existing checksums and identities preserved |
| `docker/migrations/legacy-user/000001_create_user_schema.up.sql` | Retain immutable schema history | Lost identity-import prerequisite | Archive exact historical migration contents | Existing checksums and identities preserved |
| `docker/migrations/legacy-user/000002_add_profile_email.down.sql` | Retain immutable schema history | Lost identity-import prerequisite | Archive exact historical migration contents | Existing checksums and identities preserved |
| `docker/migrations/legacy-user/000002_add_profile_email.up.sql` | Retain immutable schema history | Lost identity-import prerequisite | Archive exact historical migration contents | Existing checksums and identities preserved |
| `services/auth-service/internal/service/auth.go` | Implement authentication lifecycle | Missing password verification/session rotation | Login, signer, refresh/CSRF tokens, session/password operations | Short access plus revocable rotating refresh |
| `services/auth-service/internal/service/role_service.go` | Guard service-layer privileges | Unscoped privileged mutations | Require trusted recent admin principal before mutation | Handlers cannot accidentally bypass authorization |
| `services/auth-service/internal/service/permission_service.go` | Guard service-layer privileges | Unscoped privileged mutations | Require trusted recent admin principal before mutation | Handlers cannot accidentally bypass authorization |
| `services/auth-service/internal/service/user_service.go` | Guard service-layer privileges | Unscoped privileged mutations | Require trusted recent admin principal before mutation | Handlers cannot accidentally bypass authorization |
| `services/auth-service/internal/service/service.go` | Guard service-layer privileges | Unscoped privileged mutations | Require trusted recent admin principal before mutation | Handlers cannot accidentally bypass authorization |
| `services/auth-service/internal/database/postgres/auth.go` | Serialize session state | Replay/races/revocation gaps | User/family/token locking, owner queries, transactional audit | Single-use tokens and durable replay revocation |
| `services/auth-service/internal/database/postgres/admin_operations.go` | Repeat current admin authority | Stale or missing permissions | Locked actor/target validation and mutation audit | Demoted admins cannot mutate roles |
| `services/auth-service/internal/database/postgres/access.go` | Repeat current admin authority | Stale or missing permissions | Locked actor/target validation and mutation audit | Demoted admins cannot mutate roles |
| `services/auth-service/internal/database/postgres/user_operations.go` | Make account changes atomic | Default-role/status/audit gaps | Transactional registration/role/audit and status revocation | No partially initialized account/session state |
| `services/auth-service/internal/database/postgres/user.go` | Protect credential models | Latent serialization and duplicate identity risk | JSON exclusions, session-generation fields, Admin alias | Credential fields hidden; one identity table |
| `services/auth-service/internal/database/postgres/session.go` | Protect credential models | Latent serialization and duplicate identity risk | JSON exclusions, session-generation fields, Admin alias | Credential fields hidden; one identity table |
| `services/auth-service/internal/database/postgres/admin.go` | Protect credential models | Latent serialization and duplicate identity risk | JSON exclusions, session-generation fields, Admin alias | Credential fields hidden; one identity table |
| `services/auth-service/internal/database/postgres/auth_integration_test.go` | Exercise real transactional controls | Race/IDOR/privilege regressions | PostgreSQL token/session/admin/ownership tests | Security invariants verified on actual database |
| `services/auth-service/internal/database/postgres/ownership_integration_test.go` | Exercise real transactional controls | Race/IDOR/privilege regressions | PostgreSQL token/session/admin/ownership tests | Security invariants verified on actual database |
| `services/auth-service/internal/controller/http/auth.go` | Expose secure auth contract | Missing auth routes and unchecked payloads | Cookies/CSRF, strict DTOs, scoped routes, rate limits and error mapping | Usable protected authentication/session APIs |
| `services/auth-service/internal/controller/http/controller.go` | Expose secure auth contract | Missing auth routes and unchecked payloads | Cookies/CSRF, strict DTOs, scoped routes, rate limits and error mapping | Usable protected authentication/session APIs |
| `services/auth-service/internal/controller/http/routes.go` | Expose secure auth contract | Missing auth routes and unchecked payloads | Cookies/CSRF, strict DTOs, scoped routes, rate limits and error mapping | Usable protected authentication/session APIs |
| `services/auth-service/cmd/server/main.go` | Wire runtime security | Unprotected initialization and unsafe DB logging | Load verifier/origins/DB/limiter; auth alone loads signer | Startup checks and locally verified identity |
| `services/auth-service/internal/controller/http/router.go` | Use common safe HTTP boundary | Default raw request/recovery logging | Install sanitized middleware | Consistent limits and non-secret logs |
| `services/auth-service/migrations/000003_secure_sessions.up.sql` | Persist security architecture | Missing session/audit/least-privilege state | Forward schema/grants; deliberate rollback behavior | Hash-only sessions and restricted runtime database access |
| `services/auth-service/migrations/000003_secure_sessions.down.sql` | Persist security architecture | Missing session/audit/least-privilege state | Forward schema/grants; deliberate rollback behavior | Hash-only sessions and restricted runtime database access |
| `services/auth-service/migrations/000004_runtime_grants.up.sql` | Persist security architecture | Missing session/audit/least-privilege state | Forward schema/grants; deliberate rollback behavior | Hash-only sessions and restricted runtime database access |
| `services/auth-service/migrations/000004_runtime_grants.down.sql` | Persist security architecture | Missing session/audit/least-privilege state | Forward schema/grants; deliberate rollback behavior | Hash-only sessions and restricted runtime database access |
| `services/post-service/cmd/server/main.go` | Wire runtime security | Unprotected initialization and unsafe DB logging | Load verifier/origins/DB/limiter; auth alone loads signer | Startup checks and locally verified identity |
| `services/post-service/internal/controller/http/router.go` | Use common safe HTTP boundary | Default raw request/recovery logging | Install sanitized middleware | Consistent limits and non-secret logs |
| `services/post-service/internal/controller/http/controller.go` | Protect future business groups | Scaffold groups had no auth boundary | Attach configured shared verifier | New routes in existing groups inherit authentication |
| `services/post-service/internal/controller/http/routes.go` | Protect future business groups | Scaffold groups had no auth boundary | Attach configured shared verifier | New routes in existing groups inherit authentication |
| `services/post-service/internal/database/postgres/owned.go` | Enforce private resource access | BOLA/ownership/membership risk | Scoped repository methods with cross-user PostgreSQL tests | Foreign resources denied even for global admins |
| `services/post-service/internal/database/postgres/owned_test.go` | Enforce private resource access | BOLA/ownership/membership risk | Scoped repository methods with cross-user PostgreSQL tests | Foreign resources denied even for global admins |
| `services/post-service/migrations/000002_runtime_grants.up.sql` | Persist security architecture | Missing session/audit/least-privilege state | Forward schema/grants; deliberate rollback behavior | Hash-only sessions and restricted runtime database access |
| `services/post-service/migrations/000002_runtime_grants.down.sql` | Persist security architecture | Missing session/audit/least-privilege state | Forward schema/grants; deliberate rollback behavior | Hash-only sessions and restricted runtime database access |
| `services/chat-service/cmd/server/main.go` | Wire runtime security | Unprotected initialization and unsafe DB logging | Load verifier/origins/DB/limiter; auth alone loads signer | Startup checks and locally verified identity |
| `services/chat-service/internal/controller/http/router.go` | Use common safe HTTP boundary | Default raw request/recovery logging | Install sanitized middleware | Consistent limits and non-secret logs |
| `services/chat-service/internal/controller/http/controller.go` | Protect future business groups | Scaffold groups had no auth boundary | Attach configured shared verifier | New routes in existing groups inherit authentication |
| `services/chat-service/internal/controller/http/routes.go` | Protect future business groups | Scaffold groups had no auth boundary | Attach configured shared verifier | New routes in existing groups inherit authentication |
| `services/chat-service/internal/database/postgres/owned.go` | Enforce private resource access | BOLA/ownership/membership risk | Scoped repository methods with cross-user PostgreSQL tests | Foreign resources denied even for global admins |
| `services/chat-service/internal/database/postgres/owned_test.go` | Enforce private resource access | BOLA/ownership/membership risk | Scoped repository methods with cross-user PostgreSQL tests | Foreign resources denied even for global admins |
| `services/chat-service/migrations/000002_runtime_grants.up.sql` | Persist security architecture | Missing session/audit/least-privilege state | Forward schema/grants; deliberate rollback behavior | Hash-only sessions and restricted runtime database access |
| `services/chat-service/migrations/000002_runtime_grants.down.sql` | Persist security architecture | Missing session/audit/least-privilege state | Forward schema/grants; deliberate rollback behavior | Hash-only sessions and restricted runtime database access |
| `services/notification-service/cmd/server/main.go` | Wire runtime security | Unprotected initialization and unsafe DB logging | Load verifier/origins/DB/limiter; auth alone loads signer | Startup checks and locally verified identity |
| `services/notification-service/internal/controller/http/router.go` | Use common safe HTTP boundary | Default raw request/recovery logging | Install sanitized middleware | Consistent limits and non-secret logs |
| `services/notification-service/internal/controller/http/controller.go` | Protect future business groups | Scaffold groups had no auth boundary | Attach configured shared verifier | New routes in existing groups inherit authentication |
| `services/notification-service/internal/controller/http/routes.go` | Protect future business groups | Scaffold groups had no auth boundary | Attach configured shared verifier | New routes in existing groups inherit authentication |
| `services/notification-service/internal/database/postgres/owned.go` | Enforce private resource access | BOLA/ownership/membership risk | Scoped repository methods with cross-user PostgreSQL tests | Foreign resources denied even for global admins |
| `services/notification-service/internal/database/postgres/owned_test.go` | Enforce private resource access | BOLA/ownership/membership risk | Scoped repository methods with cross-user PostgreSQL tests | Foreign resources denied even for global admins |
| `services/notification-service/migrations/000002_runtime_grants.up.sql` | Persist security architecture | Missing session/audit/least-privilege state | Forward schema/grants; deliberate rollback behavior | Hash-only sessions and restricted runtime database access |
| `services/notification-service/migrations/000002_runtime_grants.down.sql` | Persist security architecture | Missing session/audit/least-privilege state | Forward schema/grants; deliberate rollback behavior | Hash-only sessions and restricted runtime database access |
| `docker/docker-compose.yml` | Isolate development credentials/network | Public DB and shared privileged credentials | Loopback ports, generated secrets, runtime roles and key mounts | Local isolation without deleting existing data |
| `docker/postgres-init.sh` | Isolate development credentials/network | Public DB and shared privileged credentials | Loopback ports, generated secrets, runtime roles and key mounts | Local isolation without deleting existing data |
| `docker/auth-service.Dockerfile` | Reduce container privilege | Root runtime defaults | Non-root runtime user | Reduced container privilege |
| `docker/post-service.Dockerfile` | Reduce container privilege | Root runtime defaults | Non-root runtime user | Reduced container privilege |
| `docker/chat-service.Dockerfile` | Reduce container privilege | Root runtime defaults | Non-root runtime user | Reduced container privilege |
| `docker/notification-service.Dockerfile` | Reduce container privilege | Root runtime defaults | Non-root runtime user | Reduced container privilege |
| `docker/migrate.Dockerfile` | Reduce container privilege | Root runtime defaults | Non-root runtime user | Reduced container privilege |
| `.env.example` | Keep secrets outside Git | Hard-coded or accidentally tracked secrets | Placeholder examples and key/env exclusions | Private local provisioning |
| `.gitignore` | Keep secrets outside Git | Hard-coded or accidentally tracked secrets | Placeholder examples and key/env exclusions | Private local provisioning |
| `scripts/test-security.py` | Verify actual runtime behavior | Unit-only confidence gap | Disposable DB/Redis/service/Docker checks and cleanup | Reproducible evidence without touching project data |
| `scripts/test-security-docker.py` | Verify actual runtime behavior | Unit-only confidence gap | Disposable DB/Redis/service/Docker checks and cleanup | Reproducible evidence without touching project data |
| `README.md` | Document final architecture and limits | Stale configuration/security claims | Current contracts, migration, rollout and evidence | Reviewable implementation and explicit remaining work |
| `RATE_LIMITING.md` | Document final architecture and limits | Stale configuration/security claims | Current contracts, migration, rollout and evidence | Reviewable implementation and explicit remaining work |
| `docs/MIGRATIONS.md` | Document final architecture and limits | Stale configuration/security claims | Current contracts, migration, rollout and evidence | Reviewable implementation and explicit remaining work |
| `docs/AUTH_SECURITY.md` | Document final architecture and limits | Stale configuration/security claims | Current contracts, migration, rollout and evidence | Reviewable implementation and explicit remaining work |
| `SECURITY_REVIEW.md` | Document final architecture and limits | Stale configuration/security claims | Current contracts, migration, rollout and evidence | Reviewable implementation and explicit remaining work |
| `SECURITY_IMPLEMENTATION.md` | Document final architecture and limits | Stale configuration/security claims | Current contracts, migration, rollout and evidence | Reviewable implementation and explicit remaining work |

Manifest: 92 files.
