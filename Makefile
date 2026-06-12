.PHONY: dev build test up down seed

-include .env
export

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

expand: ## AI-generate recipes up to per-cuisine targets (pending_review)
	go run ./cmd/expand

expand-dry: ## Show the library deficit plan without API calls
	go run ./cmd/expand -dry

expand-review: ## List pending AI recipes for spot-check
	go run ./cmd/expand -review

expand-activate: ## Activate reviewed AI recipes
	go run ./cmd/expand -activate
