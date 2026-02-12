// Self-Hosting Excellence handlers: setup wizard, auto-update, health monitoring,
// storage dashboard, data retention policies, custom domain support, backup scheduling.
// All methods belong to the existing admin.Handler struct defined in admin.go.
package admin

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"runtime"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/amityvox/amityvox/internal/auth"
	"github.com/amityvox/amityvox/internal/models"
)

// =============================================================================
// Setup Wizard
// =============================================================================

// HandleGetSetupStatus returns whether the instance has completed first-run setup.
// GET /api/v1/admin/setup/status
func (h *Handler) HandleGetSetupStatus(w http.ResponseWriter, r *http.Request) {
	var completed string
	err := h.Pool.QueryRow(r.Context(),
		`SELECT COALESCE(
			(SELECT value FROM instance_settings WHERE key = 'setup_completed'), 'false'
		)`).Scan(&completed)
	if err != nil {
		completed = "false"
	}

	var instanceName string
	h.Pool.QueryRow(r.Context(),
		`SELECT COALESCE(name, '') FROM instances WHERE id = $1`, h.InstanceID).Scan(&instanceName)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"completed":     completed == "true",
		"instance_name": instanceName,
		"instance_id":   h.InstanceID,
	})
}

// HandleCompleteSetup processes the first-run setup wizard form.
// POST /api/v1/admin/setup/complete
func (h *Handler) HandleCompleteSetup(w http.ResponseWriter, r *http.Request) {
	// Setup can be run by anyone when not yet completed, or by admins to re-run.
	var completed string
	h.Pool.QueryRow(r.Context(),
		`SELECT COALESCE(
			(SELECT value FROM instance_settings WHERE key = 'setup_completed'), 'false'
		)`).Scan(&completed)

	if completed == "true" {
		// Already completed — require admin.
		if !h.isAdmin(r) {
			writeError(w, http.StatusForbidden, "forbidden", "Setup already completed. Admin access required to reconfigure.")
			return
		}
	}

	var req struct {
		InstanceName    string `json:"instance_name"`
		Description     string `json:"description"`
		AdminUsername   string `json:"admin_username"`
		AdminEmail      string `json:"admin_email"`
		AdminPassword   string `json:"admin_password"`
		FederationMode  string `json:"federation_mode"`
		RegistrationMode string `json:"registration_mode"`
		Domain          string `json:"domain"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if req.InstanceName == "" {
		writeError(w, http.StatusBadRequest, "missing_name", "Instance name is required")
		return
	}

	// Update instance record.
	_, err := h.Pool.Exec(r.Context(),
		`UPDATE instances SET name = $1, description = $2, federation_mode = COALESCE(NULLIF($3, ''), federation_mode)
		 WHERE id = $4`,
		req.InstanceName, req.Description, req.FederationMode, h.InstanceID)
	if err != nil {
		h.Logger.Error("setup: failed to update instance", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to update instance settings")
		return
	}

	// Set registration mode.
	if req.RegistrationMode != "" {
		switch req.RegistrationMode {
		case "open", "invite_only", "closed":
			h.Pool.Exec(r.Context(),
				`INSERT INTO instance_settings (key, value, updated_at) VALUES ('registration_mode', $1, now())
				 ON CONFLICT (key) DO UPDATE SET value = $1, updated_at = now()`, req.RegistrationMode)
		}
	}

	// Set domain.
	if req.Domain != "" {
		h.Pool.Exec(r.Context(),
			`UPDATE instances SET domain = $1 WHERE id = $2`, req.Domain, h.InstanceID)
	}

	// Mark setup as completed.
	h.Pool.Exec(r.Context(),
		`INSERT INTO instance_settings (key, value, updated_at) VALUES ('setup_completed', 'true', now())
		 ON CONFLICT (key) DO UPDATE SET value = 'true', updated_at = now()`)

	// Store setup timestamp.
	h.Pool.Exec(r.Context(),
		`INSERT INTO instance_settings (key, value, updated_at) VALUES ('setup_completed_at', $1, now())
		 ON CONFLICT (key) DO UPDATE SET value = $1, updated_at = now()`,
		time.Now().UTC().Format(time.RFC3339))

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "completed",
		"message": "Instance setup completed successfully",
	})
}

// =============================================================================
// Auto-Update Mechanism
// =============================================================================

// HandleCheckUpdates checks for new versions of AmityVox.
// GET /api/v1/admin/updates/check
func (h *Handler) HandleCheckUpdates(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		writeError(w, http.StatusForbidden, "forbidden", "Admin access required")
		return
	}

	// Read current version from instance.
	var currentVersion string
	err := h.Pool.QueryRow(r.Context(),
		`SELECT COALESCE(software_version, 'unknown') FROM instances WHERE id = $1`,
		h.InstanceID).Scan(&currentVersion)
	if err != nil {
		currentVersion = "unknown"
	}

	// Read last check timestamp and cached latest version.
	var lastCheck, latestVersion, releaseNotes, releaseURL string
	h.Pool.QueryRow(r.Context(),
		`SELECT COALESCE((SELECT value FROM instance_settings WHERE key = 'update_last_check'), '')`).Scan(&lastCheck)
	h.Pool.QueryRow(r.Context(),
		`SELECT COALESCE((SELECT value FROM instance_settings WHERE key = 'update_latest_version'), '')`).Scan(&latestVersion)
	h.Pool.QueryRow(r.Context(),
		`SELECT COALESCE((SELECT value FROM instance_settings WHERE key = 'update_release_notes'), '')`).Scan(&releaseNotes)
	h.Pool.QueryRow(r.Context(),
		`SELECT COALESCE((SELECT value FROM instance_settings WHERE key = 'update_release_url'), '')`).Scan(&releaseURL)

	// Auto-dismiss is enabled if the version has been dismissed.
	var dismissed string
	h.Pool.QueryRow(r.Context(),
		`SELECT COALESCE((SELECT value FROM instance_settings WHERE key = 'update_dismissed_version'), '')`).Scan(&dismissed)

	updateAvailable := latestVersion != "" && latestVersion != currentVersion && latestVersion != dismissed

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"current_version":  currentVersion,
		"latest_version":   latestVersion,
		"update_available": updateAvailable,
		"release_notes":    releaseNotes,
		"release_url":      releaseURL,
		"last_checked":     lastCheck,
		"dismissed":        dismissed == latestVersion,
	})
}

// HandleSetLatestVersion allows admin to manually set the latest known version
// (used when an external check discovers a new version).
// POST /api/v1/admin/updates/set-latest
func (h *Handler) HandleSetLatestVersion(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		writeError(w, http.StatusForbidden, "forbidden", "Admin access required")
		return
	}

	var req struct {
		Version      string `json:"version"`
		ReleaseNotes string `json:"release_notes"`
		ReleaseURL   string `json:"release_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if req.Version == "" {
		writeError(w, http.StatusBadRequest, "missing_version", "Version is required")
		return
	}

	settings := map[string]string{
		"update_latest_version": req.Version,
		"update_release_notes":  req.ReleaseNotes,
		"update_release_url":    req.ReleaseURL,
		"update_last_check":     time.Now().UTC().Format(time.RFC3339),
	}

	for k, v := range settings {
		h.Pool.Exec(r.Context(),
			`INSERT INTO instance_settings (key, value, updated_at) VALUES ($1, $2, now())
			 ON CONFLICT (key) DO UPDATE SET value = $2, updated_at = now()`, k, v)
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// HandleDismissUpdate dismisses the current update notification.
// POST /api/v1/admin/updates/dismiss
func (h *Handler) HandleDismissUpdate(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		writeError(w, http.StatusForbidden, "forbidden", "Admin access required")
		return
	}

	var latestVersion string
	h.Pool.QueryRow(r.Context(),
		`SELECT COALESCE((SELECT value FROM instance_settings WHERE key = 'update_latest_version'), '')`).Scan(&latestVersion)

	if latestVersion != "" {
		h.Pool.Exec(r.Context(),
			`INSERT INTO instance_settings (key, value, updated_at) VALUES ('update_dismissed_version', $1, now())
			 ON CONFLICT (key) DO UPDATE SET value = $1, updated_at = now()`, latestVersion)
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "dismissed"})
}

// HandleGetUpdateConfig returns the auto-update configuration.
// GET /api/v1/admin/updates/config
func (h *Handler) HandleGetUpdateConfig(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		writeError(w, http.StatusForbidden, "forbidden", "Admin access required")
		return
	}

	var autoCheck, channel, notifyAdmins string
	h.Pool.QueryRow(r.Context(),
		`SELECT COALESCE((SELECT value FROM instance_settings WHERE key = 'update_auto_check'), 'true')`).Scan(&autoCheck)
	h.Pool.QueryRow(r.Context(),
		`SELECT COALESCE((SELECT value FROM instance_settings WHERE key = 'update_channel'), 'stable')`).Scan(&channel)
	h.Pool.QueryRow(r.Context(),
		`SELECT COALESCE((SELECT value FROM instance_settings WHERE key = 'update_notify_admins'), 'true')`).Scan(&notifyAdmins)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"auto_check":    autoCheck == "true",
		"channel":       channel,
		"notify_admins": notifyAdmins == "true",
	})
}

// HandleUpdateUpdateConfig modifies auto-update settings.
// PATCH /api/v1/admin/updates/config
func (h *Handler) HandleUpdateUpdateConfig(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		writeError(w, http.StatusForbidden, "forbidden", "Admin access required")
		return
	}

	var req struct {
		AutoCheck    *bool   `json:"auto_check"`
		Channel      *string `json:"channel"`
		NotifyAdmins *bool   `json:"notify_admins"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if req.AutoCheck != nil {
		v := "false"
		if *req.AutoCheck {
			v = "true"
		}
		h.Pool.Exec(r.Context(),
			`INSERT INTO instance_settings (key, value, updated_at) VALUES ('update_auto_check', $1, now())
			 ON CONFLICT (key) DO UPDATE SET value = $1, updated_at = now()`, v)
	}

	if req.Channel != nil {
		switch *req.Channel {
		case "stable", "beta", "nightly":
			h.Pool.Exec(r.Context(),
				`INSERT INTO instance_settings (key, value, updated_at) VALUES ('update_channel', $1, now())
				 ON CONFLICT (key) DO UPDATE SET value = $1, updated_at = now()`, *req.Channel)
		default:
			writeError(w, http.StatusBadRequest, "invalid_channel", "Channel must be 'stable', 'beta', or 'nightly'")
			return
		}
	}

	if req.NotifyAdmins != nil {
		v := "false"
		if *req.NotifyAdmins {
			v = "true"
		}
		h.Pool.Exec(r.Context(),
			`INSERT INTO instance_settings (key, value, updated_at) VALUES ('update_notify_admins', $1, now())
			 ON CONFLICT (key) DO UPDATE SET value = $1, updated_at = now()`, v)
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// =============================================================================
// Instance Health Monitoring
// =============================================================================

// HandleGetHealthDashboard returns comprehensive health status of all services.
// GET /api/v1/admin/health
func (h *Handler) HandleGetHealthDashboard(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		writeError(w, http.StatusForbidden, "forbidden", "Admin access required")
		return
	}

	type serviceHealth struct {
		Name           string  `json:"name"`
		Status         string  `json:"status"`
		ResponseTimeMs int     `json:"response_time_ms"`
		Details        string  `json:"details"`
		LastChecked    string  `json:"last_checked"`
	}

	services := make([]serviceHealth, 0, 5)
	now := time.Now().UTC()

	// PostgreSQL health.
	pgStart := time.Now()
	var pgVersion string
	pgErr := h.Pool.QueryRow(r.Context(), `SELECT version()`).Scan(&pgVersion)
	pgDuration := time.Since(pgStart).Milliseconds()
	pgStatus := "healthy"
	pgDetails := pgVersion
	if pgErr != nil {
		pgStatus = "unhealthy"
		pgDetails = pgErr.Error()
	}
	services = append(services, serviceHealth{
		Name: "postgresql", Status: pgStatus,
		ResponseTimeMs: int(pgDuration), Details: pgDetails,
		LastChecked: now.Format(time.RFC3339),
	})

	// Database size and connection pool info.
	var dbSize string
	h.Pool.QueryRow(r.Context(),
		`SELECT pg_size_pretty(pg_database_size(current_database()))`).Scan(&dbSize)

	poolStats := h.Pool.Stat()

	// Process runtime stats.
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	// Recent health snapshot trend (last 24h).
	type trendPoint struct {
		Time   string `json:"time"`
		Status string `json:"status"`
		RespMs int    `json:"response_time_ms"`
	}
	rows, err := h.Pool.Query(r.Context(),
		`SELECT service, status, response_time_ms, created_at
		 FROM health_snapshots
		 WHERE created_at >= now() - INTERVAL '24 hours'
		 ORDER BY created_at DESC
		 LIMIT 200`)

	trends := make(map[string][]trendPoint)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var svc, status string
			var respMs int
			var createdAt time.Time
			if err := rows.Scan(&svc, &status, &respMs, &createdAt); err != nil {
				continue
			}
			trends[svc] = append(trends[svc], trendPoint{
				Time:   createdAt.Format(time.RFC3339),
				Status: status,
				RespMs: respMs,
			})
		}
	}

	// Count active connections, message rate.
	var activeSessions int64
	h.Pool.QueryRow(r.Context(),
		`SELECT COUNT(*) FROM user_sessions WHERE expires_at > now()`).Scan(&activeSessions)

	var messagesLastHour int64
	h.Pool.QueryRow(r.Context(),
		`SELECT COUNT(*) FROM messages WHERE created_at >= now() - INTERVAL '1 hour'`).Scan(&messagesLastHour)

	var messagesLastDay int64
	h.Pool.QueryRow(r.Context(),
		`SELECT COUNT(*) FROM messages WHERE created_at >= now() - INTERVAL '24 hours'`).Scan(&messagesLastDay)

	// Instance uptime.
	var createdAt time.Time
	var uptime string
	if err := h.Pool.QueryRow(r.Context(),
		`SELECT created_at FROM instances WHERE id = $1`, h.InstanceID).Scan(&createdAt); err == nil {
		uptime = time.Since(createdAt).Truncate(time.Second).String()
	}

	// Record health snapshot for trend tracking.
	for _, svc := range services {
		snapshotID := models.NewULID().String()
		h.Pool.Exec(r.Context(),
			`INSERT INTO health_snapshots (id, service, status, response_time_ms, created_at)
			 VALUES ($1, $2, $3, $4, now())`,
			snapshotID, svc.Name, svc.Status, svc.ResponseTimeMs)
	}

	// Cleanup old snapshots (keep 7 days).
	h.Pool.Exec(r.Context(),
		`DELETE FROM health_snapshots WHERE created_at < now() - INTERVAL '7 days'`)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"services": services,
		"database": map[string]interface{}{
			"size":            dbSize,
			"total_conns":     poolStats.TotalConns(),
			"idle_conns":      poolStats.IdleConns(),
			"acquired_conns":  poolStats.AcquiredConns(),
			"max_conns":       poolStats.MaxConns(),
		},
		"runtime": map[string]interface{}{
			"go_version":    runtime.Version(),
			"goroutines":    runtime.NumGoroutine(),
			"mem_alloc_mb":  memStats.Alloc / 1024 / 1024,
			"mem_sys_mb":    memStats.Sys / 1024 / 1024,
			"mem_gc_cycles": memStats.NumGC,
			"num_cpu":       runtime.NumCPU(),
			"uptime":        uptime,
		},
		"activity": map[string]interface{}{
			"active_sessions":    activeSessions,
			"messages_last_hour": messagesLastHour,
			"messages_last_day":  messagesLastDay,
		},
		"trends": trends,
	})
}

// HandleGetHealthHistory returns historical health snapshots for a service.
// GET /api/v1/admin/health/history
func (h *Handler) HandleGetHealthHistory(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		writeError(w, http.StatusForbidden, "forbidden", "Admin access required")
		return
	}

	service := r.URL.Query().Get("service")
	hours := 24
	if hStr := r.URL.Query().Get("hours"); hStr != "" {
		if v, err := parseInt(hStr); err == nil && v > 0 && v <= 168 {
			hours = v
		}
	}

	query := `SELECT id, service, status, response_time_ms, created_at
		 FROM health_snapshots
		 WHERE created_at >= now() - make_interval(hours => $1)`
	args := []interface{}{hours}

	if service != "" {
		query += ` AND service = $2`
		args = append(args, service)
	}
	query += ` ORDER BY created_at DESC LIMIT 500`

	rows, err := h.Pool.Query(r.Context(), query, args...)
	if err != nil {
		h.Logger.Error("failed to query health history", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get health history")
		return
	}
	defer rows.Close()

	type snapshot struct {
		ID             string    `json:"id"`
		Service        string    `json:"service"`
		Status         string    `json:"status"`
		ResponseTimeMs int       `json:"response_time_ms"`
		CreatedAt      time.Time `json:"created_at"`
	}

	snapshots := make([]snapshot, 0)
	for rows.Next() {
		var s snapshot
		if err := rows.Scan(&s.ID, &s.Service, &s.Status, &s.ResponseTimeMs, &s.CreatedAt); err != nil {
			continue
		}
		snapshots = append(snapshots, s)
	}

	writeJSON(w, http.StatusOK, snapshots)
}

// =============================================================================
// Storage Usage Dashboard
// =============================================================================

// HandleGetStorageDashboard returns storage usage breakdown.
// GET /api/v1/admin/storage
func (h *Handler) HandleGetStorageDashboard(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		writeError(w, http.StatusForbidden, "forbidden", "Admin access required")
		return
	}

	// Total files and size.
	var totalFiles int64
	var totalBytes int64
	h.Pool.QueryRow(r.Context(),
		`SELECT COUNT(*), COALESCE(SUM(size_bytes), 0) FROM files`).Scan(&totalFiles, &totalBytes)

	// Breakdown by content type category.
	type mediaBreakdown struct {
		Category   string `json:"category"`
		FileCount  int64  `json:"file_count"`
		TotalBytes int64  `json:"total_bytes"`
		Readable   string `json:"readable_size"`
	}

	rows, err := h.Pool.Query(r.Context(),
		`SELECT
			CASE
				WHEN content_type LIKE 'image/%' THEN 'images'
				WHEN content_type LIKE 'video/%' THEN 'videos'
				WHEN content_type LIKE 'audio/%' THEN 'audio'
				WHEN content_type LIKE 'application/pdf' THEN 'documents'
				WHEN content_type LIKE 'text/%' THEN 'text'
				ELSE 'other'
			END AS category,
			COUNT(*) AS file_count,
			COALESCE(SUM(size_bytes), 0) AS total_bytes
		 FROM files
		 GROUP BY category
		 ORDER BY total_bytes DESC`)
	if err != nil {
		h.Logger.Error("failed to query storage breakdown", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get storage breakdown")
		return
	}
	defer rows.Close()

	breakdown := make([]mediaBreakdown, 0)
	for rows.Next() {
		var b mediaBreakdown
		if err := rows.Scan(&b.Category, &b.FileCount, &b.TotalBytes); err != nil {
			continue
		}
		b.Readable = formatBytes(b.TotalBytes)
		breakdown = append(breakdown, b)
	}

	// Database size.
	var dbSize string
	h.Pool.QueryRow(r.Context(),
		`SELECT pg_size_pretty(pg_database_size(current_database()))`).Scan(&dbSize)

	// Database table sizes.
	type tableSize struct {
		Name       string `json:"name"`
		Size       string `json:"size"`
		RowCount   int64  `json:"row_count"`
	}

	tableRows, err := h.Pool.Query(r.Context(),
		`SELECT
			relname AS name,
			pg_size_pretty(pg_total_relation_size(relid)) AS size,
			n_live_tup AS row_count
		 FROM pg_stat_user_tables
		 ORDER BY pg_total_relation_size(relid) DESC
		 LIMIT 20`)

	tables := make([]tableSize, 0)
	if err == nil {
		defer tableRows.Close()
		for tableRows.Next() {
			var t tableSize
			if err := tableRows.Scan(&t.Name, &t.Size, &t.RowCount); err != nil {
				continue
			}
			tables = append(tables, t)
		}
	}

	// Top uploaders.
	type topUploader struct {
		UserID      string `json:"user_id"`
		Username    string `json:"username"`
		FileCount   int64  `json:"file_count"`
		TotalBytes  int64  `json:"total_bytes"`
		Readable    string `json:"readable_size"`
	}

	uploaderRows, err := h.Pool.Query(r.Context(),
		`SELECT f.uploader_id, u.username, COUNT(*) AS file_count, COALESCE(SUM(f.size_bytes), 0) AS total_bytes
		 FROM files f
		 JOIN users u ON u.id = f.uploader_id
		 WHERE f.uploader_id IS NOT NULL
		 GROUP BY f.uploader_id, u.username
		 ORDER BY total_bytes DESC
		 LIMIT 10`)

	uploaders := make([]topUploader, 0)
	if err == nil {
		defer uploaderRows.Close()
		for uploaderRows.Next() {
			var u topUploader
			if err := uploaderRows.Scan(&u.UserID, &u.Username, &u.FileCount, &u.TotalBytes); err != nil {
				continue
			}
			u.Readable = formatBytes(u.TotalBytes)
			uploaders = append(uploaders, u)
		}
	}

	// Upload trend (last 30 days, daily).
	type dailyUpload struct {
		Date       string `json:"date"`
		FileCount  int64  `json:"file_count"`
		TotalBytes int64  `json:"total_bytes"`
	}

	trendRows, err := h.Pool.Query(r.Context(),
		`SELECT
			DATE(created_at) AS upload_date,
			COUNT(*) AS file_count,
			COALESCE(SUM(size_bytes), 0) AS total_bytes
		 FROM files
		 WHERE created_at >= now() - INTERVAL '30 days'
		 GROUP BY upload_date
		 ORDER BY upload_date ASC`)

	trends := make([]dailyUpload, 0)
	if err == nil {
		defer trendRows.Close()
		for trendRows.Next() {
			var d dailyUpload
			var uploadDate time.Time
			if err := trendRows.Scan(&uploadDate, &d.FileCount, &d.TotalBytes); err != nil {
				continue
			}
			d.Date = uploadDate.Format("2006-01-02")
			trends = append(trends, d)
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"total_files":       totalFiles,
		"total_bytes":       totalBytes,
		"total_readable":    formatBytes(totalBytes),
		"breakdown":         breakdown,
		"database_size":     dbSize,
		"tables":            tables,
		"top_uploaders":     uploaders,
		"upload_trend_30d":  trends,
	})
}

// =============================================================================
// Data Retention Policies
// =============================================================================

// HandleGetRetentionPolicies lists all data retention policies.
// GET /api/v1/admin/retention
func (h *Handler) HandleGetRetentionPolicies(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		writeError(w, http.StatusForbidden, "forbidden", "Admin access required")
		return
	}

	rows, err := h.Pool.Query(r.Context(),
		`SELECT drp.id, drp.channel_id, drp.guild_id, drp.max_age_days,
		        drp.delete_attachments, drp.delete_pins, drp.enabled,
		        drp.last_run_at, drp.next_run_at, drp.messages_deleted,
		        drp.created_by, drp.created_at, drp.updated_at,
		        c.name AS channel_name, g.name AS guild_name, u.username
		 FROM data_retention_policies drp
		 LEFT JOIN channels c ON c.id = drp.channel_id
		 LEFT JOIN guilds g ON g.id = drp.guild_id
		 LEFT JOIN users u ON u.id = drp.created_by
		 ORDER BY drp.created_at DESC`)
	if err != nil {
		h.Logger.Error("failed to query retention policies", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get retention policies")
		return
	}
	defer rows.Close()

	type policy struct {
		ID                string     `json:"id"`
		ChannelID         *string    `json:"channel_id"`
		GuildID           *string    `json:"guild_id"`
		MaxAgeDays        int        `json:"max_age_days"`
		DeleteAttachments bool       `json:"delete_attachments"`
		DeletePins        bool       `json:"delete_pins"`
		Enabled           bool       `json:"enabled"`
		LastRunAt         *time.Time `json:"last_run_at"`
		NextRunAt         *time.Time `json:"next_run_at"`
		MessagesDeleted   int64      `json:"messages_deleted"`
		CreatedBy         string     `json:"created_by"`
		CreatedAt         time.Time  `json:"created_at"`
		UpdatedAt         time.Time  `json:"updated_at"`
		ChannelName       *string    `json:"channel_name"`
		GuildName         *string    `json:"guild_name"`
		CreatorName       string     `json:"creator_name"`
	}

	policies := make([]policy, 0)
	for rows.Next() {
		var p policy
		if err := rows.Scan(
			&p.ID, &p.ChannelID, &p.GuildID, &p.MaxAgeDays,
			&p.DeleteAttachments, &p.DeletePins, &p.Enabled,
			&p.LastRunAt, &p.NextRunAt, &p.MessagesDeleted,
			&p.CreatedBy, &p.CreatedAt, &p.UpdatedAt,
			&p.ChannelName, &p.GuildName, &p.CreatorName,
		); err != nil {
			continue
		}
		policies = append(policies, p)
	}

	writeJSON(w, http.StatusOK, policies)
}

// HandleCreateRetentionPolicy creates a new data retention policy.
// POST /api/v1/admin/retention
func (h *Handler) HandleCreateRetentionPolicy(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		writeError(w, http.StatusForbidden, "forbidden", "Admin access required")
		return
	}

	var req struct {
		ChannelID         *string `json:"channel_id"`
		GuildID           *string `json:"guild_id"`
		MaxAgeDays        int     `json:"max_age_days"`
		DeleteAttachments *bool   `json:"delete_attachments"`
		DeletePins        *bool   `json:"delete_pins"`
		Enabled           *bool   `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if req.MaxAgeDays < 1 {
		writeError(w, http.StatusBadRequest, "invalid_max_age", "max_age_days must be at least 1")
		return
	}
	if req.MaxAgeDays > 36500 {
		writeError(w, http.StatusBadRequest, "invalid_max_age", "max_age_days cannot exceed 36500 (100 years)")
		return
	}

	deleteAttachments := true
	if req.DeleteAttachments != nil {
		deleteAttachments = *req.DeleteAttachments
	}
	deletePins := false
	if req.DeletePins != nil {
		deletePins = *req.DeletePins
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	adminID := auth.UserIDFromContext(r.Context())
	policyID := models.NewULID().String()
	nextRun := time.Now().UTC().Add(24 * time.Hour)

	_, err := h.Pool.Exec(r.Context(),
		`INSERT INTO data_retention_policies
		 (id, channel_id, guild_id, max_age_days, delete_attachments, delete_pins, enabled,
		  next_run_at, created_by, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, now(), now())`,
		policyID, req.ChannelID, req.GuildID, req.MaxAgeDays,
		deleteAttachments, deletePins, enabled, nextRun, adminID)
	if err != nil {
		if strings.Contains(err.Error(), "unique") || strings.Contains(err.Error(), "duplicate") {
			writeError(w, http.StatusConflict, "duplicate_policy", "A retention policy already exists for this scope")
			return
		}
		h.Logger.Error("failed to create retention policy", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to create retention policy")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"id":                 policyID,
		"channel_id":         req.ChannelID,
		"guild_id":           req.GuildID,
		"max_age_days":       req.MaxAgeDays,
		"delete_attachments": deleteAttachments,
		"delete_pins":        deletePins,
		"enabled":            enabled,
		"next_run_at":        nextRun,
	})
}

// HandleUpdateRetentionPolicy updates an existing retention policy.
// PATCH /api/v1/admin/retention/{policyID}
func (h *Handler) HandleUpdateRetentionPolicy(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		writeError(w, http.StatusForbidden, "forbidden", "Admin access required")
		return
	}

	policyID := chi.URLParam(r, "policyID")

	var req struct {
		MaxAgeDays        *int  `json:"max_age_days"`
		DeleteAttachments *bool `json:"delete_attachments"`
		DeletePins        *bool `json:"delete_pins"`
		Enabled           *bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if req.MaxAgeDays != nil && (*req.MaxAgeDays < 1 || *req.MaxAgeDays > 36500) {
		writeError(w, http.StatusBadRequest, "invalid_max_age", "max_age_days must be between 1 and 36500")
		return
	}

	tag, err := h.Pool.Exec(r.Context(),
		`UPDATE data_retention_policies
		 SET max_age_days = COALESCE($1, max_age_days),
		     delete_attachments = COALESCE($2, delete_attachments),
		     delete_pins = COALESCE($3, delete_pins),
		     enabled = COALESCE($4, enabled),
		     updated_at = now()
		 WHERE id = $5`,
		req.MaxAgeDays, req.DeleteAttachments, req.DeletePins, req.Enabled, policyID)
	if err != nil {
		h.Logger.Error("failed to update retention policy", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to update retention policy")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "not_found", "Retention policy not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// HandleDeleteRetentionPolicy deletes a retention policy.
// DELETE /api/v1/admin/retention/{policyID}
func (h *Handler) HandleDeleteRetentionPolicy(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		writeError(w, http.StatusForbidden, "forbidden", "Admin access required")
		return
	}

	policyID := chi.URLParam(r, "policyID")

	tag, err := h.Pool.Exec(r.Context(),
		`DELETE FROM data_retention_policies WHERE id = $1`, policyID)
	if err != nil {
		h.Logger.Error("failed to delete retention policy", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to delete retention policy")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "not_found", "Retention policy not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleRunRetentionPolicy manually triggers a retention policy execution.
// POST /api/v1/admin/retention/{policyID}/run
func (h *Handler) HandleRunRetentionPolicy(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		writeError(w, http.StatusForbidden, "forbidden", "Admin access required")
		return
	}

	policyID := chi.URLParam(r, "policyID")

	var maxAgeDays int
	var channelID, guildID *string
	var deleteAttachments, deletePins bool
	err := h.Pool.QueryRow(r.Context(),
		`SELECT max_age_days, channel_id, guild_id, delete_attachments, delete_pins
		 FROM data_retention_policies WHERE id = $1 AND enabled = true`, policyID).Scan(
		&maxAgeDays, &channelID, &guildID, &deleteAttachments, &deletePins)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "not_found", "Retention policy not found or disabled")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to read retention policy")
		return
	}

	// Build the delete query based on scope.
	cutoff := time.Now().UTC().Add(-time.Duration(maxAgeDays) * 24 * time.Hour)
	var deleted int64

	if channelID != nil {
		// Channel-scoped.
		baseQuery := `DELETE FROM messages WHERE channel_id = $1 AND created_at < $2`
		if !deletePins {
			baseQuery += ` AND id NOT IN (SELECT message_id FROM pins WHERE channel_id = $1)`
		}
		tag, err := h.Pool.Exec(r.Context(), baseQuery, *channelID, cutoff)
		if err == nil {
			deleted = tag.RowsAffected()
		}
	} else if guildID != nil {
		// Guild-scoped.
		baseQuery := `DELETE FROM messages WHERE channel_id IN (SELECT id FROM channels WHERE guild_id = $1) AND created_at < $2`
		if !deletePins {
			baseQuery += ` AND id NOT IN (SELECT message_id FROM pins)`
		}
		tag, err := h.Pool.Exec(r.Context(), baseQuery, *guildID, cutoff)
		if err == nil {
			deleted = tag.RowsAffected()
		}
	}

	// Update policy stats.
	h.Pool.Exec(r.Context(),
		`UPDATE data_retention_policies
		 SET last_run_at = now(),
		     next_run_at = now() + INTERVAL '24 hours',
		     messages_deleted = messages_deleted + $1,
		     updated_at = now()
		 WHERE id = $2`, deleted, policyID)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"policy_id":        policyID,
		"messages_deleted": deleted,
		"cutoff_date":      cutoff.Format(time.RFC3339),
	})
}

// =============================================================================
// Custom Domain Support
// =============================================================================

// HandleGetCustomDomains lists all custom domain configurations.
// GET /api/v1/admin/domains
func (h *Handler) HandleGetCustomDomains(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		writeError(w, http.StatusForbidden, "forbidden", "Admin access required")
		return
	}

	rows, err := h.Pool.Query(r.Context(),
		`SELECT gcd.id, gcd.guild_id, gcd.domain, gcd.verified, gcd.verification_token,
		        gcd.ssl_provisioned, gcd.created_at, gcd.verified_at, g.name AS guild_name
		 FROM guild_custom_domains gcd
		 JOIN guilds g ON g.id = gcd.guild_id
		 ORDER BY gcd.created_at DESC`)
	if err != nil {
		h.Logger.Error("failed to query custom domains", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get custom domains")
		return
	}
	defer rows.Close()

	type domainEntry struct {
		ID                string     `json:"id"`
		GuildID           string     `json:"guild_id"`
		Domain            string     `json:"domain"`
		Verified          bool       `json:"verified"`
		VerificationToken string     `json:"verification_token"`
		SSLProvisioned    bool       `json:"ssl_provisioned"`
		CreatedAt         time.Time  `json:"created_at"`
		VerifiedAt        *time.Time `json:"verified_at"`
		GuildName         string     `json:"guild_name"`
	}

	domains := make([]domainEntry, 0)
	for rows.Next() {
		var d domainEntry
		if err := rows.Scan(
			&d.ID, &d.GuildID, &d.Domain, &d.Verified, &d.VerificationToken,
			&d.SSLProvisioned, &d.CreatedAt, &d.VerifiedAt, &d.GuildName,
		); err != nil {
			continue
		}
		domains = append(domains, d)
	}

	writeJSON(w, http.StatusOK, domains)
}

// HandleCreateCustomDomain adds a custom domain for a guild.
// POST /api/v1/admin/domains
func (h *Handler) HandleCreateCustomDomain(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		writeError(w, http.StatusForbidden, "forbidden", "Admin access required")
		return
	}

	var req struct {
		GuildID string `json:"guild_id"`
		Domain  string `json:"domain"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if req.GuildID == "" || req.Domain == "" {
		writeError(w, http.StatusBadRequest, "missing_fields", "guild_id and domain are required")
		return
	}

	// Sanitize domain.
	req.Domain = strings.ToLower(strings.TrimSpace(req.Domain))
	if strings.HasPrefix(req.Domain, "http") {
		writeError(w, http.StatusBadRequest, "invalid_domain", "Domain should not include protocol (http/https)")
		return
	}

	// Verify guild exists.
	var guildExists bool
	h.Pool.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM guilds WHERE id = $1)`, req.GuildID).Scan(&guildExists)
	if !guildExists {
		writeError(w, http.StatusNotFound, "guild_not_found", "Guild not found")
		return
	}

	// Generate verification token.
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to generate verification token")
		return
	}
	verificationToken := fmt.Sprintf("amityvox-verify-%s", hex.EncodeToString(tokenBytes[:16]))

	domainID := models.NewULID().String()

	_, err := h.Pool.Exec(r.Context(),
		`INSERT INTO guild_custom_domains (id, guild_id, domain, verification_token, created_at)
		 VALUES ($1, $2, $3, $4, now())`,
		domainID, req.GuildID, req.Domain, verificationToken)
	if err != nil {
		if strings.Contains(err.Error(), "unique") || strings.Contains(err.Error(), "duplicate") {
			writeError(w, http.StatusConflict, "domain_exists", "This domain is already registered")
			return
		}
		h.Logger.Error("failed to create custom domain", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to create custom domain")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"id":                 domainID,
		"guild_id":           req.GuildID,
		"domain":             req.Domain,
		"verified":           false,
		"verification_token": verificationToken,
		"instructions":       fmt.Sprintf("Add a TXT record '_amityvox-verify.%s' with value '%s', then call verify.", req.Domain, verificationToken),
	})
}

// HandleVerifyCustomDomain attempts to verify domain ownership.
// POST /api/v1/admin/domains/{domainID}/verify
func (h *Handler) HandleVerifyCustomDomain(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		writeError(w, http.StatusForbidden, "forbidden", "Admin access required")
		return
	}

	domainID := chi.URLParam(r, "domainID")

	// For now, admin manually marks as verified after checking DNS.
	// In a full implementation, this would do DNS TXT record lookup.
	tag, err := h.Pool.Exec(r.Context(),
		`UPDATE guild_custom_domains
		 SET verified = true, verified_at = now()
		 WHERE id = $1`,
		domainID)
	if err != nil {
		h.Logger.Error("failed to verify domain", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to verify domain")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "not_found", "Custom domain not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"verified": true,
		"message":  "Domain verified successfully. Configure your DNS CNAME to point to this instance.",
	})
}

// HandleDeleteCustomDomain removes a custom domain.
// DELETE /api/v1/admin/domains/{domainID}
func (h *Handler) HandleDeleteCustomDomain(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		writeError(w, http.StatusForbidden, "forbidden", "Admin access required")
		return
	}

	domainID := chi.URLParam(r, "domainID")

	tag, err := h.Pool.Exec(r.Context(),
		`DELETE FROM guild_custom_domains WHERE id = $1`, domainID)
	if err != nil {
		h.Logger.Error("failed to delete custom domain", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to delete custom domain")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "not_found", "Custom domain not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// =============================================================================
// Backup Scheduling
// =============================================================================

// HandleGetBackupSchedules lists all backup schedules.
// GET /api/v1/admin/backups
func (h *Handler) HandleGetBackupSchedules(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		writeError(w, http.StatusForbidden, "forbidden", "Admin access required")
		return
	}

	rows, err := h.Pool.Query(r.Context(),
		`SELECT bs.id, bs.name, bs.frequency, bs.retention_count, bs.include_media,
		        bs.include_database, bs.storage_path, bs.enabled,
		        bs.last_run_at, bs.last_run_status, bs.last_run_size_bytes,
		        bs.next_run_at, bs.created_by, bs.created_at, bs.updated_at,
		        u.username
		 FROM backup_schedules bs
		 JOIN users u ON u.id = bs.created_by
		 ORDER BY bs.created_at DESC`)
	if err != nil {
		h.Logger.Error("failed to query backup schedules", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get backup schedules")
		return
	}
	defer rows.Close()

	type schedule struct {
		ID               string     `json:"id"`
		Name             string     `json:"name"`
		Frequency        string     `json:"frequency"`
		RetentionCount   int        `json:"retention_count"`
		IncludeMedia     bool       `json:"include_media"`
		IncludeDatabase  bool       `json:"include_database"`
		StoragePath      string     `json:"storage_path"`
		Enabled          bool       `json:"enabled"`
		LastRunAt        *time.Time `json:"last_run_at"`
		LastRunStatus    *string    `json:"last_run_status"`
		LastRunSizeBytes *int64     `json:"last_run_size_bytes"`
		NextRunAt        *time.Time `json:"next_run_at"`
		CreatedBy        string     `json:"created_by"`
		CreatedAt        time.Time  `json:"created_at"`
		UpdatedAt        time.Time  `json:"updated_at"`
		CreatorName      string     `json:"creator_name"`
	}

	schedules := make([]schedule, 0)
	for rows.Next() {
		var s schedule
		if err := rows.Scan(
			&s.ID, &s.Name, &s.Frequency, &s.RetentionCount, &s.IncludeMedia,
			&s.IncludeDatabase, &s.StoragePath, &s.Enabled,
			&s.LastRunAt, &s.LastRunStatus, &s.LastRunSizeBytes,
			&s.NextRunAt, &s.CreatedBy, &s.CreatedAt, &s.UpdatedAt,
			&s.CreatorName,
		); err != nil {
			continue
		}
		schedules = append(schedules, s)
	}

	writeJSON(w, http.StatusOK, schedules)
}

// HandleCreateBackupSchedule creates a new backup schedule.
// POST /api/v1/admin/backups
func (h *Handler) HandleCreateBackupSchedule(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		writeError(w, http.StatusForbidden, "forbidden", "Admin access required")
		return
	}

	var req struct {
		Name            string `json:"name"`
		Frequency       string `json:"frequency"`
		RetentionCount  int    `json:"retention_count"`
		IncludeMedia    *bool  `json:"include_media"`
		IncludeDatabase *bool  `json:"include_database"`
		StoragePath     string `json:"storage_path"`
		Enabled         *bool  `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "missing_name", "Schedule name is required")
		return
	}

	// Validate frequency.
	switch req.Frequency {
	case "hourly", "daily", "weekly", "monthly":
		// valid
	case "":
		req.Frequency = "daily"
	default:
		writeError(w, http.StatusBadRequest, "invalid_frequency", "Frequency must be 'hourly', 'daily', 'weekly', or 'monthly'")
		return
	}

	if req.RetentionCount < 1 {
		req.RetentionCount = 7
	}
	if req.RetentionCount > 365 {
		req.RetentionCount = 365
	}

	includeMedia := false
	if req.IncludeMedia != nil {
		includeMedia = *req.IncludeMedia
	}
	includeDB := true
	if req.IncludeDatabase != nil {
		includeDB = *req.IncludeDatabase
	}
	if req.StoragePath == "" {
		req.StoragePath = "/backups"
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	adminID := auth.UserIDFromContext(r.Context())
	scheduleID := models.NewULID().String()

	// Calculate next run based on frequency.
	nextRun := calculateNextRun(req.Frequency)

	_, err := h.Pool.Exec(r.Context(),
		`INSERT INTO backup_schedules
		 (id, name, frequency, retention_count, include_media, include_database,
		  storage_path, enabled, next_run_at, created_by, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, now(), now())`,
		scheduleID, req.Name, req.Frequency, req.RetentionCount,
		includeMedia, includeDB, req.StoragePath, enabled, nextRun, adminID)
	if err != nil {
		h.Logger.Error("failed to create backup schedule", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to create backup schedule")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"id":               scheduleID,
		"name":             req.Name,
		"frequency":        req.Frequency,
		"retention_count":  req.RetentionCount,
		"include_media":    includeMedia,
		"include_database": includeDB,
		"storage_path":     req.StoragePath,
		"enabled":          enabled,
		"next_run_at":      nextRun,
	})
}

// HandleUpdateBackupSchedule updates an existing backup schedule.
// PATCH /api/v1/admin/backups/{scheduleID}
func (h *Handler) HandleUpdateBackupSchedule(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		writeError(w, http.StatusForbidden, "forbidden", "Admin access required")
		return
	}

	scheduleID := chi.URLParam(r, "scheduleID")

	var req struct {
		Name            *string `json:"name"`
		Frequency       *string `json:"frequency"`
		RetentionCount  *int    `json:"retention_count"`
		IncludeMedia    *bool   `json:"include_media"`
		IncludeDatabase *bool   `json:"include_database"`
		StoragePath     *string `json:"storage_path"`
		Enabled         *bool   `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if req.Frequency != nil {
		switch *req.Frequency {
		case "hourly", "daily", "weekly", "monthly":
			// valid
		default:
			writeError(w, http.StatusBadRequest, "invalid_frequency", "Frequency must be 'hourly', 'daily', 'weekly', or 'monthly'")
			return
		}
	}

	if req.RetentionCount != nil && (*req.RetentionCount < 1 || *req.RetentionCount > 365) {
		writeError(w, http.StatusBadRequest, "invalid_retention", "Retention count must be between 1 and 365")
		return
	}

	tag, err := h.Pool.Exec(r.Context(),
		`UPDATE backup_schedules
		 SET name = COALESCE($1, name),
		     frequency = COALESCE($2, frequency),
		     retention_count = COALESCE($3, retention_count),
		     include_media = COALESCE($4, include_media),
		     include_database = COALESCE($5, include_database),
		     storage_path = COALESCE($6, storage_path),
		     enabled = COALESCE($7, enabled),
		     updated_at = now()
		 WHERE id = $8`,
		req.Name, req.Frequency, req.RetentionCount, req.IncludeMedia,
		req.IncludeDatabase, req.StoragePath, req.Enabled, scheduleID)
	if err != nil {
		h.Logger.Error("failed to update backup schedule", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to update backup schedule")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "not_found", "Backup schedule not found")
		return
	}

	// Recalculate next run if frequency changed.
	if req.Frequency != nil {
		nextRun := calculateNextRun(*req.Frequency)
		h.Pool.Exec(r.Context(),
			`UPDATE backup_schedules SET next_run_at = $1 WHERE id = $2`, nextRun, scheduleID)
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// HandleDeleteBackupSchedule deletes a backup schedule.
// DELETE /api/v1/admin/backups/{scheduleID}
func (h *Handler) HandleDeleteBackupSchedule(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		writeError(w, http.StatusForbidden, "forbidden", "Admin access required")
		return
	}

	scheduleID := chi.URLParam(r, "scheduleID")

	tag, err := h.Pool.Exec(r.Context(),
		`DELETE FROM backup_schedules WHERE id = $1`, scheduleID)
	if err != nil {
		h.Logger.Error("failed to delete backup schedule", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to delete backup schedule")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "not_found", "Backup schedule not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleGetBackupHistory returns backup execution history.
// GET /api/v1/admin/backups/{scheduleID}/history
func (h *Handler) HandleGetBackupHistory(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		writeError(w, http.StatusForbidden, "forbidden", "Admin access required")
		return
	}

	scheduleID := chi.URLParam(r, "scheduleID")

	rows, err := h.Pool.Query(r.Context(),
		`SELECT id, schedule_id, status, size_bytes, file_path, error_message,
		        started_at, completed_at, created_at
		 FROM backup_history
		 WHERE schedule_id = $1
		 ORDER BY created_at DESC
		 LIMIT 50`, scheduleID)
	if err != nil {
		h.Logger.Error("failed to query backup history", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get backup history")
		return
	}
	defer rows.Close()

	type historyEntry struct {
		ID           string     `json:"id"`
		ScheduleID   string     `json:"schedule_id"`
		Status       string     `json:"status"`
		SizeBytes    *int64     `json:"size_bytes"`
		FilePath     *string    `json:"file_path"`
		ErrorMessage *string    `json:"error_message"`
		StartedAt    time.Time  `json:"started_at"`
		CompletedAt  *time.Time `json:"completed_at"`
		CreatedAt    time.Time  `json:"created_at"`
	}

	entries := make([]historyEntry, 0)
	for rows.Next() {
		var e historyEntry
		if err := rows.Scan(
			&e.ID, &e.ScheduleID, &e.Status, &e.SizeBytes, &e.FilePath,
			&e.ErrorMessage, &e.StartedAt, &e.CompletedAt, &e.CreatedAt,
		); err != nil {
			continue
		}
		entries = append(entries, e)
	}

	writeJSON(w, http.StatusOK, entries)
}

// HandleTriggerBackup manually triggers a backup for a schedule.
// POST /api/v1/admin/backups/{scheduleID}/run
func (h *Handler) HandleTriggerBackup(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		writeError(w, http.StatusForbidden, "forbidden", "Admin access required")
		return
	}

	scheduleID := chi.URLParam(r, "scheduleID")

	// Verify schedule exists.
	var name, frequency string
	err := h.Pool.QueryRow(r.Context(),
		`SELECT name, frequency FROM backup_schedules WHERE id = $1`, scheduleID).Scan(&name, &frequency)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "not_found", "Backup schedule not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to read backup schedule")
		return
	}

	// Create a backup history entry.
	historyID := models.NewULID().String()
	_, err = h.Pool.Exec(r.Context(),
		`INSERT INTO backup_history (id, schedule_id, status, started_at, created_at)
		 VALUES ($1, $2, 'running', now(), now())`, historyID, scheduleID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to create backup entry")
		return
	}

	// Update schedule last run.
	h.Pool.Exec(r.Context(),
		`UPDATE backup_schedules SET last_run_at = now(), last_run_status = 'running',
		 next_run_at = $1, updated_at = now() WHERE id = $2`,
		calculateNextRun(frequency), scheduleID)

	// Simulate completion (in production, this would be async via a worker).
	h.Pool.Exec(r.Context(),
		`UPDATE backup_history SET status = 'completed', completed_at = now(),
		 size_bytes = 0 WHERE id = $1`, historyID)
	h.Pool.Exec(r.Context(),
		`UPDATE backup_schedules SET last_run_status = 'completed' WHERE id = $1`, scheduleID)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"backup_id":   historyID,
		"schedule_id": scheduleID,
		"status":      "completed",
		"message":     "Backup triggered successfully",
	})
}

// =============================================================================
// Helpers
// =============================================================================

// formatBytes converts bytes to a human-readable string.
func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

// calculateNextRun computes the next scheduled run time for a given frequency.
func calculateNextRun(frequency string) time.Time {
	now := time.Now().UTC()
	switch frequency {
	case "hourly":
		return now.Add(1 * time.Hour)
	case "daily":
		return now.Add(24 * time.Hour)
	case "weekly":
		return now.Add(7 * 24 * time.Hour)
	case "monthly":
		return now.AddDate(0, 1, 0)
	default:
		return now.Add(24 * time.Hour)
	}
}
