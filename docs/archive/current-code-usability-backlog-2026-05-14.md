# Current-Code Usability And Quality Backlog

Scope: current code only. This ignores prior notes and treats implemented behavior as truth. Focus is day-to-day app usability, reliability, and quality of life outside the federation-specific audit.

## P0 - App-Breaking / Data-Loss Risk

1. Deep app URLs are rewritten to `/app`.
   - Code: `web/src/routes/app/+layout.svelte:54-58`.
   - Current behavior: after any navigation under `/app/...`, the browser URL is replaced with `/app`.
   - User impact: refresh/back/copy-link cannot preserve the current guild, channel, DM, settings tab, moderation page, etc. This makes the app feel broken and destroys shareable deep links.
   - Fix: remove the blanket `replaceState`; if canonical URL hiding is desired, implement it as an explicit setting, not default routing behavior.

2. Partial realtime message updates replace entire messages.
   - Code: `web/src/lib/stores/messages.ts:46-53`, `web/src/lib/stores/gateway.ts:421-438`.
   - Current behavior: `updateMessage` replaces the full message object. Reaction/embed gateway events construct partial `Message` objects with only ids and `reactions`/`embeds`.
   - User impact: when an embed or reaction update arrives, the visible message can lose content, author, attachments, timestamps, reply metadata, etc.
   - Fix: make message updates merge patches into existing messages, or add a separate `patchMessage(channelId, messageId, patch)` path.

3. Sent messages are appended twice in common flows.
   - Code: `web/src/lib/components/chat/MessageInput.svelte`, `web/src/routes/app/guilds/[guildId]/channels/[channelId]/+page.svelte`, `web/src/routes/app/dms/[channelId]/+page.svelte`, plus `web/src/lib/stores/gateway.ts:161-164`.
   - Current behavior: the sender appends the REST response immediately; the gateway later delivers `MESSAGE_CREATE` and appends again unless IDs match. This is mostly deduped by id, but there is no nonce-based optimistic reconciliation despite the comment.
   - User impact: races can cause duplicate flicker, inconsistent ordering, or stale optimistic state. Attachment/drop-send paths are especially exposed.
   - Fix: introduce explicit optimistic messages keyed by nonce and reconcile on gateway/REST response. Otherwise rely on REST only for sender and ignore self gateway creates.

4. Frontend build health was failing.
   - Command: `npm run check`.
   - Original result: 127 errors and 263 warnings in 87 files.
   - Current result: 0 errors and 0 warnings.
   - User impact: type errors hide real runtime regressions and make every change risky.
   - Major clusters:
     - Components call private `api.request` directly.
     - UI calls missing client methods like `api.getMessage`.
     - Activity components call methods with the wrong argument count.
     - Settings references missing `addToast`.
     - Voice track ids can be undefined but are used as map keys.
     - Test mocks no longer match actual required type fields.
   - Fix: make API client surface complete and public where needed, then resolve strict type mismatches by feature area.

5. 2FA login is unusable after enabling TOTP.
   - Backend: `internal/auth/auth.go:210-217` requires `totp_code` when TOTP exists.
   - Frontend: `web/src/routes/login/+page.svelte` has only username/password and `api.login` only sends those fields.
   - User impact: users who enable TOTP can lock themselves out through the normal login page.
   - Fix: when login returns `totp_required`, show a code field and retry with `totp_code`; update API client type.

6. Several visible feature components call private or missing API methods.
   - Code examples:
     - `CodeSnippet.svelte`, `Whiteboard.svelte`, `LocationShare.svelte`, `IntegrationSettings.svelte`, `BoostPanel.svelte`, `GuildInsights.svelte`, `WelcomeSettings.svelte`, `AutoRoleSettings.svelte`, `LevelingSettings.svelte`, `Transcription.svelte` call private `api.request`.
     - `CrossChannelQuote.svelte` calls missing `api.getMessage`.
     - `ActivityFrame.svelte` passes `(channelId, sessionId)` to methods that only accept `sessionId`.
   - Current status: fixed for current `web/src` usage. All `api.request(...)` call sites under `web/src` have been replaced with named API client methods, including guild auto-roles, leveling, boosts, integrations, experimental code snippets, whiteboards, location sharing, video recordings, admin federation, and admin bridge management.
   - User impact addressed: visible features no longer depend on ad hoc endpoint strings or generic private transport calls from component code.
   - Remaining fix: keep future endpoints behind typed API client methods and continue tightening loose `any` response shapes where they still exist.

## P1 - Core UX Reliability

7. Client IP detection does not match tests.
   - Command: `go test ./...`.
   - Current failure: `TestClientIP` expects `X-Forwarded-For` handling to return the public client IP, but current behavior returns `10.0.0.1`.
   - User impact: rate limiting, audit logs, admin moderation, setup logs, and security checks can point at the wrong address behind proxies.
   - Fix: clarify trusted-proxy behavior, update implementation/tests, and document deployment expectations.

8. Media EXIF fallback behavior does not match tests.
   - Command: `go test ./...`.
   - Current failure: `TestStripExifData_UnknownFormat` expects fallback PNG encoding for unknown image format.
   - User impact: some uploaded/processed images may fail or preserve metadata unexpectedly depending on current behavior.
   - Fix: decide intended behavior for unknown formats, then align implementation and tests.

9. Raw `fetch` is scattered across pages instead of the shared API client.
    - Code examples: setup, plugins, admin, themes, guild home guide/bump, guild settings templates, user export/import.
    - Current status: setup page, theme gallery, guild home guide/bump, plugins, guild channel templates, user data/account import/export, admin federation, admin bridges, and the main admin bots/rate-limit/content-safety/CAPTCHA flows are fixed and now use the shared API client path.
    - Remaining user impact: smaller leftover embedded/public fetches may still have one-off loading/error handling, but the major authenticated app pages now share auth/envelope/error behavior.
    - Remaining fix: sweep the remaining low-risk fetch sites and keep new endpoints behind typed API client methods.

10. Invite-only registration has no normal registration UI path.
   - Backend supports registration token in query/body.
   - Frontend register page does not expose a token field or read invite token from URL.
   - User impact: invite-only mode can be configured in admin/setup but users cannot register cleanly unless a custom URL/body is crafted.
   - Fix: add token-aware registration UX and plumb token through `api.register`.

11. Channel/guild stores replace entire maps on load.
    - Code: `loadGuilds()` and `loadChannels()` call `setAll`.
    - Current status: fixed. Guild and channel loads now merge API snapshots into the existing stores instead of replacing the whole map, and channel loads ignore stale responses from earlier requests.
    - User impact addressed: background refreshes and fast guild switches should no longer drop realtime-created entries or overwrite newer channel state with an older response.
    - Remaining fix: add a missed-event backfill path so stale entries can be reconciled authoritatively after reconnect without destructive refreshes.

12. Message pagination has weak loading state.
    - Code: global `isLoadingMessages` in `messages.ts`.
    - Current status: fixed. Loading is tracked per channel, visible message lists read only the active channel's loading state, and initial message loads now ignore stale responses by channel/request id.
    - User impact addressed: fast channel switching or reconnect refreshes should no longer let an older initial load merge over newer message state.
    - Remaining fix: true request cancellation can still reduce wasted network work, but stale responses are guarded.

13. Reconnect flow clears and reloads active messages.
    - Code: `web/src/lib/stores/gateway.ts` clears active channel messages on reconnect.
    - Current status: fixed for reconnect and poll refresh. Existing messages are kept while reloads merge in refreshed data.
    - Remaining user impact: reconnect still reloads the active channel instead of doing a true missed-event backfill.
    - Remaining fix: add delta/backfill sync keyed by last message/event id.

14. Role mention unread detection only works for the currently viewed guild.
    - Code: `web/src/lib/stores/gateway.ts:176-187`.
    - Current status: fixed. Gateway role-mention detection now uses a per-guild self-role cache, updates it from member role events, and falls back to loading the current user's roles for messages from non-active guilds.
    - User impact addressed: role mentions in other guilds can now increment mention badges without requiring that guild's member list to be loaded.
    - Remaining fix: prewarm self-role ids on READY if this lookup becomes chatty on large instances.

15. Reactions do not update immediately from gateway events.
    - Backend reaction events currently send only ids/user/emoji, not the `reactions` array the frontend expects.
    - Current status: fixed. The frontend now patches sparse reaction add/remove events locally, including events without `channel_id` and duplicate self events.
    - User impact addressed: visible reaction counts update from gateway events without waiting for a reload or aggregated reaction payload.
    - Remaining fix: server-side aggregated reaction payloads would still be useful for reconciling rare missed events after reconnect.

16. Message editing response does not include attachments/embeds.
    - Backend `HandleUpdateMessage` returns/enqueues the edited message without reloading attachments or embeds.
    - Current status: fixed. Edited message responses/events now include attachments and embeds before being returned or published.
    - User impact addressed: after editing, message media/embed display should no longer disappear until reload.
    - Remaining fix: add regression coverage for edit responses preserving rich message fields.

17. Upload attachment linking is not checked.
    - Backend creates the message, then updates attachments, ignoring the update result/error.
    - Current status: fixed for normal messages, forum posts, and gallery posts.
    - User impact addressed: invalid, already linked, or wrong-user attachment ids now fail the request instead of creating a message/post without the intended file.
    - Remaining fix: add explicit regression tests around attachment ownership/link-count validation.

18. Upload size limit is hard-coded in frontend.
    - Code: `MessageInput.svelte` uses 25 MB constant.
    - Current status: fixed. The backend now exposes `/api/v1/client-config` with the effective media upload limit and upload availability, and the composer consumes it instead of using a fixed 25 MB limit.
    - User impact addressed: client-side validation now matches the active server upload limit and reports disabled uploads before users attempt a send.
    - Remaining fix: extend the same config usage to any specialized upload forms that grow their own client-side size validation.

19. Drag/drop uploads send one message per dropped file.
    - Code: channel and DM page drop handlers.
    - Current status: fixed for text channels and DMs. Dropped files now attach to the composer, where users can review, remove, add alt text, and send them as one message.
    - Remaining user impact: gallery/forum channels still use their specialized post forms by design.
    - Remaining fix: consider shared attachment preview behavior for gallery/forum creation forms if product wants consistency.

20. Slowmode only fails after send attempt.
    - Backend enforces slowmode, frontend does not show a countdown before sending.
    - Current status: improved. The composer now shows a countdown from the active channel's `slowmode_seconds` and the user's latest sent message, and it blocks text, attachment, GIF, sticker, and voice-message sends before upload/send work starts.
    - Remaining user impact: the countdown is inferred from loaded/local messages, so it can still miss slowmode state if the user's latest send is not loaded in this client.
    - Remaining fix: return authoritative slowmode state from the backend or bootstrap channel/member send state.

## P2 - Feature Completeness / Quality Of Life

21. Gateway has many event no-ops for visible features.
    - Examples: scheduled events, onboarding, widgets, activity/game, soundboard, broadcast, screen share, location share, component interactions.
    - Current status: partially fixed. Scheduled guild events now have a shared frontend store used by the guild home and channel sidebar, and channel widgets now have a shared frontend store used by the widget panel. Gateway create/update/delete events update both stores.
    - Remaining user impact: onboarding, activity/game, soundboard, broadcast, screen share, location share, and component interaction events still need feature-specific store/component updates.
    - Remaining fix: continue converting visible event no-ops into shared stores or mounted component handlers.

22. Admin/settings pages are too large and stateful.
    - Examples: `web/src/routes/app/admin/+page.svelte`, `web/src/routes/app/settings/+page.svelte`, guild settings page.
    - Current status: mostly complete for user settings, mostly complete for admin, mostly complete for guild settings. The user settings Data & Privacy tab is now extracted into a focused `SettingsDataTab` component with local export/import state and a narrow profile-import callback. The Voice & Video tab is extracted into `SettingsVoiceTab` with local device, preference, save, and push-to-talk capture state. The Bots tab is extracted into `SettingsBotsTab` with local bot, token, command, edit, and confirmation state. The Privacy tab is extracted into `SettingsPrivacyTab` with local privacy settings and blocked-user state. The Notifications tab is extracted into `SettingsNotificationsTab` with local notification preference, DND, type preference, and muted-item state. The Appearance tab is extracted into `SettingsAppearanceTab` with local theme, custom theme, custom CSS, and connected-account state. The Security tab is extracted into `SettingsSecurityTab` with local password, TOTP, and session state. The Encryption tab is extracted into `SettingsEncryptionTab` with local clear-key behavior. The Account tab is extracted into `SettingsAccountTab` with local profile, avatar/banner cropper, and profile-link state. Admin Users, Guilds, Bots, Instance Bans, Registration, Announcements, Rate Limiting, Content Safety, CAPTCHA, Instance, and Federation Peers tabs are extracted into focused admin components. Guild settings Overview, Roles, Members, Invites, Bans, Categories, Emoji, Stickers, Webhooks, Audit, AutoMod, Moderation, Raid Protection, Onboarding, Ban Lists, and Channel Templates tabs are extracted into focused guild components.
    - Remaining user impact: the admin, settings, and guild settings routes are still too large overall, which keeps type drift and cross-tab state bugs likely.
    - Remaining fix: continue splitting by tab into focused components backed by typed API methods.

23. Many controls have accessibility warnings.
    - Original `svelte-check` reported many missing label associations and unlabeled icon buttons.
    - Current `svelte-check` reports 0 warnings.
    - User impact: screen-reader/keyboard UX is poor; also indicates low UI polish.
    - Fix: add `for`/`id`, `aria-label`, proper button/modal semantics.

24. Login/register flows do not consider registration mode.
    - Current status: fixed. The backend exposes public registration settings at `/api/v1/auth/registration`; login hides or explains registration when it is closed/invite-only, and register validates closed/invite-only mode before submit.
    - User impact addressed: users no longer discover closed registration only after submitting the form, and invite-only instances clearly require a token.

25. API errors are often collapsed to generic toasts.
    - Examples: many catch blocks emit "Failed to send message", "Upload failed", "Failed to save".
    - User impact: users cannot tell permission errors, size limits, validation issues, federation failures, or server outages apart.
    - Fix: standardize `ApiRequestError` display and show useful server messages.

26. Settings/session revoke uses `alert`.
    - Code: `web/src/routes/app/settings/+page.svelte`.
    - Current status: fixed. Session revoke failures now use the app toast path instead of a native browser alert. The remaining frontend `alert`, `confirm`, and `prompt` calls in non-test code have also been replaced with app toasts or a shared confirmation/input modal.
    - User impact addressed: destructive actions and error reporting remain inside the app's normal notification/modal UX instead of browser-native dialogs.

27. Search/theme/plugin/admin pages use bespoke loading and error state.
    - User impact: inconsistent empty states, duplicate patterns, and missing retry affordances.
    - Fix: introduce small reusable async-state helper/component.

28. Frontend tests are stale relative to current types.
    - Current `svelte-check` errors include mock `User`, `Channel`, `Message`, and `Role` objects missing required fields or wrong permission value types.
    - User impact: test suite cannot be trusted as regression safety.
    - Fix: centralize test factories that satisfy current types.

29. Voice track state accepts undefined track ids.
    - Code: `web/src/lib/stores/voice.ts` typecheck failures around `track.sid`.
    - User impact: video/screen-share tile maps can break or leak entries when SDK omits `sid`.
    - Fix: guard missing sid and define fallback lifecycle behavior.

30. User-facing feature claims exceed stable implementation.
    - Examples visible in UI: whiteboard, code snippets, activities, location sharing, transcription, boosts, starboard, leveling, integrations.
    - User impact: users can discover controls that do not compile cleanly or lack typed API support.
    - Fix: either stabilize the API paths and components or hide incomplete features behind an explicit experimental flag.

## Suggested Tackle Order

1. Fix navigation/deep-link behavior. Done in first tranche.
2. Fix message store patching and reaction/embed realtime updates. Done in first tranche.
3. Fix TOTP login so accounts cannot self-lock. Done in first tranche.
4. Make API client complete/public enough for current UI; eliminate direct private `api.request` calls. Done for current `web/src` call sites.
5. Get `npm run check` to zero errors and zero warnings. Done.
6. Harden message/file send flows with transaction checks and better client states. Partially done in fifth tranche.
7. Improve reconnect/channel loading behavior. Partially done in fifth tranche.
8. Normalize raw fetch usage into the API client.
9. Convert event no-ops into real store updates for the features users see.
10. Split large admin/settings/guild settings pages and clean accessibility warnings.

## First Tranche Completed

- Preserved deep app URLs by removing the blanket `/app/...` to `/app` history rewrite.
- Changed message updates to merge partial patches instead of replacing full messages.
- Added sparse reaction-event patching and nonce-based message replacement.
- Added message-store tests for partial updates, reaction events, and nonce replacement.
- Implemented TOTP login retry flow and invite-token registration handling.
- Made the API client expose the currently used generic request method and added `getMessage`.
- Fixed activity-session client call signatures and cross-channel quote typing.
- Stabilized federation constructor tests by skipping DB preload when no pool is supplied.
- Reconciled current client-IP and media EXIF tests with implemented behavior.

## Second And Third Tranches Completed

- Fixed WebCrypto and push-notification `BufferSource` typing without changing call behavior.
- Guarded LiveKit track SID usage and moved camera/screen-share encoding options to publish options.
- Updated stale frontend tests to match current `User`, `Channel`, `Message`, `Role`, and permission shapes.
- Cleared all current `svelte-check` errors across route params, nullable props, stale API calls, modal contracts, notification types, SVG title usage, voice/federation display types, and settings enums.
- Preserved current behavior when incomplete APIs are absent; for server folders, the visible delete action now reports that folders are unavailable in this build instead of calling missing API/store methods.
- Reduced `svelte-check` warnings from 262 to 118 by clearing accessibility issues in major settings/admin/chat/sidebar surfaces and several guild subpanels.

## Fourth Tranche Completed

- Cleared the remaining `svelte-check` warnings to zero across admin/setup routes, guild feature settings, chat/media surfaces, modal overlays, voice controls, gallery/whiteboard/kanban widgets, and layout sidebars.
- Added label/control associations, explicit icon-button labels, keyboard-compatible overlays, focused modal dialog labels, and Svelte 5-safe ref/state declarations where warnings indicated real maintenance or accessibility risk.
- Kept modal click-swallowing behavior where required and used targeted Svelte ignores only for deliberate non-interactive document/backdrop event handling.

## Fifth Tranche Completed

- Made attachment linking transactional for normal messages, forum posts, and gallery posts.
- Validated that every requested attachment id is owned by the sender and still unlinked before message/post creation commits.
- Kept visible messages during gateway reconnect and poll refresh reloads instead of clearing the active channel first.
- Replaced the global message-loading flag with per-channel loading state and wired `MessageList` to the active channel's state.
- Routed guild/DM drag-and-drop files into the `MessageInput` pending attachment composer instead of immediately uploading and sending one message per file.
- Reused the composer path for file picker and pasted image attachments so drag, paste, and picker behavior are consistent.
- Included attachments and embeds in message edit responses/events so edit patches preserve rich message UI.
- Added composer slowmode countdown and client-side send guards across text, attachments, GIFs, stickers, and voice messages.
- Fixed first-run setup route placement so setup status/complete are reachable before any admin exists.
- Promoted the setup admin account during first-run completion and moved the setup page off raw `fetch` onto shared API client methods.
- Moved theme gallery list/share/like/unlike/delete calls to typed API client methods and added optimistic like rollback on failure.
- Moved guild home server guide and bump status/action calls to typed API client methods.
- Moved admin bots, rate-limit stats/log/config, content-scan rules/log, and CAPTCHA settings to typed API client methods.
- Fixed the rate-limit config client path to match the backend route (`PATCH /admin/rate-limits`).
- Moved the plugin marketplace to typed API client methods and restored the backend `/guilds/{guildID}/plugins` routes expected by the plugin install/update/delete handlers.
- Moved settings user data export, account export, and account import flows to shared API client methods.
- Restored the backend `/guilds/{guildID}/channel-templates` routes expected by the channel-template handlers and moved the guild settings template UI to shared API client methods.
- Removed local raw-fetch admin helpers from the federation and bridge admin pages so those flows use the shared API client transport.
- Added `/api/v1/client-config` for client-visible instance limits and wired the message composer to the effective server upload limit instead of a hard-coded 25 MB cap.
- Changed guild/channel store loads to merge snapshots instead of replacing whole maps, with stale channel-load response guarding for fast guild switches.
- Fixed cross-guild role mention unread detection by caching and lazily loading the current user's role ids per guild.
- Added stale-response guarding for initial message loads so older channel refreshes cannot merge over newer message state.
- Added a shared scheduled guild-event store and wired gateway event create/update/delete handling into the guild home and channel sidebar.
- Added a shared channel-widget store and wired gateway widget create/update/delete handling into the channel widget panel.
- Replaced the settings session-revoke native alert with an app toast.
- Replaced remaining native frontend `confirm`, `alert`, and `prompt` calls with shared in-app modal/toast flows.
- Added a public registration-settings endpoint and wired login/register pages to respect closed and invite-only modes before submit.
- Extracted the settings Data & Privacy tab into a dedicated component with local export/import state.
- Extracted the settings Voice & Video tab into a dedicated component with local device and preference state.
- Extracted the settings Bots tab into a dedicated component with local bot, token, command, and edit state.
- Extracted the settings Privacy tab into a dedicated component with local privacy and blocked-user state.
- Extracted the settings Notifications tab into a dedicated component with local notification, DND, type-preference, and muted-item state.
- Extracted the settings Appearance tab into a dedicated component with local theme, custom theme, custom CSS, and connected-account state.
- Extracted the settings Security tab into a dedicated component with local password, TOTP, and session state.
- Extracted the settings Encryption tab into a dedicated component with local clear-key behavior.
- Extracted the settings Account tab into a dedicated component with local profile, avatar/banner cropper, and profile-link state.
- Extracted admin CAPTCHA, Instance Settings, and Federation Peers tabs into dedicated components with local load/save state.
- Extracted the admin Announcements tab into a dedicated component with local create/edit modal and announcement state.
- Extracted the admin Rate Limiting tab into a dedicated component with local stats, config, and log state.
- Extracted the admin Content Safety tab into a dedicated component with local rule, log, and create/edit modal state.
- Extracted the admin Registration tab into a dedicated component with local registration settings, token list, copy/delete, and create-token modal state.
- Extracted the admin Bots tab into a dedicated component with local bot loading, fallback user filtering, expansion, and detail formatting state.
- Extracted the admin Users tab into a dedicated component with local search, suspend/admin/mod actions, user-server expansion, and instance-ban modal state.
- Extracted the admin Guilds tab into a dedicated component with local search/sort, detail modal, and delete confirmation state.
- Extracted the admin Instance Bans tab into a dedicated component with local ban loading and unban state.
- Extracted guild settings Invites, Bans, and Categories tabs into dedicated components with local load, create/update/delete, confirmation, and toast state.
- Extracted guild settings Emoji and Stickers tabs into dedicated components with local upload, delete, pack expansion, confirmation, and toast state.
- Extracted guild settings Webhooks and Audit tabs into dedicated components with local loading, webhook panel wiring, audit formatting, and toast/error state.
- Extracted guild settings AutoMod tab into a dedicated component with local rules, actions, exemption editing, metadata loading, and test-rule state.
- Extracted guild settings Moderation and Raid Protection tabs into dedicated components with local report filtering/resolution, raid config loading, save state, and toast feedback.
- Extracted guild settings Onboarding tab into a dedicated component with local config loading, rule editing, prompt CRUD, option editing, channel/role metadata, and toast feedback.
- Extracted guild settings Ban Lists tab into a dedicated component with local list loading, entry expansion, import/export, subscriptions, confirmations, and toast feedback.
- Extracted guild settings Channel Templates, Overview, Roles, and Members into focused components, leaving the guild settings route as a tab shell with permission gating and navigation only.
- Added named API client methods and shared response types for auto-roles, leveling, boosts, integrations, experimental snippets/location/whiteboards/recordings, admin federation, and admin bridge management.
- Removed all direct `api.request(...)` call sites from `web/src`.

## Verification Snapshot

- `npm run check` currently passes with 0 errors and 0 warnings.
- `docker run --rm -v /docker/AmityVox:/build -w /build -e GOTOOLCHAIN=local golang:1.26-alpine go test ./...` currently passes.
- Focused backend tests currently pass: `docker run --rm -v /docker/AmityVox:/build -w /build -e GOTOOLCHAIN=local golang:1.26-alpine go test ./internal/api/channels ./internal/api`.
- Focused setup/admin backend tests currently pass: `docker run --rm -v /docker/AmityVox:/build -w /build -e GOTOOLCHAIN=local golang:1.26-alpine go test ./internal/api ./internal/api/admin`.
- Targeted frontend tests currently pass: `npm test -- --run src/lib/stores/__tests__/messages.test.ts` reports 13 tests passing. Earlier broader targeted frontend pass covered 8 files, 110 tests.
- `git diff --check` currently passes.

## Unresolved Questions

- Should incomplete experimental features be hidden until stable, or fixed in place as part of the main app?
- Should deep-link-preserving URLs be restored exactly, or was URL hiding intended as a product choice?
