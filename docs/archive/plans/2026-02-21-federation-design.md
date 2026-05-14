# Federation Design: Full Parity

**Date:** 2026-02-21
**Status:** Approved
**Goal:** A federated guild, channel, user, or DM is indistinguishable from a local one.

---

## 1. Core Principle

Federation is a **property of data**, not a separate system. Every major table gets an `instance_id` column (nullable FK to `instances.id`). NULL = local, populated = federated. One code path for everything.

### Write Routing

Any mutation on a row where `instance_id IS NOT NULL`:

1. Check permissions locally (same code as local)
2. Sign the request with the local instance's Ed25519 key
3. Forward to the home instance via federation protocol
4. Home instance validates, applies, broadcasts update event
5. Local copy updated when the federation event arrives back

### Config Hierarchy

Admin panel DB settings override `amityvox.toml` defaults. Applies to ALL federation config. CLI flags can set initial values, but once set in the admin panel, the DB value wins.

### Cleanup Rule

When a new implementation replaces an old one, the old code is deleted entirely. No `// deprecated`, no `// TODO: remove`, no dead functions, no orphaned tables. Clean as you go.

---

## 2. Instance Identity & Discovery

### Shorthand

Each instance has a `shorthand` (max 5 characters) set in config or admin panel. Included in `.well-known/amityvox` response. Used as a visual badge on federated guilds and users.

**Collision resolution:** If two peers share the same shorthand, the newer peer (by `established_at`) gets a numeric suffix: `AV` -> `AV1`.

### Discovery Response Additions

```json
{
  "shorthand": "AV",
  "voice_mode": "direct"
}
```

### Federation Config

```toml
[federation]
mode = "open"           # open | allowlist | closed
shorthand = "AV"        # max 5 chars, shown on federated peers
voice_mode = "direct"   # direct | relay
```

All three settings are overridable in the admin panel UI.

---

## 3. Data Model Migration

### Tables Gaining `instance_id`

Every table below gets `instance_id TEXT REFERENCES instances(id) ON DELETE CASCADE`:

| Table | Effect |
|---|---|
| `guilds` | Federated guilds stored as real rows |
| `channels` | Channels, categories, threads for federated guilds |
| `roles` | Role definitions with full permission bitfields |
| `guild_members` | Membership records for federated guild members |
| `messages` | Messages in federated channels stored locally |
| `attachments` | Attachment metadata (media proxied) |
| `embeds` | Embed metadata |
| `reactions` | Reaction records |
| `pins` | Pinned messages |
| `webhooks` | Webhook definitions (remote-managed) |

The `users` table already has `instance_id` for user stubs.

### Cascade Purge

`DELETE FROM guilds WHERE instance_id = $1` cascades through foreign keys and removes all data from that instance. Channel, message, member, role, reaction, attachment, embed, pin data — all removed in one operation.

### Tables to Remove / Rename

| Table | Action | Replaced By |
|---|---|---|
| `federation_guild_cache` | Dropped (migration 064) | Real `guilds` + `channels` + `roles` tables |
| `federation_channel_mirrors` | Renamed → `federation_dm_channel_map` (migration 065) | DM-only channel ID mapping; kept for federated DM channel lookups |

### Tables to Keep

| Table | Purpose |
|---|---|
| `instances` | Instance registry (public keys, domains, capabilities) |
| `federation_peers` | Peer relationships (active/pending/blocked) |
| `federation_peer_status` | Health metrics, negotiated version/capabilities |
| `federation_peer_controls` | Per-peer block/allow/mute overrides |
| `federation_channel_peers` | Which instances have members in which channels (for targeted delivery) |
| `federation_delivery_receipts` | Delivery status tracking (fix: composite unique key) |
| `federation_dead_letters` | Failed deliveries after max retries |
| `federation_key_audit` | Public key change history |

---

## 4. Guild Sync

### Initial Join

When a user joins a federated guild, the home instance returns the full guild structure. The local instance inserts:

- `guilds` row with `instance_id` set
- `channels` rows for every channel (text, voice, announcement, forum, stage, gallery)
- Category channels (type = 4) with correct `position` ordering
- Thread channels with `parent_id` pointing to their parent channel
- `roles` rows with full permission bitfields and color/icon/position
- `guild_members` row for the joining user
- Gallery channels with their media items

All inserted in a single transaction.

### Ongoing Sync via Events

All guild mutations on the home instance generate federation events delivered to peer instances:

| Event | Local Action |
|---|---|
| `GUILD_UPDATE` | Update `guilds` row |
| `CHANNEL_CREATE` | Insert `channels` row |
| `CHANNEL_UPDATE` | Update `channels` row |
| `CHANNEL_DELETE` | Delete `channels` row |
| `ROLE_CREATE` | Insert `roles` row |
| `ROLE_UPDATE` | Update `roles` row |
| `ROLE_DELETE` | Delete `roles` row |
| `GUILD_MEMBER_ADD` | Insert `guild_members` row |
| `GUILD_MEMBER_UPDATE` | Update `guild_members` row |
| `GUILD_MEMBER_REMOVE` | Delete `guild_members` row |
| `THREAD_CREATE` | Insert `channels` row (thread type) |
| `THREAD_UPDATE` | Update thread channel row |
| `THREAD_DELETE` | Delete thread channel row |
| `CHANNEL_PINS_UPDATE` | Update pins for channel |

Same handler as local events, guarded by `instance_id` check to prevent local events from overwriting remote data.

### Backfill on Reconnect

When a peer reconnects after downtime:

1. Local sends `POST /federation/v1/sync` with `last_seen_hlc` timestamp
2. Remote queries all events since that HLC for guilds the local instance has members in
3. Remote streams events back in causal order
4. Local processes each event, updates tables
5. **Window:** 7 days max. Beyond that, full re-sync (leave + re-join guilds)

### New Endpoint

```
POST /federation/v1/sync
Body: { "last_seen_hlc": "1708500000000-42", "guild_ids": ["..."] }
Response: { "events": [...], "truncated": false }
```

---

## 5. Messaging

Messages go in the real `messages` table. No separate federation message store.

### Outbound (local user posting in remote guild)

1. Frontend calls same message POST endpoint
2. Backend detects guild has `instance_id` set
3. Signs message payload, forwards to home instance
4. Home instance validates permissions, creates message, broadcasts event
5. Event arrives back via federation, stored locally

### Inbound (remote user posting in local guild)

1. Federation inbox receives signed `MESSAGE_CREATE`
2. Signature verified, payload validated
3. Message inserted into `messages` table (author is a user stub with `instance_id`)
4. Published to NATS, dispatched to local WebSocket clients

### Critical Requirement

Every event envelope MUST include `guild_id`. The fallback extraction logic that tries to parse `guild_id` from event data is removed. If `guild_id` is missing, the event is rejected with an error, not silently dropped.

---

## 6. Media Proxy

### Endpoint

```
GET /api/v1/federation/media/{instanceId}/{fileId}
```

- Fetches from remote instance's file endpoint (`/api/v1/files/{fileId}`)
- Files ≤ 1 MB: cached in DragonflyDB, key `fed:media:{instanceId}:{fileId}`, TTL 1 hour
- Files > 1 MB: cached to disk at `media_cache_dir` with LRU eviction (`media_cache_max_size_mb`, default 1024 MB)
- Streams response to client with correct Content-Type

### URL Rewriting

Frontend helpers `buildAvatarUrl()`, `buildMediaUrl()`, `buildAttachmentUrl()` detect when the entity has an `instance_id` and automatically route through the local proxy:

```
Local:     /api/v1/files/{fileId}
Federated: /api/v1/federation/media/{instanceId}/{fileId}
```

### Covers

- User avatars and banners
- Guild icons and banners
- Message attachments
- Embed thumbnails and images
- Gallery images
- Custom emoji (if synced)

---

## 7. Presence & User Profiles

### Presence

Real-time flow: user status change -> local NATS -> federation router -> peers -> remote NATS -> remote gateway -> remote WebSocket clients.

**PRESENCE_UPDATE events include `guild_ids` array.** The query that populates `guild_ids` MUST check for errors. If the query fails, the event is NOT sent (fail-closed, not fail-open with empty array).

Gateway dispatches presence to all local clients who share a guild with the remote user. Uses the same `guildIDs` intersection check as local presence.

### User Profiles

Stored in the real `users` table with `instance_id` set. Profile data synced on demand:

1. Client requests federated user profile
2. Backend checks `users` row — if `updated_at` is older than 5 minutes, fetches from remote
3. Fetches: `GET /federation/v1/users/{userId}/profile`
4. Updates `users` row: display_name, avatar_id, about_me, custom_status, status_emoji, banner_id, accent_color, pronouns, badges
5. Returns profile to client

### New Endpoint

```
GET /federation/v1/users/{userId}/profile
Response: { full user profile fields }
```

---

## 8. Encryption (MLS Across Federation)

Encrypted channels in federated guilds use the home instance's MLS group.

### Flow

1. Federated user's client requests MLS key package from home instance via federation
2. Home instance adds user to MLS group, returns welcome message + group key material
3. Messages encrypted client-side with MLS group key
4. Encrypted payloads sent via federation — home instance cannot read them
5. MLS tree (add/remove members, epoch advancement) managed by home instance

### New Federation Endpoints

```
POST /federation/v1/guilds/{guildID}/channels/{channelID}/mls/key-package
  -> Request to join MLS group, returns welcome + key material

POST /federation/v1/guilds/{guildID}/channels/{channelID}/mls/commit
  -> Send MLS commit (member add/remove/update)

POST /federation/v1/guilds/{guildID}/channels/{channelID}/mls/welcome
  -> Deliver MLS welcome message to new member
```

### Trust Model

Same as Matrix's Megolm via federation. The home instance manages the cryptographic group but cannot read message contents. Federated users' devices hold the actual decryption keys.

---

## 9. Voice/Video/Screenshare

### Instance-Level Configuration

```toml
[federation]
voice_mode = "direct"   # direct | relay
```

### Direct Mode

Client connects directly to the remote instance's LiveKit server:

1. Local instance requests LiveKit token from remote: `POST /federation/v1/voice/token`
2. Remote validates permissions, generates token with LiveKit URL
3. Client connects to remote LiveKit directly
4. Voice/video/screenshare streams flow through remote LiveKit

**Pros:** Lower latency.
**Cons:** Remote instance sees individual user IPs.

### Relay Mode

Local LiveKit acts as a proxy:

1. Same token exchange as direct mode
2. Local LiveKit establishes a bridge connection to remote LiveKit
3. Client connects to LOCAL LiveKit only
4. Local LiveKit relays streams to/from remote LiveKit

**Pros:** Privacy — remote instance only sees local instance's IP.
**Cons:** Slightly higher latency (extra hop).

### Voice State

Voice state events (join, leave, mute, deafen, video on/off, screenshare on/off) synced via federation events in real-time. Same `VOICE_STATE_UPDATE` event type as local.

### Covers

- Guild voice channels
- DM voice/video calls
- Group DM voice/video calls
- Screenshare in all contexts

---

## 10. Remote Guild Management

### The Differentiator

If a federated user has the permission on a remote guild, they can perform the action. No other federation protocol offers this.

### Supported Operations

| Permission | Remote Actions |
|---|---|
| `MANAGE_CHANNELS` | Create, edit, delete channels and categories |
| `MANAGE_ROLES` | Create, edit, delete roles; assign/remove roles from members |
| `KICK_MEMBERS` | Kick any member (local or remote) |
| `BAN_MEMBERS` | Ban/unban members |
| `MANAGE_GUILD` | Edit guild name, icon, description, settings |
| `MANAGE_WEBHOOKS` | Create, edit, delete webhooks |
| `MANAGE_THREADS` | Create, archive, delete threads |
| `MANAGE_MESSAGES` | Delete/pin messages from others |
| `MODERATE_MEMBERS` | Timeout, warn members |
| `ADMIN` | All of the above (except owner-only: delete guild, transfer ownership) |

### Flow

1. Frontend calls the same endpoint as local (e.g., `PATCH /api/v1/guilds/{id}/channels/{id}`)
2. Backend detects guild has `instance_id` set
3. Checks permissions locally against the synced roles/members
4. Signs the management request
5. Forwards to home instance: `POST /federation/v1/guilds/{guildID}/manage`
6. Home instance validates permissions (authoritative check), applies change
7. Change broadcasts to all peers via federation events
8. Local copy updated when event arrives

### New Federation Endpoints

```
POST /federation/v1/guilds/{guildID}/manage
Body: {
  "action": "channel_create" | "channel_update" | "channel_delete" |
            "role_create" | "role_update" | "role_delete" |
            "member_kick" | "member_ban" | "member_unban" |
            "guild_update" | "webhook_create" | "webhook_update" | "webhook_delete" |
            "thread_create" | "thread_archive" | "thread_delete" |
            "message_delete" | "message_pin" | "member_timeout",
  "data": { ... action-specific payload ... }
}
```

Single management endpoint with action routing. Reduces endpoint sprawl.

---

## 11. Guild Discovery & Cross-Instance Invites

### Aggregated Discovery

When a user searches for guilds:

1. Local instance fans out search query to all active peers in parallel
2. Each peer returns matching discoverable guilds (existing `POST /federation/v1/guilds/discover`)
3. Results merged, deduplicated by `(instance_id, guild_id)`
4. Displayed in unified discovery page with instance shorthand badges
5. Timeout per peer: 5 seconds. Slow peers excluded from results (not blocking).

### Cross-Instance Invites

**Creating an invite on a remote guild (requires MANAGE_GUILD or CREATE_INVITE):**

1. Signed request to home instance via management endpoint
2. Home instance creates invite, returns invite code
3. Invite link format: `https://{home-domain}/invite/{code}`

**Accepting a cross-instance invite:**

1. User on any federated instance clicks invite link
2. Their instance resolves the invite: `GET /federation/v1/invites/{code}`
3. Returns guild preview (name, icon, member count, description)
4. User confirms join
5. Signed join-via-invite request sent to home instance
6. Home instance validates invite, adds member, returns full guild structure
7. Local instance stores guild data in real tables

### New Federation Endpoints

```
GET  /federation/v1/invites/{code}        -> Resolve invite, return guild preview
POST /federation/v1/invites/{code}/accept  -> Accept invite, join guild
```

---

## 12. Frontend — No Special Cases

### Same Components for Everything

Since all data lives in the same tables, every frontend component works identically for local and federated:

| Component | Federation Handling |
|---|---|
| Guild sidebar | Same. Badge overlay with instance shorthand. |
| Channel list | Same. Categories, threads, encryption indicators all from real rows. |
| Message list | Same. Author avatars via media proxy. |
| Member list | Same. Federated members show instance shorthand. |
| Voice/video UI | Same. Token acquisition path differs internally, UI identical. |
| Guild settings | Same components. Mutations proxied to home instance. |
| Thread panel | Same. Threads are real channel rows. |
| Gallery view | Same. Media via proxy. |
| User profiles | Same. Profile data fetched on demand, cached. |

### Federation Badge

- **Location:** Prominent badge on guild icon in sidebar + guild header
- **Content:** Instance shorthand (e.g., "AV", "RPi5")
- **Style:** Large enough to be immediately obvious, not a tiny corner overlay. Colored banner/ribbon across bottom of guild icon.
- **Also shown on:** Federated user avatars in member list and message author area (smaller)

### Code Cleanup

Remove all `isFederatedGuild()` branching and separate federation rendering paths. Remove:

- `federatedGuilds` store (guilds are just guilds now)
- `federatedGuildIds` derived set
- `isFederatedGuild()` helper
- All `if (federated) { ... } else { ... }` branches in components
- Separate federation API methods that duplicate local ones (e.g., `postFederatedGuildMessage` merges into `postMessage`)
- `FederatedGuild` type (replaced by regular `Guild` with `instance_id`)

The API client detects `instance_id` on the guild/channel and routes accordingly. Components don't care.

---

## 13. Backend Code Cleanup

### Functions/Handlers to Remove

When new implementations replace old ones, remove completely:

| Old Code | Replaced By |
|---|---|
| `federation_guild_cache` table + all queries | Real `guilds`/`channels`/`roles` tables |
| `federation_channel_mirrors` table + all queries | Direct channel rows with `instance_id` |
| `updateGuildCacheFromEvent()` | Standard guild/channel event handlers with `instance_id` guard |
| `persistInboundMessage()` mirror lookup | Direct channel lookup (channels are real rows) |
| Separate federation proxy handlers (e.g., `HandleGetFederatedGuildMessages`) | Same handlers as local, with write-routing for `instance_id` |
| `FederatedGuild` Go struct | Standard `Guild` model with `InstanceID` field |
| GuildID fallback extraction in `routeEvent()` | Mandatory GuildID in all event envelopes |

### Error Handling Fixes

| Current Bug | Fix |
|---|---|
| Silent message drop when channel mirror missing | Removed — channels are real rows, no mirror lookup |
| Silent `pool.Exec` error in `updateGuildCacheFromEvent` | Removed — cache table deleted |
| Race condition in guild member count | Wrap in transaction with `RETURNING` |
| Presence `guild_ids` query error ignored | Fail-closed: don't send if query fails |
| Delivery receipts use `ON CONFLICT (id)` | Use `ON CONFLICT (message_id, source_instance, target_instance)` |

---

## 14. Migration Plan

Implemented across multiple numbered migrations:

| Migration | File | Description |
|---|---|---|
| 063 | `063_federation_full_parity` | Add `instance_id` to guilds; add `shorthand`/`voice_mode` to `instances`; add `federation_events` table; fix `federation_delivery_receipts` unique constraint |
| 064 | `064_drop_federation_guild_cache` | Drop `federation_guild_cache` table |
| 065 | `065_rename_federation_channel_mirrors` | Rename `federation_channel_mirrors` → `federation_dm_channel_map` |
| 066 | `066_add_instance_id_to_content_tables` | Add `instance_id` to: channels, roles, guild_members, messages, attachments, embeds, reactions, pins, webhooks; backfill from parent guild |

Down migrations reverse all changes.

---

## 15. New Federation Endpoints Summary

### Remote-Facing (instance receives from peers)

```
POST /federation/v1/sync                                    -> Backfill events since HLC
GET  /federation/v1/users/{userId}/profile                  -> Full user profile
POST /federation/v1/guilds/{guildID}/manage                 -> Remote guild management (all actions)
GET  /federation/v1/invites/{code}                          -> Resolve invite preview
POST /federation/v1/invites/{code}/accept                   -> Accept cross-instance invite
POST /federation/v1/guilds/{guildID}/channels/{channelID}/mls/key-package  -> MLS key package
POST /federation/v1/guilds/{guildID}/channels/{channelID}/mls/commit       -> MLS commit
POST /federation/v1/guilds/{guildID}/channels/{channelID}/mls/welcome      -> MLS welcome
```

### Local Proxy (local user calling through to remote)

```
GET  /api/v1/federation/media/{instanceId}/{fileId}         -> Media proxy with caching
```

Most other proxy endpoints are eliminated — local API handlers detect `instance_id` and route automatically.

---

## 16. Success Criteria

Federation is complete when:

1. A user on `dev.amityvox.chat` can join a guild on `app.amityvox.chat` and it appears in their sidebar identically to a local guild — categories, channels, threads, encryption indicators, galleries, member list
2. Messages sent and received in real-time with full media rendering (via proxy)
3. Presence shows correctly for all federated users in shared guilds
4. Voice/video/screenshare works in federated guild channels and DMs
5. Remote guild management works when permissions are granted
6. Cross-instance invites work from any federated peer
7. MLS encryption works in federated encrypted channels
8. Aggregated guild discovery searches all peers
9. No separate code paths — one component, one store, one API handler for local and federated
10. All old federation-specific code (caches, mirrors, separate stores, branching) is deleted
11. All identified bugs (GuildID missing, race conditions, silent drops, presence gaps) are fixed
