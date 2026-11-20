.PHONY: build run test test-integration test-e2e vet vuln lint compose-up compose-down logs compose-mongo compose-redis compose-app compose-consul

build:
	go build -o bin/persister ./cmd/server

run:
	go run ./cmd/server

test:
	go test ./...

# Tests de integración (Mongo y Redis reales, incluye test/integration y e2e).
# FALLA si algún test se omite (t.Skip): sin Redis/Mongo arriba no hay verde.
test-integration:
	@log=$$(mktemp); \
	go test -tags=integration -count=1 -v ./... > $$log 2>&1; rc=$$?; \
	grep -E -- '^(--- (FAIL|SKIP)|FAIL|ok )' $$log; \
	if [ $$rc -ne 0 ]; then rm -f $$log; echo "ERROR: fallaron tests de integración"; exit 1; fi; \
	if grep -q -- '--- SKIP' $$log; then rm -f $$log; echo "ERROR: hay tests de integración omitidos (¿Redis y Mongo arriba? make compose-mongo compose-redis)"; exit 1; fi; \
	rm -f $$log

# Test e2e vertical: requiere Mongo + Redis (compose). Ejercita XADD → API HTTP.
test-e2e:
	go test -tags=integration ./test/e2e/...

vet:
	go vet ./...

# Vulnerabilidades en dependencias (requiere: go install golang.org/x/vuln/cmd/govulncheck@latest)
vuln:
	@command -v govulncheck >/dev/null || { echo "falta govulncheck: go install golang.org/x/vuln/cmd/govulncheck@latest"; exit 1; }
	govulncheck ./...

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

compose-consul:
	docker compose -f docker-compose.consul.yml up -d

compose-up: compose-mongo compose-redis compose-app

compose-down:
	docker compose -f docker-compose.mongo.yml down
	docker compose -f docker-compose.redis.yml down
	docker compose -f docker-compose.app.yml down
	docker compose -f docker-compose.consul.yml down

logs:
	docker compose -f docker-compose.app.yml logs -f persister