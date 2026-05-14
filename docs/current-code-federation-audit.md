# Current-Code Federation Audit

Last reviewed: 2026-05-13

This file ignores prior plans and docs. It describes what the checked-in code currently does.

## Actual Ownership Model

The live schema makes `users.instance_id` and `guilds.instance_id` required from migration 001. Local users and local guilds are created with the local instance ID.

Evidence:

- `internal/database/migrations/001_initial_schema.up.sql`: `users.instance_id TEXT NOT NULL`, `guilds.instance_id TEXT NOT NULL`.
- `cmd/amityvox/main.go`: `ensureLocalInstance` creates or loads the local `instances` row.
- `internal/auth/auth.go`: registration inserts users with `instance_id = local instance`.
- `internal/api/guilds/guilds.go`: guild creation inserts `instance_id = h.InstanceID`.

Later migrations add nullable `instance_id` to child/content tables: `channels`, `roles`, `guild_members`, `messages`, `attachments`, `embeds`, `reactions`, `pins`, `webhooks`. In practice most local writes do not fill those child `instance_id` columns.

So the current rule is:

- `users` and `guilds`: local means `instance_id == local instance ID`.
- Child/content tables: often null even for local rows; sometimes intended to be populated for federated rows, but many federation insert paths omit it.

Any code using `instance_id IS NULL` to mean local is wrong for `users` and `guilds`.

## Route Wiring

Federation routes are mounted in `cmd/amityvox/main.go`, not in the normal API server registration.

Public/signed federation endpoints include:

- `/.well-known/amityvox`
- `/federation/v1/handshake`
- `/federation/v1/inbox`
- `/federation/v1/sync`
- `/federation/v1/users/lookup`
- `/federation/v1/users/{userID}/profile`
- `/federation/v1/dm/*`
- `/federation/v1/guilds/*`
- `/federation/v1/invites/*`
- `/federation/v1/voice/token`
- `/federation/v1/guilds/{guildID}/channels/{channelID}/mls/*`

Authenticated local proxy routes include:

- `/api/v1/federation/guilds/*`
- `/api/v1/federation/users/*`
- `/api/v1/federation/peers/*`
- `/api/v1/federation/discover`
- `/api/v1/federation/invites/resolve`
- `/api/v1/federation/voice/*`
- `/api/v1/federation/media/{instanceId}/{fileId}`

Normal guild/channel handlers use `apiutil.FederationProxy`. They treat a guild as remote only if `guild.instance_id != local instance ID`. This part matches the actual schema.

## Discovery and Peer Setup

Discovery returns instance ID, domain, public key, federation mode, protocol versions, capabilities, LiveKit URL, shorthand, and voice mode.

Peer registration stores discovered instances by domain. If a known domain reports a new instance ID, `migrateInstanceID` tries to rewrite many FK references to the new ID.

Handshake is unsigned. It relies on discovery of `SenderDomain`, timestamp freshness, optional source IP checks, and federation policy. The signed endpoints later rely on the stored public key.

`IsFederationAllowed` applies:

1. cached peer control,
2. explicit block or allow in `federation_peer_controls`,
3. local `instances.federation_mode`:
   - `open`: allow,
   - `closed`: deny,
   - `allowlist`: allow only active `federation_peers`.

## Guild Federation Flow

Local user joining remote guild:

1. Frontend calls `api.joinFederatedGuild`.
2. `HandleProxyJoinFederatedGuild` signs a request to the remote instance.
3. Remote `HandleFederatedGuildJoin` verifies the signed request, creates a remote user stub, inserts `guild_members`, adds the sender instance to `federation_channel_peers`, and returns a guild snapshot.
4. Local `HandleProxyJoinFederatedGuild` inserts the remote guild, categories, channels, roles, and local membership into local tables.

Current behavior problems:

- The local mirror insert is not transactional.
- Mirrored channels are inserted without `channels.instance_id`.
- Mirrored roles are inserted without `roles.instance_id`.
- Mirrored `guild_members` are inserted without `guild_members.instance_id`.
- The returned channel snapshot is partial: no slowmode, nsfw, permissions, read-only state, forum/gallery config, voice bitrate/user limit, etc.
- The returned role snapshot is partial: no permission bitfields, hoist, mentionable, or created_at.
- If the guild row insert fails after the remote join succeeded, the handler still returns success with raw join data.

Remote guild management from local APIs:

- Normal handlers call `ProxyToHomeInstance`.
- `ForwardToHomeInstance` signs a `manageRequest` and posts to remote `/federation/v1/guilds/{guildID}/manage`.
- Remote `HandleManage` verifies the guild is owned by that instance by allowing `instance_id == local instance ID`.

This mostly matches the actual `guilds.instance_id` schema.

## Message Federation Flow

Outbound local event routing:

1. REST handlers publish local events to NATS.
2. `SyncService.StartRouter` subscribes to selected subjects.
3. `routeEvent` resolves missing guild ID from channel ID, suppresses re-forwarding for remote-owned guilds, stamps attachment instance IDs, signs a `FederatedMessage`, and sends it to channel peers or all peers.

Inbound inbox:

1. `HandleInbox` reads `SignedPayload`.
2. It verifies sender public key, signature, timestamp, optional IP, and federation allow policy.
3. If `guild_id` is missing, it tries to recover it from payload data.
4. For channel events, it calls `persistInboundMessage`.
5. It publishes mapped event data to local NATS.
6. If a guild ID exists, it calls `updateFederatedGuildFromEvent`.

Current behavior problems:

- Inbound message persistence only stores `id`, `channel_id`, `author_id`, `content`, `created_at`.
- It drops attachments, embeds, nonce, replies, mentions, flags, encryption fields, voice fields, message type, and author details.
- It inserts messages without `messages.instance_id`.
- It writes reactions to `message_reactions`, but the schema and local API use `reactions`. No migration creates `message_reactions`.
- Wire event names are inconsistent. Local API publishes `MESSAGE_REACTION_ADD` and `MESSAGE_REACTION_REMOVE`; inbound mapping expects `REACTION_ADD` and `REACTION_REMOVE`.
- `HandleInbox` accepts missing `guild_id` if it can recover one, so bad envelopes are hidden instead of rejected.

## Backfill

Backfill endpoint:

- `HandleSync` verifies signed request and returns rows from `federation_events` for authorized guild IDs after a requested HLC.
- `RequestBackfill` asks a peer for events and replays them through `persistInboundMessage` and `updateFederatedGuildFromEvent`.

Current behavior problems:

- `federation_events` rows are written only inside `persistInboundMessage`.
- Guild-only events, role events, channel events, member events, and local host-side outbound events are not consistently recorded as replayable rows.
- Stored HLC uses extracted `created_at` or current time with counter `0`, not the actual federation envelope HLC.
- The backfill replay path inherits the incomplete message persistence behavior.

## DM Federation

Remote DM creation:

- `HandleFederatedDMCreate` validates that at least one recipient is local by `users.instance_id == local instance ID`.
- It creates local mirror channels and stores mappings in `federation_dm_channel_map`.
- It registers the sender as a channel peer.

Outbound DM creation:

- `NotifyFederatedDM` signs and posts `/federation/v1/dm/create` to remote recipients' instances.
- It creates a local mapping after the remote acknowledges.

Current behavior problems:

- Federated DM channels are inserted without `channels.instance_id`; migration 066 only backfills old mapped channels.
- Federated DM messages are inserted without `messages.instance_id`.
- DM message federation stores only basic message fields.
- 1:1 DM duplicate prevention is partly DB-trigger based, partly app-side; cross-instance channel mapping can still get complicated because remote and local channel IDs differ.

## Media Federation

Frontend helpers:

- `avatarUrl(id, instanceId)` and `fileUrl(id, instanceId)` use `/api/v1/federation/media/{instanceId}/{fileId}` whenever any `instanceId` is provided.

Server proxy:

- Looks up instance domain by ID.
- Fetches `https://{domain}/api/v1/files/{fileId}`.
- Caches small files in DragonflyDB and larger files on disk.

Current behavior problems:

- Many frontend call sites pass `instance_id` directly. Since local users/guilds also have a local instance ID, local media can be routed through the federation media proxy unless the component explicitly compares against current user/local instance.
- For local development or private domains, this can turn a local file load into an HTTPS self-fetch and fail.
- The helper should know the local instance ID or callers must consistently pass null for local media. Current callers are inconsistent.

## MLS Federation

MLS handlers are mounted, but `verifyChannelInLocalGuild` rejects any guild whose `instance_id` is non-null. Because local guilds have local instance ID, MLS federation endpoints currently reject local guilds as not local.

`HandleMLSSendWelcome` also rejects any receiver whose `users.instance_id` is non-null. Because local users have local instance ID, it rejects local receivers.

This is a concrete current-code bug caused by old null-local assumptions.

## Aggregated Discover

`HandleAggregatedDiscover` queries local discoverable guilds through `queryLocalDiscoverableGuilds`.

That query filters `g.instance_id IS NULL`. Since local guilds have local instance ID, local discoverable guilds never appear in aggregated federation discover.

The normal local `/api/v1/guilds/discover` path may still show local guilds because it does not use this helper.

## Leave Cleanup

`HandleProxyLeaveFederatedGuild` removes local membership after a successful remote leave.

It then counts remaining local members using `u.instance_id IS NULL`. Since local users have local instance ID, the count is always zero. That means leaving a federated guild can delete the local mirrored guild even when other local users are still members.

## Frontend Current Behavior

The frontend mostly understands the actual ownership model:

- `isGuildFederated(guild, localInstanceId)` returns true when `guild.instance_id !== localInstanceId`.
- Many layout components show badges only when `guild.instance_id !== currentUser.instance_id`.

But media helpers do not understand the local instance, and several components pass `instance_id` blindly. This can proxy local media through federation by mistake.

`npm run check` currently fails broadly, so frontend type contracts are not reliable.

## Verification

Commands run:

- `docker run --rm -v /docker/AmityVox:/build -w /build -e GOTOOLCHAIN=local golang:1.26-alpine go test -run '^$' ./internal/federation ./internal/api/... ./internal/models ./internal/database`
  - Passes compile-only.
- `docker run --rm -v /docker/AmityVox:/build -w /build -e GOTOOLCHAIN=local golang:1.26-alpine go test ./internal/federation`
  - Fails. `TestSign_And_Verify` panics because `federation.New` dereferences a nil DB pool.
- `npm run check` in `web`
  - Fails. 127 errors and 263 warnings.

## Highest-Value Fix Order

1. Replace all `instance_id IS NULL` local checks with `instance_id = local instance ID` where the table is `users` or `guilds`.
2. Fix MLS local ownership checks and federated leave cleanup.
3. Change federation reaction persistence to use `reactions` and normalize event names.
4. Make remote guild join mirror writes transactional and populate child `instance_id`.
5. Persist full inbound message data and populate `messages.instance_id`.
6. Record replayable `federation_events` for all outbound host-side events, not only inbound message persistence.
7. Fix frontend media helpers/callers so local media does not use the federation proxy.
8. Fix `federation.New` nil-pool test panic or update tests to construct the service with a DB.

## Unresolved Questions

- Should child/content `instance_id` be backfilled and required for all rows, or should only `users` and `guilds` be authoritative?
- Should remote guild message reads always proxy home, or should local mirrors be treated as cache/source for history?
- Should federation wire event names match gateway event names or use separate protocol names with explicit translation?
