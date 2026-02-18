// Package social implements REST API handlers for Social & Growth features:
// server insights/analytics, server boosts, vanity URL marketplace,
// user achievements/badges, leveling/XP, starboard, welcome messages,
// and auto-role assignment.
package social

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/amityvox/amityvox/internal/api/apiutil"
	"github.com/amityvox/amityvox/internal/auth"
	"github.com/amityvox/amityvox/internal/events"
	"github.com/amityvox/amityvox/internal/models"
)

// Handler implements Social & Growth REST API endpoints.
type Handler struct {
	Pool     *pgxpool.Pool
	EventBus *events.Bus
	Logger   *slog.Logger
}

// --- Permission helpers ---

func (h *Handler) isMember(ctx context.Context, guildID, userID string) bool {
	var exists bool
	h.Pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM guild_members WHERE guild_id = $1 AND user_id = $2)`,
		guildID, userID,
	).Scan(&exists)
	return exists
}

func (h *Handler) isGuildAdmin(ctx context.Context, guildID, userID string) bool {
	// Check if guild owner.
	var ownerID string
	if err := h.Pool.QueryRow(ctx,
		`SELECT owner_id FROM guilds WHERE id = $1`, guildID,
	).Scan(&ownerID); err == nil && ownerID == userID {
		return true
	}
	// Check if instance admin.
	var flags int
	h.Pool.QueryRow(ctx, `SELECT flags FROM users WHERE id = $1`, userID).Scan(&flags)
	return flags&models.UserFlagAdmin != 0
}

// ============================================================
// 1. Server Insights / Analytics Dashboard
// ============================================================

// InsightsDailyRow represents one day of guild analytics.
type InsightsDailyRow struct {
	Date          string `json:"date"`
	MemberCount   int    `json:"member_count"`
	MembersJoined int    `json:"members_joined"`
	MembersLeft   int    `json:"members_left"`
	MessagesSent  int    `json:"messages_sent"`
	ReactionsAdded int   `json:"reactions_added"`
	VoiceMinutes  int    `json:"voice_minutes"`
	ActiveMembers int    `json:"active_members"`
}

// InsightsHourlyRow represents one hour of message activity.
type InsightsHourlyRow struct {
	Hour     int `json:"hour"`
	Messages int `json:"messages"`
}

// InsightsResponse is the full analytics payload.
type InsightsResponse struct {
	Daily      []InsightsDailyRow  `json:"daily"`
	PeakHours  []InsightsHourlyRow `json:"peak_hours"`
	TotalMembers  int              `json:"total_members"`
	TotalMessages int              `json:"total_messages"`
	GrowthRate    float64          `json:"growth_rate"`
}

// HandleGetInsights returns analytics data for a guild.
// GET /api/v1/guilds/{guildID}/insights?days=30
func (h *Handler) HandleGetInsights(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")

	if !h.isGuildAdmin(r.Context(), guildID, userID) {
		apiutil.WriteError(w, http.StatusForbidden, "forbidden", "Only guild admins can view insights")
		return
	}

	days := 30
	if d := r.URL.Query().Get("days"); d != "" {
		if parsed, err := strconv.Atoi(d); err == nil && parsed > 0 && parsed <= 90 {
			days = parsed
		}
	}

	// Ensure today's snapshot exists (lazy generation).
	h.ensureTodaySnapshot(r.Context(), guildID)

	// Fetch daily data.
	rows, err := h.Pool.Query(r.Context(),
		`SELECT date, member_count, members_joined, members_left,
		        messages_sent, reactions_added, voice_minutes, active_members
		 FROM guild_insights_daily
		 WHERE guild_id = $1 AND date >= CURRENT_DATE - $2::INT
		 ORDER BY date ASC`,
		guildID, days,
	)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to load insights", err)
		return
	}
	defer rows.Close()

	daily := make([]InsightsDailyRow, 0)
	for rows.Next() {
		var row InsightsDailyRow
		var date time.Time
		if err := rows.Scan(&date, &row.MemberCount, &row.MembersJoined, &row.MembersLeft,
			&row.MessagesSent, &row.ReactionsAdded, &row.VoiceMinutes, &row.ActiveMembers); err != nil {
			h.Logger.Error("failed to scan insight row", slog.String("error", err.Error()))
			continue
		}
		row.Date = date.Format("2006-01-02")
		daily = append(daily, row)
	}

	// Fetch peak hours (last 7 days aggregated).
	hourRows, err := h.Pool.Query(r.Context(),
		`SELECT hour, SUM(messages)::INT AS total
		 FROM guild_insights_hourly
		 WHERE guild_id = $1 AND date >= CURRENT_DATE - 7
		 GROUP BY hour
		 ORDER BY hour ASC`,
		guildID,
	)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to load insights", err)
		return
	}
	defer hourRows.Close()

	peakHours := make([]InsightsHourlyRow, 0)
	for hourRows.Next() {
		var row InsightsHourlyRow
		if err := hourRows.Scan(&row.Hour, &row.Messages); err != nil {
			continue
		}
		peakHours = append(peakHours, row)
	}

	// Compute totals.
	var totalMembers int
	h.Pool.QueryRow(r.Context(),
		`SELECT COUNT(*) FROM guild_members WHERE guild_id = $1`, guildID,
	).Scan(&totalMembers)

	var totalMessages int
	for _, d := range daily {
		totalMessages += d.MessagesSent
	}

	// Growth rate: compare first vs last week members.
	var growthRate float64
	if len(daily) >= 14 {
		oldCount := daily[0].MemberCount
		newCount := daily[len(daily)-1].MemberCount
		if oldCount > 0 {
			growthRate = float64(newCount-oldCount) / float64(oldCount) * 100
		}
	}

	resp := InsightsResponse{
		Daily:         daily,
		PeakHours:     peakHours,
		TotalMembers:  totalMembers,
		TotalMessages: totalMessages,
		GrowthRate:    math.Round(growthRate*100) / 100,
	}

	apiutil.WriteJSON(w, http.StatusOK, resp)
}

// ensureTodaySnapshot creates or updates today's insight row from live data.
func (h *Handler) ensureTodaySnapshot(ctx context.Context, guildID string) {
	today := time.Now().Format("2006-01-02")

	var memberCount, messagesSent, reactionsAdded, activeMembers int

	h.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM guild_members WHERE guild_id = $1`, guildID,
	).Scan(&memberCount)

	h.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM messages m
		 JOIN channels c ON c.id = m.channel_id
		 WHERE c.guild_id = $1 AND m.created_at >= CURRENT_DATE`,
		guildID,
	).Scan(&messagesSent)

	h.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM reactions r
		 JOIN messages m ON m.id = r.message_id
		 JOIN channels c ON c.id = m.channel_id
		 WHERE c.guild_id = $1 AND r.created_at >= CURRENT_DATE`,
		guildID,
	).Scan(&reactionsAdded)

	h.Pool.QueryRow(ctx,
		`SELECT COUNT(DISTINCT m.author_id) FROM messages m
		 JOIN channels c ON c.id = m.channel_id
		 WHERE c.guild_id = $1 AND m.created_at >= CURRENT_DATE`,
		guildID,
	).Scan(&activeMembers)

	var membersJoined, membersLeft int
	h.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM guild_members WHERE guild_id = $1 AND joined_at >= CURRENT_DATE`,
		guildID,
	).Scan(&membersJoined)

	id := models.NewULID().String()

	h.Pool.Exec(ctx,
		`INSERT INTO guild_insights_daily
		     (id, guild_id, date, member_count, members_joined, members_left,
		      messages_sent, reactions_added, voice_minutes, active_members)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 0, $9)
		 ON CONFLICT (guild_id, date) DO UPDATE SET
		     member_count = EXCLUDED.member_count,
		     members_joined = EXCLUDED.members_joined,
		     messages_sent = EXCLUDED.messages_sent,
		     reactions_added = EXCLUDED.reactions_added,
		     active_members = EXCLUDED.active_members`,
		id, guildID, today, memberCount, membersJoined, membersLeft,
		messagesSent, reactionsAdded, activeMembers,
	)

	// Hourly snapshot for current hour.
	currentHour := time.Now().Hour()
	var hourlyMessages int
	h.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM messages m
		 JOIN channels c ON c.id = m.channel_id
		 WHERE c.guild_id = $1
		   AND m.created_at >= date_trunc('hour', now())
		   AND m.created_at < date_trunc('hour', now()) + INTERVAL '1 hour'`,
		guildID,
	).Scan(&hourlyMessages)

	hourlyID := models.NewULID().String()
	h.Pool.Exec(ctx,
		`INSERT INTO guild_insights_hourly (id, guild_id, date, hour, messages)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (guild_id, date, hour) DO UPDATE SET messages = EXCLUDED.messages`,
		hourlyID, guildID, today, currentHour, hourlyMessages,
	)
}

// ============================================================
// 2. Server Boost / Support System
// ============================================================

// BoostInfo represents a single boost.
type BoostInfo struct {
	ID        string  `json:"id"`
	GuildID   string  `json:"guild_id"`
	UserID    string  `json:"user_id"`
	Tier      int     `json:"tier"`
	StartedAt string  `json:"started_at"`
	ExpiresAt *string `json:"expires_at,omitempty"`
	Active    bool    `json:"active"`
	Username  string  `json:"username,omitempty"`
}

// BoostSummary is the guild boost summary.
type BoostSummary struct {
	BoostCount int         `json:"boost_count"`
	BoostTier  int         `json:"boost_tier"`
	Boosters   []BoostInfo `json:"boosters"`
	UserBoosted bool       `json:"user_boosted"`
}

// HandleGetBoosts returns the boost summary for a guild.
// GET /api/v1/guilds/{guildID}/boosts
func (h *Handler) HandleGetBoosts(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")

	if !h.isMember(r.Context(), guildID, userID) {
		apiutil.WriteError(w, http.StatusForbidden, "not_member", "You are not a member of this guild")
		return
	}

	rows, err := h.Pool.Query(r.Context(),
		`SELECT b.id, b.guild_id, b.user_id, b.tier, b.started_at, b.expires_at, b.active,
		        u.username
		 FROM guild_boosts b
		 JOIN users u ON u.id = b.user_id
		 WHERE b.guild_id = $1 AND b.active = true
		 ORDER BY b.started_at ASC`,
		guildID,
	)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to load boosts", err)
		return
	}
	defer rows.Close()

	boosters := make([]BoostInfo, 0)
	userBoosted := false
	for rows.Next() {
		var b BoostInfo
		var startedAt time.Time
		var expiresAt *time.Time
		if err := rows.Scan(&b.ID, &b.GuildID, &b.UserID, &b.Tier, &startedAt, &expiresAt, &b.Active, &b.Username); err != nil {
			continue
		}
		b.StartedAt = startedAt.Format(time.RFC3339)
		if expiresAt != nil {
			s := expiresAt.Format(time.RFC3339)
			b.ExpiresAt = &s
		}
		if b.UserID == userID {
			userBoosted = true
		}
		boosters = append(boosters, b)
	}

	boostCount := len(boosters)
	boostTier := computeBoostTier(boostCount)

	apiutil.WriteJSON(w, http.StatusOK, BoostSummary{
		BoostCount:  boostCount,
		BoostTier:   boostTier,
		Boosters:    boosters,
		UserBoosted: userBoosted,
	})
}

// HandleCreateBoost adds a boost to a guild.
// POST /api/v1/guilds/{guildID}/boosts
func (h *Handler) HandleCreateBoost(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")

	if !h.isMember(r.Context(), guildID, userID) {
		apiutil.WriteError(w, http.StatusForbidden, "not_member", "You are not a member of this guild")
		return
	}

	id := models.NewULID().String()

	var b BoostInfo
	var startedAt time.Time
	err := h.Pool.QueryRow(r.Context(),
		`INSERT INTO guild_boosts (id, guild_id, user_id, tier, started_at, active)
		 VALUES ($1, $2, $3, 1, NOW(), true)
		 ON CONFLICT (guild_id, user_id) DO UPDATE SET active = true, started_at = NOW()
		 RETURNING id, guild_id, user_id, tier, started_at, active`,
		id, guildID, userID,
	).Scan(&b.ID, &b.GuildID, &b.UserID, &b.Tier, &startedAt, &b.Active)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to create boost", err)
		return
	}
	b.StartedAt = startedAt.Format(time.RFC3339)

	// Update cached boost count on guild.
	h.updateGuildBoostCount(r.Context(), guildID)

	// Award achievement.
	h.awardAchievement(r.Context(), userID, "achv_booster")

	h.EventBus.PublishGuildEvent(r.Context(), events.SubjectGuildUpdate, "GUILD_UPDATE", guildID, map[string]interface{}{
		"id":       guildID,
		"boosted":  true,
		"booster":  userID,
	})

	apiutil.WriteJSON(w, http.StatusCreated, b)
}

// HandleRemoveBoost removes a user's boost from a guild.
// DELETE /api/v1/guilds/{guildID}/boosts
func (h *Handler) HandleRemoveBoost(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")

	tag, err := h.Pool.Exec(r.Context(),
		`UPDATE guild_boosts SET active = false WHERE guild_id = $1 AND user_id = $2 AND active = true`,
		guildID, userID,
	)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to remove boost", err)
		return
	}
	if tag.RowsAffected() == 0 {
		apiutil.WriteError(w, http.StatusNotFound, "not_boosted", "You are not boosting this guild")
		return
	}

	h.updateGuildBoostCount(r.Context(), guildID)

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) updateGuildBoostCount(ctx context.Context, guildID string) {
	var count int
	h.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM guild_boosts WHERE guild_id = $1 AND active = true`, guildID,
	).Scan(&count)
	tier := computeBoostTier(count)
	h.Pool.Exec(ctx,
		`UPDATE guilds SET boost_count = $2, boost_tier = $3 WHERE id = $1`,
		guildID, count, tier,
	)
}

func computeBoostTier(count int) int {
	switch {
	case count >= 14:
		return 3
	case count >= 7:
		return 2
	case count >= 2:
		return 1
	default:
		return 0
	}
}

// ============================================================
// 3. Vanity URL Marketplace
// ============================================================

type vanityClaimRequest struct {
	Code string `json:"code"`
}

// VanityClaimInfo represents a vanity URL claim.
type VanityClaimInfo struct {
	Code      string `json:"code"`
	GuildID   string `json:"guild_id"`
	ClaimedBy string `json:"claimed_by"`
	ClaimedAt string `json:"claimed_at"`
}

// HandleClaimVanityURL claims a vanity URL code for a guild.
// POST /api/v1/guilds/{guildID}/vanity-claim
func (h *Handler) HandleClaimVanityURL(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")

	if !h.isGuildAdmin(r.Context(), guildID, userID) {
		apiutil.WriteError(w, http.StatusForbidden, "forbidden", "Only guild admins can claim vanity URLs")
		return
	}

	var req vanityClaimRequest
	if !apiutil.DecodeJSON(w, r, &req) {
		return
	}

	code := strings.TrimSpace(strings.ToLower(req.Code))
	if len(code) < 3 || len(code) > 32 {
		apiutil.WriteError(w, http.StatusBadRequest, "invalid_code", "Vanity code must be 3-32 characters")
		return
	}

	// Only alphanumeric and hyphens.
	for _, c := range code {
		if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-') {
			apiutil.WriteError(w, http.StatusBadRequest, "invalid_code", "Vanity code may only contain lowercase letters, numbers, and hyphens")
			return
		}
	}

	var claimedAt time.Time
	var conflictCode string // "already_claimed" or "code_taken" if conflict detected

	err := apiutil.WithTx(r.Context(), h.Pool, func(tx pgx.Tx) error {
		// Check if code is already taken.
		var existingGuild string
		err := tx.QueryRow(r.Context(),
			`SELECT guild_id FROM vanity_url_claims WHERE code = $1`, code,
		).Scan(&existingGuild)
		if err == nil {
			if existingGuild == guildID {
				conflictCode = "already_claimed"
			} else {
				conflictCode = "code_taken"
			}
			return nil
		}
		if err != pgx.ErrNoRows {
			return err
		}

		// Release any existing claim by this guild.
		if _, err := tx.Exec(r.Context(),
			`DELETE FROM vanity_url_claims WHERE guild_id = $1`, guildID,
		); err != nil {
			return err
		}

		// Insert new claim.
		err = tx.QueryRow(r.Context(),
			`INSERT INTO vanity_url_claims (code, guild_id, claimed_by, claimed_at)
			 VALUES ($1, $2, $3, NOW())
			 RETURNING claimed_at`,
			code, guildID, userID,
		).Scan(&claimedAt)
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				conflictCode = "code_taken"
				return nil
			}
			return err
		}

		// Also update guilds.vanity_url.
		if _, err := tx.Exec(r.Context(),
			`UPDATE guilds SET vanity_url = $2 WHERE id = $1`, guildID, code,
		); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to claim vanity URL", err)
		return
	}
	if conflictCode == "already_claimed" {
		apiutil.WriteError(w, http.StatusConflict, "already_claimed", "Your guild already owns this vanity URL")
		return
	}
	if conflictCode == "code_taken" {
		apiutil.WriteError(w, http.StatusConflict, "code_taken", "This vanity code is already claimed")
		return
	}

	apiutil.WriteJSON(w, http.StatusCreated, VanityClaimInfo{
		Code:      code,
		GuildID:   guildID,
		ClaimedBy: userID,
		ClaimedAt: claimedAt.Format(time.RFC3339),
	})
}

// HandleReleaseVanityURL releases a guild's vanity URL claim.
// DELETE /api/v1/guilds/{guildID}/vanity-claim
func (h *Handler) HandleReleaseVanityURL(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")

	if !h.isGuildAdmin(r.Context(), guildID, userID) {
		apiutil.WriteError(w, http.StatusForbidden, "forbidden", "Only guild admins can release vanity URLs")
		return
	}

	err := apiutil.WithTx(r.Context(), h.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(r.Context(), `DELETE FROM vanity_url_claims WHERE guild_id = $1`, guildID); err != nil {
			return err
		}
		if _, err := tx.Exec(r.Context(), `UPDATE guilds SET vanity_url = NULL WHERE id = $1`, guildID); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to release vanity URL")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleCheckVanityAvailability checks if a vanity code is available.
// GET /api/v1/guilds/vanity-check?code=mycode
func (h *Handler) HandleCheckVanityAvailability(w http.ResponseWriter, r *http.Request) {
	code := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("code")))
	if !apiutil.RequireNonEmpty(w, "code query parameter", code) {
		return
	}

	var exists bool
	h.Pool.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM vanity_url_claims WHERE code = $1)`, code,
	).Scan(&exists)

	apiutil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"code":      code,
		"available": !exists,
	})
}

// ============================================================
// 4. User Achievement / Badge System
// ============================================================

// AchievementDef represents an achievement definition.
type AchievementDef struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Description      string `json:"description"`
	Icon             string `json:"icon"`
	Category         string `json:"category"`
	CriteriaType     string `json:"criteria_type"`
	CriteriaThreshold int   `json:"criteria_threshold"`
	Rarity           string `json:"rarity"`
}

// UserAchievement represents a user's earned achievement.
type UserAchievement struct {
	ID            string `json:"id"`
	AchievementID string `json:"achievement_id"`
	Name          string `json:"name"`
	Description   string `json:"description"`
	Icon          string `json:"icon"`
	Category      string `json:"category"`
	Rarity        string `json:"rarity"`
	EarnedAt      string `json:"earned_at"`
}

// HandleGetAchievements returns all achievement definitions.
// GET /api/v1/achievements
func (h *Handler) HandleGetAchievements(w http.ResponseWriter, r *http.Request) {
	rows, err := h.Pool.Query(r.Context(),
		`SELECT id, name, description, icon, category, criteria_type, criteria_threshold, rarity
		 FROM achievement_definitions
		 ORDER BY category, criteria_threshold ASC`,
	)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to load achievements", err)
		return
	}
	defer rows.Close()

	defs := make([]AchievementDef, 0)
	for rows.Next() {
		var d AchievementDef
		if err := rows.Scan(&d.ID, &d.Name, &d.Description, &d.Icon, &d.Category,
			&d.CriteriaType, &d.CriteriaThreshold, &d.Rarity); err != nil {
			continue
		}
		defs = append(defs, d)
	}

	apiutil.WriteJSON(w, http.StatusOK, defs)
}

// HandleGetUserAchievements returns achievements earned by a user.
// GET /api/v1/users/{userID}/achievements
func (h *Handler) HandleGetUserAchievements(w http.ResponseWriter, r *http.Request) {
	targetUserID := chi.URLParam(r, "userID")
	if targetUserID == "@me" {
		targetUserID = auth.UserIDFromContext(r.Context())
	}

	rows, err := h.Pool.Query(r.Context(),
		`SELECT ua.id, ua.achievement_id, ad.name, ad.description, ad.icon,
		        ad.category, ad.rarity, ua.earned_at
		 FROM user_achievements ua
		 JOIN achievement_definitions ad ON ad.id = ua.achievement_id
		 WHERE ua.user_id = $1
		 ORDER BY ua.earned_at DESC`,
		targetUserID,
	)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to load achievements", err)
		return
	}
	defer rows.Close()

	achievements := make([]UserAchievement, 0)
	for rows.Next() {
		var a UserAchievement
		var earnedAt time.Time
		if err := rows.Scan(&a.ID, &a.AchievementID, &a.Name, &a.Description, &a.Icon,
			&a.Category, &a.Rarity, &earnedAt); err != nil {
			continue
		}
		a.EarnedAt = earnedAt.Format(time.RFC3339)
		achievements = append(achievements, a)
	}

	apiutil.WriteJSON(w, http.StatusOK, achievements)
}

// awardAchievement grants an achievement to a user if not already earned.
func (h *Handler) awardAchievement(ctx context.Context, userID, achievementID string) {
	id := models.NewULID().String()
	_, err := h.Pool.Exec(ctx,
		`INSERT INTO user_achievements (id, user_id, achievement_id, earned_at)
		 VALUES ($1, $2, $3, NOW())
		 ON CONFLICT (user_id, achievement_id) DO NOTHING`,
		id, userID, achievementID,
	)
	if err != nil {
		h.Logger.Error("failed to award achievement",
			slog.String("user_id", userID),
			slog.String("achievement_id", achievementID),
			slog.String("error", err.Error()),
		)
	}
}

// HandleCheckAchievements triggers a check of all criteria-based achievements for the
// requesting user and awards any newly qualified ones. Called periodically by the client.
// POST /api/v1/users/@me/achievements/check
func (h *Handler) HandleCheckAchievements(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())

	// Get all achievement definitions.
	rows, err := h.Pool.Query(r.Context(),
		`SELECT id, criteria_type, criteria_threshold FROM achievement_definitions`,
	)
	if err != nil {
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to check achievements")
		return
	}
	defer rows.Close()

	type achievementCheck struct {
		id        string
		cType     string
		threshold int
	}
	checks := make([]achievementCheck, 0)
	for rows.Next() {
		var c achievementCheck
		if rows.Scan(&c.id, &c.cType, &c.threshold) == nil {
			checks = append(checks, c)
		}
	}

	awarded := 0
	for _, c := range checks {
		var currentVal int
		var err error

		switch c.cType {
		case "message_count":
			err = h.Pool.QueryRow(r.Context(),
				`SELECT COUNT(*) FROM messages WHERE author_id = $1`, userID,
			).Scan(&currentVal)
		case "reaction_count":
			err = h.Pool.QueryRow(r.Context(),
				`SELECT COUNT(*) FROM reactions WHERE user_id = $1`, userID,
			).Scan(&currentVal)
		case "guild_join_count":
			err = h.Pool.QueryRow(r.Context(),
				`SELECT COUNT(*) FROM guild_members WHERE user_id = $1`, userID,
			).Scan(&currentVal)
		case "account_age_days":
			var createdAt time.Time
			err = h.Pool.QueryRow(r.Context(),
				`SELECT created_at FROM users WHERE id = $1`, userID,
			).Scan(&createdAt)
			if err == nil {
				currentVal = int(time.Since(createdAt).Hours() / 24)
			}
		case "boost":
			// Handled on boost creation.
			continue
		case "voice_minutes":
			// Would need voice tracking — skip for now, tracked via member_xp.
			continue
		default:
			continue
		}

		if err != nil {
			continue
		}

		if currentVal >= c.threshold {
			h.awardAchievement(r.Context(), userID, c.id)
			awarded++
		}
	}

	apiutil.WriteJSON(w, http.StatusOK, map[string]int{"awarded": awarded})
}

// ============================================================
// 5. Leveling / XP System
// ============================================================

// LevelingConfig represents a guild's leveling configuration.
type LevelingConfig struct {
	GuildID          string `json:"guild_id"`
	Enabled          bool   `json:"enabled"`
	XPPerMessage     int    `json:"xp_per_message"`
	XPCooldownSeconds int   `json:"xp_cooldown_seconds"`
	LevelUpChannelID *string `json:"level_up_channel_id,omitempty"`
	LevelUpMessage   string `json:"level_up_message"`
	StackRoles       bool   `json:"stack_roles"`
}

// LevelRole maps a level to a role.
type LevelRole struct {
	ID      string `json:"id"`
	GuildID string `json:"guild_id"`
	Level   int    `json:"level"`
	RoleID  string `json:"role_id"`
}

// MemberXP represents a member's XP state.
type MemberXP struct {
	GuildID        string  `json:"guild_id"`
	UserID         string  `json:"user_id"`
	XP             int64   `json:"xp"`
	Level          int     `json:"level"`
	MessagesCounted int    `json:"messages_counted"`
	Username       string  `json:"username,omitempty"`
	DisplayName    *string `json:"display_name,omitempty"`
	AvatarID       *string `json:"avatar_id,omitempty"`
}

// HandleGetLevelingConfig returns the leveling config for a guild.
// GET /api/v1/guilds/{guildID}/leveling
func (h *Handler) HandleGetLevelingConfig(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")

	if !h.isMember(r.Context(), guildID, userID) {
		apiutil.WriteError(w, http.StatusForbidden, "not_member", "You are not a member of this guild")
		return
	}

	var cfg LevelingConfig
	err := h.Pool.QueryRow(r.Context(),
		`SELECT guild_id, enabled, xp_per_message, xp_cooldown_seconds,
		        level_up_channel_id, level_up_message, stack_roles
		 FROM guild_leveling_config
		 WHERE guild_id = $1`,
		guildID,
	).Scan(&cfg.GuildID, &cfg.Enabled, &cfg.XPPerMessage, &cfg.XPCooldownSeconds,
		&cfg.LevelUpChannelID, &cfg.LevelUpMessage, &cfg.StackRoles)
	if err == pgx.ErrNoRows {
		cfg = LevelingConfig{
			GuildID:           guildID,
			Enabled:           false,
			XPPerMessage:      15,
			XPCooldownSeconds: 60,
			LevelUpMessage:    "Congratulations {user}, you reached level {level}!",
			StackRoles:        true,
		}
	} else if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to load leveling config", err)
		return
	}

	// Load level roles.
	roleRows, err := h.Pool.Query(r.Context(),
		`SELECT id, guild_id, level, role_id FROM guild_level_roles WHERE guild_id = $1 ORDER BY level ASC`,
		guildID,
	)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to load leveling config", err)
		return
	}
	defer roleRows.Close()

	levelRoles := make([]LevelRole, 0)
	for roleRows.Next() {
		var lr LevelRole
		if roleRows.Scan(&lr.ID, &lr.GuildID, &lr.Level, &lr.RoleID) == nil {
			levelRoles = append(levelRoles, lr)
		}
	}

	apiutil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"config": cfg,
		"level_roles": levelRoles,
	})
}

type updateLevelingConfigRequest struct {
	Enabled           *bool   `json:"enabled"`
	XPPerMessage      *int    `json:"xp_per_message"`
	XPCooldownSeconds *int    `json:"xp_cooldown_seconds"`
	LevelUpChannelID  *string `json:"level_up_channel_id"`
	LevelUpMessage    *string `json:"level_up_message"`
	StackRoles        *bool   `json:"stack_roles"`
}

// HandleUpdateLevelingConfig updates the leveling config for a guild.
// PATCH /api/v1/guilds/{guildID}/leveling
func (h *Handler) HandleUpdateLevelingConfig(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")

	if !h.isGuildAdmin(r.Context(), guildID, userID) {
		apiutil.WriteError(w, http.StatusForbidden, "forbidden", "Only guild admins can update leveling config")
		return
	}

	var req updateLevelingConfigRequest
	if !apiutil.DecodeJSON(w, r, &req) {
		return
	}

	var cfg LevelingConfig
	err := h.Pool.QueryRow(r.Context(),
		`INSERT INTO guild_leveling_config (guild_id, enabled, xp_per_message, xp_cooldown_seconds,
		     level_up_channel_id, level_up_message, stack_roles, updated_at)
		 VALUES ($1,
		     COALESCE($2, false),
		     COALESCE($3, 15),
		     COALESCE($4, 60),
		     $5,
		     COALESCE($6, 'Congratulations {user}, you reached level {level}!'),
		     COALESCE($7, true),
		     NOW())
		 ON CONFLICT (guild_id) DO UPDATE SET
		     enabled = COALESCE($2, guild_leveling_config.enabled),
		     xp_per_message = COALESCE($3, guild_leveling_config.xp_per_message),
		     xp_cooldown_seconds = COALESCE($4, guild_leveling_config.xp_cooldown_seconds),
		     level_up_channel_id = COALESCE($5, guild_leveling_config.level_up_channel_id),
		     level_up_message = COALESCE($6, guild_leveling_config.level_up_message),
		     stack_roles = COALESCE($7, guild_leveling_config.stack_roles),
		     updated_at = NOW()
		 RETURNING guild_id, enabled, xp_per_message, xp_cooldown_seconds,
		           level_up_channel_id, level_up_message, stack_roles`,
		guildID, req.Enabled, req.XPPerMessage, req.XPCooldownSeconds,
		req.LevelUpChannelID, req.LevelUpMessage, req.StackRoles,
	).Scan(&cfg.GuildID, &cfg.Enabled, &cfg.XPPerMessage, &cfg.XPCooldownSeconds,
		&cfg.LevelUpChannelID, &cfg.LevelUpMessage, &cfg.StackRoles)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to update leveling config", err)
		return
	}

	apiutil.WriteJSON(w, http.StatusOK, cfg)
}

type addLevelRoleRequest struct {
	Level  int    `json:"level"`
	RoleID string `json:"role_id"`
}

// HandleAddLevelRole adds a role reward at a level.
// POST /api/v1/guilds/{guildID}/leveling/roles
func (h *Handler) HandleAddLevelRole(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")

	if !h.isGuildAdmin(r.Context(), guildID, userID) {
		apiutil.WriteError(w, http.StatusForbidden, "forbidden", "Only guild admins can manage level roles")
		return
	}

	var req addLevelRoleRequest
	if !apiutil.DecodeJSON(w, r, &req) {
		return
	}

	if req.Level < 1 || req.Level > 100 {
		apiutil.WriteError(w, http.StatusBadRequest, "invalid_level", "Level must be between 1 and 100")
		return
	}
	if !apiutil.RequireNonEmpty(w, "role_id", req.RoleID) {
		return
	}

	id := models.NewULID().String()
	var lr LevelRole
	err := h.Pool.QueryRow(r.Context(),
		`INSERT INTO guild_level_roles (id, guild_id, level, role_id)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, guild_id, level, role_id`,
		id, guildID, req.Level, req.RoleID,
	).Scan(&lr.ID, &lr.GuildID, &lr.Level, &lr.RoleID)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint") {
			apiutil.WriteError(w, http.StatusConflict, "already_exists", "A level role already exists for this level and role")
			return
		}
		apiutil.InternalError(w, h.Logger, "Failed to add level role", err)
		return
	}

	apiutil.WriteJSON(w, http.StatusCreated, lr)
}

// HandleDeleteLevelRole removes a level role reward.
// DELETE /api/v1/guilds/{guildID}/leveling/roles/{roleMapID}
func (h *Handler) HandleDeleteLevelRole(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")
	roleMapID := chi.URLParam(r, "roleMapID")

	if !h.isGuildAdmin(r.Context(), guildID, userID) {
		apiutil.WriteError(w, http.StatusForbidden, "forbidden", "Only guild admins can manage level roles")
		return
	}

	tag, err := h.Pool.Exec(r.Context(),
		`DELETE FROM guild_level_roles WHERE id = $1 AND guild_id = $2`,
		roleMapID, guildID,
	)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to delete level role", err)
		return
	}
	if tag.RowsAffected() == 0 {
		apiutil.WriteError(w, http.StatusNotFound, "not_found", "Level role not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleGetLeaderboard returns the XP leaderboard for a guild.
// GET /api/v1/guilds/{guildID}/leveling/leaderboard?limit=25
func (h *Handler) HandleGetLeaderboard(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")

	if !h.isMember(r.Context(), guildID, userID) {
		apiutil.WriteError(w, http.StatusForbidden, "not_member", "You are not a member of this guild")
		return
	}

	limit := 25
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}

	rows, err := h.Pool.Query(r.Context(),
		`SELECT mx.guild_id, mx.user_id, mx.xp, mx.level, mx.messages_counted,
		        u.username, u.display_name, u.avatar_id
		 FROM member_xp mx
		 JOIN users u ON u.id = mx.user_id
		 WHERE mx.guild_id = $1
		 ORDER BY mx.xp DESC
		 LIMIT $2`,
		guildID, limit,
	)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to load leaderboard", err)
		return
	}
	defer rows.Close()

	leaderboard := make([]MemberXP, 0)
	for rows.Next() {
		var m MemberXP
		if err := rows.Scan(&m.GuildID, &m.UserID, &m.XP, &m.Level, &m.MessagesCounted,
			&m.Username, &m.DisplayName, &m.AvatarID); err != nil {
			continue
		}
		leaderboard = append(leaderboard, m)
	}

	apiutil.WriteJSON(w, http.StatusOK, leaderboard)
}

// HandleGetMemberXP returns a specific member's XP.
// GET /api/v1/guilds/{guildID}/leveling/members/{memberID}
func (h *Handler) HandleGetMemberXP(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")
	memberID := chi.URLParam(r, "memberID")

	if memberID == "@me" {
		memberID = userID
	}

	if !h.isMember(r.Context(), guildID, userID) {
		apiutil.WriteError(w, http.StatusForbidden, "not_member", "You are not a member of this guild")
		return
	}

	var m MemberXP
	err := h.Pool.QueryRow(r.Context(),
		`SELECT mx.guild_id, mx.user_id, mx.xp, mx.level, mx.messages_counted,
		        u.username, u.display_name, u.avatar_id
		 FROM member_xp mx
		 JOIN users u ON u.id = mx.user_id
		 WHERE mx.guild_id = $1 AND mx.user_id = $2`,
		guildID, memberID,
	).Scan(&m.GuildID, &m.UserID, &m.XP, &m.Level, &m.MessagesCounted,
		&m.Username, &m.DisplayName, &m.AvatarID)
	if err == pgx.ErrNoRows {
		m = MemberXP{GuildID: guildID, UserID: memberID, XP: 0, Level: 0, MessagesCounted: 0}
	} else if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to load member XP", err)
		return
	}

	// Compute XP needed for next level.
	nextLevelXP := XPForLevel(m.Level + 1)

	apiutil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"member":       m,
		"next_level_xp": nextLevelXP,
	})
}

// XPForLevel calculates the total XP needed to reach a given level.
// Uses a quadratic curve: level^2 * 100
func XPForLevel(level int) int64 {
	return int64(level) * int64(level) * 100
}

// LevelFromXP computes the level for a given XP total.
func LevelFromXP(xp int64) int {
	return int(math.Floor(math.Sqrt(float64(xp) / 100)))
}

// AwardXP grants XP to a member and handles level-ups. Called by the message handler
// or a NATS subscriber when a message is created.
func (h *Handler) AwardXP(ctx context.Context, guildID, userID string) {
	// Check if leveling is enabled.
	var enabled bool
	var xpPerMessage, cooldownSec int
	var levelUpChannelID *string
	var levelUpMessage string
	var stackRoles bool

	err := h.Pool.QueryRow(ctx,
		`SELECT enabled, xp_per_message, xp_cooldown_seconds, level_up_channel_id, level_up_message, stack_roles
		 FROM guild_leveling_config WHERE guild_id = $1`,
		guildID,
	).Scan(&enabled, &xpPerMessage, &cooldownSec, &levelUpChannelID, &levelUpMessage, &stackRoles)
	if err != nil || !enabled {
		return
	}

	// Check cooldown.
	var lastXPAt *time.Time
	h.Pool.QueryRow(ctx,
		`SELECT last_xp_at FROM member_xp WHERE guild_id = $1 AND user_id = $2`,
		guildID, userID,
	).Scan(&lastXPAt)

	if lastXPAt != nil && time.Since(*lastXPAt).Seconds() < float64(cooldownSec) {
		return
	}

	// Award XP.
	var newXP int64
	var oldLevel int
	err = h.Pool.QueryRow(ctx,
		`INSERT INTO member_xp (guild_id, user_id, xp, level, messages_counted, last_xp_at)
		 VALUES ($1, $2, $3, 0, 1, NOW())
		 ON CONFLICT (guild_id, user_id) DO UPDATE
		     SET xp = member_xp.xp + $3,
		         messages_counted = member_xp.messages_counted + 1,
		         last_xp_at = NOW()
		 RETURNING xp, level`,
		guildID, userID, xpPerMessage,
	).Scan(&newXP, &oldLevel)
	if err != nil {
		h.Logger.Error("failed to award xp", slog.String("error", err.Error()))
		return
	}

	newLevel := LevelFromXP(newXP)
	if newLevel > oldLevel {
		// Update level.
		h.Pool.Exec(ctx,
			`UPDATE member_xp SET level = $3 WHERE guild_id = $1 AND user_id = $2`,
			guildID, userID, newLevel,
		)

		// Assign level roles.
		h.assignLevelRoles(ctx, guildID, userID, newLevel, stackRoles)

		// Send level-up message.
		if levelUpChannelID != nil && *levelUpChannelID != "" {
			var username string
			h.Pool.QueryRow(ctx, `SELECT username FROM users WHERE id = $1`, userID).Scan(&username)
			msg := strings.ReplaceAll(levelUpMessage, "{user}", fmt.Sprintf("<@%s>", userID))
			msg = strings.ReplaceAll(msg, "{level}", strconv.Itoa(newLevel))
			msg = strings.ReplaceAll(msg, "{username}", username)

			msgID := models.NewULID().String()
			h.Pool.Exec(ctx,
				`INSERT INTO messages (id, channel_id, author_id, content, message_type, created_at)
				 VALUES ($1, $2, $3, $4, 'default', NOW())`,
				msgID, *levelUpChannelID, userID, msg,
			)

			h.EventBus.PublishChannelEvent(ctx, events.SubjectMessageCreate, "MESSAGE_CREATE", *levelUpChannelID, map[string]interface{}{
				"id":         msgID,
				"channel_id": *levelUpChannelID,
				"author_id":  userID,
				"content":    msg,
			})
		}
	}
}

func (h *Handler) assignLevelRoles(ctx context.Context, guildID, userID string, level int, stackRoles bool) {
	var query string
	if stackRoles {
		query = `SELECT role_id FROM guild_level_roles WHERE guild_id = $1 AND level <= $2`
	} else {
		// Only assign the highest level role.
		query = `SELECT role_id FROM guild_level_roles WHERE guild_id = $1 AND level <= $2 ORDER BY level DESC LIMIT 1`
	}

	rows, err := h.Pool.Query(ctx, query, guildID, level)
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var roleID string
		if rows.Scan(&roleID) == nil {
			h.Pool.Exec(ctx,
				`INSERT INTO member_roles (guild_id, user_id, role_id)
				 VALUES ($1, $2, $3)
				 ON CONFLICT DO NOTHING`,
				guildID, userID, roleID,
			)
		}
	}
}

// ============================================================
// 6. Starboard
// ============================================================

// StarboardConfig represents a guild's starboard settings.
type StarboardConfig struct {
	GuildID    string `json:"guild_id"`
	Enabled    bool   `json:"enabled"`
	ChannelID  *string `json:"channel_id,omitempty"`
	Emoji      string `json:"emoji"`
	Threshold  int    `json:"threshold"`
	SelfStar   bool   `json:"self_star"`
	NSFWAllowed bool  `json:"nsfw_allowed"`
}

// StarboardEntry represents a starred message.
type StarboardEntry struct {
	ID                 string  `json:"id"`
	GuildID            string  `json:"guild_id"`
	SourceMessageID    string  `json:"source_message_id"`
	SourceChannelID    string  `json:"source_channel_id"`
	StarboardMessageID *string `json:"starboard_message_id,omitempty"`
	StarCount          int     `json:"star_count"`
	AuthorID           string  `json:"author_id"`
	CreatedAt          string  `json:"created_at"`
}

// HandleGetStarboardConfig returns the starboard config for a guild.
// GET /api/v1/guilds/{guildID}/starboard
func (h *Handler) HandleGetStarboardConfig(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")

	if !h.isMember(r.Context(), guildID, userID) {
		apiutil.WriteError(w, http.StatusForbidden, "not_member", "You are not a member of this guild")
		return
	}

	var cfg StarboardConfig
	err := h.Pool.QueryRow(r.Context(),
		`SELECT guild_id, enabled, channel_id, emoji, threshold, self_star, nsfw_allowed
		 FROM guild_starboard_config
		 WHERE guild_id = $1`,
		guildID,
	).Scan(&cfg.GuildID, &cfg.Enabled, &cfg.ChannelID, &cfg.Emoji,
		&cfg.Threshold, &cfg.SelfStar, &cfg.NSFWAllowed)
	if err == pgx.ErrNoRows {
		cfg = StarboardConfig{
			GuildID:   guildID,
			Enabled:   false,
			Emoji:     "star",
			Threshold: 3,
			SelfStar:  false,
		}
	} else if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to load starboard config", err)
		return
	}

	apiutil.WriteJSON(w, http.StatusOK, cfg)
}

type updateStarboardConfigRequest struct {
	Enabled     *bool   `json:"enabled"`
	ChannelID   *string `json:"channel_id"`
	Emoji       *string `json:"emoji"`
	Threshold   *int    `json:"threshold"`
	SelfStar    *bool   `json:"self_star"`
	NSFWAllowed *bool   `json:"nsfw_allowed"`
}

// HandleUpdateStarboardConfig updates the starboard config for a guild.
// PATCH /api/v1/guilds/{guildID}/starboard
func (h *Handler) HandleUpdateStarboardConfig(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")

	if !h.isGuildAdmin(r.Context(), guildID, userID) {
		apiutil.WriteError(w, http.StatusForbidden, "forbidden", "Only guild admins can update starboard config")
		return
	}

	var req updateStarboardConfigRequest
	if !apiutil.DecodeJSON(w, r, &req) {
		return
	}

	if req.Threshold != nil && (*req.Threshold < 1 || *req.Threshold > 100) {
		apiutil.WriteError(w, http.StatusBadRequest, "invalid_threshold", "Threshold must be between 1 and 100")
		return
	}

	var cfg StarboardConfig
	err := h.Pool.QueryRow(r.Context(),
		`INSERT INTO guild_starboard_config (guild_id, enabled, channel_id, emoji, threshold, self_star, nsfw_allowed, updated_at)
		 VALUES ($1,
		     COALESCE($2, false),
		     $3,
		     COALESCE($4, 'star'),
		     COALESCE($5, 3),
		     COALESCE($6, false),
		     COALESCE($7, false),
		     NOW())
		 ON CONFLICT (guild_id) DO UPDATE SET
		     enabled = COALESCE($2, guild_starboard_config.enabled),
		     channel_id = COALESCE($3, guild_starboard_config.channel_id),
		     emoji = COALESCE($4, guild_starboard_config.emoji),
		     threshold = COALESCE($5, guild_starboard_config.threshold),
		     self_star = COALESCE($6, guild_starboard_config.self_star),
		     nsfw_allowed = COALESCE($7, guild_starboard_config.nsfw_allowed),
		     updated_at = NOW()
		 RETURNING guild_id, enabled, channel_id, emoji, threshold, self_star, nsfw_allowed`,
		guildID, req.Enabled, req.ChannelID, req.Emoji, req.Threshold, req.SelfStar, req.NSFWAllowed,
	).Scan(&cfg.GuildID, &cfg.Enabled, &cfg.ChannelID, &cfg.Emoji,
		&cfg.Threshold, &cfg.SelfStar, &cfg.NSFWAllowed)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to update starboard config", err)
		return
	}

	apiutil.WriteJSON(w, http.StatusOK, cfg)
}

// HandleGetStarboardEntries returns starred messages for a guild.
// GET /api/v1/guilds/{guildID}/starboard/entries?limit=25
func (h *Handler) HandleGetStarboardEntries(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")

	if !h.isMember(r.Context(), guildID, userID) {
		apiutil.WriteError(w, http.StatusForbidden, "not_member", "You are not a member of this guild")
		return
	}

	limit := 25
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}

	rows, err := h.Pool.Query(r.Context(),
		`SELECT id, guild_id, source_message_id, source_channel_id,
		        starboard_message_id, star_count, author_id, created_at
		 FROM starboard_entries
		 WHERE guild_id = $1
		 ORDER BY star_count DESC, created_at DESC
		 LIMIT $2`,
		guildID, limit,
	)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to load starboard", err)
		return
	}
	defer rows.Close()

	entries := make([]StarboardEntry, 0)
	for rows.Next() {
		var e StarboardEntry
		var createdAt time.Time
		if err := rows.Scan(&e.ID, &e.GuildID, &e.SourceMessageID, &e.SourceChannelID,
			&e.StarboardMessageID, &e.StarCount, &e.AuthorID, &createdAt); err != nil {
			continue
		}
		e.CreatedAt = createdAt.Format(time.RFC3339)
		entries = append(entries, e)
	}

	apiutil.WriteJSON(w, http.StatusOK, entries)
}

// CheckStarboard evaluates whether a message has reached the starboard threshold
// and posts/updates it in the starboard channel. Called by the reaction handler.
func (h *Handler) CheckStarboard(ctx context.Context, guildID, channelID, messageID, emoji string) {
	var cfg StarboardConfig
	err := h.Pool.QueryRow(ctx,
		`SELECT guild_id, enabled, channel_id, emoji, threshold, self_star, nsfw_allowed
		 FROM guild_starboard_config WHERE guild_id = $1`,
		guildID,
	).Scan(&cfg.GuildID, &cfg.Enabled, &cfg.ChannelID, &cfg.Emoji,
		&cfg.Threshold, &cfg.SelfStar, &cfg.NSFWAllowed)
	if err != nil || !cfg.Enabled || cfg.ChannelID == nil {
		return
	}

	// Only match the configured emoji.
	if emoji != cfg.Emoji {
		return
	}

	// Count matching reactions on the message.
	var count int
	h.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM reactions WHERE message_id = $1 AND emoji = $2`,
		messageID, emoji,
	).Scan(&count)

	// If self-star is disabled, subtract author's own reaction.
	if !cfg.SelfStar {
		var authorID string
		h.Pool.QueryRow(ctx,
			`SELECT author_id FROM messages WHERE id = $1`, messageID,
		).Scan(&authorID)
		var selfStarred bool
		h.Pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM reactions WHERE message_id = $1 AND emoji = $2 AND user_id = $3)`,
			messageID, emoji, authorID,
		).Scan(&selfStarred)
		if selfStarred {
			count--
		}
	}

	if count < cfg.Threshold {
		// Update existing entry star count if it exists.
		h.Pool.Exec(ctx,
			`UPDATE starboard_entries SET star_count = $3
			 WHERE guild_id = $1 AND source_message_id = $2`,
			guildID, messageID, count,
		)
		return
	}

	// Check NSFW.
	if !cfg.NSFWAllowed {
		var isNSFW bool
		h.Pool.QueryRow(ctx,
			`SELECT nsfw FROM channels WHERE id = $1`, channelID,
		).Scan(&isNSFW)
		if isNSFW {
			return
		}
	}

	// Check if already on starboard.
	var existingID string
	err = h.Pool.QueryRow(ctx,
		`SELECT id FROM starboard_entries WHERE guild_id = $1 AND source_message_id = $2`,
		guildID, messageID,
	).Scan(&existingID)

	if err == nil {
		// Update star count.
		h.Pool.Exec(ctx,
			`UPDATE starboard_entries SET star_count = $2 WHERE id = $1`,
			existingID, count,
		)
		return
	}

	// Get message author.
	var authorID string
	h.Pool.QueryRow(ctx,
		`SELECT author_id FROM messages WHERE id = $1`, messageID,
	).Scan(&authorID)

	// Insert new starboard entry.
	entryID := models.NewULID().String()
	h.Pool.Exec(ctx,
		`INSERT INTO starboard_entries (id, guild_id, source_message_id, source_channel_id,
		     star_count, author_id, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, NOW())
		 ON CONFLICT (guild_id, source_message_id) DO UPDATE SET star_count = EXCLUDED.star_count`,
		entryID, guildID, messageID, channelID, count, authorID,
	)

	// Post notification in starboard channel.
	var content *string
	h.Pool.QueryRow(ctx,
		`SELECT content FROM messages WHERE id = $1`, messageID,
	).Scan(&content)

	starMsg := fmt.Sprintf("**%d** %s | <#%s>\n", count, cfg.Emoji, channelID)
	if content != nil {
		starMsg += *content
	}

	starMsgID := models.NewULID().String()
	h.Pool.Exec(ctx,
		`INSERT INTO messages (id, channel_id, author_id, content, message_type, created_at)
		 VALUES ($1, $2, $3, $4, 'default', NOW())`,
		starMsgID, *cfg.ChannelID, authorID, starMsg,
	)

	// Update the entry with the starboard message ID.
	h.Pool.Exec(ctx,
		`UPDATE starboard_entries SET starboard_message_id = $2 WHERE id = $1`,
		entryID, starMsgID,
	)

	h.EventBus.PublishChannelEvent(ctx, events.SubjectMessageCreate, "MESSAGE_CREATE", *cfg.ChannelID, map[string]interface{}{
		"id":         starMsgID,
		"channel_id": *cfg.ChannelID,
		"author_id":  authorID,
		"content":    starMsg,
	})
}

// ============================================================
// 7. Welcome Message Automation
// ============================================================

// WelcomeConfig represents a guild's welcome message settings.
type WelcomeConfig struct {
	GuildID       string  `json:"guild_id"`
	Enabled       bool    `json:"enabled"`
	ChannelID     *string `json:"channel_id,omitempty"`
	Message       string  `json:"message"`
	DMEnabled     bool    `json:"dm_enabled"`
	DMMessage     string  `json:"dm_message"`
	EmbedEnabled  bool    `json:"embed_enabled"`
	EmbedColor    *string `json:"embed_color,omitempty"`
	EmbedTitle    *string `json:"embed_title,omitempty"`
	EmbedImageURL *string `json:"embed_image_url,omitempty"`
}

// HandleGetWelcomeConfig returns the welcome config for a guild.
// GET /api/v1/guilds/{guildID}/welcome
func (h *Handler) HandleGetWelcomeConfig(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")

	if !h.isMember(r.Context(), guildID, userID) {
		apiutil.WriteError(w, http.StatusForbidden, "not_member", "You are not a member of this guild")
		return
	}

	var cfg WelcomeConfig
	err := h.Pool.QueryRow(r.Context(),
		`SELECT guild_id, enabled, channel_id, message, dm_enabled, dm_message,
		        embed_enabled, embed_color, embed_title, embed_image_url
		 FROM guild_welcome_config
		 WHERE guild_id = $1`,
		guildID,
	).Scan(&cfg.GuildID, &cfg.Enabled, &cfg.ChannelID, &cfg.Message,
		&cfg.DMEnabled, &cfg.DMMessage, &cfg.EmbedEnabled,
		&cfg.EmbedColor, &cfg.EmbedTitle, &cfg.EmbedImageURL)
	if err == pgx.ErrNoRows {
		cfg = WelcomeConfig{
			GuildID: guildID,
			Enabled: false,
			Message: "Welcome to the server, {user}!",
			DMMessage: "Welcome to {guild}! Please read the rules.",
		}
	} else if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to load welcome config", err)
		return
	}

	apiutil.WriteJSON(w, http.StatusOK, cfg)
}

type updateWelcomeConfigRequest struct {
	Enabled       *bool   `json:"enabled"`
	ChannelID     *string `json:"channel_id"`
	Message       *string `json:"message"`
	DMEnabled     *bool   `json:"dm_enabled"`
	DMMessage     *string `json:"dm_message"`
	EmbedEnabled  *bool   `json:"embed_enabled"`
	EmbedColor    *string `json:"embed_color"`
	EmbedTitle    *string `json:"embed_title"`
	EmbedImageURL *string `json:"embed_image_url"`
}

// HandleUpdateWelcomeConfig updates the welcome config for a guild.
// PATCH /api/v1/guilds/{guildID}/welcome
func (h *Handler) HandleUpdateWelcomeConfig(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")

	if !h.isGuildAdmin(r.Context(), guildID, userID) {
		apiutil.WriteError(w, http.StatusForbidden, "forbidden", "Only guild admins can update welcome config")
		return
	}

	var req updateWelcomeConfigRequest
	if !apiutil.DecodeJSON(w, r, &req) {
		return
	}

	var cfg WelcomeConfig
	err := h.Pool.QueryRow(r.Context(),
		`INSERT INTO guild_welcome_config
		     (guild_id, enabled, channel_id, message, dm_enabled, dm_message,
		      embed_enabled, embed_color, embed_title, embed_image_url, updated_at)
		 VALUES ($1,
		     COALESCE($2, false),
		     $3,
		     COALESCE($4, 'Welcome to the server, {user}!'),
		     COALESCE($5, false),
		     COALESCE($6, 'Welcome to {guild}! Please read the rules.'),
		     COALESCE($7, false),
		     COALESCE($8, '#5865F2'),
		     COALESCE($9, 'Welcome!'),
		     $10,
		     NOW())
		 ON CONFLICT (guild_id) DO UPDATE SET
		     enabled = COALESCE($2, guild_welcome_config.enabled),
		     channel_id = COALESCE($3, guild_welcome_config.channel_id),
		     message = COALESCE($4, guild_welcome_config.message),
		     dm_enabled = COALESCE($5, guild_welcome_config.dm_enabled),
		     dm_message = COALESCE($6, guild_welcome_config.dm_message),
		     embed_enabled = COALESCE($7, guild_welcome_config.embed_enabled),
		     embed_color = COALESCE($8, guild_welcome_config.embed_color),
		     embed_title = COALESCE($9, guild_welcome_config.embed_title),
		     embed_image_url = COALESCE($10, guild_welcome_config.embed_image_url),
		     updated_at = NOW()
		 RETURNING guild_id, enabled, channel_id, message, dm_enabled, dm_message,
		           embed_enabled, embed_color, embed_title, embed_image_url`,
		guildID, req.Enabled, req.ChannelID, req.Message, req.DMEnabled, req.DMMessage,
		req.EmbedEnabled, req.EmbedColor, req.EmbedTitle, req.EmbedImageURL,
	).Scan(&cfg.GuildID, &cfg.Enabled, &cfg.ChannelID, &cfg.Message,
		&cfg.DMEnabled, &cfg.DMMessage, &cfg.EmbedEnabled,
		&cfg.EmbedColor, &cfg.EmbedTitle, &cfg.EmbedImageURL)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to update welcome config", err)
		return
	}

	apiutil.WriteJSON(w, http.StatusOK, cfg)
}

// SendWelcomeMessage sends a welcome message when a user joins a guild.
// Called by the guild member add handler or NATS subscriber.
func (h *Handler) SendWelcomeMessage(ctx context.Context, guildID, userID string) {
	var cfg WelcomeConfig
	err := h.Pool.QueryRow(ctx,
		`SELECT guild_id, enabled, channel_id, message, dm_enabled, dm_message,
		        embed_enabled, embed_color, embed_title, embed_image_url
		 FROM guild_welcome_config WHERE guild_id = $1`,
		guildID,
	).Scan(&cfg.GuildID, &cfg.Enabled, &cfg.ChannelID, &cfg.Message,
		&cfg.DMEnabled, &cfg.DMMessage, &cfg.EmbedEnabled,
		&cfg.EmbedColor, &cfg.EmbedTitle, &cfg.EmbedImageURL)
	if err != nil || !cfg.Enabled {
		return
	}

	var username string
	var guildName string
	h.Pool.QueryRow(ctx, `SELECT username FROM users WHERE id = $1`, userID).Scan(&username)
	h.Pool.QueryRow(ctx, `SELECT name FROM guilds WHERE id = $1`, guildID).Scan(&guildName)

	// Post welcome message in channel.
	if cfg.ChannelID != nil && *cfg.ChannelID != "" {
		msg := strings.ReplaceAll(cfg.Message, "{user}", fmt.Sprintf("<@%s>", userID))
		msg = strings.ReplaceAll(msg, "{username}", username)
		msg = strings.ReplaceAll(msg, "{guild}", guildName)
		msg = strings.ReplaceAll(msg, "{membercount}", "")

		msgID := models.NewULID().String()
		h.Pool.Exec(ctx,
			`INSERT INTO messages (id, channel_id, author_id, content, message_type, created_at)
			 VALUES ($1, $2, $3, $4, 'system_join', NOW())`,
			msgID, *cfg.ChannelID, userID, msg,
		)

		h.EventBus.PublishChannelEvent(ctx, events.SubjectMessageCreate, "MESSAGE_CREATE", *cfg.ChannelID, map[string]interface{}{
			"id":           msgID,
			"channel_id":   *cfg.ChannelID,
			"author_id":    userID,
			"content":      msg,
			"message_type": "system_join",
		})
	}
}

// ============================================================
// 8. Auto-Role Assignment
// ============================================================

// AutoRole represents an auto-role rule.
type AutoRole struct {
	ID           string `json:"id"`
	GuildID      string `json:"guild_id"`
	RoleID       string `json:"role_id"`
	RuleType     string `json:"rule_type"`
	DelaySeconds int    `json:"delay_seconds"`
	Enabled      bool   `json:"enabled"`
	CreatedAt    string `json:"created_at"`
	RoleName     string `json:"role_name,omitempty"`
}

// HandleGetAutoRoles returns auto-role rules for a guild.
// GET /api/v1/guilds/{guildID}/auto-roles
func (h *Handler) HandleGetAutoRoles(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")

	if !h.isMember(r.Context(), guildID, userID) {
		apiutil.WriteError(w, http.StatusForbidden, "not_member", "You are not a member of this guild")
		return
	}

	rows, err := h.Pool.Query(r.Context(),
		`SELECT ar.id, ar.guild_id, ar.role_id, ar.rule_type, ar.delay_seconds,
		        ar.enabled, ar.created_at, r.name
		 FROM guild_auto_roles ar
		 JOIN roles r ON r.id = ar.role_id
		 WHERE ar.guild_id = $1
		 ORDER BY ar.created_at ASC`,
		guildID,
	)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to load auto roles", err)
		return
	}
	defer rows.Close()

	rules := make([]AutoRole, 0)
	for rows.Next() {
		var ar AutoRole
		var createdAt time.Time
		if err := rows.Scan(&ar.ID, &ar.GuildID, &ar.RoleID, &ar.RuleType,
			&ar.DelaySeconds, &ar.Enabled, &createdAt, &ar.RoleName); err != nil {
			continue
		}
		ar.CreatedAt = createdAt.Format(time.RFC3339)
		rules = append(rules, ar)
	}

	apiutil.WriteJSON(w, http.StatusOK, rules)
}

type createAutoRoleRequest struct {
	RoleID       string `json:"role_id"`
	RuleType     string `json:"rule_type"`
	DelaySeconds *int   `json:"delay_seconds"`
}

// HandleCreateAutoRole creates an auto-role rule.
// POST /api/v1/guilds/{guildID}/auto-roles
func (h *Handler) HandleCreateAutoRole(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")

	if !h.isGuildAdmin(r.Context(), guildID, userID) {
		apiutil.WriteError(w, http.StatusForbidden, "forbidden", "Only guild admins can manage auto roles")
		return
	}

	var req createAutoRoleRequest
	if !apiutil.DecodeJSON(w, r, &req) {
		return
	}

	if !apiutil.RequireNonEmpty(w, "role_id", req.RoleID) {
		return
	}

	validRuleTypes := map[string]bool{"on_join": true, "after_delay": true, "on_verify": true}
	if !validRuleTypes[req.RuleType] {
		apiutil.WriteError(w, http.StatusBadRequest, "invalid_rule_type", "rule_type must be on_join, after_delay, or on_verify")
		return
	}

	delaySec := 0
	if req.DelaySeconds != nil {
		delaySec = *req.DelaySeconds
	}

	id := models.NewULID().String()
	var ar AutoRole
	var createdAt time.Time
	err := h.Pool.QueryRow(r.Context(),
		`INSERT INTO guild_auto_roles (id, guild_id, role_id, rule_type, delay_seconds, enabled, created_at)
		 VALUES ($1, $2, $3, $4, $5, true, NOW())
		 RETURNING id, guild_id, role_id, rule_type, delay_seconds, enabled, created_at`,
		id, guildID, req.RoleID, req.RuleType, delaySec,
	).Scan(&ar.ID, &ar.GuildID, &ar.RoleID, &ar.RuleType,
		&ar.DelaySeconds, &ar.Enabled, &createdAt)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint") {
			apiutil.WriteError(w, http.StatusConflict, "already_exists", "An auto-role for this role already exists")
			return
		}
		apiutil.InternalError(w, h.Logger, "Failed to create auto role", err)
		return
	}
	ar.CreatedAt = createdAt.Format(time.RFC3339)

	apiutil.WriteJSON(w, http.StatusCreated, ar)
}

type updateAutoRoleRequest struct {
	RuleType     *string `json:"rule_type"`
	DelaySeconds *int    `json:"delay_seconds"`
	Enabled      *bool   `json:"enabled"`
}

// HandleUpdateAutoRole updates an auto-role rule.
// PATCH /api/v1/guilds/{guildID}/auto-roles/{ruleID}
func (h *Handler) HandleUpdateAutoRole(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")
	ruleID := chi.URLParam(r, "ruleID")

	if !h.isGuildAdmin(r.Context(), guildID, userID) {
		apiutil.WriteError(w, http.StatusForbidden, "forbidden", "Only guild admins can manage auto roles")
		return
	}

	var req updateAutoRoleRequest
	if !apiutil.DecodeJSON(w, r, &req) {
		return
	}

	if req.RuleType != nil {
		validRuleTypes := map[string]bool{"on_join": true, "after_delay": true, "on_verify": true}
		if !validRuleTypes[*req.RuleType] {
			apiutil.WriteError(w, http.StatusBadRequest, "invalid_rule_type", "rule_type must be on_join, after_delay, or on_verify")
			return
		}
	}

	var ar AutoRole
	var createdAt time.Time
	err := h.Pool.QueryRow(r.Context(),
		`UPDATE guild_auto_roles SET
		     rule_type = COALESCE($3, rule_type),
		     delay_seconds = COALESCE($4, delay_seconds),
		     enabled = COALESCE($5, enabled)
		 WHERE id = $1 AND guild_id = $2
		 RETURNING id, guild_id, role_id, rule_type, delay_seconds, enabled, created_at`,
		ruleID, guildID, req.RuleType, req.DelaySeconds, req.Enabled,
	).Scan(&ar.ID, &ar.GuildID, &ar.RoleID, &ar.RuleType,
		&ar.DelaySeconds, &ar.Enabled, &createdAt)
	if err == pgx.ErrNoRows {
		apiutil.WriteError(w, http.StatusNotFound, "not_found", "Auto-role rule not found")
		return
	}
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to update auto role", err)
		return
	}
	ar.CreatedAt = createdAt.Format(time.RFC3339)

	apiutil.WriteJSON(w, http.StatusOK, ar)
}

// HandleDeleteAutoRole deletes an auto-role rule.
// DELETE /api/v1/guilds/{guildID}/auto-roles/{ruleID}
func (h *Handler) HandleDeleteAutoRole(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")
	ruleID := chi.URLParam(r, "ruleID")

	if !h.isGuildAdmin(r.Context(), guildID, userID) {
		apiutil.WriteError(w, http.StatusForbidden, "forbidden", "Only guild admins can manage auto roles")
		return
	}

	tag, err := h.Pool.Exec(r.Context(),
		`DELETE FROM guild_auto_roles WHERE id = $1 AND guild_id = $2`,
		ruleID, guildID,
	)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to delete auto role", err)
		return
	}
	if tag.RowsAffected() == 0 {
		apiutil.WriteError(w, http.StatusNotFound, "not_found", "Auto-role rule not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ApplyAutoRoles assigns auto-roles to a user when they join a guild.
// Called by the guild member add handler or NATS subscriber.
func (h *Handler) ApplyAutoRoles(ctx context.Context, guildID, userID string) {
	rows, err := h.Pool.Query(ctx,
		`SELECT role_id, rule_type, delay_seconds
		 FROM guild_auto_roles
		 WHERE guild_id = $1 AND enabled = true
		 ORDER BY created_at ASC`,
		guildID,
	)
	if err != nil {
		h.Logger.Error("failed to query auto roles for assignment",
			slog.String("guild_id", guildID),
			slog.String("error", err.Error()),
		)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var roleID, ruleType string
		var delaySec int
		if rows.Scan(&roleID, &ruleType, &delaySec) != nil {
			continue
		}

		switch ruleType {
		case "on_join":
			h.Pool.Exec(ctx,
				`INSERT INTO member_roles (guild_id, user_id, role_id)
				 VALUES ($1, $2, $3)
				 ON CONFLICT DO NOTHING`,
				guildID, userID, roleID,
			)
		case "after_delay":
			// For delayed assignment, schedule via goroutine.
			// In production this would use a proper job queue.
			go func(gID, uID, rID string, delay int) {
				time.Sleep(time.Duration(delay) * time.Second)
				bgCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				// Verify still a member.
				var exists bool
				h.Pool.QueryRow(bgCtx,
					`SELECT EXISTS(SELECT 1 FROM guild_members WHERE guild_id = $1 AND user_id = $2)`,
					gID, uID,
				).Scan(&exists)
				if exists {
					h.Pool.Exec(bgCtx,
						`INSERT INTO member_roles (guild_id, user_id, role_id)
						 VALUES ($1, $2, $3)
						 ON CONFLICT DO NOTHING`,
						gID, uID, rID,
					)
				}
			}(guildID, userID, roleID, delaySec)
		case "on_verify":
			// Assigned when user passes verification; not handled here.
		}
	}
}
