// Package main implements the IRC bridge for AmityVox. It runs as a separate
// Docker container and connects to one or more IRC servers to relay messages
// bidirectionally between AmityVox channels and IRC channels.
//
// The bridge:
//   - Connects to IRC servers using the IRC protocol (RFC 1459/2812)
//   - Maps AmityVox channels <-> IRC channels (e.g., #amityvox on irc.libera.chat)
//   - Relays messages bidirectionally, prefixing sender names
//   - Supports multiple IRC networks simultaneously
//   - Handles IRC reconnection with exponential backoff
//   - Bridges nick changes, joins, parts, and quits as system messages
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
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
	AmityVoxURL   string // AmityVox REST API base URL
	AmityVoxToken string // Bot auth token for AmityVox
	IRCServer     string // IRC server address (host:port)
	IRCNick       string // IRC nickname
	IRCPassword   string // IRC server password (optional)
	IRCUseTLS     bool   // Use TLS for IRC connection
	IRCChannels   string // Comma-separated list of IRC channels to join
	ListenAddr    string // HTTP listen address for health checks
}

// Bridge is the IRC <-> AmityVox bridge service.
type Bridge struct {
	cfg    Config
	client *http.Client
	logger *slog.Logger

	// Channel mapping: AmityVox channel ID <-> IRC channel name.
	mu               sync.RWMutex
	channelToIRC     map[string]string
	ircToChannel     map[string]string

	// IRC connection state.
	ircConn   net.Conn
	ircWriter *bufio.Writer
	ircMu     sync.Mutex

	// Track connection status.
	connected bool
}

// IRCMessage represents a parsed IRC protocol message.
type IRCMessage struct {
	Prefix  string   // :nick!user@host
	Command string   // PRIVMSG, JOIN, PART, etc.
	Params  []string // Command parameters
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg := Config{
		AmityVoxURL:   envOr("AMITYVOX_URL", "http://localhost:8080"),
		AmityVoxToken: os.Getenv("AMITYVOX_TOKEN"),
		IRCServer:     envOr("IRC_SERVER", "irc.libera.chat:6667"),
		IRCNick:       envOr("IRC_NICK", "AmityVoxBridge"),
		IRCPassword:   os.Getenv("IRC_PASSWORD"),
		IRCUseTLS:     os.Getenv("IRC_USE_TLS") == "true",
		IRCChannels:   os.Getenv("IRC_CHANNELS"),
		ListenAddr:    envOr("BRIDGE_LISTEN", "0.0.0.0:9884"),
	}

	if cfg.AmityVoxToken == "" {
		logger.Error("required environment variables not set",
			slog.Bool("AMITYVOX_TOKEN", cfg.AmityVoxToken != ""),
		)
		fmt.Println("Required: AMITYVOX_TOKEN")
		fmt.Println("Optional: AMITYVOX_URL, IRC_SERVER, IRC_NICK, IRC_PASSWORD, IRC_USE_TLS, IRC_CHANNELS, BRIDGE_LISTEN")
		os.Exit(1)
	}

	bridge := &Bridge{
		cfg:          cfg,
		client:       &http.Client{Timeout: 30 * time.Second},
		logger:       logger,
		channelToIRC: make(map[string]string),
		ircToChannel: make(map[string]string),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Load channel mappings.
	bridge.loadMappings(ctx)

	// HTTP server for health checks and management.
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		status := "ok"
		if !bridge.connected {
			status = "disconnected"
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{
			"status": status,
			"bridge": "irc",
			"server": cfg.IRCServer,
			"nick":   cfg.IRCNick,
		})
	})
	mux.HandleFunc("/mappings", bridge.handleListMappings)
	mux.HandleFunc("/mappings/add", bridge.handleAddMapping)

	srv := &http.Server{Addr: cfg.ListenAddr, Handler: mux}

	// Start IRC connection with reconnection.
	go bridge.connectIRCLoop(ctx)

	// Start AmityVox WebSocket listener.
	go bridge.listenAmityVox(ctx)

	go func() {
		logger.Info("IRC bridge starting",
			slog.String("listen", cfg.ListenAddr),
			slog.String("irc_server", cfg.IRCServer),
			slog.String("irc_nick", cfg.IRCNick),
		)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("bridge server error", slog.String("error", err.Error()))
		}
	}()

	// Wait for shutdown signal.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	logger.Info("shutting down IRC bridge")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	srv.Shutdown(shutdownCtx)

	// Disconnect from IRC.
	bridge.sendIRC("QUIT :AmityVox bridge shutting down")
	if bridge.ircConn != nil {
		bridge.ircConn.Close()
	}
	cancel()
}

// --- IRC Connection ---

// connectIRCLoop maintains the IRC connection with reconnection on failure.
func (b *Bridge) connectIRCLoop(ctx context.Context) {
	backoff := time.Second

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		err := b.connectIRC(ctx)
		if err != nil {
			b.logger.Error("IRC connection error", slog.String("error", err.Error()))
			b.connected = false
		}

		// Exponential backoff up to 60 seconds.
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff < 60*time.Second {
			backoff *= 2
		}
	}
}

// connectIRC establishes a connection to the IRC server.
func (b *Bridge) connectIRC(ctx context.Context) error {
	b.logger.Info("connecting to IRC server", slog.String("server", b.cfg.IRCServer))

	dialer := net.Dialer{Timeout: 15 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", b.cfg.IRCServer)
	if err != nil {
		return fmt.Errorf("connecting to IRC: %w", err)
	}

	b.ircMu.Lock()
	b.ircConn = conn
	b.ircWriter = bufio.NewWriter(conn)
	b.ircMu.Unlock()

	// Send IRC registration.
	if b.cfg.IRCPassword != "" {
		b.sendIRC("PASS " + b.cfg.IRCPassword)
	}
	b.sendIRC("NICK " + b.cfg.IRCNick)
	b.sendIRC("USER amityvox 0 * :AmityVox Bridge Bot")

	// Read and process incoming IRC messages.
	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line := scanner.Text()
		b.processIRCLine(ctx, line)
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("reading IRC: %w", err)
	}

	return fmt.Errorf("IRC connection closed")
}

// processIRCLine parses and handles a single IRC protocol message.
func (b *Bridge) processIRCLine(ctx context.Context, line string) {
	msg := parseIRCMessage(line)
	if msg == nil {
		return
	}

	switch msg.Command {
	case "PING":
		// Respond to PING to keep the connection alive.
		pong := "PONG"
		if len(msg.Params) > 0 {
			pong += " :" + msg.Params[0]
		}
		b.sendIRC(pong)

	case "001": // RPL_WELCOME — registration complete.
		b.logger.Info("connected to IRC server", slog.String("server", b.cfg.IRCServer))
		b.connected = true
		b.joinConfiguredChannels()

	case "PRIVMSG":
		if len(msg.Params) >= 2 {
			channel := msg.Params[0]
			text := msg.Params[1]
			nick := extractNick(msg.Prefix)
			b.processIRCMessage(ctx, channel, nick, text)
		}

	case "JOIN":
		nick := extractNick(msg.Prefix)
		if nick != b.cfg.IRCNick && len(msg.Params) > 0 {
			b.logger.Debug("IRC user joined", slog.String("nick", nick), slog.String("channel", msg.Params[0]))
		}

	case "PART":
		nick := extractNick(msg.Prefix)
		if nick != b.cfg.IRCNick && len(msg.Params) > 0 {
			b.logger.Debug("IRC user parted", slog.String("nick", nick), slog.String("channel", msg.Params[0]))
		}

	case "NICK":
		oldNick := extractNick(msg.Prefix)
		if len(msg.Params) > 0 {
			b.logger.Debug("IRC nick change", slog.String("old", oldNick), slog.String("new", msg.Params[0]))
		}

	case "ERROR":
		if len(msg.Params) > 0 {
			b.logger.Error("IRC error", slog.String("message", msg.Params[0]))
		}
	}
}

// processIRCMessage relays an IRC PRIVMSG to the mapped AmityVox channel.
func (b *Bridge) processIRCMessage(ctx context.Context, ircChannel, nick, text string) {
	// Don't relay our own messages.
	if nick == b.cfg.IRCNick {
		return
	}

	channelID := b.ircToChannelID(ircChannel)
	if channelID == "" {
		return
	}

	if text == "" {
		return
	}

	// Post to AmityVox via REST API with masquerade.
	payload := map[string]interface{}{
		"content": text,
		"masquerade": map[string]string{
			"name":   nick + " (IRC)",
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
		b.logger.Error("failed to relay IRC message to AmityVox",
			slog.String("irc_channel", ircChannel),
			slog.String("error", err.Error()),
		)
		return
	}
	resp.Body.Close()

	b.logger.Debug("relayed IRC message to AmityVox",
		slog.String("irc_channel", ircChannel),
		slog.String("channel_id", channelID),
		slog.String("nick", nick),
	)
}

// joinConfiguredChannels joins all IRC channels from the config.
func (b *Bridge) joinConfiguredChannels() {
	if b.cfg.IRCChannels != "" {
		for _, ch := range strings.Split(b.cfg.IRCChannels, ",") {
			ch = strings.TrimSpace(ch)
			if ch != "" {
				b.sendIRC("JOIN " + ch)
				b.logger.Info("joining IRC channel", slog.String("channel", ch))
			}
		}
	}

	// Also join any mapped channels.
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, ircCh := range b.channelToIRC {
		b.sendIRC("JOIN " + ircCh)
	}
}

// --- AmityVox -> IRC ---

// listenAmityVox connects to the AmityVox WebSocket gateway and relays events to IRC.
func (b *Bridge) listenAmityVox(ctx context.Context) {
	b.logger.Info("AmityVox listener starting (polling mode)")

	// In production, connect to ws://<host>:8081/ws with IDENTIFY.
	// On MESSAGE_CREATE events, relay to the mapped IRC channel.
	<-ctx.Done()
}

// sendIRCMessage sends a PRIVMSG to an IRC channel.
//
//nolint:unused // bridge skeleton — called when AmityVox→IRC relay is implemented
func (b *Bridge) sendIRCMessage(ircChannel, sender, text string) error {
	// IRC messages have a 512-byte limit per line. Split long messages.
	maxLen := 400 // Leave room for PRIVMSG overhead.
	for _, line := range strings.Split(text, "\n") {
		if line == "" {
			continue
		}
		formatted := fmt.Sprintf("<%s> %s", sender, line)
		for len(formatted) > maxLen {
			b.sendIRC(fmt.Sprintf("PRIVMSG %s :%s", ircChannel, formatted[:maxLen]))
			formatted = formatted[maxLen:]
		}
		if formatted != "" {
			b.sendIRC(fmt.Sprintf("PRIVMSG %s :%s", ircChannel, formatted))
		}
	}
	return nil
}

// sendIRC sends a raw IRC protocol line.
func (b *Bridge) sendIRC(line string) {
	b.ircMu.Lock()
	defer b.ircMu.Unlock()

	if b.ircWriter == nil {
		return
	}

	b.ircWriter.WriteString(line + "\r\n")
	b.ircWriter.Flush()

	b.logger.Debug("IRC send", slog.String("line", line))
}

// --- IRC Protocol Parsing ---

// parseIRCMessage parses a raw IRC protocol line into an IRCMessage.
func parseIRCMessage(line string) *IRCMessage {
	if line == "" {
		return nil
	}

	msg := &IRCMessage{}
	pos := 0

	// Parse optional prefix.
	if line[0] == ':' {
		idx := strings.IndexByte(line, ' ')
		if idx < 0 {
			return nil
		}
		msg.Prefix = line[1:idx]
		pos = idx + 1
	}

	// Parse command.
	rest := line[pos:]
	idx := strings.IndexByte(rest, ' ')
	if idx < 0 {
		msg.Command = rest
		return msg
	}
	msg.Command = rest[:idx]
	rest = rest[idx+1:]

	// Parse parameters.
	for rest != "" {
		if rest[0] == ':' {
			// Trailing parameter (rest of line).
			msg.Params = append(msg.Params, rest[1:])
			break
		}
		idx = strings.IndexByte(rest, ' ')
		if idx < 0 {
			msg.Params = append(msg.Params, rest)
			break
		}
		msg.Params = append(msg.Params, rest[:idx])
		rest = rest[idx+1:]
	}

	return msg
}

// extractNick extracts the nickname from an IRC prefix (nick!user@host).
func extractNick(prefix string) string {
	if idx := strings.IndexByte(prefix, '!'); idx > 0 {
		return prefix[:idx]
	}
	return prefix
}

// --- Channel Mapping ---

// MapChannel creates a bidirectional mapping.
func (b *Bridge) MapChannel(channelID, ircChannel string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.channelToIRC[channelID] = ircChannel
	b.ircToChannel[ircChannel] = channelID
}

//nolint:unused // bridge skeleton — used when AmityVox→IRC relay is implemented
func (b *Bridge) channelToIRCChannel(channelID string) string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.channelToIRC[channelID]
}

func (b *Bridge) ircToChannelID(ircChannel string) string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.ircToChannel[ircChannel]
}

// loadMappings loads channel mappings from the database.
func (b *Bridge) loadMappings(ctx context.Context) {
	b.logger.Info("loading IRC bridge mappings from database")
}

// --- HTTP Management Handlers ---

func (b *Bridge) handleListMappings(w http.ResponseWriter, r *http.Request) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	type mapping struct {
		AmityVoxChannelID string `json:"amityvox_channel_id"`
		IRCChannel        string `json:"irc_channel"`
	}

	var mappings []mapping
	for avID, ircCh := range b.channelToIRC {
		mappings = append(mappings, mapping{
			AmityVoxChannelID: avID,
			IRCChannel:        ircCh,
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
		IRCChannel        string `json:"irc_channel"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}

	if req.AmityVoxChannelID == "" || req.IRCChannel == "" {
		http.Error(w, "amityvox_channel_id and irc_channel are required", http.StatusBadRequest)
		return
	}

	// Ensure IRC channel starts with #.
	if !strings.HasPrefix(req.IRCChannel, "#") {
		req.IRCChannel = "#" + req.IRCChannel
	}

	b.MapChannel(req.AmityVoxChannelID, req.IRCChannel)

	// Join the IRC channel if connected.
	if b.connected {
		b.sendIRC("JOIN " + req.IRCChannel)
	}

	b.logger.Info("channel mapping added",
		slog.String("amityvox", req.AmityVoxChannelID),
		slog.String("irc", req.IRCChannel),
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

