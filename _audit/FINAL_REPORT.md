# AmityVox Pre-Release Audit -- Final Report

**Date:** 2026-02-18
**Auditor:** Automated static analysis (Claude Opus 4.6)
**Codebase:** ~72,000 LOC, commit 804f1d3
**Phases completed:** 8 (Inventory, Completeness Backend/Frontend, Event Chain, Forward Wiring, Reverse Wiring, Security, Performance, Best Practices)

---

## Release Readiness Verdict

**CONDITIONAL -- Do Not Ship Until Critical Issues Are Resolved**

The codebase is architecturally sound and feature-complete (121 of 136 features fully implemented), but two systemic defects and three security vulnerabilities make it unsafe for production release in its current state.

**Blockers for release (must fix):**
1. The `PublishJSON()` event dispatch failure causes ~85% of real-time event paths to silently drop events, breaking reactions, polls, guild lifecycle events, link embeds, and more. This is the single largest defect.
2. Three critical security vulnerabilities: session token exposure via the sessions list API, search results bypassing channel access controls, and outgoing webhook SSRF.
3. Swallowed database errors in permission checks can silently permit banning/kicking the guild owner.

**Can ship after fixing the above** with the remaining High/Medium/Low issues tracked as fast-follow work.

---

## Executive Summary

AmityVox is a remarkably complete self-hosted communication platform. Across 136 audited features, 121 are fully implemented, 12 are partial (mostly experimental features with limited real-time sync), and 3 are intentional stubs (file upload fallback, mobile client, desktop client). The Go backend consists of ~40 handler files with real database queries, proper input validation, and consistent API envelopes. The SvelteKit frontend has 114 components using modern Svelte 5 runes with zero legacy patterns, 495 tests across 33 files, and clean import resolution. The architecture -- PostgreSQL for truth, NATS for events, DragonflyDB for cache, LiveKit for voice -- is well-designed for the Raspberry Pi 5 target.

However, the audit uncovered a critical systemic defect in the real-time event pipeline: the `PublishJSON()` method creates events with empty routing envelope fields (`GuildID`, `ChannelID`, `UserID`), and the gateway's `shouldDispatchTo()` function uses a fail-closed default. This means events published via `PublishJSON()` without matching subject-prefix fallbacks are silently dropped. Of ~80 event publish calls across the codebase, only 24 produce working end-to-end chains. Guild lifecycle events (`GUILD_CREATE`, `GUILD_UPDATE`, `GUILD_DELETE`) are entirely broken because the Guild struct serializes as `{"id": ...}` but the prefix fallback looks for `{"guild_id": ...}`. All reaction, poll, automod, and embed-update events are similarly broken.

On the security front, the codebase demonstrates strong fundamentals (parameterized SQL everywhere, Argon2id hashing, 256-bit session tokens, fail-closed dispatch), but three critical and five high-severity issues were found. The most urgent is that the session management API returns raw session tokens (which are the Bearer authentication tokens) for all active sessions, enabling single-session compromise to escalate to full account takeover. The search endpoint returns messages from channels the requesting user has no access to. Outgoing webhooks have no SSRF protection, unlike the federation module which correctly blocks private IPs.

Performance analysis revealed that the message-send hot path executes 20-30 sequential database queries (permission checks alone require 3-4 round-trips each, and the handler calls permissions multiple times). The gateway dispatch loop executes database queries per-event per-client inside a mutex, creating O(events x clients) query amplification. These are fixable with query consolidation and caching, estimated at 6-8 hours of work for the top three.

---

## Findings by Severity

### Critical (10)

| ID | Source Phase | Summary | File:Line | Effort |
|----|-------------|---------|-----------|--------|
| CRIT-01 | Phase 2C (Events) | `PublishJSON()` creates events with empty routing envelope fields; ~85% of real-time events silently dropped by `shouldDispatchTo()` default `return false` | `internal/gateway/gateway.go:912-923`, `internal/events/events.go` | 8-16h |
| CRIT-02 | Phase 2C (Events) | `GUILD_CREATE`/`UPDATE`/`DELETE` all broken: Guild struct uses `json:"id"` but prefix fallback looks for `guild_id` | `internal/models/models.go:158`, `internal/gateway/gateway.go:1008-1018` | 2-4h |
| CRIT-03 | Phase 4 (Security) | Session tokens (Bearer auth tokens) exposed via `GET /users/@me/sessions` -- single session compromise escalates to full account takeover | `internal/api/users/users.go:852-876` | 1-2h |
| CRIT-04 | Phase 4 (Security) | Search results bypass channel/guild access controls -- any authenticated user can read messages from private channels | `internal/api/search_handlers.go:20-133` | 4-8h |
| CRIT-05 | Phase 4 (Security) | Outgoing webhook SSRF -- no IP validation, attacker with ManageWebhooks can target internal services (169.254.169.254, localhost, Docker network) | `internal/api/webhooks/webhooks.go:620-651` | 2-3h |
| CRIT-06 | Phase 5 (Performance) | Gateway `shouldDispatchTo` makes DB queries per-event per-client inside mutex; O(events x clients) query amplification saturates connection pool | `internal/gateway/gateway.go:995-1063` | 2-4h |
| CRIT-07 | Phase 5 (Performance) | `hasGuildPermission`/`hasChannelPermission` execute 3-4 sequential DB queries per call; many handlers call multiple times | `internal/api/guilds/guilds.go:2330-2376`, `internal/api/channels/channels.go:2665-2770` | 3-4h |
| CRIT-08 | Phase 5 (Performance) | `HandleCreateMessage` executes ~25 sequential queries before the INSERT (hottest path in the application) | `internal/api/channels/channels.go:470-620` | 2-4h |
| CRIT-09 | Phase 6 (Quality) | Swallowed DB errors in permission checks: if owner_id query fails, `ownerID=""` passes `memberID == ownerID` check, allowing ban/kick of guild owner | `internal/api/guilds/guilds.go:812,897,2342,2349` | 1-2h |
| CRIT-10 | Phase 6 (Quality) | No focus trapping in any modal component; WCAG 2.1 SC 2.4.3 violation across all 8+ modals with `aria-modal="true"` | `web/src/lib/components/common/Modal.svelte` | 2-4h |

### High (22)

| ID | Source Phase | Summary | File:Line | Effort |
|----|-------------|---------|-----------|--------|
| HIGH-01 | Phase 2C (Events) | `shouldDispatchTo()` missing prefix handlers for `amityvox.message.*`, `amityvox.poll.*`, `amityvox.automod.*`, `amityvox.presence.*`, `amityvox.user.*` | `internal/gateway/gateway.go:938-1063` | 4-6h |
| HIGH-02 | Phase 2C (Events) | 24 event types dispatched by gateway but no `case` in frontend `gateway.ts` -- silently dropped | `web/src/lib/stores/gateway.ts` | 8-12h |
| HIGH-03 | Phase 2A (Backend) | Backup trigger (`HandleTriggerBackup`) creates fake "completed" entries with 0 bytes -- no actual backup runs | `internal/api/admin/selfhost.go:1494-1499` | 4-8h |
| HIGH-04 | Phase 2B (Frontend) | StageChannelView: speakers/audience lists never populated from WebSocket -- users can "join" but see/hear no one | `web/src/lib/components/channels/StageChannelView.svelte:17-20` | 4-8h |
| HIGH-05 | Phase 3 (Wiring) | 5 Activities API methods use channel-scoped URLs (`/channels/{id}/activities/...`) but backend registers under `/activities/{id}/...` -- all return 404 | `web/src/lib/api/client.ts`, `internal/api/server.go` | 1-2h |
| HIGH-06 | Phase 3 (Wiring) | `ackWelcome` sends POST to `/encryption/welcome/{id}/ack` but backend expects DELETE to `/encryption/welcome/{id}` | `web/src/lib/api/client.ts`, `internal/api/server.go` | 15min |
| HIGH-07 | Phase 3B (Reverse) | ~215 orphaned backend routes (53% of all routes) with no frontend consumer -- social (28), admin (49), voice (21), experimental (21), activities (18) | `internal/api/server.go` | N/A (design gap) |
| HIGH-08 | Phase 4 (Security) | WebSocket gateway accepts any origin via `OriginPatterns: []string{"*"}` -- enables cross-site WebSocket hijacking | `internal/gateway/gateway.go:201` | 1h |
| HIGH-09 | Phase 4 (Security) | TOTP validation uses `==` instead of `subtle.ConstantTimeCompare` -- timing side-channel | `internal/auth/auth.go:526` | 15min |
| HIGH-10 | Phase 4 (Security) | Registration token validation leaks state via distinct error messages and database timing | `internal/api/server.go:1155-1168` | 1h |
| HIGH-11 | Phase 4 (Security) | `X-Forwarded-For` trusted without validation -- rate limiting trivially bypassable, audit logs poisoned | `internal/api/ratelimit.go:233-236`, `internal/api/server.go:1172,1208` | 1-2h |
| HIGH-12 | Phase 4 (Security) | Admin routes lack middleware-level authorization; each of 60+ handlers must manually call `isAdmin()` | `internal/api/server.go:963-1073` | 1-2h |
| HIGH-13 | Phase 5 (Performance) | N+1 query in `HandleUpdateGuildMember` role assignment: DELETE + N individual INSERTs, not in transaction | `internal/api/guilds/guilds.go:784-789` | 1h |
| HIGH-14 | Phase 5 (Performance) | N+1 query in crosspost fanout: per-follower INSERT + UPDATE + PublishJSON | `internal/api/channels/channels.go:2356-2386` | 2h |
| HIGH-15 | Phase 5 (Performance) | N+1 query in message bulk decrypt: per-message UPDATE (up to 100) | `internal/api/channels/channels.go:2825-2836` | 1h |
| HIGH-16 | Phase 5 (Performance) | Unbounded SELECT queries on 5+ guild list endpoints (bans, invites, emoji, categories, webhooks) -- no LIMIT | `internal/api/guilds/guilds.go:860,1342,1498,1652,1804` | 2-4h |
| HIGH-17 | Phase 5 (Performance) | `shouldDispatchTo` for PRESENCE_UPDATE iterates all clients to find guild IDs: O(N^2) | `internal/gateway/gateway.go:938-973` | 1h |
| HIGH-18 | Phase 6 (Quality) | 16,132 lines of API handler code across 14 packages with zero test coverage | Multiple files in `internal/api/` | 40-80h |
| HIGH-19 | Phase 6 (Quality) | Duplicated `writeJSON`/`writeError` functions across 19 sub-packages (38 duplicate definitions) | All sub-packages under `internal/api/` | 2-4h |
| HIGH-20 | Phase 6 (Quality) | Direct `json.NewEncoder` bypassing standard API envelope in ban list export and user settings | `internal/api/moderation/ban_lists.go:351`, `internal/api/users/users.go:1099,1126` | 1h |
| HIGH-21 | Phase 6 (Quality) | Swallowed errors in voice handlers: `RemoveParticipant`, `EnsureRoom`, `LogSoundboardPlay` failures silently ignored | `internal/api/voice_handlers.go:222,445,448,1009,1010` | 1h |
| HIGH-22 | Phase 6 (Quality) | Missing ARIA attributes on 7 interactive components (EmojiPicker, GiphyPicker, StickerPicker, StatusPicker, MessageInput, IncomingCallModal, ModerationModals) | Multiple component files | 2-4h |

### Medium (30)

| ID | Source Phase | Summary | File:Line | Effort |
|----|-------------|---------|-----------|--------|
| MED-01 | Phase 2A (Backend) | Code snippet execution is a stub -- returns placeholder, not 501 | `internal/api/experimental/experimental.go:1235-1236` | 1h |
| MED-02 | Phase 2A (Backend) | `SubjectMessageReactionClr` defined but never published -- no "clear all reactions" feature | `internal/events/events.go:27` | 2-4h |
| MED-03 | Phase 2A (Backend) | `SubjectRaidLockdown` defined but never published -- raid detection does not broadcast event | `internal/events/events.go:80` | 2-4h |
| MED-04 | Phase 2A (Backend) | `cross_channel_quotes` table exists in DB with indexes but no Go code reads/writes to it | `migrations/032_message_features.up.sql:2` | 1h |
| MED-05 | Phase 2B (Frontend) | StageChannelView: placeholder speaker/audience arrays never populated from any real-time source | `web/src/lib/components/channels/StageChannelView.svelte:17-20` | 4-8h |
| MED-06 | Phase 2B (Frontend) | Whiteboard `saveState()` catches all errors silently -- data loss risk if backend save fails | `web/src/lib/components/channels/Whiteboard.svelte:320-328` | 30min |
| MED-07 | Phase 2B (Frontend) | Whiteboard has no WebSocket subscription for real-time collaboration | `web/src/lib/components/channels/Whiteboard.svelte` | 4-8h |
| MED-08 | Phase 2B (Frontend) | Experimental components (Whiteboard, KanbanBoard, LocationShare, ActivityFrame) lack WebSocket real-time sync | Multiple experimental components | 8-16h |
| MED-09 | Phase 2C (Events) | CHANNEL_ACK dispatched to ALL guild members instead of the specific user -- privacy leak of read receipts | `internal/api/channels/channels.go:1273`, `internal/gateway/gateway.go` | 1h |
| MED-10 | Phase 3B (Reverse) | Embed type mismatch: Go `Embed` struct and TS `Embed` type have significantly different field structures | `internal/models/models.go`, `web/src/lib/types/index.ts` | 2-4h |
| MED-11 | Phase 3B (Reverse) | TS `Guild` type missing 6 fields the backend sends (system_channel_join/leave/kick/ban, afk_channel_id, afk_timeout) | `web/src/lib/types/index.ts` | 1h |
| MED-12 | Phase 3B (Reverse) | TS `Channel` type missing 4 fields (default_permissions, read_only, read_only_role_ids, default_auto_archive_duration) | `web/src/lib/types/index.ts` | 1h |
| MED-13 | Phase 3B (Reverse) | 5 Go model types with no TypeScript equivalent (WebAuthnCredential, ChannelPermissionOverride, ChannelTemplate, BotGuildPermission, MessageComponent) | `internal/models/models.go`, `web/src/lib/types/index.ts` | 2h |
| MED-14 | Phase 4 (Security) | CORS wildcard reflects arbitrary origins when configured with `"*"` | `internal/api/server.go:1440-1477` | 30min |
| MED-15 | Phase 4 (Security) | No Content-Type validation blocks SVG uploads (which can contain JavaScript) | `internal/media/media.go` | 1-2h |
| MED-16 | Phase 4 (Security) | READY payload design fragility: `SelfUser` includes email; future code change could leak emails for other users | `internal/gateway/gateway.go:~611` | 15min |
| MED-17 | Phase 4 (Security) | No automatic cleanup of expired sessions from `user_sessions` table | `internal/auth/auth.go` | 1-2h |
| MED-18 | Phase 4 (Security) | In-memory DM spam tracker not shared across instances; bypass in multi-instance deployments | `internal/api/channels/channels.go:42-127` | 2-4h |
| MED-19 | Phase 4 (Security) | Session validation via DB index lookup (non-constant-time) -- theoretical only, no action needed | `internal/auth/auth.go` | N/A |
| MED-20 | Phase 5 (Performance) | Frontend message store grows unbounded across channel switches | `web/src/lib/stores/messages.ts` | 2-4h |
| MED-21 | Phase 5 (Performance) | MessageList and MemberList do not use virtual scrolling | `web/src/lib/components/chat/MessageList.svelte`, `web/src/lib/components/layout/MemberList.svelte` | 4-8h |
| MED-22 | Phase 5 (Performance) | `dmSpamTracker` goroutine in `init()` has no shutdown mechanism -- goroutine leak | `internal/api/channels/channels.go:119-127` | 1h |
| MED-23 | Phase 5 (Performance) | Redundant queries for same channel/guild data within `HandleCreateMessage` (5-8 duplicate fetches) | `internal/api/channels/channels.go:470-600` | 2h |
| MED-24 | Phase 5 (Performance) | Workers do not implement backoff on errors -- sustained error load during outages | `internal/workers/workers.go:112-141` | 1-2h |
| MED-25 | Phase 5 (Performance) | Gateway `context.Background()` queries bypass request cancellation | `internal/gateway/gateway.go:1001,1023,1033,1048,1058` | 1h |
| MED-26 | Phase 5 (Performance) | Retention worker deletes from search index one-by-one (1000 individual HTTP requests per batch) | `internal/workers/retention_worker.go:189-193` | 1h |
| MED-27 | Phase 6 (Quality) | 40+ hardcoded color values in CSS/styles bypassing theme system | Multiple component files | 2-4h |
| MED-28 | Phase 6 (Quality) | Untested non-API internal packages: notifications (646L), plugins (741L), middleware (934L), scanning (379L) | Multiple files in `internal/` | 8-16h |
| MED-29 | Phase 6 (Quality) | Gateway presence publish error and `last_online` DB update error both silently ignored | `internal/gateway/gateway.go:1234,281` | 30min |
| MED-30 | Phase 6 (Quality) | Ignored DB errors in block/relationship checks allow friend requests to blocked users on DB failure | `internal/api/users/users.go:387,579`, `internal/api/moderation/moderation.go:581` | 1h |

### Low (20)

| ID | Source Phase | Summary | File:Line | Effort |
|----|-------------|---------|-----------|--------|
| LOW-01 | Phase 2A (Backend) | Custom domain DNS verification is manual-only (admin clicks verify without DNS lookup) | `internal/api/admin/selfhost.go:1101-1102` | 2-4h |
| LOW-02 | Phase 2A (Backend) | `stubHandler("upload_file")` for when S3 not configured -- intentional, well-tested | `internal/api/server.go:895` | N/A |
| LOW-03 | Phase 2A (Backend) | `SubjectVoiceServerUpdate` defined but never published -- forward-looking for multi-region | `internal/events/events.go:60` | N/A |
| LOW-04 | Phase 2A (Backend) | Empty `messages` package (placeholder with explanatory comment) | `internal/api/messages/messages.go` | N/A |
| LOW-05 | Phase 2B (Frontend) | `ActivityFrame` shows static placeholder for `builtin://` URLs | `web/src/lib/components/channels/ActivityFrame.svelte:266-275` | 2-4h |
| LOW-06 | Phase 2B (Frontend) | LocationShare live update interval catches and suppresses all errors silently | `web/src/lib/components/chat/LocationShare.svelte:141-143` | 15min |
| LOW-07 | Phase 2B (Frontend) | LocationShare `stopLiveSharing()` ignores cleanup errors; zombie location could persist | `web/src/lib/components/chat/LocationShare.svelte:155-157` | 15min |
| LOW-08 | Phase 2B (Frontend) | VideoRecorder `formattedTime` uses `$derived(() => ...)` anti-pattern (returns function not value) | `web/src/lib/components/chat/VideoRecorder.svelte:30-34` | 15min |
| LOW-09 | Phase 2B (Frontend) | KanbanBoard has no real-time updates; other users' changes require reload | `web/src/lib/components/channels/KanbanBoard.svelte` | 4-8h |
| LOW-10 | Phase 2B (Frontend) | `console.log` for build detection reload in ws.ts | `web/src/lib/api/ws.ts:113` | 5min |
| LOW-11 | Phase 3 (Wiring) | `deleteKeyPackage` parameter naming inconsistency (JS `keyPackageId` vs chi `packageID`) | `web/src/lib/api/client.ts`, `internal/api/server.go` | 5min |
| LOW-12 | Phase 4 (Security) | Default Argon2id parameters (64MB) may be conservative; should be configurable | `internal/auth/auth.go:117` | 1h |
| LOW-13 | Phase 4 (Security) | Webhook token visible in execution URL path (industry standard pattern) | `internal/api/webhooks/webhooks.go` | N/A |
| LOW-14 | Phase 4 (Security) | Federation signature parsing uses `fmt.Sscanf` instead of `hex.DecodeString` | `internal/federation/federation.go` | 15min |
| LOW-15 | Phase 4 (Security) | No per-endpoint brute force protection on login (global rate limit only) | `internal/api/server.go:~1199` | 2-4h |
| LOW-16 | Phase 4 (Security) | In-memory DM spam tracker state lost on restart (brief spam window) | `internal/api/channels/channels.go:42-127` | (covered by MED-18) |
| LOW-17 | Phase 5 (Performance) | Reorder handlers (roles/channels) execute per-item UPDATE queries in transaction | `internal/api/guilds/guilds.go:1258-1266,1309-1317` | 1h |
| LOW-18 | Phase 5 (Performance) | Server guide step insert in a loop (max 20 queries) | `internal/api/guilds/guilds.go:2887-2902` | 30min |
| LOW-19 | Phase 5 (Performance) | Frontend `handleBulkPin` makes sequential API calls (no parallelism) | `web/src/lib/components/chat/MessageList.svelte:84-96` | 30min |
| LOW-20 | Phase 5 (Performance) | Database connection pool MinConns=2 may cause cold-start penalty | `internal/database/database.go:38` | 15min |

---

## Systemic Issues

### 1. The PublishJSON Dispatch Failure (CRIT-01 + CRIT-02 + HIGH-01)

This is the single largest defect in the codebase. The `PublishJSON()` method creates events with all routing envelope fields (`GuildID`, `ChannelID`, `UserID`) set to empty strings. The gateway's `shouldDispatchTo()` function depends on these fields for routing decisions and defaults to `return false` (fail-closed). Combined with missing subject-prefix handlers for `amityvox.message.*`, `amityvox.poll.*`, and `amityvox.automod.*`, and the `id` vs `guild_id`/`channel_id` JSON tag mismatch on Guild/Channel structs, this causes:

- 33 event publish paths to fail at dispatch (events never reach any client)
- 24 event types to be dropped by the frontend (no switch case in gateway.ts)
- All reaction events, all poll events, all automod events, all embed updates, and most guild lifecycle events are broken

**Root cause fix:** Convert all `PublishJSON()` calls to `Publish()` with proper envelope fields, OR add a universal fallback in `shouldDispatchTo()` that extracts routing fields from event data before returning false, with support for both `id` and `guild_id`/`channel_id` JSON keys.

### 2. The Permission Query Cascade (CRIT-07 + CRIT-08 + MED-23)

Permission checks are implemented as 3-4 sequential database queries each, and many handlers call them multiple times without caching results. The `HandleCreateMessage` hot path executes ~25 queries before the actual INSERT, including 3 redundant fetches of `guild_id` from the channels table and 2 redundant fetches of `owner_id` from the guilds table. This is the primary driver of message send latency and database connection pool pressure.

**Root cause fix:** Consolidate permission computation into a single CTE query. Fetch all channel state in one query at handler entry. Pass computed results to downstream permission checks instead of re-querying.

### 3. The Frontend Integration Gap (HIGH-07 + HIGH-02)

53% of backend routes (215 of ~403) have no frontend consumer. Entire feature categories -- social (28 routes), admin self-hosting (49 routes), voice advanced features (21 routes), experimental (21 routes), and activities (18 routes) -- were implemented on the backend with full database persistence, input validation, and NATS event publishing, but no corresponding frontend UI was ever built. Additionally, 24 WebSocket event types that the gateway successfully dispatches have no handler in the frontend's `gateway.ts` switch statement. This means these features are backend-complete but user-inaccessible without direct API calls.

---

## Quick Wins (< 2 hours each)

1. **CRIT-03: Stop returning session tokens** -- Change `HandleGetSelfSessions` to return a hash or truncated prefix instead of the raw session ID. (1-2h)
2. **CRIT-09: Fix swallowed DB errors in permission checks** -- Add error returns and 500 responses for failed owner_id/flags queries. (1-2h)
3. **HIGH-06: Fix ackWelcome** -- Change frontend from `POST /encryption/welcome/{id}/ack` to `this.del(\`/encryption/welcome/${welcomeId}\`)`. (15min)
4. **HIGH-09: Fix TOTP timing** -- Replace `==` with `subtle.ConstantTimeCompare` in auth.go:526 and mfa_handlers.go. (15min)
5. **HIGH-05: Fix Activities URL mismatch** -- Align 5 frontend API methods to use `/activities/...` instead of `/channels/{id}/activities/...`. (1-2h)
6. **HIGH-12: Add RequireAdmin middleware** -- Create middleware that checks admin flag, apply to admin route group. (1-2h)
7. **HIGH-11: Fix X-Forwarded-For trust** -- Use `r.RemoteAddr` (already set by chi RealIP middleware) instead of manual header parsing. (1-2h)
8. **HIGH-08: Restrict WebSocket origin** -- Configure `OriginPatterns` from allowed_origins config instead of `["*"]`. (1h)
9. **HIGH-10: Normalize registration errors** -- Return single generic error for all token validation failures. (1h)
10. **MED-06: Fix Whiteboard silent save** -- Add toast notification when save fails. (30min)
11. **MED-14: Warn on wildcard CORS** -- Add startup warning if CORS origins contain `"*"` in production mode. (30min)
12. **MED-29: Log gateway errors** -- Add `slog.Warn` for presence publish and last_online update failures. (30min)

---

## Major Remediation Items (> 4 hours each)

1. **Fix PublishJSON dispatch system** (CRIT-01 + CRIT-02 + HIGH-01) -- Convert ~27 PublishJSON calls to Publish with envelope fields, add missing prefix handlers, fix id/guild_id mismatch. Estimated: 8-16 hours.

2. **Add search access control** (CRIT-04) -- Pre-filter Meilisearch queries by accessible channels or post-filter results. Estimated: 4-8 hours.

3. **Add 24 frontend event handlers** (HIGH-02) -- Implement switch cases in gateway.ts for all missing event types (reactions, embeds, member add/remove, role create, bans, emoji, polls, screen share, soundboard, etc.). Estimated: 8-12 hours.

4. **Consolidate message-send query path** (CRIT-07 + CRIT-08 + MED-23) -- Single channel state query, single permission CTE, eliminate redundant fetches. Estimated: 4-8 hours.

5. **Add SSRF protection to outgoing webhooks** (CRIT-05) -- Resolve hostnames, block private IP ranges, validate redirects. Estimated: 2-3 hours.

6. **Add focus trapping to modals** (CRIT-10) -- Implement focus trap in Modal.svelte, propagates to all 10+ downstream modals. Estimated: 2-4 hours.

7. **Implement frontend virtual scrolling** (MED-21) -- Add windowed rendering for MessageList and MemberList. Estimated: 4-8 hours.

8. **Cache gateway channel/guild lookups** (CRIT-06) -- LRU cache for channel-to-guild mapping, client-side guild membership sets. Estimated: 2-4 hours.

9. **Wire orphaned feature frontends** (HIGH-07) -- Build UI for social, admin self-hosting, voice advanced, and activities features. Estimated: 40-80+ hours (major initiative).

10. **Expand test coverage** (HIGH-18) -- Add tests for moderation, voice handlers, bots, social packages. Estimated: 40-80 hours.

---

## What's Done Well

1. **Feature completeness is exceptional.** 121 of 136 features fully implemented, including federation (guilds, DMs, voice), end-to-end encryption (MLS), 5 bridge adapters (Matrix, Discord, Telegram, Slack, IRC), a Go bot SDK, 53 database migrations, and comprehensive admin tooling. This is far beyond a typical MVP.

2. **Zero SQL injection vectors.** Every database query across ~20,000+ lines of handler code uses parameterized queries via pgx. No string concatenation in SQL was detected.

3. **Strong authentication design.** Argon2id hashing, 256-bit session token entropy, TOTP 2FA, WebAuthn/passkeys, backup codes, and HaveIBeenPwned integration for password breach checking.

4. **Fail-closed security defaults.** The gateway's `shouldDispatchTo()` defaults to `return false`, WebSocket requires authentication, and the permission system has a comprehensive 9-step resolution algorithm with Administrator bypass only at step 4.

5. **Modern frontend architecture.** 100% Svelte 5 runes adoption with zero legacy patterns. Clean import resolution. 495 tests across 33 test files. Consistent use of Map-based stores with proper reactivity triggers.

6. **Production-grade resilience patterns.** Federation retry with exponential backoff (JetStream durable consumer), frontend WebSocket reconnection with decorrelated jitter, bounded replay buffers, and proper goroutine lifecycle management with context cancellation.

7. **Consistent code style.** `slog` for all Go logging (zero `log.Print` or `fmt.Print`), consistent API envelope format, correct HTTP status codes throughout (201 for creation, 204 for deletion), and snake_case JSON fields matching between backend and frontend.

8. **Well-designed architecture.** PostgreSQL as source of truth, NATS as event bus, DragonflyDB as cache, LiveKit for voice, and S3-compatible storage. The stateless Go binary design enables horizontal scaling. The Raspberry Pi 5 target keeps resource awareness high.

9. **Comprehensive deployment tooling.** Docker Compose with multi-stage build (16MB final image), Caddy reverse proxy with Let's Encrypt, backup/restore scripts, and a first-run setup wizard.

10. **Federation done right.** Ed25519 signed messages, HLC ordering, SSRF protection on federation endpoints, instance health tracking, retry queues, blocklists/allowlists, and per-instance profiles. This is far beyond what most self-hosted platforms offer.

---

## Recommended Remediation Order

### Sprint 1: Security Blockers (1-2 days, ~12 hours)

1. CRIT-03: Stop returning session tokens in session list (1-2h)
2. CRIT-04: Add access control to message search (4-8h)
3. CRIT-05: Add SSRF protection to outgoing webhooks (2-3h)
4. CRIT-09: Fix swallowed DB errors in permission checks (1-2h)
5. HIGH-09: Use constant-time TOTP comparison (15min)

### Sprint 2: Event Pipeline Fix (2-3 days, ~16-24 hours)

6. CRIT-01 + CRIT-02: Fix PublishJSON dispatch -- convert to Publish with envelope fields, fix id/guild_id mismatch (8-16h)
7. HIGH-01: Add missing prefix handlers in shouldDispatchTo (4-6h)
8. MED-09: Fix CHANNEL_ACK privacy leak (1h)

### Sprint 3: Frontend Event Handlers + Wiring Fixes (2-3 days, ~12-16 hours)

9. HIGH-02: Add 24 missing frontend event handlers in gateway.ts (8-12h)
10. HIGH-05: Fix Activities URL mismatches (1-2h)
11. HIGH-06: Fix ackWelcome method/URL (15min)

### Sprint 4: Performance Hot Path (1-2 days, ~8-12 hours)

12. CRIT-06: Cache gateway channel/guild lookups (2-4h)
13. CRIT-07 + CRIT-08 + MED-23: Consolidate permission + message-send queries (4-8h)
14. HIGH-17: Fix O(N^2) presence dispatch (1h)

### Sprint 5: Security Hardening (1 day, ~6-8 hours)

15. HIGH-08: Restrict WebSocket origins (1h)
16. HIGH-11: Fix X-Forwarded-For trust (1-2h)
17. HIGH-12: Add RequireAdmin middleware (1-2h)
18. HIGH-10: Normalize registration errors (1h)
19. MED-15: Block SVG uploads (1-2h)

### Sprint 6: Accessibility + Quality (2-3 days)

20. CRIT-10: Add focus trapping to Modal.svelte (2-4h)
21. HIGH-22: Add ARIA attributes to picker/modal components (2-4h)
22. HIGH-19: Consolidate writeJSON/writeError (2-4h)
23. HIGH-21: Log voice handler errors (1h)

### Ongoing: Test Coverage + Feature Gaps (multi-sprint)

24. HIGH-18: Test coverage for moderation, voice, bots, social (40-80h)
25. HIGH-07: Wire orphaned backend features to frontend (40-80h+)
26. MED-21: Virtual scrolling for message/member lists (4-8h)

---

## Statistics

### Findings by Phase

| Phase | Scope | Critical | High | Medium | Low | Total |
|-------|-------|----------|------|--------|-----|-------|
| 2A | Backend Completeness | 0 | 1 | 4 | 4 | 9 |
| 2B | Frontend Completeness | 0 | 1 | 4 | 6 | 11 |
| 2C | Event Chain Audit | 2 | 1 | 1 | 0 | 4 |
| 3 | Forward Wiring (FE->BE) | 0 | 2 | 0 | 1 | 3 |
| 3B | Reverse Wiring (BE->FE) | 0 | 1 | 4 | 0 | 5 |
| 4 | Security | 3 | 5 | 6 | 5 | 19 |
| 5 | Performance | 3 | 5 | 7 | 4 | 19 |
| 6 | Best Practices / Quality | 2 | 5 | 4 | 0 | 11 |
| **Total** | | **10** | **22** | **30** | **20** | **82** |

Note: 1 additional LOW finding (LOW-20) was identified from Performance Phase 5 (pool MinConns), not counted in the per-phase Low column for Phase 5 above due to it being reclassified. Total unique findings: **82**.

### Findings by Category

| Category | Critical | High | Medium | Low | Total |
|----------|----------|------|--------|-----|-------|
| Events / Dispatch | 2 | 2 | 1 | 0 | 5 |
| Security | 3 | 5 | 6 | 5 | 19 |
| Performance | 3 | 5 | 7 | 4 | 19 |
| Completeness | 0 | 4 | 8 | 10 | 22 |
| Wiring | 0 | 3 | 4 | 1 | 8 |
| Quality / Accessibility | 2 | 3 | 4 | 0 | 9 |

### Estimated Total Remediation Effort

| Priority | Item Count | Estimated Hours |
|----------|-----------|----------------|
| Sprint 1 (Security Blockers) | 5 | 8-15h |
| Sprint 2 (Event Pipeline) | 3 | 13-23h |
| Sprint 3 (Frontend Events) | 3 | 10-14h |
| Sprint 4 (Performance) | 3 | 7-13h |
| Sprint 5 (Security Hardening) | 5 | 5-8h |
| Sprint 6 (Accessibility + Quality) | 4 | 7-13h |
| Ongoing (Tests + Feature Gaps) | 3 | 84-168h |
| **Total** | **26 items** | **134-254h** |

### Feature Completeness

| Status | Count | Percentage |
|--------|-------|------------|
| IMPLEMENTED | 121 | 89% |
| PARTIAL | 12 | 9% |
| STUB | 3 | 2% |
| MISSING | 0 | 0% |

### Event Pipeline Health

| Metric | Count |
|--------|-------|
| Total event publish paths | ~80 |
| Fully working end-to-end (OK) | 24 (30%) |
| Broken at dispatch (BROKEN-DISPATCH) | 33 (41%) |
| No frontend handler (BROKEN-FRONTEND) | 24 (30%) |
| Not published | 3 |
| Internal only | 1 |

### Test Coverage

| Component | Source Modules | Test Files | File Coverage |
|-----------|---------------|------------|--------------|
| Backend Go packages | 80 | 31 | ~39% |
| Frontend (components + stores + utils) | ~151 | 33 | ~22% |
| **Combined** | **~231** | **64** | **~28%** |
