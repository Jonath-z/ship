.PHONY: dev dev-up up down api worker web migrate migrate-down seed generate build lint test

dev:
	docker compose -f infra/compose/docker-compose.dev.yml up --build

dev-up:
	docker compose -f infra/compose/docker-compose.dev.yml up --build -d

up:
	docker compose -f infra/compose/docker-compose.dev.yml up postgres redis -d

down:
	docker compose -f infra/compose/docker-compose.dev.yml down

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
