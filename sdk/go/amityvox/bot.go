package amityvox

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/coder/websocket"
)

// EventHandler is a function that handles a gateway event.
type EventHandler func(eventType string, data json.RawMessage)

// MessageHandler handles MESSAGE_CREATE events.
type MessageHandler func(msg *Message)

// MemberHandler handles GUILD_MEMBER_ADD and GUILD_MEMBER_REMOVE events.
type MemberHandler func(event *GuildMemberEvent)

// ReactionHandler handles MESSAGE_REACTION_ADD and MESSAGE_REACTION_REMOVE events.
type ReactionHandler func(event *ReactionEvent)

// ReadyHandler handles the READY event.
type ReadyHandler func(event *ReadyEvent)

// Bot is the main entry point for building an AmityVox bot. It wraps the REST
// API client and WebSocket gateway connection, providing a simple event-driven
// interface for bot development.
type Bot struct {
	client *Client
	logger *slog.Logger

	// Event handlers.
	mu               sync.RWMutex
	rawHandlers      []EventHandler
	messageHandlers  []MessageHandler
	memberJoinH      []MemberHandler
	memberLeaveH     []MemberHandler
	reactionAddH     []ReactionHandler
	reactionRemoveH  []ReactionHandler
	readyHandlers    []ReadyHandler

	// Gateway state.
	conn       *websocket.Conn
	sessionID  string
	sequence   int64
	user       *User
	heartbeatI time.Duration
	done       chan struct{}
	cancel     context.CancelFunc
}

// NewBot creates a new bot instance with the given token and instance URL.
func NewBot(token, baseURL string, opts ...ClientOption) *Bot {
	return &Bot{
		client: NewClient(token, baseURL, opts...),
		logger: slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		})),
		done: make(chan struct{}),
	}
}

// SetLogger replaces the default logger.
func (b *Bot) SetLogger(l *slog.Logger) {
	b.logger = l
}

// Client returns the REST API client for making direct API calls.
func (b *Bot) Client() *Client {
	return b.client
}

// User returns the bot's user object (available after READY).
func (b *Bot) User() *User {
	return b.user
}

// SessionID returns the current gateway session ID.
func (b *Bot) SessionID() string {
	return b.sessionID
}

// --- Event Registration ---

// On registers a raw event handler that receives all gateway events.
func (b *Bot) On(handler EventHandler) {
	b.mu.Lock()
	b.rawHandlers = append(b.rawHandlers, handler)
	b.mu.Unlock()
}

// OnMessage registers a handler for MESSAGE_CREATE events.
func (b *Bot) OnMessage(handler MessageHandler) {
	b.mu.Lock()
	b.messageHandlers = append(b.messageHandlers, handler)
	b.mu.Unlock()
}

// OnReady registers a handler for the READY event.
func (b *Bot) OnReady(handler ReadyHandler) {
	b.mu.Lock()
	b.readyHandlers = append(b.readyHandlers, handler)
	b.mu.Unlock()
}

// OnMemberJoin registers a handler for GUILD_MEMBER_ADD events.
func (b *Bot) OnMemberJoin(handler MemberHandler) {
	b.mu.Lock()
	b.memberJoinH = append(b.memberJoinH, handler)
	b.mu.Unlock()
}

// OnMemberLeave registers a handler for GUILD_MEMBER_REMOVE events.
func (b *Bot) OnMemberLeave(handler MemberHandler) {
	b.mu.Lock()
	b.memberLeaveH = append(b.memberLeaveH, handler)
	b.mu.Unlock()
}

// OnReactionAdd registers a handler for MESSAGE_REACTION_ADD events.
func (b *Bot) OnReactionAdd(handler ReactionHandler) {
	b.mu.Lock()
	b.reactionAddH = append(b.reactionAddH, handler)
	b.mu.Unlock()
}

// OnReactionRemove registers a handler for MESSAGE_REACTION_REMOVE events.
func (b *Bot) OnReactionRemove(handler ReactionHandler) {
	b.mu.Lock()
	b.reactionRemoveH = append(b.reactionRemoveH, handler)
	b.mu.Unlock()
}

// --- Lifecycle ---

// Start connects to the WebSocket gateway and begins processing events.
// It blocks until the bot is stopped (via Stop() or OS signal).
func (b *Bot) Start() error {
	ctx, cancel := context.WithCancel(context.Background())
	b.cancel = cancel

	// Connect to gateway.
	if err := b.connect(ctx); err != nil {
		cancel()
		return fmt.Errorf("connecting to gateway: %w", err)
	}

	// Handle OS signals for graceful shutdown.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigCh:
		b.logger.Info("received signal, shutting down", slog.String("signal", sig.String()))
	case <-b.done:
		b.logger.Info("bot stopped")
	}

	cancel()
	b.disconnect()
	return nil
}

// Stop signals the bot to shut down gracefully.
func (b *Bot) Stop() {
	if b.cancel != nil {
		b.cancel()
	}
	select {
	case <-b.done:
	default:
		close(b.done)
	}
}

// connect establishes the WebSocket connection and sends the Identify message.
func (b *Bot) connect(ctx context.Context) error {
	// Build WebSocket URL from base URL.
	wsURL := strings.Replace(b.client.baseURL, "https://", "wss://", 1)
	wsURL = strings.Replace(wsURL, "http://", "ws://", 1)
	wsURL += "/api/v1/gateway"

	b.logger.Info("connecting to gateway", slog.String("url", wsURL))

	conn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		HTTPHeader: http.Header{
			"User-Agent": []string{b.client.userAgent},
		},
	})
	if err != nil {
		return fmt.Errorf("dialing gateway: %w", err)
	}

	// Set a generous read limit for large events.
	conn.SetReadLimit(1 << 20) // 1MB
	b.conn = conn

	// Read the Hello message.
	_, helloData, err := conn.Read(ctx)
	if err != nil {
		conn.Close(websocket.StatusInternalError, "failed to read hello")
		return fmt.Errorf("reading hello: %w", err)
	}

	var hello GatewayMessage
	if err := json.Unmarshal(helloData, &hello); err != nil {
		conn.Close(websocket.StatusInternalError, "invalid hello")
		return fmt.Errorf("parsing hello: %w", err)
	}

	if hello.Op == OpHello {
		var helloPayload struct {
			HeartbeatInterval int `json:"heartbeat_interval"`
		}
		if err := json.Unmarshal(hello.Data, &helloPayload); err == nil {
			b.heartbeatI = time.Duration(helloPayload.HeartbeatInterval) * time.Millisecond
		}
	}

	if b.heartbeatI == 0 {
		b.heartbeatI = 30 * time.Second
	}

	// Send Identify.
	identify := GatewayMessage{
		Op: OpIdentify,
		Data: mustMarshal(map[string]interface{}{
			"token": b.client.token,
		}),
	}
	if err := b.send(ctx, identify); err != nil {
		conn.Close(websocket.StatusInternalError, "identify failed")
		return fmt.Errorf("sending identify: %w", err)
	}

	// Start the read loop and heartbeat loop.
	go b.readLoop(ctx)
	go b.heartbeatLoop(ctx)

	b.logger.Info("gateway connected")
	return nil
}

// send writes a message to the WebSocket connection.
func (b *Bot) send(ctx context.Context, msg GatewayMessage) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshaling gateway message: %w", err)
	}
	return b.conn.Write(ctx, websocket.MessageText, data)
}

// readLoop continuously reads messages from the WebSocket and dispatches them.
func (b *Bot) readLoop(ctx context.Context) {
	defer func() {
		select {
		case <-b.done:
		default:
			close(b.done)
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		_, data, err := b.conn.Read(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return // Context cancelled, clean shutdown.
			}
			b.logger.Error("gateway read error", slog.String("error", err.Error()))
			return
		}

		var msg GatewayMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			b.logger.Error("invalid gateway message", slog.String("error", err.Error()))
			continue
		}

		if msg.Sequence > 0 {
			b.sequence = msg.Sequence
		}

		switch msg.Op {
		case OpDispatch:
			b.handleDispatch(msg.Type, msg.Data)
		case OpHeartbeatAck:
			// Expected response to our heartbeats.
		case OpReconnect:
			b.logger.Info("gateway requested reconnect")
			return
		}
	}
}

// heartbeatLoop sends periodic heartbeats to keep the connection alive.
func (b *Bot) heartbeatLoop(ctx context.Context) {
	ticker := time.NewTicker(b.heartbeatI)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-b.done:
			return
		case <-ticker.C:
			hb := GatewayMessage{
				Op:   OpHeartbeat,
				Data: mustMarshal(b.sequence),
			}
			if err := b.send(ctx, hb); err != nil {
				b.logger.Error("failed to send heartbeat", slog.String("error", err.Error()))
				return
			}
		}
	}
}

// handleDispatch routes dispatch events to registered handlers.
func (b *Bot) handleDispatch(eventType string, data json.RawMessage) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	// Dispatch to raw handlers.
	for _, h := range b.rawHandlers {
		go h(eventType, data)
	}

	switch eventType {
	case EventReady:
		var ready ReadyEvent
		if err := json.Unmarshal(data, &ready); err != nil {
			b.logger.Error("failed to parse READY", slog.String("error", err.Error()))
			return
		}
		b.user = &ready.User
		b.sessionID = ready.SessionID
		b.logger.Info("bot ready",
			slog.String("user", ready.User.Username),
			slog.Int("guilds", len(ready.GuildIDs)))
		for _, h := range b.readyHandlers {
			go h(&ready)
		}

	case EventMessageCreate:
		var msg Message
		if err := json.Unmarshal(data, &msg); err != nil {
			b.logger.Error("failed to parse MESSAGE_CREATE", slog.String("error", err.Error()))
			return
		}
		// Skip messages from the bot itself to prevent loops.
		if b.user != nil && msg.AuthorID == b.user.ID {
			return
		}
		for _, h := range b.messageHandlers {
			go h(&msg)
		}

	case EventGuildMemberAdd:
		var event GuildMemberEvent
		if err := json.Unmarshal(data, &event); err == nil {
			for _, h := range b.memberJoinH {
				go h(&event)
			}
		}

	case EventGuildMemberRemove:
		var event GuildMemberEvent
		if err := json.Unmarshal(data, &event); err == nil {
			for _, h := range b.memberLeaveH {
				go h(&event)
			}
		}

	case EventMessageReactionAdd:
		var event ReactionEvent
		if err := json.Unmarshal(data, &event); err == nil {
			for _, h := range b.reactionAddH {
				go h(&event)
			}
		}

	case EventMessageReactionDel:
		var event ReactionEvent
		if err := json.Unmarshal(data, &event); err == nil {
			for _, h := range b.reactionRemoveH {
				go h(&event)
			}
		}
	}
}

// disconnect closes the WebSocket connection.
func (b *Bot) disconnect() {
	if b.conn != nil {
		b.conn.Close(websocket.StatusNormalClosure, "bot shutting down")
		b.conn = nil
	}
}

// --- Convenience Methods ---

// Reply sends a reply to a message in the same channel.
func (b *Bot) Reply(ctx context.Context, msg *Message, content string) (*Message, error) {
	return b.client.SendMessage(ctx, msg.ChannelID, content)
}

// UpdatePresence sends a presence update through the gateway.
func (b *Bot) UpdatePresence(ctx context.Context, status string) error {
	if b.conn == nil {
		return fmt.Errorf("not connected to gateway")
	}
	return b.send(ctx, GatewayMessage{
		Op: OpPresenceUpdate,
		Data: mustMarshal(map[string]string{
			"status": status,
		}),
	})
}

// mustMarshal marshals a value to JSON or panics. Used for known-good values.
func mustMarshal(v interface{}) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("failed to marshal: %v", err))
	}
	return b
}
