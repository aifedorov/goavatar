package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

func RouteTagMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		next.ServeHTTP(w, req)
		rctx := chi.RouteContext(req.Context())
		if rctx == nil {
			return
		}
		pattern := rctx.RoutePattern()
		if pattern == "" {
			return
		}
		span := trace.SpanFromContext(req.Context())
		span.SetName(req.Method + " " + pattern)
		span.SetAttributes(attribute.String("http.route", pattern))
	})
}
