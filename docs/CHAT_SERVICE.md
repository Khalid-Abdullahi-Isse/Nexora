# Nexora chat service

The service extends the existing Gin/controller → service → GORM repository layout.
PostgreSQL in the shared `public` schema is authoritative. Both REST and WebSocket
use the same service operations. Auth-service remains the sole token issuer.

## Routing and authentication

The existing Kong prefix is `/api/v1/chats` (plural), preserved end to end with
`strip_path: false`. REST base: `https://gateway/api/v1/chats`; WebSocket:
`wss://gateway/api/v1/chats/ws`. Kong's HTTP proxy handles the upgrade and its
existing upstream timeouts exceed the default 25-second heartbeat interval.
Use TLS in production. No direct public chat port or sticky session is required.

REST and native WebSocket clients send `Authorization: Bearer <access_token>`.
Browser clients use:

```js
const socket = new WebSocket('wss://gateway/api/v1/chats/ws', [
  'chat.v1', 'bearer.' + accessToken
]);
```

Only `chat.v1` is echoed in the upgrade response. Query tokens are not accepted.
The shared verifier checks RS256 only, known public-key `kid`, token type, issuer,
`chat-service` audience, UUID subject/session/token IDs, issued-at, expiration,
15-minute access lifetime, roles and permissions. The `chats.member` permission
is required. No signing private key is mounted into chat. Socket expiry closes the
connection; refresh through auth-service, reconnect and fetch history. Existing
stateless verification means logout/revocation takes effect no later than access
expiry; chat does not perform per-event auth-service introspection.

## REST contract

All success responses use `{ "success": true, "data": ... }`; failures use
`{ "success": false, "error": { "code": "...", "message": "..." } }`.

| Method | Path relative to prefix | Input |
|---|---|---|
| POST | `/conversations` | `{"participantId":"UUID"}` |
| GET | `/conversations?limit=30` | Current user's latest conversations, last message and unread count |
| GET | `/conversations/:conversationId` | Active member only |
| GET | `/conversations/:conversationId/messages?limit=30&before=UUID` | Descending `(created_at,id)` keyset pagination |
| POST | `/conversations/:conversationId/messages` | `{"content":"Hello"}` |
| POST | `/messages/:messageId/read` | Receipt owner is the authenticated user |
| DELETE | `/messages/:messageId` | Active member and original sender only; soft delete |

Limits are 1–100, default 30. History returns `{messages, nextCursor}`; an empty
cursor ends pagination (a full final page may require one additional empty fetch).
Conversation listing currently returns the latest bounded page, without a cursor.
No N+1 queries: conversation summaries are one SQL statement with indexed subqueries.
Strict JSON rejects ownership fields and other unknown fields. Text is trimmed,
valid UTF-8, nonblank, free of NUL bytes and at most 4096 bytes by default.

Nonmembers and foreign senders receive the repository's existing opaque 404
`NOT_FOUND`, preventing resource-existence disclosure. Missing permission gives
403 `FORBIDDEN`. Malformed input is 400 `INVALID_PAYLOAD`. Database errors remain
private. Message identity, sender and timestamps always come from the server.

## WebSocket lifecycle and events

Authentication and connection rate limiting precede upgrade. The hub registers
multiple sockets per user, sends `connection.ready`, and runs one reader and one
writer per connection. Queues are bounded; slow consumers close. Ping/pong resets
the read deadline. Writes, handshakes and operations have timeouts; frames have a
size limit. Expiry, network failure, shutdown and normal disconnect remove clients.

```json
{"type":"message.send","requestId":"abc-123","data":{"conversationId":"UUID","content":"Hello"}}
{"type":"typing.start","data":{"conversationId":"UUID"}}
{"type":"typing.stop","data":{"conversationId":"UUID"}}
{"type":"message.delivered","data":{"conversationId":"UUID","messageId":"UUID"}}
{"type":"message.read","data":{"conversationId":"UUID","messageId":"UUID"}}
{"type":"conversation.leave","data":{"conversationId":"UUID"}}
{"type":"conversation.join","data":{"conversationId":"UUID"}}
```

`message.created` contains the persisted message, including `senderId`, and echoes
`requestId`. Other connected tabs/devices receive it too. Receipts are client
acknowledgements, not inferred from a socket write; they are persisted with unique
(message,user) identity and stable first delivered/read timestamps. Read implies
delivered. The supplied conversation must match the stored message. Receipt
broadcasts include authenticated `userId`, `messageId`, `conversationId`, and times.
Typing never writes to PostgreSQL. All operations verify membership.

Sockets initially receive all their active conversations. Join/leave checks
membership and resumes/mutes that conversation on this socket only; it does not
add/remove database membership. At most 256 muted conversations per socket.
Unknown events, malformed JSON and rate limits return structured `error` events.
`requestId` is correlation only, not a deduplication key: after an ambiguous
network failure fetch history before retrying a send.

## Database and Redis flow

Migration `000003_chat_api` is additive and uses the existing migration runner.
It adds member UUIDs, user foreign keys, direct-pair uniqueness, typed messages,
same-conversation replies, soft deletion, history indexes and receipts. The
existing composite membership primary key continues to prevent duplicate members.
Legacy duplicate direct conversations are preserved; the oldest two-active-member
conversation becomes the canonical pair. New concurrent creates converge through
a unique index. Inactive/unknown participants and self-chat are rejected.

User foreign keys are initially `NOT VALID`: historical orphan rows are retained,
while new writes are enforced. Audit historical data before separately validating
those constraints. Existing migrations are not rewritten. The runtime role gets
only user ID/status read access in auth's table, not password or session access.
Apply with `make migrate-chat-up` after auth migrations, or the existing global
migration job; server startup never mutates schema. The down migration removes
new chat features and must not be used on live data without a reviewed rollback.

Membership-sensitive operations lock the conversation and check active membership
inside a transaction. All future membership mutation code must use this lock too.
A send commits first, then resolves recipients and publishes. Local sockets get
immediate delivery; a Redis envelope on `chat:events:v1` carries server-selected
recipient IDs and an origin ID. Other pods fan out only to those local users.
Origin filtering prevents duplicate delivery on the publishing pod. Redis channels
and credentials are backend-only; frontend input cannot select channels/recipients.

Redis Pub/Sub is transient, with no replay guarantee. Subscriber reconnect is
handled by go-redis. An outage or a crash between commit and publication can miss
live events; messages remain in PostgreSQL and clients recover via history. There
is no outbox or exactly-once send promise. Redis rate checks fail closed during
outages, so new operations may return temporary errors. Existing committed data
is unaffected. The shared startup pattern requires Redis; `/ready` subsequently
checks PostgreSQL and shutdown state, while `/health` only checks process liveness.

## Configuration and deployment

| Variable | Default | Meaning |
|---|---|---|
| CHAT_SERVICE_PORT | 8004 | Internal HTTP/WS port |
| CHAT_MESSAGE_MAX_LENGTH | 4096 | Text bytes |
| WS_MAX_MESSAGE_SIZE | 16384 | Incoming frame bytes |
| WS_SEND_QUEUE | 64 | Outbound messages per connection |
| WS_MAX_CONNECTIONS_PER_USER | 8 | Per-pod socket cap per user |
| WS_WRITE_TIMEOUT | 10s | Write/handshake bound |
| WS_READ_TIMEOUT | 60s | Pong/read deadline |
| WS_PING_INTERVAL | 25s | Must be shorter than read timeout |
| CHAT_RATE_LIMIT | 120 | Authenticated REST operations/WS events per user per minute |

Existing `RATE_LIMIT_WEBSOCKET_REQUESTS/WINDOW` controls connection attempts;
shared global IP limiting also applies to REST/handshakes. Chat's per-user limiter
is mandatory even if the shared optional global limiter is disabled. Shared
POSTGRES_*, REDIS_ADDR, JWT_PUBLIC_KEYS_FILE, JWT_ISSUER and ALLOWED_ORIGINS remain
unchanged. Configure these through Compose environment and Helm `config` values.
No secrets are added to Git.

The existing multi-stage nonroot `docker/chat-service.Dockerfile` and Compose
service are reused. Compose now probes `/ready`. PostgreSQL/Redis remain on the
internal service network, with existing loopback-only developer port mappings.
Helm retains ClusterIP, configurable replicas/resources, public-key Secret mounts,
and ArgoCD migration ordering. Chat readiness now uses `/ready`, liveness `/health`.
On SIGINT/SIGTERM readiness fails, new work is rejected, the hub drains bounded
operations and closes sockets, HTTP drains, and dependency pools close.

## Validation

Run from the repository root (there is a Go workspace, not a root module):

```sh
make check
go test -race ./shared/... ./services/auth-service/... ./services/post-service/... ./services/chat-service/... ./services/notification-service/...
python3 scripts/validate-deployment.py
python3 scripts/render-kong-compose.py --check
docker compose --env-file .env -f docker/docker-compose.yml config --quiet
```

For chat integration tests, point `RESOURCE_TEST_DATABASE_URL` at an **isolated**
auth/chat-migrated PostgreSQL database and `CHAT_TEST_REDIS_ADDR` at isolated Redis,
then run `go test -race ./services/chat-service/...`. Repository tests use rolled
back transactions. Integration tests skip with an explicit reason if dependencies
are absent. Tests cover authentication, spoofing, membership, receipts, keyset
history, soft deletion, WebSocket events, backpressure, remote-pod delivery and
save-before-publish/Redis failure. Import
`postman/collections/chat-service.postman_collection.json`; create a Postman
WebSocket request separately using the URL/auth/events above.

Implementation follows Gorilla's [one reader / one writer contract](https://github.com/gorilla/websocket/blob/main/doc.go)
and Redis's [at-most-once Pub/Sub semantics](https://redis.io/docs/latest/develop/pubsub/).
