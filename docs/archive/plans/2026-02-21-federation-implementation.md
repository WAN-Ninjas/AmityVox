# Federation Full Parity — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Rearchitect federation so federated guilds, channels, users, and DMs are indistinguishable from local ones — one code path, same tables, no branching.

**Architecture:** All major tables gain `instance_id` column (nullable FK to `instances.id`). Federated data lives alongside local data. Write operations on federated rows are signed and forwarded to the home instance. Old cache tables (`federation_guild_cache`, `federation_channel_mirrors`) are dropped. Frontend removes all `isFederatedGuild()` branching.

**Tech Stack:** Go (chi, pgx, NATS, Ed25519), SvelteKit (Svelte 5, TypeScript), PostgreSQL, DragonflyDB

**Design Doc:** `docs/plans/2026-02-21-federation-design.md`

---

## Phase 1: Database Migration & Model Updates

### Task 1: Write Migration `063_federation_full_parity`

**Files:**
- Create: `migrations/063_federation_full_parity.up.sql`
- Create: `migrations/063_federation_full_parity.down.sql`

**Step 1: Write the up migration**

```sql
-- 063_federation_full_parity.up.sql

-- Add instance_id to core tables
ALTER TABLE guilds ADD COLUMN IF NOT EXISTS instance_id TEXT REFERENCES instances(id) ON DELETE CASCADE;
ALTER TABLE channels ADD COLUMN IF NOT EXISTS instance_id TEXT REFERENCES instances(id) ON DELETE CASCADE;
ALTER TABLE roles ADD COLUMN IF NOT EXISTS instance_id TEXT REFERENCES instances(id) ON DELETE CASCADE;
ALTER TABLE guild_members ADD COLUMN IF NOT EXISTS instance_id TEXT REFERENCES instances(id) ON DELETE CASCADE;
ALTER TABLE messages ADD COLUMN IF NOT EXISTS instance_id TEXT REFERENCES instances(id) ON DELETE CASCADE;
ALTER TABLE attachments ADD COLUMN IF NOT EXISTS instance_id TEXT REFERENCES instances(id) ON DELETE CASCADE;
ALTER TABLE embeds ADD COLUMN IF NOT EXISTS instance_id TEXT REFERENCES instances(id) ON DELETE CASCADE;
ALTER TABLE reactions ADD COLUMN IF NOT EXISTS instance_id TEXT REFERENCES instances(id) ON DELETE CASCADE;
ALTER TABLE pins ADD COLUMN IF NOT EXISTS instance_id TEXT REFERENCES instances(id) ON DELETE CASCADE;
ALTER TABLE webhooks ADD COLUMN IF NOT EXISTS instance_id TEXT REFERENCES instances(id) ON DELETE CASCADE;

-- Add partial indexes for federated data queries
CREATE INDEX IF NOT EXISTS idx_guilds_instance_id ON guilds(instance_id) WHERE instance_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_channels_instance_id ON channels(instance_id) WHERE instance_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_messages_instance_id ON messages(instance_id) WHERE instance_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_guild_members_instance_id ON guild_members(instance_id) WHERE instance_id IS NOT NULL;

-- Add shorthand and voice_mode to instances table
ALTER TABLE instances ADD COLUMN IF NOT EXISTS shorthand VARCHAR(5);
ALTER TABLE instances ADD COLUMN IF NOT EXISTS voice_mode VARCHAR(10) DEFAULT 'direct';

-- Add federation_events table for backfill support
CREATE TABLE IF NOT EXISTS federation_events (
    id TEXT PRIMARY KEY,
    instance_id TEXT NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
    event_type TEXT NOT NULL,
    guild_id TEXT,
    channel_id TEXT,
    hlc_wall_ms BIGINT NOT NULL,
    hlc_counter INTEGER NOT NULL DEFAULT 0,
    payload JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_federation_events_hlc ON federation_events(instance_id, hlc_wall_ms, hlc_counter);
CREATE INDEX IF NOT EXISTS idx_federation_events_guild ON federation_events(guild_id) WHERE guild_id IS NOT NULL;

-- Fix delivery receipts: add composite unique constraint
-- First drop the old unique on id if it causes issues (it's PK so skip)
CREATE UNIQUE INDEX IF NOT EXISTS idx_federation_delivery_receipts_composite
    ON federation_delivery_receipts(message_id, source_instance, target_instance);

-- Migrate federation_guild_cache data into real tables (if any exists)
-- This is done in Go code during startup, not in SQL, because it requires
-- parsing channels_json and roles_json JSONB columns into separate rows.

-- Drop old tables AFTER data migration is confirmed (handled in a later migration
-- or via Go startup code that checks if migration is needed)
-- For now, just mark them as deprecated by adding a comment column
-- The actual DROP happens in Task 2 after Go migration code runs.
```

**Step 2: Write the down migration**

```sql
-- 063_federation_full_parity.down.sql

DROP INDEX IF EXISTS idx_federation_events_guild;
DROP INDEX IF EXISTS idx_federation_events_hlc;
DROP TABLE IF EXISTS federation_events;

DROP INDEX IF EXISTS idx_federation_delivery_receipts_composite;

ALTER TABLE instances DROP COLUMN IF EXISTS voice_mode;
ALTER TABLE instances DROP COLUMN IF EXISTS shorthand;

DROP INDEX IF EXISTS idx_guild_members_instance_id;
DROP INDEX IF EXISTS idx_messages_instance_id;
DROP INDEX IF EXISTS idx_channels_instance_id;
DROP INDEX IF EXISTS idx_guilds_instance_id;

ALTER TABLE webhooks DROP COLUMN IF EXISTS instance_id;
ALTER TABLE pins DROP COLUMN IF EXISTS instance_id;
ALTER TABLE reactions DROP COLUMN IF EXISTS instance_id;
ALTER TABLE embeds DROP COLUMN IF EXISTS instance_id;
ALTER TABLE attachments DROP COLUMN IF EXISTS instance_id;
ALTER TABLE messages DROP COLUMN IF EXISTS instance_id;
ALTER TABLE guild_members DROP COLUMN IF EXISTS instance_id;
ALTER TABLE roles DROP COLUMN IF EXISTS instance_id;
ALTER TABLE channels DROP COLUMN IF EXISTS instance_id;
ALTER TABLE guilds DROP COLUMN IF EXISTS instance_id;
```

**Step 3: Verify migration applies cleanly**

Run: `docker run --rm -v $(pwd):/build -w /build -e GOTOOLCHAIN=auto golang:1.24-alpine go test ./internal/database/... -run TestMigrations -v`

If no migration test exists, verify manually by checking the migration files parse correctly.

**Step 4: Commit**

```
feat: add migration 063 for federation full parity
```

---

### Task 2: Update Go Models with InstanceID

**Files:**
- Modify: `internal/models/models.go` (or wherever Guild, Channel, Role, Message structs are defined)

**Step 1: Add InstanceID field to all relevant model structs**

Add `InstanceID *string` field to: Guild, Channel, Role, GuildMember, Message (if not already present). The field should be a pointer (`*string`) so it's nullable and omitted from JSON when nil.

```go
type Guild struct {
    // ... existing fields ...
    InstanceID *string `json:"instance_id,omitempty"`
}

type Channel struct {
    // ... existing fields ...
    InstanceID *string `json:"instance_id,omitempty"`
}

type Role struct {
    // ... existing fields ...
    InstanceID *string `json:"instance_id,omitempty"`
}
```

**Step 2: Add InstanceID to scan lists in all query functions**

Search for every `Scan()` call that reads Guild, Channel, Role, GuildMember, Message rows. Add `&model.InstanceID` to the scan list. Also update every `INSERT` and `UPDATE` query to include the `instance_id` column.

Key files to update:
- `internal/api/guilds/` — all guild handlers
- `internal/api/channels/` — all channel handlers
- `internal/api/messages/` — all message handlers
- `internal/database/` — any shared query functions

**Step 3: Add helper functions**

```go
// IsFederated returns true if this entity belongs to a remote instance.
func (g *Guild) IsFederated() bool {
    return g.InstanceID != nil
}

func (c *Channel) IsFederated() bool {
    return c.InstanceID != nil
}
```

**Step 4: Run Go tests**

Run: `docker run --rm -v $(pwd):/build -w /build -e GOTOOLCHAIN=auto golang:1.24-alpine go test ./internal/... -count=1`

**Step 5: Commit**

```
feat: add InstanceID field to Guild, Channel, Role, Message models
```

---

### Task 3: Add Instance Shorthand & VoiceMode to Instance Model and Discovery

**Files:**
- Modify: `internal/federation/federation.go` — add Shorthand and VoiceMode to discovery response
- Modify: `internal/models/models.go` — add fields to Instance struct (if exists)
- Modify: `amityvox.example.toml` — add shorthand and voice_mode config options

**Step 1: Update Instance model**

```go
type Instance struct {
    // ... existing fields ...
    Shorthand *string `json:"shorthand,omitempty"`
    VoiceMode string  `json:"voice_mode,omitempty"` // "direct" or "relay"
}
```

**Step 2: Update discovery response to include shorthand and voice_mode**

In `HandleDiscovery()`, add shorthand and voice_mode fields to the JSON response.

**Step 3: Update config parsing**

Add `shorthand` and `voice_mode` to `[federation]` section in config. Add admin panel DB override support (check DB first, fall back to config).

**Step 4: Add shorthand collision resolution**

When registering a remote instance (in `RegisterRemoteInstance()`), check if any existing peer has the same shorthand. If so, append a numeric suffix to the newer one.

```go
func (s *Service) resolveShorthandCollision(ctx context.Context, shorthand string, instanceID string) string {
    // Query existing peers with same shorthand, excluding this instance
    // If collision, try shorthand+"1", shorthand+"2", etc.
    // Return resolved shorthand
}
```

**Step 5: Update amityvox.example.toml**

```toml
[federation]
mode = "closed"
shorthand = ""        # Max 5 chars, shown on federated peers (e.g., "AV", "DEV")
voice_mode = "direct" # "direct" = connect to remote LiveKit, "relay" = proxy through local
```

**Step 6: Run tests, commit**

```
feat: add instance shorthand and voice_mode to federation config
```

---

## Phase 2: Backend — Federated Guild Sync (Same Tables)

### Task 4: Rewrite Guild Join to Store in Real Tables

**Files:**
- Modify: `internal/federation/guild.go` — `HandleFederatedGuildJoin()`

**Step 1: Rewrite the guild join handler**

When a remote user joins a local guild, the current code works fine (it stores in real tables). The change is on the **response** side — when a local user joins a remote guild, the local instance must store the returned guild data in real `guilds`, `channels`, `roles` tables (not `federation_guild_cache`).

Find the local proxy handler that calls the remote `/federation/v1/guilds/{guildID}/join` endpoint. After receiving the response (which includes full guild structure with channels and roles):

```go
// Instead of inserting into federation_guild_cache, insert into real tables:
err := apiutil.WithTx(ctx, h.Pool, func(tx pgx.Tx) error {
    // 1. INSERT INTO guilds (..., instance_id) VALUES (..., remoteInstanceID)
    //    ON CONFLICT (id) DO UPDATE SET name=..., icon_id=..., etc.

    // 2. For each channel in response:
    //    INSERT INTO channels (..., instance_id) VALUES (...)
    //    ON CONFLICT (id) DO UPDATE

    // 3. For each role in response:
    //    INSERT INTO roles (..., instance_id) VALUES (...)
    //    ON CONFLICT (id) DO UPDATE

    // 4. INSERT INTO guild_members (guild_id, user_id, instance_id)
    //    for the joining user

    return nil
})
```

**Step 2: Wrap the guild member insert + count update in a transaction**

Fix the race condition (Issue #1 from audit):

```go
err := apiutil.WithTx(ctx, h.Pool, func(tx pgx.Tx) error {
    tag, err := tx.Exec(ctx,
        `INSERT INTO guild_members (guild_id, user_id, joined_at)
         VALUES ($1, $2, now()) ON CONFLICT DO NOTHING`, guildID, userID)
    if err != nil {
        return fmt.Errorf("inserting guild member: %w", err)
    }
    if tag.RowsAffected() > 0 {
        _, err = tx.Exec(ctx,
            `UPDATE guilds SET member_count = member_count + 1 WHERE id = $1`, guildID)
        if err != nil {
            return fmt.Errorf("incrementing member count: %w", err)
        }
    }
    return nil
})
```

**Step 3: Remove all `federation_guild_cache` INSERT calls**

Search for `federation_guild_cache` in guild.go and remove all insertions. Replace with real table inserts.

**Step 4: Run tests, commit**

```
feat: store federated guilds in real tables instead of cache
```

---

### Task 5: Rewrite Inbound Event Handlers to Update Real Tables

**Files:**
- Modify: `internal/federation/sync.go` — `persistInboundMessage()`, inbox handler, `routeEvent()`
- Modify: `internal/federation/guild.go` — `updateGuildCacheFromEvent()` → delete entirely

**Step 1: Fix mandatory GuildID in event envelopes**

In `routeEvent()`, remove the fallback GuildID extraction logic. Instead, REQUIRE GuildID to be set before federation. In the local event handlers that publish to NATS, ensure they always include GuildID:

```go
// In routeEvent():
if event.GuildID == "" && event.ChannelID != "" {
    // Look up guild_id from channels table
    var guildID *string
    err := ss.fed.pool.QueryRow(ctx,
        `SELECT guild_id FROM channels WHERE id = $1`, event.ChannelID,
    ).Scan(&guildID)
    if err != nil || guildID == nil {
        ss.logger.Error("cannot route event: no guild_id for channel",
            slog.String("channel_id", event.ChannelID),
            slog.String("event_type", event.Type))
        return // Reject, don't silently drop
    }
    event.GuildID = *guildID
}
```

**Step 2: Rewrite `persistInboundMessage()` to use real channel IDs**

Remove the channel mirror lookup. Since federated channels are now stored as real rows in the `channels` table (with `instance_id` set), the channel ID in the event IS the real channel ID:

```go
func (ss *SyncService) persistInboundMessage(ctx context.Context, msg FederatedMessage) {
    // No more mirror lookup — channel ID is the real channel ID
    channelID := msg.ChannelID

    // Verify channel exists locally (it should if guild sync worked)
    var exists bool
    err := ss.fed.pool.QueryRow(ctx,
        `SELECT EXISTS(SELECT 1 FROM channels WHERE id = $1)`, channelID,
    ).Scan(&exists)
    if err != nil || !exists {
        ss.logger.Error("inbound message for unknown channel",
            slog.String("channel_id", channelID),
            slog.String("guild_id", msg.GuildID))
        return
    }

    // Insert message into real messages table
    // ... (existing insert logic, but without mirror ID translation)
}
```

**Step 3: Delete `updateGuildCacheFromEvent()` entirely**

Replace with real table updates. When CHANNEL_CREATE arrives, insert into `channels`. When CHANNEL_UPDATE arrives, update `channels`. When GUILD_UPDATE arrives, update `guilds`. These are the same operations as local events — just guarded by `instance_id IS NOT NULL`.

**Step 4: Store federation events for backfill**

After processing each inbound event, store it in `federation_events` table:

```go
ss.fed.pool.Exec(ctx,
    `INSERT INTO federation_events (id, instance_id, event_type, guild_id, channel_id, hlc_wall_ms, hlc_counter, payload)
     VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
    models.NewULID().String(), msg.OriginID, msg.Type, msg.GuildID, msg.ChannelID,
    msg.Timestamp.WallMs, msg.Timestamp.Counter, payloadJSON)
```

**Step 5: Run tests, commit**

```
feat: rewrite federation event handlers to use real tables
```

---

### Task 6: Fix Presence Sync

**Files:**
- Modify: `internal/federation/sync.go` — presence guild_ids population
- Modify: `internal/gateway/gateway.go` — presence dispatch for remote users

**Step 1: Fix guild_ids query error handling (fail-closed)**

```go
rows, err := ss.fed.pool.Query(ctx,
    `SELECT guild_id FROM guild_members WHERE user_id = $1`, event.UserID)
if err != nil {
    ss.logger.Error("failed to query guild memberships for presence",
        slog.String("user_id", event.UserID),
        slog.String("error", err.Error()))
    return // Don't send presence with empty guild_ids
}
defer rows.Close()

var guildIDs []string
for rows.Next() {
    var gid string
    if err := rows.Scan(&gid); err != nil {
        ss.logger.Warn("failed to scan guild_id", slog.String("error", err.Error()))
        continue
    }
    guildIDs = append(guildIDs, gid)
}
if err := rows.Err(); err != nil {
    ss.logger.Error("rows iteration error for presence guild_ids",
        slog.String("error", err.Error()))
    return
}
```

**Step 2: Fix gateway presence dispatch for remote users**

In `shouldDispatchTo()`, when handling PRESENCE_UPDATE for a user who is remote (no local WebSocket client), extract `guild_ids` from the event data and use them for guild membership intersection:

```go
// For remote users, guild_ids comes from the event payload, not local client state
if len(eventUserGuildIDs) == 0 {
    // Try extracting from event data
    if data, ok := event.Data.(map[string]interface{}); ok {
        if gids, ok := data["guild_ids"].([]interface{}); ok {
            for _, gid := range gids {
                if s, ok := gid.(string); ok {
                    eventUserGuildIDs = append(eventUserGuildIDs, s)
                }
            }
        }
    }
}
```

**Step 3: Run tests, commit**

```
fix: presence sync fail-closed on query errors, support remote user dispatch
```

---

### Task 7: Add Media Proxy Endpoint

**Files:**
- Create: `internal/api/federation/media.go` (or add to existing federation handler file)
- Modify: `internal/api/server.go` — add route

**Step 1: Write the media proxy handler**

```go
// HandleMediaProxy proxies media requests for federated content.
// GET /api/v1/federation/media/{instanceId}/{fileId}
func (h *FederationHandler) HandleMediaProxy(w http.ResponseWriter, r *http.Request) {
    instanceID := chi.URLParam(r, "instanceId")
    fileID := chi.URLParam(r, "fileId")

    // 1. Check DragonflyDB cache first
    cacheKey := fmt.Sprintf("fed:media:%s:%s", instanceID, fileID)
    // If cached, stream from cache

    // 2. Look up instance domain
    var domain string
    err := h.Pool.QueryRow(r.Context(),
        `SELECT domain FROM instances WHERE id = $1`, instanceID,
    ).Scan(&domain)
    if err != nil {
        http.Error(w, "Unknown instance", http.StatusNotFound)
        return
    }

    // 3. Fetch from remote instance
    remoteURL := fmt.Sprintf("https://%s/api/v1/files/%s", domain, fileID)
    resp, err := h.httpClient.Get(remoteURL)
    if err != nil {
        http.Error(w, "Failed to fetch remote media", http.StatusBadGateway)
        return
    }
    defer resp.Body.Close()

    // 4. Stream to client, cache in DragonflyDB
    w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
    w.Header().Set("Cache-Control", "public, max-age=3600")

    // Use io.TeeReader to simultaneously stream to client and cache
    // For files > 1MB, cache to disk instead of DragonflyDB
    io.Copy(w, resp.Body)
}
```

**Step 2: Wire route in server.go**

```go
r.Get("/api/v1/federation/media/{instanceId}/{fileId}", federationHandler.HandleMediaProxy)
```

**Step 3: Run tests, commit**

```
feat: add federation media proxy with caching
```

---

### Task 8: Add Backfill Sync Endpoint

**Files:**
- Add handler: `internal/federation/sync.go` — `HandleSync()`
- Modify: `internal/api/server.go` — add route

**Step 1: Write the sync handler (remote-facing)**

```go
// HandleSync replays events since a given HLC timestamp for backfill after reconnect.
// POST /federation/v1/sync
func (ss *SyncService) HandleSync(w http.ResponseWriter, r *http.Request) {
    // 1. Verify signed request (same as HandleInbox)
    // 2. Parse request: { last_seen_hlc: { wall_ms, counter }, guild_ids: [...] }
    // 3. Query federation_events table:
    //    SELECT * FROM federation_events
    //    WHERE (hlc_wall_ms > $1 OR (hlc_wall_ms = $1 AND hlc_counter > $2))
    //    AND guild_id = ANY($3)
    //    ORDER BY hlc_wall_ms, hlc_counter
    //    LIMIT 1000
    // 4. Return events array + truncated flag
}
```

**Step 2: Write the sync requester (called on peer reconnect)**

```go
// RequestBackfill sends a sync request to a peer after reconnection.
func (ss *SyncService) RequestBackfill(ctx context.Context, peerID string) error {
    // 1. Get last_synced_at from federation_peers
    // 2. Convert to HLC timestamp
    // 3. Get guild_ids where peer has members
    // 4. POST /federation/v1/sync to peer
    // 5. Process returned events through persistInbound*
    // 6. Update last_synced_at
}
```

**Step 3: Trigger backfill on peer reconnection**

In the health check / peer status update flow, detect when a peer transitions from degraded/offline to healthy and trigger backfill.

**Step 4: Wire route, run tests, commit**

```
feat: add federation backfill sync endpoint for reconnection recovery
```

---

## Phase 3: Remote Guild Management

### Task 9: Add Management Proxy Endpoint

**Files:**
- Create or modify: `internal/federation/manage.go`
- Modify: `internal/api/server.go`

**Step 1: Write the remote management handler (remote-facing)**

```go
// HandleManage processes remote guild management requests.
// POST /federation/v1/guilds/{guildID}/manage
func (ss *SyncService) HandleManage(w http.ResponseWriter, r *http.Request) {
    // 1. Verify signed request
    // 2. Parse action + data
    // 3. Verify the requesting user has the required permission on this guild
    // 4. Execute the action (same code as local handlers):
    //    - channel_create, channel_update, channel_delete
    //    - role_create, role_update, role_delete
    //    - member_kick, member_ban, member_unban
    //    - guild_update
    //    - webhook_create, webhook_update, webhook_delete
    //    - thread_create, thread_archive, thread_delete
    //    - message_delete, message_pin, member_timeout
    // 5. Return result
    // 6. The local handler already publishes NATS events,
    //    which the federation router will deliver to peers
}
```

**Step 2: Write the local proxy (authenticated user calling remote)**

Modify existing API handlers (guild update, channel create, etc.) to detect `instance_id` on the guild and forward to the home instance:

```go
// In existing handlers, add at the top:
if guild.IsFederated() {
    // Forward to home instance via federation management endpoint
    h.forwardToHomeInstance(w, r, guild, "channel_create", req)
    return
}
```

**Step 3: Write `forwardToHomeInstance()` helper**

```go
func (h *Handler) forwardToHomeInstance(w http.ResponseWriter, r *http.Request, guild *models.Guild, action string, data interface{}) {
    // 1. Look up home instance domain from guild.InstanceID
    // 2. Build management request: { action, data, user_id, permissions }
    // 3. Sign with federation service
    // 4. POST to /federation/v1/guilds/{guildID}/manage
    // 5. Return response to client
}
```

**Step 4: Wire routes, run tests, commit**

```
feat: add remote guild management via federation proxy
```

---

## Phase 4: Frontend — Remove Federation Branching

### Task 10: Update TypeScript Types

**Files:**
- Modify: `web/src/lib/types/index.ts`

**Step 1: Remove `FederatedGuild` interface**

The `Guild` interface already has (or will have) `instance_id`. Remove the separate `FederatedGuild` type entirely:

```typescript
// DELETE this entire interface:
// export interface FederatedGuild { ... }

// ENSURE Guild has instance_id:
export interface Guild {
    // ... existing fields ...
    instance_id: string | null;  // null = local, populated = federated
}
```

**Step 2: Add instance shorthand to types**

```typescript
export interface Instance {
    id: string;
    domain: string;
    name: string;
    shorthand: string | null;
    voice_mode: 'direct' | 'relay';
    // ... other fields
}
```

**Step 3: Remove FederatedGuild from ReadyEvent**

The READY payload should now include federated guilds in the regular `guilds` array (they have `instance_id` set). Remove the separate `federated_guilds` field.

**Step 4: Commit**

```
refactor: remove FederatedGuild type, add instance_id to Guild
```

---

### Task 11: Remove Federated Guild Store & Merge Into Regular Guild Store

**Files:**
- Modify: `web/src/lib/stores/guilds.ts`

**Step 1: Remove all federation-specific store code**

Delete:
- `federatedGuilds` Map store
- `federatedGuildIds` derived set
- `isFederatedGuild()` helper function
- `loadFederatedGuilds()` function
- `addFederatedGuild()` function
- `removeFederatedGuild()` function

**Step 2: Ensure regular guild store handles `instance_id`**

Guilds with `instance_id` set are loaded into the same store as local guilds. No special handling needed — they're just guilds.

**Step 3: Add helper for checking if guild is federated (on the Guild object, not a separate function)**

```typescript
// Utility, not a store function:
export function isGuildFederated(guild: Guild): boolean {
    return guild.instance_id != null;
}
```

**Step 4: Update READY event handler**

In the gateway/WebSocket handler that processes the READY event, remove the separate `federated_guilds` processing. Federated guilds come in the regular `guilds` array.

**Step 5: Run frontend tests, fix any failures**

Run: `docker run --rm -v $(pwd)/web:/web -w /web node:24-alpine npx vitest run`

**Step 6: Commit**

```
refactor: merge federated guilds into regular guild store
```

---

### Task 12: Remove Federation Branching from API Client

**Files:**
- Modify: `web/src/lib/api/client.ts`

**Step 1: Remove duplicate federation API methods**

The following methods should be removed and their callers updated to use the regular (local) methods, which will now detect `instance_id` on the backend and route accordingly:

Delete:
- `getFederatedGuildMessages()` → callers use `getMessages()`
- `sendFederatedGuildMessage()` → callers use `sendMessage()`
- `getFederatedGuildMembers()` → callers use `getGuildMembers()`
- `addFederatedReaction()` → callers use `addReaction()`
- `removeFederatedReaction()` → callers use `removeReaction()`
- `sendFederatedTyping()` → callers use `sendTyping()`

Keep (these are genuinely different operations, not duplicates):
- `joinFederatedGuild()` — joining a remote guild is a different flow
- `leaveFederatedGuild()` — leaving a remote guild
- `joinFederatedVoice()` / `joinFederatedVoiceByGuild()` — voice token exchange
- `getPublicFederationPeers()` — peer discovery
- `discoverRemoteGuilds()` — guild search
- `ensureFederatedUser()` — user stub creation

**Step 2: Update all callers**

Search all `.svelte` and `.ts` files for the deleted method names. Replace with the regular method (which no longer needs a `federatedGuildId` parameter since the backend detects it from the guild's `instance_id`).

**Step 3: Run frontend tests, commit**

```
refactor: remove duplicate federation API methods
```

---

### Task 13: Remove Federation Branching from Components

**Files:**
- Modify: all components that have `if (isFederatedGuild)` or `if (federatedGuildId)` branching

**Step 1: Search for all federation branching in components**

```
grep -r "isFederatedGuild\|federatedGuild\|federated_guild\|instance_domain" web/src/lib/components/
```

**Step 2: Remove each branch**

For each component found, remove the federation-specific code path. The component should use the same logic for all guilds. If it needs to know the guild is federated (e.g., to show a badge), check `guild.instance_id != null` instead of using a separate store.

**Step 3: Update message loading**

In message stores/components, remove the `federatedGuildId` parameter from `loadMessages()`. The backend now handles routing based on the channel's `instance_id`.

**Step 4: Run frontend tests, commit**

```
refactor: remove federation branching from UI components
```

---

### Task 14: Add Federation Badge to Guild Icons

**Files:**
- Modify: `web/src/lib/components/layout/GuildSidebar.svelte` (or wherever guild icons are rendered)
- Modify: `web/src/lib/components/guild/GuildHeader.svelte` (or equivalent)

**Step 1: Add badge overlay to guild icon in sidebar**

When `guild.instance_id != null`, render a prominent badge on the guild icon showing the instance shorthand:

```svelte
{#if guild.instance_id}
    <div class="federation-badge">
        {getInstanceShorthand(guild.instance_id)}
    </div>
{/if}
```

Style: colored banner/ribbon across bottom of guild icon, large enough to be immediately obvious.

**Step 2: Show instance domain in guild header**

When viewing a federated guild, show the instance domain under the guild name in the header.

**Step 3: Add badge to federated user avatars in member list**

Smaller version of the badge on user avatars for federated members.

**Step 4: Create instance shorthand lookup store**

```typescript
// Store that maps instance_id → shorthand
// Populated from instances data in READY or via API
export const instanceShorthands = writable(new Map<string, string>());
```

**Step 5: Run frontend tests, commit**

```
feat: add federation badges with instance shorthand
```

---

### Task 15: Update Media URLs to Use Proxy

**Files:**
- Modify: `web/src/lib/utils/media.ts` (or wherever `buildAvatarUrl`, `buildMediaUrl` are defined)
- Modify: `web/src/lib/api/client.ts` (if URL building is there)

**Step 1: Update URL builders to detect instance_id and route through proxy**

```typescript
export function buildAvatarUrl(avatarId: string | null, instanceId?: string | null): string {
    if (!avatarId) return '/default-avatar.png';
    if (instanceId) {
        return `/api/v1/federation/media/${instanceId}/${avatarId}`;
    }
    return `/api/v1/files/${avatarId}`;
}

export function buildMediaUrl(fileId: string, instanceId?: string | null): string {
    if (instanceId) {
        return `/api/v1/federation/media/${instanceId}/${fileId}`;
    }
    return `/api/v1/files/${fileId}`;
}
```

**Step 2: Update all avatar/media rendering to pass instance_id**

Search for `buildAvatarUrl` and `buildMediaUrl` calls. Pass the user's or guild's `instance_id` where available.

**Step 3: Run frontend tests, commit**

```
feat: route federated media through local proxy
```

---

## Phase 5: Backend — Write Routing for Federated Mutations

### Task 16: Add Write-Routing Middleware/Helper

**Files:**
- Create: `internal/api/federation/proxy.go` (or add to existing)

**Step 1: Write the federation write-routing helper**

This is the core function that existing handlers call when they detect a mutation on a federated entity:

```go
// ForwardToHomeInstance proxies a mutation request to the guild's home instance.
// Called by local API handlers when guild.InstanceID is set.
func ForwardToHomeInstance(
    ctx context.Context,
    fedService *federation.Service,
    pool *pgxpool.Pool,
    instanceID string,
    action string,
    data interface{},
    userID string,
) (*http.Response, error) {
    // 1. Look up instance domain
    // 2. Build management payload: { action, data, user_id }
    // 3. Sign payload
    // 4. POST to /federation/v1/guilds/{guildID}/manage
    // 5. Return response
}
```

**Step 2: Update existing API handlers to detect instance_id and forward**

For each handler that modifies guild data (channel CRUD, role CRUD, guild settings, message operations), add an early check:

```go
func (h *Handler) HandleUpdateChannel(w http.ResponseWriter, r *http.Request) {
    channelID := chi.URLParam(r, "channelID")

    // Load channel
    channel, err := h.getChannel(r.Context(), channelID)
    if err != nil { ... }

    // If federated, forward to home instance
    if channel.IsFederated() {
        resp, err := federation.ForwardToHomeInstance(
            r.Context(), h.Federation, h.Pool, *channel.InstanceID,
            "channel_update", req, userID)
        // Write response back to client
        return
    }

    // ... existing local handling ...
}
```

**Step 3: Run tests, commit**

```
feat: add write-routing to forward federated mutations to home instance
```

---

## Phase 6: MLS Encryption Across Federation

### Task 17: Add MLS Federation Endpoints

**Files:**
- Create: `internal/federation/mls.go`
- Modify: `internal/api/server.go`

**Step 1: Write MLS key package handler**

```go
// HandleMLSKeyPackage handles a federated user requesting to join an MLS group.
// POST /federation/v1/guilds/{guildID}/channels/{channelID}/mls/key-package
func (ss *SyncService) HandleMLSKeyPackage(w http.ResponseWriter, r *http.Request) {
    // 1. Verify signed request
    // 2. Verify user is guild member
    // 3. Add user to MLS group via encryption service
    // 4. Return welcome message + key material
}
```

**Step 2: Write MLS commit and welcome handlers**

Similar pattern for `HandleMLSCommit` and `HandleMLSWelcome`.

**Step 3: Write local proxy endpoints**

Authenticated endpoints that forward MLS requests to the home instance for federated channels.

**Step 4: Wire routes, run tests, commit**

```
feat: add MLS encryption endpoints for federation
```

---

## Phase 7: Voice/Video Relay Mode

### Task 18: Implement LiveKit Relay Mode

**Files:**
- Modify: `internal/federation/voice.go`
- Modify: federation config handling

**Step 1: Add relay mode logic**

When `voice_mode = "relay"`, instead of returning the remote LiveKit URL to the client, the local instance:
1. Requests a token from the remote instance (same as direct mode)
2. Establishes a bridge connection from local LiveKit to remote LiveKit
3. Generates a LOCAL LiveKit token for the client
4. Client connects to local LiveKit, which relays to remote

**Step 2: Add config-based routing**

```go
func (ss *SyncService) HandleVoiceTokenRequest(...) {
    voiceMode := ss.getVoiceMode() // Check DB first, then config

    if voiceMode == "relay" {
        // Generate local token, set up relay
        ss.handleVoiceRelay(w, r, remoteToken, remoteLiveKitURL)
    } else {
        // Return remote token + URL directly (existing behavior)
        ss.handleVoiceDirect(w, r, remoteToken, remoteLiveKitURL)
    }
}
```

**Step 3: Run tests, commit**

```
feat: add LiveKit relay mode for federation voice privacy
```

---

## Phase 8: Guild Discovery & Cross-Instance Invites

### Task 19: Aggregated Guild Discovery

**Files:**
- Modify: `internal/federation/guild.go` or create `internal/federation/discover.go`
- Modify: frontend discovery component

**Step 1: Write aggregated discovery handler**

```go
// HandleAggregatedDiscover fans out search to all active peers and merges results.
// GET /api/v1/federation/discover?q=&tag=&limit=
func (h *Handler) HandleAggregatedDiscover(w http.ResponseWriter, r *http.Request) {
    query := r.URL.Query().Get("q")

    // 1. Get all active peers
    // 2. Fan out discovery requests in parallel (goroutines with context + timeout)
    // 3. Collect results with 5-second timeout per peer
    // 4. Merge, deduplicate by (instance_id, guild_id)
    // 5. Add instance shorthand to each result
    // 6. Return merged results
}
```

**Step 2: Update frontend discovery to use aggregated endpoint**

**Step 3: Run tests, commit**

```
feat: aggregated guild discovery across all federation peers
```

---

### Task 20: Cross-Instance Invite Resolution

**Files:**
- Create: `internal/federation/invites.go`
- Modify: `internal/api/server.go`

**Step 1: Write invite resolution endpoint (remote-facing)**

```go
// HandleInviteResolve resolves an invite code and returns guild preview.
// GET /federation/v1/invites/{code}
func (ss *SyncService) HandleInviteResolve(w http.ResponseWriter, r *http.Request) {
    // 1. Verify signed request
    // 2. Look up invite code in invites table
    // 3. Return guild preview (name, icon, member count, description)
}
```

**Step 2: Write invite accept endpoint (remote-facing)**

```go
// HandleInviteAccept accepts a cross-instance invite.
// POST /federation/v1/invites/{code}/accept
func (ss *SyncService) HandleInviteAccept(w http.ResponseWriter, r *http.Request) {
    // 1. Verify signed request
    // 2. Look up invite, verify not expired/exhausted
    // 3. Create user stub for remote user
    // 4. Add to guild_members
    // 5. Register channel peers
    // 6. Return full guild structure
}
```

**Step 3: Write local proxy for invite resolution**

```go
// User clicks a federated invite link → local instance resolves it
// POST /api/v1/federation/invites/resolve
func (h *Handler) HandleResolveInvite(w http.ResponseWriter, r *http.Request) {
    // 1. Parse invite URL to extract domain + code
    // 2. Discover remote instance
    // 3. Sign and send resolve request
    // 4. Return preview to user
}
```

**Step 4: Wire routes, run tests, commit**

```
feat: cross-instance invite resolution and acceptance
```

---

## Phase 9: Cleanup & Drop Old Tables

### Task 21: Drop Old Federation Tables & Dead Code

**Files:**
- Create: `migrations/064_drop_federation_cache.up.sql`
- Create: `migrations/064_drop_federation_cache.down.sql`
- Modify: All files that reference `federation_guild_cache` or `federation_channel_mirrors`

**Step 1: Write migration to drop old tables**

```sql
-- 064_drop_federation_cache.up.sql
DROP TABLE IF EXISTS federation_guild_cache;
DROP TABLE IF EXISTS federation_channel_mirrors;
```

```sql
-- 064_drop_federation_cache.down.sql
CREATE TABLE IF NOT EXISTS federation_guild_cache (
    guild_id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    instance_id TEXT NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    icon_id TEXT,
    description TEXT,
    member_count INTEGER DEFAULT 0,
    channels_json JSONB DEFAULT '[]',
    roles_json JSONB DEFAULT '[]',
    cached_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (guild_id, user_id)
);

CREATE TABLE IF NOT EXISTS federation_channel_mirrors (
    local_channel_id TEXT NOT NULL,
    remote_channel_id TEXT NOT NULL,
    remote_instance_id TEXT NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
    PRIMARY KEY (local_channel_id, remote_channel_id, remote_instance_id)
);
```

**Step 2: Remove all Go code referencing dropped tables**

Search and remove:
- All `federation_guild_cache` queries
- All `federation_channel_mirrors` queries
- `updateGuildCacheFromEvent()` function (should already be removed in Task 5)
- Any helper functions that only served the old cache

**Step 3: Remove all frontend code referencing old types**

Search and remove:
- Any remaining `FederatedGuild` type usage
- `channels_json` / `roles_json` parsing
- `federation_guild_cache` references in admin panel (if any)

**Step 4: Run full test suite**

Run: `docker run --rm -v $(pwd)/web:/web -w /web node:24-alpine npx vitest run`
Run: `docker run --rm -v $(pwd):/build -w /build -e GOTOOLCHAIN=auto golang:1.24-alpine go test ./internal/... -count=1`

**Step 5: Commit**

```
refactor: drop federation_guild_cache and federation_channel_mirrors tables
```

---

## Phase 10: Backend Write-Routing for All Operations

### Task 22: Wire Write-Routing into All Mutating Handlers

**Files:**
- Modify: `internal/api/guilds/` — all mutating guild handlers
- Modify: `internal/api/channels/` — all mutating channel handlers
- Modify: `internal/api/messages/` — message delete, pin handlers

**Step 1: List all mutating endpoints that need write-routing**

- `PATCH /api/v1/guilds/{guildID}` — guild update
- `DELETE /api/v1/guilds/{guildID}` — guild delete (owner only, not remote)
- `POST /api/v1/guilds/{guildID}/channels` — channel create
- `PATCH /api/v1/channels/{channelID}` — channel update
- `DELETE /api/v1/channels/{channelID}` — channel delete
- `POST /api/v1/guilds/{guildID}/roles` — role create
- `PATCH /api/v1/guilds/{guildID}/roles/{roleID}` — role update
- `DELETE /api/v1/guilds/{guildID}/roles/{roleID}` — role delete
- `PUT /api/v1/guilds/{guildID}/members/{userID}/roles/{roleID}` — assign role
- `DELETE /api/v1/guilds/{guildID}/members/{userID}/roles/{roleID}` — remove role
- `DELETE /api/v1/guilds/{guildID}/members/{userID}` — kick member
- `PUT /api/v1/guilds/{guildID}/bans/{userID}` — ban member
- `DELETE /api/v1/guilds/{guildID}/bans/{userID}` — unban member
- `DELETE /api/v1/channels/{channelID}/messages/{messageID}` — delete message
- `PUT /api/v1/channels/{channelID}/pins/{messageID}` — pin message
- `DELETE /api/v1/channels/{channelID}/pins/{messageID}` — unpin message

**Step 2: For each handler, add federation check at the top**

Pattern:
```go
guild, err := h.getGuild(ctx, guildID)
if err != nil { ... }
if guild.IsFederated() {
    federation.ForwardToHomeInstance(ctx, h.Fed, h.Pool, *guild.InstanceID, actionName, requestBody, userID)
    return
}
// ... existing local logic ...
```

**Step 3: Run tests, commit**

```
feat: wire write-routing into all mutating API handlers
```

---

## Phase 11: Admin Panel Federation Config

### Task 23: Add Federation Config to Admin Panel

**Files:**
- Modify: admin panel frontend components
- Modify: `internal/api/admin/` — admin federation handlers

**Step 1: Add DB-backed federation config storage**

Admin panel settings for `federation_mode`, `shorthand`, `voice_mode` should save to a `federation_config` table (or use the existing instance settings):

```sql
-- In existing instances table, add columns or use a config table
-- shorthand and voice_mode already added in migration 063
```

**Step 2: Add admin API endpoint to update federation config**

```go
// PATCH /api/v1/admin/federation/config
func (h *AdminHandler) HandleUpdateFederationConfig(w http.ResponseWriter, r *http.Request) {
    // Parse: { federation_mode, shorthand, voice_mode }
    // Update instances table for local instance
    // These values now override amityvox.toml
}
```

**Step 3: Update frontend admin panel**

Add federation configuration section to admin panel with fields for mode, shorthand, voice_mode.

**Step 4: Run tests, commit**

```
feat: admin panel federation config with DB override
```

---

## Phase 12: User Profile Sync

### Task 24: Add User Profile Federation Endpoint

**Files:**
- Add handler: `internal/federation/users.go` (or modify existing)
- Modify: frontend user profile component

**Step 1: Write profile endpoint (remote-facing)**

```go
// HandleUserProfile returns a full user profile.
// GET /federation/v1/users/{userID}/profile
func (ss *SyncService) HandleUserProfile(w http.ResponseWriter, r *http.Request) {
    // 1. Verify signed request
    // 2. Query user from users table
    // 3. Return full profile: display_name, avatar_id, about_me,
    //    custom_status, status_emoji, banner_id, accent_color, pronouns, badges
}
```

**Step 2: Write profile fetch with caching**

```go
// FetchRemoteProfile fetches and caches a remote user's profile.
func (s *Service) FetchRemoteProfile(ctx context.Context, userID string, instanceID string) (*models.User, error) {
    // 1. Check if local user stub is fresh (updated_at < 5 min ago)
    // 2. If stale, fetch from remote: GET /federation/v1/users/{userID}/profile
    // 3. Update local user stub with new data
    // 4. Return user
}
```

**Step 3: Update frontend profile component**

When viewing a federated user's profile, trigger a profile refresh if the data is stale. Show all profile fields identically to local users.

**Step 4: Wire routes, run tests, commit**

```
feat: on-demand user profile sync for federated users
```

---

## Phase 13: Integration Testing & Final Cleanup

### Task 25: Full Integration Test

**Step 1: Run complete Go test suite**

```bash
docker run --rm -v $(pwd):/build -w /build -e GOTOOLCHAIN=auto golang:1.24-alpine go test ./internal/... -count=1 -v
```

Fix any failures.

**Step 2: Run complete frontend test suite**

```bash
docker run --rm -v $(pwd)/web:/web -w /web node:24-alpine npx vitest run
```

Fix any failures.

**Step 3: Docker build verification**

```bash
docker compose -f deploy/docker/docker-compose.yml build --no-cache amityvox web-init
```

Must complete without errors.

**Step 4: Search for remaining dead code**

```
grep -r "federation_guild_cache\|federation_channel_mirrors\|FederatedGuild\|isFederatedGuild\|federatedGuilds\|federatedGuildIds" --include="*.go" --include="*.ts" --include="*.svelte"
```

All results should be zero. If any remain, remove them.

**Step 5: Search for remaining TODO/FIXME in federation code**

```
grep -r "TODO\|FIXME\|HACK\|XXX" internal/federation/ web/src/lib/
```

Address any that relate to federation.

**Step 6: Commit final cleanup**

```
chore: final cleanup — remove all dead federation code
```

---

## Phase 14: Deploy & Verify

### Task 26: Deploy and Live Test

**Step 1: Build and deploy**

```bash
docker compose -f deploy/docker/docker-compose.yml build --no-cache amityvox web-init
docker compose -f deploy/docker/docker-compose.yml up -d amityvox web-init
docker compose -f deploy/docker/docker-compose.yml restart caddy
```

**Step 2: Run migration**

The migration runs automatically on startup. Verify in logs:
```bash
docker compose -f deploy/docker/docker-compose.yml logs amityvox | grep -i migrat
```

**Step 3: Live verification checklist**

- [ ] Join a guild on app.amityvox.chat from dev.amityvox.chat — guild appears in sidebar with badge
- [ ] Channel categories, threads, encryption indicators render correctly
- [ ] Send and receive messages in real-time in a federated guild
- [ ] Avatars and attachments load via media proxy
- [ ] Presence updates for federated users in shared guilds
- [ ] Voice/video works in federated guild channel
- [ ] DM a user on app.amityvox.chat from dev.amityvox.chat
- [ ] Voice/video in federated DM
- [ ] Remote guild management (edit channel name on remote guild)
- [ ] Guild discovery shows results from all peers
- [ ] Cross-instance invite works

---

## Summary: Task Dependency Graph

```
Phase 1 (Foundation):
  Task 1 (Migration) → Task 2 (Models) → Task 3 (Shorthand/Config)

Phase 2 (Backend Sync):
  Task 4 (Guild Join) → Task 5 (Event Handlers) → Task 6 (Presence)
  Task 7 (Media Proxy) — independent
  Task 8 (Backfill Sync) — depends on Task 5

Phase 3 (Management):
  Task 9 (Management Proxy) — depends on Tasks 2, 5

Phase 4 (Frontend):
  Task 10 (Types) → Task 11 (Stores) → Task 12 (API Client) → Task 13 (Components)
  Task 14 (Badges) — depends on Task 10
  Task 15 (Media URLs) — depends on Task 7

Phase 5 (Write Routing):
  Task 16 (Helper) → Task 22 (All Handlers) — depends on Task 9

Phase 6 (MLS):
  Task 17 (MLS Endpoints) — depends on Tasks 2, 5

Phase 7 (Voice):
  Task 18 (Relay Mode) — depends on Task 3

Phase 8 (Discovery):
  Task 19 (Aggregated Discovery) — depends on Task 3
  Task 20 (Invites) — depends on Task 4

Phase 9 (Cleanup):
  Task 21 (Drop Tables) — depends on ALL previous tasks

Phase 10-12 (Polish):
  Tasks 22-24 — depends on Phase 9

Phase 13-14 (Verify & Deploy):
  Tasks 25-26 — final
```

Total: **26 tasks across 14 phases**
