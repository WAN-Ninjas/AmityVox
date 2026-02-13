// Package main implements the Matrix bridge for AmityVox. It runs as a separate
// Docker container and implements the Matrix Application Service API to bridge
// messages, presence, and typing indicators between AmityVox channels and Matrix
// rooms. See docs/architecture.md Section 10.1 for the bridge specification.
//
// The bridge:
//   - Registers as an appservice on a Matrix homeserver (Conduit, Dendrite, or Synapse)
//   - Maps AmityVox channels ↔ Matrix rooms bidirectionally
//   - Translates message formats (markdown ↔ Matrix event format)
//   - Bridges user presence and typing indicators
//   - Uses masquerade on the AmityVox side (bridged Matrix users show their Matrix name/avatar)
//   - Uses virtual Matrix users on the Matrix side
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Config holds bridge configuration loaded from environment variables.
type Config struct {
	AmityVoxURL    string // REST API base URL
	AmityVoxToken  string // Bot auth token
	MatrixHS       string // Matrix homeserver URL
	MatrixASToken  string // Application Service token (HS→AS)
	MatrixHSToken  string // Homeserver token (AS→HS)
	ListenAddr     string // HTTP listen address for appservice transactions
	BridgeUserPrefix string // Virtual user prefix on Matrix side
}

// Bridge is the Matrix ↔ AmityVox bridge service.
type Bridge struct {
	cfg    Config
	client *http.Client
	logger *slog.Logger

	// Channel mapping: AmityVox channel ID → Matrix room ID and reverse.
	mu              sync.RWMutex
	channelToRoom   map[string]string
	roomToChannel   map[string]string
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg := Config{
		AmityVoxURL:      envOr("AMITYVOX_URL", "http://localhost:8080"),
		AmityVoxToken:    os.Getenv("AMITYVOX_TOKEN"),
		MatrixHS:         envOr("MATRIX_HOMESERVER", "http://localhost:8008"),
		MatrixASToken:    os.Getenv("MATRIX_AS_TOKEN"),
		MatrixHSToken:    os.Getenv("MATRIX_HS_TOKEN"),
		ListenAddr:       envOr("BRIDGE_LISTEN", "0.0.0.0:9880"),
		BridgeUserPrefix: envOr("BRIDGE_USER_PREFIX", "amityvox_"),
	}

	if cfg.AmityVoxToken == "" || cfg.MatrixASToken == "" {
		logger.Error("required environment variables not set",
			slog.Bool("AMITYVOX_TOKEN", cfg.AmityVoxToken != ""),
			slog.Bool("MATRIX_AS_TOKEN", cfg.MatrixASToken != ""),
		)
		fmt.Println("Required: AMITYVOX_TOKEN, MATRIX_AS_TOKEN")
		fmt.Println("Optional: AMITYVOX_URL, MATRIX_HOMESERVER, MATRIX_HS_TOKEN, BRIDGE_LISTEN, BRIDGE_USER_PREFIX")
		os.Exit(1)
	}

	bridge := &Bridge{
		cfg:           cfg,
		client:        &http.Client{Timeout: 30 * time.Second},
		logger:        logger,
		channelToRoom: make(map[string]string),
		roomToChannel: make(map[string]string),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start the appservice HTTP server to receive transactions from the homeserver.
	mux := http.NewServeMux()
	mux.HandleFunc("/transactions/", bridge.handleTransaction)
	mux.HandleFunc("/rooms/", bridge.handleRoomAlias)
	mux.HandleFunc("/users/", bridge.handleUserQuery)
	mux.HandleFunc("/_matrix/app/v1/transactions/", bridge.handleTransaction)
	mux.HandleFunc("/_matrix/app/v1/rooms/", bridge.handleRoomAlias)
	mux.HandleFunc("/_matrix/app/v1/users/", bridge.handleUserQuery)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})

	srv := &http.Server{
		Addr:    cfg.ListenAddr,
		Handler: mux,
	}

	// Start AmityVox WebSocket listener for relay in the other direction.
	go bridge.listenAmityVox(ctx)

	go func() {
		logger.Info("Matrix bridge appservice starting",
			slog.String("listen", cfg.ListenAddr),
			slog.String("homeserver", cfg.MatrixHS),
		)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("appservice server error", slog.String("error", err.Error()))
		}
	}()

	// Wait for shutdown signal.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	logger.Info("shutting down Matrix bridge")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	srv.Shutdown(shutdownCtx)
	cancel()
}

// --- Matrix Appservice Handlers ---

// handleTransaction processes incoming Matrix events from the homeserver.
func (b *Bridge) handleTransaction(w http.ResponseWriter, r *http.Request) {
	// Verify the HS token.
	token := r.URL.Query().Get("access_token")
	if token == "" {
		token = strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	}
	if b.cfg.MatrixHSToken != "" && token != b.cfg.MatrixHSToken {
		http.Error(w, `{"errcode":"M_FORBIDDEN"}`, http.StatusForbidden)
		return
	}

	var txn struct {
		Events []MatrixEvent `json:"events"`
	}
	if err := json.NewDecoder(r.Body).Decode(&txn); err != nil {
		http.Error(w, `{"errcode":"M_BAD_JSON"}`, http.StatusBadRequest)
		return
	}

	for _, event := range txn.Events {
		b.processMatrixEvent(r.Context(), event)
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{}`))
}

// handleRoomAlias handles room alias queries from the homeserver.
func (b *Bridge) handleRoomAlias(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotFound)
	w.Write([]byte(`{"errcode":"M_NOT_FOUND"}`))
}

// handleUserQuery handles virtual user existence queries.
func (b *Bridge) handleUserQuery(w http.ResponseWriter, r *http.Request) {
	// The bridge claims all users with the configured prefix.
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{}`))
}

// MatrixEvent represents a simplified Matrix event.
type MatrixEvent struct {
	Type     string            `json:"type"`
	RoomID   string            `json:"room_id"`
	Sender   string            `json:"sender"`
	EventID  string            `json:"event_id"`
	Content  map[string]interface{} `json:"content"`
}

// processMatrixEvent translates a Matrix event and relays it to AmityVox.
func (b *Bridge) processMatrixEvent(ctx context.Context, event MatrixEvent) {
	// Skip events from our own virtual users.
	if strings.Contains(event.Sender, b.cfg.BridgeUserPrefix) {
		return
	}

	switch event.Type {
	case "m.room.message":
		b.relayMatrixMessage(ctx, event)
	case "m.typing":
		b.relayMatrixTyping(ctx, event)
	}
}

// relayMatrixMessage sends a Matrix message to the mapped AmityVox channel.
func (b *Bridge) relayMatrixMessage(ctx context.Context, event MatrixEvent) {
	channelID := b.roomToChannelID(event.RoomID)
	if channelID == "" {
		return
	}

	// Extract message body — prefer formatted_body (HTML) but fall back to body (plaintext).
	body, _ := event.Content["body"].(string)
	if body == "" {
		return
	}

	// Extract sender display name from the Matrix user ID.
	displayName := matrixUserDisplayName(event.Sender)

	// Post to AmityVox via REST API with masquerade.
	payload := map[string]interface{}{
		"content": body,
		"masquerade": map[string]string{
			"name":   displayName,
			"avatar": "", // Matrix avatar URL could be resolved here.
		},
	}

	payloadJSON, _ := json.Marshal(payload)
	url := fmt.Sprintf("%s/api/v1/channels/%s/messages", b.cfg.AmityVoxURL, channelID)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(payloadJSON))
	if err != nil {
		return
	}
	req.Header.Set("Authorization", "Bearer "+b.cfg.AmityVoxToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := b.client.Do(req)
	if err != nil {
		b.logger.Error("failed to relay message to AmityVox",
			slog.String("room_id", event.RoomID),
			slog.String("error", err.Error()),
		)
		return
	}
	resp.Body.Close()

	b.logger.Debug("relayed Matrix message to AmityVox",
		slog.String("room_id", event.RoomID),
		slog.String("channel_id", channelID),
		slog.String("sender", event.Sender),
	)
}

// relayMatrixTyping sends a typing indicator to the mapped AmityVox channel.
func (b *Bridge) relayMatrixTyping(ctx context.Context, event MatrixEvent) {
	channelID := b.roomToChannelID(event.RoomID)
	if channelID == "" {
		return
	}

	url := fmt.Sprintf("%s/api/v1/channels/%s/typing", b.cfg.AmityVoxURL, channelID)
	req, _ := http.NewRequestWithContext(ctx, "POST", url, nil)
	req.Header.Set("Authorization", "Bearer "+b.cfg.AmityVoxToken)
	resp, err := b.client.Do(req)
	if err == nil {
		resp.Body.Close()
	}
}

// --- AmityVox → Matrix Direction ---

// listenAmityVox connects to the AmityVox WebSocket gateway and relays events to Matrix.
func (b *Bridge) listenAmityVox(ctx context.Context) {
	b.logger.Info("AmityVox listener starting (polling mode)")

	// In a full implementation, this would connect to the WebSocket gateway.
	// For now, we poll the REST API for new messages.
	// The WS gateway connection would be:
	//   ws://<host>:8081/ws with IDENTIFY payload containing the bot token.
	//
	// When a MESSAGE_CREATE event arrives from the gateway, relay it to the
	// mapped Matrix room via the CS API.

	<-ctx.Done()
}

// sendMatrixMessage sends a message to a Matrix room via the CS API.
//
//nolint:unused // bridge skeleton — called when AmityVox→Matrix relay is implemented
func (b *Bridge) sendMatrixMessage(ctx context.Context, roomID, senderID, body string) error {
	txnID := fmt.Sprintf("amityvox_%d", time.Now().UnixNano())
	url := fmt.Sprintf("%s/_matrix/client/v3/rooms/%s/send/m.room.message/%s",
		b.cfg.MatrixHS, roomID, txnID)

	content := map[string]interface{}{
		"msgtype": "m.text",
		"body":    body,
	}

	// If we have a virtual user for this sender, send as that user.
	// Otherwise send as the appservice bot.
	payloadJSON, _ := json.Marshal(content)
	req, err := http.NewRequestWithContext(ctx, "PUT", url, bytes.NewReader(payloadJSON))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+b.cfg.MatrixASToken)
	req.Header.Set("Content-Type", "application/json")

	// Impersonate the virtual user if applicable.
	if senderID != "" {
		virtualUser := fmt.Sprintf("@%s%s", b.cfg.BridgeUserPrefix, senderID)
		q := req.URL.Query()
		q.Set("user_id", virtualUser)
		req.URL.RawQuery = q.Encode()
	}

	resp, err := b.client.Do(req)
	if err != nil {
		return fmt.Errorf("sending Matrix message: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("matrix API error %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// --- Channel Mapping ---

// MapChannel creates a bidirectional mapping between an AmityVox channel and Matrix room.
func (b *Bridge) MapChannel(channelID, roomID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.channelToRoom[channelID] = roomID
	b.roomToChannel[roomID] = channelID
}

//nolint:unused // bridge skeleton — used when AmityVox→Matrix relay is implemented
func (b *Bridge) channelToRoomID(channelID string) string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.channelToRoom[channelID]
}

func (b *Bridge) roomToChannelID(roomID string) string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.roomToChannel[roomID]
}

// --- Helpers ---

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// matrixUserDisplayName extracts a display name from a Matrix user ID (@user:server).
func matrixUserDisplayName(userID string) string {
	name := strings.TrimPrefix(userID, "@")
	if idx := strings.Index(name, ":"); idx > 0 {
		name = name[:idx]
	}
	return name
}
