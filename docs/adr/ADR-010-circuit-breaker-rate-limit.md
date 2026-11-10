# ADR-010 — Circuit Breaker + Rate Limit in-app

## Contexto
El checklist de resiliencia exige Circuit Breaker (CB) y Rate Limiting (RL). El persister:
- Escribe en MongoDB (latencia variable, riesgo de cascada si Mongo se degrada)
- Sirve lecturas HTTP internas (Orquestador puede hacer ráfagas)
- Corre en réplicas horizontales (mismo consumer group Redis)

Opciones evaluadas:
| Opción | Ventaja | Desventaja |
|---|---|---|
| Traefik middleware (RL/CB en edge) | Centralizado, cero código | Requiere Traefik en este compose (no vive aquí), configuración dinámica, no protege Mongo directamente |
| Librería externa (resilience4j, go-breaker, etc.) | Feature-complete | Dependencia pesada, curva aprendizaje |
| **In-app primitivo (este repo)** | Control total, sin deps extra, protege Mongo donde falla | Código propio que mantener |

## Decisión
**Implementar CB y RL in-app** como primitivas ligeras en este repo:

### Circuit Breaker (`internal/adapters/mongodb/circuitbreaker.go`)
- Estados: `closed` → `open` (tras `failureThreshold` errores) → `half-open` (tras `timeout`) → `closed` (tras `successThreshold` éxitos)
- Envuelve `domain.DocumentRepository` via `CircuitBreakerRepository` (decorator)
- Defaults: `failureThreshold=5`, `successThreshold=2`, `timeout=30s` (configurables por env: `CB_FAILURES`, `CB_TIMEOUT`)
- Fail-open en `Execute`: error = log + delega; éxito = reset contador
- `ErrCircuitOpen` mapeado a **500 genérico** (ver ADR-011 para cache; no se añade código 503 nuevo)

### Rate Limiter (`internal/api/middleware/ratelimit.go`)
- Token bucket por IP (`c.ClientIP()`) usando `golang.org/x/time/rate`
- Map concurrente + cleanup goroutine (cada 5 min elimina limiters con tokens llenos)
- `RateLimit(rps, burst)` middleware Gin; `rps=0` o `burst=0` → deshabilitado
- Respuesta 429 con envelope `{"code":"rate_limited","message":"rate limit exceeded"}`
- Config por env: `RATE_LIMIT_RPS` (default 100), `RATE_LIMIT_BURST` (default 200)
- Montado en router condicional: `NewRouter(handler, rps, burst)`

## Consecuencias
- **Positivo**: protege Mongo en escrituras (CB) y evita abuso de lecturas (RL); cero deps de infra; configurable por env; testable unitario.
- **Negativo**: código propio (riesgo bugs); CB no comparte estado entre réplicas (cada instancia tiene su CB — aceptable, Mongo es el bottleneck compartido); RL por IP no por tenant (MVP).
- **Neutro**: no cambia API ni contratos; health no incluye estado CB (intencional).

## Alternativas evaluadas
| Alternativa | Por qué se descarta |
|---|---|
| Traefik `rateLimit`/`circuitBreaker` middlewares | Traefik no está en este compose; labels `int-ratelimit@file`/`int-cb@file` en `docker-compose.app.yml:24` son phantom (no definidos); mover Traefik aquí rompe aislamiento ecosistema. |
| `sony/gobreaker` / `juju/ratelimit` | Dependencias extra; APIs más complejas de lo necesario; primitivas propias < 100 líneas cubren el caso. |
| Redis-based RL/CB (distribuido) | Overhead Redis + complejidad; instancias ya protegen Mongo localmente; RL por IP local suficiente para MVP. |

## Estado
Aceptado. Implementado en Bloque C (commits `83a6705` RL mapping, `b39d954` config, `f23e0e4` middleware, `0fce47c` CB).