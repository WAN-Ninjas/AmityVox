// Package activities implements REST API handlers for the embedded Activities
// framework including activity management, sessions, participants, mini-games
// (Trivia, Tic-Tac-Toe, Chess, Drawing), Watch Together, and Music Listening Party.
// Mounted under /api/v1/activities and /api/v1/channels/{channelID}/activities.
package activities

import (
	"encoding/json"
	"log/slog"
	"math/rand"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/oklog/ulid/v2"

	"github.com/amityvox/amityvox/internal/api/apiutil"
	"github.com/amityvox/amityvox/internal/auth"
	"github.com/amityvox/amityvox/internal/events"
)

// Handler implements activities and games REST API endpoints.
type Handler struct {
	Pool     *pgxpool.Pool
	EventBus *events.Bus
	Logger   *slog.Logger
}

func newID() string {
	return ulid.Make().String()
}

// =============================================================================
// Activity Catalog
// =============================================================================

// HandleListActivities returns all public activities.
// GET /api/v1/activities
func (h *Handler) HandleListActivities(w http.ResponseWriter, r *http.Request) {
	category := r.URL.Query().Get("category")

	var rows pgx.Rows
	var err error
	if category != "" {
		rows, err = h.Pool.Query(r.Context(),
			`SELECT id, name, description, activity_type, icon_url, url, developer_id,
			        sdk_version, max_participants, min_participants, category, public, verified,
			        supported_channel_types, install_count, rating_sum, rating_count,
			        created_at, updated_at
			 FROM activities
			 WHERE public = true AND category = $1
			 ORDER BY install_count DESC
			 LIMIT 100`, category)
	} else {
		rows, err = h.Pool.Query(r.Context(),
			`SELECT id, name, description, activity_type, icon_url, url, developer_id,
			        sdk_version, max_participants, min_participants, category, public, verified,
			        supported_channel_types, install_count, rating_sum, rating_count,
			        created_at, updated_at
			 FROM activities
			 WHERE public = true
			 ORDER BY install_count DESC
			 LIMIT 100`)
	}
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to list activities", err)
		return
	}
	defer rows.Close()

	type activity struct {
		ID                    string    `json:"id"`
		Name                  string    `json:"name"`
		Description           *string   `json:"description,omitempty"`
		ActivityType          string    `json:"activity_type"`
		IconURL               *string   `json:"icon_url,omitempty"`
		URL                   string    `json:"url"`
		DeveloperID           *string   `json:"developer_id,omitempty"`
		SDKVersion            string    `json:"sdk_version"`
		MaxParticipants       int       `json:"max_participants"`
		MinParticipants       int       `json:"min_participants"`
		Category              string    `json:"category"`
		Public                bool      `json:"public"`
		Verified              bool      `json:"verified"`
		SupportedChannelTypes []string  `json:"supported_channel_types"`
		InstallCount          int       `json:"install_count"`
		RatingAvg             float64   `json:"rating_avg"`
		RatingCount           int       `json:"rating_count"`
		CreatedAt             time.Time `json:"created_at"`
		UpdatedAt             time.Time `json:"updated_at"`
	}

	activities := make([]activity, 0)
	for rows.Next() {
		var a activity
		var ratingSum, ratingCount int
		if err := rows.Scan(&a.ID, &a.Name, &a.Description, &a.ActivityType,
			&a.IconURL, &a.URL, &a.DeveloperID, &a.SDKVersion,
			&a.MaxParticipants, &a.MinParticipants, &a.Category, &a.Public,
			&a.Verified, &a.SupportedChannelTypes, &a.InstallCount,
			&ratingSum, &ratingCount, &a.CreatedAt, &a.UpdatedAt); err != nil {
			continue
		}
		if ratingCount > 0 {
			a.RatingAvg = float64(ratingSum) / float64(ratingCount)
		}
		a.RatingCount = ratingCount
		activities = append(activities, a)
	}

	apiutil.WriteJSON(w, http.StatusOK, activities)
}

// HandleGetActivity returns a single activity by ID.
// GET /api/v1/activities/{activityID}
func (h *Handler) HandleGetActivity(w http.ResponseWriter, r *http.Request) {
	activityID := chi.URLParam(r, "activityID")

	type activity struct {
		ID                    string          `json:"id"`
		Name                  string          `json:"name"`
		Description           *string         `json:"description,omitempty"`
		ActivityType          string          `json:"activity_type"`
		IconURL               *string         `json:"icon_url,omitempty"`
		URL                   string          `json:"url"`
		DeveloperID           *string         `json:"developer_id,omitempty"`
		SDKVersion            string          `json:"sdk_version"`
		MaxParticipants       int             `json:"max_participants"`
		MinParticipants       int             `json:"min_participants"`
		Category              string          `json:"category"`
		Public                bool            `json:"public"`
		Verified              bool            `json:"verified"`
		SupportedChannelTypes []string        `json:"supported_channel_types"`
		ConfigSchema          json.RawMessage `json:"config_schema"`
		PermissionsRequired   []string        `json:"permissions_required"`
		InstallCount          int             `json:"install_count"`
		RatingSum             int             `json:"rating_sum"`
		RatingCount           int             `json:"rating_count"`
		CreatedAt             time.Time       `json:"created_at"`
		UpdatedAt             time.Time       `json:"updated_at"`
	}

	var a activity
	err := h.Pool.QueryRow(r.Context(),
		`SELECT id, name, description, activity_type, icon_url, url, developer_id,
		        sdk_version, max_participants, min_participants, category, public, verified,
		        supported_channel_types, config_schema, permissions_required,
		        install_count, rating_sum, rating_count, created_at, updated_at
		 FROM activities WHERE id = $1`, activityID).Scan(
		&a.ID, &a.Name, &a.Description, &a.ActivityType, &a.IconURL, &a.URL,
		&a.DeveloperID, &a.SDKVersion, &a.MaxParticipants, &a.MinParticipants,
		&a.Category, &a.Public, &a.Verified, &a.SupportedChannelTypes,
		&a.ConfigSchema, &a.PermissionsRequired, &a.InstallCount,
		&a.RatingSum, &a.RatingCount, &a.CreatedAt, &a.UpdatedAt)
	if err == pgx.ErrNoRows {
		apiutil.WriteError(w, http.StatusNotFound, "not_found", "Activity not found")
		return
	}
	if err != nil {
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to get activity")
		return
	}

	apiutil.WriteJSON(w, http.StatusOK, a)
}

// HandleCreateActivity registers a new custom activity (for the Activity SDK).
// POST /api/v1/activities
func (h *Handler) HandleCreateActivity(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())

	var req struct {
		Name                  string   `json:"name"`
		Description           string   `json:"description"`
		ActivityType          string   `json:"activity_type"`
		IconURL               string   `json:"icon_url"`
		URL                   string   `json:"url"`
		MaxParticipants       int      `json:"max_participants"`
		MinParticipants       int      `json:"min_participants"`
		Category              string   `json:"category"`
		SupportedChannelTypes []string `json:"supported_channel_types"`
	}
	if !apiutil.DecodeJSON(w, r, &req) {
		return
	}

	if !apiutil.RequireNonEmpty(w, "name", req.Name) {
		return
	}
	if !apiutil.RequireNonEmpty(w, "url", req.URL) {
		return
	}

	validTypes := map[string]bool{
		"watch_together": true, "music_party": true, "game": true, "custom": true,
	}
	if !validTypes[req.ActivityType] {
		req.ActivityType = "custom"
	}
	validCategories := map[string]bool{
		"entertainment": true, "games": true, "productivity": true, "other": true,
	}
	if !validCategories[req.Category] {
		req.Category = "other"
	}

	if req.MaxParticipants < 0 {
		req.MaxParticipants = 0
	}
	if req.MinParticipants < 1 {
		req.MinParticipants = 1
	}
	if len(req.SupportedChannelTypes) == 0 {
		req.SupportedChannelTypes = []string{"voice", "text"}
	}

	id := newID()
	_, err := h.Pool.Exec(r.Context(),
		`INSERT INTO activities (id, name, description, activity_type, icon_url, url,
		 developer_id, max_participants, min_participants, category, supported_channel_types)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		id, req.Name, req.Description, req.ActivityType, req.IconURL, req.URL,
		userID, req.MaxParticipants, req.MinParticipants, req.Category, req.SupportedChannelTypes)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to create activity", err)
		return
	}

	apiutil.WriteJSON(w, http.StatusCreated, map[string]interface{}{
		"id":            id,
		"name":          req.Name,
		"description":   req.Description,
		"activity_type": req.ActivityType,
		"url":           req.URL,
		"developer_id":  userID,
		"created_at":    time.Now().UTC(),
	})
}

// HandleRateActivity allows a user to rate an activity.
// POST /api/v1/activities/{activityID}/rate
func (h *Handler) HandleRateActivity(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	activityID := chi.URLParam(r, "activityID")

	var req struct {
		Rating int    `json:"rating"`
		Review string `json:"review"`
	}
	if !apiutil.DecodeJSON(w, r, &req) {
		return
	}
	if req.Rating < 1 || req.Rating > 5 {
		apiutil.WriteError(w, http.StatusBadRequest, "invalid_rating", "Rating must be between 1 and 5")
		return
	}

	_, err := h.Pool.Exec(r.Context(),
		`INSERT INTO activity_ratings (activity_id, user_id, rating, review)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (activity_id, user_id) DO UPDATE SET rating = $3, review = $4, created_at = NOW()`,
		activityID, userID, req.Rating, req.Review)
	if err != nil {
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to rate activity")
		return
	}

	// Update aggregate rating on the activity.
	h.Pool.Exec(r.Context(),
		`UPDATE activities SET
		 rating_sum = (SELECT COALESCE(SUM(rating), 0) FROM activity_ratings WHERE activity_id = $1),
		 rating_count = (SELECT COUNT(*) FROM activity_ratings WHERE activity_id = $1),
		 updated_at = NOW()
		 WHERE id = $1`, activityID)

	apiutil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"activity_id": activityID,
		"rating":      req.Rating,
		"review":      req.Review,
	})
}

// =============================================================================
// Activity Sessions
// =============================================================================

type startSessionRequest struct {
	ActivityID string                 `json:"activity_id"`
	Config     map[string]interface{} `json:"config"`
}

// HandleStartActivitySession starts an activity in a channel.
// POST /api/v1/channels/{channelID}/activities/sessions
func (h *Handler) HandleStartActivitySession(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")

	var req startSessionRequest
	if !apiutil.DecodeJSON(w, r, &req) {
		return
	}
	if !apiutil.RequireNonEmpty(w, "activity_id", req.ActivityID) {
		return
	}

	// Verify activity exists.
	var activityName, activityURL string
	err := h.Pool.QueryRow(r.Context(),
		`SELECT name, url FROM activities WHERE id = $1`, req.ActivityID).Scan(&activityName, &activityURL)
	if err == pgx.ErrNoRows {
		apiutil.WriteError(w, http.StatusNotFound, "activity_not_found", "Activity not found")
		return
	}
	if err != nil {
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to verify activity")
		return
	}

	// Check for existing active session in this channel.
	var existingID string
	err = h.Pool.QueryRow(r.Context(),
		`SELECT id FROM activity_sessions
		 WHERE channel_id = $1 AND activity_id = $2 AND status = 'active'`,
		channelID, req.ActivityID).Scan(&existingID)
	if err == nil {
		apiutil.WriteError(w, http.StatusConflict, "session_exists",
			"An active session for this activity already exists in this channel")
		return
	}

	// Get guild_id.
	var guildID *string
	h.Pool.QueryRow(r.Context(), `SELECT guild_id FROM channels WHERE id = $1`, channelID).Scan(&guildID)

	configJSON, _ := json.Marshal(req.Config)
	sessionID := newID()

	_, err = h.Pool.Exec(r.Context(),
		`INSERT INTO activity_sessions (id, activity_id, channel_id, guild_id, host_user_id, config, status)
		 VALUES ($1, $2, $3, $4, $5, $6, 'active')`,
		sessionID, req.ActivityID, channelID, guildID, userID, configJSON)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to start session", err)
		return
	}

	// Add host as participant.
	h.Pool.Exec(r.Context(),
		`INSERT INTO activity_participants (session_id, user_id, role) VALUES ($1, $2, 'host')`,
		sessionID, userID)

	// Increment install count.
	h.Pool.Exec(r.Context(),
		`UPDATE activities SET install_count = install_count + 1 WHERE id = $1`, req.ActivityID)

	result := map[string]interface{}{
		"id":            sessionID,
		"activity_id":   req.ActivityID,
		"activity_name": activityName,
		"activity_url":  activityURL,
		"channel_id":    channelID,
		"guild_id":      guildID,
		"host_user_id":  userID,
		"config":        req.Config,
		"status":        "active",
		"started_at":    time.Now().UTC(),
	}

	if h.EventBus != nil {
		h.EventBus.PublishChannelEvent(r.Context(), "amityvox.channel.activity", "ACTIVITY_SESSION_START", channelID, result)
	}

	apiutil.WriteJSON(w, http.StatusCreated, result)
}

// HandleJoinActivitySession joins an existing activity session.
// POST /api/v1/channels/{channelID}/activities/sessions/{sessionID}/join
func (h *Handler) HandleJoinActivitySession(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	sessionID := chi.URLParam(r, "sessionID")

	// Verify session is active.
	var activityID, channelID string
	var maxParticipants int
	err := h.Pool.QueryRow(r.Context(),
		`SELECT a_s.activity_id, a_s.channel_id, a.max_participants
		 FROM activity_sessions a_s
		 JOIN activities a ON a.id = a_s.activity_id
		 WHERE a_s.id = $1 AND a_s.status = 'active'`,
		sessionID).Scan(&activityID, &channelID, &maxParticipants)
	if err == pgx.ErrNoRows {
		apiutil.WriteError(w, http.StatusNotFound, "session_not_found", "Active session not found")
		return
	}
	if err != nil {
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to get session")
		return
	}

	// Check participant count limit.
	if maxParticipants > 0 {
		var count int
		h.Pool.QueryRow(r.Context(),
			`SELECT COUNT(*) FROM activity_participants WHERE session_id = $1`, sessionID).Scan(&count)
		if count >= maxParticipants {
			apiutil.WriteError(w, http.StatusConflict, "session_full", "Activity session is full")
			return
		}
	}

	_, err = h.Pool.Exec(r.Context(),
		`INSERT INTO activity_participants (session_id, user_id, role)
		 VALUES ($1, $2, 'participant')
		 ON CONFLICT (session_id, user_id) DO NOTHING`,
		sessionID, userID)
	if err != nil {
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to join session")
		return
	}

	if h.EventBus != nil {
		h.EventBus.PublishChannelEvent(r.Context(), "amityvox.channel.activity", "ACTIVITY_PARTICIPANT_JOIN", channelID, map[string]string{
			"session_id": sessionID,
			"user_id":    userID,
			"channel_id": channelID,
		})
	}

	apiutil.WriteJSON(w, http.StatusOK, map[string]string{"status": "joined", "session_id": sessionID})
}

// HandleLeaveActivitySession leaves an activity session.
// POST /api/v1/channels/{channelID}/activities/sessions/{sessionID}/leave
func (h *Handler) HandleLeaveActivitySession(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	sessionID := chi.URLParam(r, "sessionID")

	h.Pool.Exec(r.Context(),
		`DELETE FROM activity_participants WHERE session_id = $1 AND user_id = $2`,
		sessionID, userID)

	if h.EventBus != nil {
		var channelID string
		h.Pool.QueryRow(r.Context(),
			`SELECT channel_id FROM activity_sessions WHERE id = $1`, sessionID).Scan(&channelID)
		h.EventBus.PublishChannelEvent(r.Context(), "amityvox.channel.activity", "ACTIVITY_PARTICIPANT_LEAVE", channelID, map[string]string{
			"session_id": sessionID,
			"user_id":    userID,
		})
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleEndActivitySession ends an activity session (host only).
// POST /api/v1/channels/{channelID}/activities/sessions/{sessionID}/end
func (h *Handler) HandleEndActivitySession(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	sessionID := chi.URLParam(r, "sessionID")

	tag, err := h.Pool.Exec(r.Context(),
		`UPDATE activity_sessions SET status = 'ended', ended_at = NOW()
		 WHERE id = $1 AND host_user_id = $2 AND status = 'active'`,
		sessionID, userID)
	if err != nil {
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to end session")
		return
	}
	if tag.RowsAffected() == 0 {
		apiutil.WriteError(w, http.StatusForbidden, "not_host", "Only the host can end the session")
		return
	}

	if h.EventBus != nil {
		var channelID string
		h.Pool.QueryRow(r.Context(),
			`SELECT channel_id FROM activity_sessions WHERE id = $1`, sessionID).Scan(&channelID)
		h.EventBus.PublishChannelEvent(r.Context(), "amityvox.channel.activity", "ACTIVITY_SESSION_END", channelID, map[string]string{
			"session_id": sessionID,
		})
	}

	apiutil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ended"})
}

// HandleGetActiveSession returns the active activity session for a channel.
// GET /api/v1/channels/{channelID}/activities/sessions/active
func (h *Handler) HandleGetActiveSession(w http.ResponseWriter, r *http.Request) {
	channelID := chi.URLParam(r, "channelID")

	type session struct {
		ID           string          `json:"id"`
		ActivityID   string          `json:"activity_id"`
		ActivityName string          `json:"activity_name"`
		ActivityURL  string          `json:"activity_url"`
		ActivityIcon *string         `json:"activity_icon,omitempty"`
		ChannelID    string          `json:"channel_id"`
		GuildID      *string         `json:"guild_id,omitempty"`
		HostUserID   string          `json:"host_user_id"`
		State        json.RawMessage `json:"state"`
		Config       json.RawMessage `json:"config"`
		Status       string          `json:"status"`
		StartedAt    time.Time       `json:"started_at"`
	}

	var s session
	err := h.Pool.QueryRow(r.Context(),
		`SELECT a_s.id, a_s.activity_id, a.name, a.url, a.icon_url,
		        a_s.channel_id, a_s.guild_id, a_s.host_user_id, a_s.state,
		        a_s.config, a_s.status, a_s.started_at
		 FROM activity_sessions a_s
		 JOIN activities a ON a.id = a_s.activity_id
		 WHERE a_s.channel_id = $1 AND a_s.status = 'active'
		 ORDER BY a_s.started_at DESC
		 LIMIT 1`, channelID).Scan(
		&s.ID, &s.ActivityID, &s.ActivityName, &s.ActivityURL, &s.ActivityIcon,
		&s.ChannelID, &s.GuildID, &s.HostUserID, &s.State, &s.Config,
		&s.Status, &s.StartedAt)
	if err == pgx.ErrNoRows {
		apiutil.WriteJSON(w, http.StatusOK, nil)
		return
	}
	if err != nil {
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to get session")
		return
	}

	// Get participants.
	pRows, _ := h.Pool.Query(r.Context(),
		`SELECT ap.user_id, ap.role, ap.joined_at, u.username, u.display_name, u.avatar_id
		 FROM activity_participants ap
		 JOIN users u ON u.id = ap.user_id
		 WHERE ap.session_id = $1
		 ORDER BY ap.joined_at ASC`, s.ID)

	type participant struct {
		UserID      string    `json:"user_id"`
		Role        string    `json:"role"`
		JoinedAt    time.Time `json:"joined_at"`
		Username    string    `json:"username"`
		DisplayName *string   `json:"display_name,omitempty"`
		AvatarID    *string   `json:"avatar_id,omitempty"`
	}

	participants := make([]participant, 0)
	if pRows != nil {
		defer pRows.Close()
		for pRows.Next() {
			var p participant
			if err := pRows.Scan(&p.UserID, &p.Role, &p.JoinedAt,
				&p.Username, &p.DisplayName, &p.AvatarID); err != nil {
				continue
			}
			participants = append(participants, p)
		}
	}

	apiutil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"session":      s,
		"participants": participants,
	})
}

// HandleUpdateActivityState updates shared state for an activity session.
// PATCH /api/v1/channels/{channelID}/activities/sessions/{sessionID}/state
func (h *Handler) HandleUpdateActivityState(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "sessionID")

	var req struct {
		State json.RawMessage `json:"state"`
	}
	if !apiutil.DecodeJSON(w, r, &req) {
		return
	}

	tag, err := h.Pool.Exec(r.Context(),
		`UPDATE activity_sessions SET state = $1 WHERE id = $2 AND status = 'active'`,
		req.State, sessionID)
	if err != nil {
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to update state")
		return
	}
	if tag.RowsAffected() == 0 {
		apiutil.WriteError(w, http.StatusNotFound, "not_found", "Active session not found")
		return
	}

	if h.EventBus != nil {
		var channelID string
		h.Pool.QueryRow(r.Context(),
			`SELECT channel_id FROM activity_sessions WHERE id = $1`, sessionID).Scan(&channelID)
		h.EventBus.PublishChannelEvent(r.Context(), "amityvox.channel.activity", "ACTIVITY_STATE_UPDATE", channelID, map[string]interface{}{
			"session_id": sessionID,
			"state":      req.State,
		})
	}

	apiutil.WriteJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// =============================================================================
// Mini-Games
// =============================================================================

type createGameRequest struct {
	GameType string                 `json:"game_type"` // trivia, tictactoe, chess, drawing
	Config   map[string]interface{} `json:"config"`
}

type gameMoveRequest struct {
	Move json.RawMessage `json:"move"`
}

// HandleCreateGame starts a new mini-game session in a channel.
// POST /api/v1/channels/{channelID}/activities/games
func (h *Handler) HandleCreateGame(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")

	var req createGameRequest
	if !apiutil.DecodeJSON(w, r, &req) {
		return
	}

	validTypes := map[string]bool{
		"trivia": true, "tictactoe": true, "chess": true, "drawing": true,
	}
	if !validTypes[req.GameType] {
		apiutil.WriteError(w, http.StatusBadRequest, "invalid_game_type",
			"Game type must be one of: trivia, tictactoe, chess, drawing")
		return
	}

	// Initialize game state based on type.
	initialState := initializeGameState(req.GameType, req.Config)
	stateJSON, _ := json.Marshal(initialState)
	configJSON, _ := json.Marshal(req.Config)

	gameID := newID()
	_, err := h.Pool.Exec(r.Context(),
		`INSERT INTO game_sessions (id, channel_id, game_type, state, config, status, turn_user_id)
		 VALUES ($1, $2, $3, $4, $5, 'waiting', $6)`,
		gameID, channelID, req.GameType, stateJSON, configJSON, userID)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to create game", err)
		return
	}

	// Add creator as first player.
	h.Pool.Exec(r.Context(),
		`INSERT INTO game_players (session_id, user_id, player_index) VALUES ($1, $2, 0)`,
		gameID, userID)

	result := map[string]interface{}{
		"id":         gameID,
		"channel_id": channelID,
		"game_type":  req.GameType,
		"state":      initialState,
		"config":     req.Config,
		"status":     "waiting",
		"created_at": time.Now().UTC(),
	}

	if h.EventBus != nil {
		h.EventBus.PublishChannelEvent(r.Context(), "amityvox.channel.game", "GAME_SESSION_CREATE", channelID, result)
	}

	apiutil.WriteJSON(w, http.StatusCreated, result)
}

// initializeGameState returns the initial state for a game type.
func initializeGameState(gameType string, config map[string]interface{}) map[string]interface{} {
	switch gameType {
	case "tictactoe":
		return map[string]interface{}{
			"board":     [9]string{},
			"turn":      "X",
			"winner":    nil,
			"moves":     0,
			"draw":      false,
		}
	case "chess":
		return map[string]interface{}{
			"fen":         "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1",
			"moves":       []string{},
			"check":       false,
			"checkmate":   false,
			"stalemate":   false,
			"captured":    map[string][]string{"white": {}, "black": {}},
		}
	case "trivia":
		category := "general"
		if c, ok := config["category"].(string); ok && c != "" {
			category = c
		}
		return map[string]interface{}{
			"category":      category,
			"question_index": 0,
			"questions":     generateTriviaQuestions(category),
			"scores":        map[string]int{},
			"current_question": nil,
			"answers_revealed": false,
		}
	case "drawing":
		return map[string]interface{}{
			"word":            "",
			"drawer_id":      "",
			"canvas_data":    nil,
			"guesses":        []string{},
			"round":          0,
			"max_rounds":     3,
			"time_limit":     60,
			"scores":         map[string]int{},
		}
	default:
		return map[string]interface{}{}
	}
}

// generateTriviaQuestions returns a set of trivia questions for a given category.
func generateTriviaQuestions(category string) []map[string]interface{} {
	questions := []map[string]interface{}{
		{
			"question": "What protocol does AmityVox use for real-time events?",
			"options":  []string{"HTTP Polling", "NATS", "gRPC", "GraphQL Subscriptions"},
			"answer":   1,
		},
		{
			"question": "Which database does AmityVox use as its primary data store?",
			"options":  []string{"MongoDB", "SQLite", "PostgreSQL", "MySQL"},
			"answer":   2,
		},
		{
			"question": "What format are IDs in AmityVox?",
			"options":  []string{"UUID v4", "ULID", "Snowflake", "Auto-increment"},
			"answer":   1,
		},
		{
			"question": "What language is the AmityVox backend written in?",
			"options":  []string{"Rust", "Python", "Go", "TypeScript"},
			"answer":   2,
		},
		{
			"question": "Which S3-compatible storage does AmityVox use by default?",
			"options":  []string{"MinIO", "Garage", "AWS S3", "Wasabi"},
			"answer":   1,
		},
		{
			"question": "What frontend framework does AmityVox use?",
			"options":  []string{"React", "Vue", "SvelteKit", "Angular"},
			"answer":   2,
		},
		{
			"question": "What hashing algorithm does AmityVox use for passwords?",
			"options":  []string{"bcrypt", "scrypt", "Argon2id", "SHA-256"},
			"answer":   2,
		},
		{
			"question": "What reverse proxy does AmityVox use?",
			"options":  []string{"Nginx", "Caddy", "Traefik", "HAProxy"},
			"answer":   1,
		},
		{
			"question": "What HTTP router does the AmityVox Go backend use?",
			"options":  []string{"Gin", "Echo", "chi", "Fiber"},
			"answer":   2,
		},
		{
			"question": "What license is AmityVox released under?",
			"options":  []string{"MIT", "Apache 2.0", "GPL-3.0", "AGPL-3.0"},
			"answer":   3,
		},
	}

	// Shuffle questions.
	rand.Shuffle(len(questions), func(i, j int) {
		questions[i], questions[j] = questions[j], questions[i]
	})

	return questions
}

// HandleJoinGame joins an existing game session.
// POST /api/v1/channels/{channelID}/activities/games/{gameID}/join
func (h *Handler) HandleJoinGame(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	gameID := chi.URLParam(r, "gameID")

	// Check game is in 'waiting' status.
	var gameType, status, channelID string
	err := h.Pool.QueryRow(r.Context(),
		`SELECT game_type, status, channel_id FROM game_sessions WHERE id = $1`, gameID).Scan(&gameType, &status, &channelID)
	if err == pgx.ErrNoRows {
		apiutil.WriteError(w, http.StatusNotFound, "not_found", "Game not found")
		return
	}
	if err != nil {
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to get game")
		return
	}
	if status != "waiting" {
		apiutil.WriteError(w, http.StatusConflict, "game_in_progress", "Game is already in progress")
		return
	}

	// Get next player index.
	var playerCount int
	h.Pool.QueryRow(r.Context(),
		`SELECT COUNT(*) FROM game_players WHERE session_id = $1`, gameID).Scan(&playerCount)

	// Enforce max player limits.
	maxPlayers := map[string]int{
		"tictactoe": 2, "chess": 2, "trivia": 10, "drawing": 10,
	}
	if max, ok := maxPlayers[gameType]; ok && playerCount >= max {
		apiutil.WriteError(w, http.StatusConflict, "game_full", "Game is full")
		return
	}

	_, err = h.Pool.Exec(r.Context(),
		`INSERT INTO game_players (session_id, user_id, player_index)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (session_id, user_id) DO NOTHING`,
		gameID, userID, playerCount)
	if err != nil {
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to join game")
		return
	}

	// Auto-start 2-player games when full.
	minPlayers := map[string]int{"tictactoe": 2, "chess": 2, "trivia": 2, "drawing": 2}
	if min, ok := minPlayers[gameType]; ok && playerCount+1 >= min {
		h.Pool.Exec(r.Context(),
			`UPDATE game_sessions SET status = 'playing', started_at = NOW() WHERE id = $1 AND status = 'waiting'`,
			gameID)
	}

	if h.EventBus != nil {
		h.EventBus.PublishChannelEvent(r.Context(), "amityvox.channel.game", "GAME_PLAYER_JOIN", channelID, map[string]interface{}{
			"game_id":      gameID,
			"user_id":      userID,
			"player_index": playerCount,
			"game_type":    gameType,
		})
	}

	apiutil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"game_id":      gameID,
		"player_index": playerCount,
		"status":       "joined",
	})
}

// HandleGameMove processes a game move (turn-based).
// POST /api/v1/channels/{channelID}/activities/games/{gameID}/move
func (h *Handler) HandleGameMove(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	gameID := chi.URLParam(r, "gameID")

	var req gameMoveRequest
	if !apiutil.DecodeJSON(w, r, &req) {
		return
	}

	// Get current game state.
	var gameType, status, channelID string
	var state json.RawMessage
	var turnUserID *string
	err := h.Pool.QueryRow(r.Context(),
		`SELECT game_type, status, state, turn_user_id, channel_id
		 FROM game_sessions WHERE id = $1`, gameID).Scan(&gameType, &status, &state, &turnUserID, &channelID)
	if err == pgx.ErrNoRows {
		apiutil.WriteError(w, http.StatusNotFound, "not_found", "Game not found")
		return
	}
	if err != nil {
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to get game")
		return
	}
	if status != "playing" {
		apiutil.WriteError(w, http.StatusBadRequest, "game_not_active", "Game is not currently active")
		return
	}

	// Verify it's the user's turn (for turn-based games).
	if (gameType == "tictactoe" || gameType == "chess") && turnUserID != nil && *turnUserID != userID {
		apiutil.WriteError(w, http.StatusForbidden, "not_your_turn", "It is not your turn")
		return
	}

	// Process the move (store it; actual game logic is client-side for responsiveness).
	var moveData map[string]interface{}
	json.Unmarshal(req.Move, &moveData)

	// Merge move into state.
	var currentState map[string]interface{}
	json.Unmarshal(state, &currentState)
	for k, v := range moveData {
		currentState[k] = v
	}
	newStateJSON, _ := json.Marshal(currentState)

	// Determine next turn.
	var players []string
	pRows, _ := h.Pool.Query(r.Context(),
		`SELECT user_id FROM game_players WHERE session_id = $1 ORDER BY player_index ASC`, gameID)
	if pRows != nil {
		defer pRows.Close()
		for pRows.Next() {
			var uid string
			if err := pRows.Scan(&uid); err == nil {
				players = append(players, uid)
			}
		}
	}

	var nextTurn *string
	if len(players) > 1 {
		for i, p := range players {
			if p == userID {
				next := players[(i+1)%len(players)]
				nextTurn = &next
				break
			}
		}
	}

	// Check if game is over (client sends "finished" in state).
	newStatus := "playing"
	var winnerID *string
	if finished, ok := currentState["finished"].(bool); ok && finished {
		newStatus = "finished"
		if winner, ok := currentState["winner"].(string); ok && winner != "" {
			winnerID = &winner
		}
	}

	h.Pool.Exec(r.Context(),
		`UPDATE game_sessions SET state = $1, turn_user_id = $2, status = $3,
		 winner_user_id = $4, ended_at = CASE WHEN $3 = 'finished' THEN NOW() ELSE NULL END
		 WHERE id = $5`,
		newStateJSON, nextTurn, newStatus, winnerID, gameID)

	// Update leaderboard if game finished.
	if newStatus == "finished" {
		h.updateLeaderboard(r, gameID, gameType, winnerID, players)
	}

	result := map[string]interface{}{
		"game_id":   gameID,
		"user_id":   userID,
		"move":      moveData,
		"state":     currentState,
		"status":    newStatus,
		"next_turn": nextTurn,
	}

	if h.EventBus != nil {
		h.EventBus.PublishChannelEvent(r.Context(), "amityvox.channel.game", "GAME_MOVE", channelID, result)
	}

	apiutil.WriteJSON(w, http.StatusOK, result)
}

// updateLeaderboard updates the game leaderboard after a game finishes.
func (h *Handler) updateLeaderboard(r *http.Request, gameID, gameType string, winnerID *string, players []string) {
	for _, uid := range players {
		isWinner := winnerID != nil && *winnerID == uid
		isLoser := winnerID != nil && *winnerID != uid
		isDraw := winnerID == nil

		var winsInc, lossesInc, drawsInc int
		if isWinner {
			winsInc = 1
		} else if isLoser {
			lossesInc = 1
		} else if isDraw {
			drawsInc = 1
		}

		// Get player score.
		var score int
		h.Pool.QueryRow(r.Context(),
			`SELECT score FROM game_players WHERE session_id = $1 AND user_id = $2`,
			gameID, uid).Scan(&score)

		h.Pool.Exec(r.Context(),
			`INSERT INTO game_leaderboard (user_id, game_type, wins, losses, draws, total_score, games_played, last_played_at)
			 VALUES ($1, $2, $3, $4, $5, $6, 1, NOW())
			 ON CONFLICT (user_id, game_type) DO UPDATE SET
			   wins = game_leaderboard.wins + $3,
			   losses = game_leaderboard.losses + $4,
			   draws = game_leaderboard.draws + $5,
			   total_score = game_leaderboard.total_score + $6,
			   games_played = game_leaderboard.games_played + 1,
			   last_played_at = NOW()`,
			uid, gameType, winsInc, lossesInc, drawsInc, score)
	}
}

// HandleGetGame returns the current state of a game.
// GET /api/v1/channels/{channelID}/activities/games/{gameID}
func (h *Handler) HandleGetGame(w http.ResponseWriter, r *http.Request) {
	gameID := chi.URLParam(r, "gameID")

	type gameSession struct {
		ID           string          `json:"id"`
		ChannelID    string          `json:"channel_id"`
		GameType     string          `json:"game_type"`
		State        json.RawMessage `json:"state"`
		Config       json.RawMessage `json:"config"`
		Status       string          `json:"status"`
		WinnerUserID *string         `json:"winner_user_id,omitempty"`
		TurnUserID   *string         `json:"turn_user_id,omitempty"`
		StartedAt    time.Time       `json:"started_at"`
		EndedAt      *time.Time      `json:"ended_at,omitempty"`
	}

	var g gameSession
	err := h.Pool.QueryRow(r.Context(),
		`SELECT id, channel_id, game_type, state, config, status,
		        winner_user_id, turn_user_id, started_at, ended_at
		 FROM game_sessions WHERE id = $1`, gameID).Scan(
		&g.ID, &g.ChannelID, &g.GameType, &g.State, &g.Config,
		&g.Status, &g.WinnerUserID, &g.TurnUserID, &g.StartedAt, &g.EndedAt)
	if err == pgx.ErrNoRows {
		apiutil.WriteError(w, http.StatusNotFound, "not_found", "Game not found")
		return
	}
	if err != nil {
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to get game")
		return
	}

	// Fetch players.
	pRows, _ := h.Pool.Query(r.Context(),
		`SELECT gp.user_id, gp.player_index, gp.score, u.username, u.display_name, u.avatar_id
		 FROM game_players gp
		 JOIN users u ON u.id = gp.user_id
		 WHERE gp.session_id = $1
		 ORDER BY gp.player_index ASC`, gameID)

	type player struct {
		UserID      string  `json:"user_id"`
		PlayerIndex int     `json:"player_index"`
		Score       int     `json:"score"`
		Username    string  `json:"username"`
		DisplayName *string `json:"display_name,omitempty"`
		AvatarID    *string `json:"avatar_id,omitempty"`
	}

	players := make([]player, 0)
	if pRows != nil {
		defer pRows.Close()
		for pRows.Next() {
			var p player
			if err := pRows.Scan(&p.UserID, &p.PlayerIndex, &p.Score,
				&p.Username, &p.DisplayName, &p.AvatarID); err != nil {
				continue
			}
			players = append(players, p)
		}
	}

	apiutil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"game":    g,
		"players": players,
	})
}

// HandleGetLeaderboard returns the leaderboard for a specific game type.
// GET /api/v1/activities/games/{gameType}/leaderboard
func (h *Handler) HandleGetLeaderboard(w http.ResponseWriter, r *http.Request) {
	gameType := chi.URLParam(r, "gameType")

	validTypes := map[string]bool{"trivia": true, "tictactoe": true, "chess": true, "drawing": true}
	if !validTypes[gameType] {
		apiutil.WriteError(w, http.StatusBadRequest, "invalid_game_type", "Invalid game type")
		return
	}

	rows, err := h.Pool.Query(r.Context(),
		`SELECT gl.user_id, gl.wins, gl.losses, gl.draws, gl.total_score,
		        gl.games_played, gl.last_played_at,
		        u.username, u.display_name, u.avatar_id
		 FROM game_leaderboard gl
		 JOIN users u ON u.id = gl.user_id
		 WHERE gl.game_type = $1
		 ORDER BY gl.wins DESC, gl.total_score DESC
		 LIMIT 50`, gameType)
	if err != nil {
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to get leaderboard")
		return
	}
	defer rows.Close()

	type entry struct {
		UserID       string    `json:"user_id"`
		Wins         int       `json:"wins"`
		Losses       int       `json:"losses"`
		Draws        int       `json:"draws"`
		TotalScore   int64     `json:"total_score"`
		GamesPlayed  int       `json:"games_played"`
		LastPlayedAt time.Time `json:"last_played_at"`
		Username     string    `json:"username"`
		DisplayName  *string   `json:"display_name,omitempty"`
		AvatarID     *string   `json:"avatar_id,omitempty"`
	}

	entries := make([]entry, 0)
	for rows.Next() {
		var e entry
		if err := rows.Scan(&e.UserID, &e.Wins, &e.Losses, &e.Draws,
			&e.TotalScore, &e.GamesPlayed, &e.LastPlayedAt,
			&e.Username, &e.DisplayName, &e.AvatarID); err != nil {
			continue
		}
		entries = append(entries, e)
	}

	apiutil.WriteJSON(w, http.StatusOK, entries)
}

// =============================================================================
// Watch Together
// =============================================================================

type watchTogetherRequest struct {
	VideoURL       string `json:"video_url"`
	VideoTitle     string `json:"video_title"`
	VideoThumbnail string `json:"video_thumbnail"`
}

type watchTogetherSyncRequest struct {
	CurrentTimeMs int64   `json:"current_time_ms"`
	Playing       *bool   `json:"playing"`
	PlaybackRate  float64 `json:"playback_rate"`
}

// HandleStartWatchTogether starts a Watch Together session.
// POST /api/v1/channels/{channelID}/activities/watch-together
func (h *Handler) HandleStartWatchTogether(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")

	var req watchTogetherRequest
	if !apiutil.DecodeJSON(w, r, &req) {
		return
	}
	if !apiutil.RequireNonEmpty(w, "video_url", req.VideoURL) {
		return
	}

	// Find or create the Watch Together activity.
	var activityID string
	err := h.Pool.QueryRow(r.Context(),
		`SELECT id FROM activities WHERE activity_type = 'watch_together' LIMIT 1`).Scan(&activityID)
	if err == pgx.ErrNoRows {
		activityID = newID()
		h.Pool.Exec(r.Context(),
			`INSERT INTO activities (id, name, description, activity_type, url, category, public, verified)
			 VALUES ($1, 'Watch Together', 'Watch videos together in sync', 'watch_together', 'builtin://watch-together', 'entertainment', true, true)`,
			activityID)
	}

	// Create activity session.
	var guildID *string
	h.Pool.QueryRow(r.Context(), `SELECT guild_id FROM channels WHERE id = $1`, channelID).Scan(&guildID)

	sessionID := newID()
	h.Pool.Exec(r.Context(),
		`INSERT INTO activity_sessions (id, activity_id, channel_id, guild_id, host_user_id, status)
		 VALUES ($1, $2, $3, $4, $5, 'active')`,
		sessionID, activityID, channelID, guildID, userID)

	h.Pool.Exec(r.Context(),
		`INSERT INTO activity_participants (session_id, user_id, role) VALUES ($1, $2, 'host')`,
		sessionID, userID)

	// Create watch together state.
	wtID := newID()
	_, err = h.Pool.Exec(r.Context(),
		`INSERT INTO watch_together_sessions (id, activity_session_id, video_url, video_title, video_thumbnail)
		 VALUES ($1, $2, $3, $4, $5)`,
		wtID, sessionID, req.VideoURL, req.VideoTitle, req.VideoThumbnail)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to start watch session", err)
		return
	}

	result := map[string]interface{}{
		"id":              wtID,
		"session_id":      sessionID,
		"channel_id":      channelID,
		"host_user_id":    userID,
		"video_url":       req.VideoURL,
		"video_title":     req.VideoTitle,
		"video_thumbnail": req.VideoThumbnail,
		"current_time_ms": 0,
		"playing":         true,
		"playback_rate":   1.0,
		"created_at":      time.Now().UTC(),
	}

	if h.EventBus != nil {
		h.EventBus.PublishChannelEvent(r.Context(), "amityvox.channel.activity", "WATCH_TOGETHER_START", channelID, result)
	}

	apiutil.WriteJSON(w, http.StatusCreated, result)
}

// HandleSyncWatchTogether synchronizes playback position for Watch Together.
// PATCH /api/v1/channels/{channelID}/activities/watch-together/{sessionID}/sync
func (h *Handler) HandleSyncWatchTogether(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "sessionID")

	var req watchTogetherSyncRequest
	if !apiutil.DecodeJSON(w, r, &req) {
		return
	}

	playing := true
	if req.Playing != nil {
		playing = *req.Playing
	}
	if req.PlaybackRate <= 0 {
		req.PlaybackRate = 1.0
	}

	tag, err := h.Pool.Exec(r.Context(),
		`UPDATE watch_together_sessions SET current_time_ms = $1, playing = $2,
		 playback_rate = $3, updated_at = NOW()
		 WHERE activity_session_id = $4`,
		req.CurrentTimeMs, playing, req.PlaybackRate, sessionID)
	if err != nil {
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to sync")
		return
	}
	if tag.RowsAffected() == 0 {
		apiutil.WriteError(w, http.StatusNotFound, "not_found", "Watch Together session not found")
		return
	}

	if h.EventBus != nil {
		var channelID string
		h.Pool.QueryRow(r.Context(),
			`SELECT channel_id FROM activity_sessions WHERE id = $1`, sessionID).Scan(&channelID)
		h.EventBus.PublishChannelEvent(r.Context(), "amityvox.channel.activity", "WATCH_TOGETHER_SYNC", channelID, map[string]interface{}{
			"session_id":      sessionID,
			"current_time_ms": req.CurrentTimeMs,
			"playing":         playing,
			"playback_rate":   req.PlaybackRate,
		})
	}

	apiutil.WriteJSON(w, http.StatusOK, map[string]string{"status": "synced"})
}

// =============================================================================
// Music Listening Party
// =============================================================================

type musicPartyRequest struct {
	TrackURL     string `json:"track_url"`
	TrackTitle   string `json:"track_title"`
	TrackArtist  string `json:"track_artist"`
	TrackArtwork string `json:"track_artwork"`
}

// HandleStartMusicParty starts a music listening party.
// POST /api/v1/channels/{channelID}/activities/music-party
func (h *Handler) HandleStartMusicParty(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")

	var req musicPartyRequest
	if !apiutil.DecodeJSON(w, r, &req) {
		return
	}
	if !apiutil.RequireNonEmpty(w, "track_url", req.TrackURL) {
		return
	}

	// Find or create the Music Party activity.
	var activityID string
	err := h.Pool.QueryRow(r.Context(),
		`SELECT id FROM activities WHERE activity_type = 'music_party' LIMIT 1`).Scan(&activityID)
	if err == pgx.ErrNoRows {
		activityID = newID()
		h.Pool.Exec(r.Context(),
			`INSERT INTO activities (id, name, description, activity_type, url, category, public, verified)
			 VALUES ($1, 'Music Party', 'Listen to music together in sync', 'music_party', 'builtin://music-party', 'entertainment', true, true)`,
			activityID)
	}

	var guildID *string
	h.Pool.QueryRow(r.Context(), `SELECT guild_id FROM channels WHERE id = $1`, channelID).Scan(&guildID)

	sessionID := newID()
	h.Pool.Exec(r.Context(),
		`INSERT INTO activity_sessions (id, activity_id, channel_id, guild_id, host_user_id, status)
		 VALUES ($1, $2, $3, $4, $5, 'active')`,
		sessionID, activityID, channelID, guildID, userID)
	h.Pool.Exec(r.Context(),
		`INSERT INTO activity_participants (session_id, user_id, role) VALUES ($1, $2, 'host')`,
		sessionID, userID)

	mpID := newID()
	_, err = h.Pool.Exec(r.Context(),
		`INSERT INTO music_party_sessions (id, activity_session_id, current_track_url,
		 current_track_title, current_track_artist, current_track_artwork)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		mpID, sessionID, req.TrackURL, req.TrackTitle, req.TrackArtist, req.TrackArtwork)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to start music party", err)
		return
	}

	result := map[string]interface{}{
		"id":              mpID,
		"session_id":      sessionID,
		"channel_id":      channelID,
		"host_user_id":    userID,
		"track_url":       req.TrackURL,
		"track_title":     req.TrackTitle,
		"track_artist":    req.TrackArtist,
		"track_artwork":   req.TrackArtwork,
		"current_time_ms": 0,
		"playing":         true,
		"created_at":      time.Now().UTC(),
	}

	if h.EventBus != nil {
		h.EventBus.PublishChannelEvent(r.Context(), "amityvox.channel.activity", "MUSIC_PARTY_START", channelID, result)
	}

	apiutil.WriteJSON(w, http.StatusCreated, result)
}

// HandleAddToMusicQueue adds a track to the music party queue.
// POST /api/v1/channels/{channelID}/activities/music-party/{sessionID}/queue
func (h *Handler) HandleAddToMusicQueue(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	sessionID := chi.URLParam(r, "sessionID")

	var req musicPartyRequest
	if !apiutil.DecodeJSON(w, r, &req) {
		return
	}
	if !apiutil.RequireNonEmpty(w, "track_url", req.TrackURL) {
		return
	}

	// Get current queue.
	var queue json.RawMessage
	err := h.Pool.QueryRow(r.Context(),
		`SELECT queue FROM music_party_sessions WHERE activity_session_id = $1`, sessionID).Scan(&queue)
	if err == pgx.ErrNoRows {
		apiutil.WriteError(w, http.StatusNotFound, "not_found", "Music party not found")
		return
	}
	if err != nil {
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to get queue")
		return
	}

	var queueList []map[string]interface{}
	json.Unmarshal(queue, &queueList)

	newTrack := map[string]interface{}{
		"url":      req.TrackURL,
		"title":    req.TrackTitle,
		"artist":   req.TrackArtist,
		"artwork":  req.TrackArtwork,
		"added_by": userID,
	}
	queueList = append(queueList, newTrack)
	updatedQueue, _ := json.Marshal(queueList)

	h.Pool.Exec(r.Context(),
		`UPDATE music_party_sessions SET queue = $1, updated_at = NOW()
		 WHERE activity_session_id = $2`,
		updatedQueue, sessionID)

	if h.EventBus != nil {
		var channelID string
		h.Pool.QueryRow(r.Context(),
			`SELECT channel_id FROM activity_sessions WHERE id = $1`, sessionID).Scan(&channelID)
		h.EventBus.PublishChannelEvent(r.Context(), "amityvox.channel.activity", "MUSIC_PARTY_QUEUE_ADD", channelID, map[string]interface{}{
			"session_id": sessionID,
			"track":      newTrack,
			"queue_size": len(queueList),
		})
	}

	apiutil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"track":      newTrack,
		"queue_size": len(queueList),
	})
}
