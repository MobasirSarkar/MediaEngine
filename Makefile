ifneq (,$(wildcard .env))
    include .env
    export
endif

.PHONY: tidy lint test migrate sqlc run-api run-worker compose-up compose-down smoke fmt infra

tidy:
	go mod tidy

fmt:
	gofmt -w .

lint:
	golangci-lint run ./...

test:
	go test ./... -count=1

migrate:
	migrate -path db/migrations -database "$(DB_DSN)" up

sqlc:
	sqlc generate

run-api:
	go run ./cmd/api

run-worker:
	go run ./cmd/worker

compose-up:
	docker compose -f docker/compose.yml up -d --build

compose-down:
	docker compose -f docker/compose.yml down

infra:
	docker compose -f docker/compose.yml up -d postgres nats minio

smoke:
	python.exe scripts/smoke.py
