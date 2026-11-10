# pdf-extractext-repositorios

Microservicio **persister** (Go) del sistema Hub & Spoke de extracción de texto de PDFs. Es la fuente de verdad de los documentos: consume el stream `document-events` desde Redis, persiste en MongoDB y expone la API interna de lectura/borrado. Arquitectura completa en [ARCHITECTURE.md](ARCHITECTURE.md) y diagramas en [docs/diagrams](docs/diagrams).

## Estructura

```
cmd/server/          composition root (conexiones, índices, servidor HTTP, graceful shutdown)
internal/config/     configuración por variables de entorno
internal/domain/     entidades, interfaz de repositorio y errores de dominio
internal/application/ casos de uso (DocumentService)
internal/adapters/mongodb/ persistencia BSON + creación de índices
internal/adapters/redis/  consumer de stream + DLQ (Redis Streams, consumer group)
internal/adapters/cache/  read-through cache Redis (Bloque D)
internal/adapters/health/ ping de Mongo y Redis para /health
internal/api/        router Gin, handlers, DTOs y health check
internal/testutil/   fixtures, seeders y helpers HTTP compartidos por los tests
```

## Requisitos

- Go 1.26+
- Docker (para Mongo/Redis) o instancias locales
- Red Docker `mired` ya creada: `docker network create mired`
  (la red `db-net` la crea `docker-compose.mongo.yml` al levantar)

## Correr localmente

```sh
# dependencias: Mongo 4.4 + Redis 7
docker compose -f docker-compose.mongo.yml up -d
docker compose -f docker-compose.redis.yml up -d

# tests
make test
make test-e2e        # e2e vertical (requiere Mongo + Redis del compose)

# servidor (puerto 8083)
make run
```

## Correr con Docker

Cada componente tiene su propio compose; `make compose-up` los levanta en orden (mongo → redis → app):

```sh
# Red compartida con el ecosistema
docker network create mired 2>/dev/null || true

cp .env.example .env
make compose-up        # mongo + redis + persister
curl http://127.0.0.1:8083/api/v1/health   # {"status":"ok"}
```

Composes individuales: `make compose-mongo`, `make compose-redis`, `make compose-app`. La app queda expuesta al ecosistema como `http://persister:8083` en la red `mired` (y opcionalmente `http://persister.internal` vía Traefik). Por seguridad, los puertos del host se publican solo en `127.0.0.1` (nunca en la LAN) y Mongo queda aislada en la red interna `db-net`.

### Verificación tras el arranque

```sh
# Health
curl -i http://127.0.0.1:8083/api/v1/health

# Logs (índices + consumer group)
docker logs --tail 40 persister

# Redis (stream, consumer group y consumidores)
docker exec persister-redis redis-cli XRANGE document-events - + | head -5
docker exec persister-redis redis-cli XINFO GROUPS document-events
docker exec persister-redis redis-cli XINFO CONSUMERS document-events documents-persister

# Mongo (índices) — usa mongo shell nativo en 4.4
docker exec persister-mongo mongo --quiet --eval 'db.getSiblingDB(process.env.MONGODB_DATABASE||"text_extractor_db").documents.getIndexes()' 2>&1 | grep -E 'uq_document_id|uq_checksum_active|ix_filename|ix_created_at|ix_summary_status'
```

Criterio de éxito: `/health` → `200 {"status":"ok"}`, existe el consumer group `documents-persister`, los 5 índices aparecen en Mongo.

### Paridad Dev/Prod (12 Factor IX)

| Aspecto | Dev (local) | Prod |
|---|---|---|
| MongoDB | `db-dev` network + puerto `127.0.0.1:27017` publicado | Solo `db-net` (internal), **sin** puerto publicado |
| Redis | `127.0.0.1:6379` publicado | Solo red `mired`, sin puerto host |
| App | `docker compose up` con build local | Imagen pre-built `pdf-extractext-repositorios:vX.Y.Z` |
| Logs | `docker logs -f` | Driver json-file con rotation (10MB/5 files) |

La red `db-dev` solo existe en `docker-compose.mongo.yml` para permitir tests de integración locales. En prod no se usa; Mongo queda aislado en `db-net` (internal=true).

### Conectividad con el Orquestador

El Orquestador debe estar en la red `mired`. Desde un contenedor del Orquestador:

```sh
docker exec <orquestador-container> curl -i http://persister:8083/api/v1/health
```

El servicio expone también labels Traefik (`Host(persister.internal)`) en la red `mired`. **Nota**: los middlewares `int-ratelimit@file` e `int-cb@file` referenciados en `docker-compose.app.yml:24` son **phantom** (no definidos en Traefik del ecosistema); la funcionalidad RL/CB existe in-app (ver ADR-010). El stream `document-events` debe publicarse con `XADD` usando `{"document_id": "...", "payload": "<json>"}` (el consumer lee el campo `payload`).

## Descubrimiento de servicio (Consul)

Opcional. Levanta un agente Consul single-node en la red `mired` que registra `persister` con un health check HTTP contra `/api/v1/health`.

```sh
make compose-consul
curl -s http://127.0.0.1:8500/v1/health/service/persister   # Checks[].Status == "passing"
# UI: http://127.0.0.1:8500/ui
```

**Qué NO hace:** no reemplaza el DNS de Docker (`http://persister:8083`) ni elimina los labels de Traefik (ver [ADR-009](docs/adr/ADR-009-registro-consul.md)). El check cada 10 s golpea `/health` (ping real a Mongo+Redis); si la app está caída el servicio queda `critical` en el catálogo (no se desregistra).

## Configuración

| Variable            | Default                                         | Descripción                    |
| ------------------- | ----------------------------------------------- | ------------------------------ |
| `HTTP_PORT`         | `8083`                                          | Puerto HTTP interno            |
| `MONGODB_URI`       | `mongodb://localhost:27017/text_extractor_db`   | URI de MongoDB                 |
| `MONGODB_DATABASE`  | `text_extractor_db`                             | Base de datos                  |
| `REDIS_ADDR`        | `localhost:6379`                                | Dirección de Redis             |
| `STREAM_NAME`       | `document-events`                               | Stream de eventos              |
| `STREAM_GROUP`      | `documents-persister`                           | Consumer group                 |
| `DLQ_NAME`          | `document-events-dlq`                           | Dead-letter queue              |
| `RETRY_MAX`         | `5`                                             | Intentos antes de DLQ          |
| `RETRY_BACKOFF`     | `5s`                                            | Retraso base entre reintentos  |
| `MAX_TEXT_BYTES`    | `15728640` (15 MiB)                             | Límite del texto extraído      |
| `LOG_LEVEL`         | `info`                                          | Nivel de log                   |
| `CONSUMER_NAME`     | `hostname-pid`                                  | Nombre único por réplica (Bloque B) |
| `RATE_LIMIT_RPS`    | `100`                                           | Requests/segundo por IP (Bloque C) |
| `RATE_LIMIT_BURST`  | `200`                                           | Burst token bucket (Bloque C)  |
| `CB_FAILURES`       | `5`                                             | Fallos para abrir circuit breaker (Bloque C) |
| `CB_TIMEOUT`        | `30s`                                           | Timeout antes de half-open (Bloque C) |
| `CACHE_TTL`         | `60s`                                           | TTL read-through cache (Bloque D) |

> El tope está por debajo del límite de 16 MiB por documento de MongoDB: el
> texto extraído se persiste entero en un solo documento, así que un evento
> por encima de `MAX_TEXT_BYTES` se rechaza en la ingesta (retry → DLQ) en
> lugar de fallar en la escritura.

## API interna

Solo lectura/borrado; no hay `POST /documents` (la creación llega por stream).

> **Estado actual (2026-11-10)**: `/health`, lectura por `id`/`checksum`,
> listado, descargas, soft-delete y restore implementados (sin endpoints en
> `501`). Middleware que propaga `X-Request-ID`; Swagger descartado para el MVP
> (ADR-008); test e2e vertical en `test/e2e` (requiere compose). La tabla
> describe el contrato real.

| Método | Ruta                                        | Respuesta                     |
| ------ | ------------------------------------------- | ----------------------------- |
| `GET`  | `/api/v1/health`                            | `200 {"status":"ok"}` / `503` |
| `GET`  | `/api/v1/documents/{id}`                    | `200` documento / `404 document_not_found` |
| `GET`  | `/api/v1/documents/checksum/{checksum}`     | `200` documento / `404 document_not_found` |
| `GET`  | `/api/v1/documents`                         | `200` lista paginada          |
| `GET`  | `/api/v1/documents/{id}/download/original`  | `200 text/plain`              |
| `GET`  | `/api/v1/documents/{id}/download/summary`   | `200 text/plain` / `404 summary_not_available` / `409 summary_pending` |
| `DELETE` | `/api/v1/documents/{id}`                  | `204` (soft-delete)           |
| `POST` | `/api/v1/documents/{id}/restore`            | `204` / `404` / `409 checksum_conflict` |

Errores en envelope `{"code","message","details"}` con códigos públicos `invalid_request`, `document_not_found`, `summary_not_available`, `summary_pending`, `checksum_conflict`, `rate_limited`. El payload del stream llega bajo la clave `payload`. Paginación por defecto `20`, máx `100`. Filtro `status` acepta `NOT_REQUESTED|PENDING|RESOLVED|FAILED` (`FAILED` → lista vacía).

**Supuestos provisionales:** `metadata` typed `{filename,size_bytes,page_count}` (claves extra descartadas), precedencia top-level `filename` sobre `metadata.filename`, `summary_status: FAILED` nunca llega al stream.

## ADRs de resiliencia 
- [ADR-010](docs/adr/ADR-010-circuit-breaker-rate-limit.md) — Circuit Breaker + Rate Limit in-app
- [ADR-011](docs/adr/ADR-011-cache-read-through.md) — Cache Redis read-through
- [ADR-012](docs/adr/ADR-012-saga-forward-only.md) — Saga forward-only (sin coreografía/orquestación)

## Contribuciones

Toda modificación de código (humana o asistida por IA) debe cumplir los
principios, reglas y proceso de [AGENTS.md](AGENTS.md) — obligatorio antes
de dar por terminado un cambio.
