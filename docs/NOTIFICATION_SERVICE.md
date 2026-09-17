# Realtime notification service

The existing service scaffold is now a working Gin service on **8005**. It uses
the project's shared PostgreSQL deployment, Redis connection configuration,
RS256 access tokens, runtime database role, Kong routes, Helm chart and migration
runner. No Kafka, RabbitMQ, separate database, or monitoring stack was introduced.

## The complete flow

```text
Ahmed likes Khalid's post
          │
          ▼
     post-service
          │
    PostgreSQL transaction
    saves like + outgoing event
          │
     outbox relay retries
          │ post.liked
          ▼
      Redis Stream
          │ consumer group
          ▼
 notification-service
          │
    ┌─────┴────────────┐
    ▼                  ▼
PostgreSQL         Redis Pub/Sub
notification       realtime channel
+ event receipt        │
    │                  ▼
    │          notification replicas
    │                  │
    │                  ▼
    │              WebSocket
    │                  │
    └── HTTP history ──▼
                     Khalid
```

The producer reports what happened. Notification-service decides the title,
message, type, and whether a notification is appropriate. A user liking their
own post produces no notification. Connected devices receive updates quickly;
offline users retrieve stored notifications later.

The implementation guarantees idempotent **database creation**, with at-least-once
event handling. Realtime delivery is best effort and may repeat. The frontend
must deduplicate by notification `id` and reload history/count on reconnect.

## Phase 1 — permanent storage and HTTP

1. **What:** notification records, unread count, paginated history, mark-read,
   mark-all-read and delete operations.
2. **Why:** PostgreSQL is permanent storage. WebSocket delivery must never replace
   the saved record or delete it after sending.
3. **How:** the existing `notifications.user_id` is the recipient column;
   `data` holds JSON metadata. `read_at IS NOT NULL` determines `isRead`, avoiding
   two separate read-state fields that could disagree. The unread partial index
   speeds counts; `(user_id, created_at DESC, id DESC)` supports latest history.
4. **Real user:** Khalid opens the bell and loads his first 30 notifications.
   Reading one changes only his row. Ahmed receives 404 if he tries its ID.
5. **Files:** `internal/models/notification.go` defines the API item;
   `internal/database/postgres/notifications_api.go` stores and lists it;
   `owned.go` enforces ownership for individual mutations;
   `internal/service/service.go` owns event rules;
   `internal/controller/http/` translates HTTP into service calls.
6. **Other services:** they send events; they cannot call a public create-
   notification endpoint because there is no such endpoint.
7. **Without this:** disconnected users lose history, unread badges cannot be
   rebuilt, and insufficient ownership filters would expose private data.

Page/limit follows the existing post API. `limit` is 1–100; `page` is 1–1,000,000.
Ordering is deterministic. Inserting new rows between pages can shift offset
pages; refresh page 1 and deduplicate IDs. Cursor pagination is a future upgrade
for deep histories. No API returns unlimited history.

| Method | Path | Result |
|---|---|---|
| GET | `/api/v1/notifications?page=1&limit=30` | `{items, page, limit, hasMore}` |
| GET | `/api/v1/notifications/unread-count` | `{count: 7}` |
| PATCH | `/api/v1/notifications/:id/read` | 204; repeated reads preserve first `read_at` |
| PATCH | `/api/v1/notifications/read-all` | 204; only caller's unread rows |
| DELETE | `/api/v1/notifications/:id` | 204; absent/foreign ID returns 404 |
| GET | `/api/v1/notifications/ws` | authenticated WebSocket upgrade |
| GET | `/health` | process liveness |
| GET | `/ready` | PostgreSQL and Redis readiness |

API routes require a valid access token and `notifications.manage-own` permission.
IDs in request bodies or query parameters never select the owner.
Kong exposes liveness at `/api/v1/notifications/health`; readiness stays internal.

## Phase 2 — authenticated WebSockets

1. **What:** a hub with many connections per user, bounded outgoing queues,
   read/write pumps, ping/pong, deadlines and graceful closure.
2. **Why:** normal HTTP means the frontend asks for data. WebSocket keeps a
   connection open so the server can immediately send new data.
3. **How:** shared JWT verification checks signature, issuer, audience, expiration,
   roles and permissions. Only the verified subject chooses the hub's user key.
   Browsers send a non-echoed `bearer.<JWT>` subprotocol because the browser
   WebSocket API cannot set an Authorization header. Postman can use that header.
   Origins are checked against `ALLOWED_ORIGINS`. Non-browser clients may omit
   Origin but still need valid authentication. A timer closes sockets at token expiry.
4. **Real user:** Khalid's phone and two browser tabs all receive the notification.
   One slow tab fills its queue and disconnects without blocking the others.
5. **Files:** `internal/controller/websocket/controller.go` handles the handshake
   and read pump; `client.go` owns writes and heartbeat; `hub.go` tracks connections.
6. **Other services:** none manipulates socket connections; they publish domain facts.
7. **Without this:** the frontend must poll, forged user IDs could leak data, and
   unbounded queues or blocked writers could exhaust the service.

The server emits `connection.ready`, then `notification` messages:

```json
{
  "type": "notification",
  "data": {
    "id": "notification-uuid",
    "recipientId": "khalid-uuid",
    "type": "post_like",
    "title": "New like",
    "message": "Ahmed liked your post",
    "actorId": "ahmed-uuid",
    "entityType": "post",
    "entityId": "post-uuid",
    "metadata": {"actorName": "Ahmed"},
    "isRead": false,
    "createdAt": "2026-09-17T12:00:00Z"
  }
}
```

Post/chat producers currently omit actor display names, so the service says
“Someone liked your post” unless a trusted producer supplies `metadata.actorName`.
Render all message text as plain text, never HTML. Chat events contain message ID
and conversation ID, not private message content. Clicking a chat notification
opens its conversation; clicking a post notification opens the post.

Sockets are server-push only. Use HTTP for mutations. On a notification event,
upsert by ID and fetch unread-count for an accurate badge. Re-fetch after marking
read/deleting, on window focus and on reconnect; read-state changes are not
currently broadcast to other tabs. Refresh the access token and reconnect after
expiry. Logout should close sockets locally; immediate remote session revocation
is not provided by the project's stateless JWT verification.

## Phase 3 — durable events, retries and producers

1. **What:** one shared event contract, a Redis Stream consumer group, a retry
   loop, dead letters, and transactional producer outboxes.
2. **Why:** a Redis Stream is like a notebook where services write events. Workers
   keep track of unfinished entries, allowing another worker to retry after a crash.
3. **How:** `XREADGROUP` reads new entries. Success saves the notification plus an
   event receipt in one PostgreSQL transaction, publishes realtime, then `XACK`s.
   Failure leaves the entry pending. `XAUTOCLAIM` reclaims entries idle for 30s;
   each processing attempt has a 10s timeout. Invalid events go straight to the
   dead-letter stream; transient failures go there after 10 delivery attempts.
4. **Real user:** a temporary database error delays Khalid's notification rather
   than dropping it. A restarted worker processes the pending event. Duplicate
   delivery creates only one database notification.
5. **Files:** `shared/notificationevents/event.go` defines/validates/publishes events;
   `outbox.go` durably queues and relays producer events;
   `internal/events/consumer.go` owns Redis transport and retry policy;
   `internal/service/service.go` maps event types to notification rules;
   `notifications_api.go` atomically records receipts and notifications.
6. **Other services:** post likes/comments and chat messages save an outbox event
   in the same SQL transaction as their domain change. A relay retries Redis
   publication. The notification service imports no other service's internal code.
7. **Without this:** crashes between database save and event publication could
   lose notifications; replay could create duplicates; poison events could retry forever.

Supported contracts:

| Domain event | Notification type | Click target |
|---|---|---|
| `post.liked` | `post_like` | post |
| `post.commented` | `post_comment` | post |
| `post.mentioned` | `post_mention` | post |
| `chat.message.created` | `chat_message` | conversation |
| `user.followed` | `user_follow` | user |
| `follow.requested` | `follow_request` | user |
| `system.announcement` | `system_announcement` | optional |

`eventType` is the canonical field (not the earlier example's `event`). Required:
nonzero UUID `eventId`, nonzero UUID `recipientId`, `eventType`, `createdAt`.
Social events also require nonzero `actorId`, `entityId`, and the target type above.
Metadata is a small string-to-string object. Whole payloads are limited to 8KiB.
Use one unique event ID per recipient; group-message producers generate one
notification event per other active member. Unknown event types are dead-lettered
so producer/consumer version mismatches are visible.

Idempotency uses `notification_processed_events(event_id PRIMARY KEY)` plus a
unique non-null notification event index. Receipts survive user deletion and
self-notification suppression. Concurrent attempts serialize on the receipt key.
If Redis publication fails after SQL commits, replay retrieves the same saved
notification and retries publication. Receipts must be retained for as long as
old domain events may be replayed.

ACK removes pending status, **not the stream entry**. No automatic MAXLEN trimming
is used, because trimming unprocessed entries loses events. Operators must monitor
stream/DLQ growth and choose safe retention after confirming processing. These
semantics follow [Redis XACK](https://redis.io/docs/latest/commands/xack/) and
[XAUTOCLAIM](https://redis.io/docs/latest/commands/xautoclaim/).

The outbox relay locks one row with `FOR UPDATE SKIP LOCKED`, publishes it, then
deletes it on transaction commit. If publication succeeds but commit fails, the
same event can be published again safely. Relays work with multiple replicas.
A failed outbox insert rolls back the like/comment/message too.

New producer endpoints in post-service:

- `POST /api/v1/posts/:id/likes` → 204; repeated likes create no duplicate event.
- `POST /api/v1/posts/:id/comments` with `{"content":"Great post!"}` → 201 `{id}`.

They use the existing `posts.create` permission and the public post visibility
policy. Existing private ownership helpers remain unchanged. Auth and future
follow services can adopt the same contract/outbox pattern; no artificial auth
notification trigger was added.

## Phase 4 — multiple replicas

1. **What:** a Redis Pub/Sub realtime bus, subscribed to by every replica.
2. **Why:** the worker processing an event may not own the recipient's socket.
3. **How:** after SQL persistence, publish an envelope to
   `notifications:realtime:v1`. Every replica checks its local hub and sends to
   matching connections. Redis Streams is the reliable work queue; Pub/Sub is
   only the temporary realtime announcement.
4. **Real user:** Khalid connects to replica 1; replica 3 processes Ahmed's like;
   Redis tells all replicas, and replica 1 delivers to Khalid.
5. **Files:** `internal/controller/websocket/redis_pubsub.go` subscribes/publishes;
   `hub.go` delivers locally; `cmd/server/main.go` wires the processor to the bus.
6. **Other services:** they only use the domain stream; they never publish directly
   to the notification realtime channel.
7. **Without this:** users connected to a different replica would miss live updates.

Sticky sessions are unnecessary. Each replica has a unique consumer identity in
one shared group. Pub/Sub reconnects via the existing go-redis client. Messages
missed during disconnection are recovered through HTTP history, not Pub/Sub replay.

## Phase 5 — Docker, Kong, Helm and ArgoCD

1. **What:** deployment wiring within the existing Compose file and Helm chart.
2. **Why:** the local and Kubernetes environments must run the same architecture.
3. **How:** the existing multi-stage Dockerfile builds a stripped binary and runs
   as non-root. Compose uses the existing Postgres/Redis networks and migration
   dependency. Helm provides two notification replicas, ConfigMap settings,
   existing Secret references, probes, resource requests/limits, and a ClusterIP
   service. `/health` remains live during dependency outages; `/ready` checks both.
4. **Real user:** Kong forwards HTTP and WebSocket upgrades to notification-service
   while keeping the service private on internal networks.
5. **Files:** `docker/notification-service.Dockerfile`, `docker/docker-compose.yml`,
   chart `values.yaml`, `_helpers.tpl`, `_kong.tpl`, and generated `docker/kong/kong.yml`.
6. **Other services:** post/chat images and the migration image must also be rebuilt
   because they now produce durable events and need outbox tables.
7. **Without this:** production could use stale code, miss migrations, restart
   healthy processes during database outages, or route sockets incorrectly.

SIGINT/SIGTERM switches the service to draining, rejects new work, cancels the
consumer, closes sockets, waits for workers and drains HTTP before closing Redis
and PostgreSQL. Failed/cancelled work stays pending. No resources were manually
applied to Kubernetes as part of implementation.

## Phase 6 — tests and client tools

1. **What:** unit, race, real PostgreSQL/Redis integration tests, Postman requests
   and this guide.
2. **Why:** authentication, ownership and retry guarantees need executable evidence.
3. **How:** `scripts/test-notifications.py` creates disposable containers with
   random loopback ports, applies all migrations, runs race tests, and removes
   those containers. It does not read project `.env` or use project database volumes.
4. **Real user:** tests exercise authenticated sockets, several devices, offline
   persistence, read-state changes, deleted-notification replay and failed delivery.
5. **Files:** new `*_test.go` files alongside service/config/HTTP/WebSocket/SQL/
   consumer code; shared outbox tests; post interaction tests and chat outbox checks;
   `postman/collections/notification-service.postman_collection.json`.
6. **Other services:** existing post/chat/shared tests run alongside the new tests;
   `make check` checks every service, including auth.
7. **Without this:** later changes could silently break recipient isolation,
   duplicate suppression, retry recovery or deployment configuration.

## Configuration

Keep existing `POSTGRES_*`, `REDIS_ADDR` (including password/TLS URL),
`JWT_PUBLIC_KEYS_FILE`, `JWT_ISSUER`, `ALLOWED_ORIGINS`, and `RATE_LIMIT_*` settings.
There is no new JWT secret: RS256 public-key verification is reused. The service
port remains `NOTIFICATION_SERVICE_PORT=8005`, because chat already uses 8004.

| Variable | Default | Purpose |
|---|---|---|
| `NOTIFICATION_STREAM` | `events:notifications` | same value on producers/consumer |
| `NOTIFICATION_CONSUMER_GROUP` | `notification-service` | shared across notification replicas |
| `NOTIFICATION_RETRY_IDLE` | `30s` | claim stale work; must be at least 15s |
| `NOTIFICATION_MAX_ATTEMPTS` | `10` | max deliveries before DLQ |
| `NOTIFICATION_PAGE_SIZE` | `30` | default history page length |
| `WS_PONG_TIMEOUT` | `60s` | new notification socket read deadline |
| `WS_SEND_QUEUE` | `64` | reused setting; bounded queue per connection |
| `WS_MAX_CONNECTIONS_PER_USER` | `8` | reused setting; cap per replica |
| `WS_PING_INTERVAL` | `25s` | reused setting; must be shorter than pong timeout |
| `WS_WRITE_TIMEOUT` | `5s` | reused setting; Helm currently sets 10s globally |

Defaults are validated at startup. The DLQ is `${NOTIFICATION_STREAM}:dead`.
There is no Prometheus integration in the existing backend, so this change adds
structured diagnostic logs without introducing a monitoring stack. Logs exclude
JWTs, passwords, raw event bodies and database details containing private data.

## Run locally

From the repository root, for a fresh checkout only:

```bash
python scripts/setup-security-dev.py
```

Preserve existing `.env` and `.secrets` if already configured. Then:

```bash
python scripts/render-kong-compose.py --env-file .env
docker compose --env-file .env -f docker/docker-compose.yml up -d --build
```

Compose runs the existing migration container before starting services. For an
existing stack, ensure the migration container is rerun on upgrade:

```bash
docker compose --env-file .env -f docker/docker-compose.yml build migrate post-service chat-service notification-service
docker compose --env-file .env -f docker/docker-compose.yml run --rm migrate all up
docker compose --env-file .env -f docker/docker-compose.yml up -d post-service chat-service notification-service kong-gateway
```

Use `http://localhost:8000` through Kong. Register/login with the existing auth
collection and use the returned `data.access_token`. No user-supplied signing key
or invented token format is needed. Migration SQL runs only through the migration
runner, never through server auto-migration.

## Test with Postman and WebSocket

Import the notification collection; set `baseUrl=http://localhost:8000`,
`accessToken` to Khalid's access token, and `notificationId` if known. Listing
notifications stores the first ID as a collection variable. Clear any environment
variable overriding that value. Test list, count, mark-read, read-all and delete.
Repeat a mutation with Ahmed's token to confirm 404 for Khalid's notification.

Create a **native WebSocket request** in Postman:

```text
ws://localhost:8000/api/v1/notifications/ws
Authorization: Bearer <Khalid access token>
```

The collection includes an HTTP upgrade reference, but Postman Collection Runner
does not test native WebSocket frames. Open two native connections using the same
token, publish an event below, and confirm both receive the same notification ID.
A request with only `?userId=...` must receive 401.

Browser example, from an allowed origin:

```javascript
const ws = new WebSocket(
  'ws://localhost:8000/api/v1/notifications/ws',
  ['notifications.v1', `bearer.${accessToken}`]
);
ws.onmessage = ({data}) => {
  const event = JSON.parse(data);
  if (event.type === 'connection.ready') {
    // Fetch first history page and unread count; merge by notification ID.
  }
  if (event.type === 'notification') {
    // Upsert event.data by id, then refresh unread count.
  }
};
// On close: refresh token as needed and reconnect with backoff + jitter.
```

Use HTTPS/WSS in production. Never put access tokens into query strings.

## Publish a Redis event manually

Replace the three example user/post UUIDs below with your test users/post. Set
`recipientId` to the UUID in Khalid's verified JWT subject. The actor must differ.
The notification service deliberately does not query other services' tables to
validate entity existence.

```bash
docker compose --env-file .env -f docker/docker-compose.yml exec redis \
  sh -c 'REDISCLI_AUTH="$REDIS_PASSWORD" exec redis-cli XADD events:notifications "*" payload "$1"' sh \
  '{"eventId":"11111111-1111-4111-8111-111111111111","eventType":"post.liked","recipientId":"22222222-2222-4222-8222-222222222222","actorId":"33333333-3333-4333-8333-333333333333","entityId":"44444444-4444-4444-8444-444444444444","entityType":"post","createdAt":"2026-09-17T12:00:00Z","metadata":{"actorName":"Ahmed"}}'
```

Repeat the exact payload: database history must contain one notification. Generate
a different `eventId` for a new event. Disconnect sockets, send another event,
then reconnect and fetch history to confirm offline persistence.
For the real producer flow, create a post as Khalid, like/comment on it as Ahmed
using the new post requests, or send a chat message using the existing chat API.

## Retry and dead-letter operations

Using Redis CLI with the existing Redis credentials:

```text
XPENDING events:notifications notification-service
XINFO GROUPS events:notifications
XRANGE events:notifications:dead - + COUNT 20
XLEN events:notifications
```

DLQ entries preserve `sourceId`, `payload`, and a safe error class. Investigate
invalid payloads or dependency failures. After fixing the cause, append the
original/corrected payload to the source stream with `XADD ... * payload ...`,
preserving its `eventId`. Verify processing before removing the DLQ entry with
`XDEL`. Publication retry may repeat a socket message but cannot create a second
notification. This workflow requires an operator; DLQ events do not auto-replay.
Do not delete pending source entries. Decommission empty old consumers only after
checking their pending count; do not destroy a live consumer group.

Redis is a trusted internal boundary. Do not expose stream publication publicly.
Use network isolation, credentials, backups and appropriate Redis ACLs. Streams
and Pub/Sub share the existing Redis deployment. The Lua DLQ operation targets
the current standalone Redis setup; Redis Cluster would need co-located hash-tagged
stream keys before adoption.

## Verify and deploy

```bash
make check
python scripts/test-notifications.py
node postman/validate-notifications.mjs
node postman/validate-post.mjs
python scripts/validate-deployment.py
python scripts/render-kong-compose.py --check
docker build -f docker/notification-service.Dockerfile -t social-notification:verification .
```

Integration coverage includes all eleven requested scenarios, plus concurrent
idempotency, deletion replay, self-event suppression, bad origins/expired/wrong-
audience tokens, slow clients, poison events, outbox rollback and relay failure.
Without test database/Redis environment variables, dependency tests explicitly
skip; the disposable-container script enables them. No live production deployment
or load test is implied by these checks.

ArgoCD deployment:

1. Build/push notification, post, chat and migration images with immutable tags.
2. Update their tags in the existing environment Helm values; preserve the existing
   PostgreSQL/Redis settings and pre-provisioned Secret references.
3. Commit code, migrations, chart changes and tags to the repository tracked by the
   existing ArgoCD Application. Use your normal review/merge flow.
4. Let ArgoCD sync (or sync that Application). Its existing migration Job runs at
   sync wave `-1`, before service Deployments at wave `0`.
5. Confirm migration success, both notification pods ready, correct image tags,
   Kong authentication, and two-device delivery. Inspect application logs and
   stream pending/DLQ state. Do not manually `kubectl apply` duplicate resources.

No ArgoCD Application rewrite is needed: it already tracks this Helm chart.
Do not roll migrations back while new binaries run. Notification rollback refuses
new notification types; producer rollback refuses an undrained outbox, protecting
data from silent deletion.

## Limits and future improvements

- Pub/Sub/WebSocket messages are best effort. Reconnect/history sync is required;
  a client connected during a brief Pub/Sub outage should refresh on focus or
  periodically to recover any missed updates.
- Redis AOF is already enabled. Default every-second fsync still has a crash-loss
  window. Production durability needs backups and an explicit fsync/HA policy;
  the outbox alone cannot make Redis power-loss durability absolute.
- Streams, dead letters and processed-event receipts currently have no automatic
  retention. Monitor their growth, Redis capacity, pending age and outbox backlog.
  Avoid eviction of durable stream keys.
- Database creation is idempotent; browser delivery is not exactly once.
- Per-user connection caps apply per replica. A global cap and device/session
  management could be added later.
- Post comment/client send request retries can create distinct domain actions;
  HTTP request idempotency keys are outside this notification event-id guarantee.
- No display-name lookup, preferences, mute settings, batching, email, Firebase,
  SMS, global announcements fan-out, or Kafka integration was implemented.
  Existing preference tables are preserved but new rules do not consult them.
- Future work can add those channels/settings, noisy-event rate limits, cursor
  pagination, retention automation, Prometheus metrics and immediate session
  revocation. Benchmark and size workers before claiming production throughput.

## Change inventory

### Files created

- `postman/collections/notification-service.postman_collection.json`
- `postman/validate-notifications.mjs`
- `scripts/test-notifications.py`
- `services/chat-service/migrations/000004_notification_outbox.down.sql`
- `services/chat-service/migrations/000004_notification_outbox.up.sql`
- `services/notification-service/internal/config/config_test.go`
- `services/notification-service/internal/controller/http/api_test.go`
- `services/notification-service/internal/controller/websocket/redis_pubsub.go`
- `services/notification-service/internal/controller/websocket/websocket_test.go`
- `services/notification-service/internal/database/postgres/notifications_api.go`
- `services/notification-service/internal/database/postgres/notifications_api_test.go`
- `services/notification-service/internal/events/consumer.go`
- `services/notification-service/internal/events/consumer_test.go`
- `services/notification-service/internal/models/notification.go`
- `services/notification-service/internal/service/service_test.go`
- `services/notification-service/migrations/000003_realtime_notifications.down.sql`
- `services/notification-service/migrations/000003_realtime_notifications.up.sql`
- `services/post-service/internal/database/postgres/interactions.go`
- `services/post-service/internal/database/postgres/interactions_test.go`
- `services/post-service/migrations/000004_notification_outbox.down.sql`
- `services/post-service/migrations/000004_notification_outbox.up.sql`
- `shared/notificationevents/event.go`
- `shared/notificationevents/outbox.go`
- `shared/notificationevents/outbox_test.go`
- `docs/NOTIFICATION_SERVICE.md`

### Files modified

- `.env.example`
- `README.md`
- `deployments/helm/social-media-backend/templates/_helpers.tpl`
- `deployments/helm/social-media-backend/templates/_kong.tpl`
- `deployments/helm/social-media-backend/values.yaml`
- `docker/docker-compose.yml`
- `docker/kong/kong.yml`
- `docker/notification-service.Dockerfile`
- `go.work.sum`
- `postman/README.md`
- `postman/collections/post-service.postman_collection.json`
- `scripts/validate-deployment.py`
- `services/chat-service/cmd/server/main.go`
- `services/chat-service/internal/database/postgres/chat.go`
- `services/chat-service/internal/database/postgres/chat_test.go`
- `services/notification-service/cmd/server/main.go`
- `services/notification-service/internal/config/config.go`
- `services/notification-service/internal/controller/http/controller.go`
- `services/notification-service/internal/controller/http/routes.go`
- `services/notification-service/internal/controller/websocket/client.go`
- `services/notification-service/internal/controller/websocket/controller.go`
- `services/notification-service/internal/controller/websocket/hub.go`
- `services/notification-service/internal/database/postgres/owned.go`
- `services/notification-service/internal/service/service.go`
- `services/post-service/cmd/server/main.go`
- `services/post-service/go.mod`
- `services/post-service/go.sum`
- `services/post-service/internal/controller/http/controller.go`
- `services/post-service/internal/controller/http/posts.go`
- `services/post-service/internal/controller/http/posts_test.go`
- `services/post-service/internal/controller/http/routes.go`
- `services/post-service/internal/service/service.go`
- `shared/ratelimit/middleware.go`

### Database migrations added

- Notification `000003_realtime_notifications`: additive entity/actor/event columns, supported types, event receipt table and ordering index; existing rows/ownership columns remain intact.
- Post `000004_notification_outbox`: durable outgoing post events and runtime grants.
- Chat `000004_notification_outbox`: durable outgoing chat events and runtime grants.
- Every migration includes a corresponding guarded down migration. Historical SQL files were not rewritten.

### Deployment and client changes

- Existing notification Docker image: stripped multi-stage build, non-root runtime on 8005.
- Existing Compose service: notification/WS settings, shared producer stream name, readiness check.
- Kong: explicit `/api/v1/notifications/ws` upgrade route; existing `/api/v1/notifications` API prefix retained.
- Helm: two replicas by default, notification readiness, ConfigMap keys, generated gateway configuration.
- Kubernetes: existing notification Deployment, Service, ConfigMap, Secret references and migration Job are reused; no duplicate standalone resources.
- ArgoCD: existing Application and migration sync ordering retained.
- Postman: seven notification entries (five APIs, WebSocket handshake reference, gateway health), plus post like/comment requests.
