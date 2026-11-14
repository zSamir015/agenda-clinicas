COMPOSE := docker compose -f deploy/compose.yaml

.PHONY: db-up db-down db-reset

db-up:
	$(COMPOSE) up -d --wait

db-down:
	$(COMPOSE) down

db-reset:
	$(COMPOSE) down -v
