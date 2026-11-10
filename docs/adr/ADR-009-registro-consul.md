# ADR-009 — Registro del servicio en Consul (catálogo + health check)

## Contexto
El microservicio `persister` se comunica con el Orquestador vía DNS de Docker (`http://persister:8083`) en la red `mired`. Traefik (en el repo del ecosistema) descubre el servicio mediante labels de Docker (`traefik.enable=true`, `Host(persister.internal)`). Se quiere un catálogo de servicios independiente para observabilidad y descubrimiento alternativo.

## Decisión
Añadir un agente Consul single-node en `docker-compose.consul.yml` (red `mired`) que registre estáticamente el servicio `persister` con un health check HTTP contra `/api/v1/health`. El registro se carga al arrancar el agente vía archivo HCL en `consul/config/persister.hcl`.

- **No hay self-registration en el código Go**: la app distroless no tiene shell ni capacidad de escribir en Consul. El agente hace el check.
- **No se eliminan los labels de Traefik**: siguen siendo necesarios para que Traefik del ecosistema rutee `persister.internal`. Para quitar los labels haría falta configurar `--providers.consul` en el Traefik del ecosistema (otro repo).
- **No reemplaza el DNS de Docker**: el tráfico Orquestador→Persister sigue yendo por `http://persister:8083`.
- **El agente corre en modo server con `bootstrap_expect=1`** (single-node, datos persistidos en volumen) en lugar de `-dev`, para que el catálogo sobreviva a reinicios.
- **Check cada 10 s**: el agente hace `GET http://persister:8083/api/v1/health` (ping real a Mongo + Redis). Si la app está caída, el servicio queda en estado `critical` (no se desregistra).

## Consecuencias
- Positivo: catálogo consultable en `http://127.0.0.1:8500/v1/health/service/persister` y UI en `http://127.0.0.1:8500/ui`. El Orquestador puede resolver la dirección por Consul si se migra a ese descubrimiento.
- Negativo: un contenedor extra (~90 MB) y un health check periódico que golpea `/health` (Mongo+Redis).
- Neutro: no cambia la API, el Dockerfile ni la imagen del persister.

## Alternativas evaluadas
| Alternativa | Por qué se descarta |
|---|---|
| Self-registration vía API en `cmd/server` | Requiere dependencia Go / HTTP client en distroless; más código; falla si Consul no está al arrancar. |
| `consul-template` sidecar | Otro contenedor; complejidad de templates; no aporta nada sobre config estático. |
| `--providers.consul` en Traefik | Cambio en el repo del ecosistema (fuera de alcance); quita los labels pero no el DNS de Docker. |
| Solo `-dev` (en memoria) | Pierde el catálogo al reiniciar; no hay persistencia del estado del servicio. |

## Estado
Aceptado. Implementado como compose opt-in (`make compose-consul`).