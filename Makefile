COMPOSE := docker compose -f deploy/compose.yaml

.PHONY: db-up db-down db-reset

db-up:
	$(COMPOSE) up -d --wait

db-down:
	$(COMPOSE) down

db-reset:
	$(COMPOSE) down -v

# On macOS with Colima, testcontainers needs the Colima socket. In CI
# DOCKER_HOST is already right, so these variables are only added when unset.
GO_TEST_ENV := $(if $(DOCKER_HOST),,DOCKER_HOST=unix://$(HOME)/.colima/default/docker.sock TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE=/var/run/docker.sock)

.PHONY: test-go lint-go

test-go:
	cd backend && $(GO_TEST_ENV) go test -race ./...

lint-go:
	cd backend && golangci-lint run ./...
