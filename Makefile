.PHONY: help db-up db-down redis-up migrate region-import run-api run-worker location-sync location-sync-full location-sync-daemon test test-race fmt vet dashboard-install dashboard-dev dashboard-build compose-up

COMPOSE = docker compose --env-file .env -f deployments/compose/compose.yaml

help:
	@echo "make db-up             Start PostgreSQL only"
	@echo "make redis-up          Start optional Redis profile"
	@echo "make migrate           Apply database migrations"
	@echo "make region-import     Import pinned Kemendagri and postal datasets"
	@echo "make run-api           Run the API locally"
	@echo "make run-worker        Run the worker foundation"
	@echo "make location-sync SEARCH=Bandung  Import RajaOngkir locations on demand"
	@echo "make location-sync-full  Resume hierarchical sync through postal code"
	@echo "make location-sync-daemon  Run resumable sync as a background service"
	@echo "make test              Run Go tests"
	@echo "make dashboard-dev     Run the Vite dashboard"

db-up:
	$(COMPOSE) up -d postgres

db-down:
	$(COMPOSE) down

redis-up:
	$(COMPOSE) --profile redis up -d redis

migrate:
	go run ./apps/migrate

region-import:
	go run ./apps/region-import

run-api:
	go run ./apps/api

run-worker:
	go run ./apps/worker

location-sync:
	go run ./apps/location-sync -search "$(SEARCH)" -limit "$${LIMIT:-100}" -offset "$${OFFSET:-0}"

location-sync-full:
	go run ./apps/location-sync -mode full -max-hits "$${MAX_HITS:-500}" -concurrency "$${CONCURRENCY:-4}"

location-sync-daemon:
	$(COMPOSE) --profile legacy-provider-full-sync up -d --build location-sync

test:
	go test ./...

test-race:
	go test -race ./...

fmt:
	gofmt -w $$(find apps internal migrations -name '*.go' -type f)

vet:
	go vet ./...

dashboard-install:
	npm --prefix apps/dashboard install

dashboard-dev:
	npm --prefix apps/dashboard run dev

dashboard-build:
	npm --prefix apps/dashboard run build

compose-up:
	$(COMPOSE) up --build
