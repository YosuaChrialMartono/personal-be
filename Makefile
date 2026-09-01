.PHONY: generate test run migrate-up migrate-down docker-up

generate:
	go tool sqlc generate

test:
	go test ./...

run:
	go run ./cmd/api

migrate-up:
	go run ./cmd/migrate up

migrate-down:
	go run ./cmd/migrate down

docker-up:
	docker compose up --build
