# ADR-008 — Decisión sobre OpenAPI/Swagger

- Estado: **Aceptado** (2026-10-18)
- Contexto: Existe un candidato para añadir spec OpenAPI/Swagger para la API interna. El contrato HTTP ya está completamente documentado en `ARCHITECTURE.md` §4.
- Alcance: API interna del microservicio Persistidor (consumida únicamente por el Orquestador por red interna `mired`).

## Decisión

**No incluir OpenAPI/Swagger en MVP.**

Se descarta añadir `openapi.yaml`, Swagger UI u herramientas de generación de spec para este microservicio en la fase actual.

## Justificación

1. **API interna**: Solo el Orquestador accede a esta API por red interna (`http://persister:8083`), no está expuesta públicamente (Traefik no la enruta directamente).
2. **Contrato ya documentado**: El contrato está fielmente documentado y mantenido en `ARCHITECTURE.md` §4 (tabla completa de endpoints, códigos de error, notas de diseño, 400 no alcanzable, `summary_pending` 409).
3. **KISS/YAGNI**: No añade valor funcional; añade mantenimiento adicional para mantener la spec sincronizada con el código.
4. **Estado honesto**: El repo no tiene dependencias Swagger y no se han implementado handlers de UI. Añadirlo ahora sería código especulativo.
5. **Fácil de revertir**: Si el ecosistema requiere un spec OpenAPI en el futuro, se puede adoptar con un nuevo ADR y añadirlo de forma deliberada.

## Alternativas consideradas

- **Generar openapi.yaml mínimo**: Rechazado por YAGNI. El contrato ya está explícito en arquitectura.
- **Añadir Swagger UI**: Rechazado. No necesario para consumo interno entre microservicios controlados.

## Consecuencias

- El contrato queda documentado en `ARCHITECTURE.md` (fuente única honesta y alineada con el código).
- No se añaden dependencias nuevas relacionadas con OpenAPI.
- Esta decisión cierra explícitamente el punto pendiente de "decisión Swagger/OpenAPI".

## Referencias

- `ARCHITECTURE.md` §4 — Contrato HTTP interno `/api/v1`
- `ADR-007` — Todos los deltas aplicados, incluida la fila 7 (propagación de `X-Request-ID`)
