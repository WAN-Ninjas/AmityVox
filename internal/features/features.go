package features

import (
	"context"
	"fmt"
	"sort"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Definition struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Standard    bool   `json:"standard"`
}

type State struct {
	Key             string `json:"key"`
	Name            string `json:"name"`
	Description     string `json:"description"`
	Enabled         bool   `json:"enabled"`
	InstanceEnabled bool   `json:"instance_enabled"`
	InstanceHardOff bool   `json:"instance_hard_disabled"`
	GuildEnabled    *bool  `json:"guild_enabled,omitempty"`
	DisabledBy      string `json:"disabled_by,omitempty"`
	Standard        bool   `json:"standard"`
}

var definitions = []Definition{
	{"polls", "Polls", "Create and vote on polls in message channels.", true},
	{"code_snippets", "Code Snippets", "Share formatted code snippets without execution.", true},
	{"voice_transcription", "Voice Transcription", "Transcribe voice sessions through a configured engine.", true},
	{"video_recordings", "Video Recordings", "Save and browse voice/video session recordings.", true},
	{"whiteboards", "Whiteboards", "Collaborative channel whiteboards.", true},
	{"kanban_boards", "Kanban Boards", "Collaborative project boards.", true},
	{"activities", "Activities", "Built-in shared activities and custom activity labels.", true},
	{"voice_broadcasts", "Voice Broadcasts", "One-to-many voice sessions with listener state.", true},
	{"message_effects", "Message Effects", "Enhanced visual message effects.", true},
	{"super_reactions", "Super Reactions", "Enhanced message reactions.", true},
	{"location_sharing", "Location Sharing", "Share static or live locations in channels.", true},
	{"message_summaries", "Message Summaries", "Basic non-LLM channel summaries.", true},
	{"sticker_packs", "Sticker Packs", "Guild and user sticker packs.", true},
	{"custom_emoji", "Custom Emoji", "Guild emoji management and use.", true},
	{"gif_search", "GIF Search", "Giphy-backed GIF search and posting.", true},
	{"message_bookmarks", "Message Bookmarks", "Save and browse bookmarked messages.", true},
	{"scheduled_messages", "Scheduled Messages", "Deliver messages at a future time.", true},
	{"expiring_messages", "Expiring Messages", "Automatically expire selected messages.", true},
	{"threads_and_replies", "Threads And Replies", "Inline replies and thread navigation.", true},
	{"pins", "Pins", "Pin and browse important messages.", true},
	{"full_text_search", "Full-Text Search", "Search indexed channel history.", true},
	{"federated_messaging", "Federated Messaging", "Cross-instance message delivery.", true},
	{"federated_presence", "Federated Presence", "Cross-instance online state.", true},
	{"federated_attachments", "Federated Attachments", "Cross-instance media delivery.", true},
	{"federation_admin_diagnostics", "Federation Admin Diagnostics", "Operator federation troubleshooting tools.", true},
	{"webhooks", "Webhooks", "External systems posting into channels.", true},
	{"translation", "Translation", "Translate messages through the configured provider.", true},
	{"e2ee", "End-To-End Encryption", "Client-side encrypted conversations.", true},
	{"guild_onboarding", "Guild Onboarding", "New-member onboarding flows.", true},
	{"channel_groups", "Channel Groups", "Grouped and reordered channels.", true},
	{"announcement_channels", "Announcement Channels", "Followable announcement channels.", true},
	{"automod", "AutoMod", "Automated moderation rules and actions.", true},
	{"moderation_reports", "Moderation Reports", "User reports and moderation review.", true},
	{"audit_logs", "Audit Logs", "Administrative and moderation history.", true},
	{"gallery_media", "Gallery And Media", "Channel and guild media management.", true},
	{"admin_backups", "Admin Backups", "Data, media, and combined scheduled backups.", true},
	{"theme_editor", "Theme Editor", "Visual theme editing.", true},
	{"widgets", "Guild Widgets", "Embeddable guild and channel widgets.", true},
	{"pwa_push", "PWA And Push", "Installable app and push notifications.", true},
}

func Definitions() []Definition {
	out := append([]Definition(nil), definitions...)
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

func IsKnown(key string) bool {
	for _, def := range definitions {
		if def.Key == key {
			return true
		}
	}
	return false
}

func Resolve(ctx context.Context, pool *pgxpool.Pool, guildID string) (map[string]State, error) {
	states := make(map[string]State, len(definitions))
	for _, def := range definitions {
		states[def.Key] = State{
			Key:             def.Key,
			Name:            def.Name,
			Description:     def.Description,
			Enabled:         true,
			InstanceEnabled: true,
			Standard:        def.Standard,
		}
	}

	rows, err := pool.Query(ctx, `SELECT feature_key, enabled, hard_disabled FROM instance_feature_flags`)
	if err != nil {
		return nil, fmt.Errorf("loading instance feature flags: %w", err)
	}
	for rows.Next() {
		var key string
		var enabled, hardDisabled bool
		if err := rows.Scan(&key, &enabled, &hardDisabled); err != nil {
			rows.Close()
			return nil, err
		}
		state, ok := states[key]
		if !ok {
			continue
		}
		state.InstanceEnabled = enabled
		state.InstanceHardOff = hardDisabled
		state.Enabled = enabled && !hardDisabled
		if hardDisabled {
			state.DisabledBy = "instance_hard_disable"
		} else if !enabled {
			state.DisabledBy = "instance"
		}
		states[key] = state
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if guildID == "" {
		return states, nil
	}

	rows, err = pool.Query(ctx, `SELECT feature_key, enabled FROM guild_feature_flags WHERE guild_id = $1`, guildID)
	if err != nil && err != pgx.ErrNoRows {
		return nil, fmt.Errorf("loading guild feature flags: %w", err)
	}
	for rows.Next() {
		var key string
		var enabled bool
		if err := rows.Scan(&key, &enabled); err != nil {
			rows.Close()
			return nil, err
		}
		state, ok := states[key]
		if !ok {
			continue
		}
		state.GuildEnabled = &enabled
		if state.Enabled && !enabled {
			state.Enabled = false
			state.DisabledBy = "guild"
		}
		states[key] = state
	}
	return states, rows.Err()
}

func EnabledMap(states map[string]State) map[string]bool {
	out := make(map[string]bool, len(states))
	for key, state := range states {
		out[key] = state.Enabled
	}
	return out
}
