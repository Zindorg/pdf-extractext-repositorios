# ADR-007 — Reconocimiento del contrato de eventos del Orquestador (delta-persister)

- Estado: **Aceptado** (2026-10-07, actualizado 2026-10-21)
- Contexto: `docs/proyect information of orquestador/delta-persister.md`, `docs/proyect information of orquestador/events.md`, `docs/proyect information of orquestador/openapi.yaml`
- Alcance: Slice 6 (consumidor Redis Streams) + migración de tiempos de Slices 1–5 + alineación de contrato HTTP

## Decisión

Se adopta el esquema de eventos propuesto por el Orquestador en `delta-persister.md`,
con la tabla de evaluación siguiente. Cada fila del delta queda categorizada:

| # | Cambio del delta | Decisión |
|---|---|---|
| 1 | `summary` → `summary_resolved` | Adoptar |
| 2 | `processing_time_ms` → `extraction_time_ms` + `summary_time_ms` | Adoptar (migración Slices 1–5) |
| 3 | `schema_version` obligatorio | Adoptar (`!=1` ⇒ retry → DLQ, nunca éxito) |
| 4 | `event_type` "además del nombre del mensaje" | Adoptar con fallback tolerante a `event` |
| 5 | `summary_requested` / `summary_status` | Adoptar (persistir `summary_status`, normalizar al ingerir) |
| 6 | `mime_type` top-level | Adoptar (mapear a `Metadata.MimeType`) |
| 7 | Header `X-Request-ID` → `X-Document-Id` | Aplicado como propagación de `X-Request-ID` (Slice 8); **no** se renombra a `X-Document-Id` (ver nota) |
| 8 | Red `mired` | Ya hecho (compose, Slices anteriores) |
| 9 | Prefijo `/api/v1` | Ya hecho (Slices anteriores) |
| 10 | Alias/dominio compose | Ya definido (persister.internal) |
| 11 | Ruta consumo Orquestador | Sin impacto |
| 12 | `code: summary_pending` con 409 | Aplicado (descargas); el 409 de `download/summary` usa el code literal `summary_pending` del Orquestador |
| 13 | Campo del stream = `payload` | Adoptar (envoltorio `{"document_id": "...", "payload": "<json>"}`) |

## Consecuencias y reglas

- **Fuente de verdad dual**: el contrato de eventos se lee en `ARCHITECTURE.md` §9
  y el detalle del mensaje en `docs/specs/SPEC-stream-consumer.md`.
- **Discriminador tolerante**: el consumer despacha por `event_type` si viene,
  si no por `event`; ambos presentes deben coincidir o el mensaje es inválido
  (retry → DLQ). Se respeta el contrato cerrado del Orquestador.
- **Mensajes inválidos** (`schema_version != 1`, JSON/campos rotos) ⇒ **retry →
  DLQ**, nunca tratar como éxito (expectativa #2 del Orquestador).
- **Sin idempotencia por `event_id`**: no figura en el contrato; la dedup la
  absorbe el índice único parcial de Mongo + semántica at-least-once (ACK + descarte).
- **Migración de tiempos**: el dominio guarda `ExtractionTimeMS` y `SummaryTimeMS`; la API de lectura expone `extraction_time_ms`, `summary_time_ms` (`*int64`, null sin resumen) y `processing_time_ms` (`extraction_time_ms + summary_time_ms`).
- **Eje de resumen**: `summary_status` (`NOT_REQUESTED|PENDING|RESOLVED|FAILED`). Normalización al ingerir; idempotencia por presencia de `summary`. `filename` top-level con precedencia sobre `metadata.filename`. **Supuestos provisionales**: `metadata` typed; claves extra descartadas; `summary_status: FAILED` nunca llega al stream.
- **Header de correlación (fila 7)**: se propaga `X-Request-ID` tal cual llega
  (si viene) mediante el middleware de `internal/api/middleware`, en línea con
  `ARCHITECTURE_REPOSITORIES.md` del ecosistema. Se descarta el renombre a
  `X-Document-Id`: el header es de correlación de request y no participa de la
  lógica; el identificador del documento ya viaja en la ruta/evento.
- **Contrato HTTP**: códigos públicos `invalid_request`, `document_not_found`, `summary_not_available`, `summary_pending`, `checksum_conflict`. `delivery_status` siempre `AVAILABLE`. `deleted_at` nullable. Filtro `status` acepta los 4 valores; `FAILED` devuelve lista vacía. Separación `summary_not_available` (404) vs `summary_pending` (409).
- **Docs honestas**: `ARCHITECTURE.md`, diagramas PUML y esta carpeta reflejan
  el estado real del código.

## Alternativas consideradas

- **No adoptar (contrato propio)**: mantenía `processing_time_ms` y `event:
  original/summary`, pero rompía la integración con el Orquestador al primer
  evento real. Rechazada.
- **Diferir la fila 7** (posición original): se descartó al agregar el middleware
  de request-ID en el Slice 8; la propagación de `X-Request-ID` no depende de la
  lógica de routing. La fila 12 se aplicó en el Slice 7 (descargas) con el code
  literal `summary_pending`.

## Referencias

- `ARCHITECTURE.md` §4, §8, §9, §10
- `docs/specs/SPEC-stream-consumer.md`
- `internal/domain/stream.go`
- `internal/adapters/redis/`