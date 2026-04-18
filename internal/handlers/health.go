package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"go.uber.org/zap"
)

type HealthCheck struct {
	Name  string
	Check func(ctx context.Context) error
}

type HealthHandler struct {
	checks  []HealthCheck
	timeout time.Duration
	logger  *zap.Logger
}

func NewHealthHandler(logger *zap.Logger, timeout time.Duration, checks ...HealthCheck) *HealthHandler {
	return &HealthHandler{checks: checks, timeout: timeout, logger: logger}
}

type componentStatus struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

type healthResponse struct {
	Status     string            `json:"status"`
	Components []componentStatus `json:"components"`
}

func (h *HealthHandler) Handle(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()

	results := make([]componentStatus, len(h.checks))
	var wg sync.WaitGroup
	for i, c := range h.checks {
		wg.Add(1)
		go func(i int, c HealthCheck) {
			defer wg.Done()
			if err := c.Check(ctx); err != nil {
				results[i] = componentStatus{Name: c.Name, Status: "down", Error: err.Error()}
				return
			}
			results[i] = componentStatus{Name: c.Name, Status: "ok"}
		}(i, c)
	}
	wg.Wait()

	overall := "ok"
	code := http.StatusOK
	for _, comp := range results {
		if comp.Status != "ok" {
			overall = "degraded"
			code = http.StatusServiceUnavailable
			break
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(healthResponse{Status: overall, Components: results}); err != nil {
		h.logger.Error("encode health response", zap.Error(err))
	}
}