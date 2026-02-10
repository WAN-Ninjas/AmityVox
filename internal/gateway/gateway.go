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
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

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
	mu         sync.Mutex
	done       chan struct{}
}

// Server manages WebSocket connections and event dispatch.
type Server struct {
	authService       *auth.Service
	eventBus          *events.Bus
	cache             *presence.Cache
	heartbeatInterval time.Duration
	heartbeatTimeout  time.Duration
	listenAddr        string
	logger            *slog.Logger

	clients   map[*Client]struct{}
	clientsMu sync.RWMutex

	httpServer *http.Server
}

// ServerConfig holds the configuration for creating a gateway Server.
type ServerConfig struct {
	AuthService       *auth.Service
	EventBus          *events.Bus
	Cache             *presence.Cache
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
		heartbeatInterval: cfg.HeartbeatInterval,
		heartbeatTimeout:  cfg.HeartbeatTimeout,
		listenAddr:        cfg.ListenAddr,
		logger:            cfg.Logger,
		clients:           make(map[*Client]struct{}),
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
		s.broadcastEvent(event)
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
		conn: conn,
		done: make(chan struct{}),
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
	s.clientsMu.Lock()
	s.clients[client] = struct{}{}
	s.clientsMu.Unlock()

	// Set user presence to online.
	s.cache.SetPresence(r.Context(), client.userID, presence.StatusOnline, s.heartbeatTimeout)

	s.logger.Info("client connected",
		slog.String("user_id", client.userID),
	)

	// Run the read loop until disconnect.
	s.readLoop(r.Context(), client)

	// Cleanup on disconnect.
	s.clientsMu.Lock()
	delete(s.clients, client)
	s.clientsMu.Unlock()

	s.cache.RemovePresence(context.Background(), client.userID)

	s.logger.Info("client disconnected",
		slog.String("user_id", client.userID),
	)
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

	// Send READY dispatch.
	user, err := s.authService.GetUser(ctx, userID)
	if err != nil {
		return fmt.Errorf("getting user for READY: %w", err)
	}

	readyData, _ := json.Marshal(map[string]interface{}{
		"user": user,
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

		default:
			s.logger.Debug("unhandled gateway op",
				slog.Int("op", msg.Op),
				slog.String("user_id", client.userID),
			)
		}
	}
}

// broadcastEvent dispatches a NATS event to all connected clients that should
// receive it. For now this is a simple broadcast; guild/channel-scoped filtering
// will be refined as guild membership queries are implemented.
func (s *Server) broadcastEvent(event events.Event) {
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
		s.sendMessage(client, msg)
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
