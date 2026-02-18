# Event Chain Audit: NATS -> Gateway -> Frontend

**Auditor:** Claude Opus 4.6 (Phase 2C Pre-Release Audit)
**Date:** 2026-02-18
**Scope:** All 47 NATS subjects defined in `internal/events/events.go` plus 11 additional ad-hoc subjects from experimental/voice handlers

---

## How Events Flow

```
Backend Handler
   -> bus.Publish() or bus.PublishJSON()  [sets NATS subject + Event envelope]
      -> NATS subject: amityvox.<category>.<action>
         -> gateway.go subscribes to amityvox.> wildcard
            -> dispatchEvent() calls shouldDispatchTo() per client
               -> WebSocket sends {op:0, t:"EVENT_TYPE", d:{...}}
                  -> frontend ws.ts receives, emits to gateway.ts handler
                     -> gateway.ts switch(event) dispatches to store functions
                        -> Svelte components reactively render
```

**Critical distinction:** `bus.Publish()` lets the caller set envelope fields (`GuildID`, `ChannelID`, `UserID`). `bus.PublishJSON()` only sets `Type` and `Data` -- envelope routing fields are always empty strings. The `shouldDispatchTo()` function relies heavily on envelope fields for routing decisions, so events published via `PublishJSON()` without matching subject-prefix fallbacks will be silently dropped.

---

## Full Event Chain Table

### Legend

| Status | Meaning |
|--------|---------|
| OK | Full chain works: published, dispatched, handled on frontend, store updated |
| PARTIAL-DISPATCH | Dispatched by gateway but NOT handled by frontend (silently ignored) |
| PARTIAL-PUBLISH | Some publishers use Publish() (works) but others use PublishJSON() (broken) |
| BROKEN-DISPATCH | Published but shouldDispatchTo() returns false -- never reaches any client |
| BROKEN-FRONTEND | Reaches clients but no switch case in gateway.ts -- silently dropped |
| NOT-PUBLISHED | Subject defined but no handler publishes to it |
| INTERNAL-ONLY | Not intended for client dispatch (worker-to-worker) |

---

### Core Message Events

| Subject | Event Type | Publisher(s) | Publish Method | Envelope Fields | Gateway Rule | Frontend Handler | Store Updated | Status |
|---------|-----------|-------------|---------------|----------------|-------------|-----------------|--------------|--------|
| `amityvox.message.create` | `MESSAGE_CREATE` | `channels.go:685` | `Publish()` | ChannelID, UserID | ChannelID lookup -> guild or DM recipient | `case 'MESSAGE_CREATE'` | `appendMessage()`, `incrementUnread()`, `addNotification()` | **OK** |
| `amityvox.message.create` | `MESSAGE_CREATE` | `channels.go:2116` (crosspost) | `PublishJSON()` | NONE | No envelope, no prefix match -> **false** | N/A | N/A | **BROKEN-DISPATCH** |
| `amityvox.message.create` | `MESSAGE_CREATE` | `channels.go:2379` (crosspost follow) | `PublishJSON()` | NONE | No envelope, no prefix match -> **false** | N/A | N/A | **BROKEN-DISPATCH** |
| `amityvox.message.create` | `MESSAGE_CREATE` | `social.go:1231` (level-up) | `PublishJSON()` | NONE | No envelope, no prefix match -> **false** | N/A | N/A | **BROKEN-DISPATCH** |
| `amityvox.message.create` | `MESSAGE_CREATE` | `social.go:1563` (starboard) | `PublishJSON()` | NONE | No envelope, no prefix match -> **false** | N/A | N/A | **BROKEN-DISPATCH** |
| `amityvox.message.create` | `MESSAGE_CREATE` | `social.go:1733` (milestone) | `PublishJSON()` | NONE | No envelope, no prefix match -> **false** | N/A | N/A | **BROKEN-DISPATCH** |
| `amityvox.message.create` | `MESSAGE_CREATE` | `webhooks.go:832` | `PublishJSON()` | NONE | No envelope, no prefix match -> **false** | N/A | N/A | **BROKEN-DISPATCH** |
| `amityvox.message.create` | `MESSAGE_CREATE` | `plugins.go:351` | `PublishJSON()` | NONE | No envelope, no prefix match -> **false** | N/A | N/A | **BROKEN-DISPATCH** |
| `amityvox.message.create` | `MESSAGE_CREATE` | `federation/sync.go:184` | `Publish()` | GuildID, ChannelID | Envelope ChannelID -> guild/DM | via existing handler | via existing stores | **OK** |
| `amityvox.message.update` | `MESSAGE_UPDATE` | `channels.go:788` | `Publish()` | ChannelID | ChannelID lookup -> guild or DM recipient | `case 'MESSAGE_UPDATE'` | `updateMessage()` | **OK** |
| `amityvox.message.delete` | `MESSAGE_DELETE` | `channels.go:892` | `Publish()` | ChannelID | ChannelID lookup -> guild or DM recipient | `case 'MESSAGE_DELETE'` | `removeMessage()` | **OK** |
| `amityvox.message.delete` | `MESSAGE_DELETE` | `automod.go:271` | `PublishJSON()` | NONE | No envelope, no prefix match -> **false** | N/A | N/A | **BROKEN-DISPATCH** |
| `amityvox.message.delete` | `MESSAGE_DELETE_BULK` | `retention_worker.go:182` | `Publish()` | ChannelID | ChannelID lookup -> guild or DM | `case 'MESSAGE_DELETE_BULK'` | `removeMessages()` | **OK** |
| `amityvox.message.delete_bulk` | `MESSAGE_DELETE_BULK` | `channels.go:944` | `PublishJSON()` | NONE | No envelope, no prefix match -> **false** | N/A | N/A | **BROKEN-DISPATCH** |
| `amityvox.message.reaction_add` | `MESSAGE_REACTION_ADD` | `channels.go:1033` | `PublishJSON()` | NONE | No envelope, no prefix match -> **false** | NONE | NONE | **BROKEN-DISPATCH + BROKEN-FRONTEND** |
| `amityvox.message.reaction_add` | `SUPER_REACTION_ADD` | `experimental.go:441` | `PublishJSON()` | NONE | No envelope, no prefix match -> **false** | NONE | NONE | **BROKEN-DISPATCH + BROKEN-FRONTEND** |
| `amityvox.message.reaction_remove` | `MESSAGE_REACTION_REMOVE` | `channels.go:1056, 1091` | `PublishJSON()` | NONE | No envelope, no prefix match -> **false** | NONE | NONE | **BROKEN-DISPATCH + BROKEN-FRONTEND** |
| `amityvox.message.reaction_clear` | (never published) | NONE | N/A | N/A | N/A | NONE | NONE | **NOT-PUBLISHED** |
| `amityvox.message.embed_update` | `MESSAGE_EMBED_UPDATE` | `media_workers.go:271` | `PublishJSON()` | NONE | No envelope, no prefix match -> **false** | NONE | NONE | **BROKEN-DISPATCH + BROKEN-FRONTEND** |

### Channel Events

| Subject | Event Type | Publisher(s) | Publish Method | Envelope Fields | Gateway Rule | Frontend Handler | Store Updated | Status |
|---------|-----------|-------------|---------------|----------------|-------------|-----------------|--------------|--------|
| `amityvox.channel.create` | `CHANNEL_CREATE` | `guilds.go:571`, `channels.go:2654` | `Publish()` | GuildID | GuildID membership | `case 'CHANNEL_CREATE'` | `updateChannel()` | **OK** |
| `amityvox.channel.create` | `CHANNEL_CREATE` | `users.go:364, 1719` (DM creation) | `PublishJSON()` | NONE | `amityvox.channel.*` prefix -> extracts `channel_id` from data (but DM Channel struct has `id` not `channel_id`) -> fails extraction -> **false** | N/A | N/A | **BROKEN-DISPATCH** |
| `amityvox.channel.create` | `CHANNEL_CREATE` | `guilds.go:2765` (channel clone) | `PublishJSON()` | NONE | `amityvox.channel.*` prefix -> data might have `id` not `channel_id` -> fails extraction -> **false** | N/A | N/A | **BROKEN-DISPATCH** |
| `amityvox.channel.create` | `THREAD_CREATE` | `channels.go:1456` | `Publish()` | GuildID | GuildID membership | `case 'THREAD_CREATE'` | `updateChannel()` | **OK** |
| `amityvox.channel.update` | `CHANNEL_UPDATE` | `channels.go:308` | `Publish()` | GuildID, ChannelID | GuildID membership | `case 'CHANNEL_UPDATE'` | `updateChannel()` | **OK** |
| `amityvox.channel.update` | `CHANNEL_UPDATE` | `channels.go:1316` (perm override) | `PublishJSON()` | NONE | `amityvox.channel.*` prefix, data has `channel_id` -> lookup works | `case 'CHANNEL_UPDATE'` | `updateChannel()` | **OK** |
| `amityvox.channel.update` | `CHANNEL_UPDATE` | `channels.go:2927, 3008` (group DM) | `PublishJSON()` | NONE | `amityvox.channel.*` prefix, but data is full Channel struct with `id` not `channel_id` -> extraction fails -> **false** | N/A | N/A | **BROKEN-DISPATCH** |
| `amityvox.channel.update` | `CHANNEL_UPDATE` | `moderation.go:460, 505` (lock/unlock) | `PublishJSON()` | NONE | `amityvox.channel.*` prefix, data is Channel struct with `json:"id"` not `json:"channel_id"` -> extraction returns "" -> **false** | N/A | N/A | **BROKEN-DISPATCH** (VERIFIED: Channel struct has `id` not `channel_id`) |
| `amityvox.channel.update` | `CHANNEL_WIDGET_CREATE` | `widgets.go:405` | `PublishJSON()` | NONE | `amityvox.channel.*` prefix, ChannelWidget has `channel_id` -> works | NONE (no case) | NONE | **BROKEN-FRONTEND** |
| `amityvox.channel.update` | `CHANNEL_WIDGET_UPDATE` | `widgets.go:463` | `PublishJSON()` | NONE | `amityvox.channel.*` prefix, ChannelWidget has `channel_id` -> works | NONE (no case) | NONE | **BROKEN-FRONTEND** |
| `amityvox.channel.update` | `CHANNEL_WIDGET_DELETE` | `widgets.go:506` | `PublishJSON()` | NONE | `amityvox.channel.*` prefix, data has `widget_id` + `guild_id` but NO `channel_id` -> extraction fails -> **false** | NONE | NONE | **BROKEN-DISPATCH + BROKEN-FRONTEND** |
| `amityvox.channel.delete` | `CHANNEL_DELETE` | `channels.go:349` | `Publish()` | GuildID, ChannelID | GuildID membership | `case 'CHANNEL_DELETE'` | `removeChannel()`, `removeDMChannel()` | **OK** |
| `amityvox.channel.pins_update` | `CHANNEL_PINS_UPDATE` | `channels.go:1188, 1219` | `PublishJSON()` | NONE | `amityvox.channel.*` prefix, data has `channel_id` -> lookup works | NONE (no case) | NONE | **BROKEN-FRONTEND** |
| `amityvox.channel.typing_start` | `TYPING_START` | `channels.go:1238`, `gateway.go:685` | `PublishJSON()` / `Publish()` | gateway sets ChannelID, channels.go sets none but has data `channel_id` | ChannelID lookup or `amityvox.channel.*` prefix extraction | `case 'TYPING_START'` | `addTypingUser()` | **OK** |
| `amityvox.channel.ack` | `CHANNEL_ACK` | `channels.go:1273` | `PublishJSON()` | NONE | `amityvox.channel.*` prefix, data has `channel_id` + `user_id` -> lookup dispatches to ALL guild members (should be user-specific!) | NONE (no case) | NONE | **BROKEN-FRONTEND + SECURITY (leaks read receipts to all guild members)** |

### Guild Events

| Subject | Event Type | Publisher(s) | Publish Method | Envelope Fields | Gateway Rule | Frontend Handler | Store Updated | Status |
|---------|-----------|-------------|---------------|----------------|-------------|-----------------|--------------|--------|
| `amityvox.guild.create` | `GUILD_CREATE` | `guilds.go:239` | `PublishJSON()` | NONE | `amityvox.guild.*` prefix, data is Guild struct with `json:"id"` not `json:"guild_id"` -> extraction returns "" -> **false** | N/A | N/A | **BROKEN-DISPATCH** (VERIFIED: Guild struct has `id` not `guild_id`) |
| `amityvox.guild.update` | `GUILD_UPDATE` | `guilds.go:323, 453` | `PublishJSON()` | NONE | `amityvox.guild.*` prefix, data is Guild struct with `json:"id"` not `json:"guild_id"` -> extraction returns "" -> **false** | N/A | N/A | **BROKEN-DISPATCH** (VERIFIED: Guild struct has `id` not `guild_id`) |
| `amityvox.guild.update` | `GUILD_UPDATE` | `social.go:406` (boost) | `PublishJSON()` | NONE | `amityvox.guild.*` prefix, data has `id` not `guild_id` -> extraction returns "" -> **false** | N/A | N/A | **BROKEN-DISPATCH** (VERIFIED) |
| `amityvox.guild.update` | `GUILD_ONBOARDING_UPDATE` | `onboarding.go:326` | `PublishJSON()` | NONE | `amityvox.guild.*` prefix, data has `guild_id` -> works | NONE (no case) | NONE | **BROKEN-FRONTEND** |
| `amityvox.guild.update` | `SOUNDBOARD_SOUND_CREATE` | `voice_handlers.go:905` | `PublishJSON()` | NONE | `amityvox.guild.*` prefix, data has `guild_id` -> works | NONE (no case) | NONE | **BROKEN-FRONTEND** |
| `amityvox.guild.update` | `SOUNDBOARD_SOUND_DELETE` | `voice_handlers.go:949` | `PublishJSON()` | NONE | `amityvox.guild.*` prefix, data has `guild_id` -> works | NONE (no case) | NONE | **BROKEN-FRONTEND** |
| `amityvox.guild.delete` | `GUILD_DELETE` | `guilds.go:351` | `PublishJSON()` | NONE | `amityvox.guild.*` prefix, data is `{"id": guildID}` -> extraction looks for `guild_id` not `id` -> returns "" -> **false** | N/A | N/A | **BROKEN-DISPATCH** (VERIFIED: data uses `id` key) |
| `amityvox.guild.member_add` | `GUILD_MEMBER_ADD` | `guilds.go:2286`, `invites.go:201` | `PublishJSON()` | NONE | `amityvox.guild.*` prefix, data has `guild_id` -> works | NONE (no case) | NONE | **BROKEN-FRONTEND** |
| `amityvox.guild.member_update` | `GUILD_MEMBER_UPDATE` | `guilds.go:793, 2430, 2481`, `onboarding.go:677` | `PublishJSON()` | NONE | `amityvox.guild.*` prefix, data has `guild_id` -> works | `case 'GUILD_MEMBER_UPDATE'` | `updateGuildMember()`, `loadPermissions()` | **OK** |
| `amityvox.guild.member_remove` | `GUILD_MEMBER_REMOVE` | `guilds.go:387, 840` | `PublishJSON()` | NONE | `amityvox.guild.*` prefix, data has `guild_id` -> works | NONE (no case) | NONE | **BROKEN-FRONTEND** |
| `amityvox.guild.member_remove` | `GUILD_MEMBERS_PRUNE` | `guilds.go:2632` | `PublishJSON()` | NONE | `amityvox.guild.*` prefix, data has `guild_id` -> works | NONE (no case) | NONE | **BROKEN-FRONTEND** |
| `amityvox.guild.role_create` | `GUILD_ROLE_CREATE` | `guilds.go:1102` | `PublishJSON()` | NONE | `amityvox.guild.*` prefix, data is Role struct (likely has `guild_id`) | NONE (no case) | NONE | **BROKEN-FRONTEND** |
| `amityvox.guild.role_update` | `GUILD_ROLE_UPDATE` | `guilds.go:1163` | `PublishJSON()` | NONE | `amityvox.guild.*` prefix -> works | `case 'GUILD_ROLE_UPDATE'` | `loadPermissions()` | **OK** |
| `amityvox.guild.role_delete` | `GUILD_ROLE_DELETE` | `guilds.go:1205` | `PublishJSON()` | NONE | `amityvox.guild.*` prefix, data has `guild_id` | `case 'GUILD_ROLE_DELETE'` | `loadPermissions()` | **OK** |
| `amityvox.guild.ban_add` | `GUILD_BAN_ADD` | `guilds.go:957` | `PublishJSON()` | NONE | `amityvox.guild.*` prefix, data has `guild_id` -> works | NONE (no case) | NONE | **BROKEN-FRONTEND** |
| `amityvox.guild.ban_remove` | `GUILD_BAN_REMOVE` | `guilds.go:988`, `ban_worker.go:28` | `PublishJSON()` | NONE | `amityvox.guild.*` prefix, data has `guild_id` -> works | NONE (no case) | NONE | **BROKEN-FRONTEND** |
| `amityvox.guild.emoji_update` | `GUILD_EMOJI_UPDATE` | `guilds.go:1561, 1606` | `PublishJSON()` | NONE | `amityvox.guild.*` prefix, data has `guild_id` -> works | NONE (no case) | NONE | **BROKEN-FRONTEND** |
| `amityvox.guild.emoji_update` | `GUILD_EMOJI_DELETE` | `guilds.go:1634` | `PublishJSON()` | NONE | `amityvox.guild.*` prefix, data has `guild_id` -> works | NONE (no case) | NONE | **BROKEN-FRONTEND** |

### Guild Scheduled Event Events

| Subject | Event Type | Publisher(s) | Publish Method | Envelope Fields | Gateway Rule | Frontend Handler | Store Updated | Status |
|---------|-----------|-------------|---------------|----------------|-------------|-----------------|--------------|--------|
| `amityvox.guild.event_create` | `GUILD_EVENT_CREATE` | `guildevents.go:215` | `PublishJSON()` | NONE | `amityvox.guild.*` prefix, data has `guild_id` -> works | NONE (no case) | NONE | **BROKEN-FRONTEND** |
| `amityvox.guild.event_update` | `GUILD_EVENT_UPDATE` | `guildevents.go:261, 503` | `PublishJSON()` | NONE | `amityvox.guild.*` prefix, data has `guild_id` -> works | NONE (no case) | NONE | **BROKEN-FRONTEND** |
| `amityvox.guild.event_delete` | `GUILD_EVENT_DELETE` | `guildevents.go:550` | `PublishJSON()` | NONE | `amityvox.guild.*` prefix, data has `guild_id` -> works | NONE (no case) | NONE | **BROKEN-FRONTEND** |
| `amityvox.guild.raid_lockdown` | (never published) | NONE | N/A | N/A | N/A | NONE | NONE | **NOT-PUBLISHED** |

### User / Presence Events

| Subject | Event Type | Publisher(s) | Publish Method | Envelope Fields | Gateway Rule | Frontend Handler | Store Updated | Status |
|---------|-----------|-------------|---------------|----------------|-------------|-----------------|--------------|--------|
| `amityvox.presence.update` | `PRESENCE_UPDATE` | `gateway.go:1234` (broadcastPresence) | `Publish()` | UserID | PRESENCE_UPDATE with UserID -> shared guild/friend check | `case 'PRESENCE_UPDATE'` | `updatePresence()` | **OK** |
| `amityvox.presence.update` | `PRESENCE_UPDATE` | `users/activity.go:90` | `PublishJSON()` | NONE | No UserID in envelope, no prefix match -> **false** | N/A | N/A | **BROKEN-DISPATCH** |
| `amityvox.user.update` | `USER_UPDATE` | `users.go:137` | `Publish()` | UserID | USER_UPDATE with UserID -> shared guild/friend check | `case 'USER_UPDATE'` | `currentUser.set()`, `updateUserInMembers()`, `updateUserInDMs()` | **OK** |
| `amityvox.user.update` | `USER_UPDATE` | `users.go:1001` (account deletion) | `PublishJSON()` | NONE | No UserID in envelope -> falls through -> **false** | N/A | N/A | **BROKEN-DISPATCH** |
| `amityvox.user.relationship_add` | `RELATIONSHIP_ADD` | `users.go:489, 492` | `Publish()` | UserID | Not guild prefix + UserID set -> user-specific dispatch | `case 'RELATIONSHIP_ADD'` | `addOrUpdateRelationship()`, `addNotification()` | **OK** |
| `amityvox.user.relationship_update` | `RELATIONSHIP_UPDATE` | `users.go:440, 443` (accept) | `Publish()` | UserID | Not guild prefix + UserID set -> user-specific dispatch | `case 'RELATIONSHIP_UPDATE'` | `addOrUpdateRelationship()` | **OK** |
| `amityvox.user.relationship_update` | `RELATIONSHIP_UPDATE` | `users.go:678` (block) | `PublishJSON()` | NONE | No UserID -> falls through -> **false** | N/A | N/A | **BROKEN-DISPATCH** |
| `amityvox.user.relationship_update` | `RELATIONSHIP_UPDATE` | `users.go:738` (unblock) | `PublishJSON()` | NONE | No UserID -> falls through -> **false** | N/A | N/A | **BROKEN-DISPATCH** |
| `amityvox.user.relationship_remove` | `RELATIONSHIP_REMOVE` | `users.go:533, 536` | `Publish()` | UserID | Not guild prefix + UserID set -> user-specific dispatch | `case 'RELATIONSHIP_REMOVE'` | `removeRelationship()` | **OK** |

### Voice Events

| Subject | Event Type | Publisher(s) | Publish Method | Envelope Fields | Gateway Rule | Frontend Handler | Store Updated | Status |
|---------|-----------|-------------|---------------|----------------|-------------|-----------------|--------------|--------|
| `amityvox.voice.state_update` | `VOICE_STATE_UPDATE` | `voice_handlers.go:164, 228, 294, 364, 454, 462, 705, 775`, `gateway.go:734` | Mixed | gateway: GuildID, UserID; voice_handlers: `PublishJSON()` NONE | `amityvox.voice.*` prefix, data has `guild_id` -> works | `case 'VOICE_STATE_UPDATE'` | `handleVoiceStateUpdate()` | **OK** |
| `amityvox.voice.state_update` | `SOUNDBOARD_PLAY` | `voice_handlers.go:1013` | `PublishJSON()` | NONE | `amityvox.voice.*` prefix, data has `guild_id` -> dispatched | NONE (no case) | NONE | **BROKEN-FRONTEND** |
| `amityvox.voice.state_update` | `VOICE_BROADCAST_START` | `voice_handlers.go:1208` | `PublishJSON()` | NONE | `amityvox.voice.*` prefix, data has `guild_id` -> dispatched | NONE (no case) | NONE | **BROKEN-FRONTEND** |
| `amityvox.voice.state_update` | `VOICE_BROADCAST_END` | `voice_handlers.go:1257` | `PublishJSON()` | NONE | `amityvox.voice.*` prefix, data has `guild_id` -> dispatched | NONE (no case) | NONE | **BROKEN-FRONTEND** |
| `amityvox.voice.state_update` | `SCREEN_SHARE_START` | `voice_handlers.go:1393` | `PublishJSON()` | NONE | `amityvox.voice.*` prefix, data has `guild_id` -> dispatched | NONE (no case) | NONE | **BROKEN-FRONTEND** |
| `amityvox.voice.state_update` | `SCREEN_SHARE_END` | `voice_handlers.go:1454` | `PublishJSON()` | NONE | `amityvox.voice.*` prefix, data has `guild_id` -> dispatched | NONE (no case) | NONE | **BROKEN-FRONTEND** |
| `amityvox.voice.state_update` | `SCREEN_SHARE_UPDATE` | `voice_handlers.go:1534` | `PublishJSON()` | NONE | `amityvox.voice.*` prefix, data has `guild_id` -> dispatched | NONE (no case) | NONE | **BROKEN-FRONTEND** |
| `amityvox.voice.server_update` | (never published) | NONE | N/A | N/A | N/A | NONE | NONE | **NOT-PUBLISHED** |
| `amityvox.voice.call_ring` | `CALL_RING` | `voice_handlers.go:182` | `Publish()` | ChannelID, UserID | CallRing special logic: DM recipients minus caller | `case 'CALL_RING'` | `addIncomingCall()` | **OK** |

### Poll Events

| Subject | Event Type | Publisher(s) | Publish Method | Envelope Fields | Gateway Rule | Frontend Handler | Store Updated | Status |
|---------|-----------|-------------|---------------|----------------|-------------|-----------------|--------------|--------|
| `amityvox.poll.create` | `POLL_CREATE` | `polls.go:146` | `PublishJSON()` | NONE | No envelope, `amityvox.poll.*` doesn't match any prefix -> **false** | NONE (no case) | NONE | **BROKEN-DISPATCH + BROKEN-FRONTEND** |
| `amityvox.poll.vote` | `POLL_VOTE` | `polls.go:391` | `PublishJSON()` | NONE | No envelope, `amityvox.poll.*` doesn't match any prefix -> **false** | NONE (no case) | NONE | **BROKEN-DISPATCH + BROKEN-FRONTEND** |
| `amityvox.poll.close` | `POLL_CLOSE` | `polls.go:450` | `PublishJSON()` | NONE | No envelope, `amityvox.poll.*` doesn't match any prefix -> **false** | NONE (no case) | NONE | **BROKEN-DISPATCH + BROKEN-FRONTEND** |

### AutoMod Events

| Subject | Event Type | Publisher(s) | Publish Method | Envelope Fields | Gateway Rule | Frontend Handler | Store Updated | Status |
|---------|-----------|-------------|---------------|----------------|-------------|-----------------|--------------|--------|
| `amityvox.automod.action` | `AUTOMOD_ACTION` | `automod.go:308` | `PublishJSON()` | NONE | No envelope, `amityvox.automod.*` doesn't match any prefix -> **false** | NONE (no case) | NONE | **BROKEN-DISPATCH + BROKEN-FRONTEND** |

### Announcement Events

| Subject | Event Type | Publisher(s) | Publish Method | Envelope Fields | Gateway Rule | Frontend Handler | Store Updated | Status |
|---------|-----------|-------------|---------------|----------------|-------------|-----------------|--------------|--------|
| `amityvox.announcement.create` | `ANNOUNCEMENT_CREATE` | `admin.go:910` | `PublishJSON()` | NONE | `amityvox.announcement.*` prefix -> broadcast to ALL clients | `case 'ANNOUNCEMENT_CREATE'` | `addAnnouncement()` | **OK** |
| `amityvox.announcement.update` | `ANNOUNCEMENT_UPDATE` | `admin.go:1042` | `PublishJSON()` | NONE | `amityvox.announcement.*` prefix -> broadcast to ALL clients | `case 'ANNOUNCEMENT_UPDATE'` | `updateAnnouncement()` | **OK** |
| `amityvox.announcement.delete` | `ANNOUNCEMENT_DELETE` | `admin.go:1075` | `PublishJSON()` | NONE | `amityvox.announcement.*` prefix -> broadcast to ALL clients | `case 'ANNOUNCEMENT_DELETE'` | `removeAnnouncement()` | **OK** |

### Federation Events

| Subject | Event Type | Publisher(s) | Publish Method | Envelope Fields | Gateway Rule | Frontend Handler | Store Updated | Status |
|---------|-----------|-------------|---------------|----------------|-------------|-----------------|--------------|--------|
| `amityvox.federation.retry` | `FEDERATION_RETRY` | `sync.go:459` | `Publish()` | NONE | Default -> **false** | NONE | NONE | **INTERNAL-ONLY** (worker-to-worker, not client-facing) |

### Experimental / Ad-Hoc Events (not defined in events.go constants)

| Subject | Event Type | Publisher(s) | Publish Method | Envelope Fields | Gateway Rule | Frontend Handler | Store Updated | Status |
|---------|-----------|-------------|---------------|----------------|-------------|-----------------|--------------|--------|
| `amityvox.channel.location_share` | `LOCATION_SHARE_CREATE` | `experimental.go:154` | `PublishJSON()` | NONE | `amityvox.channel.*` prefix, data has `channel_id` -> lookup works | NONE (no case) | NONE | **BROKEN-FRONTEND** |
| `amityvox.channel.location_share` | `LOCATION_SHARE_UPDATE` | `experimental.go:207` | `PublishJSON()` | NONE | `amityvox.channel.*` prefix -> depends on data | NONE | NONE | **BROKEN-FRONTEND** |
| `amityvox.channel.location_share` | `LOCATION_SHARE_STOP` | `experimental.go:235` | `PublishJSON()` | NONE | `amityvox.channel.*` prefix, data has `channel_id` -> works | NONE | NONE | **BROKEN-FRONTEND** |
| `amityvox.message.effect` | `MESSAGE_EFFECT_CREATE` | `experimental.go:366` | `PublishJSON()` | NONE | `amityvox.message.*` -> no prefix match -> **false** | NONE | NONE | **BROKEN-DISPATCH + BROKEN-FRONTEND** |
| `amityvox.channel.whiteboard_update` | `WHITEBOARD_UPDATE` | `experimental.go:1003` | `PublishJSON()` | NONE | `amityvox.channel.*` prefix, data has NO `channel_id` -> extraction fails -> **false** | NONE | NONE | **BROKEN-DISPATCH + BROKEN-FRONTEND** |
| `amityvox.channel.code_snippet` | `CODE_SNIPPET_CREATE` | `experimental.go:1166` | `PublishJSON()` | NONE | `amityvox.channel.*` prefix, data has NO `channel_id` -> extraction fails -> **false** | NONE | NONE | **BROKEN-DISPATCH + BROKEN-FRONTEND** |
| `amityvox.channel.kanban_update` | `KANBAN_CARD_CREATE` | `experimental.go:1706` | `PublishJSON()` | NONE | `amityvox.channel.*` prefix, data has NO `channel_id` -> extraction fails -> **false** | NONE | NONE | **BROKEN-DISPATCH + BROKEN-FRONTEND** |
| `amityvox.channel.kanban_update` | `KANBAN_CARD_MOVE` | `experimental.go:1742` | `PublishJSON()` | NONE | `amityvox.channel.*` prefix, data has NO `channel_id` -> extraction fails -> **false** | NONE | NONE | **BROKEN-DISPATCH + BROKEN-FRONTEND** |

---

## Summary of Issues

### Issue 1: PublishJSON() Does Not Set Envelope Routing Fields (ROOT CAUSE)

**Severity: CRITICAL**
**Affected: 27+ event publish calls**

`bus.PublishJSON()` creates an `Event{Type: eventType, Data: raw}` with `GuildID`, `ChannelID`, and `UserID` always set to empty strings. The `shouldDispatchTo()` function depends on these envelope fields for routing decisions. When they are empty, events either:
- Fall through to subject-prefix fallbacks (which may or may not extract fields from the data payload)
- Fall through to the default `return false` (fail-closed) and are silently dropped

**Fix options:**
1. Add `GuildID`, `ChannelID`, `UserID` parameters to `PublishJSON()` (breaking change)
2. Create a new method like `PublishJSONWithContext()` that accepts envelope fields
3. Convert all `PublishJSON()` calls to `Publish()` with explicit envelope fields
4. Modify `shouldDispatchTo()` to extract routing fields from `event.Data` as a universal fallback before returning false

Option 4 is the least invasive but adds DB queries. Option 3 is the most correct.

---

### Issue 2: Events Published But Never Reaching Frontend (BROKEN-DISPATCH)

**Severity: HIGH**
**Count: 19 broken dispatch paths**

These events are published via `PublishJSON()` to NATS subjects that have no matching prefix rule in `shouldDispatchTo()`, so the gateway silently drops them:

| Event | Subject | Root Cause |
|-------|---------|-----------|
| MESSAGE_CREATE (crosspost x2) | `amityvox.message.create` | No `amityvox.message.*` prefix handler; empty ChannelID |
| MESSAGE_CREATE (level-up, starboard, milestone) | `amityvox.message.create` | Same |
| MESSAGE_CREATE (webhook) | `amityvox.message.create` | Same |
| MESSAGE_CREATE (plugin) | `amityvox.message.create` | Same |
| MESSAGE_DELETE (automod) | `amityvox.message.delete` | Same |
| MESSAGE_DELETE_BULK | `amityvox.message.delete_bulk` | Same |
| MESSAGE_REACTION_ADD | `amityvox.message.reaction_add` | No `amityvox.message.*` prefix handler |
| MESSAGE_REACTION_REMOVE | `amityvox.message.reaction_remove` | Same |
| SUPER_REACTION_ADD | `amityvox.message.reaction_add` | Same |
| MESSAGE_EMBED_UPDATE | `amityvox.message.embed_update` | Same |
| MESSAGE_EFFECT_CREATE | `amityvox.message.effect` | Same |
| PRESENCE_UPDATE (activity API) | `amityvox.presence.update` | No UserID, no `amityvox.presence.*` prefix handler |
| USER_UPDATE (account deletion) | `amityvox.user.update` | No UserID in envelope |
| RELATIONSHIP_UPDATE (block/unblock) | `amityvox.user.relationship_update` | No UserID in envelope |
| GUILD_CREATE | `amityvox.guild.create` | Guild struct has `id` not `guild_id`; prefix extraction fails |
| GUILD_UPDATE (x3 calls) | `amityvox.guild.update` | Guild struct has `id` not `guild_id`; prefix extraction fails |
| GUILD_DELETE | `amityvox.guild.delete` | Data `{"id":...}` not `{"guild_id":...}`; prefix extraction fails |
| CHANNEL_CREATE (DM creation, clone) | `amityvox.channel.create` | Channel struct has `id` not `channel_id` |
| CHANNEL_UPDATE (group DM x2, lock/unlock x2) | `amityvox.channel.update` | Channel struct has `id` not `channel_id` |
| CHANNEL_WIDGET_DELETE | `amityvox.channel.update` | Data has no `channel_id` |
| POLL_CREATE/VOTE/CLOSE | `amityvox.poll.*` | No `amityvox.poll.*` prefix handler |
| AUTOMOD_ACTION | `amityvox.automod.*` | No `amityvox.automod.*` prefix handler |
| WHITEBOARD_UPDATE | `amityvox.channel.whiteboard_update` | Data has no `channel_id` |
| CODE_SNIPPET_CREATE | `amityvox.channel.code_snippet` | Data has no `channel_id` |
| KANBAN_CARD_CREATE/MOVE | `amityvox.channel.kanban_update` | Data has no `channel_id` |

---

### Issue 3: Events Dispatched But Not Handled on Frontend (BROKEN-FRONTEND)

**Severity: MEDIUM**
**Count: 24 event types reach clients but have no `case` in gateway.ts**

These events arrive at the WebSocket client but fall through the `switch` statement with no handler, so nothing happens in the UI:

| Event Type | Published Via Subject | What Should Happen |
|-----------|---------------------|-------------------|
| `GUILD_MEMBER_ADD` | `amityvox.guild.member_add` | Add member to member list, update count |
| `GUILD_MEMBER_REMOVE` | `amityvox.guild.member_remove` | Remove member from list, update count |
| `GUILD_MEMBERS_PRUNE` | `amityvox.guild.member_remove` | Update member count, refresh list |
| `GUILD_ROLE_CREATE` | `amityvox.guild.role_create` | Add role to role list, refresh permissions |
| `GUILD_BAN_ADD` | `amityvox.guild.ban_add` | Show ban notification (for mods) |
| `GUILD_BAN_REMOVE` | `amityvox.guild.ban_remove` | Show unban notification (for mods) |
| `GUILD_EMOJI_UPDATE` | `amityvox.guild.emoji_update` | Update emoji picker |
| `GUILD_EMOJI_DELETE` | `amityvox.guild.emoji_update` | Remove emoji from picker |
| `GUILD_EVENT_CREATE` | `amityvox.guild.event_create` | Add event to events list |
| `GUILD_EVENT_UPDATE` | `amityvox.guild.event_update` | Update event details |
| `GUILD_EVENT_DELETE` | `amityvox.guild.event_delete` | Remove event from list |
| `GUILD_ONBOARDING_UPDATE` | `amityvox.guild.update` | Refresh onboarding settings |
| `CHANNEL_PINS_UPDATE` | `amityvox.channel.pins_update` | Refresh pinned messages |
| `CHANNEL_ACK` | `amityvox.channel.ack` | Update read state (but see security issue) |
| `CHANNEL_WIDGET_CREATE` | `amityvox.channel.update` | Add widget to channel |
| `CHANNEL_WIDGET_UPDATE` | `amityvox.channel.update` | Update widget |
| `SOUNDBOARD_SOUND_CREATE` | `amityvox.guild.update` | Add soundboard sound |
| `SOUNDBOARD_SOUND_DELETE` | `amityvox.guild.update` | Remove soundboard sound |
| `SOUNDBOARD_PLAY` | `amityvox.voice.state_update` | Play soundboard audio in voice |
| `VOICE_BROADCAST_START` | `amityvox.voice.state_update` | Show broadcast indicator |
| `VOICE_BROADCAST_END` | `amityvox.voice.state_update` | Remove broadcast indicator |
| `SCREEN_SHARE_START` | `amityvox.voice.state_update` | Show screen share indicator |
| `SCREEN_SHARE_END` | `amityvox.voice.state_update` | Remove screen share indicator |
| `SCREEN_SHARE_UPDATE` | `amityvox.voice.state_update` | Update screen share settings |
| `LOCATION_SHARE_CREATE/UPDATE/STOP` | `amityvox.channel.location_share` | Real-time location sharing |

---

### Issue 4: Subjects Defined But Never Published

**Severity: LOW**

| Subject | Constant | Notes |
|---------|---------|-------|
| `amityvox.message.reaction_clear` | `SubjectMessageReactionClr` | No handler publishes this. There is no "clear all reactions" API endpoint. |
| `amityvox.voice.server_update` | `SubjectVoiceServerUpdate` | No handler publishes this. LiveKit connection info is returned directly via REST API. |
| `amityvox.guild.raid_lockdown` | `SubjectRaidLockdown` | No handler publishes this. Raid lockdown feature may be unimplemented or uses a different path. |

---

### Issue 5: CHANNEL_ACK Security Concern

**Severity: MEDIUM**

`CHANNEL_ACK` events (read receipts) are published to `amityvox.channel.ack` with `PublishJSON()`. In `shouldDispatchTo()`, the `amityvox.channel.*` prefix fallback extracts `channel_id` from the data, looks up the guild, and dispatches to ALL guild members who are in that guild.

This means **every guild member sees when another member reads a channel**, which is a privacy leak. Read receipts should be dispatched only to the user who performed the ack (user-specific event). The data contains `user_id` but it's not used for dispatch filtering.

---

### Issue 6: GUILD_CREATE/UPDATE/DELETE Are All Broken

**Severity: CRITICAL (VERIFIED)**

The Guild model struct (`internal/models/models.go:158`) uses `json:"id"`, NOT `json:"guild_id"`. The `shouldDispatchTo()` fallback for `amityvox.guild.*` subjects at line 1008-1018 tries to unmarshal `{"guild_id": ...}` from the event data. Since the Guild struct serializes to `{"id": "...", "name": "...", ...}`, the extraction always returns an empty string, and dispatch returns `false`.

**All three core guild lifecycle events are broken:**

- `GUILD_CREATE` (`guilds.go:239`): publishes full Guild struct -> `guild_id` extraction fails -> dropped
- `GUILD_UPDATE` (`guilds.go:323, 453`, `social.go:406`): publishes full Guild struct or map with `id` key -> dropped
- `GUILD_DELETE` (`guilds.go:351`): publishes `{"id": guildID}` -> dropped

**Impact:** Guild updates/deletions are never delivered to any connected client in real-time. Users must refresh the page to see guild name/icon changes, and deleted guilds remain visible until page refresh. The GUILD_CREATE event is less critical since the creator gets the HTTP response, but multi-device users won't see the new guild on other sessions.

**Also affected by the same `id` vs `guild_id` mismatch:**
- `CHANNEL_UPDATE` from `moderation.go:460, 505` (lock/unlock): Channel struct uses `json:"id"` not `json:"channel_id"`
- `CHANNEL_UPDATE` from `channels.go:2927, 3008` (group DM): same issue
- `CHANNEL_CREATE` from `users.go:364, 1719`, `guilds.go:2765`: same issue

---

### Issue 7: `shouldDispatchTo()` Missing Prefix Handlers

**Severity: HIGH**

The `shouldDispatchTo()` function has explicit prefix handling for:
- `amityvox.guild.*` -- yes
- `amityvox.voice.*` -- yes
- `amityvox.channel.*` -- yes
- `amityvox.announcement.*` -- yes

But has NO prefix handling for:
- `amityvox.message.*` -- events fall to envelope ChannelID check only
- `amityvox.poll.*` -- events silently dropped
- `amityvox.automod.*` -- events silently dropped
- `amityvox.presence.*` -- events fall to envelope UserID check only
- `amityvox.user.*` -- events fall to envelope UserID check only

This means ANY `PublishJSON()` call to these subjects will fail dispatch.

---

## Recommended Fixes (Priority Order)

### P0: Fix PublishJSON Dispatch Failures

Add a universal fallback in `shouldDispatchTo()` BEFORE the final `return false`:

```go
// Universal fallback: try to extract guild_id, channel_id, or user_id from event data.
var fallbackData struct {
    GuildID   string `json:"guild_id"`
    ChannelID string `json:"channel_id"`
    UserID    string `json:"user_id"`
    ID        string `json:"id"`
}
if json.Unmarshal(event.Data, &fallbackData) == nil {
    // Try channel_id first
    if fallbackData.ChannelID != "" {
        // ... lookup guild from channel, check membership or DM recipient
    }
    // Try guild_id
    if fallbackData.GuildID != "" {
        // ... check guild membership
    }
}
```

OR (better): Convert all `PublishJSON()` calls to `Publish()` with proper envelope fields.

### P1: Add Missing Frontend Event Handlers

Add `case` statements in `gateway.ts` for at minimum:
- `GUILD_MEMBER_ADD` / `GUILD_MEMBER_REMOVE` (member list updates)
- `GUILD_ROLE_CREATE` (role list, permissions)
- `MESSAGE_REACTION_ADD` / `MESSAGE_REACTION_REMOVE` (reaction display)
- `MESSAGE_EMBED_UPDATE` (link embeds)
- `POLL_CREATE` / `POLL_VOTE` / `POLL_CLOSE` (poll updates)
- `GUILD_EMOJI_UPDATE` / `GUILD_EMOJI_DELETE` (emoji picker)
- `CHANNEL_PINS_UPDATE` (pinned messages)
- `SCREEN_SHARE_START/END` (screen share indicators)
- `SOUNDBOARD_PLAY` (soundboard playback)

### P2: Fix CHANNEL_ACK Privacy Leak

Make CHANNEL_ACK a user-specific event by either:
1. Publishing with `Publish()` and setting `UserID` in the envelope
2. Adding a user-specific check in `shouldDispatchTo()` for this event type

### P3: Fix Guild/Channel Struct `id` vs `guild_id`/`channel_id` Mismatch (VERIFIED BROKEN)

The Guild model at `internal/models/models.go:158` uses `json:"id"`. The Channel model at `internal/models/models.go:198` uses `json:"id"`. The `shouldDispatchTo()` prefix fallback looks for `guild_id` and `channel_id` respectively.

Fix options:
1. Add a `guild_id` alias field to the Guild struct JSON output (breaks API consistency)
2. Modify `shouldDispatchTo()` to also check `id` field and cross-reference with the subject to determine if it's a guild or channel ID
3. Convert all Guild/Channel `PublishJSON()` calls to `Publish()` with proper envelope fields (recommended)
4. Change the fallback unmarshal to also try `id` when `guild_id`/`channel_id` is empty

---

## Statistics

| Category | Count |
|----------|-------|
| Total NATS subjects (defined in events.go) | 47 |
| Additional ad-hoc subjects (experimental, voice) | 11 |
| Total event publish calls (across codebase) | ~80 |
| Fully working event paths (OK) | 24 |
| Broken at dispatch level (BROKEN-DISPATCH) | 33 |
| Dispatched but no frontend handler (BROKEN-FRONTEND) | 24 |
| Subjects defined but never published (NOT-PUBLISHED) | 3 |
| Internal-only events (not client-facing) | 1 |
| Security concerns | 1 (CHANNEL_ACK privacy leak) |

### Critical Findings Summary

1. **GUILD_CREATE, GUILD_UPDATE, GUILD_DELETE are all broken** -- the three most fundamental guild lifecycle events never reach any connected client because the Guild struct serializes `id` not `guild_id`
2. **MESSAGE_CREATE from 7 out of 9 publisher paths are broken** -- only the main handler and federation sync work; crosspost, level-up, starboard, webhook, and plugin messages are silently dropped
3. **All reaction events (add/remove) are broken** -- no `amityvox.message.*` prefix handler in `shouldDispatchTo()`
4. **All poll events (create/vote/close) are broken** -- no `amityvox.poll.*` prefix handler
5. **All automod events are broken** -- no `amityvox.automod.*` prefix handler
6. **MESSAGE_DELETE_BULK from the REST API is broken** (retention worker's version works because it uses `Publish()` with envelope fields)
7. **MESSAGE_EMBED_UPDATE is broken** -- link preview unfurls never reach clients
8. **Block/unblock relationship events are broken** -- users don't get real-time feedback
9. **CHANNEL_ACK leaks read receipts** to all guild members instead of being user-specific

### Root Cause

The root cause of ~85% of all broken chains is the `PublishJSON()` method which creates an `Event` with all routing envelope fields (`GuildID`, `ChannelID`, `UserID`) set to empty strings. Combined with the `shouldDispatchTo()` function's fail-closed default and incomplete prefix-based fallback logic, this causes events to be silently dropped.

The `amityvox.guild.*` and `amityvox.channel.*` prefix fallbacks exist but look for `guild_id` / `channel_id` keys in the data payload, while the Go model structs use `id` as their primary key JSON tag. This mismatch causes even the fallback logic to fail for Guild and Channel struct payloads.
