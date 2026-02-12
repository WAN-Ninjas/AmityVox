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

## Phase 4 (v0.4.0) — UI/QoL Overhaul — COMPLETE

### Documentation & Infrastructure Setup
- [x] Update CLAUDE.md with feature completion safeguards
- [x] Create `docs/ui-standards.md` frontend conventions document
- [x] Set up vitest for frontend unit tests
- [x] Set up Playwright for frontend E2E tests
- [x] Add test scripts to `web/package.json`

### Sprint 1: Message Interactions
- [x] ContextMenu component
- [x] Message context menu (Copy, Reply, Edit, Delete, Pin, Copy Link)
- [x] Reply system with quoted preview
- [x] Edit message with "(edited)" indicator
- [x] Delete message with confirmation
- [x] Reactions display with toggle
- [x] Attachment context menu
- [x] Message timestamps with date separators

### Sprint 2: DMs, Unread Indicators, Pinned Messages
- [x] DM conversations in sidebar
- [x] Unread indicators (bold + badge)
- [x] Unread badge on guild icons
- [x] User popover (profile, Message, Add Friend)
- [x] Pinned messages panel
- [x] Channel topic display
- [x] Typing indicators
- [x] Online/offline presence dots

### Sprint 3: Settings That Work
- [x] Profile editing (display name, bio, email, avatar upload)
- [x] Password change
- [x] 2FA setup (TOTP QR, backup codes)
- [x] Active sessions (list, revoke)
- [x] Notification preferences (global + per-guild)
- [x] Privacy settings (DM, friend requests)
- [x] Appearance settings (theme, font, compact mode)
- [x] Settings navigation sidebar

### Sprint 4: Guild Management
- [x] Guild settings (name, description, icon, delete)
- [x] Role management (CRUD, color, hoist, mentionable)
- [x] Channel categories (CRUD, drag, collapse)
- [x] Custom emoji (view, delete, upload)
- [x] Webhook management (CRUD, copy URL)
- [x] Invite management (create, list, revoke)
- [x] Audit log viewer
- [x] Ban management (list, unban)

### Sprint 5: Advanced Features
- [x] Threads
- [x] Message search (Ctrl+K, Meilisearch)
- [x] Message edit history
- [x] User notes
- [x] Enhanced markdown (syntax highlight, spoilers, tables)
- [x] Message forwarding
- [x] Keyboard shortcuts
- [x] Notification toasts + desktop notifications
- [x] Transition animations
- [x] Image lightbox / media gallery
- [x] Mobile responsive layout
- [x] Emoji picker (unicode + custom, search, skin tone)
- [x] Giphy integration

---

## Phase 5 (v0.5.0) — Platform & Infrastructure — IN PROGRESS

### Admin Dashboard
- [x] Live stats page (users, guilds, messages, connections)
- [x] User management UI (list, search, suspend, unsuspend, set admin)
- [x] Instance settings editor (name, description, federation mode)
- [x] Federation peer management (list, add, remove)
- [x] Admin button in guild sidebar (visible only to admins)

### Channel Types
- [x] Forum channel UI (thread list, post/reply layout, tags, sorting)
- [x] Stage channel UI (speaker queue, hand raise, audience view)

### E2E Encryption UI
- [x] MLS key exchange flow in client
- [x] Encrypted channel indicator + lock icon
- [x] Device verification UI
- [ ] Key backup/recovery flow

### Desktop & Mobile
- [x] Tauri desktop wrapper
- [x] Capacitor mobile wrapper

### Infrastructure
- [x] Prometheus metrics exporter
- [x] install.sh one-liner script
- [x] Automated backup/restore tooling
- [ ] Multi-arch Docker images (amd64 + arm64)
- [ ] RPi5 performance profiling + optimization pass

---

## Phase 6 (v0.6.0) — Community & Social Features

> Inspired by: Discord (onboarding, discovery, events), Revolt/Stoat (user emoji), Matrix (spaces, knocking)

### Server Onboarding
- [x] Welcome screen with server description and featured channels
- [ ] Onboarding questionnaire flow — customizable questions that assign roles/channels
- [ ] Rules/ToS acceptance gate before participating (membership screening)
- [ ] Default channels configuration (which channels new members see first)
- [ ] Server guide — curated walkthrough for new members

### Server Discovery
- [x] Public server directory (browse, search, filter)
- [x] Category tags for servers (Gaming, Music, Education, Tech, etc.)
- [x] Server preview page (description, member count, online count, featured channels) before joining
- [ ] "Bump" system — servers can promote themselves periodically
- [x] Discoverable toggle per guild in guild settings

### Scheduled Events
- [x] Create events for voice channels, stage channels, or external locations
- [x] Event details: name, description, date/time, cover image, recurrence
- [x] RSVP / "Interested" tracking with count
- [ ] Event reminders via push notification (15 min, 1 hour before)
- [x] Event listing in guild sidebar and dedicated events tab
- [ ] Auto-cancel if event doesn't start within configurable window

### Polls
- [x] Native poll message type (not a bot — first-class support)
- [x] Multiple choice with 2–10 options
- [x] Single vote vs. multi-vote toggle
- [x] Anonymous vs. public voting
- [x] Duration (1h, 4h, 8h, 24h, 3d, 7d, never)
- [x] Results display (bar chart with percentages, live updating)
- [x] Poll close + final results announcement

### Announcement Channels
- [x] Announcement channel type with "Follow" support
- [x] Cross-server message publishing — followers get messages in their servers
- [x] "Published" indicator on announcement messages
- [ ] Follow management UI (which servers are following your announcements)

### Custom Status & Rich Presence
- [x] Custom status (text + single emoji) displayed on profile and member list
- [x] Status expiry (don't clear, 30 min, 1 hour, 4 hours, today, custom)
- [x] "About Me" section with markdown support on user profile
- [x] Connected accounts display (GitHub, Spotify, Steam, etc.) — decorative, no OAuth needed
- [ ] Activity status ("Playing X", "Listening to Y") via API for bots/integrations

### User Badges & Profile Customization
- [x] Badge system — Admin, Moderator, Server Owner, Early Supporter, Bot, Verified
- [x] Custom profile banner image (upload in settings)
- [x] Profile accent color picker
- [ ] User-level custom emoji (up to 10 personal emoji usable anywhere, a la Revolt)
- [x] Pronouns field on profile

### Saved Messages
- [x] Personal "Saved Messages" channel (like Telegram's saved messages)
- [x] Bookmark any message to saved messages via context menu
- [x] Access from sidebar (dedicated icon) — always available
- [x] Searchable, pinnable, private to you

---

## Phase 7 (v0.7.0) — Voice & Media Upgrades

> Inspired by: Discord (soundboard, text-in-voice, noise suppression, DAVE), Matrix (voice broadcast)

### Voice Channel Enhancements
- [x] Text-in-Voice — persistent text chat within voice channels, visible to connected users
- [x] Voice channel user limit (configurable per channel)
- [x] Voice channel bitrate configuration (64kbps – 384kbps, higher with guild level)
- [ ] Push-to-Talk mode (configurable keybind)
- [ ] Voice Activity Detection mode (auto-detect when speaking)
- [ ] Priority Speaker permission — louder than other users, others auto-attenuate
- [x] Server mute / server deafen (moderator actions on other users)
- [x] Move members between voice channels (moderator action)
- [x] AFK voice channel + timeout (auto-move idle users after configurable period)

### Soundboard
- [ ] Per-guild sound clips (short audio, <5 seconds)
- [ ] Play sounds in voice channel (click to play)
- [ ] Default sound slots (8 base, more with boosts or admin config)
- [ ] Upload custom sounds (name, volume normalization)
- [ ] Sound management in guild settings
- [ ] Sound cooldown to prevent spam

### Voice Broadcast
- [ ] One-way live audio streaming to a channel (like a podcast/PA system)
- [ ] Listeners hear audio in near-real-time via chunked voice messages
- [ ] Broadcast indicator in channel (who's broadcasting, duration)
- [ ] Start/stop broadcast controls for speakers
- [ ] Permission-gated (who can start a broadcast)

### Screen Sharing Improvements
- [ ] Share entire screen or single application window
- [ ] Configurable resolution (720p/1080p/4K) and frame rate (15/30/60fps)
- [ ] Multiple viewers support (up to 50 concurrent)
- [ ] Audio sharing toggle (share system audio)

### Media Processing
- [ ] Voice messages — record and send audio clips inline in text channels
- [ ] Audio waveform visualization for voice messages
- [ ] Video player improvements — inline playback with controls, volume, fullscreen
- [x] Image paste from clipboard (Ctrl+V to upload)
- [x] Drag-and-drop multiple files with individual progress bars
- [ ] File size limit display in upload dialog
- [ ] Image compression options (original vs. compressed)

---

## Phase 8 (v0.8.0) — Moderation & Safety

> Inspired by: Discord (AutoMod, verification levels, raid protection), Matrix (Mjolnir/Draupnir, ban lists, server ACLs), Spacebar (instance rights)

### AutoMod Frontend
- [x] AutoMod rule builder UI in guild settings (visual, not just API)
- [x] Rule types: keyword filter, regex, invite links, mention spam, caps, link filter, spam detection
- [x] Actions: delete message, timeout user, alert channel, log only
- [x] Exempt roles and channels per rule
- [x] AutoMod action log viewer (what was caught, when, what action was taken)
- [ ] Test rule — preview what messages would match before enabling

### Server Verification Levels
- [x] None — anyone can chat immediately
- [x] Low — must have verified email
- [x] Medium — must be registered for 5+ minutes
- [x] High — must be a member for 10+ minutes
- [x] Highest — must have verified phone number (or admin bypass)
- [x] UI in guild settings to select level

### Raid Protection
- [x] Join rate detection (X joins in Y seconds triggers lockdown)
- [x] Automatic lockdown mode (pause invites, restrict new member permissions)
- [ ] DM spam detection (new member sending identical DMs to many members)
- [ ] Anti-raid alert in mod channel
- [x] Lockdown controls (enable/disable from guild settings or slash command)
- [x] New member holding period (restrict actions for first N minutes)

### Moderation Actions
- [x] Timeout/mute members (configurable duration: 60s, 5m, 10m, 1h, 1d, 1w, custom)
- [x] Timeout indicator on user avatar and in member list
- [x] Warn member (logged warning with reason, viewable in user's mod history)
- [x] Mod history per user (all warnings, timeouts, kicks, bans in one place)
- [x] Bulk message delete (select messages by user or date range)
- [x] Slow mode per channel (configurable interval: 5s, 10s, 15s, 30s, 1m, 2m, 5m, 10m, 15m, 30m, 1h, 2h, 6h)
- [x] Lock channel (temporarily prevent all messages, mod-only toggle)

### NSFW & Content Safety
- [x] NSFW channel designation (toggle in channel settings)
- [x] Age gate / content warning before viewing NSFW channels
- [ ] Content filter setting per user (blur all images in NSFW, blur suspicious, show all)
- [x] Report message to server mods (with reason)
- [x] Report message to instance admins
- [ ] User block improvements — blocked users' messages hidden with "blocked message" placeholder

### Shared Ban Lists (Inspired by Matrix Mjolnir)
- [x] Exportable ban lists (JSON format with user IDs, reasons, timestamps)
- [x] Import ban lists from other servers/instances
- [x] Subscribable ban lists — auto-apply bans from trusted list sources
- [ ] Ban list management UI in guild settings

### Instance-Wide Moderation (Admin)
- [x] Instance-wide user ban (not just per-guild)
- [ ] IP-based rate limiting visibility in admin panel
- [x] Registration controls (open, invite-only via registration tokens, closed)
- [x] Registration token generation and management UI
- [x] Instance announcement system (broadcast message to all users)
- [ ] Content scanning / data loss prevention (configurable regex patterns on uploads)
- [ ] CAPTCHA on registration (configurable: none, hCaptcha, reCAPTCHA)

---

## Phase 9 (v0.9.0) — Extensibility & Theming

> Inspired by: Revolt/Stoat (custom CSS, theme editor), Discord (bot ecosystem, Activities), Spacebar (plugin system), Matrix (widgets, integrations)

### Custom Themes
- [x] Theme system with CSS variables (colors, fonts, border radius, spacing)
- [x] Built-in theme presets (Dark, Light, AMOLED, Solarized, Nord, Dracula, Monokai, Catppuccin)
- [ ] Theme editor UI — live preview with color pickers for every variable
- [ ] Import/export themes as JSON files
- [x] Per-user theme selection (stored in user settings, synced across devices)
- [ ] Custom CSS injection (advanced mode — paste raw CSS, sandboxed to prevent XSS)
- [ ] Community theme gallery (share themes with other users via shareable links)

### Bot API & Framework
- [ ] Bot account creation via admin panel or API
- [ ] Bot token authentication (separate from user sessions)
- [ ] Bot permissions model (scoped to guilds, limited by role hierarchy)
- [ ] Slash commands framework — register commands, receive interactions via HTTP webhook
- [ ] Message components — buttons, select menus, modals (for interactive bot messages)
- [ ] Bot presence/status updates via API
- [ ] Bot event subscriptions (per-guild, filter by event type)
- [ ] Bot rate limiting (separate from user limits, configurable)
- [ ] Official bot SDK (Go package) for building bots against AmityVox API
- [ ] Bot directory page in admin panel (list installed bots, manage permissions)

### Widgets & Embeds
- [ ] Embeddable server widget (HTML/iframe for external websites)
- [ ] Widget shows online member count, server name, invite button
- [ ] Channel widgets — embed interactive web pages within channels (like Matrix widgets)
- [ ] Built-in widget types: Etherpad (collaborative notes), YouTube player, countdown timer
- [ ] Widget permissions (who can add/remove widgets)

### Plugin/WASM System
- [ ] WASM plugin runtime — load plugins as WebAssembly modules server-side
- [ ] Plugin API: hook into message events, guild events, scheduled tasks
- [ ] Plugin sandboxing (memory limits, CPU limits, no filesystem access)
- [ ] Plugin marketplace / directory (list, install, configure)
- [ ] Plugin configuration UI in guild settings
- [ ] Example plugins: welcome message, auto-role, leveling system, starboard

### Webhook Improvements
- [ ] Webhook templates (pre-built formats for GitHub, GitLab, Jira, Sentry, etc.)
- [ ] Webhook message formatting preview
- [ ] Webhook execution logs (last 100 deliveries with status codes)
- [ ] Incoming + Outgoing webhook types (outgoing: trigger on channel events, POST to external URL)

---

## Phase 10 (v1.0.0) — Federation & Interoperability

> Inspired by: Matrix (DAG-based federation, server ACLs, bridges), Revolt (planned federation), Spacebar (multi-instance)

### Federation Improvements
- [ ] Federation status dashboard in admin panel (peer health, last sync, event lag)
- [ ] Per-peer federation controls (allow/block specific instances)
- [ ] Federated user profiles (view profile from remote instance)
- [ ] Federated message delivery reliability improvements (delivery receipts, retry UI)
- [ ] Federated search (search messages across federated instances, opt-in)
- [ ] Instance blocklist/allowlist management UI
- [ ] Federation protocol versioning (negotiate capabilities on handshake)

### Bridge UI
- [ ] Matrix bridge status dashboard (connected rooms, virtual users, sync lag)
- [ ] Discord bridge status dashboard (connected channels, bot status)
- [ ] Bridge configuration UI (channel mapping, user mapping)
- [ ] Bridge message attribution (show which platform a message originated from)
- [ ] Additional bridge targets: Telegram, Slack, IRC, XMPP

### Multi-Instance Awareness
- [ ] Client support for connecting to multiple AmityVox instances simultaneously
- [ ] Instance switcher in guild sidebar
- [ ] Unified notification stream across instances
- [ ] Cross-instance DMs (via federation)

### Data Portability
- [ ] GDPR data export (download all your data as JSON/ZIP)
- [ ] Account migration between instances (export + import profile, settings, relationships)
- [ ] Message archive export (per-channel, as JSON or HTML)
- [ ] Guild template export/import (clone guild structure including roles, channels, categories, permissions)

---

## Phase 11 (v1.1.0) — Power User & QoL Features

> Inspired by: All competitors — the polish that separates "functional" from "delightful"

### Message Improvements
- [x] Message scheduling — compose now, send at a specific date/time
- [ ] Message bookmarks/reminders — save any message with optional reminder time
- [ ] Bulk message operations — select multiple messages, bulk delete/move/pin
- [ ] Message link previews — pasting an AmityVox message link shows inline embed of that message
- [ ] Quoted replies with message preview for cross-channel replies
- [x] Silent messages — send without triggering notifications (prefix with @silent or toggle)
- [x] Slow mode bypass for specific roles
- [x] LaTeX/KaTeX math rendering (`$$formula$$` renders inline math)
- [ ] Message translation — click to translate a message (via LibreTranslate or similar self-hosted service)

### Channel Improvements
- [ ] Channel templates — save channel configuration as reusable template
- [x] Channel archive — archive instead of delete (read-only, searchable, hidden from sidebar)
- [ ] Read-only channels (announcement-style, only certain roles can post)
- [ ] Channel-specific custom emoji (emoji only available within that channel)
- [ ] Default thread auto-archive duration per channel
- [ ] Channel clone — duplicate channel with all settings/permissions

### Navigation & UX
- [x] Command palette (Ctrl+K) — unified search for channels, users, messages, settings, actions
- [ ] Quick switcher — type channel/guild name to jump instantly
- [ ] Channel groups — custom grouping beyond categories (user-side organization)
- [ ] Collapsible sidebar sections (DMs, each guild's categories)
- [x] Mark all as read (per guild or global)
- [x] Jump to unread — button that scrolls to first unread message
- [x] Scroll to bottom button with unread count
- [ ] Back/forward navigation history (like browser)
- [ ] Recent channels list (quickly return to previously viewed channels)

### Notification Improvements
- [x] Notification center — dedicated panel showing all recent notifications in one place
- [x] Notification grouping (group by channel/guild)
- [ ] Notification sounds — custom sounds per notification type
- [ ] Do Not Disturb scheduling (auto-enable DND during certain hours)
- [ ] Per-thread notification settings
- [x] Mention highlights in channel list (show which channels have mentions vs just unreads)

### Accessibility
- [x] Screen reader support (ARIA labels, semantic HTML, focus management)
- [x] High contrast theme variant
- [x] Reduced motion mode (disable animations)
- [x] Font size scaling (apply globally, including UI chrome not just messages)
- [ ] Keyboard-only navigation for all features
- [ ] Alt text for uploaded images (optional field on upload)
- [x] Dyslexia-friendly font option (OpenDyslexic)

### Stickers
- [ ] Sticker pack system — upload sets of images as sticker packs
- [ ] Sticker picker in message input (separate tab from emoji)
- [ ] Guild sticker packs (custom stickers per server)
- [ ] User sticker packs (personal stickers usable anywhere)
- [ ] Sticker pack sharing — share pack via link

---

## Phase 12 (v1.2.0) — Advanced Infrastructure

> Production hardening, observability, and operational excellence

### Observability
- [ ] Structured logging improvements (request tracing with correlation IDs)
- [ ] Distributed tracing (OpenTelemetry integration)
- [ ] Grafana dashboard templates (pre-built dashboards for AmityVox metrics)
- [ ] Health check improvements (deep health — check DB, NATS, DragonflyDB, S3, Meilisearch)
- [ ] Alert rules (Prometheus alert templates for common failure modes)

### Performance
- [ ] Connection pooling optimization (tune pgx pool for RPi5)
- [ ] Message pagination with cursor-based pagination (replace offset-based)
- [ ] Lazy-load member list (only fetch visible members, not entire guild)
- [ ] Image lazy loading in message list (IntersectionObserver)
- [ ] Service worker for offline support and caching
- [ ] Bundle size optimization (code splitting, tree shaking audit)
- [ ] WebSocket reconnection improvements (exponential backoff with jitter, connection quality indicator)

### Security Hardening
- [ ] Content Security Policy headers
- [ ] Subresource Integrity for all CDN assets
- [ ] Rate limit improvements (sliding window, per-endpoint tuning)
- [ ] Session security — detect concurrent sessions from different geolocations
- [ ] Password breach checking (HaveIBeenPwned API, k-anonymity model)
- [ ] File upload scanning (ClamAV integration for virus scanning)
- [ ] 2FA requirement for moderators (guild-level setting, a la Discord)

### CI/CD
- [ ] Multi-arch Docker builds (amd64 + arm64) in GitHub Actions
- [ ] Automated E2E test suite in CI (Playwright against Docker Compose stack)
- [ ] Automated dependency updates (Dependabot or Renovate)
- [ ] Release automation (tag → build → publish Docker image → create GitHub release)
- [ ] Changelog generation from conventional commits

### Deployment Options
- [ ] Kubernetes Helm chart
- [ ] Ansible playbook for bare-metal deployment
- [ ] One-click deploy buttons (DigitalOcean, Railway, Render)
- [ ] ARM64-optimized Docker image (tested on RPi5)
- [ ] SQLite-compatible mode for single-user/small instances (stretch goal)

---

## Stretch Goals (Post v1.0)

> Nice-to-haves, experimental features, and long-term vision items

### Experimental
- [ ] Location sharing (share GPS coordinates in chat, render on map)
- [ ] Live location sharing (continuous, time-limited)
- [ ] Confetti / special effects on messages (party popper, fireworks — a la Spacebar POGGERS)
- [ ] Reaction animations (super reactions with particle effects)
- [ ] AI-powered message summarization ("summarize last 100 messages")
- [ ] Voice channel transcription (speech-to-text via Whisper, opt-in)
- [ ] Collaborative whiteboard widget
- [ ] In-app video/screen recording (Discord Clips equivalent)
- [ ] Code snippet sharing with syntax highlighting and "Run" button (sandboxed execution)
- [ ] Kanban board channel type (project management within a guild)

### Activities & Games
- [ ] Embedded Activities framework (iframe-based apps in voice/text channels)
- [ ] Activity SDK for developers to build custom activities
- [ ] Built-in activities: Watch Together (YouTube sync), Music listening party
- [ ] Mini-games: Trivia, Tic-Tac-Toe, Chess, Drawing

### Social & Growth
- [ ] Server insights/analytics dashboard (member growth, message volume, peak hours)
- [ ] Server boost/support system (members can "boost" a server for perks)
- [ ] Vanity URL marketplace (claim custom invite URLs)
- [ ] User achievement/badge system (earned through activity)
- [ ] Leveling/XP system (configurable per guild, role rewards at levels)
- [ ] Starboard — automatically repost messages with N+ star reactions to a showcase channel
- [ ] Welcome message automation (customizable welcome message when user joins)
- [ ] Auto-role assignment (assign roles automatically on join, based on rules)

### Interoperability
- [ ] ActivityPub integration (follow/post to Mastodon, Lemmy, etc.)
- [ ] RSS feed channels (subscribe to RSS feeds, auto-post new items)
- [ ] Calendar integration (sync events with Google Calendar, CalDAV)
- [ ] Email-to-channel gateway (receive emails as messages in a channel)
- [ ] SMS bridge (send/receive SMS via channel)
- [ ] Telegram bridge adapter
- [ ] Slack bridge adapter
- [ ] IRC bridge adapter

### Self-Hosting Excellence
- [ ] Web-based setup wizard (first-run configuration via browser)
- [ ] Auto-update mechanism (check for new versions, one-click update)
- [ ] Instance health monitoring dashboard (no external tools needed)
- [ ] Storage usage dashboard (S3 bucket size, media breakdown by type)
- [ ] Data retention policies (auto-delete messages older than X days, configurable per channel)
- [ ] Custom domain support per guild (guild.example.com aliases)
- [ ] Backup scheduling (automated daily/weekly backups with configurable retention)
