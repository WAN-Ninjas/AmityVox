package api

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"time"

	"github.com/alexedwards/argon2id"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/jackc/pgx/v5"

	"github.com/amityvox/amityvox/internal/api/apiutil"
	"github.com/amityvox/amityvox/internal/auth"
	"github.com/amityvox/amityvox/internal/models"
)

// --- TOTP (Time-based One-Time Password) Handlers ---

// TOTPEnableRequest is the request body for POST /auth/totp/enable.
type TOTPEnableRequest struct {
	Password string `json:"password"` // Re-authenticate before enabling 2FA.
}

// TOTPEnableResponse is returned when TOTP setup begins.
type TOTPEnableResponse struct {
	Secret    string `json:"secret"`      // Base32-encoded TOTP secret.
	QRCodeURI string `json:"qr_code_uri"` // otpauth:// URI for QR code generation.
}

// TOTPVerifyRequest is the request body for POST /auth/totp/verify.
type TOTPVerifyRequest struct {
	Code string `json:"code"` // 6-digit TOTP code.
}

// TOTPDisableRequest is the request body for DELETE /auth/totp.
type TOTPDisableRequest struct {
	Password string `json:"password"` // Re-authenticate before disabling 2FA.
	Code     string `json:"code"`     // Current TOTP code for confirmation.
}

// handleTOTPEnable handles POST /api/v1/auth/totp/enable.
// Generates a TOTP secret for the user and returns the QR code URI.
func (s *Server) handleTOTPEnable(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	if userID == "" {
		WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	var req TOTPEnableRequest
	if !DecodeJSON(w, r, &req) {
		return
	}

	if !apiutil.RequireNonEmpty(w, "Password", req.Password) {
		return
	}

	// Verify password.
	var passwordHash *string
	err := s.DB.Pool.QueryRow(r.Context(),
		`SELECT password_hash FROM users WHERE id = $1`, userID).Scan(&passwordHash)
	if err != nil || passwordHash == nil {
		WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to verify credentials")
		return
	}

	match, err := argon2id.ComparePasswordAndHash(req.Password, *passwordHash)
	if err != nil || !match {
		WriteError(w, http.StatusUnauthorized, "invalid_password", "Incorrect password")
		return
	}

	// Check if TOTP is already enabled.
	var existingSecret *string
	s.DB.Pool.QueryRow(r.Context(),
		`SELECT totp_secret FROM users WHERE id = $1`, userID).Scan(&existingSecret)
	if existingSecret != nil && *existingSecret != "" {
		WriteError(w, http.StatusConflict, "totp_already_enabled", "TOTP is already enabled")
		return
	}

	// Generate a 20-byte TOTP secret.
	secretBytes := make([]byte, 20)
	if _, err := rand.Read(secretBytes); err != nil {
		WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to generate TOTP secret")
		return
	}
	secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(secretBytes)

	// Store the secret (pending verification — stored but not yet confirmed).
	// We store it immediately; if the user never verifies, they can re-enable.
	_, err = s.DB.Pool.Exec(r.Context(),
		`UPDATE users SET totp_secret = $1 WHERE id = $2`, secret, userID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to save TOTP secret")
		return
	}

	// Get the username for the QR code URI.
	var username string
	s.DB.Pool.QueryRow(r.Context(),
		`SELECT username FROM users WHERE id = $1`, userID).Scan(&username)

	issuer := "AmityVox"
	if s.Config.Instance.Name != "" {
		issuer = s.Config.Instance.Name
	}

	qrURI := fmt.Sprintf("otpauth://totp/%s:%s?secret=%s&issuer=%s&algorithm=SHA1&digits=6&period=30",
		issuer, username, secret, issuer)

	WriteJSON(w, http.StatusOK, TOTPEnableResponse{
		Secret:    secret,
		QRCodeURI: qrURI,
	})
}

// handleTOTPVerify handles POST /api/v1/auth/totp/verify.
// Verifies a TOTP code to confirm 2FA setup.
func (s *Server) handleTOTPVerify(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	if userID == "" {
		WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	var req TOTPVerifyRequest
	if !DecodeJSON(w, r, &req) {
		return
	}

	if !apiutil.RequireNonEmpty(w, "TOTP code", req.Code) {
		return
	}

	// Get the user's TOTP secret.
	var secret *string
	err := s.DB.Pool.QueryRow(r.Context(),
		`SELECT totp_secret FROM users WHERE id = $1`, userID).Scan(&secret)
	if err != nil || secret == nil || *secret == "" {
		WriteError(w, http.StatusBadRequest, "totp_not_setup", "TOTP has not been set up. Call /auth/totp/enable first")
		return
	}

	// Validate the code.
	if !validateTOTP(*secret, req.Code) {
		WriteError(w, http.StatusUnauthorized, "invalid_code", "Invalid TOTP code")
		return
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"verified": true,
		"message":  "TOTP verification successful. Two-factor authentication is now active.",
	})
}

// handleTOTPDisable handles DELETE /api/v1/auth/totp.
// Disables TOTP 2FA for the authenticated user.
func (s *Server) handleTOTPDisable(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	if userID == "" {
		WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	var req TOTPDisableRequest
	if !DecodeJSON(w, r, &req) {
		return
	}

	if req.Password == "" || req.Code == "" {
		WriteError(w, http.StatusBadRequest, "missing_fields", "Password and TOTP code are required to disable TOTP")
		return
	}

	// Verify password.
	var passwordHash *string
	err := s.DB.Pool.QueryRow(r.Context(),
		`SELECT password_hash FROM users WHERE id = $1`, userID).Scan(&passwordHash)
	if err != nil || passwordHash == nil {
		WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to verify credentials")
		return
	}

	match, err := argon2id.ComparePasswordAndHash(req.Password, *passwordHash)
	if err != nil || !match {
		WriteError(w, http.StatusUnauthorized, "invalid_password", "Incorrect password")
		return
	}

	// Verify TOTP code.
	var secret *string
	s.DB.Pool.QueryRow(r.Context(),
		`SELECT totp_secret FROM users WHERE id = $1`, userID).Scan(&secret)
	if secret == nil || *secret == "" {
		WriteError(w, http.StatusBadRequest, "totp_not_enabled", "TOTP is not enabled")
		return
	}

	if !validateTOTP(*secret, req.Code) {
		WriteError(w, http.StatusUnauthorized, "invalid_code", "Invalid TOTP code")
		return
	}

	// Remove TOTP secret.
	_, err = s.DB.Pool.Exec(r.Context(),
		`UPDATE users SET totp_secret = NULL WHERE id = $1`, userID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to disable TOTP")
		return
	}

	WriteNoContent(w)
}

// --- 2FA Backup Codes ---

// handleGenerateBackupCodes generates new backup recovery codes for the authenticated user.
// POST /api/v1/auth/backup-codes
func (s *Server) handleGenerateBackupCodes(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	if userID == "" {
		WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Password == "" {
		WriteError(w, http.StatusBadRequest, "invalid_body", "Password is required")
		return
	}

	// Verify password.
	var passwordHash *string
	s.DB.Pool.QueryRow(r.Context(),
		`SELECT password_hash FROM users WHERE id = $1`, userID).Scan(&passwordHash)
	if passwordHash == nil {
		WriteError(w, http.StatusBadRequest, "no_password", "Account has no password")
		return
	}

	match, err := argon2id.ComparePasswordAndHash(req.Password, *passwordHash)
	if err != nil || !match {
		WriteError(w, http.StatusUnauthorized, "invalid_password", "Password is incorrect")
		return
	}

	// Verify TOTP is enabled (backup codes only make sense with 2FA).
	var totpSecret *string
	s.DB.Pool.QueryRow(r.Context(),
		`SELECT totp_secret FROM users WHERE id = $1`, userID).Scan(&totpSecret)
	if totpSecret == nil || *totpSecret == "" {
		WriteError(w, http.StatusBadRequest, "totp_not_enabled", "Enable TOTP before generating backup codes")
		return
	}

	// Generate 10 backup codes.
	codes := make([]string, 10)
	for i := range codes {
		buf := make([]byte, 4)
		if _, err := rand.Read(buf); err != nil {
			WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to generate codes")
			return
		}
		codes[i] = fmt.Sprintf("%08x", buf)
	}

	// Delete old codes and insert new ones.
	err = apiutil.WithTx(r.Context(), s.DB.Pool, func(tx pgx.Tx) error {
		tx.Exec(r.Context(), `DELETE FROM backup_codes WHERE user_id = $1`, userID)
		for _, code := range codes {
			hash, err := argon2id.CreateHash(code, argon2id.DefaultParams)
			if err != nil {
				return err
			}
			tx.Exec(r.Context(),
				`INSERT INTO backup_codes (user_id, code_hash, used) VALUES ($1, $2, false)`,
				userID, hash)
		}
		return nil
	})
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to generate codes")
		return
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"codes": codes,
	})
}

// handleConsumeBackupCode validates and consumes a backup code during login.
// POST /api/v1/auth/backup-codes/verify
func (s *Server) handleConsumeBackupCode(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	if userID == "" {
		WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	var req struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Code == "" {
		WriteError(w, http.StatusBadRequest, "invalid_body", "Code is required")
		return
	}

	// Get all unused backup codes for this user.
	rows, err := s.DB.Pool.Query(r.Context(),
		`SELECT id, code_hash FROM backup_codes WHERE user_id = $1 AND used = false`,
		userID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to verify code")
		return
	}
	defer rows.Close()

	var matchedID string
	for rows.Next() {
		var id, hash string
		if err := rows.Scan(&id, &hash); err != nil {
			continue
		}
		match, err := argon2id.ComparePasswordAndHash(req.Code, hash)
		if err == nil && match {
			matchedID = id
			break
		}
	}

	if matchedID == "" {
		WriteError(w, http.StatusUnauthorized, "invalid_code", "Invalid or already used backup code")
		return
	}

	// Mark the code as used.
	s.DB.Pool.Exec(r.Context(),
		`UPDATE backup_codes SET used = true, used_at = now() WHERE id = $1`, matchedID)

	WriteJSON(w, http.StatusOK, map[string]string{"status": "verified"})
}

// --- TOTP Core (RFC 6238) ---

// generateTOTP generates a TOTP code for the given base32-encoded secret and time step.
func generateTOTP(secret string, timeStep int64) string {
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
	if err != nil {
		return ""
	}

	// Convert time step to big-endian 8-byte array.
	msg := make([]byte, 8)
	binary.BigEndian.PutUint64(msg, uint64(timeStep))

	// HMAC-SHA1.
	mac := hmac.New(sha1.New, key)
	mac.Write(msg)
	hash := mac.Sum(nil)

	// Dynamic truncation.
	offset := hash[len(hash)-1] & 0x0F
	code := binary.BigEndian.Uint32(hash[offset:offset+4]) & 0x7FFFFFFF

	// 6-digit code.
	otp := code % uint32(math.Pow10(6))
	return fmt.Sprintf("%06d", otp)
}

// validateTOTP checks a TOTP code against the secret, allowing ±1 time step drift.
// Uses constant-time comparison to prevent timing attacks.
func validateTOTP(secret, code string) bool {
	now := time.Now().Unix()
	timeStep := now / 30

	// Check current period and ±1 for clock drift tolerance.
	for _, offset := range []int64{-1, 0, 1} {
		expected := generateTOTP(secret, timeStep+offset)
		if subtle.ConstantTimeCompare([]byte(expected), []byte(code)) == 1 {
			return true
		}
	}
	return false
}

// --- WebAuthn (FIDO2) Handlers ---

// webauthnUser adapts a database user row to the webauthn.User interface.
type webauthnUser struct {
	id          string
	username    string
	displayName string
	credentials []webauthn.Credential
}

func (u *webauthnUser) WebAuthnID() []byte          { return []byte(u.id) }
func (u *webauthnUser) WebAuthnName() string         { return u.username }
func (u *webauthnUser) WebAuthnDisplayName() string  { return u.displayName }
func (u *webauthnUser) WebAuthnCredentials() []webauthn.Credential { return u.credentials }

// loadWebAuthnUser loads a user and their WebAuthn credentials from the database.
func (s *Server) loadWebAuthnUser(r *http.Request, userID string) (*webauthnUser, error) {
	var username string
	var displayName *string
	err := s.DB.Pool.QueryRow(r.Context(),
		`SELECT username, display_name FROM users WHERE id = $1`, userID,
	).Scan(&username, &displayName)
	if err != nil {
		return nil, fmt.Errorf("loading user: %w", err)
	}

	dn := username
	if displayName != nil && *displayName != "" {
		dn = *displayName
	}

	// Load existing WebAuthn credentials.
	rows, err := s.DB.Pool.Query(r.Context(),
		`SELECT credential_id, public_key, sign_count FROM webauthn_credentials WHERE user_id = $1`,
		userID)
	if err != nil {
		return nil, fmt.Errorf("loading credentials: %w", err)
	}
	defer rows.Close()

	var creds []webauthn.Credential
	for rows.Next() {
		var credID, pubKey []byte
		var signCount int64
		if err := rows.Scan(&credID, &pubKey, &signCount); err != nil {
			continue
		}
		creds = append(creds, webauthn.Credential{
			ID:        credID,
			PublicKey: pubKey,
			Authenticator: webauthn.Authenticator{
				SignCount: uint32(signCount),
			},
		})
	}

	return &webauthnUser{
		id:          userID,
		username:    username,
		displayName: dn,
		credentials: creds,
	}, nil
}

// webauthnSessionKey returns the cache key for a WebAuthn session challenge.
func webauthnSessionKey(userID, ceremony string) string {
	return "webauthn:" + ceremony + ":" + userID
}

// handleWebAuthnRegisterBegin handles POST /api/v1/auth/webauthn/register/begin.
// Generates a WebAuthn registration challenge and returns options to the client.
func (s *Server) handleWebAuthnRegisterBegin(w http.ResponseWriter, r *http.Request) {
	if s.WebAuthn == nil {
		WriteError(w, http.StatusServiceUnavailable, "webauthn_disabled", "WebAuthn is not configured")
		return
	}

	userID := auth.UserIDFromContext(r.Context())
	if userID == "" {
		WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	user, err := s.loadWebAuthnUser(r, userID)
	if err != nil {
		InternalError(w, s.Logger, "Failed to load user data", err)
		return
	}

	options, session, err := s.WebAuthn.BeginRegistration(user)
	if err != nil {
		s.Logger.Error("WebAuthn BeginRegistration failed", "error", err.Error())
		WriteError(w, http.StatusInternalServerError, "webauthn_error", "Failed to begin registration")
		return
	}

	// Store session data in cache with 5-minute TTL.
	if err := s.Cache.Set(r.Context(), webauthnSessionKey(userID, "register"), session, 5*time.Minute); err != nil {
		InternalError(w, s.Logger, "Failed to store session", err)
		return
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"options": options,
	})
}

// handleWebAuthnRegisterFinish handles POST /api/v1/auth/webauthn/register/finish.
// Validates the attestation response and stores the new credential.
func (s *Server) handleWebAuthnRegisterFinish(w http.ResponseWriter, r *http.Request) {
	if s.WebAuthn == nil {
		WriteError(w, http.StatusServiceUnavailable, "webauthn_disabled", "WebAuthn is not configured")
		return
	}

	userID := auth.UserIDFromContext(r.Context())
	if userID == "" {
		WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	user, err := s.loadWebAuthnUser(r, userID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to load user data")
		return
	}

	// Retrieve session data from cache.
	var session webauthn.SessionData
	found, err := s.Cache.Get(r.Context(), webauthnSessionKey(userID, "register"), &session)
	if err != nil || !found {
		WriteError(w, http.StatusBadRequest, "session_expired", "Registration session expired or not found")
		return
	}

	// Parse the credential name from a query parameter or header.
	credName := r.URL.Query().Get("name")
	if credName == "" {
		credName = "Security Key"
	}

	credential, err := s.WebAuthn.FinishRegistration(user, session, r)
	if err != nil {
		s.Logger.Error("WebAuthn FinishRegistration failed", "error", err.Error())
		WriteError(w, http.StatusBadRequest, "registration_failed", "Failed to verify registration")
		return
	}

	// Store the credential in the database.
	credID := models.NewULID().String()
	_, err = s.DB.Pool.Exec(r.Context(),
		`INSERT INTO webauthn_credentials (id, user_id, credential_id, public_key, sign_count, name, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, now())`,
		credID, userID, credential.ID, credential.PublicKey, credential.Authenticator.SignCount, credName,
	)
	if err != nil {
		InternalError(w, s.Logger, "Failed to store credential", err)
		return
	}

	WriteJSON(w, http.StatusCreated, map[string]interface{}{
		"id":         credID,
		"name":       credName,
		"created_at": time.Now().UTC(),
	})
}

// handleWebAuthnLoginBegin handles POST /api/v1/auth/webauthn/login/begin.
// Generates a WebAuthn assertion challenge for an authenticated user.
func (s *Server) handleWebAuthnLoginBegin(w http.ResponseWriter, r *http.Request) {
	if s.WebAuthn == nil {
		WriteError(w, http.StatusServiceUnavailable, "webauthn_disabled", "WebAuthn is not configured")
		return
	}

	userID := auth.UserIDFromContext(r.Context())
	if userID == "" {
		WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	user, err := s.loadWebAuthnUser(r, userID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to load user data")
		return
	}

	if len(user.credentials) == 0 {
		WriteError(w, http.StatusBadRequest, "no_credentials", "No WebAuthn credentials registered")
		return
	}

	options, session, err := s.WebAuthn.BeginLogin(user)
	if err != nil {
		s.Logger.Error("WebAuthn BeginLogin failed", "error", err.Error())
		WriteError(w, http.StatusInternalServerError, "webauthn_error", "Failed to begin login")
		return
	}

	// Store session data in cache with 5-minute TTL.
	if err := s.Cache.Set(r.Context(), webauthnSessionKey(userID, "login"), session, 5*time.Minute); err != nil {
		InternalError(w, s.Logger, "Failed to store session", err)
		return
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"options": options,
	})
}

// handleWebAuthnLoginFinish handles POST /api/v1/auth/webauthn/login/finish.
// Validates the assertion response, verifying the user's security key.
func (s *Server) handleWebAuthnLoginFinish(w http.ResponseWriter, r *http.Request) {
	if s.WebAuthn == nil {
		WriteError(w, http.StatusServiceUnavailable, "webauthn_disabled", "WebAuthn is not configured")
		return
	}

	userID := auth.UserIDFromContext(r.Context())
	if userID == "" {
		WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	user, err := s.loadWebAuthnUser(r, userID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to load user data")
		return
	}

	// Retrieve session data from cache.
	var session webauthn.SessionData
	found, err := s.Cache.Get(r.Context(), webauthnSessionKey(userID, "login"), &session)
	if err != nil || !found {
		WriteError(w, http.StatusBadRequest, "session_expired", "Login session expired or not found")
		return
	}

	credential, err := s.WebAuthn.FinishLogin(user, session, r)
	if err != nil {
		s.Logger.Error("WebAuthn FinishLogin failed", "error", err.Error())
		WriteError(w, http.StatusUnauthorized, "login_failed", "Failed to verify security key")
		return
	}

	// Update sign count in database to detect cloned keys.
	s.DB.Pool.Exec(r.Context(),
		`UPDATE webauthn_credentials SET sign_count = $1 WHERE credential_id = $2 AND user_id = $3`,
		credential.Authenticator.SignCount, credential.ID, userID,
	)

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"verified": true,
		"message":  "WebAuthn authentication successful",
	})
}
