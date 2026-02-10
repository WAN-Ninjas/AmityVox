package api

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"time"

	"github.com/alexedwards/argon2id"

	"github.com/amityvox/amityvox/internal/auth"
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
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if req.Password == "" {
		WriteError(w, http.StatusBadRequest, "missing_password", "Password is required to enable TOTP")
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
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if req.Code == "" {
		WriteError(w, http.StatusBadRequest, "missing_code", "TOTP code is required")
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
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
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
	tx, err := s.DB.Pool.Begin(r.Context())
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to generate codes")
		return
	}
	defer tx.Rollback(r.Context())

	tx.Exec(r.Context(), `DELETE FROM backup_codes WHERE user_id = $1`, userID)
	for _, code := range codes {
		hash, err := argon2id.CreateHash(code, argon2id.DefaultParams)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to hash code")
			return
		}
		tx.Exec(r.Context(),
			`INSERT INTO backup_codes (user_id, code_hash, used) VALUES ($1, $2, false)`,
			userID, hash)
	}

	if err := tx.Commit(r.Context()); err != nil {
		WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to save codes")
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
func validateTOTP(secret, code string) bool {
	now := time.Now().Unix()
	timeStep := now / 30

	// Check current period and ±1 for clock drift tolerance.
	for _, offset := range []int64{-1, 0, 1} {
		if generateTOTP(secret, timeStep+offset) == code {
			return true
		}
	}
	return false
}

// --- WebAuthn (FIDO2) Handlers ---
// These will be fully implemented when the WebAuthn library is integrated.

// WebAuthnRegisterBeginResponse is returned to start WebAuthn registration.
type WebAuthnRegisterBeginResponse struct {
	Options interface{} `json:"options"` // PublicKeyCredentialCreationOptions from WebAuthn spec.
}

// WebAuthnRegisterFinishRequest completes WebAuthn registration.
type WebAuthnRegisterFinishRequest struct {
	Credential interface{} `json:"credential"` // AuthenticatorAttestationResponse from browser.
	Name       string      `json:"name"`       // User-friendly name for this credential.
}

// WebAuthnLoginBeginResponse is returned to start WebAuthn authentication.
type WebAuthnLoginBeginResponse struct {
	Options interface{} `json:"options"` // PublicKeyCredentialRequestOptions from WebAuthn spec.
}

// WebAuthnLoginFinishRequest completes WebAuthn authentication.
type WebAuthnLoginFinishRequest struct {
	Credential interface{} `json:"credential"` // AuthenticatorAssertionResponse from browser.
}

// handleWebAuthnRegisterBegin handles POST /api/v1/auth/webauthn/register/begin.
func (s *Server) handleWebAuthnRegisterBegin(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	if userID == "" {
		WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	WriteError(w, http.StatusNotImplemented, "not_implemented",
		"WebAuthn registration will be available in v0.2.0")
}

// handleWebAuthnRegisterFinish handles POST /api/v1/auth/webauthn/register/finish.
func (s *Server) handleWebAuthnRegisterFinish(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	if userID == "" {
		WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	var req WebAuthnRegisterFinishRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	WriteError(w, http.StatusNotImplemented, "not_implemented",
		"WebAuthn registration will be available in v0.2.0")
}

// handleWebAuthnLoginBegin handles POST /api/v1/auth/webauthn/login/begin.
func (s *Server) handleWebAuthnLoginBegin(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	if userID == "" {
		WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	WriteError(w, http.StatusNotImplemented, "not_implemented",
		"WebAuthn authentication will be available in v0.2.0")
}

// handleWebAuthnLoginFinish handles POST /api/v1/auth/webauthn/login/finish.
func (s *Server) handleWebAuthnLoginFinish(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	if userID == "" {
		WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	var req WebAuthnLoginFinishRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	WriteError(w, http.StatusNotImplemented, "not_implemented",
		"WebAuthn authentication will be available in v0.2.0")
}
