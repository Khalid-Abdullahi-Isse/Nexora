# Postman API Collection

## Overview

This collection covers **every implemented HTTP route** in Social Media Backend:

| Microservice | Local / Docker host port | Implemented routes | Requests in service folder |
| --- | --- | --- | --- |
| auth-service | 8001 | `GET /health`, `POST /api/v1/auth/register` | 3 (including validation error) |
| post-service | 8003 | `GET /health` | 1 |
| chat-service | 8004 | `GET /health` | 1 |
| notification-service | 8005 | `GET /health` | 1 |

There are five distinct routes and ten runnable requests: six in isolated service folders and four convenience copies in **05 - Health Checks**. Saved 400 and 409 registration responses are documented examples, not live captures. No speculative endpoints are included.

The source of truth is `services/<service>/internal/controller/http/routes.go`, together with `router.go`, `controller.go`, `request.go`, `response.go`, service logic and `cmd/server/main.go`. Ports come from `shared/Envfolder/EnvLoader.go`, `.env.example` and `docker/docker-compose.yml`. No local `.env`, Kubernetes deployment manifests or deployed domains were present during inspection.

## Import

1. Open Postman.
2. Import `collections/social-media-backend.postman_collection.json`.
3. Import a template from `environments/`.
4. Select that environment.
5. Start the required services and their dependencies.
6. Run **05 - Health Checks**, or an individual service's Health request.

## Environments

- **local**: native Go processes on localhost, ports 8001, 8003, 8004 and 8005 (the configured defaults).
- **docker**: Postman/Newman runs on the host and uses published localhost ports 8001, 8003, 8004 and 8005. Docker DNS names such as `auth-service` are not reachable from ordinary host Postman. For a runner inside `social-network`, override service base URLs with the corresponding Compose DNS name and container port.
- **development**, **staging**, **production**: HTTPS `*.example.com` placeholder domains, explicitly labeled in the environment names. Replace these with your actual deployment addresses before use. No gateway is present in this repository.

All service base URLs are **origins without a trailing slash or API prefix**. For example, `authBaseUrl={{protocol}}://{{host}}:8001`. This keeps `/health` unversioned and registration at `{{authBaseUrl}}/api/{{apiVersion}}/auth/register`.

The collection pre-request script recursively resolves URL templates into request-local variables, validates missing/circular references, and leaves saved environment templates intact. It only resolves the selected request's dependencies. Avoid defining environment keys again as globals, collection variables or runner data columns unless you deliberately want to override them.

## Authentication

All current endpoints are public and explicitly use **No Auth**. Empty secret-typed `accessToken`, `refreshToken`, `adminAccessToken` and `userAccessToken` variables are reserved for future APIs. Roles, permissions and sessions exist in the auth domain, but no login, refresh, logout, role administration or authorization middleware is exposed. **There is no automatic token capture yet because no implemented response returns a token.** Registration returns an account, not a session.

When token endpoints are implemented, inspect their actual response DTOs, add success-only extraction to their post-response scripts, and use Bearer `{{accessToken}}` on protected requests/folders. Keep public requests on No Auth. Do not infer token fields or role names from the database models.

## Registration and dynamic IDs

Set `registerEmail` to a fresh test email and `registerPassword` to a private test password in your selected environment. Both templates are blank. Email must be valid and at most 255 characters. Password must satisfy the DTO's 8–72-character binding and the service's 8–72-byte limit; ASCII avoids that distinction. No name or role field is accepted.

Run **02 - Register Account**. The pre-request script serializes credentials as JSON, so special characters are escaped safely. On HTTP 201, tests validate the response contract and save `data.id` as `userId`. A new attempt clears an old `userId` first to prevent accidental reuse after failure. Other IDs are omitted because no other create API exists. No current read/update/delete endpoint consumes `userId`.

Run **03 - Registration Validation Error** to exercise HTTP 400 with an empty body. To check HTTP 409 manually, resend a previously successful email; the normal registration tests will correctly fail because that request expects 201. Its saved response examples describe the 400/409 contracts without duplicating a conflict request.

Registration is limited by default to **three attempts per IP per ten minutes**, including invalid and duplicate attempts. HTTP 429 includes `Retry-After`. Redis failure may cause 503. Wait for the advertised interval; do not reset rate-limit keys to make tests pass. Every successful registration persists a test account; no deletion endpoint exists for cleanup. Running the entire collection therefore needs registration inputs and creates an account. Use the Health Checks folder for a read-only smoke run.

## Updating service URLs

To move auth to port 9001, change only `authBaseUrl` from `{{protocol}}://{{host}}:8001` to `{{protocol}}://{{host}}:9001`. All auth requests and its health-check copy follow the change. To move every service to another machine, change `host` once. To change the common transport, change `protocol` once. Individual base URLs can instead contain a complete independent address.

Change `apiVersion` once when the backend actually implements a new version. It cannot make an unsupported backend version exist. A per-service reverse-proxy mount can be included in that service's base URL if it fronts both health and API paths. If a future deployment routes health separately, introduce a service-specific health base variable then.

## Running individual microservices

From the repository root, start only the desired Compose service and its declared dependencies:

```sh
docker compose -f docker/docker-compose.yml up -d --build auth-service
```

Select **01 - Auth Service** in Postman. Other API services are not required. Compose starts Redis, PostgreSQL and the migration job because those are declared dependencies. Its migration job processes all service schemas; review the existing migration README before initial startup on an existing database.

For native execution, configure the backend environment and run, for example:

```sh
go run ./services/auth-service/cmd/server
```

All four current binaries require Redis. Auth also initializes PostgreSQL. When native Go uses Compose PostgreSQL, set the backend's `POSTGRES_PORT=15432`; the native default is 5432. Postman only needs the HTTP service addresses, never database credentials.

## Running the full system

```sh
docker compose -f docker/docker-compose.yml up -d --build
npx --yes newman@6.2.1 run postman/collections/social-media-backend.postman_collection.json \
  -e postman/environments/docker.postman_environment.json \
  --folder '05 - Health Checks' --timeout-request 5000
```

Run commands from the repository root. A successful health test checks HTTP 200, JSON, service identity, `status=ok`, Redis connectivity and, for auth, PostgreSQL connectivity. Health failures are not treated as successful smoke checks. There is no gateway or meaningful cross-service end-to-end flow yet; registration is the only business HTTP operation.

To run the auth folder in Newman, create a private environment copy ending in `.local.json`, populate the registration inputs there, and pass its path with `-e` and `--folder '01 - Auth Service'`. To retain captured IDs across separate Newman invocations, use `--export-environment postman/environments/run.local.json`. Store reports under ignored `postman/results/`. Do not overwrite the committed safe templates with populated exports.

## WebSockets

Chat and notification contain placeholder WebSocket controller/client/hub files. They have no upgrade handler, registered URL, authentication method, events or message wire format. Constructors are instantiated but no WebSocket handler is mounted. No `chatWsUrl` or fake handshake request is included. Document these contracts and add the appropriate Postman WebSocket workflow when they are implemented.

## Issues and implementation gaps

- Auth registration is the only business route. Login/refresh/logout, profile/follow APIs, post/comment/like APIs, chat and notification operations remain unimplemented despite domain and persistence models.
- Chat, post and notification reserve `/api/v1/chats`, `/api/v1/posts`, and `/api/v1/notifications` groups without any handlers; requesting those prefixes does not access an API. User has no API group yet.
- No gateway, media service or admin HTTP API exists. No 401/403 resource tests or invented 404 resource route is included.
- All four current health endpoints exist; configured HTTP ports and Docker mappings agree. No duplicate registered routes were found.
- Post/chat/notification use placeholder nil PostgreSQL persistence wiring. Their health verifies Redis only and does not prove database readiness.
- During this task, another process deleted `services/user-service/` after its health endpoint had been verified. The final collection follows the current source tree and excludes this service. `go.work`, Compose (build and migration mounts), and environment examples still reference the deleted directory/service. Workspace builds and Compose rebuilds need those references reconciled by the ongoing backend work; this task does not modify them. The existing user container can still respond on port 8002 despite its source being absent.
- Current production templates are placeholders, not evidence of a deployed or production-ready system.

## Maintenance and security

Keep the collection JSON as the editable source of truth. To add a service, inspect its mounted routes, create its own folder and base URL in all five environments, and add a health convenience request only if a real endpoint exists. Update DTO bodies, exact expected statuses and successful ID/token extraction alongside backend changes. Keep the two copies of each health request synchronized. Extend this inventory and run `node postman/validate.mjs` to detect route coverage drift.

Never commit populated passwords, tokens or private exports. Secret typing masks values in the UI; it does not encrypt exported JSON. `.gitignore` excludes `*.local.json` environment copies, `postman/secrets/`, `postman/results/` and `*.postman-secrets.json` while retaining the five blank templates.

Postman references: [variable scripting](https://learning.postman.com/latest-v-12/docs/tests-and-scripts/write-scripts/postman-sandbox-reference/pm-variables) and [collection schema](https://schema.postman.com/).

## Verification results

Verified on 2026-09-11 against existing local Docker containers, without rebuilding services or modifying backend code:

- Official Postman v2.1 JSON schema validation passed.
- JavaScript syntax, environment variable resolution, independent URL overrides and API-version changes passed.
- All initially running health endpoints returned 200; the final four-service health folder was rerun after the source-tree change.
- Auth folder: 3 requests and 9 assertions passed (health, 201 registration with UUID capture, 400 validation).
- A separate duplicate registration returned 409 `EMAIL_EXISTS`; exported `userId` was confirmed.
- One new test account remains because the API provides no delete operation. Temporary credentials and populated verification exports were removed.
- Login, token refresh and resource read/update/delete cannot be exercised because those routes do not exist. Live results describe the running containers, which may differ from concurrently edited source.
