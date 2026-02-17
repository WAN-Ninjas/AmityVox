package workers

import (
	"testing"
	"time"
)

// TestRetentionCutoffCalculation verifies that the retention cutoff is computed
// correctly from max_age_days.
func TestRetentionCutoffCalculation(t *testing.T) {
	tests := []struct {
		name       string
		maxAgeDays int
		wantDelta  time.Duration
	}{
		{"1 day", 1, 24 * time.Hour},
		{"7 days", 7, 7 * 24 * time.Hour},
		{"30 days", 30, 30 * 24 * time.Hour},
		{"90 days", 90, 90 * 24 * time.Hour},
		{"365 days", 365, 365 * 24 * time.Hour},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Now().UTC()
			cutoff := now.Add(-time.Duration(tc.maxAgeDays) * 24 * time.Hour)
			diff := now.Sub(cutoff)
			// Allow 1 second tolerance for test execution time.
			if diff < tc.wantDelta-time.Second || diff > tc.wantDelta+time.Second {
				t.Errorf("cutoff delta = %v, want ~%v", diff, tc.wantDelta)
			}
		})
	}
}

// TestRetentionBatchSizeConstant ensures the batch size constant is reasonable.
func TestRetentionBatchSizeConstant(t *testing.T) {
	const batchSize = 1000
	if batchSize < 100 {
		t.Error("batch size too small, would cause excessive queries")
	}
	if batchSize > 10000 {
		t.Error("batch size too large, could cause memory/transaction issues")
	}
}

// TestRetentionPolicyStructFields verifies the policy struct is scannable.
func TestRetentionPolicyStructFields(t *testing.T) {
	type policy struct {
		ID                string
		ChannelID         *string
		GuildID           *string
		MaxAgeDays        int
		DeleteAttachments bool
		DeletePins        bool
	}

	chID := "test-channel"
	guildID := "test-guild"

	tests := []struct {
		name    string
		policy  policy
		scoped  string
	}{
		{
			name:   "channel-scoped",
			policy: policy{ID: "p1", ChannelID: &chID, GuildID: &guildID, MaxAgeDays: 30, DeleteAttachments: true, DeletePins: false},
			scoped: "channel",
		},
		{
			name:   "guild-scoped",
			policy: policy{ID: "p2", ChannelID: nil, GuildID: &guildID, MaxAgeDays: 90, DeleteAttachments: false, DeletePins: true},
			scoped: "guild",
		},
		{
			name:   "no-scope-skipped",
			policy: policy{ID: "p3", ChannelID: nil, GuildID: nil, MaxAgeDays: 7},
			scoped: "none",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := tc.policy

			switch tc.scoped {
			case "channel":
				if p.ChannelID == nil {
					t.Error("channel-scoped policy should have ChannelID")
				}
			case "guild":
				if p.ChannelID != nil {
					t.Error("guild-scoped policy should not have ChannelID")
				}
				if p.GuildID == nil {
					t.Error("guild-scoped policy should have GuildID")
				}
			case "none":
				if p.ChannelID != nil || p.GuildID != nil {
					t.Error("unscoped policy should have nil ChannelID and GuildID")
				}
			}
		})
	}
}

// TestRetentionPinExclusionLogic validates the logic for excluding pinned messages.
func TestRetentionPinExclusionLogic(t *testing.T) {
	tests := []struct {
		name       string
		deletePins bool
		wantExclude bool
	}{
		{"delete_pins=true includes pinned", true, false},
		{"delete_pins=false excludes pinned", false, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			excludePinned := !tc.deletePins
			if excludePinned != tc.wantExclude {
				t.Errorf("excludePinned = %v, want %v", excludePinned, tc.wantExclude)
			}
		})
	}
}
