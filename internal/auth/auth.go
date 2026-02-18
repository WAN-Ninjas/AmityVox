// Package auth implements authentication for AmityVox, including user
// registration with Argon2id password hashing, login with session creation,
// logout, session validation, and the authentication middleware that extracts
// user identity from Bearer tokens. TOTP 2FA and WebAuthn will be added in
// a later phase but the session infrastructure supports them.
package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"log/slog"
	"math"
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
	Token    string  `json:"token,omitempty"` // Registration token for invite-only instances.
}

// LoginRequest is the request body for user login.
type LoginRequest struct {
	Username string  `json:"username"`
	Password string  `json:"password"`
	TOTPCode *string `json:"totp_code,omitempty"` // Required if user has TOTP enabled.
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
		           status_emoji, status_presence, status_expires_at, bio,
		           banner_id, accent_color, pronouns,
		           bot_owner_id, email, flags, created_at`,
		userID, s.instanceID, req.Username, hash, req.Email,
	).Scan(
		&user.ID, &user.InstanceID, &user.Username, &user.DisplayName,
		&user.AvatarID, &user.StatusText, &user.StatusEmoji, &user.StatusPresence,
		&user.StatusExpiresAt, &user.Bio, &user.BannerID, &user.AccentColor,
		&user.Pronouns, &user.BotOwnerID, &user.Email, &user.Flags, &user.CreatedAt,
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
		        status_emoji, status_presence, status_expires_at, bio,
		        banner_id, accent_color, pronouns,
		        bot_owner_id, password_hash, totp_secret, email, flags, created_at
		 FROM users
		 WHERE username = $1 AND instance_id = $2`,
		req.Username, s.instanceID,
	).Scan(
		&user.ID, &user.InstanceID, &user.Username, &user.DisplayName,
		&user.AvatarID, &user.StatusText, &user.StatusEmoji, &user.StatusPresence,
		&user.StatusExpiresAt, &user.Bio, &user.BannerID, &user.AccentColor,
		&user.Pronouns, &user.BotOwnerID, &passwordHash, &user.TOTPSecret,
		&user.Email, &user.Flags, &user.CreatedAt,
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

	// Check if TOTP is enabled and verify the code.
	if user.TOTPSecret != nil && *user.TOTPSecret != "" {
		if req.TOTPCode == nil || *req.TOTPCode == "" {
			return nil, nil, &AuthError{Code: "totp_required", Message: "Two-factor authentication code is required", Status: 403}
		}
		if !s.validateTOTP(*user.TOTPSecret, *req.TOTPCode) {
			return nil, nil, &AuthError{Code: "invalid_totp", Message: "Invalid two-factor authentication code", Status: 401}
		}
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
		if err := s.checkUserFlags(ctx, cached.UserID); err != nil {
			return "", err
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

	// Check if user is suspended or deleted.
	if err := s.checkUserFlags(ctx, userID); err != nil {
		return "", err
	}

	// Update last_active_at asynchronously.
	go func() {
		bgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		s.pool.Exec(bgCtx, "UPDATE user_sessions SET last_active_at = now() WHERE id = $1", sessionID)
	}()

	return userID, nil
}

// checkUserFlags verifies a user is not suspended or deleted.
func (s *Service) checkUserFlags(ctx context.Context, userID string) error {
	var flags int
	err := s.pool.QueryRow(ctx, `SELECT flags FROM users WHERE id = $1`, userID).Scan(&flags)
	if err != nil {
		s.logger.Error("failed to check user flags", slog.String("user_id", userID), slog.String("error", err.Error()))
		return fmt.Errorf("checking user flags: %w", err)
	}
	const flagSuspended = 1 << 0
	const flagDeleted = 1 << 1
	if flags&flagSuspended != 0 {
		return &AuthError{Code: "account_suspended", Message: "Your account has been suspended", Status: 403}
	}
	if flags&flagDeleted != 0 {
		return &AuthError{Code: "account_deleted", Message: "This account has been deleted", Status: 403}
	}
	return nil
}

// ChangePasswordRequest is the request body for changing a user's password.
type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// ChangePassword validates the current password and updates it to a new one.
func (s *Service) ChangePassword(ctx context.Context, userID string, req ChangePasswordRequest) error {
	if err := validatePassword(req.NewPassword); err != nil {
		return err
	}

	// Get current password hash.
	var passwordHash *string
	err := s.pool.QueryRow(ctx,
		`SELECT password_hash FROM users WHERE id = $1`, userID,
	).Scan(&passwordHash)
	if err != nil {
		return fmt.Errorf("querying user: %w", err)
	}

	if passwordHash == nil {
		return &AuthError{Code: "no_password", Message: "Account does not have a password set", Status: 400}
	}

	match, err := argon2id.ComparePasswordAndHash(req.CurrentPassword, *passwordHash)
	if err != nil {
		return fmt.Errorf("comparing password hash: %w", err)
	}
	if !match {
		return &AuthError{Code: "invalid_password", Message: "Current password is incorrect", Status: 401}
	}

	newHash, err := argon2id.CreateHash(req.NewPassword, argon2id.DefaultParams)
	if err != nil {
		return fmt.Errorf("hashing new password: %w", err)
	}

	_, err = s.pool.Exec(ctx,
		`UPDATE users SET password_hash = $2 WHERE id = $1`, userID, newHash)
	if err != nil {
		return fmt.Errorf("updating password: %w", err)
	}

	s.logger.Info("user changed password", slog.String("user_id", userID))
	return nil
}

// ChangeEmailRequest is the request body for changing a user's email.
type ChangeEmailRequest struct {
	Password string `json:"password"`
	NewEmail string `json:"new_email"`
}

// ChangeEmail validates the password and updates the user's email address.
func (s *Service) ChangeEmail(ctx context.Context, userID string, req ChangeEmailRequest) error {
	if req.NewEmail == "" || len(req.NewEmail) > 254 {
		return &AuthError{Code: "invalid_email", Message: "A valid email address is required", Status: 400}
	}

	// Get current password hash.
	var passwordHash *string
	err := s.pool.QueryRow(ctx,
		`SELECT password_hash FROM users WHERE id = $1`, userID,
	).Scan(&passwordHash)
	if err != nil {
		return fmt.Errorf("querying user: %w", err)
	}

	if passwordHash == nil {
		return &AuthError{Code: "no_password", Message: "Account does not have a password set", Status: 400}
	}

	match, err := argon2id.ComparePasswordAndHash(req.Password, *passwordHash)
	if err != nil {
		return fmt.Errorf("comparing password hash: %w", err)
	}
	if !match {
		return &AuthError{Code: "invalid_password", Message: "Password is incorrect", Status: 401}
	}

	// Check if email is already taken.
	var exists bool
	s.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM users WHERE email = $1 AND id != $2)`,
		req.NewEmail, userID,
	).Scan(&exists)
	if exists {
		return &AuthError{Code: "email_taken", Message: "This email address is already in use", Status: 409}
	}

	_, err = s.pool.Exec(ctx,
		`UPDATE users SET email = $2 WHERE id = $1`, userID, req.NewEmail)
	if err != nil {
		return fmt.Errorf("updating email: %w", err)
	}

	s.logger.Info("user changed email", slog.String("user_id", userID))
	return nil
}

// GetUser retrieves a user by ID from the database.
func (s *Service) GetUser(ctx context.Context, userID string) (*models.User, error) {
	var user models.User
	err := s.pool.QueryRow(ctx,
		`SELECT id, instance_id, username, display_name, avatar_id, status_text,
		        status_emoji, status_presence, status_expires_at, bio,
		        banner_id, accent_color, pronouns,
		        bot_owner_id, email, flags, created_at
		 FROM users WHERE id = $1`,
		userID,
	).Scan(
		&user.ID, &user.InstanceID, &user.Username, &user.DisplayName,
		&user.AvatarID, &user.StatusText, &user.StatusEmoji, &user.StatusPresence,
		&user.StatusExpiresAt, &user.Bio, &user.BannerID, &user.AccentColor,
		&user.Pronouns, &user.BotOwnerID, &user.Email, &user.Flags, &user.CreatedAt,
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

// validateTOTP checks a TOTP code against the secret, allowing ±1 time step drift.
// Uses constant-time comparison to prevent timing attacks.
func (s *Service) validateTOTP(secret, code string) bool {
	now := time.Now().Unix()
	timeStep := now / 30

	for _, offset := range []int64{-1, 0, 1} {
		expected := generateTOTPCode(secret, timeStep+offset)
		if subtle.ConstantTimeCompare([]byte(expected), []byte(code)) == 1 {
			return true
		}
	}
	return false
}

// generateTOTPCode generates a 6-digit TOTP code per RFC 6238.
func generateTOTPCode(secret string, timeStep int64) string {
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
	if err != nil {
		return ""
	}

	msg := make([]byte, 8)
	binary.BigEndian.PutUint64(msg, uint64(timeStep))

	mac := hmac.New(sha1.New, key)
	mac.Write(msg)
	hash := mac.Sum(nil)

	offset := hash[len(hash)-1] & 0x0F
	code := binary.BigEndian.Uint32(hash[offset:offset+4]) & 0x7FFFFFFF
	otp := code % uint32(math.Pow10(6))
	return fmt.Sprintf("%06d", otp)
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
