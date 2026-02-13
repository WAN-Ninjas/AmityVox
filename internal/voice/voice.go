// Package voice implements LiveKit integration for voice and video channels.
// It handles token generation, room lifecycle management, and voice state
// tracking. See docs/architecture.md Section 8 for the WebSocket events.
package voice

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	lksdk "github.com/livekit/server-sdk-go/v2"
	"github.com/livekit/protocol/auth"
	"github.com/livekit/protocol/livekit"
)

// VoiceState tracks a user's current voice channel presence.
type VoiceState struct {
	UserID          string `json:"user_id"`
	GuildID         string `json:"guild_id"`
	ChannelID       string `json:"channel_id"`
	SelfMute        bool   `json:"self_mute"`
	SelfDeaf        bool   `json:"self_deaf"`
	Muted           bool   `json:"muted"`            // server-side mute by moderator
	Deafened        bool   `json:"deafened"`          // server-side deafen by moderator
	InputMode       string `json:"input_mode"`        // "vad" or "ptt"
	PrioritySpeaker bool   `json:"priority_speaker"`  // attenuate others when speaking
	Broadcasting    bool   `json:"broadcasting"`       // one-way broadcast mode
	ScreenSharing   bool   `json:"screen_sharing"`     // screen share active
}

// VoicePreferences holds per-user voice settings persisted in the database.
type VoicePreferences struct {
	UserID            string  `json:"user_id"`
	InputMode         string  `json:"input_mode"`          // "vad" or "ptt"
	PTTKey            string  `json:"ptt_key"`             // keybind for push-to-talk
	VADThreshold      float64 `json:"vad_threshold"`       // 0.0-1.0
	NoiseSuppression  bool    `json:"noise_suppression"`
	EchoCancellation  bool    `json:"echo_cancellation"`
	AutoGainControl   bool    `json:"auto_gain_control"`
	InputVolume       float64 `json:"input_volume"`        // 0.0-2.0
	OutputVolume      float64 `json:"output_volume"`       // 0.0-2.0
}

// SoundboardSound represents a guild soundboard clip.
type SoundboardSound struct {
	ID         string  `json:"id"`
	GuildID    string  `json:"guild_id"`
	Name       string  `json:"name"`
	FileURL    string  `json:"file_url"`
	Volume     float64 `json:"volume"`
	DurationMs int     `json:"duration_ms"`
	Emoji      *string `json:"emoji,omitempty"`
	CreatorID  string  `json:"creator_id"`
	PlayCount  int64   `json:"play_count"`
	CreatedAt  string  `json:"created_at"`
}

// SoundboardConfig holds per-guild soundboard settings.
type SoundboardConfig struct {
	GuildID         string `json:"guild_id"`
	Enabled         bool   `json:"enabled"`
	MaxSounds       int    `json:"max_sounds"`
	CooldownSeconds int    `json:"cooldown_seconds"`
	AllowExternal   bool   `json:"allow_external"`
}

// VoiceBroadcast represents an active one-way audio broadcast.
type VoiceBroadcast struct {
	ID            string  `json:"id"`
	GuildID       string  `json:"guild_id"`
	ChannelID     string  `json:"channel_id"`
	BroadcasterID string  `json:"broadcaster_id"`
	Title         string  `json:"title"`
	StartedAt     string  `json:"started_at"`
	EndedAt       *string `json:"ended_at,omitempty"`
	ListenerCount int     `json:"listener_count"`
}

// ScreenShareSession represents an active screen sharing session.
type ScreenShareSession struct {
	ID           string  `json:"id"`
	ChannelID    string  `json:"channel_id"`
	UserID       string  `json:"user_id"`
	ShareType    string  `json:"share_type"`    // "screen" or "window"
	Resolution   string  `json:"resolution"`     // "720p", "1080p", "4k"
	Framerate    int     `json:"framerate"`      // 15, 30, or 60
	AudioEnabled bool    `json:"audio_enabled"`
	MaxViewers   int     `json:"max_viewers"`
	StartedAt    string  `json:"started_at"`
	EndedAt      *string `json:"ended_at,omitempty"`
}

// Config holds configuration for the voice service.
type Config struct {
	URL       string
	APIKey    string
	APISecret string
	Pool      *pgxpool.Pool
	Logger    *slog.Logger
}

// Service manages LiveKit rooms and voice state.
type Service struct {
	roomClient *lksdk.RoomServiceClient
	apiKey     string
	apiSecret  string
	pool       *pgxpool.Pool
	logger     *slog.Logger

	// In-memory voice state tracking.
	states   map[string]*VoiceState // keyed by userID
	statesMu sync.RWMutex
}

// New creates a new voice service connected to LiveKit.
func New(cfg Config) (*Service, error) {
	if cfg.URL == "" || cfg.APIKey == "" || cfg.APISecret == "" {
		return nil, fmt.Errorf("LiveKit URL, API key, and API secret are required")
	}

	roomClient := lksdk.NewRoomServiceClient(cfg.URL, cfg.APIKey, cfg.APISecret)

	return &Service{
		roomClient: roomClient,
		apiKey:     cfg.APIKey,
		apiSecret:  cfg.APISecret,
		pool:       cfg.Pool,
		logger:     cfg.Logger,
		states:     make(map[string]*VoiceState),
	}, nil
}

// GenerateToken creates a LiveKit access token for a user joining a voice channel.
// The token grants permission to publish/subscribe audio and optionally video.
// metadata is a JSON string embedded in the token for participant display info.
func (s *Service) GenerateToken(userID, channelID string, canPublish, canSubscribe, canVideo bool, metadata string) (string, error) {
	at := auth.NewAccessToken(s.apiKey, s.apiSecret)
	grant := &auth.VideoGrant{
		RoomJoin: true,
		Room:     channelID, // use channel ID as room name
	}

	// Set publish/subscribe permissions based on guild permissions.
	grant.CanPublish = &canPublish
	grant.CanSubscribe = &canSubscribe
	grant.CanPublishData = &canPublish

	at.SetVideoGrant(grant).
		SetIdentity(userID).
		SetValidFor(24 * time.Hour)

	if metadata != "" {
		at.SetMetadata(metadata)
	}

	token, err := at.ToJWT()
	if err != nil {
		return "", fmt.Errorf("generating LiveKit token: %w", err)
	}

	return token, nil
}

// EnsureRoom creates a LiveKit room for a voice channel if it doesn't exist.
func (s *Service) EnsureRoom(ctx context.Context, channelID string) error {
	_, err := s.roomClient.CreateRoom(ctx, &livekit.CreateRoomRequest{
		Name:            channelID,
		EmptyTimeout:    300, // 5 minutes after last participant leaves
		MaxParticipants: 100,
	})
	if err != nil {
		// Room may already exist — that's fine.
		s.logger.Debug("room create (may already exist)",
			slog.String("channel_id", channelID),
			slog.String("error", err.Error()),
		)
	}
	return nil
}

// DeleteRoom removes a LiveKit room when a voice channel is deleted.
func (s *Service) DeleteRoom(ctx context.Context, channelID string) error {
	_, err := s.roomClient.DeleteRoom(ctx, &livekit.DeleteRoomRequest{
		Room: channelID,
	})
	return err
}

// ListParticipants returns current participants in a voice channel.
func (s *Service) ListParticipants(ctx context.Context, channelID string) ([]*livekit.ParticipantInfo, error) {
	resp, err := s.roomClient.ListParticipants(ctx, &livekit.ListParticipantsRequest{
		Room: channelID,
	})
	if err != nil {
		return nil, fmt.Errorf("listing participants: %w", err)
	}
	return resp.Participants, nil
}

// MuteParticipant server-mutes a participant in a voice channel.
// Preserves the current deafen state to avoid conflicting permission updates.
func (s *Service) MuteParticipant(ctx context.Context, channelID, userID string, muted bool) error {
	// Check current deafen state so we don't accidentally undo it.
	s.statesMu.RLock()
	deafened := false
	if vs, ok := s.states[userID]; ok {
		deafened = vs.Deafened
	}
	s.statesMu.RUnlock()

	_, err := s.roomClient.UpdateParticipant(ctx, &livekit.UpdateParticipantRequest{
		Room:     channelID,
		Identity: userID,
		Permission: &livekit.ParticipantPermission{
			CanPublish:   !muted,
			CanSubscribe: !deafened,
		},
	})
	return err
}

// DeafenParticipant server-deafens a participant in a voice channel.
// Preserves the current mute state to avoid conflicting permission updates.
func (s *Service) DeafenParticipant(ctx context.Context, channelID, userID string, deafened bool) error {
	// Check current mute state so we don't accidentally undo it.
	s.statesMu.RLock()
	muted := false
	if vs, ok := s.states[userID]; ok {
		muted = vs.Muted
	}
	s.statesMu.RUnlock()

	_, err := s.roomClient.UpdateParticipant(ctx, &livekit.UpdateParticipantRequest{
		Room:     channelID,
		Identity: userID,
		Permission: &livekit.ParticipantPermission{
			CanPublish:   !muted,
			CanSubscribe: !deafened,
		},
	})
	return err
}

// RemoveParticipant kicks a user from a voice channel.
func (s *Service) RemoveParticipant(ctx context.Context, channelID, userID string) error {
	_, err := s.roomClient.RemoveParticipant(ctx, &livekit.RoomParticipantIdentity{
		Room:     channelID,
		Identity: userID,
	})
	return err
}

// SetServerMute sets the server-side mute flag on a user's voice state.
func (s *Service) SetServerMute(userID string, muted bool) {
	s.statesMu.Lock()
	defer s.statesMu.Unlock()
	if vs, ok := s.states[userID]; ok {
		vs.Muted = muted
	}
}

// SetServerDeafen sets the server-side deafen flag on a user's voice state.
func (s *Service) SetServerDeafen(userID string, deafened bool) {
	s.statesMu.Lock()
	defer s.statesMu.Unlock()
	if vs, ok := s.states[userID]; ok {
		vs.Deafened = deafened
	}
}

// UpdateVoiceState updates the in-memory voice state for a user.
func (s *Service) UpdateVoiceState(userID, guildID, channelID string, selfMute, selfDeaf bool) {
	s.statesMu.Lock()
	defer s.statesMu.Unlock()

	if channelID == "" {
		// User is disconnecting from voice.
		delete(s.states, userID)
		return
	}

	s.states[userID] = &VoiceState{
		UserID:    userID,
		GuildID:   guildID,
		ChannelID: channelID,
		SelfMute:  selfMute,
		SelfDeaf:  selfDeaf,
	}
}

// GetVoiceState returns a user's current voice state, or nil if not connected.
func (s *Service) GetVoiceState(userID string) *VoiceState {
	s.statesMu.RLock()
	defer s.statesMu.RUnlock()
	return s.states[userID]
}

// GetChannelVoiceStates returns all voice states for a given channel.
func (s *Service) GetChannelVoiceStates(channelID string) []*VoiceState {
	s.statesMu.RLock()
	defer s.statesMu.RUnlock()

	var states []*VoiceState
	for _, vs := range s.states {
		if vs.ChannelID == channelID {
			states = append(states, vs)
		}
	}
	return states
}

// GetGuildVoiceStates returns all voice states for a given guild.
func (s *Service) GetGuildVoiceStates(guildID string) []*VoiceState {
	s.statesMu.RLock()
	defer s.statesMu.RUnlock()

	var states []*VoiceState
	for _, vs := range s.states {
		if vs.GuildID == guildID {
			states = append(states, vs)
		}
	}
	return states
}

// SetInputMode updates the input mode (VAD/PTT) on a user's voice state.
func (s *Service) SetInputMode(userID, mode string) {
	s.statesMu.Lock()
	defer s.statesMu.Unlock()
	if vs, ok := s.states[userID]; ok {
		vs.InputMode = mode
	}
}

// SetPrioritySpeaker toggles priority speaker on a user's voice state.
func (s *Service) SetPrioritySpeaker(userID string, priority bool) {
	s.statesMu.Lock()
	defer s.statesMu.Unlock()
	if vs, ok := s.states[userID]; ok {
		vs.PrioritySpeaker = priority
	}
}

// SetBroadcasting toggles the broadcasting flag on a user's voice state.
func (s *Service) SetBroadcasting(userID string, broadcasting bool) {
	s.statesMu.Lock()
	defer s.statesMu.Unlock()
	if vs, ok := s.states[userID]; ok {
		vs.Broadcasting = broadcasting
	}
}

// SetScreenSharing toggles the screen sharing flag on a user's voice state.
func (s *Service) SetScreenSharing(userID string, sharing bool) {
	s.statesMu.Lock()
	defer s.statesMu.Unlock()
	if vs, ok := s.states[userID]; ok {
		vs.ScreenSharing = sharing
	}
}

// GetVoicePreferences loads a user's voice preferences from the database.
func (s *Service) GetVoicePreferences(ctx context.Context, userID string) (*VoicePreferences, error) {
	prefs := &VoicePreferences{
		UserID:           userID,
		InputMode:        "vad",
		PTTKey:           "Space",
		VADThreshold:     0.3,
		NoiseSuppression: true,
		EchoCancellation: true,
		AutoGainControl:  true,
		InputVolume:      1.0,
		OutputVolume:     1.0,
	}

	err := s.pool.QueryRow(ctx,
		`SELECT input_mode, ptt_key, vad_threshold, noise_suppression,
		        echo_cancellation, auto_gain_control, input_volume, output_volume
		 FROM voice_preferences WHERE user_id = $1`, userID,
	).Scan(&prefs.InputMode, &prefs.PTTKey, &prefs.VADThreshold,
		&prefs.NoiseSuppression, &prefs.EchoCancellation, &prefs.AutoGainControl,
		&prefs.InputVolume, &prefs.OutputVolume)

	if err != nil {
		// Return defaults if no row exists.
		return prefs, nil
	}
	return prefs, nil
}

// UpdateVoicePreferences upserts a user's voice preferences.
func (s *Service) UpdateVoicePreferences(ctx context.Context, prefs *VoicePreferences) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO voice_preferences (user_id, input_mode, ptt_key, vad_threshold,
		    noise_suppression, echo_cancellation, auto_gain_control, input_volume, output_volume, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, now())
		 ON CONFLICT (user_id) DO UPDATE SET
		    input_mode = EXCLUDED.input_mode,
		    ptt_key = EXCLUDED.ptt_key,
		    vad_threshold = EXCLUDED.vad_threshold,
		    noise_suppression = EXCLUDED.noise_suppression,
		    echo_cancellation = EXCLUDED.echo_cancellation,
		    auto_gain_control = EXCLUDED.auto_gain_control,
		    input_volume = EXCLUDED.input_volume,
		    output_volume = EXCLUDED.output_volume,
		    updated_at = now()`,
		prefs.UserID, prefs.InputMode, prefs.PTTKey, prefs.VADThreshold,
		prefs.NoiseSuppression, prefs.EchoCancellation, prefs.AutoGainControl,
		prefs.InputVolume, prefs.OutputVolume,
	)
	if err != nil {
		return fmt.Errorf("upserting voice preferences: %w", err)
	}
	return nil
}

// GetSoundboardConfig loads the soundboard configuration for a guild.
func (s *Service) GetSoundboardConfig(ctx context.Context, guildID string) (*SoundboardConfig, error) {
	cfg := &SoundboardConfig{
		GuildID:         guildID,
		Enabled:         true,
		MaxSounds:       8,
		CooldownSeconds: 5,
		AllowExternal:   false,
	}

	err := s.pool.QueryRow(ctx,
		`SELECT enabled, max_sounds, cooldown_seconds, allow_external
		 FROM soundboard_config WHERE guild_id = $1`, guildID,
	).Scan(&cfg.Enabled, &cfg.MaxSounds, &cfg.CooldownSeconds, &cfg.AllowExternal)

	if err != nil {
		// Return defaults if no row exists.
		return cfg, nil
	}
	return cfg, nil
}

// UpdateSoundboardConfig upserts the soundboard configuration for a guild.
func (s *Service) UpdateSoundboardConfig(ctx context.Context, cfg *SoundboardConfig) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO soundboard_config (guild_id, enabled, max_sounds, cooldown_seconds, allow_external, updated_at)
		 VALUES ($1, $2, $3, $4, $5, now())
		 ON CONFLICT (guild_id) DO UPDATE SET
		    enabled = EXCLUDED.enabled,
		    max_sounds = EXCLUDED.max_sounds,
		    cooldown_seconds = EXCLUDED.cooldown_seconds,
		    allow_external = EXCLUDED.allow_external,
		    updated_at = now()`,
		cfg.GuildID, cfg.Enabled, cfg.MaxSounds, cfg.CooldownSeconds, cfg.AllowExternal,
	)
	if err != nil {
		return fmt.Errorf("upserting soundboard config: %w", err)
	}
	return nil
}

// GetSoundboardSounds lists all sounds for a guild.
func (s *Service) GetSoundboardSounds(ctx context.Context, guildID string) ([]SoundboardSound, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, guild_id, name, file_url, volume, duration_ms, emoji, creator_id, play_count, created_at
		 FROM soundboard_sounds WHERE guild_id = $1
		 ORDER BY name ASC`, guildID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying soundboard sounds: %w", err)
	}
	defer rows.Close()

	var sounds []SoundboardSound
	for rows.Next() {
		var sound SoundboardSound
		if err := rows.Scan(&sound.ID, &sound.GuildID, &sound.Name, &sound.FileURL,
			&sound.Volume, &sound.DurationMs, &sound.Emoji, &sound.CreatorID,
			&sound.PlayCount, &sound.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning soundboard sound: %w", err)
		}
		sounds = append(sounds, sound)
	}
	return sounds, nil
}

// GetSoundboardSound loads a single soundboard sound by ID.
func (s *Service) GetSoundboardSound(ctx context.Context, soundID string) (*SoundboardSound, error) {
	var sound SoundboardSound
	err := s.pool.QueryRow(ctx,
		`SELECT id, guild_id, name, file_url, volume, duration_ms, emoji, creator_id, play_count, created_at
		 FROM soundboard_sounds WHERE id = $1`, soundID,
	).Scan(&sound.ID, &sound.GuildID, &sound.Name, &sound.FileURL,
		&sound.Volume, &sound.DurationMs, &sound.Emoji, &sound.CreatorID,
		&sound.PlayCount, &sound.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("querying soundboard sound: %w", err)
	}
	return &sound, nil
}

// CreateSoundboardSound inserts a new soundboard sound.
func (s *Service) CreateSoundboardSound(ctx context.Context, sound *SoundboardSound) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO soundboard_sounds (id, guild_id, name, file_url, volume, duration_ms, emoji, creator_id)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		sound.ID, sound.GuildID, sound.Name, sound.FileURL,
		sound.Volume, sound.DurationMs, sound.Emoji, sound.CreatorID,
	)
	if err != nil {
		return fmt.Errorf("inserting soundboard sound: %w", err)
	}
	return nil
}

// DeleteSoundboardSound removes a soundboard sound by ID.
func (s *Service) DeleteSoundboardSound(ctx context.Context, soundID string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM soundboard_sounds WHERE id = $1`, soundID)
	if err != nil {
		return fmt.Errorf("deleting soundboard sound: %w", err)
	}
	return nil
}

// CountSoundboardSounds returns the number of sounds in a guild.
func (s *Service) CountSoundboardSounds(ctx context.Context, guildID string) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM soundboard_sounds WHERE guild_id = $1`, guildID,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting soundboard sounds: %w", err)
	}
	return count, nil
}

// IncrementSoundPlayCount bumps the play count for a sound.
func (s *Service) IncrementSoundPlayCount(ctx context.Context, soundID string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE soundboard_sounds SET play_count = play_count + 1 WHERE id = $1`, soundID)
	return err
}

// CheckSoundboardCooldown returns true if the user is still on cooldown.
func (s *Service) CheckSoundboardCooldown(ctx context.Context, userID, guildID string, cooldownSeconds int) (bool, error) {
	var count int
	err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM soundboard_play_log
		 WHERE user_id = $1 AND guild_id = $2 AND played_at > now() - ($3 || ' seconds')::interval`,
		userID, guildID, fmt.Sprintf("%d", cooldownSeconds),
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("checking soundboard cooldown: %w", err)
	}
	return count > 0, nil
}

// LogSoundboardPlay records a soundboard play event for cooldown tracking.
func (s *Service) LogSoundboardPlay(ctx context.Context, logID, soundID, guildID, channelID, userID string) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO soundboard_play_log (id, sound_id, guild_id, channel_id, user_id)
		 VALUES ($1, $2, $3, $4, $5)`,
		logID, soundID, guildID, channelID, userID,
	)
	return err
}

// CreateBroadcast inserts a new voice broadcast record.
func (s *Service) CreateBroadcast(ctx context.Context, broadcast *VoiceBroadcast) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO voice_broadcasts (id, guild_id, channel_id, broadcaster_id, title)
		 VALUES ($1, $2, $3, $4, $5)`,
		broadcast.ID, broadcast.GuildID, broadcast.ChannelID,
		broadcast.BroadcasterID, broadcast.Title,
	)
	if err != nil {
		return fmt.Errorf("inserting voice broadcast: %w", err)
	}
	return nil
}

// EndBroadcast marks a broadcast as ended.
func (s *Service) EndBroadcast(ctx context.Context, broadcastID string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE voice_broadcasts SET ended_at = now() WHERE id = $1`, broadcastID)
	return err
}

// GetActiveBroadcast returns the active broadcast for a channel, or nil if none.
func (s *Service) GetActiveBroadcast(ctx context.Context, channelID string) (*VoiceBroadcast, error) {
	var b VoiceBroadcast
	err := s.pool.QueryRow(ctx,
		`SELECT id, guild_id, channel_id, broadcaster_id, title, started_at, ended_at, listener_count
		 FROM voice_broadcasts WHERE channel_id = $1 AND ended_at IS NULL
		 ORDER BY started_at DESC LIMIT 1`, channelID,
	).Scan(&b.ID, &b.GuildID, &b.ChannelID, &b.BroadcasterID,
		&b.Title, &b.StartedAt, &b.EndedAt, &b.ListenerCount)
	if err != nil {
		return nil, err
	}
	return &b, nil
}

// UpdateBroadcastListeners updates the listener count for an active broadcast.
func (s *Service) UpdateBroadcastListeners(ctx context.Context, broadcastID string, count int) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE voice_broadcasts SET listener_count = $1 WHERE id = $2`, count, broadcastID)
	return err
}

// CreateScreenShareSession inserts a new screen share session record.
func (s *Service) CreateScreenShareSession(ctx context.Context, session *ScreenShareSession) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO screen_share_sessions (id, channel_id, user_id, share_type, resolution, framerate, audio_enabled, max_viewers)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		session.ID, session.ChannelID, session.UserID, session.ShareType,
		session.Resolution, session.Framerate, session.AudioEnabled, session.MaxViewers,
	)
	if err != nil {
		return fmt.Errorf("inserting screen share session: %w", err)
	}
	return nil
}

// EndScreenShareSession marks a screen share session as ended.
func (s *Service) EndScreenShareSession(ctx context.Context, sessionID string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE screen_share_sessions SET ended_at = now() WHERE id = $1`, sessionID)
	return err
}

// GetActiveScreenShare returns the active screen share for a user in a channel.
func (s *Service) GetActiveScreenShare(ctx context.Context, channelID, userID string) (*ScreenShareSession, error) {
	var ss ScreenShareSession
	err := s.pool.QueryRow(ctx,
		`SELECT id, channel_id, user_id, share_type, resolution, framerate, audio_enabled, max_viewers, started_at, ended_at
		 FROM screen_share_sessions
		 WHERE channel_id = $1 AND user_id = $2 AND ended_at IS NULL
		 ORDER BY started_at DESC LIMIT 1`, channelID, userID,
	).Scan(&ss.ID, &ss.ChannelID, &ss.UserID, &ss.ShareType,
		&ss.Resolution, &ss.Framerate, &ss.AudioEnabled, &ss.MaxViewers,
		&ss.StartedAt, &ss.EndedAt)
	if err != nil {
		return nil, err
	}
	return &ss, nil
}

// GetChannelScreenShares returns all active screen shares in a channel.
func (s *Service) GetChannelScreenShares(ctx context.Context, channelID string) ([]ScreenShareSession, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, channel_id, user_id, share_type, resolution, framerate, audio_enabled, max_viewers, started_at, ended_at
		 FROM screen_share_sessions WHERE channel_id = $1 AND ended_at IS NULL
		 ORDER BY started_at ASC`, channelID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying screen shares: %w", err)
	}
	defer rows.Close()

	var sessions []ScreenShareSession
	for rows.Next() {
		var ss ScreenShareSession
		if err := rows.Scan(&ss.ID, &ss.ChannelID, &ss.UserID, &ss.ShareType,
			&ss.Resolution, &ss.Framerate, &ss.AudioEnabled, &ss.MaxViewers,
			&ss.StartedAt, &ss.EndedAt); err != nil {
			return nil, fmt.Errorf("scanning screen share session: %w", err)
		}
		sessions = append(sessions, ss)
	}
	return sessions, nil
}
