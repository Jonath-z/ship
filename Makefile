.PHONY: dev up down api worker web migrate seed generate test

dev: up
	@echo "Postgres + Redis running. Now run 'make api', 'make worker', 'make web' in separate shells."

up:
	docker compose -f infra/compose/docker-compose.dev.yml up -d

down:
	docker compose -f infra/compose/docker-compose.dev.yml down

api:
	cd server && go run ./cmd/api

worker:
	cd server && go run ./cmd/worker

web:
	pnpm --filter @ship/web dev

migrate:
	cd server && go run ./cmd/api -migrate-only

seed:
	bash scripts/seed.sh

generate:
	bash scripts/generate-client.sh

test:
	cd server && go test ./...
	pnpm -r test
