# AmityVox TODO

## Phase 1 (v0.1.0) — COMPLETE
- [x] Go project scaffold + build system (Makefile, Dockerfile, CLI subcommands)
- [x] PostgreSQL schema + migrations (001–005: users, guilds, channels, messages, roles, permissions, invites, bans, audit log, webhooks, emoji, federation peers, read state, user notes/settings, vanity URLs, edit history, member counts, backup codes)
- [x] Config loading (amityvox.toml + env var overrides)
- [x] NATS connection + JetStream stream provisioning
- [x] DragonflyDB cache/presence connection
- [x] User registration + login + Argon2id password hashing + session management
- [x] TOTP 2FA (enable, verify, backup codes)
- [x] REST API skeleton (/api/v1/*) with chi router
- [x] Guild CRUD + membership + categories
- [x] Channel CRUD + permission overrides
- [x] Message send/receive/edit/delete + reactions + pins + threads
- [x] WebSocket gateway (identify, heartbeat, resume, event dispatch)
- [x] Permission bitfield system + role management
- [x] File upload (S3/Garage integration, thumbnails, blurhash, EXIF strip)
- [x] Typing indicators + presence tracking
- [x] DM + group DM channels
- [x] Invite system (create, accept, delete, vanity URLs)
- [x] Search (Meilisearch integration)
- [x] Webhooks (CRUD + execution)
- [x] Audit log
- [x] Admin CLI (create-user, suspend, set-admin, list-users)
- [x] Rate limiting middleware

## Phase 2 (v0.2.0) — COMPLETE
- [x] LiveKit voice/video integration (token gen, room management, voice state)
- [x] Media processing (thumbnails 128/256/512, blurhash, EXIF strip, S3 upload)
- [x] WebAuthn/FIDO2 authentication (registration + login, DragonflyDB sessions)
- [x] Federation sync protocol (HLC ordering, signed delivery, inbox handler, retry queue)
- [x] MLS encryption delivery service (key packages, welcome messages, group state, commits)

## Phase 3 (v0.3.0) — COMPLETE
- [x] Docker Compose deployment (Dockerfile + docker-compose.yml + Caddyfile)
- [x] Video transcoding worker (ffmpeg/ffprobe dispatch, NATS job queue)
- [x] Embed unfurling worker (OpenGraph metadata extraction, link previews)
- [x] AutoMod system (word/regex/invite/mention/caps/spam/link filters, configurable rules, CRUD API)
- [x] Push notifications (WebPush/VAPID integration, per-guild preferences, recipient resolution)
- [x] Matrix bridge adapter (bridges/matrix/ — Application Service API, virtual users)
- [x] Discord bridge adapter (bridges/discord/ — bot gateway, webhook relay)
- [x] Integration tests with dockertest (PostgreSQL 16, NATS 2, Redis 7)
- [x] SvelteKit frontend scaffold (web/ — full UI with auth, guilds, channels, messages, settings, admin)

---

## Phase 4 (v0.4.0) — UI/QoL Overhaul — IN PROGRESS

### Documentation & Infrastructure Setup
- [x] Update CLAUDE.md with feature completion safeguards
- [x] Create `docs/ui-standards.md` frontend conventions document
- [x] Set up vitest for frontend unit tests
- [x] Set up Playwright for frontend E2E tests
- [x] Add test scripts to `web/package.json`

### Sprint 1: Message Interactions
> Goal: Messages feel interactive — users can right-click, reply, react, edit, delete.

- [x] **ContextMenu component** — Reusable `<ContextMenu>` positioned near cursor, closes on click-outside/Escape. Used across messages, attachments, users, channels.
- [x] **Message context menu** — Right-click message: Copy Text, Reply, Edit (own only), Delete (own or admin), Pin/Unpin, Copy Message Link
- [x] **Reply system** — Click "Reply" shows reply bar above input with quoted message preview + X to cancel. Sends `reply_to` field. Display reply reference above message in chat with clickable jump.
- [x] **Edit message** — Edit own messages via context menu or Up arrow on last message. Input shows edit mode (yellow border). PUT `/api/v1/channels/{id}/messages/{id}`. Show "(edited)" indicator with timestamp.
- [x] **Delete message** — Confirmation modal. DELETE endpoint. Remove from message list. Admins can delete any message.
- [x] **Reactions display** — Reaction chips below message with emoji + count. Click to toggle own reaction. Long-press/hover to see who reacted. POST/DELETE `/api/v1/channels/{id}/messages/{id}/reactions/{emoji}`.
- [x] **Attachment context menu** — Right-click attachment: Download, Open in New Tab, Copy URL. Images also get Copy Image.
- [x] **Message timestamps** — Hover shows full datetime tooltip. Show relative time ("2m ago", "Yesterday"). Date separator bars between different days.

**Sprint 1 Tests:**
- [x] messageInteraction store unit test: start/cancel reply, start/cancel edit, mutual exclusivity
- [ ] ContextMenu unit test: open, close, position, keyboard
- [ ] MessageItem unit test: context menu items, edit mode, reactions
- [ ] MessageInput unit test: reply bar, edit mode, submit
- [ ] E2E: send message, edit, delete, react

### Sprint 2: DMs, Unread Indicators, Pinned Messages
> Goal: Users can DM each other, see what's unread, and pin important messages.

- [x] **DM conversations** — Sidebar "Direct Messages" section below guilds. Show list of DM conversations with avatars. "New Message" button to start DM. GET/POST `/api/v1/users/@me/channels`.
- [x] **Unread indicators** — Bold channel name + numeric badge for unread messages. Track last-read message position per channel. PUT `/api/v1/channels/{id}/ack` when channel is viewed and scrolled to bottom.
- [x] **Unread badge on guilds** — White dot on guild icon sidebar if any channel within has unreads. Mention count badge if @mentioned.
- [x] **User popover** — Click username in chat or member list: avatar, display name, bio, role list, "Message" button (opens/creates DM), "Add Friend" button.
- [x] **Pinned messages panel** — Pin icon in channel header shows pin count. Click opens slide-out panel with pinned messages. GET `/api/v1/channels/{id}/pins`. Jump to message on click.
- [x] **Channel topic** — Show channel topic below name in header. Truncate with ellipsis, click to expand full topic.
- [x] **Typing indicators** — "User is typing..." animated dots at bottom of message list. Multiple users: "User1 and User2 are typing..." / "Several people are typing...". Uses existing WebSocket OpTyping.
- [x] **Online/offline presence** — Green (online), yellow (idle), red (DnD), gray (offline) dot on avatars. Uses existing presence system backend.

**Sprint 2 Tests:**
- [x] DMs store unit test: add, remove, update, sort (6 tests)
- [x] unreads store unit test: increment, clear, total (5 tests)
- [x] presence store unit test: update, remove, derived (6 tests)
- [ ] UserPopover unit test: render, actions
- [ ] PinnedMessages unit test: render, jump
- [ ] E2E: create DM, send DM, check unread badge

### Sprint 3: Settings That Work
> Goal: All settings pages are functional, not just UI shells.

- [x] **Profile editing** — Change display name, bio, email. Upload avatar (file picker + hover overlay). PATCH `/api/v1/users/@me`. Changes reflect immediately.
- [x] **Password change** — Current password + new password + confirm new. Validation (min 8 chars, match). POST `/api/v1/auth/password`.
- [x] **2FA setup** — Enable TOTP: show QR code, manual secret, verify with 6-digit code, show backup codes. POST `/api/v1/auth/totp/enable`, `/verify`.
- [x] **Active sessions** — Table with browser/IP/last active. "Revoke" button per session. Current session indicator. GET/DELETE `/api/v1/users/@me/sessions`.
- [ ] **Notification preferences** — Global: desktop notifications on/off, sounds on/off. Per-guild: mute all, mentions only, everything. Per-channel: mute toggle.
- [ ] **Privacy settings** — Who can DM me: everyone / friends only / nobody. Who can add me as friend: everyone / mutual guilds / nobody.
- [x] **Appearance settings** — Theme: dark/light. Font size slider (12-20px). Compact mode toggle. Stored in localStorage.
- [x] **Settings navigation** — Left sidebar with sections: My Account, Security, Appearance. Consolidated from 5 tabs to 3 functional sections.

**Sprint 3 Tests:**
- [ ] Profile form unit test: validation, submit, avatar preview
- [ ] TwoFactorSetup unit test: QR display, code verify
- [ ] SessionList unit test: render, revoke
- [ ] settings store unit test: load, save, sync
- [ ] E2E: change profile, change password, toggle appearance

### Sprint 4: Guild Management
> Goal: Guild owners/admins can fully manage their server.

- [x] **Guild settings page** — Edit name, description, icon upload with preview. Delete guild with type-name-to-confirm safety. Danger zone styling.
- [x] **Role management** — Create roles with name. View list with color dot, hoisted/mentionable badges, position.
- [ ] **Channel categories** — Create/rename/delete categories. Drag channels between categories. Collapse/expand categories. Category-level permission overrides.
- [x] **Custom emoji** — View emoji grid with images and names. Delete emoji. Upload pending (requires backend file association).
- [ ] **Webhook management** — Create webhook (name, avatar, channel). Edit name/avatar. Delete webhook. Copy webhook URL. Regenerate token.
- [x] **Invite management** — Create invite with expiry (30m–never) and max uses. Table of active invites with uses, expiry, copy link, revoke.
- [x] **Audit log viewer** — List of 50 entries with actor, action type label, timestamp, reason. 16 action type labels.
- [x] **Ban management** — List banned users with avatar, name, reason. Unban button.

**Sprint 4 Tests:**
- [ ] RoleEditor unit test: permission toggle, color, drag reorder
- [ ] EmojiUploader unit test: preview, validate name
- [ ] InviteManager unit test: create, revoke, expiry display
- [ ] E2E: create role, assign to member, create invite, use invite

### Sprint 5: Advanced Features
> Goal: Power-user features that make AmityVox competitive.

- [ ] **Threads** — "Create Thread" in message context menu. Thread panel slides in from right. Separate message stream. Thread indicator on parent message with reply count. POST `/api/v1/channels/{id}/messages/{id}/threads`.
- [ ] **Message search** — Ctrl+K opens search modal. Full-text via Meilisearch. Filters: from user, in channel, has attachment, date range. Results with highlighted matches and surrounding context. Click to jump to message in channel.
- [ ] **Message edit history** — Click "(edited)" badge to see previous versions. Modal with diff view showing what changed. GET `/api/v1/channels/{id}/messages/{id}/history`.
- [ ] **User notes** — In user popover: "Note" textarea. Private to you. Auto-saves on blur. PUT `/api/v1/users/{id}/notes`.
- [ ] **Enhanced markdown** — Code blocks with syntax highlighting (highlight.js or Shiki). Spoiler tags (`||text||`). Block quotes (`> text`). Ordered/unordered lists. Tables.
- [ ] **Message forwarding** — "Forward" in context menu. Channel/DM picker dialog. Forwarded message shows attribution and original timestamp.
- [ ] **Keyboard shortcuts** — Ctrl+K: search. Escape: close any panel/modal. Up: edit last message. Ctrl+Shift+M: mute current channel. Alt+Up/Down: navigate channels. ?: show shortcuts help.
- [ ] **Notification toasts** — In-app toast component (bottom-right, auto-dismiss 5s). Desktop notification API for mentions/DMs when tab not focused. Notification permission request on first use.
- [ ] **Transition animations** — Page transitions (fade/slide), sidebar expand/collapse, modal open/close, message appear. Svelte `transition:` and `animate:` directives.
- [ ] **Image lightbox / media gallery** — Click image to open full-size overlay. Arrow keys to navigate between images in channel. Zoom support. Download button.
- [ ] **Mobile responsive layout** — Collapsible sidebars (swipe/hamburger), touch-friendly hit targets, bottom navigation bar on small screens, no horizontal scroll.
- [ ] **Emoji picker** — Grid of unicode + custom emoji. Category tabs. Search/filter. Skin tone selector. Recently used. Insert into message input at cursor.
- [ ] **Giphy integration** — GIF button in message input opens Giphy search panel. Search, trending, preview. Click to insert GIF URL as message. Backend proxy endpoint to avoid exposing API key to client. Requires GIPHY_API_KEY in config. See `docs/giphy-setup.md` for setup instructions.

**Sprint 5 Tests:**
- [ ] ThreadPanel unit test: create, render, reply
- [ ] SearchPanel unit test: query, results, jump
- [ ] Markdown renderer unit test: each syntax element
- [ ] Toast system unit test: show, dismiss, stack
- [ ] E2E: search message, create thread, forward message

---

## Phase 5 (Future)

### Admin Dashboard
- [ ] Live stats page (users, guilds, messages, connections — polls /admin/stats)
- [ ] User management UI (list, search, suspend, unsuspend, set admin)
- [ ] Instance settings editor (name, description, federation mode, registration)
- [ ] Federation peer management (list, add, remove peers)

### Channel Types
- [ ] Forum channel UI (thread list, post/reply layout, tags, sorting)
- [ ] Stage channel UI (speaker queue, hand raise, audience view)

### E2E Encryption UI
- [ ] MLS key exchange flow in client (key package upload, welcome handling)
- [ ] Encrypted channel indicator + lock icon
- [ ] Device verification UI
- [ ] Key backup/recovery flow

### Desktop & Mobile
- [ ] Tauri desktop wrapper (window management, system tray, native notifications)
- [ ] Capacitor mobile wrapper (iOS + Android, push notification integration)

### Extensibility
- [ ] Plugin/WASM system (API for third-party plugins, sandboxed execution)
- [ ] Bot API improvements (slash commands, interactions, message components)
- [ ] Custom theme support (user-uploadable CSS themes)

### Infrastructure
- [ ] Prometheus metrics exporter (message rates, connection counts, latency)
- [ ] install.sh one-liner script for fresh deployments
- [ ] Multi-arch Docker images (amd64 + arm64 in CI)
- [ ] Automated backup/restore tooling for PostgreSQL + S3
- [ ] RPi5 performance profiling + optimization pass
