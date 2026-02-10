# AmityVox TODO

## Phase 2 (v0.2.0) — COMPLETE
- [x] LiveKit voice/video integration (token gen, room management, voice state)
- [x] Media processing (thumbnails 128/256/512, blurhash, EXIF strip, S3 upload)
- [x] WebAuthn/FIDO2 authentication (registration + login, DragonflyDB sessions)
- [x] Federation sync protocol (HLC ordering, signed delivery, inbox handler, retry queue)
- [x] MLS encryption delivery service (key packages, welcome messages, group state, commits)

## Phase 3 (v0.3.0) — In Progress
- [x] Docker Compose deployment (Dockerfile + docker-compose.yml + Caddyfile)
- [x] Video transcoding worker (ffmpeg/ffprobe dispatch, NATS job queue)
- [x] Embed unfurling worker (OpenGraph metadata extraction, link previews)
- [x] AutoMod system (word/regex/invite/mention/caps/spam/link filters, configurable rules, CRUD API)
- [ ] Push notifications (WebPush/FCM integration)
- [ ] Matrix bridge adapter (bridges/matrix/)
- [ ] Discord bridge adapter (bridges/discord/)
- [ ] Integration tests with dockertest (PostgreSQL, NATS, DragonflyDB)
- [ ] SvelteKit frontend scaffold (web/ directory)
