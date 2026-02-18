# Frontend Completeness Audit - Phase 2B

**Date:** 2026-02-18
**Auditor:** Claude Opus 4.6 (automated)
**Scope:** All Svelte/TS files under `web/src/`

---

## TODO/FIXME Markers Found

**Result: NONE FOUND**

A comprehensive grep for `TODO` and `FIXME` across all `.svelte` and `.ts` files in `web/src/` returned zero matches. This is a positive indicator that no deferred work items remain in the frontend codebase.

---

## Stub/Placeholder Components

### 1. StageChannelView - Placeholder Speaker/Audience Lists

- **File:** `/docker/AmityVox/web/src/lib/components/channels/StageChannelView.svelte`
- **Line:** 17-20
- **Description:** The component contains a code comment stating: `"Placeholder speaker/audience lists. In production these would be populated via WebSocket presence events from the voice server."` The `speakers` and `audience` arrays are initialized empty and never populated from any WebSocket event source. After a user joins the stage (via `api.joinVoice()`), the speaker/audience lists remain empty because no WebSocket handler populates them. The component renders correctly and the join/leave functionality calls real API endpoints, but the real-time participant lists are static.
- **Severity:** **MEDIUM** - The component is functional for join/leave but the core value proposition (seeing who is speaking/listening) does not work.

### 2. ActivityFrame - Built-in Activity Placeholder

- **File:** `/docker/AmityVox/web/src/lib/components/channels/ActivityFrame.svelte`
- **Line:** 266-275
- **Description:** When an activity URL starts with `builtin://`, the component shows a static placeholder with the text "Built-in activity running" instead of rendering any actual built-in activity. This is by design (built-in activities would need dedicated implementations), but it means `builtin://` activities are non-functional from the UI perspective.
- **Severity:** **LOW** - External URL activities work correctly via iframe. Built-in activities are an extension point, not a broken feature.

---

## Broken UI Flows

### 1. Whiteboard - Silent Save Failure Suppressed

- **File:** `/docker/AmityVox/web/src/lib/components/channels/Whiteboard.svelte`
- **Line:** 320-328
- **Description:** The `saveState()` function catches all errors silently (`catch { // Silent save. }`). If the backend save consistently fails (e.g. network issue, auth expiry), the user draws without any feedback that their work is not persisting. This violates the project's error handling rule ("Never silently swallow errors").
- **Severity:** **MEDIUM** - Data loss risk if save fails silently.

### 2. LocationShare - Silent Live Update Failure

- **File:** `/docker/AmityVox/web/src/lib/components/chat/LocationShare.svelte`
- **Line:** 141-143
- **Description:** The live location update interval catches and silently suppresses all errors. If the API endpoint is unreachable, the user's live location will stop updating without any notification.
- **Severity:** **LOW** - Cosmetic degradation; the next interval retries.

### 3. LocationShare - Silent Cleanup Failure

- **File:** `/docker/AmityVox/web/src/lib/components/chat/LocationShare.svelte`
- **Line:** 155-157
- **Description:** The `stopLiveSharing()` function ignores errors on DELETE cleanup. A zombie live location could persist server-side.
- **Severity:** **LOW** - Edge case, cleanup is best-effort.

---

## Dead Imports

**Result: NO DEAD IMPORTS FOUND**

All import statements from `$lib/`, `$components/`, and `$app/` paths were verified to resolve to existing files. Specifically checked:
- `$components/` alias resolves via `svelte.config.js` line 17 to `src/lib/components`
- `$lib/stores/gateway.reconnect.ts` exists (imported by `ConnectionIndicator.svelte`)
- `$lib/utils/handleUtils.ts` exists (imported by `HandleUtils.test.ts`)
- `$lib/utils/gifFavorites.ts` exists (imported by `GiphyPicker.svelte` and test files)
- All API client methods referenced by experimental components (`createKanbanBoard`, `getActiveSession`, `joinVoice`, etc.) exist in `client.ts`
- All backend routes referenced by the API client are registered in `internal/api/server.go`

The `$components` alias is configured at:
- **File:** `/docker/AmityVox/web/svelte.config.js`, line 17

---

## Console Statements to Remove

### Legitimate (should keep)

These are error-handling `console.error` calls in catch blocks that provide debugging context. They are appropriate for production since they only fire on actual errors.

| File | Line | Statement | Verdict |
|------|------|-----------|---------|
| `web/src/lib/api/ws.ts` | 45 | `console.error('[GW] Handler error:', e)` | **KEEP** - Gateway error handling |
| `web/src/lib/api/ws.ts` | 71 | `console.error('[GW] Failed to parse message:', e)` | **KEEP** - Protocol error |
| `web/src/lib/api/ws.ts` | 81 | `console.error('[GW] Auth rejected, not reconnecting')` | **KEEP** - Auth failure |
| `web/src/lib/api/ws.ts` | 204 | `console.error('[GW] Max reconnect attempts reached')` | **KEEP** - Critical failure |
| `web/src/lib/stores/voice.ts` | 181, 254 | `console.error('[Voice] Failed to...')` | **KEEP** - Voice failures |

### Should Review for Removal

| File | Line | Statement | Severity |
|------|------|-----------|----------|
| `web/src/lib/api/ws.ts` | 113 | `console.log('[GW] New build detected, reloading page')` | **LOW** - Informational log, consider removing for cleaner production output |
| `web/src/lib/components/common/GiphyPicker.svelte` | 69 | `console.debug('Failed to load Giphy categories:', e)` | **LOW** - Debug-level log, fine for production (filtered by default in most browsers) |
| `web/src/routes/app/friends/+page.svelte` | 61 | `.catch((e) => console.error('Failed to load DMs:', e))` | **LOW** - Acceptable error logging |
| `web/src/lib/components/layout/ChannelSidebar.svelte` | 343 | `console.error('Failed to mark DM as read:', err)` | **LOW** - Acceptable |
| `web/src/lib/components/layout/ChannelSidebar.svelte` | 366 | `console.error('Failed to close DM:', err)` | **LOW** - Acceptable |
| `web/src/lib/components/guild/SoundboardSettings.svelte` | 85, 221 | `console.error('Soundboard ... error:', err)` | **LOW** - Acceptable |
| `web/src/lib/components/chat/SearchModal.svelte` | 46 | `console.error('Search failed:', err)` | **LOW** - Acceptable |
| `web/src/lib/components/chat/ThreadPanel.svelte` | 82 | `console.error('Failed to send thread message:', err)` | **LOW** - Acceptable |
| `web/src/lib/components/voice/Soundboard.svelte` | 54, 97 | `console.error('Soundboard ... error:', err)` | **LOW** - Acceptable |
| `web/src/lib/components/voice/ScreenShareControls.svelte` | 87 | `console.error('Screen share error:', err)` | **LOW** - Acceptable |
| `web/src/lib/components/voice/VoiceBroadcast.svelte` | 82, 113, 140 | `console.error('... error:', err)` | **LOW** - Acceptable |
| `web/src/lib/components/voice/CameraSettings.svelte` | 60 | `console.error('[Camera] Settings error:', err)` | **LOW** - Acceptable |
| `web/src/lib/components/voice/VoiceControls.svelte` | 85, 105, 127, 153 | `console.error('Failed to ...')` | **LOW** - Acceptable |

**Summary:** 1 `console.log`, 1 `console.debug`, and ~20 `console.error` statements. All `console.error` calls are in catch blocks and serve as legitimate error reporting. The `console.log` on line 113 of `ws.ts` is the only informational log that could be removed or guarded behind a debug flag.

---

## Incomplete Features

### 1. StageChannelView - No Real-Time Participant Display

- **File:** `/docker/AmityVox/web/src/lib/components/channels/StageChannelView.svelte`
- **Lines:** 17-20, 124-168
- **Description:** The stage channel UI can join/leave via API, but the speaker and audience lists are never populated from WebSocket events. After joining, the stage shows "No one is speaking yet" and "No audience members" permanently, regardless of how many people are actually connected. The voice token is obtained but there is no LiveKit Room connection or track subscription to actually receive/transmit audio, nor any WebSocket event handler that updates the participant lists.
- **Severity:** **HIGH** - Core feature visually present but functionally incomplete. Users can "join" a stage but cannot see or hear anyone.

### 2. Whiteboard - No Real-Time Collaboration

- **File:** `/docker/AmityVox/web/src/lib/components/channels/Whiteboard.svelte`
- **Description:** The whiteboard supports local drawing and persists state to the backend via PATCH requests, but there is no WebSocket subscription for real-time collaboration. If two users open the same whiteboard, they will not see each other's drawings until one reloads. The collaborator bar at the bottom references `whiteboard.collaborators` but this is only populated from the initial GET load, not updated in real-time.
- **Severity:** **MEDIUM** - Single-user drawing works. Multi-user collaboration (the stated purpose) does not work in real-time.

### 3. KanbanBoard - No Real-Time Updates

- **File:** `/docker/AmityVox/web/src/lib/components/channels/KanbanBoard.svelte`
- **Description:** Board operations (create card, move card, delete card, add column) all work via API calls and reload the board data. However, if another user modifies the board, the current user will not see changes until they perform their own action (which triggers `loadBoard()`). No WebSocket subscription for board change events.
- **Severity:** **LOW** - Functional for single-user workflows. Multi-user concurrent editing without real-time sync is a limitation, not a bug.

### 4. VideoRecorder - formattedTime is a Derived Function, Not Value

- **File:** `/docker/AmityVox/web/src/lib/components/chat/VideoRecorder.svelte`
- **Line:** 30-34
- **Description:** `formattedTime` is defined as `$derived(() => { ... })` which returns a function, not a string. It is then called as `{formattedTime()}` in the template (lines 308, 352). This works at runtime (the template invokes the function) but is inconsistent with Svelte 5 conventions where `$derived` should produce a value, not a function. The correct pattern would be `$derived((() => { ... })())` or moving the logic to a function called without `$derived`.
- **Severity:** **LOW** - Functions correctly but is an anti-pattern. In Svelte 5, `$derived(() => expr)` stores the arrow function itself, then the template calls it. This works but is confusing and may break with future Svelte optimizations.

### 5. Experimental Components - No WebSocket Integration

- **Files:** Multiple experimental components
- **Description:** The following experimental features all save/load state via REST API but have no WebSocket event handlers for real-time updates:
  - Whiteboard (`Whiteboard.svelte`) - collaborator cursors, drawing sync
  - KanbanBoard (`KanbanBoard.svelte`) - card moves, column changes
  - LocationShare (`LocationShare.svelte`) - live location pings from other users
  - ActivityFrame (`ActivityFrame.svelte`) - participant join/leave events
  - CodeSnippet (`CodeSnippet.svelte`) - N/A (view-only after creation, no real-time needed)
  - VideoRecorder (`VideoRecorder.svelte`) - N/A (local recording, no real-time needed)
- **Severity:** **MEDIUM** (aggregate) - These components are functional in single-user scenarios but lack the real-time collaborative aspect implied by their design.

---

## Summary Statistics

| Category | Critical | High | Medium | Low |
|----------|----------|------|--------|-----|
| TODO/FIXME markers | 0 | 0 | 0 | 0 |
| Stub/placeholder components | 0 | 0 | 1 | 1 |
| Broken UI flows | 0 | 0 | 1 | 2 |
| Dead imports | 0 | 0 | 0 | 0 |
| Console statements | 0 | 0 | 0 | 1 |
| Incomplete features | 0 | 1 | 2 | 2 |
| **Totals** | **0** | **1** | **4** | **6** |

---

## Key Findings

1. **No TODO/FIXME markers remain** - The codebase is clean of deferred work markers.

2. **No dead imports** - All import statements resolve to existing files. All API client methods referenced by components exist in `client.ts` and have corresponding backend routes in `server.go`.

3. **All modals have submit logic** - Every Modal component was verified to have either API call handlers or form submission logic wired to buttons. No orphaned modals were found.

4. **No empty onclick handlers** - Grep for empty arrow functions (`() => {}`) in onclick attributes returned zero matches. All button handlers reference real functions.

5. **StageChannelView is the most incomplete feature** - It has a fully designed UI but the speaker/audience lists are static placeholders. This is the only HIGH severity finding.

6. **Experimental components lack WebSocket real-time sync** - Whiteboard, KanbanBoard, LocationShare, and ActivityFrame all work via REST API but do not subscribe to WebSocket events for real-time collaboration. This is a design limitation across the experimental feature set.

7. **Console statements are appropriate** - Nearly all are `console.error` in catch blocks. Only one `console.log` (build detection in gateway) could be reconsidered.

---

## Recommendations

1. **StageChannelView (HIGH):** Either wire up WebSocket events to populate speaker/audience lists, or add a prominent "Stage channels are in preview - participant lists coming soon" notice to set user expectations.

2. **Whiteboard silent save (MEDIUM):** Add a visual indicator (toast or inline badge) when save fails, so users know their drawing may not persist.

3. **Experimental real-time sync (MEDIUM):** Consider adding a "Changes may not appear in real-time for other users" notice to Whiteboard and KanbanBoard interfaces, or implement WebSocket subscriptions for state change events.

4. **VideoRecorder $derived pattern (LOW):** Refactor `formattedTime` from `$derived(() => { ... })` to `$derived.by(() => { ... })` or a regular function for Svelte 5 convention compliance.
