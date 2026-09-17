# Postman API collections through Kong

Import `collections/kong-gateway.postman_collection.json` and an environment from
`environments/`. `social-media-backend.postman_collection.json` contains the same
complete API inventory under the original collection filename. Both use Kong's
single `base_url`, defaulting to `http://localhost:8000`.

All **41 implemented routes** are covered, including the two WebSocket handshake
references. Focused auth, post, chat and notification collections use the same
variable names and gateway URLs.

| Service | Routes | Coverage |
| --- | ---: | --- |
| Auth | 13 | Health, registration, login, refresh, CSRF, identity, sessions, password, logout, admin roles |
| Post | 10 | Health/readiness, feed, user posts, create/read/update/delete, likes, comments |
| Chat | 10 | Health/readiness, conversations, messages, read receipts, delete, WebSocket |
| Notification | 8 | Health/readiness, list, unread count, mark read/all, delete, WebSocket |

## Setup and authentication

For the existing kind deployment, run `./scripts/kong-port-forward.sh`. For Compose,
follow [API Gateway](../docs/API_GATEWAY.md). Kong exposes application traffic on
8000; port 8001 is Kong Admin, not the auth application.

1. Import a collection and select `local` or `docker`. Both point at Kong on localhost.
2. Run **05 - Health Checks** for a read-only check of all four services and the
   three implemented readiness endpoints. Auth has no separate `/ready` route.
3. Set `email` and `password` privately in the selected environment. Register a new
   account if needed, then run Login and Authenticated user in **01 - Auth Service**.
4. Run the desired service folder or individual request.

Login and refresh capture `access_token` and `csrf_token` into collection variables.
Authenticated user captures `user_id`. Keep Postman's cookie jar enabled: refresh,
CSRF bootstrap and logout use the HttpOnly refresh cookie; there is no JSON refresh
token. `origin` must match a configured allowed frontend origin. Login, refresh,
CSRF bootstrap and cookie logout send Origin and X-CSRF-Protection; refresh and
cookie logout also send X-CSRF-Token.

Environments deliberately omit captured token/ID keys so blank environment values
do not override collection captures. When switching environments or accounts, clear
old collection tokens/IDs and log in again. Focused collections have independent
variables: copy an access token privately from a login collection when using them.
Never commit credentials, populated exports or tokens. Use ignored `*.local.json`
environment files and `postman/results/` for local exports/reports.

Development, staging and production URLs are `example.com` placeholders. Replace
`base_url`, `ws_base_url` and `origin` with your deployed gateway/frontend addresses.
The former per-service URL templates have been replaced with gateway templates;
native standalone service debugging requires overriding request URLs yourself.

## Request prerequisites and execution

- Post creation captures `post_id` and `user_id`. The Post folder includes positive
  requests and explicit validation/authentication failures. Likes and comments run
  before deletion; the final GET checks that the deleted post returns 404.
- Chat requires `participant_id` for another existing account. Creating a direct
  conversation captures `conversation_id`; sending a message captures `message_id`.
  Conversation/message operations require membership and `chats.member` permission.
- Notification listing captures the first `notification_id` when available. An empty
  inbox cannot supply an ID for mark-read/delete. Generate an event from another
  account (for example, liking your post or sending you a message), then list again.
- **06 - Session mutations** contains individual operations that revoke sessions or
  change credentials. Supply `session_id` explicitly for revocation and
  `new_password` for password changes. Log in again after invalidating your session.
- **07 - Admin roles** requires `admin_access_token` from a recently authenticated
  admin with `admin.users.manage`, plus existing `target_user_id` and `role_id` UUIDs.
- Registration creates a persistent account; no account deletion endpoint exists.
  Rate limits still apply, including registration limits. Respect Retry-After.

Run selected folders, not the entire collection as a single end-to-end scenario:
admin operations, session invalidation, a populated inbox and a second chat account
have separate prerequisites.

```sh
node postman/validate.mjs
npx newman run postman/collections/kong-gateway.postman_collection.json \
  -e postman/environments/docker.postman_environment.json \
  --folder '05 - Health Checks' --timeout-request 5000
```

## WebSockets

**08 - WebSocket references** documents HTTP upgrade headers. Collection Runner
cannot exercise native WebSocket frames. Create a native WebSocket request at:

- `{{ws_base_url}}/api/v1/chats/ws`, subprotocol `chat.v1`
- `{{ws_base_url}}/api/v1/notifications/ws`, subprotocol `notifications.v1`

Use `Authorization: Bearer {{access_token}}`. Never put tokens or user IDs in query
strings. See [Chat service](../docs/CHAT_SERVICE.md) and
[Notification service](../docs/NOTIFICATION_SERVICE.md) for frame formats and browser
subprotocol authentication.

## Maintenance and validation

Mounted Go `routes.go` files and their handlers are the source of truth. Update the
focused collection and both complete collections when adding endpoints. Run
`node postman/validate.mjs` to check full route coverage, request scripts, variable
references and blank secrets in every collection/environment. Existing focused
validator commands delegate to the same full check.

Kong routes are maintained in Helm and generated for Compose:

```sh
python3 scripts/render-kong-compose.py
python3 scripts/render-kong-compose.py --check
python3 scripts/validate-deployment.py
```

Health/readiness aliases rewrite to each upstream's `/health` or `/ready`; business
paths are forwarded unchanged. Static checks do not prove that a running deployment
has the latest backend images or gateway declaration.

Local kind workflow: run `./scripts/start-dev.sh` from the backend root and select
Local or Development (`base_url=http://localhost:8000`). All requests, including
chat and notifications, use Kong. Stop forwarding with `./scripts/stop-dev.sh`.
