# AmityVox Performance Audit Report

**Date:** 2026-02-18
**Phase:** 5 -- Pre-Release Performance Audit
**Auditor:** Claude Opus 4.6

---

## Critical Performance Issues

### CRIT-01: Gateway `shouldDispatchTo` makes DB queries per-event per-client (O(events x clients) query amplification)

**File:** `/docker/AmityVox/internal/gateway/gateway.go`, lines 995-1063

**Description:** The `shouldDispatchTo` method is called inside a loop over all connected clients (line 912) while holding the `clientsMu` read lock (line 904). For certain event types -- call ring events (line 995-1004), generic channel events (lines 1020-1037), and typing events (lines 1040-1063) -- it executes one or more DB queries per client:

```go
// Line 904-923: dispatch loop iterates ALL clients under read lock
s.clientsMu.RLock()
defer s.clientsMu.RUnlock()
for client := range s.clients {
    if s.shouldDispatchTo(client, subject, event) {  // <-- DB queries here
        s.sendMessage(client, msg)
    }
}
```

Within `shouldDispatchTo`:
- **Call ring events** (line 1001): `SELECT EXISTS(SELECT 1 FROM channel_recipients ...)` -- 1 query per client
- **Channel events** (lines 1023-1036): `SELECT guild_id FROM channels ...` + potentially `SELECT EXISTS(SELECT 1 FROM channel_recipients ...)` -- up to 2 queries per client
- **Typing events** (lines 1048-1061): `SELECT guild_id FROM channels ...` + potentially `SELECT EXISTS(...)` -- up to 2 queries per client

Additionally, all these queries use `context.Background()` instead of the event context, meaning they cannot be cancelled if the event dispatch is abandoned.

**Impact:** With 100 connected clients and a typing event in a DM channel, this generates 200 DB queries. With 1,000 clients, 2,000 queries. On a Raspberry Pi 5 with a 25-connection pool, this saturates the pool and blocks all other API requests. Response time degrades from milliseconds to seconds.

**Severity:** CRITICAL

**Recommended Fix:**
1. Cache channel-to-guild mappings in memory (LRU cache, TTL ~60s). Channel guild_id rarely changes.
2. Cache DM channel recipient lists in the Client struct (loaded at identify time, updated on CHANNEL_UPDATE events).
3. For call ring events, query the recipient list once before the dispatch loop, then check the set for each client.
4. Use a proper context with timeout instead of `context.Background()`.

---

### CRIT-02: `hasGuildPermission` / `hasChannelPermission` execute 3-4 sequential DB queries per call

**Files:**
- `/docker/AmityVox/internal/api/guilds/guilds.go`, lines 2330-2376
- `/docker/AmityVox/internal/api/channels/channels.go`, lines 2665-2709 and 2714-2770

**Description:** Every permission check executes 3-4 sequential queries:
1. `SELECT owner_id FROM guilds WHERE id = $1` (owner check)
2. `SELECT flags FROM users WHERE id = $1` (admin flag check)
3. `SELECT default_permissions FROM guilds WHERE id = $1` (default perms)
4. `SELECT r.permissions_allow, r.permissions_deny FROM roles r JOIN member_roles mr ...` (role perms)

For `hasChannelPermission`, an additional query is prepended: `SELECT guild_id, channel_type FROM channels WHERE id = $1`.

Many handlers call permission checks multiple times. For example, `HandleCreateMessage` in `channels.go` calls `hasChannelPermission` up to 3 times (lines 508, 568-569) plus `hasGuildPermission` indirectly, generating 12-15 DB round-trips before the message is even created.

**Impact:** Each permission check adds 3-4 sequential DB round-trips (~1-4ms each on local PG, much more under load). A single `HandleCreateMessage` request executes ~15-20 individual queries across permission checks, channel state reads, slowmode checks, guild lookups, encryption checks, DM spam checks, and the actual insert. On a loaded system, connection pool exhaustion is likely.

**Severity:** CRITICAL

**Recommended Fix:**
1. Consolidate permission computation into a single query using CTEs or a stored function that returns the computed permission bitfield.
2. Cache permission results per (user_id, guild_id) with short TTL (5-10s) in DragonflyDB.
3. At minimum, merge the owner_id + default_permissions queries into a single `SELECT owner_id, default_permissions FROM guilds WHERE id = $1`.

---

### CRIT-03: `HandleCreateMessage` executes up to 12 sequential queries before the INSERT

**File:** `/docker/AmityVox/internal/api/channels/channels.go`, lines 470-620

**Description:** The create message path executes these queries sequentially:
1. `hasChannelPermission` call (line 367): 3-5 queries
2. `SELECT locked, archived, read_only, read_only_role_ids FROM channels` (line 475)
3. `SELECT guild_id FROM channels` (line 491) -- redundant, already fetched in step 2
4. `SELECT owner_id FROM guilds` (line 494) -- already fetched in hasChannelPermission
5. `SELECT flags FROM users` (line 501) -- already fetched in hasChannelPermission
6. `hasChannelPermission` again for Administrator (line 508): 3-5 more queries
7. `SELECT COUNT(*) FROM member_roles` (line 515)
8. `SELECT encrypted FROM channels` (line 551) -- already fetched in step 2
9. `SELECT COALESCE(slowmode_seconds, 0) FROM channels` (line 567) -- already fetched in step 2
10. `hasChannelPermission` x2 for ManageMessages and ManageChannels (lines 568-569): 6-10 more queries
11. `SELECT MAX(created_at) FROM messages` (line 572)
12. `SELECT guild_id FROM channels` (line 587) -- third time fetching same data
13. `SELECT timeout_until FROM guild_members` (line 590)
14. `SELECT user_id FROM channel_recipients` (line 606)

Total: approximately 20-30 DB round-trips per message send.

**Impact:** On a Pi 5, each round-trip to PG is ~0.5-2ms, so message send latency is 10-60ms just in DB queries. Under load with the 25-connection pool, this blocks other requests. This is the hottest path in the entire application.

**Severity:** CRITICAL

**Recommended Fix:**
1. Fetch all needed channel state in a single query at the top: `SELECT guild_id, locked, archived, read_only, read_only_role_ids, encrypted, slowmode_seconds, owner_id FROM channels WHERE id = $1`.
2. Compute permissions once and reuse the result.
3. Combine the guild owner + user flags + default_permissions + role permissions into a single CTE query.
4. This should reduce the path from ~25 queries to ~4-5.

---

## High-Impact Issues

### HIGH-01: N+1 query in `HandleUpdateGuildMember` role assignment

**File:** `/docker/AmityVox/internal/api/guilds/guilds.go`, lines 784-789

**Description:** Role assignment in member update deletes all existing roles then re-inserts each one individually inside a loop:

```go
h.Pool.Exec(r.Context(), `DELETE FROM member_roles WHERE guild_id = $1 AND user_id = $2`, guildID, memberID)
for _, roleID := range req.Roles {
    h.Pool.Exec(r.Context(),
        `INSERT INTO member_roles (guild_id, user_id, role_id) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`,
        guildID, memberID, roleID)
}
```

**Impact:** If a member has 10 roles, this is 11 queries (1 DELETE + 10 INSERTs). Not wrapped in a transaction, so partial failure leaves inconsistent state.

**Severity:** HIGH

**Recommended Fix:** Use a single query with `unnest($3::text[])`:
```sql
DELETE FROM member_roles WHERE guild_id = $1 AND user_id = $2;
INSERT INTO member_roles (guild_id, user_id, role_id)
SELECT $1, $2, unnest($3::text[])
ON CONFLICT DO NOTHING;
```
And wrap both in a transaction.

---

### HIGH-02: N+1 query in crosspost fanout

**File:** `/docker/AmityVox/internal/api/channels/channels.go`, lines 2356-2386

**Description:** When cross-posting an announcement to followers, each follower channel gets a separate INSERT + UPDATE + PublishJSON:

```go
for _, fi := range followers {
    _, err := h.Pool.Exec(r.Context(), `INSERT INTO messages ...`)
    h.Pool.Exec(r.Context(), `UPDATE channels SET last_message_id = $1 WHERE id = $2`, ...)
    h.EventBus.PublishJSON(...)
}
```

**Impact:** With 50 follower channels, this is 100 DB queries + 50 NATS publishes, all sequential. Announcement channels with many followers will experience multi-second response times.

**Severity:** HIGH

**Recommended Fix:** Batch the INSERTs using a single `INSERT INTO messages ... SELECT ... FROM unnest(...)` and batch the channel updates similarly. Publish a single bulk event instead of one per follower.

---

### HIGH-03: N+1 query in message bulk decrypt

**File:** `/docker/AmityVox/internal/api/channels/channels.go`, lines 2825-2836

**Description:** Bulk message decryption runs a separate UPDATE query per message:

```go
for _, msg := range req.Messages {
    tag, err := h.Pool.Exec(ctx,
        `UPDATE messages SET content = $1, encrypted = false, encryption_session_id = NULL
         WHERE id = $2 AND channel_id = $3 AND encrypted = true`,
        msg.Content, msg.ID, channelID,
    )
}
```

**Impact:** With 100 messages (the maximum), this is 100 sequential UPDATE queries.

**Severity:** HIGH

**Recommended Fix:** Use a single batch update with `unnest` arrays or a temporary table join. Alternatively, use `pgx.Batch` to pipeline all updates in a single network round-trip.

---

### HIGH-04: Unbounded SELECT queries on guild list endpoints

**Files:**
- `/docker/AmityVox/internal/api/guilds/guilds.go`, line 860: `SELECT ... FROM guild_bans WHERE guild_id = $1` (no LIMIT)
- `/docker/AmityVox/internal/api/guilds/guilds.go`, line 1342: `SELECT ... FROM invites WHERE guild_id = $1` (no LIMIT)
- `/docker/AmityVox/internal/api/guilds/guilds.go`, line 1498: `SELECT ... FROM custom_emoji WHERE guild_id = $1` (no LIMIT)
- `/docker/AmityVox/internal/api/guilds/guilds.go`, line 1652: `SELECT ... FROM guild_categories WHERE guild_id = $1` (no LIMIT)
- `/docker/AmityVox/internal/api/guilds/guilds.go`, line 1804: `SELECT ... FROM webhooks WHERE guild_id = $1` (no LIMIT)

**Description:** These list endpoints return all rows for a guild without pagination or LIMIT. While guild-scoped data is typically bounded by practical limits (guilds rarely have thousands of bans or emoji), a malicious or extreme-use guild could accumulate unbounded rows.

**Impact:** A guild with 10,000 bans returns all 10,000 rows in a single JSON response. This consumes memory proportional to row count and can cause large response payloads (memory + bandwidth).

**Severity:** HIGH

**Recommended Fix:** Add cursor-based pagination (using `before` parameter and LIMIT) to all list endpoints. Default LIMIT 100, max 1000.

---

### HIGH-05: `shouldDispatchTo` for PRESENCE_UPDATE iterates all clients to find guild IDs

**File:** `/docker/AmityVox/internal/gateway/gateway.go`, lines 938-973

**Description:** For PRESENCE_UPDATE events, the method iterates all connected clients to find the event user's guild memberships:

```go
s.clientsMu.RLock()
for otherClient := range s.clients {
    if otherClient.userID == event.UserID {
        otherClient.mu.Lock()
        for gid := range otherClient.guildIDs {
            eventUserGuildIDs = append(eventUserGuildIDs, gid)
        }
        otherClient.mu.Unlock()
        break
    }
}
s.clientsMu.RUnlock()
```

This is O(N) per presence event per recipient client. Since this itself is called from the O(clients) dispatch loop, presence updates are O(clients^2).

Additionally, this re-acquires `clientsMu.RLock()` inside a call that is already holding `clientsMu.RLock()` from the dispatch loop (line 904). While re-entrant RLock on `sync.RWMutex` is allowed, this is a latent correctness concern if the lock is ever upgraded.

**Impact:** With 500 clients, a single presence update causes 500 iterations x up to 500 inner iterations = 250,000 operations. On a Pi 5, this creates noticeable latency spikes.

**Severity:** HIGH

**Recommended Fix:** Use the `userClients` map (which already exists, line 113) to look up the event user's clients in O(1) instead of iterating all clients:
```go
s.userClientsMu.RLock()
if clientSet, ok := s.userClients[event.UserID]; ok {
    for c := range clientSet {
        c.mu.Lock()
        for gid := range c.guildIDs { ... }
        c.mu.Unlock()
        break
    }
}
s.userClientsMu.RUnlock()
```

---

## Medium-Impact Issues

### MED-01: Frontend message store grows unbounded across channel switches

**File:** `/docker/AmityVox/web/src/lib/stores/messages.ts`

**Description:** The `messagesByChannel` store is a `Map<string, Message[]>` that accumulates messages for every channel the user visits. Messages are never evicted when switching channels. Over a long session visiting many channels, memory grows without bound.

**Impact:** A user visiting 50 channels with 50 messages each stores 2,500 Message objects in memory. Each message includes content, author, attachments, and embeds. Over an extended session (hours), this can consume tens of MB of browser memory.

**Severity:** MEDIUM

**Recommended Fix:** Implement an LRU eviction policy: when the map exceeds a threshold (e.g., 20 channels), evict the least-recently-accessed channel's messages. The `clearChannelMessages` function exists but is never called automatically.

---

### MED-02: MessageList and MemberList do not use virtual scrolling

**Files:**
- `/docker/AmityVox/web/src/lib/components/chat/MessageList.svelte`
- `/docker/AmityVox/web/src/lib/components/layout/MemberList.svelte`

**Description:** Both components render all items directly in the DOM:

`MessageList.svelte` (line 375):
```svelte
{#each messages as msg, i (msg.id)}
    <MessageItem ... />
{/each}
```

`MemberList.svelte` renders all members in groups without virtualization. No windowing library (e.g., svelte-virtual-list) is used.

**Impact:** A channel with 500 loaded messages renders 500 MessageItem components in the DOM simultaneously. Each MessageItem includes avatar, username, content with markdown rendering, reactions, and attachments. On low-end devices, this causes jank during scrolling and high memory usage. MemberList with 1,000 members (the LIMIT from the API) renders all 1,000 member entries.

**Severity:** MEDIUM

**Recommended Fix:** Implement virtual scrolling for both lists. Only render the visible items plus a small buffer. This is standard practice for chat applications. Consider using a library like `svelte-virtual-list` or a custom IntersectionObserver-based approach.

---

### MED-03: `dmSpamTracker` goroutine in `init()` has no shutdown mechanism

**File:** `/docker/AmityVox/internal/api/channels/channels.go`, lines 119-127

**Description:** The DM spam tracker cleanup runs in a goroutine started from `init()`:

```go
func init() {
    go func() {
        ticker := time.NewTicker(5 * time.Minute)
        for range ticker.C {
            dmSpamTracker.cleanup()
        }
    }()
}
```

This goroutine:
1. Has no context cancellation -- it runs forever, even during graceful shutdown.
2. Is started at package import time via `init()`, not tied to the server lifecycle.
3. The ticker is never stopped.

**Impact:** During graceful shutdown, this goroutine leaks. In tests, it also leaks since there is no way to stop it. While the impact is minor (single goroutine, no DB connections), it violates the "stateless core" design principle and makes testing harder.

**Severity:** MEDIUM

**Recommended Fix:** Move the cleanup goroutine to the Handler's lifecycle. Start it in a `Start(ctx)` method and cancel via context. Or integrate it as a periodic worker in the workers.Manager.

---

### MED-04: Redundant queries for the same channel/guild data within a single request

**File:** `/docker/AmityVox/internal/api/channels/channels.go`, lines 470-600

**Description:** Within `HandleCreateMessage`, the same data is fetched multiple times:
- `channels.guild_id` is queried at lines 491, 587 (and also inside `hasChannelPermission`)
- `channels.encrypted` is queried at line 551 after `locked, archived, read_only, read_only_role_ids` was already fetched at line 475
- `channels.slowmode_seconds` is queried at line 567
- `guilds.owner_id` is queried at line 494 (and also inside `hasGuildPermission`)
- `users.flags` is queried at line 501 (and also inside `hasGuildPermission`)

Each of these is a separate network round-trip to PostgreSQL.

**Impact:** 5-8 redundant queries per message send. Adds ~5-16ms of unnecessary latency.

**Severity:** MEDIUM

**Recommended Fix:** Fetch all channel fields in a single query at the top of the handler. Pass the channel struct to permission checks instead of re-querying.

---

### MED-05: Workers do not implement backoff on errors

**File:** `/docker/AmityVox/internal/workers/workers.go`, lines 112-141

**Description:** The `startPeriodic` function runs a worker function at a fixed interval regardless of whether it succeeds or fails:

```go
for {
    select {
    case <-ctx.Done():
        return
    case <-ticker.C:
        if err := fn(ctx); err != nil {
            m.logger.Error("worker error", ...)
        }
    }
}
```

If a worker consistently fails (e.g., DB is temporarily unavailable), it retries at the same interval with no backoff. This generates sustained error load and log spam.

**Impact:** During a PostgreSQL failover or maintenance window, all 8+ periodic workers hammer the DB every tick interval, generating error logs and wasted connections.

**Severity:** MEDIUM

**Recommended Fix:** Implement exponential backoff after consecutive failures. Reset the backoff after a successful run. Example: double the interval after each failure, cap at 5 minutes, reset to normal after success.

---

### MED-06: Gateway `context.Background()` queries bypass request cancellation

**File:** `/docker/AmityVox/internal/gateway/gateway.go`, lines 1001, 1023, 1033, 1048, 1058

**Description:** All database queries in `shouldDispatchTo` use `context.Background()`:

```go
s.pool.QueryRow(context.Background(),
    `SELECT guild_id FROM channels WHERE id = $1`, event.ChannelID).Scan(&guildID)
```

**Impact:** If the WebSocket connection is closed or the server is shutting down, these queries continue to execute and consume pool connections. During shutdown, pending queries block graceful termination.

**Severity:** MEDIUM

**Recommended Fix:** Thread a proper context through the dispatch path. The NATS subscription callback should receive a context that is cancelled on server shutdown.

---

### MED-07: Retention worker deletes from search index one-by-one

**File:** `/docker/AmityVox/internal/workers/retention_worker.go`, lines 189-193

**Description:**
```go
if m.search != nil {
    for _, id := range messageIDs {
        m.search.DeleteMessage(ctx, id)
    }
}
```

Each batch of 1,000 messages deletes from Meilisearch one at a time.

**Impact:** 1,000 individual HTTP requests to Meilisearch per batch. With retention policies on large channels, this creates sustained load on the search service.

**Severity:** MEDIUM

**Recommended Fix:** Use Meilisearch's batch delete API to delete all IDs in a single request.

---

## Low-Impact Issues

### LOW-01: Reorder handlers (roles/channels) execute per-item UPDATE queries in a transaction

**Files:**
- `/docker/AmityVox/internal/api/guilds/guilds.go`, lines 1258-1266 (role reorder)
- `/docker/AmityVox/internal/api/guilds/guilds.go`, lines 1309-1317 (channel reorder)

**Description:**
```go
for _, item := range req {
    _, err := tx.Exec(r.Context(),
        `UPDATE roles SET position = $3 WHERE id = $1 AND guild_id = $2`,
        item.ID, guildID, item.Position)
}
```

**Impact:** With 20 roles, this is 20 UPDATE queries within a single transaction. The transaction holds a DB connection for the duration. Low frequency operation so impact is minimal.

**Severity:** LOW

**Recommended Fix:** Use a single `UPDATE ... FROM (VALUES ...) AS v(id, pos) WHERE roles.id = v.id` for batch position updates.

---

### LOW-02: Guide step insert in a loop

**File:** `/docker/AmityVox/internal/api/guilds/guilds.go`, lines 2887-2902

**Description:** Server guide steps are inserted one at a time in a loop (max 20 steps). Each step is a separate INSERT + RETURNING.

**Impact:** Max 20 queries. Low frequency admin operation. Minimal impact.

**Severity:** LOW

**Recommended Fix:** Use a batch insert with `unnest` arrays or `pgx.Batch`.

---

### LOW-03: `handleBulkPin` in MessageList.svelte makes sequential API calls

**File:** `/docker/AmityVox/web/src/lib/components/chat/MessageList.svelte`, lines 84-96

**Description:**
```typescript
for (const msgId of selectedMessages) {
    try {
        await api.pinMessage(channelId, msgId);
        pinned++;
    } catch { }
}
```

Each pin is a sequential API call. No parallelism, no bulk endpoint used.

**Impact:** Pinning 10 messages makes 10 sequential HTTP requests. User waits for all to complete.

**Severity:** LOW

**Recommended Fix:** Use `Promise.all` or `Promise.allSettled` to parallelize the pin requests. Better yet, add a bulk pin API endpoint.

---

### LOW-04: Database connection pool MinConns is 2 (potentially too low for Pi 5)

**File:** `/docker/AmityVox/internal/database/database.go`, line 38

**Description:** `config.MinConns = 2` means the pool idles with only 2 connections. On request spikes, new connections must be established (cold start penalty of ~5-20ms per connection to local PG).

**Impact:** First burst of requests after idle period experiences connection establishment overhead.

**Severity:** LOW

**Recommended Fix:** Set `MinConns` to `max(5, MaxConns/4)` to maintain a warm pool. For a 25-connection pool, `MinConns = 6` is reasonable.

---

### LOW-05: `new Map(map)` reactivity trigger in Svelte stores

**Files:** Multiple files in `/docker/AmityVox/web/src/lib/stores/`

**Description:** Store updates use `return new Map(map)` to trigger Svelte reactivity. This creates a full shallow copy of the Map on every mutation. For large maps (e.g., `messagesByChannel` with many channels), this creates GC pressure.

**Impact:** Each store update allocates a new Map object. With frequent WebSocket events (typing, presence), this creates many short-lived objects. Impact is minor on modern browsers with efficient GC.

**Severity:** LOW

**Recommended Fix:** This is actually the correct Svelte pattern for triggering reactivity with Maps. No change needed -- the current implementation is idiomatic. If performance becomes measurable, consider using $state runes (Svelte 5) with Map.set() which is natively reactive.

---

## Performance Best Practices Assessment

### What is done well

1. **Batch loading of related entities.** The `enrichMessagesWithAuthors`, `enrichMessagesWithAttachments`, and `enrichMessagesWithEmbeds` functions in `channels.go` (lines 1903-2040) correctly use `WHERE id = ANY($1)` to batch-load in single queries. This avoids the classic N+1 pattern for message fetches.

2. **Member role batch loading.** `HandleGetGuildMembers` (guilds.go:629-646) loads all member roles in a single query with `WHERE guild_id = $1`, then joins in memory. Correct approach.

3. **Connection pool configuration.** The database pool is properly configured with `MaxConns`, `MinConns`, `MaxConnLifetime` (30 min), `MaxConnIdleTime` (5 min), and `HealthCheckPeriod` (30s). These are reasonable defaults for the Pi 5 target.

4. **Federation retry with exponential backoff.** The federation sync service (`sync.go`, lines 650-664) implements proper exponential backoff: 5s, 30s, 2m, 10m, 1h capped. Dead letter queue after 10 attempts. JetStream durable consumer with `NakWithDelay`. This is well-engineered.

5. **WebSocket reconnection with exponential backoff and jitter.** The frontend `gateway.reconnect.ts` implements proper exponential backoff with decorrelated jitter, max 50 attempts, online/offline detection, and connection quality monitoring. This is production-grade.

6. **LazyImage with IntersectionObserver.** The `LazyImage.svelte` component correctly implements lazy loading with IntersectionObserver, configurable rootMargin (200px preload), threshold, and proper cleanup on unmount.

7. **Retention worker uses batched deletes.** The retention worker processes messages in batches of 1,000 with proper context cancellation checks between batches.

8. **Messages have LIMIT clauses.** `HandleGetMessages` (channels.go:360-427) properly enforces LIMIT with a default of 50 and max of 100.

9. **Members have LIMIT clause.** `HandleGetGuildMembers` uses `LIMIT 1000`.

10. **Audit log has LIMIT.** `HandleGetAuditLog` uses `LIMIT 100`.

11. **Goroutine lifecycle management.** Workers use `context.WithCancel` and `sync.WaitGroup` for clean shutdown. The gateway heartbeat monitor exits cleanly when the context is cancelled. The `readLoop` call is blocking with cleanup after return.

12. **Replay buffer is bounded.** Gateway client replay buffer is capped at 100 events (line 918-919).

13. **Database indexes cover primary access patterns.** The initial migration includes indexes on `messages(channel_id, id DESC)`, `guild_members(user_id)`, `attachments(message_id)`, `embeds(message_id)`, `channels(guild_id, position)`, and all foreign key lookup columns. These cover the main query patterns.

### Summary of findings

| Severity | Count | Key Theme |
|----------|-------|-----------|
| Critical | 3     | Gateway DB queries in dispatch loop; permission check query cascade; message send query cascade |
| High     | 5     | N+1 in role assignment and crossposts; unbounded SELECT lists; O(N^2) presence dispatch |
| Medium   | 7     | Frontend memory growth; no virtual scrolling; goroutine leak; redundant queries; no worker backoff |
| Low      | 5     | Loop updates; pool warm-up; frontend sequential calls; Svelte reactivity pattern |

### Top 3 highest-impact fixes (effort vs. payoff)

1. **Consolidate `HandleCreateMessage` queries** (CRIT-03 + MED-04): Fetch all channel state in 1 query instead of 8+ separate ones. Estimated: ~2 hours. Payoff: 60-70% reduction in message send latency.

2. **Cache channel-to-guild mapping in gateway** (CRIT-01): Add an LRU cache for channel guild_id lookups. Estimated: ~1 hour. Payoff: Eliminates most DB queries from the hot dispatch path.

3. **Consolidate permission checks** (CRIT-02): Merge the 3-4 queries per permission check into 1. Estimated: ~3 hours. Payoff: Reduces DB load by 50%+ across all permission-gated endpoints.
