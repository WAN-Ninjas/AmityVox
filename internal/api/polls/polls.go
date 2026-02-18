// Package polls implements REST API handlers for poll operations including
// creating polls, voting, retrieving results, closing, and deleting polls.
// Mounted under /api/v1/channels/{channelID}/polls.
package polls

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/amityvox/amityvox/internal/api/apiutil"
	"github.com/amityvox/amityvox/internal/auth"
	"github.com/amityvox/amityvox/internal/events"
	"github.com/amityvox/amityvox/internal/models"
)

// Handler implements poll-related REST API endpoints.
type Handler struct {
	Pool     *pgxpool.Pool
	EventBus *events.Bus
	Logger   *slog.Logger
}

type createPollRequest struct {
	Question  string   `json:"question"`
	Options   []string `json:"options"`
	MultiVote bool     `json:"multi_vote"`
	Anonymous bool     `json:"anonymous"`
	Duration  int      `json:"duration"` // seconds, 0 = no expiry
}

type votePollRequest struct {
	OptionIDs []string `json:"option_ids"`
}

// HandleCreatePoll creates a new poll in a channel.
// POST /api/v1/channels/{channelID}/polls
func (h *Handler) HandleCreatePoll(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")

	var req createPollRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apiutil.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	// Validate question.
	if req.Question == "" {
		apiutil.WriteError(w, http.StatusBadRequest, "missing_question", "Question is required")
		return
	}
	if utf8.RuneCountInString(req.Question) > 300 {
		apiutil.WriteError(w, http.StatusBadRequest, "question_too_long", "Question must be at most 300 characters")
		return
	}

	// Validate options.
	if len(req.Options) < 2 {
		apiutil.WriteError(w, http.StatusBadRequest, "too_few_options", "Poll must have at least 2 options")
		return
	}
	if len(req.Options) > 10 {
		apiutil.WriteError(w, http.StatusBadRequest, "too_many_options", "Poll must have at most 10 options")
		return
	}
	for _, opt := range req.Options {
		if opt == "" {
			apiutil.WriteError(w, http.StatusBadRequest, "empty_option", "Poll options must not be empty")
			return
		}
		if utf8.RuneCountInString(opt) > 100 {
			apiutil.WriteError(w, http.StatusBadRequest, "option_too_long", "Each poll option must be at most 100 characters")
			return
		}
	}

	pollID := models.NewULID().String()

	var expiresAt *time.Time
	if req.Duration > 0 {
		t := time.Now().Add(time.Duration(req.Duration) * time.Second)
		expiresAt = &t
	}

	tx, err := h.Pool.Begin(r.Context())
	if err != nil {
		h.Logger.Error("failed to begin transaction", slog.String("error", err.Error()))
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to create poll")
		return
	}
	defer tx.Rollback(r.Context())

	// Insert the poll.
	var poll models.Poll
	err = tx.QueryRow(r.Context(),
		`INSERT INTO polls (id, channel_id, author_id, question, multi_vote, anonymous, expires_at, closed, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, false, now())
		 RETURNING id, channel_id, message_id, author_id, question, multi_vote, anonymous, expires_at, closed, created_at`,
		pollID, channelID, userID, req.Question, req.MultiVote, req.Anonymous, expiresAt,
	).Scan(
		&poll.ID, &poll.ChannelID, &poll.MessageID, &poll.AuthorID,
		&poll.Question, &poll.MultiVote, &poll.Anonymous, &poll.ExpiresAt,
		&poll.Closed, &poll.CreatedAt,
	)
	if err != nil {
		h.Logger.Error("failed to insert poll", slog.String("error", err.Error()))
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to create poll")
		return
	}

	// Insert poll options.
	options := make([]models.PollOption, 0, len(req.Options))
	for i, text := range req.Options {
		optionID := models.NewULID().String()
		var opt models.PollOption
		err = tx.QueryRow(r.Context(),
			`INSERT INTO poll_options (id, poll_id, text, position, vote_count)
			 VALUES ($1, $2, $3, $4, 0)
			 RETURNING id, poll_id, text, position, vote_count`,
			optionID, pollID, text, i,
		).Scan(&opt.ID, &opt.PollID, &opt.Text, &opt.Position, &opt.VoteCount)
		if err != nil {
			h.Logger.Error("failed to insert poll option", slog.String("error", err.Error()))
			apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to create poll")
			return
		}
		options = append(options, opt)
	}

	if err := tx.Commit(r.Context()); err != nil {
		h.Logger.Error("failed to commit transaction", slog.String("error", err.Error()))
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to create poll")
		return
	}

	poll.Options = options
	poll.TotalVotes = 0
	poll.UserVotes = []string{}

	h.EventBus.PublishChannelEvent(r.Context(), events.SubjectPollCreate, "POLL_CREATE", channelID, poll)

	apiutil.WriteJSON(w, http.StatusCreated, poll)
}

// HandleGetPoll returns a poll by ID with options and vote counts.
// GET /api/v1/channels/{channelID}/polls/{pollID}
func (h *Handler) HandleGetPoll(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	pollID := chi.URLParam(r, "pollID")

	var poll models.Poll
	err := h.Pool.QueryRow(r.Context(),
		`SELECT id, channel_id, message_id, author_id, question, multi_vote, anonymous, expires_at, closed, created_at
		 FROM polls WHERE id = $1`,
		pollID,
	).Scan(
		&poll.ID, &poll.ChannelID, &poll.MessageID, &poll.AuthorID,
		&poll.Question, &poll.MultiVote, &poll.Anonymous, &poll.ExpiresAt,
		&poll.Closed, &poll.CreatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			apiutil.WriteError(w, http.StatusNotFound, "poll_not_found", "Poll not found")
			return
		}
		h.Logger.Error("failed to get poll", slog.String("error", err.Error()))
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to get poll")
		return
	}

	// Load options with vote counts.
	rows, err := h.Pool.Query(r.Context(),
		`SELECT id, poll_id, text, position, vote_count
		 FROM poll_options WHERE poll_id = $1
		 ORDER BY position ASC`,
		pollID,
	)
	if err != nil {
		h.Logger.Error("failed to get poll options", slog.String("error", err.Error()))
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to get poll options")
		return
	}
	defer rows.Close()

	options := make([]models.PollOption, 0)
	totalVotes := 0
	for rows.Next() {
		var opt models.PollOption
		if err := rows.Scan(&opt.ID, &opt.PollID, &opt.Text, &opt.Position, &opt.VoteCount); err != nil {
			h.Logger.Error("failed to scan poll option", slog.String("error", err.Error()))
			apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to read poll options")
			return
		}
		totalVotes += opt.VoteCount
		options = append(options, opt)
	}

	poll.Options = options
	poll.TotalVotes = totalVotes

	// Load the requesting user's votes.
	voteRows, err := h.Pool.Query(r.Context(),
		`SELECT option_id FROM poll_votes WHERE poll_id = $1 AND user_id = $2`,
		pollID, userID,
	)
	if err != nil {
		h.Logger.Error("failed to get user votes", slog.String("error", err.Error()))
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to get user votes")
		return
	}
	defer voteRows.Close()

	userVotes := make([]string, 0)
	for voteRows.Next() {
		var optionID string
		if err := voteRows.Scan(&optionID); err != nil {
			h.Logger.Error("failed to scan user vote", slog.String("error", err.Error()))
			apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to read user votes")
			return
		}
		userVotes = append(userVotes, optionID)
	}
	poll.UserVotes = userVotes

	apiutil.WriteJSON(w, http.StatusOK, poll)
}

// HandleVotePoll casts a vote on a poll.
// POST /api/v1/channels/{channelID}/polls/{pollID}/votes
func (h *Handler) HandleVotePoll(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	pollID := chi.URLParam(r, "pollID")

	var req votePollRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apiutil.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if len(req.OptionIDs) == 0 {
		apiutil.WriteError(w, http.StatusBadRequest, "no_options", "At least one option ID is required")
		return
	}

	// Fetch the poll to check constraints.
	var multiVote, closed bool
	var expiresAt *time.Time
	var channelID string
	err := h.Pool.QueryRow(r.Context(),
		`SELECT channel_id, multi_vote, closed, expires_at FROM polls WHERE id = $1`,
		pollID,
	).Scan(&channelID, &multiVote, &closed, &expiresAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			apiutil.WriteError(w, http.StatusNotFound, "poll_not_found", "Poll not found")
			return
		}
		h.Logger.Error("failed to get poll", slog.String("error", err.Error()))
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to get poll")
		return
	}

	// Check if poll is closed.
	if closed {
		apiutil.WriteError(w, http.StatusBadRequest, "poll_closed", "This poll is closed")
		return
	}

	// Check if poll has expired.
	if expiresAt != nil && expiresAt.Before(time.Now()) {
		apiutil.WriteError(w, http.StatusBadRequest, "poll_expired", "This poll has expired")
		return
	}

	// If not multi_vote, only allow one option.
	if !multiVote && len(req.OptionIDs) > 1 {
		apiutil.WriteError(w, http.StatusBadRequest, "single_vote_only", "This poll only allows voting for one option")
		return
	}

	// Verify all option IDs belong to this poll.
	var validCount int
	err = h.Pool.QueryRow(r.Context(),
		`SELECT COUNT(*) FROM poll_options WHERE poll_id = $1 AND id = ANY($2)`,
		pollID, req.OptionIDs,
	).Scan(&validCount)
	if err != nil {
		h.Logger.Error("failed to validate option IDs", slog.String("error", err.Error()))
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to validate options")
		return
	}
	if validCount != len(req.OptionIDs) {
		apiutil.WriteError(w, http.StatusBadRequest, "invalid_option", "One or more option IDs are invalid")
		return
	}

	tx, err := h.Pool.Begin(r.Context())
	if err != nil {
		h.Logger.Error("failed to begin transaction", slog.String("error", err.Error()))
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to cast vote")
		return
	}
	defer tx.Rollback(r.Context())

	// Remove any existing votes by this user on this poll and decrement counts.
	existingRows, err := tx.Query(r.Context(),
		`SELECT option_id FROM poll_votes WHERE poll_id = $1 AND user_id = $2`,
		pollID, userID,
	)
	if err != nil {
		h.Logger.Error("failed to get existing votes", slog.String("error", err.Error()))
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to cast vote")
		return
	}

	var existingOptionIDs []string
	for existingRows.Next() {
		var optID string
		if err := existingRows.Scan(&optID); err != nil {
			existingRows.Close()
			h.Logger.Error("failed to scan existing vote", slog.String("error", err.Error()))
			apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to cast vote")
			return
		}
		existingOptionIDs = append(existingOptionIDs, optID)
	}
	existingRows.Close()

	// Decrement vote counts for previously voted options.
	if len(existingOptionIDs) > 0 {
		_, err = tx.Exec(r.Context(),
			`UPDATE poll_options SET vote_count = vote_count - 1
			 WHERE poll_id = $1 AND id = ANY($2)`,
			pollID, existingOptionIDs,
		)
		if err != nil {
			h.Logger.Error("failed to decrement vote counts", slog.String("error", err.Error()))
			apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to cast vote")
			return
		}

		// Remove existing votes.
		_, err = tx.Exec(r.Context(),
			`DELETE FROM poll_votes WHERE poll_id = $1 AND user_id = $2`,
			pollID, userID,
		)
		if err != nil {
			h.Logger.Error("failed to remove existing votes", slog.String("error", err.Error()))
			apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to cast vote")
			return
		}
	}

	// Insert new votes and increment counts.
	for _, optionID := range req.OptionIDs {
		_, err = tx.Exec(r.Context(),
			`INSERT INTO poll_votes (poll_id, option_id, user_id, created_at)
			 VALUES ($1, $2, $3, now())`,
			pollID, optionID, userID,
		)
		if err != nil {
			h.Logger.Error("failed to insert vote", slog.String("error", err.Error()))
			apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to cast vote")
			return
		}

		_, err = tx.Exec(r.Context(),
			`UPDATE poll_options SET vote_count = vote_count + 1
			 WHERE poll_id = $1 AND id = $2`,
			pollID, optionID,
		)
		if err != nil {
			h.Logger.Error("failed to increment vote count", slog.String("error", err.Error()))
			apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to cast vote")
			return
		}
	}

	if err := tx.Commit(r.Context()); err != nil {
		h.Logger.Error("failed to commit transaction", slog.String("error", err.Error()))
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to cast vote")
		return
	}

	h.EventBus.PublishChannelEvent(r.Context(), events.SubjectPollVote, "POLL_VOTE", channelID, map[string]interface{}{
		"poll_id":    pollID,
		"channel_id": channelID,
		"user_id":    userID,
		"option_ids": req.OptionIDs,
	})

	apiutil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"poll_id":    pollID,
		"option_ids": req.OptionIDs,
	})
}

// HandleClosePoll closes a poll so no more votes can be cast.
// POST /api/v1/channels/{channelID}/polls/{pollID}/close
func (h *Handler) HandleClosePoll(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	pollID := chi.URLParam(r, "pollID")

	// Fetch the poll to check ownership.
	var authorID, channelID string
	var closed bool
	err := h.Pool.QueryRow(r.Context(),
		`SELECT author_id, channel_id, closed FROM polls WHERE id = $1`,
		pollID,
	).Scan(&authorID, &channelID, &closed)
	if err != nil {
		if err == pgx.ErrNoRows {
			apiutil.WriteError(w, http.StatusNotFound, "poll_not_found", "Poll not found")
			return
		}
		h.Logger.Error("failed to get poll", slog.String("error", err.Error()))
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to get poll")
		return
	}

	if closed {
		apiutil.WriteError(w, http.StatusBadRequest, "already_closed", "This poll is already closed")
		return
	}

	// Check authorization: only the author or an admin can close a poll.
	if authorID != userID {
		var userFlags int
		h.Pool.QueryRow(r.Context(), `SELECT flags FROM users WHERE id = $1`, userID).Scan(&userFlags)
		if userFlags&models.UserFlagAdmin == 0 {
			apiutil.WriteError(w, http.StatusForbidden, "forbidden", "Only the poll author or an admin can close this poll")
			return
		}
	}

	_, err = h.Pool.Exec(r.Context(),
		`UPDATE polls SET closed = true WHERE id = $1`, pollID)
	if err != nil {
		h.Logger.Error("failed to close poll", slog.String("error", err.Error()))
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to close poll")
		return
	}

	h.EventBus.PublishChannelEvent(r.Context(), events.SubjectPollClose, "POLL_CLOSE", channelID, map[string]string{
		"poll_id":    pollID,
		"channel_id": channelID,
		"closed_by":  userID,
	})

	apiutil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"poll_id": pollID,
		"closed":  true,
	})
}

// HandleDeletePoll deletes a poll entirely.
// DELETE /api/v1/channels/{channelID}/polls/{pollID}
func (h *Handler) HandleDeletePoll(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	pollID := chi.URLParam(r, "pollID")

	// Fetch the poll to check ownership.
	var authorID string
	err := h.Pool.QueryRow(r.Context(),
		`SELECT author_id FROM polls WHERE id = $1`,
		pollID,
	).Scan(&authorID)
	if err != nil {
		if err == pgx.ErrNoRows {
			apiutil.WriteError(w, http.StatusNotFound, "poll_not_found", "Poll not found")
			return
		}
		h.Logger.Error("failed to get poll", slog.String("error", err.Error()))
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to get poll")
		return
	}

	// Check authorization: only the author or an admin can delete a poll.
	if authorID != userID {
		var userFlags int
		h.Pool.QueryRow(r.Context(), `SELECT flags FROM users WHERE id = $1`, userID).Scan(&userFlags)
		if userFlags&models.UserFlagAdmin == 0 {
			apiutil.WriteError(w, http.StatusForbidden, "forbidden", "Only the poll author or an admin can delete this poll")
			return
		}
	}

	tag, err := h.Pool.Exec(r.Context(), `DELETE FROM polls WHERE id = $1`, pollID)
	if err != nil {
		h.Logger.Error("failed to delete poll", slog.String("error", err.Error()))
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to delete poll")
		return
	}
	if tag.RowsAffected() == 0 {
		apiutil.WriteError(w, http.StatusNotFound, "poll_not_found", "Poll not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
