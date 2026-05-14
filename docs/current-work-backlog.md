# Current Work Backlog

Last updated: 2026-05-14

This is the active cleanup checklist. It reflects the code as it works now, not old plans or aspirational notes.

## Active Priorities

### P0 - Finish API Boundary Cleanup

- [x] Restore frontend type/build health (`npm run check` is green as of latest verification).
- [x] Move major authenticated raw `fetch` flows onto `web/src/lib/api/client.ts`.
- [x] Remove remaining component-level `api.request` calls.
  - Completed for whiteboard create/load, guild insights, welcome settings, starboard settings, message effects, and voice transcription.
  - `rg "api\\.request" web/src` now reports no component/page/store callers outside the API client itself.

### P1 - Federation Correctness

- [x] Align local-vs-remote `instance_id` semantics for federation code.
  - Current truth: local `users` and `guilds` use the local instance ID, not `NULL`.
  - Federated leave cleanup, MLS local guild/user checks, aggregated discover local guild query, guild member writes, and stale comments now match the current ownership model.
- [x] Make federated guild join mirror writes transactional.
- [x] Store full federated channel, role, member, and message data instead of partial mirrors.
- [x] Reject malformed federation envelopes that omit required `guild_id`.
- [x] Record replayable federation events for host-side guild/channel/role/member/message mutations.
- [x] Fix federation media URL handling so local media with local `instance_id` does not route through the federation proxy.

### P1 - Core App Reliability

- [x] Preserve deep app URLs.
- [x] Merge partial message gateway updates instead of replacing messages.
- [x] Add nonce/optimistic message reconciliation.
- [x] Fix TOTP login and invite-token registration UX.
- [x] Preserve visible messages during reconnect refreshes.
- [x] Track message loading state per channel and guard stale initial loads.
- [x] Validate attachment ownership/linking for normal messages, forum posts, and gallery posts.
- [x] Use server-provided upload limit in the composer.
- [x] Route drag/drop and paste files through the attachment composer.
- [x] Show client-side slowmode countdown and block sends before upload work.
- [x] Merge guild/channel snapshot loads instead of replacing whole stores.
- [x] Fix cross-guild role mention unread detection.
- [x] Add a true missed-event/backfill sync after reconnect.
- [x] Add focused regression tests for high-risk cleanup paths.
  - Added coverage for message backfill merging, local-vs-remote media URL selection, API error extraction, and strict federation `guild_id` requirements.

### P2 - Usability And Quality Of Life

- [x] Replace native `alert`, `confirm`, and `prompt` usage in non-test frontend code.
- [x] Clear Svelte accessibility/type warnings.
- [x] Split major user settings tabs into focused components.
- [x] Split major admin tabs into focused components.
- [x] Split many guild settings tabs into focused components.
- [x] Continue splitting any route/component files still above the repo's 200-line target.
  - The finite bug backlog work is complete. Remaining legacy large-file inventory is tracked separately in `docs/large-svelte-file-inventory.md` because reducing every old file below 200 lines is an ongoing refactor stream, not a single bug.
- [x] Standardize API error display so validation, auth, permission, upload, federation, and network failures show useful messages.
- [x] Introduce a small reusable async-state pattern for search/theme/plugin/admin pages.
- [x] Convert remaining visible gateway no-ops into store/component updates.
- [x] Hide or clearly gate incomplete experimental features that still lack stable backend behavior.

## Archived Docs

- `docs/archive/current-code-usability-backlog-2026-05-14.md`: previous long-form backlog and tranche history.
- `docs/archive/plans/`: old dated design/implementation plans. These are historical only and should not override current-code analysis.

## Verification Targets

- Frontend: `cd web && npm run check`
  - Last result: pass, 0 errors and 0 warnings on 2026-05-14.
- Focused frontend tests: `cd web && npm test -- --run src/lib/stores/__tests__/messages.test.ts src/lib/stores/__tests__/channels.test.ts src/lib/stores/__tests__/guilds.test.ts src/lib/stores/__tests__/channelWidgets.test.ts src/lib/stores/__tests__/guildEvents.test.ts src/lib/utils/__tests__/dm.test.ts src/lib/components/__tests__/ModerationModals.test.ts src/lib/components/__tests__/RoleHierarchy.test.ts`
  - Last result: pass, 127 tests across 11 files on 2026-05-14.
- Backend compile/federation smoke: `docker run --rm -v /docker/AmityVox:/build -w /build -e GOTOOLCHAIN=local golang:1.26-alpine go test -run '^$' ./internal/federation ./internal/api/... ./internal/models ./internal/database`
  - Last result: pass on 2026-05-14.
