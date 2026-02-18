# AmityVox Pre-Release Audit: Phase 6 - Best Practices & Code Quality Report

**Date:** 2026-02-18
**Auditor:** Claude Opus 4.6 (automated)
**Scope:** Backend (Go), Frontend (SvelteKit), Code Consistency

---

## Critical Quality Issues

### CRT-1: Silently Swallowed DB Query Errors in Permission Checks

**Severity:** Critical
**Files:**
- `/docker/AmityVox/internal/api/guilds/guilds.go:812` -- `_ = h.Pool.QueryRow(...).Scan(&ownerID)` in kick handler
- `/docker/AmityVox/internal/api/guilds/guilds.go:897` -- `_ = h.Pool.QueryRow(...).Scan(&ownerID)` in ban handler
- `/docker/AmityVox/internal/api/guilds/guilds.go:2342` -- `h.Pool.QueryRow(...).Scan(&userFlags)` (no error check at all)
- `/docker/AmityVox/internal/api/guilds/guilds.go:2349` -- `h.Pool.QueryRow(...).Scan(&defaultPerms)` (no error check at all)

**Problem:** In the kick and ban handlers, if the DB query to fetch `owner_id` fails (connection error, timeout), `ownerID` stays as the zero-value empty string. The subsequent `if memberID == ownerID` check passes silently, allowing the kick/ban to proceed against the owner. In the permission check function `hasPermission()`, if the user flags query fails, `userFlags` defaults to 0 and admin users lose their admin privileges silently. If `defaultPerms` fails to load, `computedPerms` starts at 0, and guild-level default permissions are lost.

**Recommended fix:** Return an error or respond with 500 when these queries fail. For `hasPermission()`, return `false` (fail-closed) and log the error.

---

### CRT-2: No Focus Trapping in Any Modal Component

**Severity:** Critical (Accessibility)
**Files:**
- `/docker/AmityVox/web/src/lib/components/common/Modal.svelte`
- All 8+ components using `aria-modal="true"`

**Problem:** None of the modal components implement focus trapping. When `aria-modal="true"` is set, screen readers expect focus to be contained within the dialog. Without focus trapping, keyboard users can Tab out of the modal into background content, which is both a WCAG 2.1 violation (Success Criterion 2.4.3 Focus Order) and a confusing UX for screen reader users.

**Recommended fix:** Add a focus trap utility (either a custom Svelte action or a library like `focus-trap`) to the `Modal.svelte` component. All modals that use it will inherit the fix.

---

## High-Priority Issues

### HIGH-1: 16,132 Lines of Untested API Handler Code

**Severity:** High
**Description:** 14 out of 20 API handler sub-packages have zero test files. The untested packages include security-critical code (moderation, bots, social), complex business logic (activities, polls, widgets, onboarding), and high-traffic paths (voice_handlers, search_handlers).

**Untested packages (by size):**
1. `social/` -- 2,011 lines (friend system, blocking, user profiles)
2. `experimental/` -- 1,769 lines (new features)
3. `voice_handlers.go` -- 1,616 lines (voice/video calling, soundboard)
4. `bots/` -- 1,453 lines (bot API, commands, webhooks)
5. `activities/` -- 1,424 lines (activities, games, status)
6. `widgets/` -- 1,148 lines (whiteboards, kanban, polls)
7. `moderation/` -- 1,996 lines across 3 files (moderation, bans, raid protection)
8. `guildevents/` -- 756 lines (scheduled events)
9. `integrations/` -- 739 lines (third-party integrations)
10. `onboarding/` -- 733 lines (new member flow)
11. `stickers/` -- 670 lines (sticker packs)
12. `polls/` -- 522 lines (poll creation/voting)
13. `search_handlers.go` -- 427 lines (search)
14. `themes/` -- 403 lines (theme management)
15. `bookmarks/` -- 237 lines (message bookmarks)
16. `giphy_handler.go` -- 228 lines (GIF search proxy)

**Recommended fix:** Prioritize test creation for moderation (security), voice (complexity), bots (security), and social (high-traffic) packages.

---

### HIGH-2: Duplicated writeJSON/writeError Functions Across 19 Sub-Packages

**Severity:** High
**Files:** Every sub-package under `internal/api/` defines its own local `writeJSON` and `writeError` functions (38 duplicate function definitions total).

**Problem:** The exported `WriteJSON`/`WriteError` in `server.go` uses struct types (`SuccessResponse`, `ErrorResponse`), while the sub-package duplicates use `map[string]interface{}`. While they produce identical JSON output today, this creates a maintenance risk: if the envelope format changes (e.g., adding a `request_id` field), all 19 copies must be updated independently. This also inflates binary size unnecessarily.

**Pattern found:**
- `server.go:1366` -- Exported `WriteJSON` (uses `SuccessResponse` struct)
- `channels/channels.go:3103` -- Local `writeJSON` (uses `map[string]interface{}`)
- Same pattern in: `admin/admin.go:54`, `experimental/experimental.go:47`, `widgets/widgets.go:1136`, `webhooks/webhooks.go:972`, `users/users.go:1777`, `stickers/stickers.go:658`, `onboarding/onboarding.go:721`, `guildevents/events.go:96`, `bots/bots.go:39`, `integrations/integrations.go:721`, `invites/invites.go:32`, `bookmarks/bookmarks.go:29`, `social/social.go:36`, `themes/themes.go:391`, `guilds/guilds.go:3308`, `polls/polls.go:510`, `activities/activities.go:36`, `moderation/moderation.go:809`

**Recommended fix:** Move to a shared `internal/httputil` package that sub-packages can import, or restructure the package hierarchy so sub-packages can use the exported versions from the parent `api` package.

---

### HIGH-3: Direct json.NewEncoder Bypassing Standard Envelope

**Severity:** High
**Files:**
- `/docker/AmityVox/internal/api/moderation/ban_lists.go:351` -- `json.NewEncoder(w).Encode(export)` bypasses error envelope for ban list export
- `/docker/AmityVox/internal/api/users/users.go:1099` -- `json.NewEncoder(w).Encode(map[string]interface{}{"data": settings})` bypasses local `writeJSON` wrapper
- `/docker/AmityVox/internal/api/users/users.go:1126` -- Same pattern for settings update response

**Problem:** The ban_lists.go export handler writes raw JSON without using the standard `{"data": ...}` envelope. While this is a file download, the Content-Type is still `application/json`, so clients parsing the response programmatically will get a different structure than every other endpoint. The users.go settings handlers manually construct the envelope inline rather than using the local `writeJSON` helper.

**Recommended fix:** For ban_lists export, this is arguably intentional (file download format). Document the exception. For users.go settings handlers, replace the inline `json.NewEncoder` calls with `writeJSON(w, http.StatusOK, settings)`.

---

### HIGH-4: Swallowed Errors in Voice Handlers (Fire-and-Forget)

**Severity:** High
**Files:**
- `/docker/AmityVox/internal/api/voice_handlers.go:222` -- `_ = s.Voice.RemoveParticipant(...)` on disconnect
- `/docker/AmityVox/internal/api/voice_handlers.go:445` -- `_ = s.Voice.RemoveParticipant(...)` on move
- `/docker/AmityVox/internal/api/voice_handlers.go:448` -- `_ = s.Voice.EnsureRoom(...)` on move
- `/docker/AmityVox/internal/api/voice_handlers.go:1009` -- `_ = s.Voice.LogSoundboardPlay(...)` on play
- `/docker/AmityVox/internal/api/voice_handlers.go:1010` -- `_ = s.Voice.IncrementSoundPlayCount(...)` on play

**Problem:** Voice participant removal errors during disconnect/move are silently ignored. If `RemoveParticipant` fails, ghost participants may persist in LiveKit rooms, consuming resources and appearing in participant lists. `EnsureRoom` failure before move means the user gets moved to a non-existent room. Soundboard logging/counting failures lose audit data.

**Recommended fix:** Log errors at minimum. For `RemoveParticipant` and `EnsureRoom`, log errors with `slog.Warn` and consider returning an error response for the move handler.

---

### HIGH-5: Missing ARIA Attributes on Key Interactive Components

**Severity:** High (Accessibility)

**Components missing `role` and `aria-label`:**

| Component | Missing |
|-----------|---------|
| `EmojiPicker.svelte` | No `role="dialog"` or `aria-label` on container |
| `GiphyPicker.svelte` | No `role="dialog"` or `aria-label` on container |
| `StickerPicker.svelte` | No `role="dialog"` or `aria-label` on container |
| `StatusPicker.svelte` | No `role` or `aria-label` on container |
| `MessageInput.svelte` | No `aria-label` on text input area |
| `SearchModal.svelte` | Uses `Modal` wrapper (inherits `role="dialog"`) but no `aria-label` passed to Modal |
| `EditHistoryModal.svelte` | Uses `Modal` wrapper but no `aria-label` passed to Modal |
| `IncomingCallModal.svelte` | Has Escape handling but no `role="dialog"` or `aria-modal="true"` |
| `CreateGuildModal.svelte` | Uses `Modal` wrapper but no `aria-label` passed to Modal |
| `InviteModal.svelte` | Uses `Modal` wrapper but no `aria-label` passed to Modal |
| `ProfileLinkEditor.svelte` | No ARIA attributes |
| `ModerationModals.svelte` | No ARIA attributes |

**Recommended fix:** Add `role="dialog"`, `aria-modal="true"`, and descriptive `aria-label` attributes to picker components. For modals using the `Modal` wrapper, pass a `title` prop (which renders an `<h2>`) or add an `aria-label` prop to the Modal component's dialog div.

---

## Medium-Priority Issues

### MED-1: Hardcoded Color Values in CSS/Styles

**Severity:** Medium
**Description:** 40+ instances of hardcoded hex colors and RGB values found in component styles and inline styles, bypassing the theme system.

**Key offenders:**
- `/docker/AmityVox/web/src/lib/components/channels/Whiteboard.svelte:52,73,82,130,253,301` -- Hardcoded `#ffffff`, `#1a1a2e`, and color palette
- `/docker/AmityVox/web/src/lib/components/guild/SoundboardSettings.svelte:771-829` -- `#ef4444`, `#22c55e` in `<style>` block
- `/docker/AmityVox/web/src/lib/components/voice/VoiceBroadcast.svelte:243-357` -- `#ef4444`, `#dc2626` in `<style>` block
- `/docker/AmityVox/web/src/lib/components/voice/Soundboard.svelte:321` -- `#ef4444` in style
- `/docker/AmityVox/web/src/lib/components/chat/MessageEffects.svelte:48` -- Confetti color palette
- `/docker/AmityVox/web/src/lib/components/guild/RoleEditor.svelte:105,114` -- Default role color `#99aab5`
- `/docker/AmityVox/web/src/lib/components/guild/MembersPanel.svelte:255,286` -- Role color fallback
- `/docker/AmityVox/web/src/lib/components/guild/WelcomeSettings.svelte:33,209` -- Discord blue `#5865F2`

**Note:** Some of these are intentional (role color pickers need a default, confetti needs varied colors, whiteboards need a drawing palette). The `<style>` block usages in SoundboardSettings and VoiceBroadcast are the most problematic as they bypass theme tokens for UI chrome colors.

**Recommended fix:** Replace `<style>` block hardcoded colors with CSS variable references where they represent UI state colors (error/success). Keep intentional user-facing color values (role defaults, drawing palettes) as-is but add comments explaining why.

---

### MED-2: console.error Statements in Production Components

**Severity:** Medium
**Files (non-test, excluding ws.ts gateway internals):**
- `/docker/AmityVox/web/src/lib/stores/voice.ts:181,254` -- Voice join/camera errors
- `/docker/AmityVox/web/src/lib/components/layout/ChannelSidebar.svelte:343,366` -- DM ack/close errors
- `/docker/AmityVox/web/src/lib/components/guild/SoundboardSettings.svelte:85,221` -- Soundboard load/create errors
- `/docker/AmityVox/web/src/lib/components/chat/SearchModal.svelte:46` -- Search failure
- `/docker/AmityVox/web/src/lib/components/chat/ThreadPanel.svelte:82` -- Thread send failure
- `/docker/AmityVox/web/src/lib/components/voice/Soundboard.svelte:54,97` -- Load/play errors
- `/docker/AmityVox/web/src/lib/components/voice/ScreenShareControls.svelte:87` -- Screen share error
- `/docker/AmityVox/web/src/lib/components/voice/VoiceBroadcast.svelte:82,113,140` -- Broadcast errors
- `/docker/AmityVox/web/src/lib/components/voice/CameraSettings.svelte:60` -- Camera settings error
- `/docker/AmityVox/web/src/lib/components/voice/VoiceControls.svelte:85,105,127,153` -- Voice preference errors

**Total:** 25 `console.error` statements, 1 `console.log`, 1 `console.debug` in production code.

**Assessment:** The `console.error` calls in `ws.ts` (5 instances) are appropriate for gateway debugging. Component-level `console.error` calls should ideally route through the toast system to inform users. The `console.log` in `ws.ts:113` (build detection reload) is informational. The `console.debug` in `GiphyPicker.svelte:69` is low-priority.

**Recommended fix:** Keep `console.error` in `ws.ts` (gateway internals). For component error handlers, ensure a toast notification is also shown to the user (some already do both). Remove `console.debug` from GiphyPicker.

---

### MED-3: Untested Non-API Internal Packages

**Severity:** Medium
**Packages with source code but no tests:**
- `notifications/` -- 646 lines (push notifications, preferences)
- `plugins/` -- 741 lines (WASM sandbox, plugin runtime)
- `middleware/` -- 934 lines (CSP, security headers, tracing)
- `scanning/` -- 379 lines (ClamAV virus scanning)

**Recommended fix:** Add tests for middleware (security headers are easily testable with httptest) and scanning (mock ClamAV client).

---

### MED-4: Gateway Presence Publish Error Silently Ignored

**Severity:** Medium
**File:** `/docker/AmityVox/internal/gateway/gateway.go:1234`
```go
_ = s.eventBus.Publish(ctx, events.SubjectPresenceUpdate, events.Event{...})
```

**Problem:** If NATS publish fails for presence updates, users may appear online/offline incorrectly to others with no log trail.

**Recommended fix:** Log the error with `slog.Warn`.

---

### MED-5: Gateway last_online Update Error Ignored

**Severity:** Medium
**File:** `/docker/AmityVox/internal/gateway/gateway.go:281`
```go
_, _ = s.pool.Exec(context.Background(), `UPDATE users SET last_online = now() WHERE id = $1`, client.userID)
```

**Problem:** If this DB update fails, the user's `last_online` timestamp becomes stale. Using `context.Background()` instead of a derived context also means the operation has no timeout.

**Recommended fix:** Use a context with timeout and log errors.

---

### MED-6: Federation JSON Marshal Errors Silently Ignored

**Severity:** Medium
**File:** `/docker/AmityVox/internal/federation/guild.go:728,750`
```go
resp.ChannelsJSON, _ = json.Marshal(channels)
resp.RolesJSON, _ = json.Marshal(roles)
```

**Problem:** While `json.Marshal` on simple maps is very unlikely to fail, ignoring the error means a corrupt or empty response could be sent to federated peers without any diagnostic information.

**Recommended fix:** Log errors if they occur (even if unlikely).

---

### MED-7: Ignored DB Errors in Block/Relationship Checks

**Severity:** Medium
**Files:**
- `/docker/AmityVox/internal/api/users/users.go:387` -- `_ = h.Pool.QueryRow(...).Scan(&blocked)` in friend request handler
- `/docker/AmityVox/internal/api/users/users.go:579` -- `_ = h.Pool.QueryRow(...).Scan(&alreadyBlocked)` in block handler
- `/docker/AmityVox/internal/api/moderation/moderation.go:581` -- `_ = h.Pool.QueryRow(...).Scan(&prevLockdownActive)` in raid config

**Problem:** If the block check query fails, `blocked` defaults to `false`, allowing friend requests to blocked users. If the duplicate-block check fails, `alreadyBlocked` defaults to `false`, potentially causing a DB constraint error downstream instead of a clean 409 response.

**Recommended fix:** Check errors and return 500 on DB failure.

---

## Low-Priority Issues

### LOW-1: console.log for Build Detection Reload

**Severity:** Low
**File:** `/docker/AmityVox/web/src/lib/api/ws.ts:113`
```typescript
console.log('[GW] New build detected, reloading page');
```

**Assessment:** Informational and useful for debugging deployments. Acceptable in production.

---

### LOW-2: console.debug in GiphyPicker

**Severity:** Low
**File:** `/docker/AmityVox/web/src/lib/components/common/GiphyPicker.svelte:69`
```typescript
console.debug('Failed to load Giphy categories:', e);
```

**Assessment:** Debug-level logging. Should be removed or guarded behind `import.meta.env.DEV`.

---

### LOW-3: argIdx Suppression Pattern

**Severity:** Low
**Files:**
- `/docker/AmityVox/internal/api/admin/selfhost.go:1548` -- `_ = argIdx`
- `/docker/AmityVox/internal/api/channels/channels.go:3076` -- `_ = argIdx`
- `/docker/AmityVox/internal/api/guilds/guilds.go:3175` -- `_ = argIdx`

**Problem:** Used to suppress "unused variable" warnings in dynamic SQL builders. The pattern is idiomatic Go but suggests the SQL builder could be refactored.

---

### LOW-4: Positive Finding -- No `log.Print` or `fmt.Print` in Production Code

**Severity:** Informational
**Description:** Zero instances of `log.Print`, `log.Fatal`, `log.Println`, or `fmt.Print` found in non-test Go files under `internal/`. All production logging uses `slog` consistently. The only `fmt.Printf` instances are in `integration_test.go`'s `TestMain` function where `t` is not yet available.

---

### LOW-5: Positive Finding -- No Legacy Svelte Reactive Patterns

**Severity:** Informational
**Description:** Zero instances of `$:` reactive labels or `export let` found in any Svelte component. All 114 components use Svelte 5 runes (`$state`, `$derived`, `$effect`, `$props()`) consistently. 90 components with props use `$props()`, 24 components without props genuinely read from stores directly.

---

### LOW-6: Positive Finding -- Consistent API Status Codes

**Severity:** Informational
**Description:** Status codes are used correctly throughout: `201 Created` for all creation handlers (50+ locations), `204 No Content` for deletions, `200 OK` for reads. Error codes (400, 401, 403, 404, 409, 429, 500) are used appropriately with descriptive error codes.

---

### LOW-7: Positive Finding -- Consistent Naming Conventions

**Severity:** Informational
**Description:** Backend uses `snake_case` for JSON fields consistently. Frontend TypeScript types mirror this convention. No mismatches found between frontend API client methods and backend routes.

---

## Test Coverage Summary

### Backend Test Coverage (Go)

| Package | Source Files | Test Files | Lines (src) | Coverage | Priority |
|---------|-------------|------------|-------------|----------|----------|
| `api/` (root) | 8 | 3 | 4,947 | ~38% by file | Medium |
| `api/activities/` | 1 | 0 | 1,424 | 0% | High |
| `api/admin/` | 3 | 1 | ~3,000 | ~33% by file | Medium |
| `api/bookmarks/` | 1 | 0 | 237 | 0% | Low |
| `api/bots/` | 1 | 0 | 1,453 | 0% | High |
| `api/channels/` | 5 | 2 | ~4,500 | ~40% by file | Medium |
| `api/experimental/` | 1 | 0 | 1,769 | 0% | Medium |
| `api/guildevents/` | 1 | 0 | 756 | 0% | Medium |
| `api/guilds/` | 3 | 1 | ~4,500 | ~33% by file | Medium |
| `api/integrations/` | 1 | 0 | 739 | 0% | Medium |
| `api/invites/` | 1 | 1 | ~350 | 100% by file | Done |
| `api/messages/` | 1 | 0 | 5 (stub) | N/A | N/A |
| `api/moderation/` | 3 | 0 | 1,996 | 0% | **Critical** |
| `api/onboarding/` | 1 | 0 | 733 | 0% | Medium |
| `api/polls/` | 1 | 0 | 522 | 0% | Low |
| `api/social/` | 1 | 0 | 2,011 | 0% | High |
| `api/stickers/` | 1 | 0 | 670 | 0% | Low |
| `api/themes/` | 1 | 0 | 403 | 0% | Low |
| `api/users/` | 7 | 1 | ~2,500 | ~14% by file | High |
| `api/webhooks/` | 1 | 1 | ~1,000 | 100% by file | Done |
| `api/widgets/` | 1 | 0 | 1,148 | 0% | Medium |
| `auth/` | 2 | 1 | -- | ~50% | Medium |
| `automod/` | 3 | 1 | ~1,100 | ~33% | Medium |
| `config/` | 1 | 1 | -- | 100% | Done |
| `database/` | 1 | 1 | -- | 100% | Done |
| `encryption/` | 2 | 1 | -- | ~50% | Medium |
| `events/` | 1 | 1 | -- | 100% | Done |
| `federation/` | 5 | 4 | -- | ~80% | Low |
| `gateway/` | 1 | 1 | -- | 100% | Done |
| `media/` | 1 | 1 | -- | 100% | Done |
| `middleware/` | 3 | 0 | 934 | 0% | Medium |
| `models/` | 2 | 2 | -- | 100% | Done |
| `notifications/` | 1 | 0 | 646 | 0% | Medium |
| `permissions/` | 1 | 1 | -- | 100% | Done |
| `plugins/` | 2 | 0 | 741 | 0% | Medium |
| `presence/` | 1 | 1 | -- | 100% | Done |
| `scanning/` | 1 | 0 | 379 | 0% | Low |
| `search/` | 1 | 1 | -- | 100% | Done |
| `voice/` | 1 | 1 | -- | 100% | Done |
| `workers/` | 7 | 2 | -- | ~29% | Medium |

**Summary:** 80 source files, 31 test files. ~39% file-level coverage. 16,132 lines of API handler code have zero test coverage.

### Frontend Test Coverage

| Directory | Source Files | Test Files | Coverage % | Priority |
|-----------|-------------|------------|------------|----------|
| `components/admin/` | 6 | 0 | 0% | Low |
| `components/channels/` | 11 | 3 | 27% | Medium |
| `components/chat/` | 17 | 2 | 12% | High |
| `components/common/` | 25 | 7 | 28% | Medium |
| `components/encryption/` | 1 | 1 | 100% | Done |
| `components/gallery/` | 6 | 2 | 33% | Low |
| `components/guild/` | 16 | 3 | 19% | Medium |
| `components/home/` | 4 | 0 | 0% | Low |
| `components/layout/` | 7 | 0 | 0% | High |
| `components/voice/` | 9 | 1 | 11% | Medium |
| `components/` (root) | 1 | 0 | 0% | Low |
| `stores/` | 27 | 12 | 44% | Medium |
| `utils/` | ~10 | 5 | ~50% | Low |
| `encryption/` | 1 | 1 | 100% | Done |

**Summary:** 114 components + 27 stores + ~10 utils = ~151 source modules. 33 test files. ~22% file-level coverage.

**Untested stores (15):**
`announcements`, `auth`, `callRing`, `gateway`, `gateway.reconnect`, `instances`, `layout`, `members`, `moderation`, `navigation`, `nicknames`, `notifications`, `settings`, `typing`, `voice`

**Untested critical components:**
- `MessageInput.svelte` (primary user interaction point)
- `MessageList.svelte` (message rendering)
- `MessageItem.svelte` (individual messages)
- `ChannelSidebar.svelte` (navigation)
- `MemberList.svelte` (member display)
- `TopBar.svelte` (header navigation)
- `VoiceChannelView.svelte` (voice UI)
- `VoiceControls.svelte` (voice controls)

---

## Accessibility Audit

| Component | ARIA Labels | Keyboard Nav | Focus Trap | Status |
|-----------|------------|--------------|------------|--------|
| `Modal.svelte` | `role="dialog"`, `aria-modal="true"` | Escape to close | **MISSING** | Needs focus trap |
| `ContextMenu.svelte` | `role="menu"`, `tabindex="-1"` | Escape to close | N/A (overlay) | OK |
| `ContextMenuItem.svelte` | `role="menuitem"` | Inherited | N/A | OK |
| `CommandPalette.svelte` | `role="dialog"`, `aria-modal`, `aria-label` | Escape, Up/Down arrows | **MISSING** | Needs focus trap |
| `QuickSwitcher.svelte` | `role="dialog"`, `aria-modal`, `aria-label` | Escape, Up/Down arrows | **MISSING** | Needs focus trap |
| `EmojiPicker.svelte` | **MISSING** (no role, no aria-label) | Escape to close | N/A | **Needs ARIA** |
| `GiphyPicker.svelte` | **MISSING** (no role, no aria-label) | Escape to close | N/A | **Needs ARIA** |
| `StickerPicker.svelte` | **MISSING** (no role, no aria-label) | Escape to close | N/A | **Needs ARIA** |
| `StatusPicker.svelte` | **MISSING** (no role, no aria-label) | Escape to close | N/A | **Needs ARIA** |
| `ImageLightbox.svelte` | `role="dialog"`, `aria-modal` | Escape to close | **MISSING** | Needs focus trap |
| `ProfileModal.svelte` | `role="dialog"`, `aria-modal` | Escape to close | **MISSING** | Needs focus trap |
| `NotificationCenter.svelte` | `role="dialog"`, `aria-modal`, `aria-label` | Escape to close | **MISSING** | Needs focus trap |
| `IncomingCallModal.svelte` | **MISSING** (no role, no aria-modal) | Escape to decline | **MISSING** | **Needs ARIA + focus trap** |
| `OnboardingModal.svelte` | `role="dialog"`, `aria-modal` | N/A | **MISSING** | Needs focus trap |
| `SearchModal.svelte` | Inherits from Modal | Escape via Modal | **MISSING** (inherited) | Needs focus trap (via Modal) |
| `EditHistoryModal.svelte` | Inherits from Modal | Escape via Modal | **MISSING** (inherited) | Needs focus trap (via Modal) |
| `CreateGuildModal.svelte` | Inherits from Modal | Escape via Modal | **MISSING** (inherited) | Needs focus trap (via Modal) |
| `InviteModal.svelte` | Inherits from Modal | Escape via Modal | **MISSING** (inherited) | Needs focus trap (via Modal) |
| `ModerationModals.svelte` | **MISSING** | N/A | **MISSING** | **Needs ARIA** |
| `MessageInput.svelte` | **MISSING** (no aria-label) | Escape cancels edit | N/A | **Needs ARIA** |
| `ToastContainer.svelte` | `role="alert"`, dismiss `aria-label` | N/A | N/A | OK |
| `ConnectionIndicator.svelte` | `role="status"`, `aria-label` | N/A | N/A | OK |
| `KanbanBoard.svelte` | `role="dialog"`, `role="region"`, `aria-label` | Escape, Enter | N/A | OK |
| `AnnouncementBanner.svelte` | `aria-label` on dismiss | N/A | N/A | OK |
| `RetentionSettings.svelte` | `role="radiogroup"`, `aria-labelledby` | N/A | N/A | OK |

**Summary:** 7 components have no ARIA attributes at all. 9 modal-like components lack focus trapping. Adding focus trapping to `Modal.svelte` would fix 10+ downstream components.

---

## Code Consistency Summary

### API Response Format
All API handlers consistently use the `{"data": ...}` / `{"error": {"code": "...", "message": "..."}}` envelope pattern. The 19 local `writeJSON`/`writeError` duplicates produce the same JSON structure as the exported versions. One documented exception: ban list export endpoint.

### HTTP Status Codes
Status codes are used correctly throughout: `201 Created` for creation, `204 No Content` for deletion, `200 OK` for reads. Error codes used appropriately.

### Naming Conventions
Backend `snake_case` JSON fields consistently match frontend TypeScript types. No mismatches found.

### Svelte 5 Runes
100% adoption. Zero legacy patterns. All 114 components use modern Svelte 5 APIs.

### Go Logging
100% `slog` adoption in production code. Zero `log.Print` or `fmt.Print` statements.

---

## Recommendations Summary (Priority Order)

1. **Critical:** Fix swallowed DB errors in permission checks (`guilds.go:812,897,2342,2349`)
2. **Critical:** Add focus trapping to `Modal.svelte` (fixes 10+ modal components)
3. **High:** Add ARIA attributes to picker components (EmojiPicker, GiphyPicker, StickerPicker, StatusPicker)
4. **High:** Add `role="dialog"` and `aria-modal="true"` to IncomingCallModal and ModerationModals
5. **High:** Log errors in voice handler fire-and-forget operations
6. **High:** Begin test creation starting with: moderation, voice_handlers, bots, social
7. **High:** Consolidate writeJSON/writeError into a shared package
8. **Medium:** Fix ignored DB errors in block/relationship checks (`users.go:387,579`)
9. **Medium:** Replace hardcoded colors in `<style>` blocks with CSS variables
10. **Medium:** Add tests for middleware (CSP/security headers) and notifications packages
11. **Medium:** Add tests for MessageInput, MessageList, ChannelSidebar components
12. **Low:** Remove console.debug from GiphyPicker
13. **Low:** Log gateway presence publish errors
