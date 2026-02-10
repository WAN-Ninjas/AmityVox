package api

import (
	"testing"
	"time"
)

func TestGenerateTOTP(t *testing.T) {
	// RFC 6238 test vector: secret = base32("12345678901234567890")
	// which is GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ
	secret := "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"

	// Time step 0 should produce a consistent code.
	code := generateTOTP(secret, 0)
	if len(code) != 6 {
		t.Errorf("code length = %d, want 6", len(code))
	}

	// Same inputs should produce the same code.
	code2 := generateTOTP(secret, 0)
	if code != code2 {
		t.Errorf("generateTOTP not deterministic: %q != %q", code, code2)
	}

	// Different time steps should produce different codes (with high probability).
	code3 := generateTOTP(secret, 1)
	if code == code3 {
		t.Log("same code for different time steps (unlikely but possible)")
	}
}

func TestGenerateTOTP_InvalidSecret(t *testing.T) {
	// Invalid base32 should return empty string.
	code := generateTOTP("!!!invalid!!!", 0)
	if code != "" {
		t.Errorf("expected empty code for invalid secret, got %q", code)
	}
}

func TestValidateTOTP(t *testing.T) {
	secret := "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"

	// Generate a code for the current time step.
	now := time.Now().Unix()
	timeStep := now / 30
	currentCode := generateTOTP(secret, timeStep)

	// Current code should validate.
	if !validateTOTP(secret, currentCode) {
		t.Errorf("current TOTP code should be valid")
	}

	// Invalid code should not validate.
	if validateTOTP(secret, "000000") && generateTOTP(secret, timeStep) != "000000" {
		t.Errorf("arbitrary code should not validate")
	}

	// Empty code should not validate.
	if validateTOTP(secret, "") {
		t.Errorf("empty code should not validate")
	}
}

func TestValidateTOTP_DriftTolerance(t *testing.T) {
	secret := "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
	now := time.Now().Unix()
	timeStep := now / 30

	// Code from previous period should validate (drift tolerance ±1).
	prevCode := generateTOTP(secret, timeStep-1)
	if !validateTOTP(secret, prevCode) {
		t.Errorf("previous period code should be valid (drift tolerance)")
	}

	// Code from next period should validate.
	nextCode := generateTOTP(secret, timeStep+1)
	if !validateTOTP(secret, nextCode) {
		t.Errorf("next period code should be valid (drift tolerance)")
	}
}

func TestGenerateTOTP_CodeFormat(t *testing.T) {
	secret := "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"

	// Test multiple time steps to ensure codes are always 6 digits.
	for ts := int64(0); ts < 100; ts++ {
		code := generateTOTP(secret, ts)
		if len(code) != 6 {
			t.Errorf("time step %d: code length = %d, want 6", ts, len(code))
		}
		// Should be all digits.
		for _, c := range code {
			if c < '0' || c > '9' {
				t.Errorf("time step %d: non-digit character %q in code %q", ts, c, code)
			}
		}
	}
}
