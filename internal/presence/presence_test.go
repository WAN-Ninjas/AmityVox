package presence

import (
	"encoding/json"
	"testing"
	"time"
)

func TestStatusConstants(t *testing.T) {
	statuses := []string{
		StatusOnline,
		StatusIdle,
		StatusFocus,
		StatusBusy,
		StatusInvisible,
		StatusOffline,
	}

	seen := make(map[string]bool)
	for _, s := range statuses {
		if s == "" {
			t.Error("empty status constant")
		}
		if seen[s] {
			t.Errorf("duplicate status constant: %q", s)
		}
		seen[s] = true
	}

	if len(statuses) != 6 {
		t.Errorf("expected 6 status constants, got %d", len(statuses))
	}
}

func TestPrefixConstants(t *testing.T) {
	prefixes := map[string]string{
		"session":   PrefixSession,
		"presence":  PrefixPresence,
		"ratelimit": PrefixRateLimit,
		"cache":     PrefixCache,
	}

	for name, prefix := range prefixes {
		if prefix == "" {
			t.Errorf("%s prefix is empty", name)
		}
		// Each prefix should end with ":"
		if prefix[len(prefix)-1] != ':' {
			t.Errorf("%s prefix %q does not end with ':'", name, prefix)
		}
	}
}

func TestSessionData_JSON(t *testing.T) {
	now := time.Date(2026, 2, 10, 12, 0, 0, 0, time.UTC)
	sd := SessionData{
		UserID:    "user_001",
		ExpiresAt: now,
	}

	data, err := json.Marshal(sd)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded SessionData
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.UserID != sd.UserID {
		t.Errorf("user_id = %q, want %q", decoded.UserID, sd.UserID)
	}
	if !decoded.ExpiresAt.Equal(sd.ExpiresAt) {
		t.Errorf("expires_at = %v, want %v", decoded.ExpiresAt, sd.ExpiresAt)
	}
}

func TestSessionData_EmptyUserID(t *testing.T) {
	sd := SessionData{
		UserID:    "",
		ExpiresAt: time.Now(),
	}

	data, err := json.Marshal(sd)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded SessionData
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.UserID != "" {
		t.Errorf("user_id = %q, want empty string", decoded.UserID)
	}
}

func TestPrefixKeyGeneration(t *testing.T) {
	tests := []struct {
		prefix string
		key    string
		want   string
	}{
		{PrefixSession, "abc123", "session:abc123"},
		{PrefixPresence, "user_001", "presence:user_001"},
		{PrefixRateLimit, "global:127.0.0.1", "ratelimit:global:127.0.0.1"},
		{PrefixCache, "guild:settings:g1", "cache:guild:settings:g1"},
	}

	for _, tt := range tests {
		got := tt.prefix + tt.key
		if got != tt.want {
			t.Errorf("prefix+key = %q, want %q", got, tt.want)
		}
	}
}
