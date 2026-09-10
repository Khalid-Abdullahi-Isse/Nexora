# Account ownership refactor

Auth-service is now the only account-creation owner. User-service owns social profiles and follows. No other microservice or Docker architecture was changed.

## Current HTTP routes

| Service | Route | Behavior |
| --- | --- | --- |
| Auth (8001) | POST /api/v1/auth/register | Creates an account from email and password; returns a safe 201 response |
| Auth (8001) | GET /health | HTTP 200 |
| User (8002) | GET /health | HTTP 200 |
| User (8002) | POST /api/v1/users | Removed; HTTP 404 |

Registration request:

```json
{"email":"new@example.com","password":"a-long-unique-password"}
```

Registration normalizes email, validates data, checks uniqueness, hashes passwords with bcrypt, generates an account UUID, and inserts an active account into users. Passwords must be 8–72 bytes (bcrypt limit); HTTP binding also validates request shape. Responses never include password hashes. Concurrent duplicate email inserts map to HTTP 409.

Login/logout/refresh/JWT generation and profile/follow endpoints were not added. These remain future functionality with the ownership established here. No public role/admin routes were added and no default role is assigned because the existing project has no default-role policy.

## Table ownership

The existing public schema is preserved; ownership is logical, with no new auth/user PostgreSQL schemas or duplicate tables.

- Auth: users, roles, permissions, user_roles, role_permissions, sessions.
- User: profiles, follows.
- No refresh_tokens table currently exists; any future refresh-token persistence belongs to auth.

RoleService owns role creation/lookup, assignment/removal, and listing user roles. PermissionService owns permission creation, assignment to roles, and user permission checks. These use auth's PostgreSQL implementation; suspended/inactive users do not pass permission checks. Session and access-control entities remain in auth.

No Go internal packages or database objects are shared between services. Future profile operations must use a validated auth user ID. User-service no longer creates independent account UUIDs, stores account models, handles email identity, or imports authentication/password packages.

## Files moved or created in auth-service

- internal/service/UserService.go replaced by internal/service/user_service.go.
- internal/service/user.go: authoritative account entity and creation input.
- internal/models/models.go moved/consolidated into service/user.go and service/identity.go; no new domain layer.
- internal/service/role_service.go and permission_service.go.
- internal/database/postgres/user_operations.go, access.go, and errors.go.
- internal/service/user_service_test.go.
- internal/controller/http/controller_test.go.
- internal/database/postgres/ownership_integration_test.go.
- migrations/000002_import_legacy_profile_accounts.up.sql and .down.sql.

Modified auth files: service/service.go, HTTP controller/request/response/routes files, postgres/database.go documentation, dependencies.go, cmd/server/main.go, go.mod, and go.sum. Existing role/permission/join/session persistence models and migration 000001 are preserved.

## Removed from user-service

- internal/service/user.go.
- internal/service/user_service.go.
- internal/service/user_service_test.go (replaced by auth account tests).
- internal/database/postgres/user.go.
- Account request DTO and controller method.
- POST /api/v1/users registration.
- Email mapping on the PostgreSQL Profile model.

Modified user files: dependencies.go, HTTP controller/request/routes files, controller_test.go, database/postgres/profile.go, and CREATE_USER.md. Module tidy was run; user-service already had no remaining authentication dependencies. The controller test now asserts account routes are absent. Profile/follow models, migrations, and configuration are preserved.

## Migration and existing data

The local database had only profiles/follows and two old profile-created identities. Auth's existing 000001 migration and new 000002 migration were applied together in a transaction. The ownership transfer:

- Copies each legacy email into auth-owned users, preserving the profile user_id and timestamps.
- Creates these imported accounts as inactive with an unusable password marker. No password existed in the old API; a future verified password-setup flow is needed before these accounts can authenticate.
- Leaves all profiles/follows, IDs, names, and email values intact. Both existing profiles were verified to join to auth accounts using the same UUID.
- Aborts on conflicting UUID/email identities instead of overwriting data.
- Preserves the historical profile email column/index solely to avoid data deletion. User-service no longer reads or writes this column. Removing it later is a separate destructive migration requiring a reviewed data-retention decision.
- Does not automatically delete imported accounts on rollback; the down migration explicitly refuses destructive reversal.

For deployments where auth migration 000001 is already applied, apply only auth migration 000002. The migration skips legacy import when the historical profiles.email column does not exist. Runtime services never access each other's tables; this cross-table copy is a one-time operator migration. Migrations are not run automatically on server startup.

No database, volume, profile, or existing account was deleted. Two newly registered smoke-test accounts remain in the local auth users table; integration-test writes rolled back.

## Validation results

- Auth and user: gofmt -w ., go mod tidy, go vet ./..., go test ./..., go build ./... all passed.
- Auth: go test -race ./... passed with AUTH_TEST_DATABASE_URL set to the migrated PostgreSQL database, so the database integration test actually ran.
- Integration test verified account creation/lookup, duplicate constraint mapping, role creation/lookup/assignment/listing/removal, permission creation/assignment/checks, and denial for suspended accounts. Writes were rolled back.
- HTTP tests verified valid/invalid registration, duplicate mapping, sanitized server errors, no credential disclosure, health, and removal of user-service account routes.
- Live Docker registration on localhost:8001 returned 201, exact/case-insensitive duplicates returned 409, and invalid input returned 400.
- Four simultaneous registration requests for one email returned one 201 and three 409 responses.
- PostgreSQL confirmed the new account UUID/email, active status, and bcrypt hash. Service tests also verified the hash against the supplied password.
- localhost:8002/api/v1/users returned 404 for POST.
- Auth and user /health returned 200.
- Both Docker image builds passed. Only auth and user containers were recreated; auth, user, PostgreSQL, and Redis are healthy. Volumes were preserved.
- Source scan found no account logic left in user-service. RegisterRoutes means HTTP route registration, and PostgresPassword/UserPassword refer only to database connection credentials.

## Final service trees

```text
auth-service/
├── cmd/
│   └── server/
│       └── main.go
├── dependencies.go
├── go.mod
├── go.sum
├── internal/
│   ├── config/
│   │   └── config.go
│   ├── controller/
│   │   └── http/
│   │       ├── controller.go
│   │       ├── controller_test.go
│   │       ├── request.go
│   │       ├── response.go
│   │       ├── router.go
│   │       └── routes.go
│   ├── database/
│   │   ├── postgres/
│   │   │   ├── access.go
│   │   │   ├── base.go
│   │   │   ├── database.go
│   │   │   ├── errors.go
│   │   │   ├── ownership_integration_test.go
│   │   │   ├── permission.go
│   │   │   ├── role.go
│   │   │   ├── role_permission.go
│   │   │   ├── session.go
│   │   │   ├── user.go
│   │   │   ├── user_operations.go
│   │   │   └── user_role.go
│   │   └── redis/
│   │       └── database.go
│   └── service/
│       ├── identity.go
│       ├── permission_service.go
│       ├── role_service.go
│       ├── service.go
│       ├── user.go
│       ├── user_service.go
│       └── user_service_test.go
└── migrations/
    ├── 000001_create_auth_schema.down.sql
    ├── 000001_create_auth_schema.up.sql
    ├── 000002_import_legacy_profile_accounts.down.sql
    └── 000002_import_legacy_profile_accounts.up.sql
```

```text
user-service/
├── CREATE_USER.md
├── cmd/
│   └── server/
│       └── main.go
├── dependencies.go
├── go.mod
├── go.sum
├── internal/
│   ├── config/
│   │   └── config.go
│   ├── controller/
│   │   └── http/
│   │       ├── controller.go
│   │       ├── controller_test.go
│   │       ├── request.go
│   │       ├── response.go
│   │       ├── router.go
│   │       └── routes.go
│   ├── database/
│   │   ├── postgres/
│   │   │   ├── base.go
│   │   │   ├── database.go
│   │   │   ├── follow.go
│   │   │   └── profile.go
│   │   └── redis/
│   │       └── database.go
│   ├── models/
│   │   └── models.go
│   └── service/
│       └── service.go
└── migrations/
    ├── 000001_create_user_schema.down.sql
    ├── 000001_create_user_schema.up.sql
    ├── 000002_add_profile_email.down.sql
    └── 000002_add_profile_email.up.sql
```

