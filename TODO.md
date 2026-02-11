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

## Phase 4 (v0.4.0) — Next

### Frontend Polish
- [ ] Transition animations (page transitions, sidebar expand/collapse)
- [ ] Notification panel (toast notifications for mentions, DMs, system events)
- [ ] Search UI (global search modal with filters: messages, users, guilds)
- [ ] Mobile responsive layout (collapsible sidebars, touch gestures, bottom nav)
- [ ] Markdown rendering in messages (bold, italic, code, links, spoilers)
- [ ] Emoji picker component
- [ ] Image lightbox / media gallery viewer
- [ ] Unread indicators (badge counts on guilds/channels, unread line in messages)
- [ ] Context menus (right-click on messages, users, channels)
- [ ] Keyboard shortcuts (Ctrl+K search, Ctrl+/ help, arrow navigation)

### Admin Dashboard
- [ ] Live stats page (users, guilds, messages, connections — polls /admin/stats)
- [ ] User management UI (list, search, suspend, unsuspend, set admin)
- [ ] Instance settings editor (name, description, federation mode, registration)
- [ ] Federation peer management (list, add, remove peers)
- [ ] Audit log viewer with filters

### Channel Types
- [ ] Forum channel UI (thread list, post/reply layout, tags, sorting)
- [ ] Stage channel UI (speaker queue, hand raise, audience view)
- [ ] Thread panel (side panel, thread list in parent channel)

### E2E Encryption UI
- [ ] MLS key exchange flow in client (key package upload, welcome handling)
- [ ] Encrypted channel indicator + lock icon
- [ ] Device verification UI
- [ ] Key backup/recovery flow

### Desktop & Mobile
- [ ] Tauri desktop wrapper (window management, system tray, native notifications)
- [ ] Capacitor mobile wrapper (iOS + Android, push notification integration)
- [ ] Desktop-specific features (global hotkeys, auto-start, update checker)

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
