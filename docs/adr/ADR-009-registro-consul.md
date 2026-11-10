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