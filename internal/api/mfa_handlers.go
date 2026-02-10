package api

import (
	"encoding/json"
	"net/http"

	"github.com/amityvox/amityvox/internal/auth"
)

// --- TOTP (Time-based One-Time Password) Handlers ---
// These will be fully implemented when the TOTP library is integrated.
// For now they define the request/response contracts and return 501.

// TOTPEnableRequest is the request body for POST /auth/totp/enable.
type TOTPEnableRequest struct {
	Password string `json:"password"` // Re-authenticate before enabling 2FA.
}

// TOTPEnableResponse is returned when TOTP setup begins.
type TOTPEnableResponse struct {
	Secret    string `json:"secret"`     // Base32-encoded TOTP secret.
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

	WriteError(w, http.StatusNotImplemented, "not_implemented",
		"TOTP two-factor authentication will be available in v0.2.0")
}

// handleTOTPVerify handles POST /api/v1/auth/totp/verify.
// Verifies a TOTP code to complete 2FA setup or during login.
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

	WriteError(w, http.StatusNotImplemented, "not_implemented",
		"TOTP two-factor authentication will be available in v0.2.0")
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

	WriteError(w, http.StatusNotImplemented, "not_implemented",
		"TOTP two-factor authentication will be available in v0.2.0")
}

// --- WebAuthn (FIDO2) Handlers ---
// These will be fully implemented when the WebAuthn library is integrated.
// For now they define the request/response contracts and return 501.

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
// Initiates WebAuthn credential registration for the authenticated user.
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
// Completes WebAuthn credential registration by verifying the attestation response.
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
// Initiates WebAuthn authentication challenge.
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
// Completes WebAuthn authentication by verifying the assertion response.
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
