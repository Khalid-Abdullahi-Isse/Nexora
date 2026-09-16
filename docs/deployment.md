# Deploying Social Media Backend

## Architecture and repository boundaries

```text
Git repository → CI tests/builds → container registry
       ↑ immutable image tags committed to Helm values
       └→ ArgoCD → Helm rendering → Kubernetes
                                  ├ PostgreSQL (one social_media database)
                                  ├ Redis (authenticated, append-only storage)
                                  ├ migration Job
                                  └ auth / post / chat / notification
```

Docker Compose remains the local application development workflow. Helm is the
only Kubernetes manifest source; ArgoCD renders the chart and reconciles Git.
ArgoCD does not execute `helm install` or build images. No CI workflow existed, so
this change does not introduce one.

The chart is `deployments/helm/social-media-backend/`; Applications and the
AppProject are in `deployments/argocd/`. Fixed Service names preserve application
DNS. Use one release per namespace. There are four Deployments, two StatefulSets,
six Services, two PVCs, one ServiceAccount, configuration, and a migration Job.

All services use **one database, `social_media`**, with `app_auth`, `app_post`,
`app_chat`, and `app_notification` roles. PostgreSQL initialization creates those
roles; SQL migrations grant their privileges. PostgreSQL and Redis have no public
Service, ingress route, hostPort, or host port mapping.

## Audit and cleanup

| Path | Decision | Reason |
| --- | --- | --- |
| `kubernetes/` | Replaced with Helm, removed | All workloads, configuration, storage and guidance replaced |
| `deployments/kubernetes/` | Removed | Empty experimental scaffold |
| `docker/user-service.Dockerfile` | Removed | Retired runtime source already deleted; no active references |
| Five active Dockerfiles and Compose | Retained | Local development and CI image builds |
| `docker/migrate.Dockerfile` | Updated | Packages original SQL in the image |
| `scripts/kind-deploy.sh`, `kind-status.sh` | Updated | Helm deployment and new namespace |
| All SQL, Go source, existing security scripts | Retained | Application and development dependencies |

There were no other Helm charts, ingress duplicates, or CI deployment references.
The working tree already contained extensive unrelated changes; they were preserved.
Tracked-file signature checks found no private keys, common token signatures, or
actual tracked `.env`/key files. This is not a full historical secrets audit.

Removing repository YAML does **not** delete existing cluster resources. The old
`social-media` namespace and all its data are retained. New Helm development uses
`social-media-dev` and a fresh, separate database. Do not use `--take-ownership`,
apply new selectors over the old StatefulSet, or delete old PVCs to migrate it.
Moving existing data requires a reviewed backup/restore or resource adoption plan.

## Docker Compose

From the repository root, use the existing setup:

```bash
# Only when .env does not yet exist:
python3 scripts/setup-security-dev.py
# Preserves the existing host port mappings and bind-mounted migrations:
docker compose --env-file .env -f docker/docker-compose.yml up --build -d
```

The Compose file and application Dockerfiles were not rewritten. The migration
Dockerfile now copies the legacy-user and four active SQL directories. Compose's
bind mounts continue to override those paths for local editing. Never edit applied
SQL history; add a new sequential migration instead.

## Local kind and Helm

Prerequisites: Docker, kind, kubectl, Helm 3 or 4, Python 3, OpenSSL. Run:

```bash
./scripts/kind-deploy.sh
./scripts/kind-status.sh
```

The helper checks tools/Docker, reuses `social-media`, starts stopped kind nodes,
builds the existing Dockerfiles, loads all five `social-*:latest` images, creates
local Secrets, and installs Helm into `social-media-dev`. It generates credentials
once under ignored `.secrets/kind/` and reuses them; it never overwrites Compose
credentials or the old namespace. It changes the kubeconfig's selected context to
`kind-social-media`, and all operations explicitly target that context.

Manual image load and install, **after provisioning Secrets**:

```bash
# Only if the cluster does not already exist:
kind create cluster --name social-media

for service in auth post chat notification migrate; do
  file="docker/$service-service.Dockerfile"
  [ "$service" != migrate ] || file=docker/migrate.Dockerfile
  docker build -f "$file" -t "social-$service:latest" .
  kind load docker-image "social-$service:latest" --name social-media
done

helm upgrade --install social-media deployments/helm/social-media-backend \
  -f deployments/helm/social-media-backend/values-dev.yaml \
  --kube-context kind-social-media --namespace social-media-dev \
  --create-namespace --wait --timeout 10m
```

`values-dev.yaml` uses `imagePullPolicy: Never`; every node must have those images.
For registry-backed development, override the image repositories/tags and set
`global.imagePullPolicy=IfNotPresent`. For rebuilt `latest` images, the helper sets
`rolloutVersion` to trigger replacement Pods. With manual Helm, set that value or
restart the four Deployments after loading images. Immutable tags naturally change
the Pod specification. Do not use timestamp values in ArgoCD templates.

## Values and environments

| File | Purpose |
| --- | --- |
| `values.yaml` | Shared defaults, ports, configuration, Secret references, probes, resources |
| `values-dev.yaml` | Loaded kind images, small PVCs, kind `standard` StorageClass |
| `values-staging.yaml` | Registry placeholders, production application checks, TLS, HTTPS |
| `values-production.yaml` | Same security requirements; auth/post have two replicas |

The Go loader supports `development`, `test`, and `production`, not `staging`;
staging therefore runs with `APP_ENV=production`. Chat and notification default to one replica. Chat now supports Redis-backed
cross-Pod fanout; see [chat service](CHAT_SERVICE.md). Notification remains a scaffold.

Per-service resources merge with `resources.defaults`; PostgreSQL, Redis and the
migration Job have separate requests/limits. `postgres.storage` and `redis.storage`
allow a StorageClass, size, or `existingClaim`. Empty StorageClass uses the cluster
default. PVCs are retained on Helm uninstall and excluded from ArgoCD prune/delete.
This is protection against accidental cleanup, **not a backup**. Deleting a
namespace, PVC, or kind cluster can still destroy data. Keep credentials and take
backups; do not change passwords in a Secret and expect existing roles to rotate.

`postgres.enabled=false` / `redis.enabled=false` support externally provisioned
backends with configurable hosts. Keep default internal names when using bundled
StatefulSets. External PostgreSQL still requires the same database and roles.
The bundled single-instance databases do not provide HA, automated backup,
monitoring, or disaster recovery; arrange those before a production launch.

## Secrets and JWT keys

Default credential Secret: `social-media-backend-secrets`.
Required keys:

```text
POSTGRES_ADMIN_PASSWORD
AUTH_DATABASE_PASSWORD
POST_DATABASE_PASSWORD
CHAT_DATABASE_PASSWORD
NOTIFICATION_DATABASE_PASSWORD
REDIS_PASSWORD
REDIS_ADDR
```

Each Deployment maps its corresponding password key to `POSTGRES_PASSWORD`.
For development, `REDIS_ADDR` is `redis://:<URL-encoded-password>@redis:6379/0`;
for staging/production use `rediss://:<URL-encoded-password>@redis:6379/0`.
Production database and Redis passwords must satisfy the application's minimum
24-character requirement. Do not place them in committed values or command history.

After the helper has generated local files, this illustrates provisioning:

```bash
kubectl --context kind-social-media create namespace social-media-dev --dry-run=client -o yaml | kubectl --context kind-social-media apply -f -
kubectl --context kind-social-media -n social-media-dev create secret generic social-media-backend-secrets \
  --from-env-file=.secrets/kind/credentials.env --dry-run=client -o yaml | kubectl --context kind-social-media apply -f -
kubectl --context kind-social-media -n social-media-dev create secret generic jwt-keys \
  --from-file=.secrets/kind/public-keys.json --from-file=.secrets/kind/private.pem \
  --dry-run=client -o yaml | kubectl --context kind-social-media apply -f -
```

Auth mounts both JWT files read-only at `/keys/public-keys.json` and
`/keys/private.pem`. Other services project **only** the public file. JSON key IDs
must match `JWT_KEY_ID`; the helper uses `development-1`. The private key never
enters a ConfigMap. Secret file modes and Pod groups permit the nonroot UID to read
them. Pods have no Kubernetes API token, so they cannot fetch the remaining Secret
keys through the API.

In each remote environment, provision the same Secret names before syncing, either
manually, through External Secrets Operator, or through Sealed Secrets. Those
controllers must be installed separately; this chart consumes their output and
does not create broad controller permissions. Provider credentials and encrypted
Secret manifests belong in a separately managed bootstrap application. Configure
registry pull Secrets through `global.imagePullSecrets`.

Optional development-only inline credentials: set `secrets.create=true` and fill
`secrets.developmentData` in an ignored `*.local.yaml`. Helm stores supplied values
in release metadata; prefer existing Secrets. Production inline credentials are
rejected. Existing Secret updates do not automatically restart Pods: coordinate
credential rotation with database role updates and roll out applications. A
nonsecret `rolloutVersion` change in Git can trigger the application rollout.

## TLS and ingress

Production checks require verified PostgreSQL TLS, Redis TLS, secure cookies and
explicit HTTP TLS termination. The chart supports these without changing Go code.
Provide these Secrets in the target namespace:

| Secret | Keys / requirements |
| --- | --- |
| `postgres-tls` | `tls.crt`, `tls.key`, `ca.crt`; server certificate SAN includes `postgres` |
| `redis-tls` | `tls.crt`, `tls.key`, `ca.crt`; server certificate SAN includes `redis` |
| `social-media-ingress-tls` | Standard TLS Secret for the configured API hostname |

Names are configurable. Applications receive only the CA file from backend TLS
Secrets. PostgreSQL clients use `PGSSLROOTCERT` and `verify-full`. Go Redis uses
`SSL_CERT_FILE` to trust the configured CA; Redis serves TLS only on 6379 when
enabled. PostgreSQL enables TLS on 5432; its clients require TLS, although server
`pg_hba.conf` is not customized to reject every possible plaintext client. Backend
certificates may be issued by your PKI/cert-manager; real credentials and certificates
are not supplied in Git. Rotate certificates and restart the affected StatefulSets
and applications because the processes load TLS material at startup.

Ingress is disabled in base/dev values. Staging/production examples enable HTTPS
but require real hostnames, TLS Secrets, and an installed ingress controller.
`className: nginx` is a configurable example, not an installed controller.
Routes preserve `/api/v1/auth`, `/api/v1/posts`, `/api/v1/chats`, and
`/api/v1/notifications`; there is no rewrite and no database/cache route.
`/health` stays internal. Chat registers `/api/v1/chats/ws`, already covered by the Kong chat prefix.
Notification still has no registered WebSocket route.

When WebSocket handlers are implemented, add their actual paths through
`ingress.routes`. Choose a controller supporting HTTP Upgrade and configure its
idle/read timeouts through `ingress.annotations` (controller-specific). No chart
rewrite rule strips WebSocket paths. Configure `TRUSTED_PROXIES` with actual proxy
CIDRs and `ALLOWED_ORIGINS` with actual frontend origins, never a trust-all network.

## Migrations and ordering

The image contains all five migration histories, including retired-user SQL needed
by auth's import. The existing Go runner uses a PostgreSQL advisory lock and
migration version tables, so repeated runs skip applied migrations and concurrent
runs serialize. Failed or dirty migrations block deployment; inspect and repair
with the existing migration guide, never blindly force a version or drop data.

**ArgoCD:** use a full sync. Sync waves are:

1. `-3`: ServiceAccount, ConfigMaps, Services and PVCs; external Secrets already exist.
2. `-2`: PostgreSQL and Redis, waiting for healthy StatefulSets.
3. `-1`: migration `Sync` hook, waiting for PostgreSQL and running `all up`.
4. `0`: application Deployments and ingress.

A `PreSync` hook would run before this chart's new database/configuration exists,
so it is deliberately not used. The Job uses `BeforeHookCreation,HookSucceeded`;
failed Jobs remain inspectable. Hooks do not run during selective resource syncs:
use full application syncs when changing images or migrations. Existing application
Pods continue serving during upgrades, so migrations must be backward compatible.

**Direct Helm:** post-install/post-upgrade hooks wait for PostgreSQL, run once per
operation, and leave the completed Job for logs. The next operation replaces it.
Use `--wait --timeout 10m`. Helm does not implement ArgoCD waves: applications can
start before the migration hook, and health only checks dependency connectivity.
Direct Helm is therefore the local workflow; do not direct public traffic to a
fresh Helm install until the command succeeds. For production rollout ordering,
use ArgoCD. A failed hook fails the Helm operation; do not treat ready Pods alone
as migration success.

The migration CLI uses `APP_ENV=test` solely to bypass the shared loader's ban on
an administrator runtime identity. It still receives configured TLS/CA settings;
application Pods retain production validation and least-privilege identities.

## ArgoCD and image updates

Install the pinned local controller with `./scripts/argocd-bootstrap.sh`.
The project and Applications now use the verified Git remote
`https://github.com/Khalid-Abdullahi-Isse/Nexora.git`, branch `main`.
The bootstrap script installs ArgoCD v3.5.2 and the AppProject, but does not apply
Applications before the chart exists remotely. Publish the reviewed deployment
files first. Configure repository credentials in ArgoCD separately if needed.

For local development, load images into the destination kind cluster first. Remote
development must override `Never` and local repositories. For staging/production,
replace registry repositories, `sha-REPLACE_WITH_GIT_COMMIT`, frontend/API hosts,
TLS references, and StorageClass as needed. Provision Secrets first. Then:

```bash
kubectl --context kind-social-media apply -f deployments/argocd/project.yaml
kubectl --context kind-social-media apply -f deployments/argocd/application-dev.yaml
# For each separately prepared environment:
# kubectl --context YOUR_CONTEXT apply -f deployments/argocd/application-staging.yaml
# kubectl --context YOUR_CONTEXT apply -f deployments/argocd/application-production.yaml
```

The project restricts Git to that repository, destinations to the three application
namespaces, and resource kinds to the chart's needs. Namespace creation is the only
allowed cluster-scoped kind. Applications enable prune/self-heal; they intentionally
have no cascading-deletion finalizer. Do not have Helm CLI and ArgoCD manage the
same release simultaneously. To move the tested dev release to ArgoCD, retain the
same namespace, release name and values, review the first diff, then stop using the
Helm helper for that namespace. An alternate environment can be used first.

Future CI should test, build all five existing Dockerfiles, push immutable
`sha-<git-commit>` tags, update the corresponding environment values, and commit
that change. ArgoCD detects **Git changes**, not a new image behind an unchanged
tag. Do not commit secrets or create a competing kubectl deployment workflow.
Use immutable registry tag policies or signed release tags; tag syntax alone does
not prevent a registry tag from being overwritten. Production `latest` is rejected.
Chart initialization script mirrors `docker/postgres-init.sh`; the validation
script requires them to match if initialization changes.

## Validation, verification and rollback

```bash
helm lint deployments/helm/social-media-backend
helm template social-media deployments/helm/social-media-backend
# Requires Python 3 and PyYAML in addition to Helm:
python3 scripts/validate-deployment.py
bash -n scripts/kind-deploy.sh scripts/kind-status.sh

./scripts/kind-status.sh
kubectl --context kind-social-media get pods,svc,statefulsets,jobs,pvc -n social-media-dev
kubectl --context kind-social-media logs job/social-media-backend-migrate -c migrate -n social-media-dev
kubectl --context kind-social-media port-forward svc/auth-service 8001:8001 -n social-media-dev
# In another terminal:
curl --fail http://localhost:8001/health
```

Post/chat/notification use ports 8003/8004/8005 respectively. Use another host port
if Compose already occupies it. For in-cluster routing and all health endpoints:

```bash
kubectl --context kind-social-media exec -n social-media-dev deployment/auth-service -c auth -- sh -ec '
for endpoint in auth-service:8001 post-service:8003 chat-service:8004 notification-service:8005; do
  wget -q -O - "http://$endpoint/health"; echo
done'
```

Helm-only rollback (first inspect history and database compatibility):

```bash
helm history social-media --kube-context kind-social-media -n social-media-dev
helm rollback social-media REVISION --kube-context kind-social-media -n social-media-dev --wait --timeout 10m
```

For GitOps, revert the image/config commit in Git and let ArgoCD synchronize. Do
not issue Helm rollback against an ArgoCD-managed release. Neither workflow rolls
back SQL automatically: auth includes an intentionally irreversible migration.
Use backward-compatible expand/contract changes and take backups before risky SQL.

## Troubleshooting

```bash
kubectl --context kind-social-media describe pod <pod-name> -n social-media-dev
kubectl --context kind-social-media logs <pod-name> -n social-media-dev -c <container-name>
kubectl --context kind-social-media logs <pod-name> -n social-media-dev -c <container-name> --previous
kubectl --context kind-social-media logs <pod-name> -n social-media-dev -c wait-dependencies
kubectl --context kind-social-media describe job social-media-backend-migrate -n social-media-dev
kubectl --context kind-social-media get events -n social-media-dev --sort-by=.metadata.creationTimestamp
kubectl --context kind-social-media describe pvc postgres-data redis-data -n social-media-dev
```

`ErrImageNeverPull`: load images on every kind node. `Pending` PVCs: verify the
StorageClass/provisioner. Authentication failure: restore matching credentials;
initialization scripts only run on an empty data directory. TLS failure: inspect
certificate SANs, trust roots, expiry and `rediss://`. Failed migration: read the
Job's `migrate` logs and `wait-postgres` init logs. Long dependency outages can
trigger application liveness restarts because `/health` checks both dependencies.

References: [ArgoCD sync waves](https://argo-cd.readthedocs.io/en/latest/user-guide/sync-waves/)
and [Helm chart hooks](https://helm.sh/docs/topics/charts_hooks/).

## Verification performed for this refactor

- Helm lint and semantic render validation passed for base, dev, staging and
  production values in both Helm and ArgoCD modes.
- Kubernetes server-side dry-run accepted the development resources.
- Fresh Helm installation and repeated upgrades succeeded in `social-media-dev`;
  rerun migrations reported `no change` for all five histories.
- All four Service DNS health checks returned database/Redis connected.
- Production application mode was exercised locally with temporary certificates:
  PostgreSQL reported SSL connections for all four runtime roles, Redis TLS worked,
  and the migration hook completed with verified PostgreSQL TLS. Development values
  were restored afterward. This did not validate public ingress or remote PKI.
- Existing Compose configuration validation and shell syntax checks passed.
- Secret checks found no known local passwords or private-key blocks in tracked or
  nonignored project files; Git history was not exhaustively scanned.
- Original `social-media` Pods and PVCs remain intact. No persistent data was deleted.
- ArgoCD paths, values and hook annotations were validated by rendering. ArgoCD v3.5.2 was subsequently installed and its HTTPS health endpoint verified.
  A controller sync was not run: the chart must first be published to Git; remote
  registry/TLS settings still need configuring.

## Local ArgoCD access

The bootstrap requires kubectl, curl and Python 3. It uses cached pinned images
(`IfNotPresent`) and allows ten minutes for initial rollouts.
The controller uses the official pinned non-HA installation, with cluster-wide
controller permissions; the Social Media AppProject limits what this application
may deploy. It is exposed only through a ClusterIP Service. Open it locally:

```bash
kubectl --context kind-social-media port-forward -n argocd svc/argocd-server 8080:443
```

Visit `https://localhost:8080` (the default certificate is self-signed), user
`admin`. Retrieve the generated password in your own terminal, without committing
or sharing it:

```bash
kubectl --context kind-social-media get secret argocd-initial-admin-secret \
  -n argocd -o jsonpath='{.data.password}' | base64 --decode; echo
```

Change the initial admin password after login. Never expose this local controller
publicly without configuring authentication and trusted TLS. The remote `main`
branch must contain `deployments/helm/social-media-backend/Chart.yaml` before the
dev Application can render and synchronize. Local working-tree changes alone are
invisible to ArgoCD.


## Chat rollout performed on 2026-09-16

Local context `kind-social-media`, namespace `social-media-dev`, Helm release
`social-media` revision **6** now runs `social-chat:chat-v1-20260916`.
The migration image is `social-migrate:chat-v1-20260916`.

For this existing release, the Helm migration template supports optional
`migration.helmHook: post-install,pre-upgrade` and `migration.jobName` overrides.
The rollout used `social-media-backend-migrate-chat-v1-20260916`, retaining the
previous migration job and running migration 000003 before updating chat pods.
Default hook behavior and ArgoCD hook ordering remain unchanged. Use a new unique
job name for another rollout when retaining prior job resources is required.

Existing Helm values were reused; only chat/migration image tags and migration
hook settings were overridden. Other service pods were not restarted. A database
backup was saved locally with mode 0600 before migration. Migration version is 3,
dirty=false. Live verification through Kong used real auth-service registration
and login, then conversation creation, authenticated WebSocket messaging, a read
receipt and REST history. All four gateway health endpoints passed. Two unique
verification accounts and their test conversation were retained. No existing data
or previous migration job was deleted.

This was a local Helm rollout. No Git push or ArgoCD synchronization was performed.
