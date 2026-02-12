// Package plugins implements the WASM plugin runtime for AmityVox. Plugins are
// WebAssembly modules loaded server-side that can hook into message events,
// guild events, and scheduled tasks. Each plugin runs in a sandboxed environment
// with configurable memory and CPU limits and no filesystem access.
package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/amityvox/amityvox/internal/events"
	"github.com/amityvox/amityvox/internal/models"
)

// HookType defines the event types that plugins can subscribe to.
type HookType string

const (
	HookMessageCreate HookType = "message_create"
	HookMessageUpdate HookType = "message_update"
	HookMessageDelete HookType = "message_delete"
	HookMemberJoin    HookType = "member_join"
	HookMemberLeave   HookType = "member_leave"
	HookGuildUpdate   HookType = "guild_update"
	HookScheduled     HookType = "scheduled"
	HookReactionAdd   HookType = "reaction_add"
	HookReactionDel   HookType = "reaction_remove"
)

// PluginManifest describes a plugin's capabilities, hooks, and configuration schema.
type PluginManifest struct {
	Hooks       []HookType             `json:"hooks"`
	Permissions []string               `json:"permissions"` // e.g., "send_message", "manage_roles"
	ConfigSchema map[string]ConfigField `json:"config_schema"`
}

// ConfigField describes a single configurable field in a plugin's settings.
type ConfigField struct {
	Type        string      `json:"type"`        // "string", "number", "boolean", "channel", "role"
	Label       string      `json:"label"`
	Description string      `json:"description"`
	Default     interface{} `json:"default,omitempty"`
	Required    bool        `json:"required"`
	Min         *float64    `json:"min,omitempty"`
	Max         *float64    `json:"max,omitempty"`
	Options     []string    `json:"options,omitempty"` // For enum-like fields
}

// PluginContext provides the execution context for a plugin invocation.
// This is the data passed to the WASM module when a hook fires.
type PluginContext struct {
	GuildID   string          `json:"guild_id"`
	ChannelID string          `json:"channel_id,omitempty"`
	UserID    string          `json:"user_id,omitempty"`
	HookType  HookType        `json:"hook_type"`
	EventData json.RawMessage `json:"event_data"`
	Config    json.RawMessage `json:"config"` // Guild-specific plugin config
}

// PluginResponse is the structured output from a plugin execution.
type PluginResponse struct {
	Actions []PluginAction `json:"actions"`
}

// PluginAction represents a single action the plugin wants to perform.
type PluginAction struct {
	Type    string          `json:"type"` // "send_message", "add_role", "remove_role", "react", "log"
	Payload json.RawMessage `json:"payload"`
}

// ExecutionResult captures the outcome of a plugin execution for logging.
type ExecutionResult struct {
	PluginID     string        `json:"plugin_id"`
	GuildID      string        `json:"guild_id"`
	HookType     HookType      `json:"hook_type"`
	Status       string        `json:"status"` // "success", "error", "timeout"
	Duration     time.Duration `json:"duration"`
	MemoryBytes  int64         `json:"memory_bytes"`
	ErrorMessage string        `json:"error_message,omitempty"`
	Actions      int           `json:"actions"`
}

// Runtime manages WASM plugin loading, execution, and lifecycle. It maintains a
// registry of loaded plugins per guild and handles event dispatching to the
// appropriate plugin hooks.
type Runtime struct {
	pool     *pgxpool.Pool
	eventBus *events.Bus
	logger   *slog.Logger

	// mu protects the instances map.
	mu        sync.RWMutex
	instances map[string]*PluginInstance // key: guild_id:plugin_id
}

// PluginInstance represents a loaded and ready-to-execute plugin for a specific guild.
type PluginInstance struct {
	PluginID string
	GuildID  string
	Name     string
	Version  string
	Manifest PluginManifest
	Config   json.RawMessage
	Sandbox  *Sandbox
	Enabled  bool
}

// NewRuntime creates a new plugin runtime with database and event bus access.
func NewRuntime(pool *pgxpool.Pool, eventBus *events.Bus, logger *slog.Logger) *Runtime {
	return &Runtime{
		pool:      pool,
		eventBus:  eventBus,
		logger:    logger,
		instances: make(map[string]*PluginInstance),
	}
}

// Start initializes the runtime, loads enabled plugins from the database, and
// begins listening for plugin-relevant events on the event bus.
func (rt *Runtime) Start(ctx context.Context) error {
	rt.logger.Info("starting plugin runtime")

	// Load all enabled guild plugins from the database.
	rows, err := rt.pool.Query(ctx,
		`SELECT gp.guild_id, gp.plugin_id, gp.config, gp.enabled,
		        p.name, p.version, p.manifest, p.wasm_s3_key, p.wasm_hash
		 FROM guild_plugins gp
		 JOIN plugins p ON p.id = gp.plugin_id
		 WHERE gp.enabled = true`)
	if err != nil {
		return fmt.Errorf("loading guild plugins: %w", err)
	}
	defer rows.Close()

	loaded := 0
	for rows.Next() {
		var guildID, pluginID, name, version string
		var config json.RawMessage
		var manifest json.RawMessage
		var enabled bool
		var wasmKey, wasmHash *string

		if err := rows.Scan(&guildID, &pluginID, &config, &enabled,
			&name, &version, &manifest, &wasmKey, &wasmHash); err != nil {
			rt.logger.Error("failed to scan guild plugin", slog.String("error", err.Error()))
			continue
		}

		var pm PluginManifest
		if err := json.Unmarshal(manifest, &pm); err != nil {
			rt.logger.Error("failed to parse plugin manifest",
				slog.String("plugin_id", pluginID),
				slog.String("error", err.Error()))
			continue
		}

		instance := &PluginInstance{
			PluginID: pluginID,
			GuildID:  guildID,
			Name:     name,
			Version:  version,
			Manifest: pm,
			Config:   config,
			Sandbox:  NewSandbox(DefaultSandboxConfig()),
			Enabled:  enabled,
		}

		key := guildID + ":" + pluginID
		rt.mu.Lock()
		rt.instances[key] = instance
		rt.mu.Unlock()
		loaded++
	}

	rt.logger.Info("plugin runtime started", slog.Int("loaded_plugins", loaded))

	// Subscribe to events that plugins care about.
	rt.subscribeEvents()

	return nil
}

// subscribeEvents registers NATS subscriptions for events that plugins can hook into.
func (rt *Runtime) subscribeEvents() {
	hookSubjects := map[HookType]string{
		HookMessageCreate: events.SubjectMessageCreate,
		HookMessageUpdate: events.SubjectMessageUpdate,
		HookMessageDelete: events.SubjectMessageDelete,
		HookMemberJoin:    events.SubjectGuildMemberAdd,
		HookMemberLeave:   events.SubjectGuildMemberRemove,
		HookGuildUpdate:   events.SubjectGuildUpdate,
		HookReactionAdd:   events.SubjectMessageReactionAdd,
		HookReactionDel:   events.SubjectMessageReactionDel,
	}

	for hookType, subject := range hookSubjects {
		ht := hookType
		_, err := rt.eventBus.Subscribe(subject, func(event events.Event) {
			rt.dispatchEvent(ht, event)
		})
		if err != nil {
			rt.logger.Error("failed to subscribe for plugin hook",
				slog.String("hook", string(ht)),
				slog.String("error", err.Error()))
		}
	}
}

// dispatchEvent routes an event to all plugin instances that have registered for the hook.
func (rt *Runtime) dispatchEvent(hookType HookType, event events.Event) {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	for _, instance := range rt.instances {
		if !instance.Enabled {
			continue
		}

		// Check if this plugin subscribes to this hook type.
		subscribes := false
		for _, h := range instance.Manifest.Hooks {
			if h == hookType {
				subscribes = true
				break
			}
		}
		if !subscribes {
			continue
		}

		// Check guild match for guild-scoped events.
		if event.GuildID != "" && event.GuildID != instance.GuildID {
			continue
		}

		// Execute in a separate goroutine with timeout.
		go rt.executePlugin(instance, hookType, event)
	}
}

// executePlugin runs a plugin's hook handler within its sandbox.
func (rt *Runtime) executePlugin(instance *PluginInstance, hookType HookType, event events.Event) {
	start := time.Now()

	pctx := PluginContext{
		GuildID:   instance.GuildID,
		ChannelID: event.ChannelID,
		UserID:    event.UserID,
		HookType:  hookType,
		EventData: event.Data,
		Config:    instance.Config,
	}

	// Execute within sandbox limits.
	result, err := instance.Sandbox.Execute(pctx)

	duration := time.Since(start)
	status := "success"
	errMsg := ""
	actionCount := 0

	if err != nil {
		status = "error"
		errMsg = err.Error()
		rt.logger.Error("plugin execution failed",
			slog.String("plugin", instance.Name),
			slog.String("guild_id", instance.GuildID),
			slog.String("hook", string(hookType)),
			slog.String("error", err.Error()),
			slog.Duration("duration", duration))
	} else if result != nil {
		actionCount = len(result.Actions)
		// Process plugin actions.
		for _, action := range result.Actions {
			rt.processAction(instance, action)
		}
	}

	// Log execution.
	logID := models.NewULID().String()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	rt.pool.Exec(ctx,
		`INSERT INTO plugin_execution_log (id, guild_id, plugin_id, hook_type, status, duration_ms, memory_bytes, error_message, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW())`,
		logID, instance.GuildID, instance.PluginID, string(hookType),
		status, duration.Milliseconds(), instance.Sandbox.MemoryUsed(), nilIfEmpty(errMsg),
	)

	rt.logger.Debug("plugin executed",
		slog.String("plugin", instance.Name),
		slog.String("guild_id", instance.GuildID),
		slog.String("hook", string(hookType)),
		slog.String("status", status),
		slog.Duration("duration", duration),
		slog.Int("actions", actionCount))
}

// processAction handles a single action returned by a plugin execution.
func (rt *Runtime) processAction(instance *PluginInstance, action PluginAction) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	switch action.Type {
	case "send_message":
		var payload struct {
			ChannelID string `json:"channel_id"`
			Content   string `json:"content"`
		}
		if err := json.Unmarshal(action.Payload, &payload); err != nil {
			rt.logger.Error("invalid send_message payload", slog.String("error", err.Error()))
			return
		}
		if payload.ChannelID == "" || payload.Content == "" {
			return
		}

		msgID := models.NewULID().String()
		// Get the plugin's bot user (if any) or use system.
		var authorID string
		err := rt.pool.QueryRow(ctx,
			`SELECT id FROM users WHERE username = $1 AND instance_id = (SELECT id FROM instances LIMIT 1)`,
			"plugin-"+instance.Name,
		).Scan(&authorID)
		if err != nil {
			// Fall back: use guild owner as sender for now.
			rt.pool.QueryRow(ctx,
				`SELECT owner_id FROM guilds WHERE id = $1`, instance.GuildID,
			).Scan(&authorID)
		}
		if authorID == "" {
			return
		}

		_, err = rt.pool.Exec(ctx,
			`INSERT INTO messages (id, channel_id, author_id, content, message_type, flags, created_at)
			 VALUES ($1, $2, $3, $4, 'default', 0, NOW())`,
			msgID, payload.ChannelID, authorID, payload.Content)
		if err != nil {
			rt.logger.Error("failed to send plugin message", slog.String("error", err.Error()))
			return
		}

		// Publish event.
		rt.eventBus.PublishJSON(ctx, events.SubjectMessageCreate, "MESSAGE_CREATE", map[string]string{
			"id":         msgID,
			"channel_id": payload.ChannelID,
			"author_id":  authorID,
			"content":    payload.Content,
		})

	case "add_role":
		var payload struct {
			UserID string `json:"user_id"`
			RoleID string `json:"role_id"`
		}
		if err := json.Unmarshal(action.Payload, &payload); err != nil {
			return
		}
		rt.pool.Exec(ctx,
			`INSERT INTO member_roles (guild_id, user_id, role_id)
			 VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`,
			instance.GuildID, payload.UserID, payload.RoleID)

	case "remove_role":
		var payload struct {
			UserID string `json:"user_id"`
			RoleID string `json:"role_id"`
		}
		if err := json.Unmarshal(action.Payload, &payload); err != nil {
			return
		}
		rt.pool.Exec(ctx,
			`DELETE FROM member_roles WHERE guild_id = $1 AND user_id = $2 AND role_id = $3`,
			instance.GuildID, payload.UserID, payload.RoleID)

	case "react":
		var payload struct {
			MessageID string `json:"message_id"`
			Emoji     string `json:"emoji"`
		}
		if err := json.Unmarshal(action.Payload, &payload); err != nil {
			return
		}
		// Plugins react as the guild owner for now.
		var ownerID string
		rt.pool.QueryRow(ctx,
			`SELECT owner_id FROM guilds WHERE id = $1`, instance.GuildID,
		).Scan(&ownerID)
		if ownerID != "" {
			rt.pool.Exec(ctx,
				`INSERT INTO reactions (message_id, user_id, emoji, created_at)
				 VALUES ($1, $2, $3, NOW()) ON CONFLICT DO NOTHING`,
				payload.MessageID, ownerID, payload.Emoji)
		}

	case "log":
		var payload struct {
			Level   string `json:"level"`
			Message string `json:"message"`
		}
		if err := json.Unmarshal(action.Payload, &payload); err != nil {
			return
		}
		rt.logger.Info("plugin log",
			slog.String("plugin", instance.Name),
			slog.String("guild_id", instance.GuildID),
			slog.String("level", payload.Level),
			slog.String("message", payload.Message))

	default:
		rt.logger.Warn("unknown plugin action type",
			slog.String("type", action.Type),
			slog.String("plugin", instance.Name))
	}
}

// LoadPlugin loads or reloads a plugin for a specific guild.
func (rt *Runtime) LoadPlugin(ctx context.Context, guildID, pluginID string) error {
	var name, version string
	var config, manifest json.RawMessage
	var enabled bool

	err := rt.pool.QueryRow(ctx,
		`SELECT gp.config, gp.enabled,
		        p.name, p.version, p.manifest
		 FROM guild_plugins gp
		 JOIN plugins p ON p.id = gp.plugin_id
		 WHERE gp.guild_id = $1 AND gp.plugin_id = $2`,
		guildID, pluginID,
	).Scan(&config, &enabled, &name, &version, &manifest)
	if err != nil {
		return fmt.Errorf("loading plugin %s for guild %s: %w", pluginID, guildID, err)
	}

	var pm PluginManifest
	if err := json.Unmarshal(manifest, &pm); err != nil {
		return fmt.Errorf("parsing manifest for plugin %s: %w", pluginID, err)
	}

	instance := &PluginInstance{
		PluginID: pluginID,
		GuildID:  guildID,
		Name:     name,
		Version:  version,
		Manifest: pm,
		Config:   config,
		Sandbox:  NewSandbox(DefaultSandboxConfig()),
		Enabled:  enabled,
	}

	key := guildID + ":" + pluginID
	rt.mu.Lock()
	rt.instances[key] = instance
	rt.mu.Unlock()

	rt.logger.Info("plugin loaded",
		slog.String("plugin", name),
		slog.String("version", version),
		slog.String("guild_id", guildID))

	return nil
}

// UnloadPlugin removes a plugin instance for a guild.
func (rt *Runtime) UnloadPlugin(guildID, pluginID string) {
	key := guildID + ":" + pluginID
	rt.mu.Lock()
	delete(rt.instances, key)
	rt.mu.Unlock()
}

// Stop shuts down the plugin runtime and releases all resources.
func (rt *Runtime) Stop() {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	for key, instance := range rt.instances {
		if instance.Sandbox != nil {
			instance.Sandbox.Close()
		}
		delete(rt.instances, key)
	}

	rt.logger.Info("plugin runtime stopped")
}

// nilIfEmpty returns nil if the string is empty, otherwise a pointer to the string.
func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
