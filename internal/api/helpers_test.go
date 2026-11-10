package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/marco/pdf-extractext-repositorios/internal/api"
	"github.com/marco/pdf-extractext-repositorios/internal/application"
	"github.com/marco/pdf-extractext-repositorios/internal/domain"
	"github.com/marco/pdf-extractext-repositorios/internal/testutil"
	"github.com/stretchr/testify/require"
)

// fakeRepo es un repo que no persiste nada y devuelve los errores que el
// test necesita inyectar en SoftDelete y Restore.
type fakeRepo struct {
	softDeleteErr error
	restoreErr    error
}

func (fakeRepo) Insert(context.Context, domain.Document) error { return nil }
func (fakeRepo) Complete(context.Context, string, string, int64) (*domain.Document, error) {
	return nil, nil
}
func (fakeRepo) FindByDocumentID(context.Context, string) (*domain.Document, error) {
	return nil, domain.ErrNotFound
}
func (fakeRepo) FindByChecksum(context.Context, string) (*domain.Document, error) {
	return nil, domain.ErrNotFound
}
func (fakeRepo) List(context.Context, domain.ListFilter, int, int) ([]domain.Document, int64, error) {
	return nil, 0, nil
}
func (r fakeRepo) SoftDelete(context.Context, string) error { return r.softDeleteErr }
func (r fakeRepo) Restore(context.Context, string) error    { return r.restoreErr }

type healthy struct{}

func (healthy) Ping() error { return nil }

type unhealthy struct{}

func (unhealthy) Ping() error { return errors.New("down") }

// newRouter monta la API sobre el service con el health check indicado.
func newRouter(service *application.DocumentService, health api.HealthChecker) *gin.Engine {
	return api.NewRouter(api.NewDocumentHandler(service, health), 0, 0)
}

// newTestRouter construye la API sobre el MemoryRepository compartido (testutil)
// con el DocumentService real, de modo que los tests ejercen handler + service.
func newTestRouter() (*gin.Engine, *application.DocumentService) {
	repo := testutil.NewMemoryRepository()
	service := application.NewDocumentService(repo)
	return newRouter(service, healthy{}), service
}

// seedPending inserta un documento PENDING vía el servicio.
func seedPending(t *testing.T, svc *application.DocumentService, id, checksum, text string) {
	t.Helper()
	seedWithStatus(t, svc, id, checksum, text, domain.SummaryPending)
}

// seedWithStatus inserta un documento con el estado de resumen indicado vía el servicio.
func seedWithStatus(t *testing.T, svc *application.DocumentService, id, checksum, text string, status domain.SummaryStatus) {
	t.Helper()
	doc := testutil.NewDoc(id, "mi archivo.pdf", checksum, status, time.Now().UTC())
	doc.ExtractedText = text
	doc.ExtractionTimeMS = 342
	_, err := svc.CreatePending(context.Background(), doc)
	require.NoError(t, err)
}

// seedCompleted inserta un documento con el resumen resuelto vía el servicio.
func seedCompleted(t *testing.T, svc *application.DocumentService, id, checksum, text, summary string) {
	t.Helper()
	seedPending(t, svc, id, checksum, text)
	_, err := svc.CompleteWithSummary(context.Background(), id, summary, 120)
	require.NoError(t, err)
}

// documentResponse hace GET y devuelve el DTO y el JSON crudo de la respuesta.
func documentResponse(t *testing.T, router *gin.Engine, path string) (api.DocumentResponse, map[string]any) {
	t.Helper()
	rr := testutil.DoGet(t, router, path)
	require.Equal(t, http.StatusOK, rr.Code)

	var dto api.DocumentResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &dto))
	var raw map[string]any
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &raw))
	return dto, raw
}

// requireErrorBody verifica el envelope de error (code + message).
func requireErrorBody(t *testing.T, rr *httptest.ResponseRecorder, code, message string) {
	t.Helper()
	require.JSONEq(t, fmt.Sprintf(`{"code":%q,"message":%q}`, code, message), rr.Body.String())
}
