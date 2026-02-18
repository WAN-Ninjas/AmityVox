# Backend Completeness Audit (Phase 2A)

**Date:** 2026-02-18
**Scope:** All Go backend code -- stubs, placeholders, unused subjects, dead code, incomplete features

---

## 1. TODO/FIXME/Placeholder Markers Found

### Finding 1.1: Code snippet execution is a stub
- **File:** `/docker/AmityVox/internal/api/experimental/experimental.go`
- **Lines:** 1235-1236
- **Context:** `HandleRunCodeSnippet` has a `NOTE:` comment stating "Server-side code execution requires a sandboxed runtime (not yet implemented)." The handler returns a hardcoded placeholder string `[AmityVox] Server-side execution for {language} is not yet configured.` rather than actually running the code.
- **Severity:** MEDIUM
- **Impact:** The code snippet "run" endpoint exists and is routed, but never actually executes user code. It writes placeholder output to the database. The feature is intentionally incomplete (sandboxed execution is a significant security undertaking), but users get a misleading response that looks like a success.
- **Recommendation:** Either remove the `/run` endpoint and mark the feature as "view only", or add a clear `501 Not Implemented` response so the frontend can display a proper message.

### Finding 1.2: Custom domain DNS verification is manual-only
- **File:** `/docker/AmityVox/internal/api/admin/selfhost.go`
- **Lines:** 1101-1102
- **Context:** `HandleVerifyCustomDomain` contains the comment `// For now, admin manually marks as verified after checking DNS.` and `// In a full implementation, this would do DNS TXT record lookup.` The handler simply sets `verified = true` without any actual DNS validation.
- **Severity:** LOW
- **Impact:** Domain verification is trust-based (admin clicks "verify" without proof). This is acceptable for self-hosted instances where the admin controls DNS, but the endpoint comment acknowledges it's incomplete.
- **Recommendation:** Acceptable as-is for a self-hosted product. Add a `net.LookupTXT` check as an enhancement in the future.

### Finding 1.3: Backup trigger simulates completion synchronously
- **File:** `/docker/AmityVox/internal/api/admin/selfhost.go`
- **Lines:** 1494-1499
- **Context:** `HandleTriggerBackup` inserts a backup history entry with status "running", then immediately marks it as "completed" with `size_bytes = 0`. Comment says `// Simulate completion (in production, this would be async via a worker).` No actual backup (pg_dump or file copy) is performed.
- **Severity:** HIGH
- **Impact:** The backup scheduling UI gives a false sense of security. Admins see "completed" backups that contain no data. The real backup mechanism is in `scripts/backup.sh` which runs outside the app, but the in-app "Run Backup" button does nothing useful.
- **Recommendation:** Either wire `HandleTriggerBackup` to actually invoke `pg_dump` via `exec.Command`, or remove the "Run" button from the UI and document that backups must be run via the CLI script.

---

## 2. Stub/Placeholder Handlers

### Finding 2.1: `stubHandler("upload_file")` route
- **File:** `/docker/AmityVox/internal/api/server.go`
- **Line:** 895
- **Context:** When `s.Media` is nil (media service not configured), the `/files/upload` route is wired to `stubHandler("upload_file")` which returns `501 Not Implemented`.
- **Severity:** LOW
- **Impact:** This is a legitimate fallback -- if no S3 storage is configured, uploads are correctly rejected. The stub is well-tested (`TestStubHandler` in `api_test.go`).
- **Recommendation:** No action needed. This is the expected pattern for optional features.

### Finding 2.2: All major handler files are fully implemented (NOT stubs)
After thorough inspection of all five flagged handler files:

**`internal/api/experimental/experimental.go` (1,769 lines):** FULLY IMPLEMENTED. Contains 20+ handlers covering location sharing, message effects, super reactions, AI summarization, voice transcription, whiteboards, code snippets, video recordings, and kanban boards. Every handler performs real DB queries (INSERT/SELECT/UPDATE/DELETE with pgx), validates input, and publishes NATS events. One exception: `HandleRunCodeSnippet` returns a placeholder response (see Finding 1.1 above).

**`internal/api/activities/activities.go` (1,424 lines):** FULLY IMPLEMENTED. Activity catalog CRUD, activity sessions, mini-games (trivia, tic-tac-toe, chess, drawing with full initialization logic and turn-based state management), Watch Together, Music Listening Party with queue management. All handlers hit the database.

**`internal/api/social/social.go` (2,011 lines):** FULLY IMPLEMENTED. Server insights (with live daily/hourly snapshot generation), boost system (with tier computation), vanity URL claims (with transaction-safe claiming), achievements (with automatic criteria checking across message_count, reaction_count, guild_join_count, account_age_days), leveling/XP (with cooldown, role assignment, level-up messages), starboard (full reaction-threshold checking with self-star exclusion and NSFW filtering), welcome messages, and auto-roles (including delayed assignment via goroutines). All handlers are production-quality.

**`internal/api/bots/bots.go` (1,453 lines):** FULLY IMPLEMENTED. Bot CRUD, secure token generation (SHA-256 hashed, avbot_ prefixed), slash command registration, bot guild permissions with scope validation, message component interactions (button/select menu), bot presence management, event subscriptions with webhook URLs, rate limit configuration, and admin bot listing with batch-loaded related data.

**`internal/api/admin/selfhost.go` (1,633 lines):** FULLY IMPLEMENTED. Setup wizard, auto-update checks, health dashboard (PostgreSQL health, runtime stats, trend tracking), storage dashboard (breakdown by content type, table sizes, top uploaders, upload trends), data retention policies (with actual DELETE execution), custom domains, backup scheduling. Two caveats documented above: DNS verification is manual, and backup trigger is simulated.

---

## 3. Unused NATS Subjects Analysis

### Finding 3.1: `SubjectMessageReactionClr` ("amityvox.message.reaction_clear")
- **File:** `/docker/AmityVox/internal/events/events.go`, line 27
- **Defined:** Yes
- **Published (Publish/PublishJSON calls using this constant):** No
- **Subscribed:** No (the gateway subscribes to wildcard `amityvox.message.>` which would match, but no code publishes to it)
- **Severity:** MEDIUM
- **Analysis:** This subject is intended for "clear all reactions from a message" functionality. The reaction system has add (`SubjectMessageReactionAdd`) and remove (`SubjectMessageReactionDel`) which are both published, but there is no "clear all" handler that would publish to `SubjectMessageReactionClr`. This represents an **incomplete feature** -- the "clear all reactions" operation is missing.
- **Recommendation:** Either implement a `HandleClearAllReactions` handler that DELETEs all reactions for a message and publishes to this subject, or remove the constant if the feature is not planned.

### Finding 3.2: `SubjectRaidLockdown` ("amityvox.guild.raid_lockdown")
- **File:** `/docker/AmityVox/internal/events/events.go`, line 80
- **Defined:** Yes
- **Published:** No -- grep finds zero `Publish.*SubjectRaidLockdown` calls across the entire codebase
- **Subscribed:** No
- **Severity:** MEDIUM
- **Analysis:** The guild raid protection config exists in the database (`guild_raid_config` table) and moderation handlers manage it, but no code actually publishes a lockdown event when a raid is detected. The auto-moderation system (`internal/api/moderation/moderation.go`) likely handles raid detection but doesn't broadcast a real-time lockdown event to connected clients.
- **Recommendation:** When raid lockdown is triggered (e.g., when join rate exceeds threshold), publish to this subject so the gateway can notify guild admins and lock the UI in real time.

### Finding 3.3: `SubjectVoiceServerUpdate` ("amityvox.voice.server_update")
- **File:** `/docker/AmityVox/internal/events/events.go`, line 60
- **Defined:** Yes
- **Published:** No
- **Subscribed:** No
- **Severity:** LOW
- **Analysis:** Voice is powered by LiveKit, which has its own signaling. `SubjectVoiceStateUpdate` IS published (when users join/leave voice channels), but `SubjectVoiceServerUpdate` (which would carry LiveKit server endpoint changes) is never used. In a multi-region deployment, this would notify clients to reconnect to a different LiveKit server. For a single-instance self-hosted deployment, this is unnecessary.
- **Recommendation:** This is a forward-looking constant for multi-region voice. No action needed for v1.0. Can be removed during a cleanup pass if multi-region voice is not on the roadmap.

---

## 4. Dead Code

### Finding 4.1: `internal/api/messages/messages.go` -- Empty package
- **File:** `/docker/AmityVox/internal/api/messages/messages.go`
- **Lines:** 5 total (package declaration + comment)
- **Content:** `// Package messages is reserved for future message-specific utilities.`
- **Severity:** LOW
- **Impact:** Zero. The file is a placeholder with an explanatory comment noting that message operations live in the `channels` package (which is correct -- messages are scoped under `/channels/{channelID}/messages/`).
- **Recommendation:** No action needed. This is a documented architectural decision, not dead code. Removing the file would be fine but unnecessary.

### Finding 4.2: `cross_channel_quotes` table has no readers
- **File:** `/docker/AmityVox/internal/database/migrations/032_message_features.up.sql`, line 2
- **Table Schema:** `cross_channel_quotes (id, message_id, channel_id, quoted_message_id, quoted_channel_id, created_at)`
- **Severity:** MEDIUM
- **Impact:** The table exists in the database with indexes but no Go code reads from or writes to it. This was part of a cross-channel quote/reference feature that was created in migration 032 but never had API handlers or frontend UI built.
- **Recommendation:** Either implement cross-channel quoting (a useful feature: "quote a message from another channel") or add a DOWN migration to remove the table.

---

## 5. Incomplete Features

### Finding 5.1: Code snippet server-side execution
- **Feature:** Run code snippets in a sandboxed environment
- **What works:** Code snippets can be created, stored, and retrieved with full DB persistence. The language is validated against 30+ supported languages.
- **What's missing:** The `/run` endpoint returns a placeholder instead of actual execution. No sandbox runtime (e.g., Docker-based execution, WASM, or Firecracker) is integrated.
- **Severity:** MEDIUM
- **Files:** `/docker/AmityVox/internal/api/experimental/experimental.go:1216-1255`

### Finding 5.2: Backup execution is simulated
- **Feature:** In-app backup scheduling and manual trigger
- **What works:** Backup schedules can be CRUD'd with full metadata. History tracking is in place.
- **What's missing:** `HandleTriggerBackup` immediately marks backups as "completed" with 0 bytes. No actual pg_dump or S3 backup is performed. The real backup mechanism is the external `scripts/backup.sh`.
- **Severity:** HIGH
- **Files:** `/docker/AmityVox/internal/api/admin/selfhost.go:1456-1507`

### Finding 5.3: Custom domain DNS verification not automated
- **Feature:** Custom domain management for guilds
- **What works:** Domain registration, verification token generation, manual admin verification, deletion.
- **What's missing:** Automated DNS TXT record lookup (`net.LookupTXT`) to verify domain ownership.
- **Severity:** LOW
- **Files:** `/docker/AmityVox/internal/api/admin/selfhost.go:1091-1122`

### Finding 5.4: Three NATS subjects defined but never published
- **Feature:** Reaction clear-all, raid lockdown notification, voice server migration
- **What works:** The constants are defined and the wildcard JetStream subjects would route them.
- **What's missing:** No handlers publish to `SubjectMessageReactionClr`, `SubjectRaidLockdown`, or `SubjectVoiceServerUpdate`.
- **Severity:** MEDIUM (reaction clear-all), MEDIUM (raid lockdown), LOW (voice server update)
- **Files:** `/docker/AmityVox/internal/events/events.go:27,60,80`

---

## Summary

| Category | Critical | High | Medium | Low | Total |
|---|---|---|---|---|---|
| TODO/FIXME markers | 0 | 1 | 1 | 1 | 3 |
| Stub/placeholder handlers | 0 | 0 | 0 | 1 | 1 |
| Unused NATS subjects | 0 | 0 | 2 | 1 | 3 |
| Dead code | 0 | 0 | 1 | 1 | 2 |
| Incomplete features | 0 | 1 | 2 | 1 | 4 |
| **Total** | **0** | **2** | **6** | **5** | **13** |

### Overall Assessment

The backend is remarkably complete for a project of this scope. All five large handler files (experimental, activities, social, bots, selfhost) contain fully implemented handlers with real database queries, proper input validation, error handling, and NATS event publication. There are zero CRITICAL findings.

The two HIGH severity findings are both in the admin self-hosting tooling:
1. Backup trigger creates fake "completed" entries (no actual backup runs)
2. These are admin-facing features that do not affect the core messaging platform

The MEDIUM findings (unused NATS subjects, cross_channel_quotes table, code snippet execution stub) represent features that were designed but not fully connected. None of these affect the stability or security of the running platform.
