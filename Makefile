
.DEFAULT_GOAL := docker-build

.PHONY: check docker-build docker-up docker-down docker-logs docker-ps docker-restart docker-clean

GO_PACKAGES := ./shared/... \
	./services/auth-service/... \
	./services/user-service/... \
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
