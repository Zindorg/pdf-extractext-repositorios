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
internal/adapters/health/ ping de Mongo y Redis para /health
internal/api/        router Gin, handlers, DTOs y health check
internal/testutil/   fixtures de documentos compartidos por los tests
```

## Requisitos

- Go 1.26+
- Docker (para Mongo/Redis) o instancias locales
- Red Docker `mired` ya creada: `docker network create mired`
  (la red `db-net` la crea `docker-compose.mongo.yml` al levantar)

## Correr localmente

```sh
# dependencias: Mongo 8 (replica set) + Redis 7
docker compose -f docker-compose.mongo.yml up -d
docker compose -f docker-compose.redis.yml up -d

# tests
make test

# servidor (puerto 8083)
make run
```

## Correr con Docker

Cada componente tiene su propio compose; `make compose-up` los levanta en orden (mongo → redis → app):

```sh
cp .env.example .env
make compose-up        # mongo + redis + persister
curl http://localhost:8083/api/v1/health   # {"status":"ok"}
```

Composes individuales: `make compose-mongo`, `make compose-redis`, `make compose-app`. La app queda expuesta al ecosistema como `http://persister:8083` en la red `mired`.

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
| `MAX_TEXT_BYTES`    | `52428800` (50 MB)                              | Límite del texto extraído      |
| `LOG_LEVEL`         | `info`                                          | Nivel de log                   |

## API interna

Solo lectura/borrado; no hay `POST /documents` (la creación llega por stream).

> **Estado actual (2026-10-07)**: `/health`, lectura por `id`/`checksum`,
> listado, soft-delete y restore implementados. Pendientes (responden `501`):
> descargas `download/original` y `download/summary`. La tabla describe el
> resto del contrato final.

| Método | Ruta                                        | Respuesta                     |
| ------ | ------------------------------------------- | ----------------------------- |
| `GET`  | `/api/v1/health`                            | `200 {"status":"ok"}` / `503` |
| `GET`  | `/api/v1/documents/{id}`                    | `200` documento / `404`       |
| `GET`  | `/api/v1/documents/checksum/{checksum}`     | `200` documento / `404`       |
| `GET`  | `/api/v1/documents`                         | `200` lista paginada          |
| `GET`  | `/api/v1/documents/{id}/download/original`  | `200 text/plain`              |
| `GET`  | `/api/v1/documents/{id}/download/summary`   | `200 text/plain` / `409`      |
| `DELETE` | `/api/v1/documents/{id}`                  | `204` (soft-delete)           |
| `POST` | `/api/v1/documents/{id}/restore`            | `204` / `409`                 |

Errores en envelope `{"code","message","details"}`; paginación por defecto `20`, máx `100`.

## Contribuciones

Toda modificación de código (humana o asistida por IA) debe cumplir los
principios, reglas y proceso de [AGENTS.md](AGENTS.md) — obligatorio antes
de dar por terminado un cambio.
