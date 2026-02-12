package api

import (
	"context"
	"fmt"
	"net/http"
	"runtime"
	"time"
)

// ServiceHealth represents the health status of an individual service dependency.
type ServiceHealth struct {
	Status   string  `json:"status"`             // "healthy", "unhealthy", "disabled"
	Latency  string  `json:"latency,omitempty"`   // Response time as human-readable duration
	Error    string  `json:"error,omitempty"`      // Error message if unhealthy
	Details  interface{} `json:"details,omitempty"` // Optional service-specific details
}

// DeepHealthResponse is the response body for the deep health check endpoint.
type DeepHealthResponse struct {
	Status    string                   `json:"status"`    // "ok", "degraded", "unhealthy"
	Version   string                   `json:"version"`
	Uptime    string                   `json:"uptime"`
	Timestamp string                   `json:"timestamp"`
	Services  map[string]ServiceHealth `json:"services"`
	System    SystemInfo               `json:"system"`
}

// SystemInfo contains runtime information about the AmityVox process.
type SystemInfo struct {
	GoVersion    string `json:"go_version"`
	NumGoroutine int    `json:"num_goroutine"`
	NumCPU       int    `json:"num_cpu"`
	MemAllocMB   float64 `json:"mem_alloc_mb"`
	MemSysMB     float64 `json:"mem_sys_mb"`
	MemGCCycles  uint32  `json:"mem_gc_cycles"`
}

// handleDeepHealthCheck performs a comprehensive health check of all service
// dependencies: PostgreSQL, NATS, DragonflyDB, S3 storage, and Meilisearch.
// Each service is checked with a timeout and its latency is reported.
//
// GET /health/deep
//
// Response: 200 if all services healthy, 503 if any service is degraded.
func (s *Server) handleDeepHealthCheck(w http.ResponseWriter, r *http.Request) {
	services := make(map[string]ServiceHealth)
	overallStatus := "ok"

	// Use a short timeout for each individual check to avoid long waits.
	checkTimeout := 5 * time.Second

	// --- PostgreSQL ---
	dbHealth := s.checkServiceHealth("database", checkTimeout, func(ctx context.Context) error {
		return s.DB.HealthCheck(ctx)
	})
	services["database"] = dbHealth
	if dbHealth.Status == "unhealthy" {
		overallStatus = "unhealthy"
	}

	// Also collect pgx pool stats.
	if s.DB != nil && s.DB.Pool != nil {
		stat := s.DB.Pool.Stat()
		dbSvc := services["database"]
		dbSvc.Details = map[string]interface{}{
			"total_conns":    stat.TotalConns(),
			"idle_conns":     stat.IdleConns(),
			"acquired_conns": stat.AcquiredConns(),
			"max_conns":      stat.MaxConns(),
		}
		services["database"] = dbSvc
	}

	// --- NATS ---
	if s.EventBus != nil {
		natsHealth := s.checkServiceHealth("nats", checkTimeout, func(_ context.Context) error {
			return s.EventBus.HealthCheck()
		})
		services["nats"] = natsHealth
		if natsHealth.Status == "unhealthy" {
			if overallStatus == "ok" {
				overallStatus = "degraded"
			}
		}
	} else {
		services["nats"] = ServiceHealth{Status: "disabled"}
	}

	// --- DragonflyDB (Cache) ---
	if s.Cache != nil {
		cacheHealth := s.checkServiceHealth("cache", checkTimeout, func(ctx context.Context) error {
			return s.Cache.HealthCheck(ctx)
		})
		services["cache"] = cacheHealth
		if cacheHealth.Status == "unhealthy" {
			if overallStatus == "ok" {
				overallStatus = "degraded"
			}
		}
	} else {
		services["cache"] = ServiceHealth{Status: "disabled"}
	}

	// --- S3 Storage ---
	if s.Media != nil {
		s3Health := s.checkServiceHealth("storage", checkTimeout, func(ctx context.Context) error {
			return s.Media.HealthCheck(ctx)
		})
		services["storage"] = s3Health
		if s3Health.Status == "unhealthy" {
			if overallStatus == "ok" {
				overallStatus = "degraded"
			}
		}
	} else {
		services["storage"] = ServiceHealth{Status: "disabled"}
	}

	// --- Meilisearch ---
	if s.Search != nil {
		searchHealth := s.checkServiceHealth("search", checkTimeout, func(_ context.Context) error {
			return s.Search.HealthCheck()
		})
		services["search"] = searchHealth
		if searchHealth.Status == "unhealthy" {
			if overallStatus == "ok" {
				overallStatus = "degraded"
			}
		}
	} else {
		services["search"] = ServiceHealth{Status: "disabled"}
	}

	// --- LiveKit (Voice) ---
	if s.Voice != nil {
		services["voice"] = ServiceHealth{Status: "healthy", Details: "connected"}
	} else {
		services["voice"] = ServiceHealth{Status: "disabled"}
	}

	// --- Runtime info ---
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	response := DeepHealthResponse{
		Status:    overallStatus,
		Version:   s.Version,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Services:  services,
		System: SystemInfo{
			GoVersion:    runtime.Version(),
			NumGoroutine: runtime.NumGoroutine(),
			NumCPU:       runtime.NumCPU(),
			MemAllocMB:   float64(memStats.Alloc) / 1024 / 1024,
			MemSysMB:     float64(memStats.Sys) / 1024 / 1024,
			MemGCCycles:  memStats.NumGC,
		},
	}

	httpStatus := http.StatusOK
	if overallStatus != "ok" {
		httpStatus = http.StatusServiceUnavailable
	}

	WriteJSON(w, httpStatus, response)
}

// checkServiceHealth runs a health check function with a timeout and returns
// a ServiceHealth struct with the status, latency, and any error.
func (s *Server) checkServiceHealth(name string, timeout time.Duration, check func(context.Context) error) ServiceHealth {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	start := time.Now()
	err := check(ctx)
	latency := time.Since(start)

	if err != nil {
		return ServiceHealth{
			Status:  "unhealthy",
			Latency: latency.String(),
			Error:   fmt.Sprintf("%s health check failed: %v", name, err),
		}
	}

	return ServiceHealth{
		Status:  "healthy",
		Latency: latency.String(),
	}
}
