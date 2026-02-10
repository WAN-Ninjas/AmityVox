// Package gateway implements the WebSocket gateway for real-time event dispatch.
// It handles client connections, heartbeats, authentication, presence updates,
// and event broadcasting via NATS subscriptions. See docs/architecture.md Section 8
// for the full protocol specification.
package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/amityvox/amityvox/internal/auth"
	"github.com/amityvox/amityvox/internal/events"
	"github.com/amityvox/amityvox/internal/presence"
)

// Opcodes define the WebSocket protocol operation codes per Section 8.2.
const (
	OpDispatch          = 0
	OpHeartbeat         = 1
	OpIdentify          = 2
	OpPresenceUpdate    = 3
	OpVoiceStateUpdate  = 4
	OpResume            = 5
	OpReconnect         = 6
	OpRequestMembers    = 7
	OpTyping            = 8
	OpSubscribe         = 9
	OpHello             = 10
	OpHeartbeatAck      = 11
)

// GatewayMessage is the wire format for all WebSocket messages.
type GatewayMessage struct {
	Op   int              `json:"op"`
	Type string           `json:"t,omitempty"`
	Data json.RawMessage  `json:"d,omitempty"`
	Seq  *int64           `json:"s,omitempty"`
}

// IdentifyPayload is the data sent by clients in op:2 IDENTIFY.
type IdentifyPayload struct {
	Token string `json:"token"`
}

// ResumePayload is the data sent by clients in op:5 RESUME.
type ResumePayload struct {
	Token     string `json:"token"`
	SessionID string `json:"session_id"`
	Seq       int64  `json:"seq"`
}

// RequestMembersPayload is sent by clients in op:7 REQUEST_MEMBERS.
type RequestMembersPayload struct {
	GuildID string `json:"guild_id"`
	Query   string `json:"query"`
	Limit   int    `json:"limit"`
}

// HelloPayload is the data sent in op:10 HELLO.
type HelloPayload struct {
	HeartbeatInterval int64 `json:"heartbeat_interval"`
}

// Client represents a single connected WebSocket client.
type Client struct {
	conn       *websocket.Conn
	userID     string
	sessionID  string
	seq        int64
	identified bool
	guildIDs   map[string]bool // guilds this user is a member of
	mu         sync.Mutex
	done       chan struct{}
	replayBuf  []GatewayMessage // buffer for resume replay
}

// Server manages WebSocket connections and event dispatch.
type Server struct {
	authService       *auth.Service
	eventBus          *events.Bus
	cache             *presence.Cache
	pool              *pgxpool.Pool
	heartbeatInterval time.Duration
	heartbeatTimeout  time.Duration
	listenAddr        string
	logger            *slog.Logger

	clients   map[*Client]struct{}
	clientsMu sync.RWMutex

	// userClients maps userID -> set of clients for that user (multi-device).
	userClients   map[string]map[*Client]struct{}
	userClientsMu sync.RWMutex

	httpServer *http.Server
}

// ServerConfig holds the configuration for creating a gateway Server.
type ServerConfig struct {
	AuthService       *auth.Service
	EventBus          *events.Bus
	Cache             *presence.Cache
	Pool              *pgxpool.Pool
	HeartbeatInterval time.Duration
	HeartbeatTimeout  time.Duration
	ListenAddr        string
	Logger            *slog.Logger
}

// NewServer creates a new WebSocket gateway server.
func NewServer(cfg ServerConfig) *Server {
	return &Server{
		authService:       cfg.AuthService,
		eventBus:          cfg.EventBus,
		cache:             cfg.Cache,
		pool:              cfg.Pool,
		heartbeatInterval: cfg.HeartbeatInterval,
		heartbeatTimeout:  cfg.HeartbeatTimeout,
		listenAddr:        cfg.ListenAddr,
		logger:            cfg.Logger,
		clients:           make(map[*Client]struct{}),
		userClients:       make(map[string]map[*Client]struct{}),
	}
}

// Start begins listening for WebSocket connections and subscribes to NATS events
// for dispatching to connected clients.
func (s *Server) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", s.handleWebSocket)

	s.httpServer = &http.Server{
		Addr:    s.listenAddr,
		Handler: mux,
	}

	// Subscribe to all events for gateway dispatch.
	_, err := s.eventBus.SubscribeWildcard("amityvox.>", func(subject string, event events.Event) {
		s.dispatchEvent(subject, event)
	})
	if err != nil {
		return fmt.Errorf("subscribing to events: %w", err)
	}

	s.logger.Info("WebSocket gateway starting", slog.String("listen", s.listenAddr))

	ln, err := net.Listen("tcp", s.listenAddr)
	if err != nil {
		return fmt.Errorf("listening on %s: %w", s.listenAddr, err)
	}

	if err := s.httpServer.Serve(ln); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("WebSocket server error: %w", err)
	}
	return nil
}

// Shutdown gracefully stops the WebSocket gateway server and disconnects all clients.
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("WebSocket gateway shutting down")

	s.clientsMu.RLock()
	for client := range s.clients {
		s.sendReconnect(client)
		client.conn.Close(websocket.StatusGoingAway, "server shutting down")
	}
	s.clientsMu.RUnlock()

	return s.httpServer.Shutdown(ctx)
}

// handleWebSocket upgrades an HTTP connection to WebSocket and manages the
// client lifecycle: HELLO -> IDENTIFY -> event loop.
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: []string{"*"},
	})
	if err != nil {
		s.logger.Error("WebSocket accept failed", slog.String("error", err.Error()))
		return
	}

	client := &Client{
		conn:     conn,
		done:     make(chan struct{}),
		guildIDs: make(map[string]bool),
	}

	// Send HELLO with heartbeat interval.
	helloData, _ := json.Marshal(HelloPayload{
		HeartbeatInterval: s.heartbeatInterval.Milliseconds(),
	})
	s.sendMessage(client, GatewayMessage{
		Op:   OpHello,
		Data: helloData,
	})

	// Wait for IDENTIFY with timeout.
	identifyCtx, identifyCancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer identifyCancel()

	if err := s.waitForIdentify(identifyCtx, client); err != nil {
		s.logger.Debug("client failed to identify", slog.String("error", err.Error()))
		conn.Close(websocket.StatusPolicyViolation, "identify timeout or invalid token")
		return
	}

	// Register the client.
	s.registerClient(client)

	// Set user presence to online.
	s.cache.SetPresence(r.Context(), client.userID, presence.StatusOnline, s.heartbeatTimeout)

	s.logger.Info("client connected",
		slog.String("user_id", client.userID),
	)

	// Run the read loop until disconnect.
	s.readLoop(r.Context(), client)

	// Cleanup on disconnect.
	s.unregisterClient(client)

	s.cache.RemovePresence(context.Background(), client.userID)

	s.logger.Info("client disconnected",
		slog.String("user_id", client.userID),
	)
}

// registerClient adds a client to all tracking maps.
func (s *Server) registerClient(client *Client) {
	s.clientsMu.Lock()
	s.clients[client] = struct{}{}
	s.clientsMu.Unlock()

	s.userClientsMu.Lock()
	if s.userClients[client.userID] == nil {
		s.userClients[client.userID] = make(map[*Client]struct{})
	}
	s.userClients[client.userID][client] = struct{}{}
	s.userClientsMu.Unlock()
}

// unregisterClient removes a client from all tracking maps.
func (s *Server) unregisterClient(client *Client) {
	s.clientsMu.Lock()
	delete(s.clients, client)
	s.clientsMu.Unlock()

	s.userClientsMu.Lock()
	if clients, ok := s.userClients[client.userID]; ok {
		delete(clients, client)
		if len(clients) == 0 {
			delete(s.userClients, client.userID)
		}
	}
	s.userClientsMu.Unlock()
}

// loadGuildMemberships queries the database to populate the client's guild list.
func (s *Server) loadGuildMemberships(ctx context.Context, client *Client) {
	if s.pool == nil {
		return
	}
	rows, err := s.pool.Query(ctx,
		`SELECT guild_id FROM guild_members WHERE user_id = $1`, client.userID)
	if err != nil {
		s.logger.Error("failed to load guild memberships", slog.String("error", err.Error()))
		return
	}
	defer rows.Close()

	client.mu.Lock()
	defer client.mu.Unlock()
	for rows.Next() {
		var guildID string
		if rows.Scan(&guildID) == nil {
			client.guildIDs[guildID] = true
		}
	}
}

// waitForIdentify reads the first client message expecting an IDENTIFY op
// with a valid session token.
func (s *Server) waitForIdentify(ctx context.Context, client *Client) error {
	var msg GatewayMessage
	if err := wsjson.Read(ctx, client.conn, &msg); err != nil {
		return fmt.Errorf("reading identify message: %w", err)
	}

	if msg.Op != OpIdentify {
		return fmt.Errorf("expected op %d (IDENTIFY), got %d", OpIdentify, msg.Op)
	}

	var payload IdentifyPayload
	if err := json.Unmarshal(msg.Data, &payload); err != nil {
		return fmt.Errorf("parsing identify payload: %w", err)
	}

	if payload.Token == "" {
		return fmt.Errorf("empty token in identify payload")
	}

	userID, err := s.authService.ValidateSession(ctx, payload.Token)
	if err != nil {
		return fmt.Errorf("invalid session token: %w", err)
	}

	client.userID = userID
	client.sessionID = payload.Token
	client.identified = true

	// Load guild memberships for event filtering.
	s.loadGuildMemberships(ctx, client)

	// Send READY dispatch.
	user, err := s.authService.GetUser(ctx, userID)
	if err != nil {
		return fmt.Errorf("getting user for READY: %w", err)
	}

	// Include guild IDs in READY payload.
	guildIDList := make([]string, 0, len(client.guildIDs))
	for gid := range client.guildIDs {
		guildIDList = append(guildIDList, gid)
	}

	readyData, _ := json.Marshal(map[string]interface{}{
		"user":      user,
		"guild_ids": guildIDList,
		"session_id": client.sessionID,
	})

	s.sendMessage(client, GatewayMessage{
		Op:   OpDispatch,
		Type: "READY",
		Data: readyData,
	})

	return nil
}

// readLoop reads messages from the WebSocket connection until it closes.
func (s *Server) readLoop(ctx context.Context, client *Client) {
	for {
		var msg GatewayMessage
		err := wsjson.Read(ctx, client.conn, &msg)
		if err != nil {
			return
		}

		switch msg.Op {
		case OpHeartbeat:
			s.sendMessage(client, GatewayMessage{Op: OpHeartbeatAck})
			s.cache.SetPresence(ctx, client.userID, presence.StatusOnline, s.heartbeatTimeout)

		case OpPresenceUpdate:
			var data struct {
				Status string `json:"status"`
			}
			if err := json.Unmarshal(msg.Data, &data); err == nil {
				s.cache.SetPresence(ctx, client.userID, data.Status, s.heartbeatTimeout)
			}

		case OpTyping:
			var data struct {
				ChannelID string `json:"channel_id"`
			}
			if err := json.Unmarshal(msg.Data, &data); err == nil {
				// Set typing indicator with 8-second auto-expiry.
				typingKey := fmt.Sprintf("typing:%s:%s", data.ChannelID, client.userID)
				s.cache.SetPresence(ctx, typingKey, "typing", 8*time.Second)

				typingData, _ := json.Marshal(map[string]string{
					"channel_id": data.ChannelID,
					"user_id":    client.userID,
				})
				s.eventBus.Publish(ctx, events.SubjectTypingStart, events.Event{
					Type:      "TYPING_START",
					ChannelID: data.ChannelID,
					UserID:    client.userID,
					Data:      typingData,
				})
			}

		case OpResume:
			s.handleResume(ctx, client, msg.Data)

		case OpRequestMembers:
			s.handleRequestMembers(ctx, client, msg.Data)

		case OpSubscribe:
			// Channel-level subscription for DMs or specific channels.
			var data struct {
				ChannelIDs []string `json:"channel_ids"`
			}
			if err := json.Unmarshal(msg.Data, &data); err == nil {
				s.logger.Debug("client subscribed to channels",
					slog.String("user_id", client.userID),
					slog.Int("count", len(data.ChannelIDs)),
				)
			}

		default:
			s.logger.Debug("unhandled gateway op",
				slog.Int("op", msg.Op),
				slog.String("user_id", client.userID),
			)
		}
	}
}

// handleResume attempts to resume a disconnected session by replaying missed events.
func (s *Server) handleResume(ctx context.Context, client *Client, data json.RawMessage) {
	var payload ResumePayload
	if err := json.Unmarshal(data, &payload); err != nil {
		s.logger.Debug("invalid resume payload", slog.String("error", err.Error()))
		s.sendReconnect(client)
		return
	}

	// Validate the token.
	userID, err := s.authService.ValidateSession(ctx, payload.Token)
	if err != nil || userID != client.userID {
		s.logger.Debug("resume token validation failed")
		s.sendReconnect(client)
		return
	}

	// Replay buffered events after the client's last seen sequence.
	client.mu.Lock()
	var replayed int
	for _, msg := range client.replayBuf {
		if msg.Seq != nil && *msg.Seq > payload.Seq {
			s.sendMessage(client, msg)
			replayed++
		}
	}
	client.mu.Unlock()

	// Send RESUMED dispatch.
	resumedData, _ := json.Marshal(map[string]int{"replayed": replayed})
	s.sendMessage(client, GatewayMessage{
		Op:   OpDispatch,
		Type: "RESUMED",
		Data: resumedData,
	})

	s.logger.Debug("client resumed",
		slog.String("user_id", client.userID),
		slog.Int("replayed", replayed),
	)
}

// handleRequestMembers fetches guild members and dispatches them to the requesting client.
func (s *Server) handleRequestMembers(ctx context.Context, client *Client, data json.RawMessage) {
	var payload RequestMembersPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return
	}

	if payload.GuildID == "" {
		return
	}

	// Verify the client is a member of this guild.
	client.mu.Lock()
	isMember := client.guildIDs[payload.GuildID]
	client.mu.Unlock()
	if !isMember {
		return
	}

	if payload.Limit <= 0 || payload.Limit > 100 {
		payload.Limit = 100
	}

	if s.pool == nil {
		return
	}

	var query string
	var args []interface{}
	if payload.Query != "" {
		query = `SELECT u.id, u.username, u.display_name, u.avatar_id, u.status_presence,
		                gm.nickname, gm.joined_at
		         FROM guild_members gm
		         JOIN users u ON u.id = gm.user_id
		         WHERE gm.guild_id = $1 AND u.username ILIKE '%' || $2 || '%'
		         ORDER BY u.username LIMIT $3`
		args = []interface{}{payload.GuildID, payload.Query, payload.Limit}
	} else {
		query = `SELECT u.id, u.username, u.display_name, u.avatar_id, u.status_presence,
		                gm.nickname, gm.joined_at
		         FROM guild_members gm
		         JOIN users u ON u.id = gm.user_id
		         WHERE gm.guild_id = $1
		         ORDER BY u.username LIMIT $2`
		args = []interface{}{payload.GuildID, payload.Limit}
	}

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		s.logger.Error("failed to query guild members", slog.String("error", err.Error()))
		return
	}
	defer rows.Close()

	type memberInfo struct {
		UserID         string  `json:"user_id"`
		Username       string  `json:"username"`
		DisplayName    *string `json:"display_name,omitempty"`
		AvatarID       *string `json:"avatar_id,omitempty"`
		StatusPresence string  `json:"status_presence"`
		Nickname       *string `json:"nickname,omitempty"`
		JoinedAt       string  `json:"joined_at"`
	}

	members := make([]memberInfo, 0)
	for rows.Next() {
		var m memberInfo
		var joinedAt time.Time
		if err := rows.Scan(&m.UserID, &m.Username, &m.DisplayName, &m.AvatarID,
			&m.StatusPresence, &m.Nickname, &joinedAt); err == nil {
			m.JoinedAt = joinedAt.Format(time.RFC3339)
			members = append(members, m)
		}
	}

	chunkData, _ := json.Marshal(map[string]interface{}{
		"guild_id": payload.GuildID,
		"members":  members,
	})

	s.sendMessage(client, GatewayMessage{
		Op:   OpDispatch,
		Type: "GUILD_MEMBERS_CHUNK",
		Data: chunkData,
	})
}

// dispatchEvent routes a NATS event to the appropriate connected clients based
// on event type and guild/channel membership.
func (s *Server) dispatchEvent(subject string, event events.Event) {
	msg := GatewayMessage{
		Op:   OpDispatch,
		Type: event.Type,
		Data: event.Data,
	}

	s.clientsMu.RLock()
	defer s.clientsMu.RUnlock()

	for client := range s.clients {
		if !client.identified {
			continue
		}

		if s.shouldDispatchTo(client, subject, event) {
			s.sendMessage(client, msg)

			// Buffer for potential resume replay (keep last 100 events per client).
			client.mu.Lock()
			client.replayBuf = append(client.replayBuf, msg)
			if len(client.replayBuf) > 100 {
				client.replayBuf = client.replayBuf[len(client.replayBuf)-100:]
			}
			client.mu.Unlock()
		}
	}
}

// shouldDispatchTo determines if a client should receive a given event based on
// guild membership and event targeting.
func (s *Server) shouldDispatchTo(client *Client, subject string, event events.Event) bool {
	// User-specific events: only dispatch to the targeted user.
	if event.UserID != "" && !strings.HasPrefix(subject, "amityvox.guild.") {
		if event.Type == "PRESENCE_UPDATE" || event.Type == "USER_UPDATE" {
			return event.UserID == client.userID
		}
	}

	// Guild events: only dispatch to guild members.
	if event.GuildID != "" {
		client.mu.Lock()
		isMember := client.guildIDs[event.GuildID]
		client.mu.Unlock()
		return isMember
	}

	// If the subject indicates a guild event, try to extract guild context from subject.
	if strings.HasPrefix(subject, "amityvox.guild.") {
		// Guild events without GuildID in payload — look at the data for guild_id.
		var data struct {
			GuildID string `json:"guild_id"`
		}
		if json.Unmarshal(event.Data, &data) == nil && data.GuildID != "" {
			client.mu.Lock()
			isMember := client.guildIDs[data.GuildID]
			client.mu.Unlock()
			return isMember
		}
	}

	// Channel events: look up which guild the channel belongs to.
	if event.ChannelID != "" && s.pool != nil {
		var guildID *string
		s.pool.QueryRow(context.Background(),
			`SELECT guild_id FROM channels WHERE id = $1`, event.ChannelID).Scan(&guildID)
		if guildID != nil {
			client.mu.Lock()
			isMember := client.guildIDs[*guildID]
			client.mu.Unlock()
			return isMember
		}
		// DM channel — check if the user is a participant.
		// For DMs, dispatch to both participants.
		return true
	}

	// Typing events for specific channels.
	if strings.HasPrefix(subject, "amityvox.channel.") {
		return true // DM or uncategorized — allow
	}

	// Default: dispatch to all (e.g., system events).
	return true
}

// NotifyGuildJoin updates all connected clients for a user when they join a new guild.
func (s *Server) NotifyGuildJoin(userID, guildID string) {
	s.userClientsMu.RLock()
	clients := s.userClients[userID]
	s.userClientsMu.RUnlock()

	for client := range clients {
		client.mu.Lock()
		client.guildIDs[guildID] = true
		client.mu.Unlock()
	}
}

// NotifyGuildLeave updates all connected clients for a user when they leave a guild.
func (s *Server) NotifyGuildLeave(userID, guildID string) {
	s.userClientsMu.RLock()
	clients := s.userClients[userID]
	s.userClientsMu.RUnlock()

	for client := range clients {
		client.mu.Lock()
		delete(client.guildIDs, guildID)
		client.mu.Unlock()
	}
}

// sendMessage sends a GatewayMessage to a client. Thread-safe.
func (s *Server) sendMessage(client *Client, msg GatewayMessage) {
	client.mu.Lock()
	defer client.mu.Unlock()

	if msg.Op == OpDispatch {
		client.seq++
		msg.Seq = &client.seq
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := wsjson.Write(ctx, client.conn, msg); err != nil {
		s.logger.Debug("failed to send message to client",
			slog.String("user_id", client.userID),
			slog.String("error", err.Error()),
		)
	}
}

// sendReconnect sends an op:6 RECONNECT to a client before shutdown.
func (s *Server) sendReconnect(client *Client) {
	s.sendMessage(client, GatewayMessage{Op: OpReconnect})
}

// ClientCount returns the number of currently connected clients.
func (s *Server) ClientCount() int {
	s.clientsMu.RLock()
	defer s.clientsMu.RUnlock()
	return len(s.clients)
}
