// Package consul registra el persister en el servidor Consul del ecosistema
// vía HTTP API (self-registration), igual que el Orquestador.
// Usa solo la librería estándar: no agrega dependencias a go.mod.
package consul

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	serviceName   = "persister"
	retryInterval = 5 * time.Second
)

// Registrar registra/desregistra el servicio. Configuración por entorno:
// CONSUL_HTTP_ADDR, CONSUL_HTTP_TOKEN y SERVICE_ADDRESS.
type Registrar struct {
	addr    string
	token   string
	address string
	port    int
	id      string
	client  *http.Client
}

// FromEnv arma el Registrar a partir de las variables de entorno.
func FromEnv(httpPort string) *Registrar {
	port, _ := strconv.Atoi(httpPort)
	host, _ := os.Hostname()
	address := os.Getenv("SERVICE_ADDRESS")
	if address == "" {
		address = serviceName
	}
	return &Registrar{
		addr:    strings.TrimRight(os.Getenv("CONSUL_HTTP_ADDR"), "/"),
		token:   os.Getenv("CONSUL_HTTP_TOKEN"),
		address: address,
		port:    port,
		id:      serviceName + "-" + host,
		client:  &http.Client{Timeout: 5 * time.Second},
	}
}

// Enabled indica si hay a dónde registrarse (CONSUL_HTTP_ADDR definido).
func (r *Registrar) Enabled() bool { return r.addr != "" && r.port > 0 }

func (r *Registrar) payload() map[string]any {
	return map[string]any{
		"ID":      r.id,
		"Name":    serviceName,
		"Address": r.address,
		"Port":    r.port,
		"Tags":    []string{"documents", "internal", "v1"},
		"Meta":    map[string]string{"role": "persister"},
		"Check": map[string]any{
			"CheckID":  "persister-health",
			"Name":     "persister /api/v1/health",
			"HTTP":     fmt.Sprintf("http://%s:%d/api/v1/health", r.address, r.port),
			"Interval": "10s",
			"Timeout":  "3s",
			// Si queda crítico mucho tiempo, Consul lo desregistra solo.
			"DeregisterCriticalServiceAfter": "5m",
		},
	}
}

// Register reintenta hasta lograr el registro o hasta que ctx se cancele
// (Consul puede no estar listo cuando arranca el persister).
// Pensado para correr en una goroutine.
func (r *Registrar) Register(ctx context.Context) {
	for {
		err := r.do(ctx, http.MethodPut, "/v1/agent/service/register", r.payload())
		if err == nil {
			slog.Info("registrado en Consul", "id", r.id, "addr", r.addr)
			return
		}
		slog.Warn("registro en Consul falló, reintentando", "error", err)
		select {
		case <-ctx.Done():
			return
		case <-time.After(retryInterval):
		}
	}
}

// Deregister quita el servicio del catálogo (llamar al apagar).
func (r *Registrar) Deregister() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := r.do(ctx, http.MethodPut, "/v1/agent/service/deregister/"+r.id, nil); err != nil {
		slog.Warn("deregistro en Consul falló", "error", err)
		return
	}
	slog.Info("desregistrado de Consul", "id", r.id)
}

func (r *Registrar) do(ctx context.Context, method, path string, body any) error {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, r.addr+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if r.token != "" {
		req.Header.Set("X-Consul-Token", r.token)
	}
	resp, err := r.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("consul %s %s: %s: %s", method, path, resp.Status, bytes.TrimSpace(msg))
	}
	return nil
}
