.PHONY: build run test test-integration vet lint compose-up compose-down logs compose-mongo compose-redis compose-app

build:
	go build -o bin/persister ./cmd/server

run:
	go run ./cmd/server

test:
	go test ./...

# Tests de integración: requieren MongoDB levantado y accesible.
test-integration:
	go test -tags=integration ./internal/adapters/mongodb/...

vet:
	go vet ./...

# Formato + estática con la toolchain estándar (no requiere herramientas externas)
lint:
	@fmt=$$(gofmt -l cmd internal); if [ -n "$$fmt" ]; then echo "sin formatear:"; echo "$$fmt"; exit 1; fi
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