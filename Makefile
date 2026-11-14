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

.PHONY: test-go test-crypto lint-go

test-go:
	cd backend && $(GO_TEST_ENV) go test -race ./...

# Needs libmongocrypt: brew install mongodb/brew/libmongocrypt pkgconf cmake
test-crypto:
	cd backend && $(GO_TEST_ENV) go test -race -tags cse ./internal/crypto/...

lint-go:
	cd backend && golangci-lint run ./...

.PHONY: gen gen-go api

gen: gen-go

gen-go:
	cd backend && go tool oapi-codegen -config oapi-codegen.yaml ../openapi/openapi.yaml

api:
	set -a; [ -f .env ] && . ./.env; set +a; cd backend && go run ./cmd/api
