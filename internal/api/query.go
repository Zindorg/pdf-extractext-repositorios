package api

import (
	"fmt"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/marco/pdf-extractext-repositorios/internal/domain"
)

// parseListFilter valida y construye el ListFilter desde la query string.
func parseListFilter(c *gin.Context) (domain.ListFilter, error) {
	filter := domain.ListFilter{}
	var err error
	if filter.SummaryStatus, err = parseStatus(c.Query("status")); err != nil {
		return filter, err
	}
	if s := c.Query("filename"); s != "" {
		filter.Filename = &s
	}
	if filter.CreatedFrom, err = parseTimeQuery(c.Query("created_from"), "created_from"); err != nil {
		return filter, err
	}
	if filter.CreatedTo, err = parseTimeQuery(c.Query("created_to"), "created_to"); err != nil {
		return filter, err
	}
	filter.IncludeDeleted = c.Query("include_deleted") == "true"
	return filter, nil
}

func parseStatus(raw string) (*domain.SummaryStatus, error) {
	if raw == "" {
		return nil, nil
	}
	switch s := domain.SummaryStatus(raw); s {
	case domain.SummaryNotRequested, domain.SummaryPending, domain.SummaryResolved, domain.SummaryFailed:
		return &s, nil
	default:
		return nil, fmt.Errorf("invalid status")
	}
}

func parseTimeQuery(raw, name string) (*time.Time, error) {
	if raw == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil, fmt.Errorf("invalid %s", name)
	}
	return &t, nil
}

func parsePagination(c *gin.Context) (int, int, error) {
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil {
		return 0, 0, fmt.Errorf("invalid page")
	}
	pageSize, err := strconv.Atoi(c.DefaultQuery("page_size", strconv.Itoa(domain.DefaultPageSize)))
	if err != nil {
		return 0, 0, fmt.Errorf("invalid page_size")
	}
	return page, pageSize, nil
}
