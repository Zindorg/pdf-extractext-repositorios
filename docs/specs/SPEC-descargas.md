# SPEC — Lectura y descargas HTTP (Slice 7)

> Documento de especificación para conectar los endpoints GET de lectura y
> descarga que hoy responden `501` en `internal/api`.
> Fuente de contrato: `docs/proyect information of orquestador/ARCHITECTURE_REPOSITORIES.md`
> (§10 API interna) + `delta-persister.md` (delta 12: `summary_pending`),
> reflejado en `ARCHITECTURE.md` §10.
> Proceso conducido con TDD (rojo → verde), por fases con revisión del usuario.

## 1. Objetivo y alcance

Implementar en `internal/api` los cinco handlers pendientes:

| Método | Ruta | Handler |
|---|---|---|
| GET | `/documents/{document_id}` | `GetByDocumentID` |
| GET | `/documents/checksum/{checksum}` | `GetByChecksum` |
| GET | `/documents` | `List` |
| GET | `/documents/{document_id}/download/original` | `DownloadOriginal` |
| GET | `/documents/{document_id}/download/summary` | `DownloadSummary` |

**Sobre el drift doc–código detectado**: `README.md` y `ARCHITECTURE.md` §10
declaraban "lectura por `document_id`/`checksum`, listado … implementados", pero
los handlers respondían `501`. Este slice los conecta usando los use cases ya
existentes y reconcilia las docs con el estado real.

**Fuera de alcance**: request-ID (delta 7 diferido), auth, swagger, smoke e2e
contra compose (etapa final), búsqueda full-text, `POST /documents` (no existe
por diseño, ADR-001).

## 2. Decisiones de este slice

| # | Decisión | Justificación |
|---|---|---|
| D1 | El `409` de `download/summary` usa `code: summary_pending` | Delta 12 del Orquestador, diferido a este slice en ADR-007. Se aplica en la **única** tabla `errorMappings` (`SUMMARY_NOT_READY` → `SUMMARY_PENDING`) y se renombra el sentinel `ErrSummaryNotReady` → `ErrSummaryPending` por coherencia. |
| D2 | `Content-Type` de descargas: `text/plain; charset=utf-8` | Contrato §10: éxito `text/plain`. |
| D3 | `Content-Disposition`: `attachment; filename*=UTF-8''…` (RFC 5987) | Filename del original = `Metadata.Filename` (fallback `document-{id}.txt`); filename del resumen = `summary-{document_id}.txt`. Codificación percent de la cadena UTF-8 (`url.PathEscape`). |
| D4 | Errores de **validación HTTP** (query de `List`, ids vacíos) → `400` con `code: BAD_REQUEST` | No son errores de dominio: no pasan por `errorMappings` (esa tabla es solo para errores de dominio). Envelope estándar `{"code","message","details"}`. |
| D5 | `page`/`page_size` no numéricos → `400`; vacíos → defaults del domain | Contrato `400`; la normalización de defaults ya vive en el servicio. |
| D6 | `status` inválido (`≠ PENDING|COMPLETED`) y `created_from`/`created_to` no RFC3339 → `400` | Evitar que el repo reciba filtros malformados. |
| D7 | Soft-deleted → `404` en todos los endpoints (incl. descargas) | Regla única de visibilidad del servicio (`notFoundIfDeleted`). |
| D8 | Resumen descargable solo si `IsCompleted()`; si no → `ErrSummaryPending` (409) | Contrato `409` aún `PENDING`. |

## 3. Contrato por endpoint

Envelope de error consistente: `{"code":"…","message":"…","details":{…}}`.

| Ruta | Éxito | Errores |
|---|---|---|
| `/documents/{document_id}` | `200` + `DocumentResponse` | `400` id vacío · `404` |
| `/documents/checksum/{checksum}` | `200` + `DocumentResponse` | `400` checksum vacío · `404` |
| `/documents` | `200` `{items,total,page,page_size}` | `400` query inválida (D4–D6) |
| `/documents/{id}/download/original` | `200` texto plano + `Content-Disposition` | `400` · `404` |
| `/documents/{id}/download/summary` | `200` texto plano + `Content-Disposition` | `400` · `404` · `409` `summary_pending` |

`DocumentResponse` ya existe (`internal/api/dto.go`): expone `processing_time_ms`
(= `extraction_time_ms + summary_time_ms`), `metadata`, `summary`; nunca el `_id`
interno.

Listado excluye soft-deleted por defecto (`IncludeDeleted=false`); el servicio y
el repo ya soportan `status`, `filename`, `created_from`, `created_to` y
`include_deleted`.

## 4. Comportamiento del esqueleto a reemplazar

- `notImplemented(c)` (501) en `handlers.go` para `GetByDocumentID`,
  `GetByChecksum`, `List`, `DownloadOriginal`, `DownloadSummary`.
- Guard-tests de skeleton que se **reemplazan** por tests conductuales:
  `TestBusinessEndpointsNotImplemented` (`handlers_test.go`) y
  `TestGetByDocumentIDNotImplemented` (`router_test.go`).

## 5. Criterios de aceptación (GREEN)

Tests unitarios de router+handler con `testutil.MemoryRepository` + servicio real:

1. `GET /documents/{id}` → `200` con todos los campos del DTO; `404` para
   desconocido y soft-deleted.
2. `GET /documents/checksum/{checksum}` → `200`; `404`.
3. `GET /documents` → `200` `{items,total,page,page_size}`, paginación default 20,
   excluye deleted, filtra `status`; `400` ante `status` inválido o
   `created_from` malformado.
4. `GET /download/original` → `200`, cuerpo = texto extraído, header
   `Content-Disposition: attachment; filename*=UTF-8''…`; `404`.
5. `GET /download/summary` → `200` con `summary-{document_id}.txt` en el header;
   `409` con `{"code":"summary_pending"}` si el documento está `PENDING`; `404`.

## 6. Verificación obligatoria

```sh
make lint
go test ./...
go vet -tags integration ./...
go mod tidy -diff
```

Los `docker-compose.*` no se tocan en este slice.