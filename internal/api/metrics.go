// Package api: metrics.go implements a lightweight Prometheus-compatible /metrics
// endpoint that exposes instance-level counters and gauges without requiring an
// external dependency on the Prometheus Go client library.
package api

import (
	"fmt"
	"net/http"
	"runtime"
	"sync/atomic"
	"time"
)

// Metrics tracks lightweight counters for the /metrics endpoint.
type Metrics struct {
	HTTPRequestsTotal   atomic.Int64
	HTTPRequestDuration atomic.Int64 // total microseconds
	WSConnectionsTotal  atomic.Int64
	WSConnectionsCurr   atomic.Int64
	MessagesCreated     atomic.Int64
	StartTime           time.Time
}

// GlobalMetrics is the singleton instance.
var GlobalMetrics = &Metrics{
	StartTime: time.Now(),
}

// handleMetrics exposes Prometheus-compatible metrics in text exposition format.
func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	m := GlobalMetrics
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	// Query live counts from the database.
	var userCount, guildCount, channelCount, messageCount int64
	s.DB.Pool.QueryRow(r.Context(), `SELECT COUNT(*) FROM users`).Scan(&userCount)
	s.DB.Pool.QueryRow(r.Context(), `SELECT COUNT(*) FROM guilds`).Scan(&guildCount)
	s.DB.Pool.QueryRow(r.Context(), `SELECT COUNT(*) FROM channels`).Scan(&channelCount)
	s.DB.Pool.QueryRow(r.Context(), `SELECT COUNT(*) FROM messages`).Scan(&messageCount)

	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")

	fmt.Fprintf(w, "# HELP amityvox_http_requests_total Total HTTP requests served.\n")
	fmt.Fprintf(w, "# TYPE amityvox_http_requests_total counter\n")
	fmt.Fprintf(w, "amityvox_http_requests_total %d\n\n", m.HTTPRequestsTotal.Load())

	fmt.Fprintf(w, "# HELP amityvox_http_request_duration_seconds Total time spent processing HTTP requests.\n")
	fmt.Fprintf(w, "# TYPE amityvox_http_request_duration_seconds counter\n")
	fmt.Fprintf(w, "amityvox_http_request_duration_seconds %f\n\n", float64(m.HTTPRequestDuration.Load())/1e6)

	fmt.Fprintf(w, "# HELP amityvox_websocket_connections_total Total WebSocket connections opened.\n")
	fmt.Fprintf(w, "# TYPE amityvox_websocket_connections_total counter\n")
	fmt.Fprintf(w, "amityvox_websocket_connections_total %d\n\n", m.WSConnectionsTotal.Load())

	fmt.Fprintf(w, "# HELP amityvox_websocket_connections_current Current WebSocket connections.\n")
	fmt.Fprintf(w, "# TYPE amityvox_websocket_connections_current gauge\n")
	fmt.Fprintf(w, "amityvox_websocket_connections_current %d\n\n", m.WSConnectionsCurr.Load())

	fmt.Fprintf(w, "# HELP amityvox_messages_created_total Total messages created.\n")
	fmt.Fprintf(w, "# TYPE amityvox_messages_created_total counter\n")
	fmt.Fprintf(w, "amityvox_messages_created_total %d\n\n", m.MessagesCreated.Load())

	fmt.Fprintf(w, "# HELP amityvox_users_total Total registered users.\n")
	fmt.Fprintf(w, "# TYPE amityvox_users_total gauge\n")
	fmt.Fprintf(w, "amityvox_users_total %d\n\n", userCount)

	fmt.Fprintf(w, "# HELP amityvox_guilds_total Total guilds.\n")
	fmt.Fprintf(w, "# TYPE amityvox_guilds_total gauge\n")
	fmt.Fprintf(w, "amityvox_guilds_total %d\n\n", guildCount)

	fmt.Fprintf(w, "# HELP amityvox_channels_total Total channels.\n")
	fmt.Fprintf(w, "# TYPE amityvox_channels_total gauge\n")
	fmt.Fprintf(w, "amityvox_channels_total %d\n\n", channelCount)

	fmt.Fprintf(w, "# HELP amityvox_messages_total Total messages stored.\n")
	fmt.Fprintf(w, "# TYPE amityvox_messages_total gauge\n")
	fmt.Fprintf(w, "amityvox_messages_total %d\n\n", messageCount)

	fmt.Fprintf(w, "# HELP amityvox_goroutines Current number of goroutines.\n")
	fmt.Fprintf(w, "# TYPE amityvox_goroutines gauge\n")
	fmt.Fprintf(w, "amityvox_goroutines %d\n\n", runtime.NumGoroutine())

	fmt.Fprintf(w, "# HELP amityvox_memory_alloc_bytes Current memory allocation in bytes.\n")
	fmt.Fprintf(w, "# TYPE amityvox_memory_alloc_bytes gauge\n")
	fmt.Fprintf(w, "amityvox_memory_alloc_bytes %d\n\n", mem.Alloc)

	fmt.Fprintf(w, "# HELP amityvox_memory_sys_bytes Total memory obtained from the OS.\n")
	fmt.Fprintf(w, "# TYPE amityvox_memory_sys_bytes gauge\n")
	fmt.Fprintf(w, "amityvox_memory_sys_bytes %d\n\n", mem.Sys)

	uptime := time.Since(m.StartTime).Seconds()
	fmt.Fprintf(w, "# HELP amityvox_uptime_seconds Time since server start.\n")
	fmt.Fprintf(w, "# TYPE amityvox_uptime_seconds gauge\n")
	fmt.Fprintf(w, "amityvox_uptime_seconds %f\n", uptime)
}
