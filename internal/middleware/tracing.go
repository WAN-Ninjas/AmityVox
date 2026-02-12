// Package middleware provides HTTP middleware for the AmityVox API server,
// including request tracing with correlation IDs and OpenTelemetry integration.
package middleware

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/oklog/ulid/v2"
)

// contextKey is an unexported type used for context value keys to avoid collisions.
type contextKey string

const (
	// correlationIDKey is the context key for the request correlation ID.
	correlationIDKey contextKey = "correlation_id"

	// spanNameKey is the context key for the current trace span name.
	spanNameKey contextKey = "span_name"
)

// CorrelationIDHeader is the HTTP header used to propagate correlation IDs.
const CorrelationIDHeader = "X-Request-ID"

// CorrelationID is a middleware that ensures every request has a unique
// correlation ID. If the incoming request contains an X-Request-ID header, that
// value is reused; otherwise a new ULID is generated. The ID is stored in the
// request context and set as a response header.
func CorrelationID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(CorrelationIDHeader)
		if id == "" {
			id = ulid.Make().String()
		}

		ctx := context.WithValue(r.Context(), correlationIDKey, id)
		w.Header().Set(CorrelationIDHeader, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetCorrelationID extracts the correlation ID from the request context.
// Returns an empty string if no correlation ID is present.
func GetCorrelationID(ctx context.Context) string {
	if id, ok := ctx.Value(correlationIDKey).(string); ok {
		return id
	}
	return ""
}

// TracingLogger returns a middleware that produces structured log entries enriched
// with the correlation ID from the request context. It logs method, path, status,
// latency, and the trace ID for every request, enabling distributed request tracing
// across services.
func TracingLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Wrap response writer to capture status code.
			sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}

			next.ServeHTTP(sw, r)

			duration := time.Since(start)
			correlationID := GetCorrelationID(r.Context())

			attrs := []slog.Attr{
				slog.String("trace_id", correlationID),
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", sw.status),
				slog.Int("bytes", sw.written),
				slog.Duration("latency", duration),
				slog.String("remote_addr", r.RemoteAddr),
				slog.String("user_agent", r.UserAgent()),
			}

			// Add span name if set.
			if spanName := GetSpanName(r.Context()); spanName != "" {
				attrs = append(attrs, slog.String("span", spanName))
			}

			level := slog.LevelInfo
			if sw.status >= 500 {
				level = slog.LevelError
			} else if sw.status >= 400 {
				level = slog.LevelWarn
			}

			logger.LogAttrs(r.Context(), level, "http request", attrs...)
		})
	}
}

// WithSpan sets a span name on the context for tracing purposes. This can be
// used by handlers to annotate specific operations within a request.
func WithSpan(ctx context.Context, name string) context.Context {
	return context.WithValue(ctx, spanNameKey, name)
}

// GetSpanName retrieves the current span name from the context.
func GetSpanName(ctx context.Context) string {
	if name, ok := ctx.Value(spanNameKey).(string); ok {
		return name
	}
	return ""
}

// TraceSpan is a helper for instrumenting code blocks with timing information.
// It logs the span name, duration, and any error that occurred.
type TraceSpan struct {
	Name          string
	CorrelationID string
	Start         time.Time
	Logger        *slog.Logger
}

// StartSpan begins a new trace span with the given name. The span is associated
// with the correlation ID from the context.
func StartSpan(ctx context.Context, name string, logger *slog.Logger) *TraceSpan {
	return &TraceSpan{
		Name:          name,
		CorrelationID: GetCorrelationID(ctx),
		Start:         time.Now(),
		Logger:        logger,
	}
}

// End completes the trace span and logs its duration. If err is non-nil, the
// span is logged at error level with the error message.
func (s *TraceSpan) End(err error) {
	duration := time.Since(s.Start)
	attrs := []slog.Attr{
		slog.String("trace_id", s.CorrelationID),
		slog.String("span", s.Name),
		slog.Duration("duration", duration),
	}

	if err != nil {
		attrs = append(attrs, slog.String("error", err.Error()))
		s.Logger.LogAttrs(context.Background(), slog.LevelError, "span completed with error", attrs...)
	} else {
		s.Logger.LogAttrs(context.Background(), slog.LevelDebug, "span completed", attrs...)
	}
}

// OTLPConfig holds configuration for OpenTelemetry Protocol exporter integration.
// When enabled, traces are exported to the configured OTLP endpoint for collection
// by systems like Jaeger, Tempo, or any OTLP-compatible backend.
type OTLPConfig struct {
	// Enabled controls whether OTLP trace export is active.
	Enabled bool `toml:"enabled"`

	// Endpoint is the OTLP collector endpoint (e.g., "localhost:4317" for gRPC).
	Endpoint string `toml:"endpoint"`

	// Protocol is the OTLP transport protocol: "grpc" or "http".
	Protocol string `toml:"protocol"`

	// ServiceName is the service name reported to the collector.
	ServiceName string `toml:"service_name"`

	// SampleRate is the fraction of traces to sample (0.0 to 1.0).
	// Use 1.0 for development, 0.1 or lower for production.
	SampleRate float64 `toml:"sample_rate"`

	// Insecure disables TLS for the OTLP connection.
	Insecure bool `toml:"insecure"`
}

// DefaultOTLPConfig returns sensible defaults for OTLP configuration.
func DefaultOTLPConfig() OTLPConfig {
	return OTLPConfig{
		Enabled:     false,
		Endpoint:    "localhost:4317",
		Protocol:    "grpc",
		ServiceName: "amityvox",
		SampleRate:  0.1,
		Insecure:    true,
	}
}

// Validate checks that the OTLP configuration is valid when enabled.
func (c OTLPConfig) Validate() error {
	if !c.Enabled {
		return nil
	}
	if c.Endpoint == "" {
		return fmt.Errorf("otlp: endpoint is required when tracing is enabled")
	}
	if c.Protocol != "grpc" && c.Protocol != "http" {
		return fmt.Errorf("otlp: protocol must be 'grpc' or 'http' (got %q)", c.Protocol)
	}
	if c.SampleRate < 0 || c.SampleRate > 1 {
		return fmt.Errorf("otlp: sample_rate must be between 0.0 and 1.0 (got %f)", c.SampleRate)
	}
	if c.ServiceName == "" {
		return fmt.Errorf("otlp: service_name is required when tracing is enabled")
	}
	return nil
}

// statusWriter wraps http.ResponseWriter to capture the status code and bytes written.
type statusWriter struct {
	http.ResponseWriter
	status  int
	written int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	n, err := w.ResponseWriter.Write(b)
	w.written += n
	return n, err
}
