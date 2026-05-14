# AmityVox Codebase Summary

Last reviewed: 2026-05-14

## Shape

AmityVox is a Go monolith with a SvelteKit frontend.

Current active cleanup list: `docs/current-work-backlog.md`.
Current federation audit: `docs/current-code-federation-audit.md`.
Archived plans and stale backlog material live under `docs/archive/`.

- Backend entrypoint: `cmd/amityvox/main.go`
- HTTP API: `internal/api`
- WebSocket gateway: `internal/gateway`
- PostgreSQL migrations: `internal/database/migrations`
- Shared structs: `internal/models/models.go`
- Federation protocol and proxying: `internal/federation`
- Frontend: `web/src`

The server starts as one binary. `runServe` loads config, connects PostgreSQL, runs migrations, bootstraps the local `instances` row and Ed25519 federation key, starts NATS, cache, media, search, workers, API, gateway, and federation routers.

## Main Runtime Flows

Local REST routes live under `/api/v1`. Domain handlers are mounted in `internal/api/server.go`.

Real-time events flow through `internal/events` over NATS. The gateway subscribes and dispatches to WebSocket clients. Federation also subscribes to selected local events and forwards them to peer instances.

Message CRUD is implemented mostly in `internal/api/channels/channels.go`, not `internal/api/messages`.

Frontend API calls are centralized in `web/src/lib/api/client.ts`. Svelte stores in `web/src/lib/stores` keep auth, guilds, channels, messages, DMs, voice, presence, permissions, etc.

## Federation Design in Current Code

The intended direction is "federation as data": federated guilds, channels, users, messages, and related content are stored in the same tables as local data, marked by `instance_id`.

Important federation files:

- `internal/federation/federation.go`: discovery, handshake, signatures, peer registry, allow checks, key refresh.
- `internal/federation/sync.go`: inbox, event forwarding, retry queue, dead letters, backfill sync.
- `internal/federation/guild.go`: remote-facing guild endpoints and local proxy endpoints.
- `internal/federation/proxy.go`: generic federated guild mutation proxy used by normal API handlers.
- `internal/federation/dm.go`: federated DM mirroring and DM message delivery.
- `internal/federation/voice.go`: federated LiveKit token flow.
- `internal/federation/mls.go`: MLS key package, welcome, commit, group state federation endpoints.
- `internal/federation/users.go`: federated profile fetch and local proxy endpoint.
- `internal/federation/invites.go`: cross-instance invite resolve/accept.
- `internal/federation/manage.go`: remote guild management actions.

Public federation endpoints are mounted directly in `cmd/amityvox/main.go`, not in `internal/api/server.go`.

## Federation Data Model

Initial schema uses non-null `instance_id` on `users` and `guilds`, so local rows belong to the local instance row. Later migrations add nullable `instance_id` to `channels`, `roles`, `guild_members`, `messages`, `attachments`, `embeds`, `reactions`, `pins`, and `webhooks`.

This differs from the design doc wording that says `NULL = local`. Current code is mixed:

- `users.instance_id` and `guilds.instance_id` are required by schema.
- Several code comments and queries still assume local users have `NULL instance_id`.
- `channels.instance_id`, `messages.instance_id`, and related content are often left null on new inserts even when representing federated content.

That mismatch is a major source of federation fragility.

## Federation Join Flow

Local user joins remote guild:

1. Frontend calls `/api/v1/federation/guilds/join`.
2. `HandleProxyJoinFederatedGuild` signs a join request to remote `/federation/v1/guilds/{guildID}/join` or invite accept.
3. Remote `HandleFederatedGuildJoin` verifies signature, creates remote user stub, inserts `guild_members`, registers channel peers, and returns guild structure.
4. Local proxy inserts returned guild, categories, channels, roles, and membership into real tables.

Current weakness: the local insert is not one transaction. If guild insert succeeds but channel/role/member inserts partly fail, the user can see a half-synced guild.

## Federation Event Flow

Outbound:

1. Local event published to NATS.
2. `SyncService.StartRouter` subscribes and calls `routeEvent`.
3. `routeEvent` resolves missing `guild_id` from `channel_id`, prevents re-forwarding events for remote-owned guilds, stamps attachment `instance_id`, signs the message, and sends to peers.

Inbound:

1. `HandleInbox` reads signed payload.
2. Sender key is loaded from cache or DB.
3. Signature, timestamp, optional IP check, and allow rules are checked.
4. Payload is converted to event bus format.
5. Message-like events are persisted by `persistInboundMessage`.
6. Guild-level events are applied by `updateFederatedGuildFromEvent`.

Current weakness: `HandleInbox` still silently extracts missing `guild_id` from event data. The design says missing `guild_id` must be rejected. This fallback can hide bad envelopes and make routing failures intermittent.

## High-Risk Federation Issues

1. `internal/federation/guild.go` has obvious corruption in `HandleProxyLeaveFederatedGuild`: duplicated `if guildID == "" {` and a missing close brace in the visible block. Backend compile-only tests passed, so the surrounding braces happen to balance, but this file needs careful cleanup.

2. `HandleProxyLeaveFederatedGuild` counts remaining local users with `u.instance_id IS NULL`, but `users.instance_id` is `NOT NULL` in the initial schema. This can delete local federated-guild cache even when local members remain.

3. Federation inserts often omit new `instance_id` columns:
   - `HandleProxyJoinFederatedGuild` inserts channels, roles, and guild_members without `instance_id`.
   - `persistInboundMessage` inserts messages without `instance_id`.
   - Reaction persistence uses `message_reactions`, while migration 066 adds `instance_id` to `reactions`; table naming should be verified.

4. Join response roles are too thin. `buildGuildJoinResponse` returns only role `id`, `name`, `color`, and `position`, losing permission bitfields, hoist, mentionable, and other role fields. Remote guild permissions can be wrong after sync.

5. Join response channels are partial. It omits many fields normal channel rows use: slowmode, nsfw, permissions, archive/read-only state, forum/gallery fields, voice bitrate/user limit, etc.

6. Inbound message persistence ignores attachments, embeds, nonce, replies, flags, encryption, voice message fields, mentions, and author object. UI may show the event live but history/backfill loses data.

7. Backfill stores `federation_events` only from inbound message persistence. Many outbound/local host events may not be recorded with real HLC data, so reconnect sync may miss guild state and non-message events.

8. `federation.New` queries the DB unconditionally to preload federation mode. Unit tests that pass a nil pool panic. Existing `go test ./internal/federation` fails at `TestSign_And_Verify` for this reason.

9. Source of truth for federation config is split. Docs say admin DB settings override TOML; code has config fields, instance columns, and admin endpoints. Verify every runtime path uses the DB override consistently.

10. Frontend type drift is broad. `npm run check` currently reports 127 errors in 87 files, so UI behavior should not be trusted from types alone.

## Verification Snapshot

Commands run:

- `docker run --rm -v /docker/AmityVox:/build -w /build -e GOTOOLCHAIN=local golang:1.26-alpine go test -run '^$' ./internal/federation ./internal/api/... ./internal/models ./internal/database`
  - Result: pass, compile only.
- `docker run --rm -v /docker/AmityVox:/build -w /build -e GOTOOLCHAIN=local golang:1.26-alpine go test ./internal/federation`
  - Result: fail. `TestSign_And_Verify` panics because `federation.New` dereferences nil `Pool`.
- `npm run check` in `web`
  - Result: fail. 127 errors, 263 warnings.

## Next Best Work

Fix federation in this order:

1. Align local-vs-remote `instance_id` semantics in schema, models, and queries. Decide if local rows use local instance ID or null, then make it consistent.
2. Make remote guild join storage transactional and include full channel/role/member fields.
3. Require `guild_id` in federation envelopes and remove fallback extraction.
4. Persist complete inbound message data including attachments, embeds, replies, mentions, encryption, voice fields, and `instance_id`.
5. Audit event/backfill recording so every host-side federated mutation has a replayable `federation_events` row.
6. Fix federated leave cleanup and add tests covering multiple local users in one remote guild.
7. Repair federation unit tests, especially nil-pool construction.
8. Bring frontend typecheck back to green before making large UI changes.

## Unresolved Questions

- Should local users/guilds use local `instance_id` or `NULL`? Schema and comments disagree.
- Is `message_reactions` still canonical, or should federation use `reactions` after migration 066?
- Should federated guild reads trust local mirrors first, or always proxy to the home instance for messages and members?
- What minimum federation parity is required first: guild text chat, DMs, voice, MLS, or admin moderation?
