# Blueprint de Arquitectura — Microservicio de Persistencia de Documentos (MongoDB)

> Documento rector del proyecto. Mantener actualizado junto con el código.
> Última revisión: 2026-09-24. Alineado con `Especificaciones_v3.md` (hub & spoke, aislamiento de BD y zero-disk) + decisiones propias de este microservicio (dedup por checksum, ciclo de vida PENDING/COMPLETED).

## 1. Tipo de arquitectura

| Nivel | Tipo de arquitectura |
|---|---|
| **Sistema** (todo el ecosistema) | **Arquitectura Hub & Spoke** (microservicios): el **Orquestador** es el único punto de exposición pública vía **Traefik**; los workers (Extractor, Resumidor, este Persistidor) se comunican **solo con el Orquestador** por **URL de red interna**, nunca por IP. **Aislamiento de BD**: solo este microservicio conecta a MongoDB |
| **Servicio** (dentro de este microservicio) | **Arquitectura Hexagonal / Puertos y Adaptadores** (variante simplificada de Clean Architecture), de 4 capas concéntricas con **núcleo de dominio puro** |

Patrón central de diseño: **Repository pattern** (puerto en dominio + adaptador MongoDB) y **Consumer pattern** (suscripción a Redis Streams como adaptador de ingesta).

**Regla Hub & Spoke**: este microservicio no expone API pública. Escribe únicamente por eventos del stream (lo produce el Orquestador) y atiende lecturas/borrados por HTTP interno que **solo el Orquestador** invoca.

---

## 2. Stack

| Capa | Elección | Motivo |
|---|---|---|
| Lenguaje | **Go 1.26+** | Binario estático, bajo consumo por request, alta concurrencia nativa |
| HTTP | **Gin** | Binding, validación y rutas listas; el más usado para REST en Go |
| BD | **MongoDB 7** | BD del ecosistema original; dedup garantizado por índice único parcial |
| Driver Mongo | `go.mongodb.org/mongo-driver/v2` | Driver oficial de MongoDB para Go |
| Mensajería | **Redis Streams** (cliente `go-redis/v9`) | Ingesta asíncrona del texto original y del resumen por eventos; consumer groups y DLQ nativos |
| Migraciones | — | No aplica: índices y consumer group asegurados por la app al arrancar |
| ID | `document_id` (UUID) de negocio + `_id` (ObjectId) interno | Clave pública vs clave de almacenamiento separadas |
| Validación | `go-playground/validator` | Binding + reglas en DTOs y eventos |
| Docs API | `swaggo/swag` | UI en `/swagger/index.html` (entorno interno de consumo) |
| Tests | stdlib `testing` + `httptest` + `testify/mock` | TDD: unit, integración, contrato, e2e |
| Deployment | Docker multi-stage + Compose (redes internas aisladas) | URL por nombre de servicio (`persister:8083`), nunca IP |

---

## 3. Vista de sistema (contexto)

```
[CLIENTE EXTERNO]
       │  HTTPS — REST + JSON
       ▼
[TRAEFIK v3]                          (red pública · rule: /api)
       │
       ▼
[ORQUESTADOR]                         ◄ hub central (Go · 8080) · decide la tarea
       │
       │  ESCRITURA (async)                    │  LECTURA / BORRADO (sync)
       │  XADD document-events                 │  HTTP interno
       │  original / summary                   │  GET · DELETE http://persister:8083
       ▼                                      ▼
[REDIS STREAMS]                   [MS PERSISTENCIA]            ◄ ESTE PROYECTO
document-events ·                    Go + Gin · 8083
document-events-dlq                  solo red interna
       │  XREADGROUP / XACK / XAUTOCLAIM
       └──────────────────────────▶
       │
       │                           │  BSON (driver → documentos)
       ▼                          ▼
                         [MongoDB 7 · db-net]
                         colección `documents`
                         data-per-service
```

**Flujo de este microservicio:**

1. **Escritura (async)** — `Orquestador --XADD--> Redis --XREADGROUP--> Persistidor`: el Orquestador publica `original` (y `summary` si `summarize=true`); el consumer las procesa por `document_id`.
2. **Lectura/borrado (sync)** — `Orquestador --HTTP interno--> Persistidor` (`http://persister:8083`): responde texto/summary, descargas, soft-delete, restore y health bajo demanda del Orquestador.
3. **Persistencia (BSON)** — `Persistidor --BSON--> MongoDB` (`db-net`, aislada): el driver oficial usa BSON hacia la colección `documents`.

> **Fuera de alcance**: los workers Extractor y Resumidor no aparecen porque este microservicio **solo se comunica con el Orquestador** (hub & spoke). El Orquestador correlaciona por `document_id` el resumen que produce el Resumidor.

---

## 4. Flujo de ingesta (dos fases por `document_id`)

```
FASE 1 — ORIGINAL                     FASE 2 — SUMMARY
─────────────────────────             ─────────────────────────
event: "original"                     event: "summary"
document_id  (UUID)                   document_id  (UUID)
checksum     (sha256 extracted_text)  summary      (string)
extracted_text                        │
metadata                               │
  filename                            │
  mime_type                           │
  size_bytes                          │
  page_count                          ▼
             ┌──────────────────────────────────────────┐
             │  consumer document-events/documents-*     │
             │                                          │
             │  original  → insert PENDING              │
             │  summary   → update → COMPLETED          │
             └──────────────────────────────────────────┘
```

| Evento | Acción | Duplicado | Resumen huérfano |
|---|---|---|---|
| `original` | Insert doc `PENDING` | `document_id` o `checksum` activo ya existe ⇒ **ACK + descarte** (no es error) | — |
| `summary` | Update por `document_id` → `COMPLETED` (+ `summary`, `updated_at`) | Ya `COMPLETED` ⇒ **ACK + descarte** | `document_id` inexistente ⇒ **retry con backoff** (`XAUTOCLAIM`) → **DLQ** |

- **Solo se escribe por eventos**: no existe un endpoint REST de creación. Si `summarize=false` el Orquestador nunca encola `summary` y el documento queda `PENDING` **sin timeout** (decisión de negocio explícita): se guarda indefinidamente con su checksum ocupado.
- Los **duplicados legítimos** (re-entrega at-least-once de eventos ya procesados) ⇒ **ACK + descarte**; en la API REST un alta manual duplicada devolvería `409 Conflict`.
- Mensajes inválidos (validación) ⇒ retry → DLQ.

---

## 5. Vista interna (componentes y flujo de dependencia)

```
      ┌──────────────────────────────────────────────────────┐
      │  CAPA 1 · TRANSPORTE   internal/api                   │
      │  Gin Router · Handlers · DTOs + validación            │
      │  Middleware (logger, recovery, request-id)            │
      │  Swagger UI (/swagger/index.html)                     │
      └───────────────────┬──────────────────────────────────┘
                          │  invoca casos de uso
      ┌───────────────────▼──────────────────────────────────┐
      │  CAPA 2 · APLICACIÓN  internal/application           │
      │  Service: createPending, completeWithSummary,        │
      │  get, findByChecksum, list, softDelete, restore      │
      └───────────────────┬──────────────────────────────────┘
                          │  usa el PUERTO
      ┌───────────────────▼──────────────────────────────────┐
      │  CAPA 3 · DOMINIO  internal/domain                    │  núcleo puro:
      │  Document (entidad) · Status · errores de dominio     │  cero dependencias
      │  interfaz Repository (port)                           │  de framework/BD
      └───────────────────┬──────────────────────────────────┘
                          ▲  implementan (adapters)
      ┌───────────────────────────────────────────────────────┐
      │  CAPA 4 · ADAPTADORES                                  │
      │  mongodb/  repository (driver oficial) · mapper BSON   │
      │            ensure_indexes (índice único parcial)       │
      │  redis/    consumer (XGROUP/XREAD/XACK/XAUTOCLAIM)     │
      │            retry + DLQ                                  │
      └───────────────────┬──────────────────────┬─────────────┘
                          │  BSON                │  STREAM
              ┌───────────▼───────────┐   ┌───────▼──────────┐
              │  MongoDB 7            │   │  Redis           │
              └───────────────────────┘   └──────────────────┘
```

**Regla de dependencia (única regla del Hexagonal): todo apunta hacia adentro.** El dominio no conoce Gin, ni el driver de MongoDB, ni Redis ni HTTP. Los adaptadores implementan el *port* `Repository` y consumen el stream. La inversión se resuelve en el **composition root** (`cmd/server/main.go`), que construye de afuera hacia adentro: DB → repos, Redis → consumer → service → HTTP.

---

## 6. Estructura del proyecto

```
pdf-extractext-repositorios/
├── cmd/server/main.go            # composition root: wiring, graceful shutdown
├── internal/
│   ├── config/config.go          # env → struct tipado
│   ├── domain/
│   │   ├── model.go              # Document, Status, Metadata
│   │   ├── repository.go         # interfaz Repository (puerto)
│   │   └── errors.go             # ErrNotFound, ErrDuplicateChecksum,
│   │                             # ErrDuplicateDocumentID, ErrSummaryNotReady,
│   │                             # ErrRestoreConflict
│   ├── application/service.go    # reglas de negocio: dedup, completar, list,
│   │                             # soft-delete, restore
│   ├── adapters/mongodb/
│   │   ├── repository.go         # impl con el driver oficial
│   │   ├── mapper.go             # BSON ↔ domain.Document
│   │   └── ensure_indexes.go     # índices únicos + filename + created_at
│   ├── adapters/redis/
│   │   ├── consumer.go           # XGROUP/XREADGROUP/XACK
│   │   ├── handlers.go           # original / summary
│   │   └── retry_dlq.go          # XAUTOCLAIM + copia a DLQ
│   └── api/
│       ├── router.go             # gin, grupos, middleware, swagger
│       ├── handlers.go           # handlers HTTP
│       ├── dto.go                # request/response DTOs + validación
│       └── errors.go             # mapping dominio → HTTP + envelope
├── docs/adr/                     # ADR-0001… (véase §12)
├── CONTEXT.md                    # glosario del dominio
├── Dockerfile                    # multi-stage → distroless
├── docker-compose.yml            # mongo + redis + app (redes internas)
├── .env.example  .gitignore  Makefile  go.mod  go.sum  README.md  ARCHITECTURE.md
```

---

## 7. Componentes, responsabilidad y patrones

| Componente | Paquete | Responsabilidad | Patrón |
|---|---|---|---|
| Composition root | `cmd/server/main.go` | Wiring, config, conexiones, índices, consumer group, graceful shutdown | inyección por constructor |
| Config | `internal/config` | Env → struct tipado | Settings/12-factor |
| Dominio | `internal/domain` | Entidad, Status, port Repository, errores | Entidad + Puerto |
| Aplicación | `internal/application` | Reglas de negocio (dedup, completar, restore-conflict) | Service Layer / Casos de uso |
| Adapter Mongo | `internal/adapters/mongodb` | Persistencia real, índices, mapeo BSON↔dominio | Repository + Data Mapper |
| Adapter Redis | `internal/adapters/redis` | Consumo del stream, retry, DLQ | Consumer Group |
| API | `internal/api` | Contratos HTTP internos (lectura/descarga/borrado/health), validación, docs | DTO, Handler |
| Infra | docker-compose | `mongo` · `redis` · `app` en redes internas aisladas | Red `internal-net` + `db-net` |

---

## 8. Arquitectura de datos

### Modelo de dominio

```go
type Status string

const (
    StatusPending   Status = "PENDING"
    StatusCompleted Status = "COMPLETED"
)

type Metadata struct {
    Filename  string `json:"filename"`
    MimeType  string `json:"mime_type"`
    SizeBytes int64  `json:"size_bytes"`
    PageCount int    `json:"page_count"`
}

type Document struct {
    DocumentID       string    `json:"document_id"`    // UUID — clave de negocio
    Checksum         string    `json:"checksum"`       // sha256(extracted_text)
    Status           Status    `json:"status"`
    ExtractedText    string    `json:"extracted_text"`
    Summary          *string   `json:"summary"`        // null mientras PENDING
    Metadata         Metadata  `json:"metadata"`
    ProcessingTimeMS int64     `json:"processing_time_ms"`
    CreatedAt        time.Time `json:"created_at"`
    UpdatedAt        time.Time `json:"updated_at"`
    DeletedAt        *time.Time `json:"deleted_at"`    // soft-delete
}
```

### Documento BSON (colección `documents`)

```go
type persistedDocument struct {
    ID               primitive.ObjectID `bson:"_id"`
    DocumentID       string             `bson:"document_id"`
    Checksum         string             `bson:"checksum"`
    Status           string             `bson:"status"`
    ExtractedText    string             `bson:"extracted_text"`
    Summary          *string            `bson:"summary"`
    Metadata         metadata           `bson:"metadata"`
    ProcessingTimeMS int64              `bson:"processing_time_ms"`
    CreatedAt        time.Time          `bson:"created_at"`
    UpdatedAt        time.Time          `bson:"updated_at"`
    DeletedAt        *time.Time         `bson:"deleted_at"`
}
```

### Índices (creados por `ensureIndexes` al arrancar)

```javascript
{ key: { "document_id": 1 },            name: "uq_document_id",   unique: true }
{ key: { "checksum": 1 },               name: "uq_checksum_active",
  unique: true,
  partialFilterExpression: { "deleted_at": null } }   // solo activos
{ key: { "metadata.filename": 1 },      name: "ix_filename" }
{ key: { "created_at": -1 },            name: "ix_created_at" }
```

### Reglas y decisiones sobre los datos

- **Un solo agregado**: `Document`. Sin relaciones entre documentos.
- **Dos identidades separadas**: `document_id` (UUID) es la clave pública de negocio (index única no parcial); `_id` (ObjectId) es interna de Mongo y nunca sale hacia la API.
- **Deduplicación a nivel de BD**: índice único parcial sobre `checksum` (filter `deleted_at: null`). Insertar un checksum ya activo → error del driver código `11000` → `409 Conflict`.
- **Dedup por contenido**: `checksum = SHA-256(extracted_text)`, calculado por el **productor** (Orquestador); este servicio lo persiste tal cual (fuente de verdad del valor, no lo recalcula).
- **`processing_time_ms`**: lo registra el Orquestador (tiempo total de extracción+resumen) y viaja en el evento `original`; el Persistidor solo lo almacena (valor informativo, conforme `Especificaciones_v3.md` §6.B).
- **Status** como ciclo de vida: `PENDING` (llegó el original) → `COMPLETED` (llegó el resumen). No hay timeout: un `PENDING` conserva su checksum ocupado.
- **Soft delete** como estado (`deleted_at`), no borrado físico: un doc borrado sale del índice parcial → **libera** su checksum → permite re-ingerir el mismo contenido con un **nuevo** `document_id`.
- **Restore**: re-activar (unset `deleted_at`) colisiona en el índice único con un nuevo activo ya re-ingerido → `11000` → `ErrRestoreConflict` → `409`. El índice es la fuente de verdad, sin carreras.
- **Mapeo**: el adaptador convierte BSON ↔ `Document`; `ObjectId`, `Status` y tipos BSON nunca salen hacia la API (expone `document_id`, `status` como string).

---

## 9. Contrato del stream Redis

```
STREAM  document-events
GROUP   documents-persister (created by the app al arrancar)

Mensaje "original":
{
  "event": "original",
  "document_id": "c9bf9e57-1685-4c89-bafb-ff5af830be8a",
  "checksum": "a6f3…64hex",
  "extracted_text": "…texto extraído…",
  "processing_time_ms": 342,
  "metadata": { "filename": "informe_financiero.pdf",
                "mime_type": "application/pdf",
                "size_bytes": 2048576,
                "page_count": 12 }
}

Mensaje "summary":
{
  "event": "summary",
  "document_id": "c9bf9e57-1685-4c89-bafb-ff5af830be8a",
  "summary": "…resumen…"
}

DLQ  document-events-dlq   (copia del mensaje + causa cuando agota retries)
```

**Semántica de consumo**: consumer group con entregas *at-least-once*. Procesar ⇒ `XACK`. Fallo recuperable ⇒ reinversión con `XAUTOCLAIM` y backoff (max retries); pasado el límite ⇒ copia a `document-events-dlq` + `XACK` para no bloquear el group. Duplicados legítimos (mismo `document_id`/`checksum` activo, o `summary` sobre un `COMPLETED`) ⇒ `XACK` + descarte silencioso.

---

## 10. API — Contrato HTTP interno `/api/v1`

> **Accesible solo por el Orquestador** por URL de red interna (`http://persister:8083`). No se expone por Traefik. La creación NO usa HTTP: llega por stream (§4/§9).

| Método | Ruta | Request | Éxito | Errores |
|---|---|---|---|---|
| GET | `/documents/{document_id}` | — | `200` + documento (`extracted_text`, `summary`, `metadata`, `processing_time_ms`) | `400` · `404` |
| GET | `/documents/checksum/{checksum}` | — | `200` + doc | `400` · `404` |
| GET | `/documents/{document_id}/download/original` | — | `200` `text/plain` + `Content-Disposition` (RFC 5987) | `400` · `404` |
| GET | `/documents/{document_id}/download/summary` | — | `200` `text/plain` + `Content-Disposition` | `400` · `404` · `409` aún `PENDING` |
| GET | `/documents` | `page, page_size, status, filename, created_from, created_to, include_deleted` | `200` `{items, total, page, page_size}` | `400` |
| DELETE | `/documents/{document_id}` | — | `204` (soft) | `400` · `404` |
| POST | `/documents/{document_id}/restore` | — | `204` | `400` · `404` · `409` conflicto checksum |
| GET | `/health` | — | `200` `{status:"ok"}` | `503` BD/Redis caídos |

> **Nota de diseño**: no hay `POST /documents`. La escritura es evento-dirigida (hub & spoke). Si más adelante se quisiera una vía REST de alta, se agregaría como decisión formal (ADR) y devolvería `409` ante checksum/document_id activo.

Envelope de error consistente: `{"code": "...", "message": "...", "details": {...}}` + `X-Request-ID`. Paginación default 20, máx 100. Listado excluye soft-deleted por defecto.

---

## 11. Cross-cutting concerns

- **Config**: env vars con defaults (`HTTP_PORT`, `MONGODB_URI`, `MONGODB_DATABASE`, `REDIS_ADDR`, `STREAM_NAME`, `STREAM_GROUP`, `DLQ_NAME`, `RETRY_MAX`, `RETRY_BACKOFF`, `MAX_TEXT_BYTES`, `LOG_LEVEL`) — 12-factor.
- **Errores**: errores de dominio → HTTP (`404/409/400`) vía mapeo central en `internal/api/errors.go`.
- **Observabilidad**: middleware de request-ID (`X-REQUEST-ID`), logger de acceso, `/health` con ping a Mongo y Redis (consumido por el Orquestador para detectar disponibilidad).
- **Seguridad**: límite de tamaño de cuerpo (`MAX_TEXT_BYTES`) contra abuso; sin auth en MVP (red interna) — extensible con middleware de API key/JWT.
- **Validación**: en DTOs y eventos (binding + validator) para los bordes; reglas de negocio en el servicio.

---

## 12. Decisiones de arquitectura (ADR)

Live en `docs/adr/`:

- **ADR-001 — Ingesta asíncrona por Redis Streams**: el texto extraído y el resumen llegan como eventos en un stream (consumer group). Dos eventos por `document_id`: `original` (crea PENDING) y `summary` (completa) si `summarize=true`. **La creación no tiene vía HTTP** (hub & spoke, §2.1 de la spec). El Orquestador XADD; solo este servicio consume.
- **ADR-002 — Checksum del productor + índice único parcial**: `checksum = SHA-256(extracted_text)` lo calcula el productor y viaja en el evento; la garantía fuerte de dedup vive en el índice único parcial de Mongo (`deleted_at: null`), no solo en la app.
- **ADR-003 — document_id como clave de negocio**: UUID público distinto de `_id` interno; index único non-partial; soft-delete libera el checksum pero `document_id` nunca se reutiliza.
- **ADR-004 — PENDING sin timeout + retry/DLQ**: un documento sin resumen queda guardado indefinidamente con su checksum ocupado; los mensajes fallidos usan `XAUTOCLAIM` con backoff y luego `document-events-dlq`.
- **ADR-005 — Go + MongoDB + Redis**: Go 1.26 (binario estático), MongoDB 7 data-per-service aislado en `db-net`, Redis Streams como canal de ingesta del ecosistema.
- **ADR-006 — Hub & Spoke con URL interna (no Traefik en este servicio)**: este microservicio NO se expone por Traefik; el Orquestador lo alcanza por nombre de servicio Docker (`http://persister:8083`), cumpliendo "URL, nunca IP" sin romper el aislamiento (spec §2.1/§7). Traefik solo enruta hacia el Orquestador (red pública).

---

## 13. Arquitectura de despliegue

- `Dockerfile` multi-stage: `golang:1.26-alpine` (build) → `distroless` nonroot (binario ~10MB, sin shell).
- `docker-compose.yml` de ESTE microservicio: `mongo` + `redis` + `app`, en redes internas. **Traefik NO vive en este compose**: pertenece al ecosistema y solo expone al Orquestador. La app crea índices y el consumer group al arrancar; no hay servicio de migraciones.
- Redes: `internal-net` (app + redis, donde el Orquestador también está en el compose general) y `db-net` con `internal: true` (solo `app` + `mongo`, aislada del exterior).

```yaml
networks:
  internal-net: { driver: bridge }
  db-net:
    driver: bridge
    internal: true

mongo:
  image: mongo:7
  networks: [ db-net ]
  volumes: [ "mongodata:/data/db" ]
  healthcheck:
    test: ["CMD", "mongosh", "--quiet", "--eval", "db.adminCommand('ping').ok"]
    interval: 5s
    timeout: 5s
    retries: 10

redis:
  image: redis:7-alpine
  networks: [ internal-net ]
  healthcheck:
    test: ["CMD", "redis-cli", "ping"]
    interval: 5s
    timeout: 5s
    retries: 10

app:
  build: .
  networks: [ internal-net, db-net ]   # único con acceso a la BD
  environment:
    - HTTP_PORT=8083
    - MONGODB_URI=mongodb://mongo:27017/text_extractor_db
    - REDIS_ADDR=redis:6379
    - STREAM_NAME=document-events
    - STREAM_GROUP=documents-persister
    - DLQ_NAME=document-events-dlq
  healthcheck:
    test: ["CMD", "wget", "-qO-", "http://localhost:8083/api/v1/health"]
    interval: 10s
    timeout: 5s
    retries: 5
```

- **Comunicación entre microservicios por URL, nunca IP**: el Orquestador llama al Persistidor con `http://persister:8083` y este se conecta a Mongo/Redis por nombre de servicio Docker (`mongo`, `redis`). Los nombres de servicio son DNS estables que no cambian al re-arrancar el contenedor (el IP sí cambia), cumpliendo el requisito de las "mini-MV".
- En el compose del **ecosistema** (otro repo) viven: `traefik` (red pública), `orchestrator`, `extractor`, `summarizer` (red interna compartida con este servicio) y la red `public-net` del cliente. Este compose se unifica o acopla al del ecosistema compartiendo `internal-net`.

---

## 14. Arquitectura de testing

Pirámide de pruebas, todo conducido por **TDD** (rojo → verde):

| Capa | Tipo | Cobertura |
|---|---|---|
| Service | Unit (repo fake con `testify/mock`) | Create PENDING ok, checksum duplicado → 409, document_id duplicado → 409, completeWithSummary ok, resumen huérfano → error, GET borrado → 404, soft-delete inexistente → 404, restore conflicto → 409, list con filtros/paginación/total |
| Handlers | `httptest` | Status codes, shapes JSON, `Content-Disposition`, validación 400, envelope de error |
| Adapter Mongo | Integración (MongoDB real en Docker) | `11000` duplicado, soft-delete → re-ingest mismo checksum OK, restore bloqueado con nuevo activo, PENDING persistido sin resumen, orden/paginación |
| Consumer Redis | Integración (Redis real en Docker) | original → PENDING, original+summary → COMPLETED, duplicado ACK+descarte, summary huérfano → retry → DLQ |
| e2e | Smoke contra compose | XADD `original`/`summary` al stream → verificar en Mongo vía HTTP interno (`http://persister:8083`): lectura, descarga, soft-delete |

Comandos: `make test`, `make test-integration`, `go vet ./...`.

---

## 15. Extensiones y evolución

| Futuro cambio | Dónde entra | Sin tocar el resto |
|---|---|---|
| Búsqueda full-text del texto extraído | **text index** o **Atlas Search** + método nuevo en Repository | API y service casi intactos |
| Autenticación | Middleware nuevo en `internal/api` | Dominio intacto |
| Cambiar de BD (p.ej. PostgreSQL) | Nuevo adapter que implemente el mismo port | Service/API intactos |
| Comprimir/almacenar binarios en S3 | Adapter nuevo + campo `storage_ref` | Dominio casi intacto |
| Timeout/sLA para el resumen pendiente | Timer/worker + transición a estado nuevo en el domain | Ingesta intacta |
| Nuevos endpoints | Handler + caso de uso nuevo en service | Arquitectura sin fuerzas |
| Alta carga | Sharded Mongo / más replicas de consumer group | App sin cambios de código |

---

## 16. Gobernanza de arquitectura

- Este documento es el **estándar rector**: todo código nuevo debe respetar el flujo de dependencia (§5).
- Implementación conducida con TDD; revisión final con el skill `code-review-and-quality`.
- Al agregar funcionalidad nueva: ubicar por capa (§6), respetar puertos/adaptadores, y actualizar esta guía y los ADR si cambia una decisión.