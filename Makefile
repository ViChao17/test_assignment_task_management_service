.PHONY: build run test test-integration vet up down logs

build:
	@mkdir -p bin
	go build -o bin/taskmanager ./cmd/api

run:
	go run ./cmd/api

test:
	go test ./...

test-integration:
	go test -v ./integration -run TestTeamStatsSQL

vet:
	go vet ./...

up:
	docker compose up --build -d

down:
	docker compose down

logs:
	docker compose logs -f app
