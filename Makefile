.PHONY: dev dev-up up down api worker web migrate migrate-down seed generate build lint test

SHIP_ENV_FILE := $(if $(wildcard .env),.env,.env.example)
DEV_COMPOSE := docker compose --env-file $(SHIP_ENV_FILE) -f infra/compose/docker-compose.dev.yml

dev:
	$(DEV_COMPOSE) up --build

dev-up:
	$(DEV_COMPOSE) up --build -d

up:
	$(DEV_COMPOSE) up postgres redis -d

down:
	$(DEV_COMPOSE) down

api:
	go run ./server/cmd/api

worker:
	go run ./server/cmd/worker

web:
	pnpm --filter @ship/web dev

migrate:
	go run ./server/cmd/api -migrate-only

migrate-down:
	go run ./server/cmd/api -migrate-down

seed:
	bash scripts/seed.sh

generate:
	bash scripts/generate-client.sh

build:
	go build ./...
	pnpm build

lint:
	test -z "$$(gofmt -l server)"
	go vet ./...
	pnpm lint

test:
	go test ./...
	pnpm test
