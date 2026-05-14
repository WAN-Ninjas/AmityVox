# AmityVox Project Changelog

All significant changes to AmityVox are recorded here. Format follows [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).

---

## [Unreleased]

### Changed — Documentation Cleanup (2026-05-14)

- Promoted `docs/current-work-backlog.md` as the active cleanup checklist.
- Archived stale dated plans under `docs/archive/plans/`.
- Archived the previous long-form usability backlog under `docs/archive/current-code-usability-backlog-2026-05-14.md`.
- Left pointer files in `docs/current-code-usability-backlog.md` and `docs/plans/README.md`.

### Changed — Frontend API Boundary Cleanup (2026-05-14)

- Removed remaining component-level `api.request` calls from whiteboard, guild insights, welcome settings, starboard settings, message effects, and voice transcription surfaces.
- Added named API client methods and shared response types for those feature surfaces.
- Verified `rg "api\\.request" web/src` is clean outside the API client and `npm run check` reports 0 errors / 0 warnings.

### Fixed — Federation Local Ownership Checks (2026-05-14)

- Updated MLS federation local guild and receiver-user checks to accept rows owned by the local instance ID.
- Updated federated-guild leave cleanup so one local user leaving does not delete a mirrored remote guild while other local members remain.
- Updated aggregated federation discover so locally owned discoverable guilds appear when `guilds.instance_id` is the local instance ID.
- Kept NULL compatibility for older rows while aligning behavior to the current schema.
- Made federated guild join mirror writes transactional and populated mirrored channel, role, and membership `instance_id` values.
- Expanded federated guild join snapshots to include richer channel and role fields.
- Persisted inbound federated messages with richer message fields, attachments, embeds, and canonical `reactions` rows.
- Rejected malformed channel/guild/message/reaction federation envelopes that omit `guild_id`.
- Recorded host-side outbound federation events with HLC timestamps for replay/backfill.

### Fixed — Frontend Reconnect And Media UX (2026-05-14)

- Added loaded-channel message backfill after gateway reconnect before normal active-channel refresh.
- Added local-instance-aware media URL helpers and a backend local-media redirect for stale clients.
- Added shared client-config loading and experimental feature flags; always-visible translation controls are hidden unless enabled.
- Added API error message extraction and wired `createAsyncOp` plus plugin marketplace loading/install paths through it.
- Refreshed the active channel on pin updates and stopped channel widget events from falling through to activity handlers.
- Added focused tests for message backfill, media URL selection, API error extraction, and federation `guild_id` requirements.
- Moved remaining large Svelte file tracking to `docs/large-svelte-file-inventory.md` so the active bug backlog is finite.

### Changed — Federation Remediation (2026-02-23)

**Config** (`amityvox.example.toml` / `[federation]` section)

Five new tuning fields added to `[federation]` with documented defaults:

| Key | Default | Description |
|---|---|---|
| `peer_inbox_limit` | `20` | Max inbound federation messages processed per peer per cycle |
| `delivery_concurrency` | `50` | Parallel goroutines for outbound event delivery |
| `media_cache_dir` | `/tmp/amityvox-media-cache` | Disk cache directory for federated media > 1 MB |
| `media_cache_max_size_mb` | `1024` | Max disk usage for on-disk media LRU cache (MB) |
| `backfill_window_days` | `7` | Max days of events replayed on backfill sync |

**Media Proxy** (`GET /api/v1/federation/media/{instanceId}/{fileId}`)

- Files ≤ 1 MB are cached in DragonflyDB (key `fed:media:{instanceId}:{fileId}`, TTL 1 hour).
- Files > 1 MB are cached to disk under `media_cache_dir` using LRU eviction bounded by `media_cache_max_size_mb`.

**Database Migrations**

| # | Name | Change |
|---|---|---|
| 065 | `rename_federation_channel_mirrors` | Renamed `federation_channel_mirrors` → `federation_dm_channel_map` |
| 066 | `add_instance_id_to_content_tables` | Added `instance_id` (FK → `instances.id`, nullable) to: `channels`, `roles`, `guild_members`, `messages`, `attachments`, `embeds`, `reactions`, `pins`, `webhooks`; backfilled from parent guild; added partial indexes |

**Federation Backfill**

- `HandleSync` now enforces the 7-day backfill window configured via `backfill_window_days`.
- A federation events retention worker runs hourly and deletes `federation_events` rows older than the window.

**Reliability**

- Silent `Exec()` calls in the federation layer now log a `WARN` on failure instead of discarding the error.

---

## [1.0.0] — Federation Full Parity baseline

- See `docs/plans/2026-02-21-federation-design.md` for the full federation parity design.
- See `docs/plans/2026-02-21-federation-implementation.md` for the 26-task implementation plan.
