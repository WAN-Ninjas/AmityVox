# AmityVox TODO

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
- [ ] Full SvelteKit UI polish (animations, notifications panel, search UI, mobile responsive)
- [ ] Admin dashboard with live stats (connected via REST /admin/stats)
- [ ] Desktop client (Tauri wrapper)
- [ ] Forum/stage channel UI
- [ ] Threads UI
- [ ] E2E encryption UI (MLS key exchange in client)
- [ ] Plugin/WASM system
- [ ] Mobile clients (Capacitor or native)
