// Package events implements the internal event bus using NATS pub/sub. REST API
// handlers publish events to NATS subjects, and the WebSocket gateway subscribes
// to dispatch real-time updates to connected clients. NATS JetStream provides
// persistent streams for federation message queuing and reliable delivery.
package events

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go"
)

// Subject constants define the NATS subject hierarchy for all event types.
// Subjects follow the pattern: amityvox.<category>.<action>
const (
	// Message events.
	SubjectMessageCreate       = "amityvox.message.create"
	SubjectMessageUpdate       = "amityvox.message.update"
	SubjectMessageDelete       = "amityvox.message.delete"
	SubjectMessageDeleteBulk   = "amityvox.message.delete_bulk"
	SubjectMessageReactionAdd  = "amityvox.message.reaction_add"
	SubjectMessageReactionDel  = "amityvox.message.reaction_remove"
	SubjectMessageReactionClr  = "amityvox.message.reaction_clear"
	SubjectMessageEmbedUpdate  = "amityvox.message.embed_update"

	// Channel events.
	SubjectChannelCreate     = "amityvox.channel.create"
	SubjectChannelUpdate     = "amityvox.channel.update"
	SubjectChannelDelete     = "amityvox.channel.delete"
	SubjectChannelPinsUpdate = "amityvox.channel.pins_update"
	SubjectTypingStart       = "amityvox.channel.typing_start"

	// Guild events.
	SubjectGuildCreate       = "amityvox.guild.create"
	SubjectGuildUpdate       = "amityvox.guild.update"
	SubjectGuildDelete       = "amityvox.guild.delete"
	SubjectGuildMemberAdd    = "amityvox.guild.member_add"
	SubjectGuildMemberUpdate = "amityvox.guild.member_update"
	SubjectGuildMemberRemove = "amityvox.guild.member_remove"
	SubjectGuildRoleCreate   = "amityvox.guild.role_create"
	SubjectGuildRoleUpdate   = "amityvox.guild.role_update"
	SubjectGuildRoleDelete   = "amityvox.guild.role_delete"
	SubjectGuildBanAdd       = "amityvox.guild.ban_add"
	SubjectGuildBanRemove    = "amityvox.guild.ban_remove"
	SubjectGuildEmojiUpdate  = "amityvox.guild.emoji_update"

	// Guild channel group events.
	SubjectChannelGroupCreate      = "amityvox.guild.channel_group_create"
	SubjectChannelGroupUpdate      = "amityvox.guild.channel_group_update"
	SubjectChannelGroupDelete      = "amityvox.guild.channel_group_delete"
	SubjectChannelGroupItemsUpdate = "amityvox.guild.channel_group_items_update"

	// User/presence events.
	SubjectPresenceUpdate      = "amityvox.presence.update"
	SubjectUserUpdate          = "amityvox.user.update"
	SubjectRelationshipAdd     = "amityvox.user.relationship_add"
	SubjectRelationshipUpdate  = "amityvox.user.relationship_update"
	SubjectRelationshipRemove  = "amityvox.user.relationship_remove"

	// Voice events.
	SubjectVoiceStateUpdate  = "amityvox.voice.state_update"
	SubjectVoiceServerUpdate = "amityvox.voice.server_update"
	SubjectCallRing          = "amityvox.voice.call_ring"

	// Read state events.
	SubjectChannelAck = "amityvox.channel.ack"

	// AutoMod events.
	SubjectAutomodAction = "amityvox.automod.action"

	// Poll events.
	SubjectPollCreate = "amityvox.poll.create"
	SubjectPollVote   = "amityvox.poll.vote"
	SubjectPollClose  = "amityvox.poll.close"

	// Guild event events.
	SubjectGuildEventCreate = "amityvox.guild.event_create"
	SubjectGuildEventUpdate = "amityvox.guild.event_update"
	SubjectGuildEventDelete = "amityvox.guild.event_delete"

	// Moderation events.
	SubjectRaidLockdown = "amityvox.guild.raid_lockdown"

	// Announcement events (instance-wide, broadcast to all clients).
	SubjectAnnouncementCreate = "amityvox.announcement.create"
	SubjectAnnouncementUpdate = "amityvox.announcement.update"
	SubjectAnnouncementDelete = "amityvox.announcement.delete"

	// Notification events (server-generated, dispatched to specific users).
	SubjectNotificationCreate = "amityvox.notification.create"
	SubjectNotificationUpdate = "amityvox.notification.update"
	SubjectNotificationDelete = "amityvox.notification.delete"

	// Federation events.
	SubjectFederationRetry = "amityvox.federation.retry"
)

// Event is the envelope for all events published through NATS. It mirrors the
// WebSocket gateway dispatch format so events can be forwarded to clients with
// minimal transformation.
type Event struct {
	Type      string          `json:"t"`
	GuildID   string          `json:"guild_id,omitempty"`
	ChannelID string          `json:"channel_id,omitempty"`
	UserID    string          `json:"user_id,omitempty"`
	Data      json.RawMessage `json:"d"`
}

// Bus wraps a NATS connection and provides publish/subscribe methods for the
// AmityVox event system. It is the central nervous system connecting REST
// handlers, the WebSocket gateway, and background workers.
type Bus struct {
	conn   *nats.Conn
	js     nats.JetStreamContext
	logger *slog.Logger
}

// New connects to the NATS server at the given URL and returns an event Bus.
// It also initializes JetStream for persistent stream support.
func New(natsURL string, logger *slog.Logger) (*Bus, error) {
	opts := []nats.Option{
		nats.Name("amityvox"),
		nats.ReconnectWait(2 * time.Second),
		nats.MaxReconnects(60),
		nats.DisconnectErrHandler(func(_ *nats.Conn, err error) {
			if err != nil {
				logger.Warn("NATS disconnected", slog.String("error", err.Error()))
			}
		}),
		nats.ReconnectHandler(func(nc *nats.Conn) {
			logger.Info("NATS reconnected", slog.String("url", nc.ConnectedUrl()))
		}),
		nats.ErrorHandler(func(_ *nats.Conn, _ *nats.Subscription, err error) {
			logger.Error("NATS error", slog.String("error", err.Error()))
		}),
	}

	nc, err := nats.Connect(natsURL, opts...)
	if err != nil {
		return nil, fmt.Errorf("connecting to NATS at %s: %w", natsURL, err)
	}

	js, err := nc.JetStream()
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("initializing JetStream: %w", err)
	}

	logger.Info("NATS connection established", slog.String("url", nc.ConnectedUrl()))

	return &Bus{conn: nc, js: js, logger: logger}, nil
}

// EnsureStreams creates the JetStream streams required by AmityVox if they don't
// already exist. Call this during server startup.
func (b *Bus) EnsureStreams() error {
	streams := []nats.StreamConfig{
		{
			Name: "AMITYVOX_EVENTS",
			Subjects: []string{
				"amityvox.message.>",
				"amityvox.channel.>",
				"amityvox.guild.>",
				"amityvox.presence.>",
				"amityvox.user.>",
				"amityvox.voice.>",
				"amityvox.automod.>",
				"amityvox.poll.>",
				"amityvox.announcement.>",
				"amityvox.notification.>",
			},
			Retention: nats.LimitsPolicy,
			MaxAge:    24 * time.Hour,
			Storage:   nats.FileStorage,
			Replicas:  1,
		},
		{
			Name:      "AMITYVOX_FEDERATION",
			Subjects:  []string{"amityvox.federation.>"},
			Retention: nats.WorkQueuePolicy,
			MaxAge:    7 * 24 * time.Hour,
			Storage:   nats.FileStorage,
			Replicas:  1,
		},
	}

	for _, cfg := range streams {
		info, err := b.js.StreamInfo(cfg.Name)
		if err != nil && err != nats.ErrStreamNotFound {
			return fmt.Errorf("checking stream %s: %w", cfg.Name, err)
		}
		if info == nil {
			_, err := b.js.AddStream(&cfg)
			if err != nil {
				return fmt.Errorf("creating stream %s: %w", cfg.Name, err)
			}
			b.logger.Info("JetStream stream created", slog.String("stream", cfg.Name))
		} else {
			b.logger.Debug("JetStream stream exists", slog.String("stream", cfg.Name))
		}
	}

	return nil
}

// Publish sends an event to the specified NATS subject. The event data is JSON
// encoded before publishing.
func (b *Bus) Publish(_ context.Context, subject string, event Event) error {
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshaling event for %s: %w", subject, err)
	}

	if err := b.conn.Publish(subject, data); err != nil {
		return fmt.Errorf("publishing to %s: %w", subject, err)
	}

	b.logger.Debug("event published",
		slog.String("subject", subject),
		slog.String("type", event.Type),
	)

	return nil
}

// PublishJSON is a convenience method that marshals arbitrary data into an Event
// and publishes it to the specified subject.
//
// Deprecated: Use PublishGuildEvent, PublishChannelEvent, PublishUserEvent, or
// PublishBroadcastEvent instead, which populate the routing envelope fields
// required by the gateway's shouldDispatchTo logic.
func (b *Bus) PublishJSON(ctx context.Context, subject, eventType string, data interface{}) error {
	b.logger.Warn("PublishJSON called without routing envelope — use typed publish methods instead",
		slog.String("subject", subject),
		slog.String("type", eventType),
	)
	raw, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshaling event data: %w", err)
	}

	return b.Publish(ctx, subject, Event{
		Type: eventType,
		Data: raw,
	})
}

// PublishGuildEvent publishes an event routed to all members of a guild.
func (b *Bus) PublishGuildEvent(ctx context.Context, subject, eventType, guildID string, data interface{}) error {
	raw, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshaling event data: %w", err)
	}
	return b.Publish(ctx, subject, Event{
		Type:    eventType,
		GuildID: guildID,
		Data:    raw,
	})
}

// PublishChannelEvent publishes an event routed to all viewers of a channel.
func (b *Bus) PublishChannelEvent(ctx context.Context, subject, eventType, channelID string, data interface{}) error {
	raw, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshaling event data: %w", err)
	}
	return b.Publish(ctx, subject, Event{
		Type:      eventType,
		ChannelID: channelID,
		Data:      raw,
	})
}

// PublishUserEvent publishes an event targeted at a specific user.
func (b *Bus) PublishUserEvent(ctx context.Context, subject, eventType, userID string, data interface{}) error {
	raw, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshaling event data: %w", err)
	}
	return b.Publish(ctx, subject, Event{
		Type:   eventType,
		UserID: userID,
		Data:   raw,
	})
}

// PublishBroadcastEvent publishes an event to all connected clients.
func (b *Bus) PublishBroadcastEvent(ctx context.Context, subject, eventType string, data interface{}) error {
	raw, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshaling event data: %w", err)
	}
	return b.Publish(ctx, subject, Event{
		Type:    eventType,
		GuildID: "__broadcast__",
		Data:    raw,
	})
}

// Subscribe creates a subscription to the specified NATS subject. The handler
// receives decoded Event objects. Returns a Subscription that can be used to
// unsubscribe.
func (b *Bus) Subscribe(subject string, handler func(Event)) (*nats.Subscription, error) {
	sub, err := b.conn.Subscribe(subject, func(msg *nats.Msg) {
		var event Event
		if err := json.Unmarshal(msg.Data, &event); err != nil {
			b.logger.Error("failed to unmarshal event",
				slog.String("subject", subject),
				slog.String("error", err.Error()),
			)
			return
		}
		handler(event)
	})
	if err != nil {
		return nil, fmt.Errorf("subscribing to %s: %w", subject, err)
	}

	b.logger.Debug("subscribed to subject", slog.String("subject", subject))
	return sub, nil
}

// SubscribeWildcard subscribes to all events matching a wildcard pattern.
// For example, "amityvox.guild.>" matches all guild events.
func (b *Bus) SubscribeWildcard(pattern string, handler func(string, Event)) (*nats.Subscription, error) {
	sub, err := b.conn.Subscribe(pattern, func(msg *nats.Msg) {
		var event Event
		if err := json.Unmarshal(msg.Data, &event); err != nil {
			b.logger.Error("failed to unmarshal event",
				slog.String("subject", msg.Subject),
				slog.String("error", err.Error()),
			)
			return
		}
		handler(msg.Subject, event)
	})
	if err != nil {
		return nil, fmt.Errorf("subscribing to %s: %w", pattern, err)
	}

	b.logger.Debug("subscribed to pattern", slog.String("pattern", pattern))
	return sub, nil
}

// QueueSubscribe creates a queue-group subscription for load-balanced message
// processing across multiple server instances.
func (b *Bus) QueueSubscribe(subject, queue string, handler func(Event)) (*nats.Subscription, error) {
	sub, err := b.conn.QueueSubscribe(subject, queue, func(msg *nats.Msg) {
		var event Event
		if err := json.Unmarshal(msg.Data, &event); err != nil {
			b.logger.Error("failed to unmarshal event",
				slog.String("subject", subject),
				slog.String("error", err.Error()),
			)
			return
		}
		handler(event)
	})
	if err != nil {
		return nil, fmt.Errorf("queue subscribing to %s (queue: %s): %w", subject, queue, err)
	}

	b.logger.Debug("queue subscribed",
		slog.String("subject", subject),
		slog.String("queue", queue),
	)
	return sub, nil
}

// Conn returns the underlying NATS connection for advanced use cases.
func (b *Bus) Conn() *nats.Conn {
	return b.conn
}

// JetStream returns the JetStream context for stream operations.
func (b *Bus) JetStream() nats.JetStreamContext {
	return b.js
}

// HealthCheck verifies the NATS connection is alive.
func (b *Bus) HealthCheck() error {
	if !b.conn.IsConnected() {
		return fmt.Errorf("NATS connection is not active (status: %s)", b.conn.Status())
	}
	return nil
}

// Close drains pending messages and closes the NATS connection.
func (b *Bus) Close() {
	b.logger.Info("closing NATS connection")
	b.conn.Drain()
}
