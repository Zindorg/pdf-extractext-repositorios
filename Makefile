.PHONY: build run test vet lint compose-up compose-down logs compose-mongo compose-redis compose-app

build:
	go build -o bin/persister ./cmd/server

run:
	go run ./cmd/server

test:
	go test ./...

vet:
	go vet ./...

# Compose por componente (mired debe existir: docker network create mired)
compose-mongo:
	docker compose -f docker-compose.mongo.yml up -d

compose-redis:
	docker compose -f docker-compose.redis.yml up -d

compose-app:
	docker compose -f docker-compose.app.yml up -d --build

compose-up: compose-mongo compose-redis compose-app

compose-down:
	docker compose -f docker-compose.mongo.yml down
	docker compose -f docker-compose.redis.yml down
	docker compose -f docker-compose.app.yml down

logs:
	docker compose -f docker-compose.app.yml logs -f persister