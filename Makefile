
.DEFAULT_GOAL := docker-build

.PHONY: check docker-build docker-up docker-down docker-logs docker-ps docker-restart docker-clean

GO_PACKAGES := ./shared/... \
	./services/auth-service/... \
	./services/post-service/... \
	./services/chat-service/... \
	./services/notification-service/...

# Explicit module patterns are needed because the backend has no root go.mod.
check:
	go vet $(GO_PACKAGES)
	go test $(GO_PACKAGES)
	go build $(GO_PACKAGES)

docker-build:
	docker compose -f docker/docker-compose.yml build

docker-up:
	docker compose -f docker/docker-compose.yml up -d --build

docker-down:
	docker compose -f docker/docker-compose.yml down

docker-logs:
	docker compose -f docker/docker-compose.yml logs -f

docker-ps:
	docker compose -f docker/docker-compose.yml ps

docker-restart:
	docker compose -f docker/docker-compose.yml restart

# WARNING: removes local PostgreSQL and Redis volumes and all contained data.
docker-clean:
	docker compose -f docker/docker-compose.yml down -v

# The pinned golang-migrate library is built with Go; no external CLI is needed.
# Export values directly, never interpolate user input or credentials into recipes.
export name CONFIRM
MIGRATION_SERVICES := auth user post chat notification
.PHONY: migrate-tool migrate-up migrate-down migrate-status migrate-import-legacy \
 $(addprefix migration-,$(MIGRATION_SERVICES)) \
 $(foreach s,$(MIGRATION_SERVICES),migrate-$(s)-up migrate-$(s)-down migrate-$(s)-status migrate-$(s)-import-legacy)

migrate-tool:
	@GOWORK=off go build -C shared -o ../bin/migrate ./cmd/migrate

migrate-up migrate-down migrate-status migrate-import-legacy: migrate-tool
	@./bin/migrate all $(patsubst migrate-%,%,$@)

$(foreach s,$(MIGRATION_SERVICES),migrate-$(s)-up migrate-$(s)-down migrate-$(s)-status migrate-$(s)-import-legacy): migrate-tool
	@./bin/migrate $(word 2,$(subst -, ,$@)) $(subst migrate-$(word 2,$(subst -, ,$@))-,,$@)

$(addprefix migration-,$(MIGRATION_SERVICES)): migrate-tool
	@./bin/migrate $(patsubst migration-%,%,$@) create
