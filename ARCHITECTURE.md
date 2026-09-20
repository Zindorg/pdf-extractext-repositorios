# Blueprint de Arquitectura — Microservicio de Persistencia de Documentos PDF

> Documento rector del proyecto. Mantener actualizado junto con el código.

## 1. Tipo de arquitectura

El proyecto tiene **dos niveles de arquitectura**, que conviene nombrar por separado:

| Nivel | Tipo de arquitectura |
|---|---|
| **Sistema** (todo el ecosistema) | **Arquitectura de Microservicios**, síncrona (REST/HTTP), con **API Gateway / Edge Router** (Traefik) y **data-per-service** (cada microservicio es dueño de su propia BD) |
| **Servicio** (dentro de este microservicio) | **Arquitectura Hexagonal / Puertos y Adaptadores** (variante simplificada de Clean Architecture), de 4 capas concéntricas con **núcleo de dominio puro** |

Patrón central de diseño: **Repository pattern** (puerto en dominio + adaptador MongoDB).

---

## 2. Stack

| Capa | Elección | Motivo |
|---|---|---|
| Lenguaje | **Go 1.24+** | Escalabilidad futura: binario estático, bajo consumo por request, alta concurrencia nativa |
| HTTP | **Gin** | Binding, validación y rutas listas; el más usado para REST en Go |
| BD | **MongoDB 7** | BD del ecosistema original; dedup garantizado por índice único parcial |
| Driver | `go.mongodb.org/mongo-driver/v2` | Driver oficial de MongoDB para Go |
| Migraciones | — | No aplica: índices asegurados por la app al arrancar (`ensureIndexes`) |
| ID | ObjectId (driver) | `_id` se expone como string hex; no sale del adaptador |
| Validación | `go-playground/validator` | Binding + reglas en DTOs |
| Docs API | `swaggo/swag` | UI en `/swagger/index.html` para consumidores externos |
| Tests | stdlib `testing` + `httptest` + `testify/mock` | TDD: unit, integración, contrato, e2e |
| Deployment | Docker multi-stage + Compose + **Traefik** | Routing por URL (nunca por IP) |

---

## 3. Vista de sistema (contexto)

```
                ┌───────────────────────────────────────────────┐
                │  SISTEMAS EXTERNOS / OTROS MICROSERVICIOS     │
                │  (orquestador de resúmenes, equipos externos) │
                └───────────────────────┬───────────────────────┘
                                        │ HTTPS — REST / JSON
                           ┌────────────▼────────────┐
                           │  TRAEFIK  (v3)          │  Edge Router
                           │  descubre contenedores, │  routing por URL
                           │  rule: Host(tu.dominio) │  (nunca por IP)
                           └────────────┬────────────┘
                                        │  /api/v1
                    ┌───────────────────▼───────────────────┐
                    │  MS PERSISTENCIA DE DOCUMENTOS        │  ◄── ESTE PROYECTO
                    │  Go + Gin  · puerto 8080              │      binario estático
                    └───────────────┬───────────┬───────────┘
                                    │ driver (BSON) │ health
                       ┌────────────▼────┐
                       │  MongoDB 7      │  documents
                       │  (solo suya)    │  data-per-service
                       └─────────────────┘
```

Comunicación **síncrona por pedido**: el orquestador pide el texto completo (`GET /documents/{id}`) y este servicio lo entrega; el resumen lo realiza otro microservicio (fuera de este alcance). No hay eventos ni colas en el alcance actual.

---

## 4. Vista interna (componentes y flujo de dependencia)

```
                ┌──────────────────────────────────────────────┐
                │  CAPA 1 · TRANSPORTE   internal/api           │
                │  Gin Router · Handlers · DTOs + validación    │
                │  Middleware (logger, recovery, request-id)    │
                │  Swagger UI (/swagger/index.html)             │
                └───────────────────┬──────────────────────────┘
                                    │  invoca casos de uso
                ┌───────────────────▼──────────────────────────┐
                │  CAPA 2 · APLICACIÓN  internal/application    │
                │  Service: create (dedup), get, list,          │
                │  soft-delete, restore, download               │
                └───────────────────┬──────────────────────────┘
                                    │  usa el PUERTO
                ┌───────────────────▼──────────────────────────┐
                │  CAPA 3 · DOMINIO  internal/domain            │  núcleo puro:
                │  Document (entidad) · errores de dominio      │  cero dependencias
                │  interfaz Repository (port)                   │  de framework/BD
                └───────────────────┬──────────────────────────┘
                                    ▲  implementa (adapter)
                ┌──────────────────────────────────────────────┐
                │  CAPA 4 · ADAPTADOR  MongoDB                 │
                │  repository (driver oficial) · mapper BSON   │
                │  ensure_indexes (índice único parcial)       │
                └───────────────────┬──────────────────────────┘
                                    │  BSON
                        ┌───────────▼───────────┐
                        │  MongoDB 7            │
                        └───────────────────────┘
```

**Regla de dependencia (la única regla del Hexagonal): todo apunta hacia adentro.** El dominio no conoce Gin, ni el driver de MongoDB, ni HTTP. El adaptador MongoDB implementa el *port* `Repository`. La inversión se resuelve en el **composition root** (`cmd/server/main.go`), que construye de afuera hacia adentro.

---

## 5. Estructura del proyecto

```
pdf-extractext-repositorios/
├── cmd/server/main.go            # composition root: wiring, graceful shutdown
├── internal/
│   ├── config/config.go          # env → struct tipado
│   ├── domain/
│   │   ├── model.go              # Document, CreateDocumentParams
│   │   ├── repository.go         # interfaz Repository (puerto)
│   │   └── errors.go             # ErrNotFound, ErrDuplicateChecksum, ErrRestoreConflict
│   ├── application/service.go    # reglas de negocio: dedup, soft-delete, restore, list
│   ├── adapters/mongodb/
│   │   ├── repository.go         # impl con el driver oficial
│   │   ├── mapper.go             # BSON ↔ domain.Document
│   │   └── ensure_indexes.go     # índice único parcial + filename + created_at
│   └── api/
│       ├── router.go             # gin, grupos, middleware, swagger
│       ├── handlers.go           # handlers HTTP
│       ├── dto.go                # request/response DTOs + validación
│       └── errors.go             # mapping dominio → HTTP + envelope
├── Dockerfile                    # multi-stage → distroless
├── docker-compose.yml            # mongo + app + traefik
├── .env.example  .gitignore  Makefile  go.mod  go.sum  README.md  ARCHITECTURE.md
```

---

## 6. Componentes, responsabilidad y patrones

| Componente | Paquete | Responsabilidad | Patrón |
|---|---|---|---|
| Composition root | `cmd/server/main.go` | Wiring, config, conexión, graceful shutdown | inyección por constructor |
| Config | `internal/config` | Env → struct tipado | Settings/12-factor |
| Dominio | `internal/domain` | Entidad, port Repository, errores | Entidad + Puerto |
| Aplicación | `internal/application` | Reglas de negocio (dedup, 404, restore-conflict) | Service Layer / Casos de uso |
| Adapter | `internal/adapters/mongodb` | Persistencia real, índices (`CreateMany`), mapeo BSON↔dominio | Repository + Data Mapper |
| API | `internal/api` | Contratos HTTP, validación, docs | DTO, Handler |
| Infra | docker-compose | `mongo` · `app` · `traefik` | Índices al arranque, Edge Router |

---

## 7. Arquitectura de datos

### Modelo de dominio

```go
type Document struct {
    ID          string     `json:"id"`             // hex del ObjectId
    Checksum    string     `json:"checksum"`       // lowercase, 64 hex
    Filename    string     `json:"filename"`
    ContentType string     `json:"content_type"`
    TextContent string     `json:"text_content"`
    PageCount   *int       `json:"page_count"`
    FileSize    *int64     `json:"file_size"`
    DeletedAt   *time.Time `json:"deleted_at"`
    CreatedAt   time.Time  `json:"created_at"`
    UpdatedAt   time.Time  `json:"updated_at"`
}
```

### Documento BSON (colección `documents`)

```go
type persistedDocument struct {
    ID          primitive.ObjectID `bson:"_id"`
    Checksum    string             `bson:"checksum"`
    Filename    string             `bson:"filename"`
    ContentType string             `bson:"content_type"`
    TextContent string             `bson:"text_content"`
    PageCount   *int               `bson:"page_count"`
    FileSize    *int64             `bson:"file_size"`
    DeletedAt   *time.Time         `bson:"deleted_at"`
    CreatedAt   time.Time          `bson:"created_at"`
    UpdatedAt   time.Time          `bson:"updated_at"`
}
```

### Índices (creados por `ensureIndexes` al arrancar)

```go
{
  key:   { "checksum": 1 },
  name:  "uq_checksum_active",
  unique: true,
  partialFilterExpression: { "deleted_at": null },  // solo indexa activos
}
{ key: { "filename": 1 },    name: "ix_filename" }
{ key: { "created_at": -1 }, name: "ix_created_at" }
```

### Reglas y decisiones sobre los datos

- **Un solo agregado**: `Document`. Sin relaciones entre documentos.
- **Deduplicación a nivel de BD**: **índice único parcial** sobre `checksum` (filter `deleted_at: null`) → garantía fuerte (no solo de aplicación). Insertar un checksum ya activo → error del driver código `11000` → `409 Conflict`.
- **Soft delete** como estado (`deleted_at`), no borrado físico. Un doc borrado sale del índice parcial → **libera** su checksum: permite re-cargar el mismo archivo.
- **Restore**: re-activar (unset `deleted_at`) colisiona en el índice único con un nuevo activo ya re-cargado → `11000` → `ErrRestoreConflict` → `409`. El índice es la fuente de verdad, sin carreras.
- **Mapeo**: el adaptador convierte BSON ↔ `Document`; `ObjectId` y tipos BSON nunca salen hacia la API (expone `id` como string hex).

---

## 8. API — Contrato base `/api/v1`

| Método | Ruta | Request | Éxito | Errores |
|---|---|---|---|---|
| POST | `/documents` | `CreateDocumentRequest` | `201` + doc | `400` validación · `409` checksum activo (con doc existente) · `413` texto muy grande |
| GET | `/documents/{id}` | — | `200` + doc | `400` · `404` |
| GET | `/documents/{id}/download` | — | `200` `text/plain` + `Content-Disposition: attachment` (RFC 5987) | `400` · `404` |
| GET | `/documents` | `page`, `page_size`, `filename`, `created_from`, `created_to`, `include_deleted` | `200` `{items, total, page, page_size}` | `400` |
| DELETE | `/documents/{id}` | — | `204` (soft) | `400` · `404` |
| POST | `/documents/{id}/restore` | — | `204` | `400` · `404` · `409` conflicto |
| GET | `/health` | — | `200` `{status:"ok"}` | `503` BD caída |

Envelope de error consistente: `{"code": "...", "message": "...", "details": {...}}` + `X-Request-ID`. Paginación default 20, máx 100. Listado excluye soft-deleted por defecto.

---

## 9. Cross-cutting concerns

- **Config**: env vars con defaults (`HTTP_PORT`, `MONGODB_URI`, `MONGODB_DATABASE`, `MAX_TEXT_BYTES`, `LOG_LEVEL`) — 12-factor.
- **Errores**: errores de dominio → HTTP (`404/409/400`) vía mapeo central en `internal/api/errors.go`.
- **Observabilidad**: middleware de request-ID (`X-Request-ID`), logger de acceso, `/health` con ping a BD para Traefik.
- **Seguridad**: límite de tamaño de cuerpo (`MAX_TEXT_BYTES`) contra abuso; sin auth en MVP (red interna) — extensible con middleware de API key/JWT.
- **Validación**: en DTOs (binding + validator) para el borde; reglas de negocio en el servicio.

---

## 10. Arquitectura de despliegue

- `Dockerfile` multi-stage: `golang:1.24-alpine` (build) → `distroless` nonroot (binario ~10MB, sin shell).
- `docker-compose.yml`: servicios `mongo` (healthcheck) → `app` → `traefik`. Sin servicio de migraciones: la app crea los índices al arrancar.

```yaml
mongo:
  image: mongo:7
  volumes:
    - mongodata:/data/db
  healthcheck:
    test: ["CMD", "mongosh", "--quiet", "--eval", "db.adminCommand('ping').ok"]
    interval: 5s
    timeout: 5s
    retries: 10
```
- **Traefik v3** como Edge Router: descubre contenedores por labels y enruta por **Host/URL**:

```yaml
traefik.http.routers.documents.rule=Host(`docs.localhost`)
traefik.http.services.documents.loadbalancer.server.port=8080
```

- Cada microservicio del ecosistema = su contenedor + su BD (data-per-service, acoplamiento desacoplado).

---

## 11. Arquitectura de testing

Pirámide de pruebas, todo conducido por **TDD** (rojo → verde):

| Capa | Tipo | Cobertura |
|---|---|---|
| Service | Unit (repo fake con `testify/mock`) | Create ok, checksum duplicado → 409, GET borrado → 404, soft-delete inexistente → 404, restore conflicto → 409, list con filtros/paginación/total |
| Handlers | `httptest` | Status codes, shapes JSON, `Content-Disposition`, validación 400, envelope de error |
| Adapter Mongo | Integración (MongoDB real en Docker) | `11000` duplicado, soft-delete → re-insert mismo checksum OK, restore bloqueado con nuevo activo, orden/paginación |
| e2e | Smoke con curl contra compose | Flujo completo por la URL de Traefik |

Comandos: `make test`, `make test-integration`, `go vet ./...`.

---

## 12. Extensiones y evolución

| Futuro cambio | Dónde entra | Sin tocar el resto |
|---|---|---|
| Búsqueda full-text del texto | **text index** o **Atlas Search** + método nuevo en Repository (solo adapter/dominio) | API y service casi intactos |
| Autenticación | Middleware nuevo en `internal/api` | Dominio intacto |
| Cambiar de BD (p.ej. PostgreSQL) | Nuevo adapter que implemente el mismo port | Service/API intactos |
| Nuevos endpoints | Handler + caso de uso nuevo en service | Arquitectura sin fuerzas |
| Alta carga → sharding | Sharded cluster / Atlas a nivel de DB | App sin cambios (solo `MONGODB_URI`) |

---

## 13. Decisiones de arquitectura (ADR)

- **ADR-001 — Go + Gin**: escalabilidad futura, binario único, bajo consumo por request.
- **ADR-002 — MongoDB sobre PostgreSQL**: se adopta la BD del ecosistema original. Garantías clave mantenidas con el **índice único parcial** (dedup a nivel de BD, soft-delete libera checksum, restore conflictivo → 409). PostgreSQL documentado como alternativa considerada: schema fijo, ACID y full-text nativo (GIN) — no aplica por acoplamiento con el stack original.
- **ADR-003 — Hexagonal + Repository**: la "costura" se abstrae en el puerto; un solo adaptador optimizado (ni ORM multi-DB ni dominio acoplado).
- **ADR-004 — Soft-delete libera checksum**: borrado ≠ ocupado; restore con conflicto → 409.
- **ADR-005 — Traefik como Edge Router**: sincronismo REST, routing por URL, sin discovery propio.
- **ADR-006 — Sin full-text en el alcance**: el texto viaja completo al orquestador; se agrega vía text index / Atlas Search si se necesita.

---

## 14. Gobernanza de arquitectura

- Este documento es el **estándar rector**: todo código nuevo debe respetar el flujo de dependencia (§4).
- Implementación conducida con TDD; revisión final con el skill `code-review-and-quality`.
- Al agregar funcionalidad nueva: ubicar por capa (§5), respetar puertos/adaptadores, y actualizar esta guía y los ADR si cambia una decisión.

---

*Generado el 2026-09-18 como blueprint inicial de un proyecto greenfield. Actualizar a medida que la implementación evolucione.*