# Reglas del proyecto — obligatorias

> **Desde el 2026-10-07** toda modificación de este repositorio (humana o
> asistida por IA) **debe** cumplir los principios, reglas y proceso de este
> documento. Un cambio no se considera terminado sin pasar la
> [verificación obligatoria](#verificación-obligatoria).

## Principios de código (obligatorios)

| Principio | Regla dura en este repo | Skill (detalle) |
|---|---|---|
| **Clean Code** | Funciones **≤ 20 líneas de cuerpo** (regla única de tamaño, sin otros umbrales; ver [excepciones vigentes](#excepciones-vigentes-a-la-regla-de-tamaño)); nombres que revelan intención; cero código comentado; errores envueltos con `fmt.Errorf("capa: %w", err)` | [clean-code](.agents/skills/clean-code/SKILL.md) |
| **DRY** | Ningún patrón duplicado: repetición → helper/tabla/constante compartida | [dry-refactoring](.agents/skills/dry-refactoring/SKILL.md) |
| **KISS** | La solución más simple que cumpla; cero abstracciones especulativas | [kiss](.agents/skills/kiss/SKILL.md) |
| **SOLID** | Interfaces consumer-side, una responsabilidad por unidad, dependencias hacia adentro; los decoradores se tipan contra el puerto (`domain.DocumentRepository`), nunca contra un adaptador concreto | [solid](.agents/skills/solid/SKILL.md) |
| **YAGNI** | Ni código, ni config, ni flags "por si acaso"; sin símbolos que ningún código de producción use (los tests no justifican su existencia) | [yagni](.agents/skills/yagni/SKILL.md) |

### Excepciones vigentes a la regla de tamaño

Medición del 2026-11-18 (cuerpo sin firma ni llave de cierre; cuentan
comentarios y líneas en blanco). **No se agregan excepciones nuevas**: cada
función de esta lista se acorta al tocarla y se retira de la lista.

`trimStream`, `runCleanupDLQ`, `main`, `AccessLog`, `runRestoreByChecksum`,
`loadNumbers`, `RateLimit`, `registrar.do`, `startMongo`, `handlers.List`,
`recordResult`, `OnOriginal`, `serveHTTP`, `NewRouter`, `parseListFilter`.

## Reglas canónicas de este repo

Decisiones de la auditoría de calidad del 2026-10-07 (actualizadas el
2026-11-18): son la línea de base, no se re-litigan; si alguna deja de
encajar, se cambia con una decisión explícita y se actualiza este archivo.

- **Errores de dominio → HTTP**: única tabla `errorMappings` en
  `internal/api/errors.go` — añadir una fila, nunca un `switch` nuevo.
- **Paginación**: constantes `domain.DefaultPageSize` / `domain.MaxPageSize`;
  el servicio normaliza, el repositorio recibe valores ya válidos.
- **Config**: env vars numéricas/duración con el genérico `envParse[T]`
  (`internal/config`); los strings con `env`.
- **Índices Mongo**: nombres en las constantes `idx*` de
  `internal/adapters/mongodb/ensure_indexes.go`, compartidas con la
  clasificación de errores 11000.
- **Fixtures de test**: constructor único `internal/testutil` — compartido
  entre tests unitarios y de integración.
- **Dependencias (DIP)**: `internal/api` solo consume interfaces
  consumer-side (`DocumentUseCases`, `HealthChecker`); el wiring vive en el
  composition root (`cmd/server`).
- **Reloj**: `domain.Now()` (UTC) es la única fuente de marcas de tiempo que
  mutan documentos.
- **Docs honestas**: `README.md`, `ARCHITECTURE.md`, los **ADR**, los
  diagramas PUML y las **labels y configuración de los `docker-compose.*`**
  deben reflejar el estado real del código. Antes de cerrar un cambio se
  contrasta cada afirmación de los ADR y del README que el cambio toque
  (defaults, fail-open vs fail-fast, tipo de backoff, qué invalida qué)
  contra el código; nunca se documentan funciones que no existen.
- **Sin código inerte**: no se dejan sentinels, tipos, funciones ni errores
  que ningún código de producción use "para la próxima fase". Un símbolo
  solo referenciado por tests o por una tabla de mapeo se elimina (reemplaza
  la regla "Esqueleto pendiente" del 2026-10-07, ya sin objeto).
- **Secretos**: `.env.example` y los `docker-compose.*` solo llevan valores
  vacíos o de relleno, nunca credenciales ni tokens reales. Si un secreto
  llegó al repositorio, se rota (borrarlo del archivo no alcanza).
- **Tests de comportamiento**: los decoradores y adaptadores se prueban
  recorriendo el flujo real (miss → carga → guarda → hit), no sembrando el
  estado interno a mano. Cada garantía escrita en un ADR tiene al menos un
  test que falla si se rompe, y las políticas de resiliencia (cache, breaker,
  retry, recuperación del consumer) tienen su camino de error probado.

## Uso de IA

La salida de una IA es un **borrador**, no una fuente de verdad. Antes de
aceptarla:

1. Leer el diff completo y entender cada línea; no se acepta código que no se
   puede explicar.
2. Verificar versiones, flags y APIs contra la documentación oficial y contra
   `go.mod`; la IA inventa nombres que parecen reales.
3. Revisar los tests que genera: tienen que **fallar sin la implementación**
   (ver el rojo) y no deben limitarse a repetir lo que hace el código.
4. Correr la [verificación obligatoria](#verificación-obligatoria), incluidos
   los tests de integración y el escaneo de vulnerabilidades.
5. Contrastar el resultado con los ADR y con "Docs honestas".

## Proceso obligatorio

1. **Tests primero** para lógica nueva o bugs: rojo → verde → refactor
   ([test-driven-development](.agents/skills/test-driven-development/SKILL.md)).
   Si la feature no tiene spec, redactarla antes:
   [spec-driven-development](.agents/skills/spec-driven-development/SKILL.md).
2. **Revisión antes de merge**: aplicar
   [code-review-and-quality](.agents/skills/code-review-and-quality/SKILL.md)
   al diff y corregir los hallazgos bloqueantes.
3. **Docs en el mismo cambio**: si el cambio altera un comportamiento
   documentado (README, ARCHITECTURE, ADR, composes), se actualiza en el
   mismo commit.

## Verificación obligatoria

Ejecutar antes de dar por terminado cualquier cambio:

```sh
make lint                          # gofmt + go vet
go test ./...
make test-integration              # Mongo y Redis reales; FALLA si algún test se omite
make vuln                          # govulncheck: vulnerabilidades en dependencias
go mod tidy -diff
git grep -n -i -E "token=[0-9a-f-]{16,}" -- ":!AGENTS.md"   # sin secretos (debe no imprimir nada)

# solo si se toca un docker-compose.*:
docker compose -f docker-compose.mongo.yml config -q
docker compose -f docker-compose.redis.yml config -q
docker compose -f docker-compose.app.yml config -q
docker compose -f docker-compose.consul.yml config -q   # solo si el archivo existe en el repo
```

`make test-integration` necesita Mongo y Redis levantados
(`make compose-mongo compose-redis`). Todo debe quedar en verde; si
`make lint` formatea un archivo, incluirlo en el cambio.