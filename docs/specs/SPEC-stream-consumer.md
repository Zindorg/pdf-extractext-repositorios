# SPEC — Consumidor de Redis Streams (Slice 6)

> Documento de especificación para implementar el consumidor de `document-events`.
> Fuente de contrato: `docs/proyect information of orquestador/ARCHITECTURE_REPOSITORIES.md`
> (§9 stream, §4 ingesta) + deltas aceptados por este microservicio en
> `delta-persister.md` (registrados en `docs/adr/ADR-007.md`).
> Proceso conducido con TDD (rojo → verde).

## 1. Objetivo y alcance

Implementar el **adapter Redis** que consume eventos del stream `document-events`
con consumer group `documents-persister`, delega en `DocumentService` y gestiona
ACK / retry / DLQ. Cierra la fase de lógica del esqueleto pendiente en
`internal/adapters/redis`.

**Fuera de alcance:** smoke e2e contra compose (etapa final del proyecto),
búsqueda full-text, auth, middleware de request-id, descargas HTTP.

## 2. Terminología

| Término | Definición |
|---|---|
| `original` | Evento del texto extraído (FASE 1). Crea el documento `PENDING`. |
| `summary_resolved` | Evento del resumen (FASE 2). Completa el documento (`COMPLETED`). |
| `schema_version` | Versión del shape del mensaje. Hoy `1`. |
| DLQ | Stream `document-events-dlq`, copia del mensaje fallido + causa. |
| Consumer group | Entrega *at-least-once*: un mensaje puede entregarse varias veces. |

## 3. Message schema (contrato cerrado, con deltas aceptados)

### 3.1 Evento `original`

```json
{
  "event_type": "original",
  "schema_version": 1,
  "document_id": "c9bf9e57-1685-4c89-bafb-ff5af830be8a",
  "checksum": "a6f3…64hex",
  "extracted_text": "…texto extraído…",
  "extraction_time_ms": 342,
  "mime_type": "application/pdf",
  "summary_requested": true,
  "summary_status": "PENDING",
  "metadata": {
    "filename": "informe_financiero.pdf",
    "size_bytes": 2048576,
    "page_count": 12
  }
}
```

| Campo | Requerido | Anotación |
|---|---|---|
| `event_type` | sí | Discriminador: `original`. Fallback tolerante a `event` (ver §3.3) |
| `schema_version` | sí | `!= 1` ⇒ mensaje inválido ⇒ retry → DLQ |
| `document_id` | sí | Clave de negocio (UUID) |
| `checksum` | sí | `sha256(extracted_text)`, minúsculas |
| `extracted_text` | sí | Texto completo |
| `extraction_time_ms` | sí | Tiempo de la pata extracción (reemplaza `processing_time_ms`) |
| `mime_type` | sí | Campo top-level; se mapea a `Metadata.MimeType` en el dominio |
| `summary_requested` | sí | Solo informativo (parse, no persiste) |
| `summary_status` | sí | `PENDING`. Solo informativo |
| `metadata.filename` | sí | — |
| `metadata.size_bytes` / `page_count` | no | Opcionales |

### 3.2 Evento `summary_resolved`

```json
{
  "event_type": "summary_resolved",
  "schema_version": 1,
  "document_id": "c9bf9e57-1685-4c89-bafb-ff5af830be8a",
  "summary": "…resumen…",
  "summary_time_ms": 120,
  "summary_status": "RESOLVED"
}
```

| Campo | Requerido | Anotación |
|---|---|---|
| `event_type` | sí | Discriminador: `summary_resolved`. Fallback tolerante a `event` |
| `schema_version` | sí | `!= 1` ⇒ mensaje inválido ⇒ retry → DLQ |
| `document_id` | sí | Clave de negocio |
| `summary` | sí | Texto resumido |
| `summary_time_ms` | sí | Tiempo de la pata resumen (se persiste vía `CompleteWithSummary`) |
| `summary_status` | sí | `RESOLVED`. Solo informativo |

### 3.3 Discriminador: `event_type` vs `event`

El delta (#4) agrega `event_type` "además del nombre del mensaje"; sin acceso al
`events.md` del Orquestador el campo exacto no está cerrado. Estrategia tolerante:
el consumer despacha por `event_type` si viene, si no por `event`. Valores
aceptados: `original` y `summary_resolved`. Ambos campos deben coincidir si
vienen juntos; si no, el mensaje se trata como inválido (retry → DLQ).

Cada entrada del stream envuelve el payload en un único campo `event`:
`XADD document-events * event <json>`, siendo `<json>` el objeto de §3.1/§3.2.
Una entrada sin ese campo es inválida (retry → DLQ).

### 3.4 Rupturas respecto al esqueleto actual

| # | Esqueleto actual (`events.go`) | Contrato cerrado | Tipo |
|---|---|---|---|
| 1 | `event`, valores `"original"` / `"summary"` | `event_type`/`event`, valores `"original"` / `"summary_resolved"` | rompe |
| 2 | `processing_time_ms` en `original` | `extraction_time_ms` (y `summary_time_ms` en el resumen) | rompe |
| 3 | sin `schema_version` | obligatorio (`=1`) | aditivo |
| 4 | `mime_type` dentro de `metadata` | campo top-level | rompe |
| 5 | sin `summary_requested` / `summary_status` | presentes (informativos) | aditivo |

## 4. Mapeo evento → caso de uso

| `event_type` | Llamada service |
|---|---|
| `original` | `service.CreatePending(ctx, domain.Document{DocumentID, Checksum, ExtractedText, ExtractionTimeMS, Metadata{Filename, MimeType, SizeBytes, PageCount}})` |
| `summary_resolved` | `service.CompleteWithSummary(ctx, documentID, summary, summaryTimeMS)` |

`CreatePending` fuerza `Status=PENDING` + `CreatedAt`/`UpdatedAt` con `domain.Now()`.
`CompleteWithSummary` es idempotente: si el doc está `COMPLETED` ⇒ retorno sin
error (semántica **ACK + descarte** del blueprint §9). Tras la migración del
Ciclo A, `ProcessingTimeMS` deja de persistirse: se guardan
`ExtractionTimeMS` + `SummaryTimeMS` y la API de lectura expone la suma.

## 5. ACK / Retry / DLQ policy

| Situación | Acción |
|---|---|
| Éxito (service → `nil`) | `XACK` |
| Dup legítimo `original` (`ErrDuplicateChecksum` / `ErrDuplicateDocumentID`) | `XACK` + descarte silencioso |
| Dup legítimo `summary_resolved` (doc ya `COMPLETED`) | `XACK` + descarte (lo absorbe el service) |
| `schema_version != 1`, JSON inválido o campos requeridos ausentes | Mensaje inválido ⇒ **retry → DLQ** (no éxito, no descarte) |
| `summary_resolved` huérfano (`ErrNotFound`) | Retry con backoff (`RETRY_MAX` intentos) → DLQ |
| Error transitorio (timeout, conexión) | Retry con backoff constante (`RETRY_BACKOFF`) → DLQ |
| Otro error de dominio no catalogado | Retry max `RETRY_MAX`, luego DLQ |

### 5.1 DLQ payload

Copia del mensaje original + campos de causa:

```json
{
  "message": { "…mensaje original…" },
  "error": "error del último intento",
  "attempts": 5,
  "failed_at": "2026-10-07T18:30:00Z"
}
```

### 5.2 Backoff

Backoff **constante** = `RETRY_BACKOFF` (sin exponencial ni jitter: YAGNI). El
re-encolado para reintento se apoya en el mecanismo nativo del consumer group:
el mensaje queda *pending* sin `XACK` y se reclama con `XAUTOCLAIM` cuando lleva
más de un `RETRY_BACKOFF` sin resolver (`MinIdle` de XAUTOCLAIM). Los intentos
se llevan en un hash Redis `&lt;stream&gt;:retries` (HINCRBY por message ID).

## 6. Idempotencia

Idempotencia delegada en la **dedup de consistencia fuerte del índice único
parcial de Mongo** (checksum `deleted_at: null`) + semántica *at-least-once*:
re-entregas legítimas ⇒ ACK + descarte. **No** se usa `event_id` ni un set de
procesados en Redis (no figuran en el contrato; YAGNI).

## 7. Puerto consumer-side (DIP)

```go
// internal/domain/stream.go
type StreamConsumer interface {
    Run(ctx context.Context) error
}
```

El `*redis.StreamConsumer` (adapter) implementa `domain.StreamConsumer`.

## 8. Config (env vars)

| Variable | Default | En `Config` (`internal/config`) |
|---|---|---|
| `REDIS_ADDR` | `localhost:6379` | `RedisAddr` (ya existe) |
| `STREAM_NAME` | `document-events` | `StreamName` (ya existe) |
| `STREAM_GROUP` | `documents-persister` | `StreamGroup` (ya existe) |
| `DLQ_NAME` | `document-events-dlq` | `DLQName` (ya existe) |
| `RETRY_MAX` | `5` | `RetryMax` (ya existe) |
| `RETRY_BACKOFF` | `5s` | `RetryBackoff` (ya existe) |

Sin variables nuevas (no hay `EVENT_DEDUP_TTL`).

## 9. Estructura objetivo

```
internal/
├── domain/
│   └── stream.go                # NUEVO — interface StreamConsumer
├── adapters/redis/
│   ├── events.go                # MODIFICAR — schema contrato cerrado (3.x)
│   ├── handlers.go              # MODIFICAR — OnOriginal/OnSummaryResolved reales
│   ├── retry_dlq.go             # MODIFICAR — lógica backoff/DLQ real
│   ├── consumer.go              # MODIFICAR — Run(): loop XREADGROUP/XACK
│   ├── consumer_test.go         # NUEVO — tests unitarios + integración
│   └── events_test.go           # MODIFICAR — fixture al nuevo schema
cmd/server/main.go               # MODIFICAR — wiring consumer + graceful shutdown
```

## 10. Comportamiento del esqueleto a respetar / reemplazar

Mientras la fase de lógica no conecte nada, se respetan los marcadores
(`errNotImplemented`, `Run` noop, `ack: nil`). Este slice **reemplaza** esos
marcadores por la implementación real; no vuelve a declarar lógica muerta.

## 11. Criterios de aceptación (GREEN)

1. `original` válido ⇒ `CreatePending` llamado; documento creado `PENDING`.
2. `original` con `checksum`/`document_id` activo ya existente ⇒ `XACK` + descarte, sin error.
3. `summary_resolved` válido ⇒ `CompleteWithSummary`; documento `COMPLETED`.
4. `summary_resolved` sobre doc ya `COMPLETED` ⇒ `XACK` + descarte.
5. `summary_resolved` huérfano ⇒ reintentos (max `RETRY_MAX`) ⇒ DLQ.
6. `schema_version != 1`, campos ausentes o JSON inválido ⇒ retry → DLQ (nunca éxito).
7. `event_type` ausente con `event` presente ⇒ despacho por `event` (tolerancia §3.3).
8. Error transitorio ⇒ backoff constante (`RETRY_BACKOFF`) + reintento (`XAUTOCLAIM`).
9. `Run(ctx)` devuelve al cancelar `ctx`; no pierde mensajes sin ACK.
10. `go test ./...`, `make lint` y `go vet -tags integration ./...` en verde.

## 12. Verificación obligatoria

```sh
make lint
go test ./...
go vet -tags integration ./...
go mod tidy -diff
docker compose -f docker-compose.mongo.yml config -q
docker compose -f docker-compose.redis.yml config -q
docker compose -f docker-compose.app.yml config -q
```