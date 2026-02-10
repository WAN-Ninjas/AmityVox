package voice

import (
	"testing"
)

func TestVoiceStateTracking(t *testing.T) {
	s := &Service{
		states: make(map[string]*VoiceState),
	}

	// Initially empty.
	if vs := s.GetVoiceState("user1"); vs != nil {
		t.Fatalf("expected nil voice state, got %+v", vs)
	}

	// Join voice.
	s.UpdateVoiceState("user1", "guild1", "channel1", false, false)
	vs := s.GetVoiceState("user1")
	if vs == nil {
		t.Fatal("expected voice state, got nil")
	}
	if vs.ChannelID != "channel1" || vs.GuildID != "guild1" {
		t.Fatalf("unexpected voice state: %+v", vs)
	}

	// Update mute.
	s.UpdateVoiceState("user1", "guild1", "channel1", true, false)
	vs = s.GetVoiceState("user1")
	if !vs.SelfMute {
		t.Fatal("expected self_mute true")
	}

	// Second user joins same channel.
	s.UpdateVoiceState("user2", "guild1", "channel1", false, false)

	// Get channel states.
	states := s.GetChannelVoiceStates("channel1")
	if len(states) != 2 {
		t.Fatalf("expected 2 channel states, got %d", len(states))
	}

	// Get guild states.
	guildStates := s.GetGuildVoiceStates("guild1")
	if len(guildStates) != 2 {
		t.Fatalf("expected 2 guild states, got %d", len(guildStates))
	}

	// Disconnect user1.
	s.UpdateVoiceState("user1", "guild1", "", false, false)
	if vs := s.GetVoiceState("user1"); vs != nil {
		t.Fatalf("expected nil after disconnect, got %+v", vs)
	}

	states = s.GetChannelVoiceStates("channel1")
	if len(states) != 1 {
		t.Fatalf("expected 1 channel state after disconnect, got %d", len(states))
	}
}

func TestGenerateTokenRequiresService(t *testing.T) {
	// Service without LiveKit config fails to create.
	_, err := New(Config{})
	if err == nil {
		t.Fatal("expected error creating service without config")
	}
}
