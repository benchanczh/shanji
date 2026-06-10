.PHONY: dev build test up down

up: ## Start PostgreSQL (port 5433)
	docker compose up -d db

down:
	docker compose down

dev: up ## Run the API server (auto-migrates on boot, port 8090)
	go run ./cmd/server

build:
	go build -o bin/server ./cmd/server

test:
	go test ./...

seed: ## Import recipe library from seed/seed.json (idempotent)
	go run ./cmd/seed
