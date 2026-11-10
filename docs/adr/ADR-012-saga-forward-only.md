# ADR-012 — Saga forward-only (sin coreografía ni orquestación)

## Contexto
El checklist de resiliencia exige "Saga" como patrón. El `persister` recibe eventos del stream Redis en orden no garantizado:
- `original` (documento + texto extraído + `summary_requested`)
- `summary_resolved` (resumen + `summary_time_ms`)

El flujo actual:
1. `original` → `Insert` (pending, `summary_status=PENDING` o `NOT_REQUESTED`)
2. `summary_resolved` → `Complete` (resuelto, idempotente por `HasSummary()`)
3. Reintentos con backoff exponencial → DLQ tras `RETRY_MAX`
4. Deduplicación por `checksum` (índice único parcial `uq_checksum_active`)

No hay transacción distribuida multi-servicio: el `persister` es un único participante que persiste y expone API de consulta. El Orquestador (fuera de este repo) decide cuándo pedir resumen y maneja timeouts/reintentos a su nivel.

## Decisión
**Forward-only**: no se implementa saga coreografiada (eventos de compensación) ni orquestada (state machine centralizado).

La idempotencia natural del dominio + deduplicación por checksum + retry/DLQ cubren el caso de uso:
- `Insert` es idempotente por `document_id` (409 si duplicado)
- `Complete` es idempotente por `HasSummary()` (devuelve existente sin error)
- `SoftDelete`/`Restore` son idempotentes por estado
- Fallo tras éxito parcial → evento queda en DLQ → reprocesamiento manual/automático externo

Si en el futuro se requiere rollback automático (ej. `summary_failed` que borre el documento), se añadiría un handler específico, no una infraestructura de saga genérica.

## Consecuencias
- **Positivo**: código simple, sin coordinador, sin state machine, sin eventos compensatorios, latencia mínima.
- **Negativo**: recuperación ante fallos parciales requiere intervención manual en DLQ (o job externo); no hay "undo" automático.
- **Neutro**: no cambia la API ni el contrato de eventos; el Orquestador sigue dueño del flujo.

## Alternativas evaluadas
| Alternativa | Por qué se descarta |
|---|---|
| Coreografía (eventos `compensate` / `rollback`) | Requiere publicar eventos de compensación, consumirlos, manejar orden y fallos en cascada; over-engineering para un solo participante. |
| Orquestación (state machine en Orquestador o sidecar) | Mueve la complejidad fuera pero acopla al Orquestador; YAGNI para MVP. |
| Saga library (ej. `temporal`, `cadence`) | Dependencia pesada, curva de aprendizaje, infra extra; no justificado. |

## Estado
Aceptado. Implementado como forward-only en Bloques B (retry/DLQ), C (CB/RL), D (cache). Documentado aquí para trazabilidad del checklist.