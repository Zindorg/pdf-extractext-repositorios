# ADR-011 — Cache Redis read-through

## Contexto
El checklist exige Cache como patrón de resiliencia. El persister sirve lecturas HTTP (`FindByDocumentID`, `FindByChecksum`, `List`) que golpean Mongo en cada request. Bajo carga de réplicas, Mongo es bottleneck.

Opciones:
| Opción | Ventaja | Desventaja |
|---|---|---|
| Read-through decorator (este repo) | Transparente, fail-open, TTL acota staleness, sin invalidación compleja | Requiere Redis (ya existe), código propio |
| Write-through | Consistencia fuerte | Complejidad (invalidar en todas escrituras, transacciones) |
| Look-aside (manual en handlers) | Control explícito | Duplica lógica, rompe capa application |
| Traefik cache | Cero código | Solo GET estáticos, no API dinámica; Traefik no en este compose |

## Decisión
**Read-through cache** como decorator de `domain.DocumentRepository` (`internal/adapters/cache/CachedRepository`):

### Detalles
- **Keys**: `doc:{documentID}` y `docchk:{checksum}` (JSON del `Document` completo)
- **Reads cacheados**: `FindByDocumentID`, `FindByChecksum`
- **NO cacheado**: `List` (paginación + filtros dinámicos), `Insert`, `Complete`, `SoftDelete`, `Restore`
- **Invalidación (DEL)**: `Complete`, `SoftDelete`, `Restore` → borran `doc:{id}` y `docchk:{checksum}`; `Insert` invalida `docchk:{checksum}` (no warm de `doc:{id}`)
- **TTL**: `CACHE_TTL` env var (default 60s, `0` = disabled pass-through)
- **Fail-open**: error Redis → `slog.Warn` + delega al repo subyacente (Mongo protegido por CB)
- **Ordering**: `CachedRepository(CircuitBreakerRepository(MongoRepo))` — cache outermost
  - Reads: cache → miss → CB → Mongo (Redis errors no abren CB)
  - Writes: CB → Mongo → cache invalidation (si CB abierto, write falla y no invalida; staleness acotado por TTL)

### Config
- `CACHE_TTL` (duration, default `60s`, `0` = disabled)

### Tests
- Unitarios con miniredis (hit, miss, invalidación, TTL=0, fail-open)
- Integración con Redis/Mongo reales (`SKIP_INTEGRATION`)

## Consecuencias
- **Positivo**: descarga Mongo en lecturas repetidas; fail-open = resiliente a Redis caído; TTL acota ventana de staleness; transparente para service/handlers.
- **Negativo**: staleness eventual (hasta TTL) tras `Complete`/`SoftDelete`/`Restore` antes de invalidar; `Insert` no calienta cache (primer GET = miss); memoria Redis ~tamaño docs cacheados.
- **Neutro**: no cambia API, contratos, ni dominio; `List` siempre fresco.

## Alternativas evaluadas
| Alternativa | Por qué se descarta |
|---|---|
| Write-through | Requiere invalidar en `Insert` (warm) + transaccionalidad; complejidad > beneficio para reads mayoritarios. |
| Look-aside en handlers | Rompe capa application; duplica lógica de cache en cada handler. |
| Redis read-replica / cluster | Overhead infra; single Redis suficiente para MVP. |
| Traefik cache | Solo static assets; no API con query params/paginación. |

## Estado
Aceptado. Implementado en Bloque D (config `CACHE_TTL`, `cache/CachedRepository`, wiring en `main.go`).