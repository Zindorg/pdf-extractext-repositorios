# Reglas del proyecto — obligatorias

> **Desde el 2026-10-07** toda modificación de este repositorio (humana o
> asistida por IA) **debe** cumplir los principios, reglas y proceso de este
> documento. Un cambio no se considera terminado sin pasar la
> [verificación obligatoria](#verificación-obligatoria).

## Principios de código (obligatorios)

| Principio | Regla dura en este repo | Skill (detalle) |
|---|---|---|
| **Clean Code** | Funciones **≤ 20 líneas** (regla única de tamaño, sin otros umbrales); nombres que revelan intención; cero código comentado; errores envueltos con `fmt.Errorf("capa: %w", err)` | [clean-code](.agents/skills/clean-code/SKILL.md) |
| **DRY** | Ningún patrón duplicado: repetición → helper/tabla/constante compartida | [dry-refactoring](.agents/skills/dry-refactoring/SKILL.md) |
| **KISS** | La solución más simple que cumpla; cero abstracciones especulativas | [kiss](.agents/skills/kiss/SKILL.md) |
| **SOLID** | Interfaces consumer-side, una responsabilidad por unidad, dependencias hacia adentro | [solid](.agents/skills/solid/SKILL.md) |
| **YAGNI** | Ni código, ni config, ni flags "por si acaso" | [yagni](.agents/skills/yagni/SKILL.md) |

## Reglas canónicas de este repo

Decisiones de la auditoría de calidad del 2026-10-07: son la línea de base,
no se re-litigan; si alguna deja de encajar, se cambia con una decisión
explícita y se actualiza este archivo.

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
- **Docs honestas**: `README.md`, `ARCHITECTURE.md` y los diagramas PUML
  deben reflejar el estado real del código (endpoints `501` y pendientes
  marcados como tales); nunca documentar funciones que no existen.
- **Esqueleto pendiente**: mientras la fase de lógica no lo conecte, se
  respetan los marcadores (`501`, `_ = consumer`, sentinels/vars inertes);
  no se simula implementación.

## Proceso obligatorio

1. **Tests primero** para lógica nueva o bugs: rojo → verde → refactor
   ([test-driven-development](.agents/skills/test-driven-development/SKILL.md)).
   Si la feature no tiene spec, redactarla antes:
   [spec-driven-development](.agents/skills/spec-driven-development/SKILL.md).
2. **Revisión antes de merge**: aplicar
   [code-review-and-quality](.agents/skills/code-review-and-quality/SKILL.md)
   al diff y corregir los hallazgos bloqueantes.

## Verificación obligatoria

Ejecutar antes de dar por terminado cualquier cambio:

```sh
make lint                          # gofmt + go vet
go test ./...
go vet -tags integration ./...     # compila los tests de integración
go mod tidy -diff

# solo si se toca un docker-compose.*:
docker compose -f docker-compose.mongo.yml config -q
docker compose -f docker-compose.redis.yml config -q
docker compose -f docker-compose.app.yml config -q
```

Todo debe quedar en verde; si `make lint` formatea un archivo, incluirlo en
el cambio.
