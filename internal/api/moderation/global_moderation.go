package moderation

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/amityvox/amityvox/internal/auth"
	"github.com/amityvox/amityvox/internal/models"
)

// isGlobalModOrAdmin checks if the requesting user has the global moderator or admin flag.
func (h *Handler) isGlobalModOrAdmin(r *http.Request) bool {
	userID := auth.UserIDFromContext(r.Context())
	var flags int
	err := h.Pool.QueryRow(r.Context(),
		`SELECT flags FROM users WHERE id = $1`, userID).Scan(&flags)
	if err != nil {
		return false
	}
	return flags&(models.UserFlagAdmin|models.UserFlagGlobalMod) != 0
}

// HandleReportUser handles POST /api/v1/users/{userID}/report.
// Any authenticated user can report another user.
func (h *Handler) HandleReportUser(w http.ResponseWriter, r *http.Request) {
	reporterID := auth.UserIDFromContext(r.Context())
	reportedUserID := chi.URLParam(r, "userID")

	if reporterID == reportedUserID {
		writeError(w, http.StatusBadRequest, "invalid_report", "You cannot report yourself")
		return
	}

	var req struct {
		Reason           string  `json:"reason"`
		ContextGuildID   *string `json:"context_guild_id"`
		ContextChannelID *string `json:"context_channel_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	if req.Reason == "" {
		writeError(w, http.StatusBadRequest, "missing_reason", "Reason is required")
		return
	}
	if req.ContextGuildID != nil && *req.ContextGuildID == "" {
		req.ContextGuildID = nil
	}
	if req.ContextChannelID != nil && *req.ContextChannelID == "" {
		req.ContextChannelID = nil
	}

	// Verify reported user exists.
	var exists bool
	if err := h.Pool.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM users WHERE id = $1)`, reportedUserID).Scan(&exists); err != nil {
		h.Logger.Error("failed to check user existence", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to verify user")
		return
	}
	if !exists {
		writeError(w, http.StatusNotFound, "user_not_found", "User not found")
		return
	}

	id := models.NewULID().String()
	_, err := h.Pool.Exec(r.Context(),
		`INSERT INTO user_reports (id, reporter_id, reported_user_id, reason, context_guild_id, context_channel_id)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		id, reporterID, reportedUserID, req.Reason, req.ContextGuildID, req.ContextChannelID)
	if err != nil {
		h.Logger.Error("failed to create user report", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to create report")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{"id": id, "status": "open"})
}

// HandleGetUserReports handles GET /api/v1/moderation/user-reports?status=
// Only global moderators and admins can access this.
func (h *Handler) HandleGetUserReports(w http.ResponseWriter, r *http.Request) {
	if !h.isGlobalModOrAdmin(r) {
		writeError(w, http.StatusForbidden, "forbidden", "Global moderator or admin access required")
		return
	}

	status := r.URL.Query().Get("status")
	var reports []models.UserReport

	query := `SELECT ur.id, ur.reporter_id, ur.reported_user_id, ur.reason,
		ur.context_guild_id, ur.context_channel_id, ur.status,
		ur.resolved_by, ur.resolved_at, ur.notes, ur.created_at,
		reporter.username AS reporter_name, reported.username AS reported_user_name
		FROM user_reports ur
		JOIN users reporter ON reporter.id = ur.reporter_id
		JOIN users reported ON reported.id = ur.reported_user_id`

	var rows pgx.Rows
	var qerr error
	if status != "" {
		query += ` WHERE ur.status = $1 ORDER BY ur.created_at DESC LIMIT 100`
		rows, qerr = h.Pool.Query(r.Context(), query, status)
	} else {
		query += ` ORDER BY ur.created_at DESC LIMIT 100`
		rows, qerr = h.Pool.Query(r.Context(), query)
	}
	if qerr != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to fetch reports")
		return
	}
	defer rows.Close()
	for rows.Next() {
		var rpt models.UserReport
		if err := rows.Scan(&rpt.ID, &rpt.ReporterID, &rpt.ReportedUserID, &rpt.Reason,
			&rpt.ContextGuildID, &rpt.ContextChannelID, &rpt.Status,
			&rpt.ResolvedBy, &rpt.ResolvedAt, &rpt.Notes, &rpt.CreatedAt,
			&rpt.ReporterName, &rpt.ReportedUserName); err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "Failed to read reports")
			return
		}
		reports = append(reports, rpt)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to read reports")
		return
	}

	if reports == nil {
		reports = []models.UserReport{}
	}
	writeJSON(w, http.StatusOK, reports)
}

// HandleResolveUserReport handles PATCH /api/v1/moderation/user-reports/{reportID}.
func (h *Handler) HandleResolveUserReport(w http.ResponseWriter, r *http.Request) {
	if !h.isGlobalModOrAdmin(r) {
		writeError(w, http.StatusForbidden, "forbidden", "Global moderator or admin access required")
		return
	}

	reportID := chi.URLParam(r, "reportID")
	modID := auth.UserIDFromContext(r.Context())

	var req struct {
		Status string  `json:"status"`
		Notes  *string `json:"notes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if req.Status != "resolved" && req.Status != "dismissed" {
		writeError(w, http.StatusBadRequest, "invalid_status", "Status must be 'resolved' or 'dismissed'")
		return
	}

	now := time.Now()
	tag, err := h.Pool.Exec(r.Context(),
		`UPDATE user_reports SET status = $1, resolved_by = $2, resolved_at = $3, notes = $4 WHERE id = $5`,
		req.Status, modID, now, req.Notes, reportID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to resolve report")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "not_found", "Report not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"status": req.Status})
}

// HandleCreateIssue handles POST /api/v1/issues.
// Any authenticated user can submit an issue.
func (h *Handler) HandleCreateIssue(w http.ResponseWriter, r *http.Request) {
	reporterID := auth.UserIDFromContext(r.Context())

	var req struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		Category    string `json:"category"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	if req.Title == "" || req.Description == "" {
		writeError(w, http.StatusBadRequest, "missing_fields", "Title and description are required")
		return
	}
	if req.Category == "" {
		req.Category = "general"
	}
	validCategories := map[string]bool{"general": true, "bug": true, "abuse": true, "suggestion": true}
	if !validCategories[req.Category] {
		writeError(w, http.StatusBadRequest, "invalid_category", "Category must be one of: general, bug, abuse, suggestion")
		return
	}

	id := models.NewULID().String()
	_, err := h.Pool.Exec(r.Context(),
		`INSERT INTO reported_issues (id, reporter_id, title, description, category) VALUES ($1, $2, $3, $4, $5)`,
		id, reporterID, req.Title, req.Description, req.Category)
	if err != nil {
		h.Logger.Error("failed to create issue", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to create issue")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{"id": id, "status": "open"})
}

// HandleGetIssues handles GET /api/v1/moderation/issues?status=
func (h *Handler) HandleGetIssues(w http.ResponseWriter, r *http.Request) {
	if !h.isGlobalModOrAdmin(r) {
		writeError(w, http.StatusForbidden, "forbidden", "Global moderator or admin access required")
		return
	}

	status := r.URL.Query().Get("status")
	var issues []models.ReportedIssue

	query := `SELECT ri.id, ri.reporter_id, ri.title, ri.description, ri.category,
		ri.status, ri.resolved_by, ri.resolved_at, ri.notes, ri.created_at,
		u.username AS reporter_name
		FROM reported_issues ri
		JOIN users u ON u.id = ri.reporter_id`

	var irows pgx.Rows
	var ierr error
	if status != "" {
		query += ` WHERE ri.status = $1 ORDER BY ri.created_at DESC LIMIT 100`
		irows, ierr = h.Pool.Query(r.Context(), query, status)
	} else {
		query += ` ORDER BY ri.created_at DESC LIMIT 100`
		irows, ierr = h.Pool.Query(r.Context(), query)
	}
	if ierr != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to fetch issues")
		return
	}
	defer irows.Close()
	for irows.Next() {
		var issue models.ReportedIssue
		if err := irows.Scan(&issue.ID, &issue.ReporterID, &issue.Title, &issue.Description,
			&issue.Category, &issue.Status, &issue.ResolvedBy, &issue.ResolvedAt,
			&issue.Notes, &issue.CreatedAt, &issue.ReporterName); err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "Failed to read issues")
			return
		}
		issues = append(issues, issue)
	}
	if err := irows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to read issues")
		return
	}

	if issues == nil {
		issues = []models.ReportedIssue{}
	}
	writeJSON(w, http.StatusOK, issues)
}

// HandleResolveIssue handles PATCH /api/v1/moderation/issues/{issueID}.
func (h *Handler) HandleResolveIssue(w http.ResponseWriter, r *http.Request) {
	if !h.isGlobalModOrAdmin(r) {
		writeError(w, http.StatusForbidden, "forbidden", "Global moderator or admin access required")
		return
	}

	issueID := chi.URLParam(r, "issueID")
	modID := auth.UserIDFromContext(r.Context())

	var req struct {
		Status string  `json:"status"`
		Notes  *string `json:"notes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	validStatuses := map[string]bool{"in_progress": true, "resolved": true, "dismissed": true}
	if !validStatuses[req.Status] {
		writeError(w, http.StatusBadRequest, "invalid_status", "Status must be 'in_progress', 'resolved', or 'dismissed'")
		return
	}

	now := time.Now()
	var resolvedAt *time.Time
	var resolvedBy *string
	if req.Status == "resolved" || req.Status == "dismissed" {
		resolvedAt = &now
		resolvedBy = &modID
	}

	tag, err := h.Pool.Exec(r.Context(),
		`UPDATE reported_issues SET status = $1, resolved_by = $2, resolved_at = $3, notes = $4 WHERE id = $5`,
		req.Status, resolvedBy, resolvedAt, req.Notes, issueID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to update issue")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "not_found", "Issue not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"status": req.Status})
}

// HandleGetModerationStats handles GET /api/v1/moderation/stats.
func (h *Handler) HandleGetModerationStats(w http.ResponseWriter, r *http.Request) {
	if !h.isGlobalModOrAdmin(r) {
		writeError(w, http.StatusForbidden, "forbidden", "Global moderator or admin access required")
		return
	}

	var stats models.ModerationStats

	if err := h.Pool.QueryRow(r.Context(),
		`SELECT COUNT(*) FROM message_reports WHERE status = 'admin_pending'`).Scan(&stats.OpenMessageReports); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to load moderation stats")
		return
	}
	if err := h.Pool.QueryRow(r.Context(),
		`SELECT COUNT(*) FROM user_reports WHERE status = 'open'`).Scan(&stats.OpenUserReports); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to load moderation stats")
		return
	}
	if err := h.Pool.QueryRow(r.Context(),
		`SELECT COUNT(*) FROM reported_issues WHERE status IN ('open', 'in_progress')`).Scan(&stats.OpenIssues); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to load moderation stats")
		return
	}

	writeJSON(w, http.StatusOK, stats)
}

// HandleGetAllMessageReports handles GET /api/v1/moderation/message-reports?status=
// Reuses existing message_reports table, filtered to admin_pending reports.
func (h *Handler) HandleGetAllMessageReports(w http.ResponseWriter, r *http.Request) {
	if !h.isGlobalModOrAdmin(r) {
		writeError(w, http.StatusForbidden, "forbidden", "Global moderator or admin access required")
		return
	}

	status := r.URL.Query().Get("status")
	if status == "" {
		status = "admin_pending"
	}

	rows, err := h.Pool.Query(r.Context(),
		`SELECT mr.id, mr.guild_id, mr.channel_id, mr.message_id, mr.reporter_id,
			mr.reason, mr.status, mr.resolved_by, mr.resolved_at, mr.created_at,
			reporter.username AS reporter_name
		FROM message_reports mr
		JOIN users reporter ON reporter.id = mr.reporter_id
		WHERE mr.status = $1
		ORDER BY mr.created_at DESC LIMIT 100`, status)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to fetch message reports")
		return
	}
	defer rows.Close()

	type messageReportRow struct {
		models.MessageReport
		ReporterName *string `json:"reporter_name,omitempty"`
	}

	var reports []messageReportRow
	for rows.Next() {
		var rpt messageReportRow
		if err := rows.Scan(&rpt.ID, &rpt.GuildID, &rpt.ChannelID, &rpt.MessageID,
			&rpt.ReporterID, &rpt.Reason, &rpt.Status, &rpt.ResolvedBy,
			&rpt.ResolvedAt, &rpt.CreatedAt, &rpt.ReporterName); err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "Failed to read message reports")
			return
		}
		reports = append(reports, rpt)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to read message reports")
		return
	}

	if reports == nil {
		reports = []messageReportRow{}
	}
	writeJSON(w, http.StatusOK, reports)
}

// HandleResolveMessageReport handles PATCH /api/v1/moderation/message-reports/{reportID}.
func (h *Handler) HandleResolveMessageReport(w http.ResponseWriter, r *http.Request) {
	if !h.isGlobalModOrAdmin(r) {
		writeError(w, http.StatusForbidden, "forbidden", "Global moderator or admin access required")
		return
	}

	reportID := chi.URLParam(r, "reportID")
	modID := auth.UserIDFromContext(r.Context())

	var req struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if req.Status != "resolved" && req.Status != "dismissed" {
		writeError(w, http.StatusBadRequest, "invalid_status", "Status must be 'resolved' or 'dismissed'")
		return
	}

	now := time.Now()
	tag, err := h.Pool.Exec(r.Context(),
		`UPDATE message_reports SET status = $1, resolved_by = $2, resolved_at = $3 WHERE id = $4`,
		req.Status, modID, now, reportID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to resolve message report")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "not_found", "Report not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"status": req.Status})
}
