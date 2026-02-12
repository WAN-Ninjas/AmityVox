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
	UserID    string `json:"user_id"`
	GuildID   string `json:"guild_id"`
	ChannelID string `json:"channel_id"`
	SelfMute  bool   `json:"self_mute"`
	SelfDeaf  bool   `json:"self_deaf"`
	Muted     bool   `json:"muted"`     // server-side mute by moderator
	Deafened  bool   `json:"deafened"`   // server-side deafen by moderator
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
func (s *Service) GenerateToken(userID, channelID string, canPublish, canSubscribe, canVideo bool) (string, error) {
	at := auth.NewAccessToken(s.apiKey, s.apiSecret)
	grant := &auth.VideoGrant{
		RoomJoin: true,
		Room:     channelID, // use channel ID as room name
	}

	// Set publish/subscribe permissions based on guild permissions.
	grant.CanPublish = &canPublish
	grant.CanSubscribe = &canSubscribe
	grant.CanPublishData = &canPublish

	at.AddGrant(grant).
		SetIdentity(userID).
		SetValidFor(24 * time.Hour)

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
