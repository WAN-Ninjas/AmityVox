// Package auth implements authentication for AmityVox, including user
// registration with Argon2id password hashing, login with session creation,
// logout, session validation, and the authentication middleware that extracts
// user identity from Bearer tokens. TOTP 2FA and WebAuthn will be added in
// a later phase but the session infrastructure supports them.
package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/alexedwards/argon2id"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/amityvox/amityvox/internal/models"
	"github.com/amityvox/amityvox/internal/presence"
)

var usernameRegex = regexp.MustCompile(`^[a-zA-Z0-9_.-]{2,32}$`)

// Service provides authentication operations against PostgreSQL and the cache.
type Service struct {
	pool            *pgxpool.Pool
	cache           *presence.Cache
	instanceID      string
	sessionDuration time.Duration
	regEnabled      bool
	inviteOnly      bool
	requireEmail    bool
	logger          *slog.Logger
}

// Config holds the parameters needed to create an auth Service.
type Config struct {
	Pool            *pgxpool.Pool
	Cache           *presence.Cache
	InstanceID      string
	SessionDuration time.Duration
	RegEnabled      bool
	InviteOnly      bool
	RequireEmail    bool
	Logger          *slog.Logger
}

// NewService creates a new authentication service.
func NewService(cfg Config) *Service {
	return &Service{
		pool:            cfg.Pool,
		cache:           cfg.Cache,
		instanceID:      cfg.InstanceID,
		sessionDuration: cfg.SessionDuration,
		regEnabled:      cfg.RegEnabled,
		inviteOnly:      cfg.InviteOnly,
		requireEmail:    cfg.RequireEmail,
		logger:          cfg.Logger,
	}
}

// RegisterRequest is the request body for user registration.
type RegisterRequest struct {
	Username string  `json:"username"`
	Password string  `json:"password"`
	Email    *string `json:"email,omitempty"`
}

// LoginRequest is the request body for user login.
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// AuthError represents an authentication-related error with an HTTP-friendly code.
type AuthError struct {
	Code    string
	Message string
	Status  int
}

func (e *AuthError) Error() string {
	return e.Message
}

// Register creates a new user account and returns the created user and a session.
func (s *Service) Register(ctx context.Context, req RegisterRequest, ip, userAgent string) (*models.User, *models.UserSession, error) {
	if !s.regEnabled {
		return nil, nil, &AuthError{Code: "registration_disabled", Message: "Registration is disabled on this instance", Status: 403}
	}

	if err := validateUsername(req.Username); err != nil {
		return nil, nil, err
	}

	if err := validatePassword(req.Password); err != nil {
		return nil, nil, err
	}

	if s.requireEmail && (req.Email == nil || *req.Email == "") {
		return nil, nil, &AuthError{Code: "email_required", Message: "Email is required for registration", Status: 400}
	}

	hash, err := argon2id.CreateHash(req.Password, argon2id.DefaultParams)
	if err != nil {
		return nil, nil, fmt.Errorf("hashing password: %w", err)
	}

	userID := models.NewULID().String()

	var user models.User
	err = s.pool.QueryRow(ctx,
		`INSERT INTO users (id, instance_id, username, password_hash, email, status_presence, created_at)
		 VALUES ($1, $2, $3, $4, $5, 'offline', now())
		 RETURNING id, instance_id, username, display_name, avatar_id, status_text,
		           status_presence, bio, bot_owner_id, email, flags, created_at`,
		userID, s.instanceID, req.Username, hash, req.Email,
	).Scan(
		&user.ID, &user.InstanceID, &user.Username, &user.DisplayName,
		&user.AvatarID, &user.StatusText, &user.StatusPresence, &user.Bio,
		&user.BotOwnerID, &user.Email, &user.Flags, &user.CreatedAt,
	)
	if err != nil {
		if strings.Contains(err.Error(), "unique constraint") || strings.Contains(err.Error(), "duplicate key") {
			return nil, nil, &AuthError{Code: "username_taken", Message: "Username is already taken", Status: 409}
		}
		return nil, nil, fmt.Errorf("inserting user: %w", err)
	}

	session, err := s.createSession(ctx, user.ID, ip, userAgent)
	if err != nil {
		return nil, nil, err
	}

	s.logger.Info("user registered",
		slog.String("user_id", user.ID),
		slog.String("username", user.Username),
	)

	return &user, session, nil
}

// Login authenticates a user by username and password and creates a new session.
func (s *Service) Login(ctx context.Context, req LoginRequest, ip, userAgent string) (*models.User, *models.UserSession, error) {
	if err := validateUsername(req.Username); err != nil {
		return nil, nil, err
	}

	var user models.User
	var passwordHash *string
	err := s.pool.QueryRow(ctx,
		`SELECT id, instance_id, username, display_name, avatar_id, status_text,
		        status_presence, bio, bot_owner_id, password_hash, email, flags, created_at
		 FROM users
		 WHERE username = $1 AND instance_id = $2`,
		req.Username, s.instanceID,
	).Scan(
		&user.ID, &user.InstanceID, &user.Username, &user.DisplayName,
		&user.AvatarID, &user.StatusText, &user.StatusPresence, &user.Bio,
		&user.BotOwnerID, &passwordHash, &user.Email, &user.Flags, &user.CreatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil, &AuthError{Code: "invalid_credentials", Message: "Invalid username or password", Status: 401}
	}
	if err != nil {
		return nil, nil, fmt.Errorf("querying user: %w", err)
	}

	if user.IsSuspended() {
		return nil, nil, &AuthError{Code: "user_suspended", Message: "This account has been suspended", Status: 403}
	}

	if user.IsDeleted() {
		return nil, nil, &AuthError{Code: "invalid_credentials", Message: "Invalid username or password", Status: 401}
	}

	if passwordHash == nil {
		return nil, nil, &AuthError{Code: "invalid_credentials", Message: "Invalid username or password", Status: 401}
	}

	match, err := argon2id.ComparePasswordAndHash(req.Password, *passwordHash)
	if err != nil {
		return nil, nil, fmt.Errorf("comparing password hash: %w", err)
	}
	if !match {
		return nil, nil, &AuthError{Code: "invalid_credentials", Message: "Invalid username or password", Status: 401}
	}

	session, err := s.createSession(ctx, user.ID, ip, userAgent)
	if err != nil {
		return nil, nil, err
	}

	s.logger.Info("user logged in",
		slog.String("user_id", user.ID),
		slog.String("username", user.Username),
	)

	return &user, session, nil
}

// Logout invalidates a session by removing it from the database and cache.
func (s *Service) Logout(ctx context.Context, sessionID string) error {
	_, err := s.pool.Exec(ctx, "DELETE FROM user_sessions WHERE id = $1", sessionID)
	if err != nil {
		return fmt.Errorf("deleting session: %w", err)
	}

	if err := s.cache.DeleteSession(ctx, sessionID); err != nil {
		s.logger.Warn("failed to delete cached session", slog.String("error", err.Error()))
	}

	return nil
}

// ValidateSession checks if a session token is valid and returns the associated
// user ID. It checks the cache first, falling back to the database.
func (s *Service) ValidateSession(ctx context.Context, sessionID string) (string, error) {
	// Check cache first.
	cached, err := s.cache.GetSession(ctx, sessionID)
	if err != nil {
		s.logger.Debug("cache lookup failed, falling back to database",
			slog.String("error", err.Error()),
		)
	}

	if cached != nil {
		if time.Now().After(cached.ExpiresAt) {
			s.cache.DeleteSession(ctx, sessionID)
			return "", &AuthError{Code: "session_expired", Message: "Session has expired", Status: 401}
		}
		return cached.UserID, nil
	}

	// Fall back to database.
	var userID string
	var expiresAt time.Time
	err = s.pool.QueryRow(ctx,
		`SELECT user_id, expires_at FROM user_sessions WHERE id = $1`,
		sessionID,
	).Scan(&userID, &expiresAt)
	if err == pgx.ErrNoRows {
		return "", &AuthError{Code: "invalid_session", Message: "Invalid or expired session", Status: 401}
	}
	if err != nil {
		return "", fmt.Errorf("querying session: %w", err)
	}

	if time.Now().After(expiresAt) {
		s.pool.Exec(ctx, "DELETE FROM user_sessions WHERE id = $1", sessionID)
		return "", &AuthError{Code: "session_expired", Message: "Session has expired", Status: 401}
	}

	// Cache the session for future lookups.
	s.cache.SetSession(ctx, sessionID, presence.SessionData{
		UserID:    userID,
		ExpiresAt: expiresAt,
	})

	// Update last_active_at asynchronously.
	go func() {
		bgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		s.pool.Exec(bgCtx, "UPDATE user_sessions SET last_active_at = now() WHERE id = $1", sessionID)
	}()

	return userID, nil
}

// GetUser retrieves a user by ID from the database.
func (s *Service) GetUser(ctx context.Context, userID string) (*models.User, error) {
	var user models.User
	err := s.pool.QueryRow(ctx,
		`SELECT id, instance_id, username, display_name, avatar_id, status_text,
		        status_presence, bio, bot_owner_id, email, flags, created_at
		 FROM users WHERE id = $1`,
		userID,
	).Scan(
		&user.ID, &user.InstanceID, &user.Username, &user.DisplayName,
		&user.AvatarID, &user.StatusText, &user.StatusPresence, &user.Bio,
		&user.BotOwnerID, &user.Email, &user.Flags, &user.CreatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, &AuthError{Code: "user_not_found", Message: "User not found", Status: 404}
	}
	if err != nil {
		return nil, fmt.Errorf("querying user %s: %w", userID, err)
	}
	return &user, nil
}

// createSession generates a secure session token and stores it in the database
// and cache.
func (s *Service) createSession(ctx context.Context, userID, ip, userAgent string) (*models.UserSession, error) {
	token, err := generateSessionToken()
	if err != nil {
		return nil, fmt.Errorf("generating session token: %w", err)
	}

	expiresAt := time.Now().Add(s.sessionDuration)

	var ipStr *string
	if ip != "" {
		// Strip port if present (r.RemoteAddr is "host:port").
		host, _, err := net.SplitHostPort(ip)
		if err != nil {
			// No port — use as-is (e.g. X-Forwarded-For).
			host = ip
		}
		if parsed := net.ParseIP(host); parsed != nil {
			s := parsed.String()
			ipStr = &s
		}
	}

	var session models.UserSession
	err = s.pool.QueryRow(ctx,
		`INSERT INTO user_sessions (id, user_id, ip_address, user_agent, created_at, last_active_at, expires_at)
		 VALUES ($1, $2, $3, $4, now(), now(), $5)
		 RETURNING id, user_id, device_name, user_agent, created_at, last_active_at, expires_at`,
		token, userID, ipStr, userAgent, expiresAt,
	).Scan(
		&session.ID, &session.UserID, &session.DeviceName,
		&session.UserAgent, &session.CreatedAt, &session.LastActiveAt, &session.ExpiresAt,
	)
	if err != nil {
		return nil, fmt.Errorf("inserting session: %w", err)
	}

	// Cache the session for fast lookups.
	s.cache.SetSession(ctx, token, presence.SessionData{
		UserID:    userID,
		ExpiresAt: expiresAt,
	})

	return &session, nil
}

// generateSessionToken creates a cryptographically random 32-byte hex-encoded token.
func generateSessionToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("reading random bytes: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func validateUsername(username string) *AuthError {
	if !usernameRegex.MatchString(username) {
		return &AuthError{
			Code:    "invalid_username",
			Message: "Username must be 2-32 characters and contain only letters, numbers, underscores, hyphens, and dots",
			Status:  400,
		}
	}
	return nil
}

func validatePassword(password string) *AuthError {
	length := utf8.RuneCountInString(password)
	if length < 8 {
		return &AuthError{
			Code:    "password_too_short",
			Message: "Password must be at least 8 characters",
			Status:  400,
		}
	}
	if length > 128 {
		return &AuthError{
			Code:    "password_too_long",
			Message: "Password must be at most 128 characters",
			Status:  400,
		}
	}
	return nil
}
