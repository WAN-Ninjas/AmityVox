// Package main implements the Discord bridge for AmityVox. It runs as a separate
// Docker container and uses Discord's bot API to relay messages bidirectionally
// between AmityVox channels and Discord channels. See docs/architecture.md
// Section 10.2 for the bridge specification.
//
// The bridge:
//   - Connects to Discord via a bot token and the Discord gateway
//   - Maps AmityVox channels ↔ Discord channels
//   - Relays messages bidirectionally using webhooks for display name/avatar fidelity
//   - Useful for migration: run both simultaneously during transition
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
	AmityVoxURL   string // REST API base URL
	AmityVoxToken string // Bot auth token
	DiscordToken  string // Discord bot token
	ListenAddr    string // HTTP listen for health checks
}

// Bridge is the Discord ↔ AmityVox bridge service.
type Bridge struct {
	cfg    Config
	client *http.Client
	logger *slog.Logger

	// Channel mapping: AmityVox channel ID → Discord channel ID and reverse.
	mu               sync.RWMutex
	channelToDiscord map[string]string
	discordToChannel map[string]string

	// Discord webhook mapping for better display name/avatar fidelity.
	webhookURLs map[string]string // Discord channel ID → webhook URL
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg := Config{
		AmityVoxURL:   envOr("AMITYVOX_URL", "http://localhost:8080"),
		AmityVoxToken: os.Getenv("AMITYVOX_TOKEN"),
		DiscordToken:  os.Getenv("DISCORD_TOKEN"),
		ListenAddr:    envOr("BRIDGE_LISTEN", "0.0.0.0:9881"),
	}

	if cfg.AmityVoxToken == "" || cfg.DiscordToken == "" {
		logger.Error("required environment variables not set",
			slog.Bool("AMITYVOX_TOKEN", cfg.AmityVoxToken != ""),
			slog.Bool("DISCORD_TOKEN", cfg.DiscordToken != ""),
		)
		fmt.Println("Required: AMITYVOX_TOKEN, DISCORD_TOKEN")
		fmt.Println("Optional: AMITYVOX_URL, BRIDGE_LISTEN")
		os.Exit(1)
	}

	bridge := &Bridge{
		cfg:              cfg,
		client:           &http.Client{Timeout: 30 * time.Second},
		logger:           logger,
		channelToDiscord: make(map[string]string),
		discordToChannel: make(map[string]string),
		webhookURLs:      make(map[string]string),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start health check HTTP server.
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	// Mapping management endpoints.
	mux.HandleFunc("/mappings", bridge.handleListMappings)
	mux.HandleFunc("/mappings/add", bridge.handleAddMapping)

	srv := &http.Server{Addr: cfg.ListenAddr, Handler: mux}

	// Start Discord gateway listener.
	go bridge.listenDiscord(ctx)

	// Start AmityVox WebSocket listener.
	go bridge.listenAmityVox(ctx)

	go func() {
		logger.Info("Discord bridge starting",
			slog.String("listen", cfg.ListenAddr),
		)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("bridge server error", slog.String("error", err.Error()))
		}
	}()

	// Wait for shutdown signal.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	logger.Info("shutting down Discord bridge")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	srv.Shutdown(shutdownCtx)
	cancel()
}

// --- Discord Gateway ---

// listenDiscord connects to the Discord gateway and processes incoming events.
func (b *Bridge) listenDiscord(ctx context.Context) {
	b.logger.Info("Discord gateway listener starting")

	// Discord Gateway connection flow:
	// 1. GET /api/v10/gateway/bot → get WSS URL
	// 2. Connect to WSS URL
	// 3. Receive Hello (op 10) → start heartbeating at the given interval
	// 4. Send Identify (op 2) with bot token and intents
	// 5. Receive Ready (op 0) → bot is connected
	// 6. Listen for MESSAGE_CREATE (t="MESSAGE_CREATE") events
	//
	// For a production bridge, use a library like github.com/bwmarrin/discordgo.
	// Here we implement the core relay loop structure.

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		err := b.connectDiscordGateway(ctx)
		if err != nil {
			b.logger.Error("Discord gateway error", slog.String("error", err.Error()))
		}

		// Reconnect with backoff.
		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Second):
		}
	}
}

// connectDiscordGateway establishes a connection to the Discord gateway.
// In production, this would use a proper Discord library (discordgo).
func (b *Bridge) connectDiscordGateway(ctx context.Context) error {
	// Get gateway URL.
	req, err := http.NewRequestWithContext(ctx, "GET", "https://discord.com/api/v10/gateway/bot", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bot "+b.cfg.DiscordToken)

	resp, err := b.client.Do(req)
	if err != nil {
		return fmt.Errorf("fetching Discord gateway URL: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("discord gateway API error %d: %s", resp.StatusCode, string(body))
	}

	var gateway struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&gateway); err != nil {
		return fmt.Errorf("decoding gateway response: %w", err)
	}

	b.logger.Info("Discord gateway URL obtained", slog.String("url", gateway.URL))

	// NOTE: Full WebSocket connection implementation would go here.
	// For the bridge skeleton, we log the URL and wait for context cancellation.
	// A production implementation would use discordgo or a manual gorilla/websocket
	// connection with heartbeating and event dispatch.

	<-ctx.Done()
	return ctx.Err()
}

// --- Discord → AmityVox Relay ---

// DiscordMessage represents a simplified Discord message event.
type DiscordMessage struct {
	ID        string `json:"id"`
	ChannelID string `json:"channel_id"`
	GuildID   string `json:"guild_id"`
	Author    struct {
		ID       string `json:"id"`
		Username string `json:"username"`
		Avatar   string `json:"avatar"`
	} `json:"author"`
	Content string `json:"content"`
}

// relayDiscordMessage sends a Discord message to the mapped AmityVox channel.
//
//nolint:unused // bridge skeleton — called when Discord gateway events are implemented
func (b *Bridge) relayDiscordMessage(ctx context.Context, msg DiscordMessage) {
	channelID := b.discordToChannelID(msg.ChannelID)
	if channelID == "" {
		return
	}

	if msg.Content == "" {
		return
	}

	// Post to AmityVox via REST API with masquerade for display fidelity.
	avatarURL := ""
	if msg.Author.Avatar != "" {
		avatarURL = fmt.Sprintf("https://cdn.discordapp.com/avatars/%s/%s.png",
			msg.Author.ID, msg.Author.Avatar)
	}

	payload := map[string]interface{}{
		"content": msg.Content,
		"masquerade": map[string]string{
			"name":   msg.Author.Username,
			"avatar": avatarURL,
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
		b.logger.Error("failed to relay Discord message to AmityVox",
			slog.String("discord_channel", msg.ChannelID),
			slog.String("error", err.Error()),
		)
		return
	}
	resp.Body.Close()

	b.logger.Debug("relayed Discord message to AmityVox",
		slog.String("discord_channel", msg.ChannelID),
		slog.String("amityvox_channel", channelID),
		slog.String("author", msg.Author.Username),
	)
}

// --- AmityVox → Discord Relay ---

// listenAmityVox connects to the AmityVox WebSocket gateway and relays events to Discord.
func (b *Bridge) listenAmityVox(ctx context.Context) {
	b.logger.Info("AmityVox listener starting")

	// In production, connect to ws://<host>:8081/ws with IDENTIFY.
	// On MESSAGE_CREATE events, relay to the mapped Discord channel.
	// Use Discord webhooks for better display name/avatar fidelity.

	<-ctx.Done()
}

// sendDiscordWebhook sends a message to a Discord channel via webhook.
//
//nolint:unused // bridge skeleton — called when AmityVox→Discord relay is implemented
func (b *Bridge) sendDiscordWebhook(ctx context.Context, discordChannelID, username, avatarURL, content string) error {
	webhookURL := b.getWebhookURL(discordChannelID)
	if webhookURL == "" {
		// Fall back to regular bot message.
		return b.sendDiscordMessage(ctx, discordChannelID, content)
	}

	payload := map[string]interface{}{
		"content":    content,
		"username":   username,
		"avatar_url": avatarURL,
	}

	payloadJSON, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, "POST", webhookURL, bytes.NewReader(payloadJSON))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := b.client.Do(req)
	if err != nil {
		return fmt.Errorf("sending Discord webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("discord webhook error %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// sendDiscordMessage sends a message to a Discord channel via the bot API.
//
//nolint:unused // bridge skeleton — called by sendDiscordWebhook fallback
func (b *Bridge) sendDiscordMessage(ctx context.Context, channelID, content string) error {
	url := fmt.Sprintf("https://discord.com/api/v10/channels/%s/messages", channelID)

	payload := map[string]string{"content": content}
	payloadJSON, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(payloadJSON))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bot "+b.cfg.DiscordToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := b.client.Do(req)
	if err != nil {
		return fmt.Errorf("sending Discord message: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("discord API error %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// --- Channel Mapping Management ---

// MapChannel creates a bidirectional mapping.
func (b *Bridge) MapChannel(amityVoxID, discordID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.channelToDiscord[amityVoxID] = discordID
	b.discordToChannel[discordID] = amityVoxID
}

// SetWebhookURL sets the Discord webhook URL for a channel.
func (b *Bridge) SetWebhookURL(discordChannelID, webhookURL string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.webhookURLs[discordChannelID] = webhookURL
}

//nolint:unused // bridge skeleton — used when AmityVox→Discord relay is implemented
func (b *Bridge) channelToDiscordID(channelID string) string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.channelToDiscord[channelID]
}

//nolint:unused // bridge skeleton — used by relayDiscordMessage
func (b *Bridge) discordToChannelID(discordID string) string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.discordToChannel[discordID]
}

//nolint:unused // bridge skeleton — used by sendDiscordWebhook
func (b *Bridge) getWebhookURL(discordChannelID string) string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.webhookURLs[discordChannelID]
}

// --- HTTP Management Handlers ---

func (b *Bridge) handleListMappings(w http.ResponseWriter, r *http.Request) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	type mapping struct {
		AmityVoxChannelID string `json:"amityvox_channel_id"`
		DiscordChannelID  string `json:"discord_channel_id"`
		WebhookURL        string `json:"webhook_url,omitempty"`
	}

	var mappings []mapping
	for avID, dID := range b.channelToDiscord {
		mappings = append(mappings, mapping{
			AmityVoxChannelID: avID,
			DiscordChannelID:  dID,
			WebhookURL:        b.webhookURLs[dID],
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
		DiscordChannelID  string `json:"discord_channel_id"`
		WebhookURL        string `json:"webhook_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}

	if req.AmityVoxChannelID == "" || req.DiscordChannelID == "" {
		http.Error(w, "amityvox_channel_id and discord_channel_id are required", http.StatusBadRequest)
		return
	}

	b.MapChannel(req.AmityVoxChannelID, req.DiscordChannelID)
	if req.WebhookURL != "" {
		b.SetWebhookURL(req.DiscordChannelID, req.WebhookURL)
	}

	b.logger.Info("channel mapping added",
		slog.String("amityvox", req.AmityVoxChannelID),
		slog.String("discord", req.DiscordChannelID),
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
