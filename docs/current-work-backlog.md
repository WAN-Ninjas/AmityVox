# Current Work Backlog

Last updated: 2026-05-22

This is the active cleanup checklist. It reflects the code as it works now, not old plans or aspirational notes.

## Completed Cleanup Baseline

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
- [x] Store full federated channel, role, member, DM, and message data instead of partial mirrors.
  - Guild join mirrors now store richer channel/role/member data.
  - Federated DM create/message/update/delete/reaction mirrors now carry stable ownership and route through dedicated DM endpoints.
  - Invite/manage member paths, channel-create replay, and remote guild post messages now write explicit ownership.
- [x] Reject malformed federation envelopes that omit required `guild_id`.
- [x] Record replayable federation events for host-side guild/channel/role/member/message mutations.
  - Host-side events are now recorded, and replay IDs now use deterministic compacted-payload hashes.
  - DB-backed sync coverage now verifies authorized replay ordering and duplicate canonical event IDs.
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
  - Reconnect now reconciles each loaded channel against the latest server message window, pages forward from the latest visible message, and refreshes active guild/channel, member-list, read-state, DM, notification, and permission snapshots.
  - Member add/remove gateway events now update the active guild roster.
  - Loaded channels now also page backward through visible history so older visible edits/deletes outside the latest page are refreshed after reconnect.
- [x] Add focused regression tests for high-risk cleanup paths.
  - Added coverage for message backfill merging, local-vs-remote media URL selection, API error extraction, and strict federation `guild_id` requirements.

### P2 - Usability And Quality Of Life

- [x] Replace native `alert`, `confirm`, and `prompt` usage in non-test frontend code.
- [x] Clear Svelte accessibility/type warnings.
- [x] Split major user settings tabs into focused components.
- [x] Split major admin tabs into focused components.
- [x] Split many guild settings tabs into focused components.
- [ ] Continue splitting any route/component files still above the repo's 200-line target.
  - The finite bug backlog work is complete. Remaining legacy large-file inventory is tracked separately in `docs/large-svelte-file-inventory.md` because reducing every old file below 200 lines is an ongoing refactor stream, not a single bug.
- [x] Standardize API error display so validation, auth, permission, upload, federation, and network failures show useful messages.
  - Shared formatter now covers main routes, setup/registration/invites, friends, discovery, bookmarks, user settings, admin surfaces, guild/settings surfaces, layout sidebars/groups, chat actions, channel/gallery tools, voice controls, member-list action, and common guild management flows.
  - Remaining grep hits are registration/admin settings copy fields, not exception handling.
- [x] Introduce a small reusable async-state pattern for search/theme/plugin/admin pages.
	- Helper exists; plugin, search modal, theme gallery, discover, bookmarks, admin dashboard, admin federation, bridge admin, admin utility tabs, admin guild/registration/captcha/rate-limit/content-safety/domain/retention panels, profile/security/privacy/notification/voice/bot settings, auth/setup/invite routes, moderation route panes, and guild boost/insights/invite/widget/webhook/role/moderation/raid/audit/category/invite/ban/ban-list/template/retention/plugin/emoji/sticker/automod/onboarding/member panels use it.
	- Older chat/gallery/common/gallery loaders and submit states now use the shared helper; remaining large-file work is structural extraction rather than scattered local async flags.
- [x] Convert remaining visible gateway no-ops into store/component updates.
  - Core message/guild/member/widget/event paths improved.
  - Soundboard, broadcast, screen share, location, and activity/game gateway events now have store/component ownership.
  - Bot component interactions are intentionally backend/bot-worker events; visible changes are expected through `MESSAGE_UPDATE`.
- [x] Hide or clearly gate incomplete experimental features that still lack stable backend behavior.

## Reopened Feature-Completion Backlog

1. [x] Complete federated DM parity.
   - Done: set `channels.instance_id` for remote-created DM/group mirrors.
   - Done: persist federated DM messages with `messages.instance_id`, rich fields, attachments, and embeds.
   - Done: route local DM `MESSAGE_CREATE` events through the dedicated DM federation endpoint instead of the guild inbox.
   - Done: federate DM message edits, deletes, reaction adds, and reaction removals through signed dedicated DM endpoints.
2. [x] Make federation event replay idempotent.
   - Done: stored federation events now use a deterministic ID derived from instance, event type, guild/channel, HLC, and payload.
   - Done: anonymous federated embeds now get deterministic IDs during replay.
   - Done: event ID hashing now compacts JSON payloads so whitespace-only replay differences do not duplicate rows.
   - Done: event ID hashing now canonicalizes JSON object key order so semantically identical replay payloads dedupe.
   - Done: added DB-backed sync backfill coverage for authorized peer replay ordering and duplicate canonical event IDs.
3. [x] Close remaining federation `instance_id` write gaps.
   - Done: remote guild post messages, channel-create replay, invite accept membership, manage-created channels/roles, and manage member joins now write explicit ownership.
4. [x] Replace frontend reconnect message-only fetch with real missed-event reconciliation.
   - Done: loaded channels now reconcile latest server windows for edits, deletes, reactions, pins, and missed creates beyond one page.
   - Done: loaded channels now page backward through visible history so older visible edits/deletes outside the latest page are refreshed after reconnect.
   - Done: reconnect refreshes active guild channels, members, and permissions in addition to guilds, DMs, read state, channel-guild map, and notifications.
5. [x] Finish API error standardization across high-traffic routes and settings panes.
   - Done: login, setup, registration, invite acceptance, friends, discovery, bookmarks, user settings, admin dashboard/tabs, plugin install, member-list action, guild overview, invite, ban, emoji, role-delete, guild settings, layout sidebars/groups, message actions, video recorder, instance switcher, common profile/group/status/GIF/sticker, gallery/channel tools, embeds, bump, and voice-control flows use the shared API error formatter.
   - Remaining grep hits are registration/admin settings copy fields, not exception handling.
6. [x] Finish async-state consolidation for search, theme, discover, and admin surfaces.
	- Done: search modal, theme gallery, discover including federation peers, bookmarks, admin dashboard, admin federation, bridge admin, admin utility tabs, admin guild/registration/captcha/rate-limit/content-safety/domain/retention panels, profile/security/privacy/notification/voice/bot settings, auth/setup/invite routes, moderation route panes, and the remaining lower-traffic guild settings panes use the shared async helper.
	- Done: older forum/gallery/channel panels, pins, edit history, cross-channel quotes, translation, stickers, GIFs, profile popovers/modals/links, media gallery/admin/tag tools, location sharing, video recording, and composer upload/passphrase states now use the shared helper where the state represents an async operation.
7. [x] Assign or implement remaining gateway event no-op owners.
   - Done: active member roster, soundboard playback, voice broadcasts, screen-share badges, location shares, and activity/game invalidation now have explicit gateway ownership.
   - Done: bot component interaction events are documented as backend/bot-worker events; client-visible changes flow through message updates.
8. [ ] Continue large Svelte component reduction using `docs/large-svelte-file-inventory.md`.
   - In progress: `ChannelSidebar.svelte` reduced from 1697 to 739 lines by extracting sidebar modals, context menus, DM/events/voice/header sections, text channel rows, and forum/gallery channel rows. Extracted components are below the 200-line target.
   - In progress: `ChannelGroups.svelte` reduced from 905 to 850 lines by extracting the channel group create modal.
   - In progress: `MessageInput.svelte` reduced from 1170 to 976 lines by extracting pending-file preview, encrypted passphrase prompt, status bars, and schedule picker. Extracted components are below the 200-line target.
   - In progress: `MessageItem.svelte` reduced from 1172 to 989 lines by extracting attachments, embeds/reactions, and the message context menu. Extracted components are below the 200-line target.
9. [x] Add multi-instance federation integration tests for guild join, DM, media, and backfill behavior.
   - Done: DB-backed federation sync backfill test covers authorized peer replay ordering and duplicate canonical event IDs.
   - Done: signed inbound guild join coverage verifies remote member ownership, channel-peer creation, and duplicate join idempotency.
   - Done: signed inbound DM coverage verifies mirror creation, recipient rows, duplicate create/message idempotency, and remote attachment ownership.
   - Done: signed inbound guild message coverage verifies remote attachment persistence, response metadata, media `instance_id`, and nonce idempotency.
   - Done: remote guild message proxy now forwards attachment metadata after validating local upload ownership.
10. [x] Update stale federation/codebase docs after each completed tranche.
	- Done: current backlog and large Svelte inventory reflect this tranche's federation tests and component reductions.
	- Done: guild message attachment federation and dependency audit cleanup are reflected here.
	- Done: async-state follow-up wording now reflects the older chat/gallery/common cleanup.

## Archived Docs

- `docs/archive/current-code-usability-backlog-2026-05-14.md`: previous long-form backlog and tranche history.
- `docs/archive/plans/`: old dated design/implementation plans. These are historical only and should not override current-code analysis.

## Verification Targets

- Frontend: `cd web && npm run check`
  - Last result: pass, 0 errors and 0 warnings on 2026-05-22.
- Focused frontend tests: `cd web && npm test -- --run src/lib/stores/__tests__/messages.test.ts src/lib/stores/__tests__/channels.test.ts src/lib/stores/__tests__/guilds.test.ts src/lib/stores/__tests__/channelWidgets.test.ts src/lib/stores/__tests__/guildEvents.test.ts src/lib/stores/__tests__/presence.test.ts src/lib/stores/__tests__/activityEvents.test.ts src/lib/stores/__tests__/voiceBroadcasts.test.ts src/lib/stores/__tests__/locationShares.test.ts src/lib/utils/__tests__/dm.test.ts src/lib/components/__tests__/ModerationModals.test.ts src/lib/components/__tests__/RoleHierarchy.test.ts src/lib/components/__tests__/MembersPanel.test.ts src/lib/components/__tests__/StatusPicker.test.ts src/lib/components/__tests__/RoleEditor.test.ts`
  - Last result: pass, 174 tests across 15 files on 2026-05-22.
- Backend compile/federation smoke: `docker run --rm -v /docker/AmityVox:/build -w /build -e GOTOOLCHAIN=local golang:1.26-alpine go test -run '^$' ./internal/federation ./internal/api/... ./internal/models ./internal/database ./internal/integration`
  - Last result: pass on 2026-05-22.
- Frontend dependency audit: `cd web && npm audit --json`
  - Last result: pass, 0 vulnerabilities on 2026-05-22.
