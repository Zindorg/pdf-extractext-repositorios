.PHONY: build run test test-integration test-e2e vet vuln lint check-version image push release deploy compose-up compose-down logs compose-mongo compose-redis compose-app compose-consul admin-migrate admin-reindex admin-dlq admin-dlq-requeue admin-restore

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


vuln:
	@command -v govulncheck >/dev/null || { echo "falta govulncheck: go install golang.org/x/vuln/cmd/govulncheck@latest"; exit 1; }
	govulncheck ./...

# Formato + estática con la toolchain estándar (no requiere herramientas externas)
lint:
	@fmt=$$(gofmt -l cmd internal); if [ -n "$$fmt" ]; then echo "sin formatear:"; echo "$$fmt"; exit 1; fi
	go vet ./...


COMPOSE_APP = docker compose -f docker-compose.app.yml
COMPOSE_ENV = $(if $(VERSIONREP),VERSIONREP=$(VERSIONREP)) $(if $(REGISTRY),REGISTRY=$(REGISTRY))
IMAGE = $(REGISTRY)pdf-extractext-repositorios:v$(VERSIONREP)

check-version:
	@test -n "$(VERSIONREP)" || { echo "ERROR: definir VERSIONREP (ej: make image VERSIONREP=1.0.2)"; exit 1; }

# BUILD: construye la imagen versionada (no ejecuta nada)
image: check-version
	$(COMPOSE_ENV) $(COMPOSE_APP) build

# RELEASE: publica el artefacto en el registry (no reutilizar tags: cada versión es nueva)
push: check-version
	@test -n "$(REGISTRY)" || { echo "ERROR: definir REGISTRY (ej: registry.tuorg/) para publicar"; exit 1; }
	docker push $(IMAGE)

release: image push


deploy: check-version
	@if [ -n "$(REGISTRY)" ]; then $(COMPOSE_ENV) $(COMPOSE_APP) pull persister; fi
	$(COMPOSE_ENV) $(COMPOSE_APP) up -d --no-build


ADMIN = $(COMPOSE_ENV) $(COMPOSE_APP) run --rm --no-deps -l traefik.enable=false persister

# Crea/asegura los índices de Mongo (idempotente).
admin-migrate: check-version
	$(ADMIN) migrate

# Borra y recrea los índices. CUIDADO: mientras tanto no hay índices únicos; detener antes el consumer
# (PERSISTER_REPLICAS=0 make deploy ...) y volver a levantarlo después.
admin-reindex: check-version
	$(ADMIN) reindex

# Lista los mensajes de la DLQ (no modifica nada).
admin-dlq: check-version
	$(ADMIN) cleanup-dlq

# Reencola los mensajes de la DLQ al stream principal y los borra de la DLQ.
admin-dlq-requeue: check-version
	$(ADMIN) cleanup-dlq --requeue

# Restaura un documento borrado lógicamente: make admin-restore CHECKSUM=<checksum> VERSIONREP=<versión>
admin-restore: check-version
	@test -n "$(CHECKSUM)" || { echo "ERROR: definir CHECKSUM (ej: make admin-restore CHECKSUM=abc123 VERSIONREP=1.1.1)"; exit 1; }
	$(ADMIN) restore-by-checksum $(CHECKSUM)

# Compose por componente (mired debe existir: docker network create mired)
compose-mongo:
	docker compose -f docker-compose.mongo.yml up -d

compose-redis:
	docker compose -f docker-compose.redis.yml up -d

compose-app: deploy

compose-consul:
	docker compose -f docker-compose.consul.yml up -d

# Desarrollo local: construye (image) y luego ejecuta (compose-app) como pasos separados.
compose-up: compose-mongo compose-redis image compose-app

# down y logs no dependen de la versión; el valor "0" solo satisface la interpolación del compose.
compose-down:
	docker compose -f docker-compose.mongo.yml down
	docker compose -f docker-compose.redis.yml down
	VERSIONREP=$(or $(VERSIONREP),0) $(COMPOSE_APP) down
	docker compose -f docker-compose.consul.yml down

logs:
	VERSIONREP=$(or $(VERSIONREP),0) $(COMPOSE_APP) logs -f persister