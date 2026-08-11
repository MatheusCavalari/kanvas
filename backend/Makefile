include .env
export

MIGRATE := go run -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@v4.18.1
SQLC := go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.27.0

.PHONY: migrate-up migrate-down sqlc-generate test test-integration lint run

migrate-up:
	$(MIGRATE) -database "$(DATABASE_URL)" -path db/migrations up

migrate-down:
	$(MIGRATE) -database "$(DATABASE_URL)" -path db/migrations down 1

sqlc-generate:
	$(SQLC) generate

test:
	go test ./... -race

test-integration:
	go test ./... -race -tags=integration

lint:
	go run github.com/golangci/golangci-lint/cmd/golangci-lint@latest run

run:
	go run ./cmd/api
