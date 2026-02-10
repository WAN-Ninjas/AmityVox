package encryption

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/amityvox/amityvox/internal/auth"
	"github.com/amityvox/amityvox/internal/models"
)

// Service implements the MLS Delivery Service. It handles storage and retrieval
// of key packages, welcome messages, group state, and commit messages. The server
// never accesses plaintext or private key material.
type Service struct {
	pool   *pgxpool.Pool
	logger *slog.Logger
}

// Config holds configuration for the encryption service.
type Config struct {
	Pool   *pgxpool.Pool
	Logger *slog.Logger
}

// NewService creates a new MLS delivery service.
func NewService(cfg Config) *Service {
	return &Service{
		pool:   cfg.Pool,
		logger: cfg.Logger,
	}
}

// --- Key Package Handlers ---

// HandleUploadKeyPackage handles POST /api/v1/encryption/key-packages.
// Clients upload MLS key packages so other users can add them to groups.
func (s *Service) HandleUploadKeyPackage(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())

	var req struct {
		DeviceID  string `json:"device_id"`
		Data      []byte `json:"data"`       // Base64-encoded MLS KeyPackage
		ExpiresAt string `json:"expires_at"` // RFC3339 timestamp
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if req.DeviceID == "" || len(req.Data) == 0 {
		writeError(w, http.StatusBadRequest, "missing_fields", "device_id and data are required")
		return
	}

	expiresAt := time.Now().Add(30 * 24 * time.Hour) // default 30 days
	if req.ExpiresAt != "" {
		parsed, err := time.Parse(time.RFC3339, req.ExpiresAt)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_expires", "expires_at must be RFC3339 format")
			return
		}
		expiresAt = parsed
	}

	id := models.NewULID().String()
	_, err := s.pool.Exec(r.Context(),
		`INSERT INTO mls_key_packages (id, user_id, device_id, data, expires_at, created_at)
		 VALUES ($1, $2, $3, $4, $5, now())`,
		id, userID, req.DeviceID, req.Data, expiresAt,
	)
	if err != nil {
		s.logger.Error("failed to store key package", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to store key package")
		return
	}

	writeJSON(w, http.StatusCreated, KeyPackage{
		ID:        id,
		UserID:    userID,
		DeviceID:  req.DeviceID,
		Data:      req.Data,
		ExpiresAt: expiresAt,
		CreatedAt: time.Now().UTC(),
	})
}

// HandleGetKeyPackages handles GET /api/v1/encryption/key-packages/{userID}.
// Returns available key packages for a user so others can establish encrypted sessions.
func (s *Service) HandleGetKeyPackages(w http.ResponseWriter, r *http.Request) {
	targetUserID := chi.URLParam(r, "userID")

	rows, err := s.pool.Query(r.Context(),
		`SELECT id, user_id, device_id, data, expires_at, created_at
		 FROM mls_key_packages
		 WHERE user_id = $1 AND expires_at > now()
		 ORDER BY created_at DESC`,
		targetUserID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to query key packages")
		return
	}
	defer rows.Close()

	var packages []KeyPackage
	for rows.Next() {
		var kp KeyPackage
		if err := rows.Scan(&kp.ID, &kp.UserID, &kp.DeviceID, &kp.Data, &kp.ExpiresAt, &kp.CreatedAt); err != nil {
			continue
		}
		packages = append(packages, kp)
	}

	if packages == nil {
		packages = []KeyPackage{}
	}
	writeJSON(w, http.StatusOK, packages)
}

// HandleClaimKeyPackage handles POST /api/v1/encryption/key-packages/{userID}/claim.
// Claims (consumes) one key package for the target user. This is used when adding
// a user to an encrypted group — the key package is consumed so it can't be reused.
func (s *Service) HandleClaimKeyPackage(w http.ResponseWriter, r *http.Request) {
	targetUserID := chi.URLParam(r, "userID")

	// Claim one non-expired key package via DELETE RETURNING.
	var kp KeyPackage
	err := s.pool.QueryRow(r.Context(),
		`DELETE FROM mls_key_packages
		 WHERE id = (
			 SELECT id FROM mls_key_packages
			 WHERE user_id = $1 AND expires_at > now()
			 ORDER BY created_at ASC LIMIT 1
		 )
		 RETURNING id, user_id, device_id, data, expires_at, created_at`,
		targetUserID,
	).Scan(&kp.ID, &kp.UserID, &kp.DeviceID, &kp.Data, &kp.ExpiresAt, &kp.CreatedAt)

	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "no_key_packages", "No available key packages for this user")
		return
	}
	if err != nil {
		s.logger.Error("failed to claim key package", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to claim key package")
		return
	}

	writeJSON(w, http.StatusOK, kp)
}

// HandleDeleteKeyPackage handles DELETE /api/v1/encryption/key-packages/{packageID}.
// Users can delete their own key packages.
func (s *Service) HandleDeleteKeyPackage(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	packageID := chi.URLParam(r, "packageID")

	result, err := s.pool.Exec(r.Context(),
		`DELETE FROM mls_key_packages WHERE id = $1 AND user_id = $2`,
		packageID, userID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to delete key package")
		return
	}

	if result.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "not_found", "Key package not found or not owned by you")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// --- Welcome Message Handlers ---

// HandleSendWelcome handles POST /api/v1/encryption/channels/{channelID}/welcome.
// Sends an MLS Welcome message to a user being added to an encrypted channel.
func (s *Service) HandleSendWelcome(w http.ResponseWriter, r *http.Request) {
	channelID := chi.URLParam(r, "channelID")

	var req struct {
		ReceiverID string `json:"receiver_id"`
		Data       []byte `json:"data"` // Opaque MLS Welcome bytes
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if req.ReceiverID == "" || len(req.Data) == 0 {
		writeError(w, http.StatusBadRequest, "missing_fields", "receiver_id and data are required")
		return
	}

	id := models.NewULID().String()
	_, err := s.pool.Exec(r.Context(),
		`INSERT INTO mls_welcome_messages (id, channel_id, receiver_id, data, created_at)
		 VALUES ($1, $2, $3, $4, now())`,
		id, channelID, req.ReceiverID, req.Data,
	)
	if err != nil {
		s.logger.Error("failed to store welcome message", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to store welcome message")
		return
	}

	writeJSON(w, http.StatusCreated, WelcomeMessage{
		ID:         id,
		ChannelID:  channelID,
		ReceiverID: req.ReceiverID,
		Data:       req.Data,
		CreatedAt:  time.Now().UTC(),
	})
}

// HandleGetWelcomes handles GET /api/v1/encryption/welcome.
// Returns pending MLS Welcome messages for the authenticated user.
func (s *Service) HandleGetWelcomes(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())

	rows, err := s.pool.Query(r.Context(),
		`SELECT id, channel_id, receiver_id, data, created_at
		 FROM mls_welcome_messages
		 WHERE receiver_id = $1
		 ORDER BY created_at ASC`,
		userID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to query welcome messages")
		return
	}
	defer rows.Close()

	var messages []WelcomeMessage
	for rows.Next() {
		var wm WelcomeMessage
		if err := rows.Scan(&wm.ID, &wm.ChannelID, &wm.ReceiverID, &wm.Data, &wm.CreatedAt); err != nil {
			continue
		}
		messages = append(messages, wm)
	}

	if messages == nil {
		messages = []WelcomeMessage{}
	}
	writeJSON(w, http.StatusOK, messages)
}

// HandleAckWelcome handles DELETE /api/v1/encryption/welcome/{welcomeID}.
// Acknowledges and removes a welcome message after the client has processed it.
func (s *Service) HandleAckWelcome(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	welcomeID := chi.URLParam(r, "welcomeID")

	result, err := s.pool.Exec(r.Context(),
		`DELETE FROM mls_welcome_messages WHERE id = $1 AND receiver_id = $2`,
		welcomeID, userID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to acknowledge welcome")
		return
	}

	if result.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "not_found", "Welcome message not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// --- Group State Handlers ---

// HandleGetGroupState handles GET /api/v1/encryption/channels/{channelID}/group-state.
// Returns the current MLS group state for an encrypted channel.
func (s *Service) HandleGetGroupState(w http.ResponseWriter, r *http.Request) {
	channelID := chi.URLParam(r, "channelID")

	var gs GroupState
	err := s.pool.QueryRow(r.Context(),
		`SELECT channel_id, epoch, tree_hash, updated_at
		 FROM mls_group_states WHERE channel_id = $1`,
		channelID,
	).Scan(&gs.ChannelID, &gs.Epoch, &gs.TreeHash, &gs.UpdatedAt)

	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "no_group", "No MLS group state for this channel")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to query group state")
		return
	}

	writeJSON(w, http.StatusOK, gs)
}

// HandleUpdateGroupState handles PUT /api/v1/encryption/channels/{channelID}/group-state.
// Updates the MLS group state (epoch + tree hash) for an encrypted channel.
func (s *Service) HandleUpdateGroupState(w http.ResponseWriter, r *http.Request) {
	channelID := chi.URLParam(r, "channelID")

	var req struct {
		Epoch    uint64 `json:"epoch"`
		TreeHash []byte `json:"tree_hash"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	_, err := s.pool.Exec(r.Context(),
		`INSERT INTO mls_group_states (channel_id, epoch, tree_hash, updated_at)
		 VALUES ($1, $2, $3, now())
		 ON CONFLICT (channel_id) DO UPDATE SET
			epoch = EXCLUDED.epoch,
			tree_hash = EXCLUDED.tree_hash,
			updated_at = now()`,
		channelID, req.Epoch, req.TreeHash,
	)
	if err != nil {
		s.logger.Error("failed to update group state", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to update group state")
		return
	}

	writeJSON(w, http.StatusOK, GroupState{
		ChannelID: channelID,
		Epoch:     req.Epoch,
		TreeHash:  req.TreeHash,
		UpdatedAt: time.Now().UTC(),
	})
}

// --- Commit Handlers ---

// HandlePublishCommit handles POST /api/v1/encryption/channels/{channelID}/commits.
// Publishes an MLS Commit message that advances the group state.
func (s *Service) HandlePublishCommit(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")

	var req struct {
		Epoch uint64 `json:"epoch"`
		Data  []byte `json:"data"` // Opaque MLS Commit bytes
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if len(req.Data) == 0 {
		writeError(w, http.StatusBadRequest, "missing_data", "Commit data is required")
		return
	}

	id := models.NewULID().String()
	_, err := s.pool.Exec(r.Context(),
		`INSERT INTO mls_commits (id, channel_id, sender_id, epoch, data, created_at)
		 VALUES ($1, $2, $3, $4, $5, now())`,
		id, channelID, userID, req.Epoch, req.Data,
	)
	if err != nil {
		s.logger.Error("failed to store commit", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to store commit")
		return
	}

	writeJSON(w, http.StatusCreated, Commit{
		ID:        id,
		ChannelID: channelID,
		SenderID:  userID,
		Epoch:     req.Epoch,
		Data:      req.Data,
		CreatedAt: time.Now().UTC(),
	})
}

// HandleGetCommits handles GET /api/v1/encryption/channels/{channelID}/commits.
// Returns MLS Commit messages for a channel, optionally filtered by epoch.
func (s *Service) HandleGetCommits(w http.ResponseWriter, r *http.Request) {
	channelID := chi.URLParam(r, "channelID")

	sinceEpochStr := r.URL.Query().Get("since_epoch")
	sinceEpoch := int64(0)
	if sinceEpochStr != "" {
		fmt.Sscanf(sinceEpochStr, "%d", &sinceEpoch)
	}

	rows, err := s.pool.Query(r.Context(),
		`SELECT id, channel_id, sender_id, epoch, data, created_at
		 FROM mls_commits
		 WHERE channel_id = $1 AND epoch >= $2
		 ORDER BY epoch ASC, created_at ASC
		 LIMIT 100`,
		channelID, sinceEpoch,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to query commits")
		return
	}
	defer rows.Close()

	var commits []Commit
	for rows.Next() {
		var c Commit
		if err := rows.Scan(&c.ID, &c.ChannelID, &c.SenderID, &c.Epoch, &c.Data, &c.CreatedAt); err != nil {
			continue
		}
		commits = append(commits, c)
	}

	if commits == nil {
		commits = []Commit{}
	}
	writeJSON(w, http.StatusOK, commits)
}

// --- Helpers ---

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]interface{}{"data": data})
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"error": map[string]string{"code": code, "message": message},
	})
}
