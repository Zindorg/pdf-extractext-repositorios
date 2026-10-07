# ADR-007 — Reconocimiento del contrato de eventos del Orquestador (delta-persister)

- Estado: **Aceptado** (2026-10-07)
- Contexto: `docs/proyect information of orquestador/delta-persister.md`
- Alcance: Slice 6 (consumidor Redis Streams) + migración de tiempos de Slices 1–5

## Decisión

Se adopta el esquema de eventos propuesto por el Orquestador en `delta-persister.md`,
con la tabla de evaluación siguiente. Cada fila del delta queda categorizada:

| # | Cambio del delta | Decisión |
|---|---|---|
| 1 | `summary` → `summary_resolved` | Adoptar |
| 2 | `processing_time_ms` → `extraction_time_ms` + `summary_time_ms` | Adoptar (migración Slices 1–5) |
| 3 | `schema_version` obligatorio | Adoptar (`!=1` ⇒ retry → DLQ, nunca éxito) |
| 4 | `event_type` "además del nombre del mensaje" | Adoptar con fallback tolerante a `event` |
| 5 | `summary_requested` / `summary_status` | Adoptar (solo parse, no persistir) |
| 6 | `mime_type` top-level | Adoptar (mapear a `Metadata.MimeType`) |
| 7 | Header `X-Request-ID` → `X-Document-Id` | Diferir (depende de middleware routing) |
| 8 | Red `mired` | Ya hecho (compose, Slices anteriores) |
| 9 | Prefijo `/api/v1` | Ya hecho (Slices anteriores) |
| 10 | Alias/dominio compose | Ya definido |
| 11 | Ruta consumo Orquestador | Sin impacto |
| 12 | `code: summary_pending` con 409 | Diferir a slice de descargas |

## Consecuencias y reglas

- **Fuente de verdad dual**: el contrato de eventos se lee en `ARCHITECTURE.md` §9
  y el detalle del mensaje en `docs/specs/SPEC-stream-consumer.md`.
- **Discriminador tolerante**: el consumer despacha por `event_type` si viene,
  si no por `event`; ambos presentes deben coincidir o el mensaje es inválido
  (retry → DLQ). Sin `events.md` del Orquestador, se mantiene esta compatibilidad.
- **Mensajes inválidos** (`schema_version != 1`, JSON/campos rotos) ⇒ **retry →
  DLQ**, nunca tratar como éxito (expectativa #2 del Orquestador).
- **Sin idempotencia por `event_id`**: no figura en el contrato; la dedup la
  absorbe el índice único parcial de Mongo + semántica at-least-once (ACK + descarte).
- **Migración de tiempos**: `ProcessingTimeMS` deja de persistirse; el dominio
  guarda `ExtractionTimeMS` y `SummaryTimeMS`; la API de lectura expone la suma
  como `processing_time_ms`.
- **Docs honestas**: `ARCHITECTURE.md`, diagramas PUML y esta carpeta reflejan
  el estado real del código.

## Alternativas consideradas

- **No adoptar (contrato propio)**: mantenía `processing_time_ms` y `event:
  original/summary`, pero rompía la integración con el Orquestador al primer
  evento real. Rechazada.
- **Adoptar todo (incluidas filas 7 y 12)**: sobre-ingeniería; la fila 7
  depende de un middleware pendiente y la 12 de la fase de descargas. Diferidas.

## Referencias

- `ARCHITECTURE.md` §4, §8, §9, §10
- `docs/specs/SPEC-stream-consumer.md`
- `internal/domain/stream.go`
- `internal/adapters/redis/`