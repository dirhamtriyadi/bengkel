SHELL := /bin/sh
DATABASE_URL ?= postgres://bengkel:bengkel@localhost:5432/bengkel?sslmode=disable

.PHONY: help setup dev up down logs migrate migrate-down migrate-status seed reset docs test lint build frontend-install frontend-dev frontend-build

help:
	@echo "setup          Copy env, install dependencies, migrate, seed"
	@echo "dev            Run API (frontend: make frontend-dev)"
	@echo "up/down        Start/stop full Docker stack"
	@echo "migrate        Apply Goose migrations"
	@echo "seed           Run idempotent DatabaseSeeder"
	@echo "docs           Regenerate Swagger docs"
	@echo "test/lint      Validate backend and frontend"
	@echo "build          Build both applications"

setup:
	test -f .env || cp .env.example .env
	test -f frontend/.env.local || cp frontend/.env.example frontend/.env.local
	go mod download
	$(MAKE) frontend-install
	$(MAKE) migrate
	$(MAKE) seed

dev:
	go run ./cmd/api

up:
	docker compose up --build

down:
	docker compose down

logs:
	docker compose logs -f --tail=200

migrate:
	go run github.com/pressly/goose/v3/cmd/goose -dir migrations postgres "$(DATABASE_URL)" up

migrate-down:
	go run github.com/pressly/goose/v3/cmd/goose -dir migrations postgres "$(DATABASE_URL)" down

migrate-status:
	go run github.com/pressly/goose/v3/cmd/goose -dir migrations postgres "$(DATABASE_URL)" status

seed:
	go run ./cmd/seed

reset:
	go run github.com/pressly/goose/v3/cmd/goose -dir migrations postgres "$(DATABASE_URL)" reset
	$(MAKE) seed

docs:
	go run github.com/swaggo/swag/cmd/swag@v1.16.4 init -g ./cmd/api/main.go -o ./docs --parseDependency --parseInternal

frontend-install:
	cd frontend && npm ci

frontend-dev:
	cd frontend && npm run dev

test:
	go test -race -cover ./...
	cd frontend && npm run typecheck

lint:
	go vet ./...
	cd frontend && npm run lint

build:
	CGO_ENABLED=0 go build -o bin/api ./cmd/api
	cd frontend && npm run build
