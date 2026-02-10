package models

import (
	"testing"
	"time"
)

func TestUserFlags(t *testing.T) {
	tests := []struct {
		name        string
		flags       int
		suspended   bool
		deleted     bool
		admin       bool
		bot         bool
	}{
		{"no flags", 0, false, false, false, false},
		{"suspended", UserFlagSuspended, true, false, false, false},
		{"deleted", UserFlagDeleted, false, true, false, false},
		{"admin", UserFlagAdmin, false, false, true, false},
		{"bot", UserFlagBot, false, false, false, true},
		{"suspended+admin", UserFlagSuspended | UserFlagAdmin, true, false, true, false},
		{"all flags", UserFlagSuspended | UserFlagDeleted | UserFlagAdmin | UserFlagBot | UserFlagVerified, true, true, true, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			u := User{Flags: tc.flags}
			if got := u.IsSuspended(); got != tc.suspended {
				t.Errorf("IsSuspended() = %v, want %v", got, tc.suspended)
			}
			if got := u.IsDeleted(); got != tc.deleted {
				t.Errorf("IsDeleted() = %v, want %v", got, tc.deleted)
			}
			if got := u.IsAdmin(); got != tc.admin {
				t.Errorf("IsAdmin() = %v, want %v", got, tc.admin)
			}
			if got := u.IsBot(); got != tc.bot {
				t.Errorf("IsBot() = %v, want %v", got, tc.bot)
			}
		})
	}
}

func TestGuildMember_IsTimedOut(t *testing.T) {
	tests := []struct {
		name     string
		timeout  *time.Time
		expected bool
	}{
		{"nil timeout", nil, false},
		{"future timeout", timePtr(time.Now().Add(1 * time.Hour)), true},
		{"past timeout", timePtr(time.Now().Add(-1 * time.Hour)), false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := GuildMember{TimeoutUntil: tc.timeout}
			if got := m.IsTimedOut(); got != tc.expected {
				t.Errorf("IsTimedOut() = %v, want %v", got, tc.expected)
			}
		})
	}
}

func TestInvite_IsExpired(t *testing.T) {
	tests := []struct {
		name     string
		expires  *time.Time
		expected bool
	}{
		{"nil expiry", nil, false},
		{"future expiry", timePtr(time.Now().Add(1 * time.Hour)), false},
		{"past expiry", timePtr(time.Now().Add(-1 * time.Hour)), true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			inv := Invite{ExpiresAt: tc.expires}
			if got := inv.IsExpired(); got != tc.expected {
				t.Errorf("IsExpired() = %v, want %v", got, tc.expected)
			}
		})
	}
}

func TestInvite_IsMaxUsesReached(t *testing.T) {
	tests := []struct {
		name     string
		maxUses  *int
		uses     int
		expected bool
	}{
		{"nil max uses", nil, 5, false},
		{"under limit", intPtr(10), 5, false},
		{"at limit", intPtr(10), 10, true},
		{"over limit", intPtr(10), 15, true},
		{"zero max allows unlimited", intPtr(0), 0, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			inv := Invite{MaxUses: tc.maxUses, Uses: tc.uses}
			if got := inv.IsMaxUsesReached(); got != tc.expected {
				t.Errorf("IsMaxUsesReached() = %v, want %v", got, tc.expected)
			}
		})
	}
}

func TestChannelTypeConstants(t *testing.T) {
	// Verify constants are distinct and non-empty.
	types := []string{
		ChannelTypeText, ChannelTypeVoice, ChannelTypeDM,
		ChannelTypeGroup, ChannelTypeAnnouncement, ChannelTypeForum, ChannelTypeStage,
	}
	seen := make(map[string]bool)
	for _, ct := range types {
		if ct == "" {
			t.Errorf("channel type constant is empty")
		}
		if seen[ct] {
			t.Errorf("duplicate channel type: %s", ct)
		}
		seen[ct] = true
	}
}

func TestRelationshipConstants(t *testing.T) {
	statuses := []string{
		RelationshipFriend, RelationshipBlocked,
		RelationshipPendingOutgoing, RelationshipPendingIncoming,
	}
	seen := make(map[string]bool)
	for _, s := range statuses {
		if s == "" {
			t.Errorf("relationship status constant is empty")
		}
		if seen[s] {
			t.Errorf("duplicate relationship status: %s", s)
		}
		seen[s] = true
	}
}

func timePtr(t time.Time) *time.Time { return &t }
func intPtr(n int) *int              { return &n }
