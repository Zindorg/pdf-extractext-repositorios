# ADR-009 — Agente Consul client para registro en servidor del ecosistema

## Contexto
El ecosistema provee un **único servidor Consul** (`servicios/consul`, datacenter `dc-converter`, nodo `ds`) en la red `mired`. El Orquestador se auto-registra vía HTTP API (`PUT /v1/agent/service/register`). El Persister debe registrarse en **ese mismo servidor** para ser descubrible por Traefik y otros servicios.

## Decisión
Levantar un **agente Consul en modo client** (`docker-compose.consul.yml`, red `mired`) que:
- Se une al servidor vía `retry_join = ["consul"]` (DNS en `mired`)
- Datacenter `dc-converter`
- Registra el servicio `persister` vía HCL estático (`consul/config/persister.hcl`) con check HTTP `/api/v1/health` c/10s
- Opt-in vía `make compose-consul`

### Detalles de configuración
**`consul/config/client.hcl`**:
```hcl
datacenter = "dc-converter"
node_name  = "persister-consul-client"
server     = false
retry_join = ["consul"]
data_dir   = "/consul/data"
client_addr = "0.0.0.0"
bind_addr   = "0.0.0.0"
disable_update_check = true
```

**`consul/config/persister.hcl`** (sin cambios):
```hcl
service {
  name    = "persister"
  address = "persister"
  port    = 8083
  tags    = ["documents","internal","v1"]
  meta    = { role = "persister" }
  check {
    id        = "persister-health"
    name      = "persister /api/v1/health"
    http      = "http://persister:8083/api/v1/health"
    interval  = "10s"
    timeout   = "3s"
  }
}
```

### ACLs
Hoy `default_policy: allow` → no requiere token. Cuando el ecosistema pase a `deny`, se crearán tokens por servicio con policy mínima (`service "persister" { policy = "write" }`).

### Gossip encryption
No configurado hoy. Si se activa, se genera con `consul keygen` y se distribuye por el mismo canal de secretos.

## Consecuencias
- **Positivo**: el servicio `persister` aparece en el catálogo central (`servicios/consul`); Traefik y Orquestador lo descubren ahí; health checks los hace el agente local (resiliente).
- **Negativo**: un contenedor extra (~90 MB) y un health check periódico contra `/health`.
- **Neutro**: no cambia la API, el Dockerfile ni la imagen del persister.

## Alternativas evaluadas
| Alternativa | Por qué se descarta |
|---|---|
| Self-registration vía HTTP API en `cmd/server` (como hace el Orquestador) | Requiere dependencia Go + lógica de deregister; health check dependiente del proceso vivo; si crashea sin deregister, queda "passing" fantasma. |
| Servidor Consul standalone (lo que había antes) | Crea catálogo separado; el Orquestador no ve el servicio; desperdicia recursos. |
| `--providers.consul` en Traefik del ecosistema | Cambio fuera de alcance; no resuelve el registro del servicio. |

## Estado
Aceptado. Implementado como compose opt-in (`make compose-consul`).

## Actualización 2026-11-18 — estado real del código
El código implementa **auto-registro por HTTP** (`internal/adapters/consul/registrar.go`, arrancado desde `cmd/server/main.go`): si `CONSUL_HTTP_ADDR` está definido, el servicio se registra al iniciar con ID `persister-<hostname>`, reintenta cada 5 s hasta lograrlo, usa `CONSUL_HTTP_TOKEN` (cabecera `X-Consul-Token`) y se desregistra al apagar. El check HTTP a `/api/v1/health` corre cada 10 s y Consul retira el servicio por sí solo tras 5 minutos en estado crítico (`DeregisterCriticalServiceAfter`), lo que mitiga el riesgo de "passing fantasma" descrito en la tabla de alternativas.

El agente client de la **Decisión** sigue documentado como alternativa opt-in (`make compose-consul`), pero no es lo que corre por defecto. Pendiente de decidir: mantener ambos caminos o elegir uno y retirar el otro; verificar además que `docker-compose.consul.yml` y `consul/config/` existan en el repositorio.