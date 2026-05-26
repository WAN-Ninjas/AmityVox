# API Reference

This reference was checked against `internal/api/server.go`, `cmd/amityvox/main.go`, `internal/gateway/gateway.go`, and shared response helpers on 2026-05-26.

## Conventions

Base REST path:

```text
/api/v1
```

Most authenticated API calls require:

```http
Authorization: Bearer <session-token>
```

Standard success envelope:

```json
{ "data": {} }
```

Standard error envelope:

```json
{ "error": { "code": "string", "message": "string" } }
```

Raw JSON, binary/file bodies, no-content responses, protocol responses, and health/metrics responses can differ from the standard envelope.

Path parameters use `{name}`. Query parameters are handler-specific unless listed in code comments or handler structs.

## Public System Endpoints

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/health` | Basic process/API health |
| `GET` | `/health/deep` | Dependency health for database, NATS, cache, storage, search, voice, and runtime |
| `GET` | `/metrics` | Prometheus metrics when enabled |
| `GET` | `/api/v1/client-config` | Public client configuration |

## Auth and MFA

| Method | Path |
|---|---|
| `GET` | `/api/v1/auth/registration` |
| `POST` | `/api/v1/auth/register` |
| `POST` | `/api/v1/auth/login` |
| `POST` | `/api/v1/auth/logout` |
| `POST` | `/api/v1/auth/password` |
| `POST` | `/api/v1/auth/email` |
| `POST` | `/api/v1/auth/totp/enable` |
| `POST` | `/api/v1/auth/totp/verify` |
| `DELETE` | `/api/v1/auth/totp` |
| `POST` | `/api/v1/auth/backup-codes` |
| `POST` | `/api/v1/auth/backup-codes/verify` |
| `POST` | `/api/v1/auth/webauthn/register/begin` |
| `POST` | `/api/v1/auth/webauthn/register/finish` |
| `POST` | `/api/v1/auth/webauthn/login/begin` |
| `POST` | `/api/v1/auth/webauthn/login/finish` |

## Users, Profiles, Relationships, DMs

| Method | Path |
|---|---|
| `GET` | `/api/v1/users/@me` |
| `PATCH` | `/api/v1/users/@me` |
| `DELETE` | `/api/v1/users/@me` |
| `GET` | `/api/v1/users/@me/guilds` |
| `GET` | `/api/v1/users/@me/dms` |
| `GET` | `/api/v1/users/@me/relationships` |
| `GET` | `/api/v1/users/@me/read-state` |
| `GET` | `/api/v1/users/@me/sessions` |
| `DELETE` | `/api/v1/users/@me/sessions/{sessionID}` |
| `GET` | `/api/v1/users/@me/settings` |
| `PATCH` | `/api/v1/users/@me/settings` |
| `GET` | `/api/v1/users/@me/blocked` |
| `GET` | `/api/v1/users/@me/bookmarks` |
| `GET` | `/api/v1/users/@me/bots` |
| `POST` | `/api/v1/users/@me/bots` |
| `GET` | `/api/v1/users/@me/export` |
| `GET` | `/api/v1/users/@me/export-account` |
| `POST` | `/api/v1/users/@me/import-account` |
| `GET` | `/api/v1/users/@me/instance-profiles` |
| `POST` | `/api/v1/users/@me/instance-profiles` |
| `DELETE` | `/api/v1/users/@me/instance-profiles/{profileID}` |
| `PUT` | `/api/v1/users/@me/activity` |
| `GET` | `/api/v1/users/@me/activity` |
| `GET` | `/api/v1/users/@me/hidden-threads` |
| `GET` | `/api/v1/users/@me/emoji` |
| `POST` | `/api/v1/users/@me/emoji` |
| `DELETE` | `/api/v1/users/@me/emoji/{emojiID}` |
| `GET` | `/api/v1/users/@me/links` |
| `POST` | `/api/v1/users/@me/links` |
| `PATCH` | `/api/v1/users/@me/links/{linkID}` |
| `DELETE` | `/api/v1/users/@me/links/{linkID}` |
| `GET` | `/api/v1/users/@me/issues` |
| `POST` | `/api/v1/users/@me/group-dms` |
| `PUT` | `/api/v1/users/@me/guild-positions` |
| `GET` | `/api/v1/users/resolve` |
| `GET` | `/api/v1/users/{userID}` |
| `GET` | `/api/v1/users/{userID}/note` |
| `PUT` | `/api/v1/users/{userID}/note` |
| `POST` | `/api/v1/users/{userID}/dm` |
| `PUT` | `/api/v1/users/{userID}/friend` |
| `DELETE` | `/api/v1/users/{userID}/friend` |
| `PUT` | `/api/v1/users/{userID}/block` |
| `PATCH` | `/api/v1/users/{userID}/block` |
| `DELETE` | `/api/v1/users/{userID}/block` |
| `GET` | `/api/v1/users/{userID}/mutual-friends` |
| `GET` | `/api/v1/users/{userID}/mutual-guilds` |
| `GET` | `/api/v1/users/{userID}/badges` |
| `GET` | `/api/v1/users/{userID}/links` |
| `POST` | `/api/v1/users/{userID}/report` |

## Guilds

| Method | Path |
|---|---|
| `POST` | `/api/v1/guilds/` |
| `GET` | `/api/v1/guilds/discover` |
| `GET` | `/api/v1/guilds/vanity/{code}` |
| `GET` | `/api/v1/guilds/{guildID}/preview` |
| `POST` | `/api/v1/guilds/{guildID}/join` |
| `GET` | `/api/v1/guilds/{guildID}` |
| `PATCH` | `/api/v1/guilds/{guildID}` |
| `DELETE` | `/api/v1/guilds/{guildID}` |
| `POST` | `/api/v1/guilds/{guildID}/leave` |
| `POST` | `/api/v1/guilds/{guildID}/transfer` |
| `GET` | `/api/v1/guilds/{guildID}/channels` |
| `PATCH` | `/api/v1/guilds/{guildID}/channels` |
| `POST` | `/api/v1/guilds/{guildID}/channels` |
| `POST` | `/api/v1/guilds/{guildID}/channels/{channelID}/clone` |
| `GET` | `/api/v1/guilds/{guildID}/guide` |
| `PUT` | `/api/v1/guilds/{guildID}/guide` |
| `GET` | `/api/v1/guilds/{guildID}/bump` |
| `POST` | `/api/v1/guilds/{guildID}/bump` |
| `GET` | `/api/v1/guilds/{guildID}/members/@me/permissions` |
| `GET` | `/api/v1/guilds/{guildID}/members` |
| `GET` | `/api/v1/guilds/{guildID}/members/search` |
| `GET` | `/api/v1/guilds/{guildID}/members/{memberID}` |
| `PATCH` | `/api/v1/guilds/{guildID}/members/{memberID}` |
| `DELETE` | `/api/v1/guilds/{guildID}/members/{memberID}` |
| `POST` | `/api/v1/guilds/{guildID}/members/{memberID}/warn` |
| `GET` | `/api/v1/guilds/{guildID}/members/{memberID}/warnings` |
| `GET` | `/api/v1/guilds/{guildID}/members/{memberID}/roles` |
| `PUT` | `/api/v1/guilds/{guildID}/members/{memberID}/roles/{roleID}` |
| `DELETE` | `/api/v1/guilds/{guildID}/members/{memberID}/roles/{roleID}` |
| `GET` | `/api/v1/guilds/{guildID}/prune` |
| `POST` | `/api/v1/guilds/{guildID}/prune` |
| `GET` | `/api/v1/guilds/{guildID}/bans` |
| `PUT` | `/api/v1/guilds/{guildID}/bans/{userID}` |
| `DELETE` | `/api/v1/guilds/{guildID}/bans/{userID}` |
| `GET` | `/api/v1/guilds/{guildID}/roles` |
| `PATCH` | `/api/v1/guilds/{guildID}/roles` |
| `POST` | `/api/v1/guilds/{guildID}/roles` |
| `PATCH` | `/api/v1/guilds/{guildID}/roles/{roleID}` |
| `DELETE` | `/api/v1/guilds/{guildID}/roles/{roleID}` |
| `GET` | `/api/v1/guilds/{guildID}/invites` |
| `POST` | `/api/v1/guilds/{guildID}/invites` |
| `GET` | `/api/v1/guilds/{guildID}/categories` |
| `POST` | `/api/v1/guilds/{guildID}/categories` |
| `PATCH` | `/api/v1/guilds/{guildID}/categories/{categoryID}` |
| `DELETE` | `/api/v1/guilds/{guildID}/categories/{categoryID}` |
| `GET` | `/api/v1/guilds/{guildID}/audit-log` |
| `GET` | `/api/v1/guilds/{guildID}/vanity-url` |
| `PATCH` | `/api/v1/guilds/{guildID}/vanity-url` |
| `GET` | `/api/v1/guilds/{guildID}/features` |
| `PATCH` | `/api/v1/guilds/{guildID}/features/{featureKey}` |

## Guild Extensions

| Area | Endpoints |
|---|---|
| Guild templates | `POST/GET /guilds/{guildID}/templates/`, `GET/DELETE /guilds/{guildID}/templates/{templateID}`, `POST /guilds/{guildID}/templates/{templateID}/apply` |
| Channel templates | `GET/POST /guilds/{guildID}/channel-templates`, `DELETE /guilds/{guildID}/channel-templates/{templateID}`, `POST /guilds/{guildID}/channel-templates/{templateID}/apply` |
| Plugins | `GET/POST /guilds/{guildID}/plugins`, `PATCH/DELETE /guilds/{guildID}/plugins/{installID}` |
| Emoji | `GET/POST /guilds/{guildID}/emoji`, `PATCH/DELETE /guilds/{guildID}/emoji/{emojiID}` |
| Webhooks | `GET/POST /guilds/{guildID}/webhooks`, `PATCH/DELETE /guilds/{guildID}/webhooks/{webhookID}`, `GET /guilds/{guildID}/webhooks/{webhookID}/logs` |
| Widget | `GET/PATCH /guilds/{guildID}/widget`, `GET /guilds/{guildID}/widget.json` |
| Soundboard | `GET/PATCH /guilds/{guildID}/soundboard/config`, `GET/POST /guilds/{guildID}/soundboard/sounds`, `DELETE/POST /guilds/{guildID}/soundboard/sounds/{soundID}` |
| Reports/warnings | `DELETE /guilds/{guildID}/warnings/{warningID}`, `GET /guilds/{guildID}/reports`, `PATCH /guilds/{guildID}/reports/{reportID}` |
| Raid config | `GET/PATCH /guilds/{guildID}/raid-config` |
| Ban lists | `GET/POST /guilds/{guildID}/ban-lists/`, `DELETE /guilds/{guildID}/ban-lists/{listID}`, entries/export/import/subscriptions under `/ban-lists` |
| Stickers | `GET/POST /guilds/{guildID}/sticker-packs/`, pack sticker list/add/delete |
| Onboarding | `GET/PUT /guilds/{guildID}/onboarding/`, prompt CRUD, complete, status |
| Events | `GET/POST /guilds/{guildID}/events/`, event get/update/delete, RSVP, RSVP list |
| Retention | `GET/POST /guilds/{guildID}/retention/`, `PATCH/DELETE /guilds/{guildID}/retention/{policyID}` |
| Channel groups | `GET/POST /guilds/{guildID}/channel-groups/`, update/delete, set/remove channels |
| Gallery/media tags | `GET /guilds/{guildID}/gallery`, `GET/POST /guilds/{guildID}/media-tags`, `DELETE /guilds/{guildID}/media-tags/{tagID}` |
| AutoMod | `GET/POST /guilds/{guildID}/automod/rules`, test/get/update/delete rule, `GET /automod/actions` |

## Channels and Messages

| Method | Path |
|---|---|
| `GET` | `/api/v1/channels/{channelID}` |
| `PATCH` | `/api/v1/channels/{channelID}` |
| `DELETE` | `/api/v1/channels/{channelID}` |
| `GET` | `/api/v1/channels/{channelID}/messages` |
| `POST` | `/api/v1/channels/{channelID}/messages` |
| `POST` | `/api/v1/channels/{channelID}/messages/bulk-delete` |
| `GET` | `/api/v1/channels/{channelID}/messages/{messageID}` |
| `PATCH` | `/api/v1/channels/{channelID}/messages/{messageID}` |
| `DELETE` | `/api/v1/channels/{channelID}/messages/{messageID}` |
| `GET` | `/api/v1/channels/{channelID}/messages/{messageID}/edits` |
| `POST` | `/api/v1/channels/{channelID}/messages/{messageID}/crosspost` |
| `POST` | `/api/v1/channels/{channelID}/messages/{messageID}/components/{componentID}/interact` |
| `GET` | `/api/v1/channels/{channelID}/messages/{messageID}/reactions` |
| `PUT` | `/api/v1/channels/{channelID}/messages/{messageID}/reactions/{emoji}` |
| `DELETE` | `/api/v1/channels/{channelID}/messages/{messageID}/reactions/{emoji}` |
| `DELETE` | `/api/v1/channels/{channelID}/messages/{messageID}/reactions/{emoji}/{targetUserID}` |
| `GET` | `/api/v1/channels/{channelID}/pins` |
| `PUT` | `/api/v1/channels/{channelID}/pins/{messageID}` |
| `DELETE` | `/api/v1/channels/{channelID}/pins/{messageID}` |
| `POST` | `/api/v1/channels/{channelID}/typing` |
| `POST` | `/api/v1/channels/{channelID}/decrypt-messages` |
| `POST` | `/api/v1/channels/{channelID}/ack` |
| `PUT` | `/api/v1/channels/{channelID}/permissions/{overrideID}` |
| `DELETE` | `/api/v1/channels/{channelID}/permissions/{overrideID}` |
| `POST` | `/api/v1/channels/{channelID}/messages/{messageID}/threads` |
| `POST` | `/api/v1/channels/{channelID}/messages/{messageID}/report` |
| `POST` | `/api/v1/channels/{channelID}/messages/{messageID}/report-admin` |
| `POST` | `/api/v1/channels/{channelID}/messages/{messageID}/translate` |
| `GET` | `/api/v1/channels/{channelID}/threads` |
| `POST` | `/api/v1/channels/{channelID}/threads/{threadID}/hide` |
| `DELETE` | `/api/v1/channels/{channelID}/threads/{threadID}/hide` |
| `POST` | `/api/v1/channels/{channelID}/lock` |
| `POST` | `/api/v1/channels/{channelID}/unlock` |
| `GET` | `/api/v1/channels/{channelID}/webhooks` |
| `GET` | `/api/v1/channels/{channelID}/export` |
| `PUT` | `/api/v1/messages/{messageID}/bookmark` |
| `DELETE` | `/api/v1/messages/{messageID}/bookmark` |

## Channel Extensions

| Area | Endpoints |
|---|---|
| Forum posts/tags | `GET/POST /channels/{channelID}/tags`, tag update/delete, `GET/POST /channels/{channelID}/posts`, post pin/close |
| Gallery | `GET /channels/{channelID}/gallery`, gallery tags CRUD, gallery posts create/list/pin/close |
| Templates | `GET/POST /channels/{channelID}/templates/`, delete/apply template |
| Emoji | `GET/POST /channels/{channelID}/emoji`, `DELETE /channels/{channelID}/emoji/{emojiID}` |
| Announcement followers | `POST/GET /channels/{channelID}/followers`, `DELETE /followers/{followerID}`, `POST /messages/{messageID}/publish` |
| Scheduled messages | `POST/GET /channels/{channelID}/scheduled-messages`, `DELETE /scheduled-messages/{messageID}` |
| Group DM recipients | `PUT/DELETE /channels/{channelID}/recipients/{userID}` |
| Polls | `POST /polls`, `GET /polls/{pollID}`, vote/close/delete |
| Widgets | `GET/POST /channels/{channelID}/widgets/`, `PATCH/DELETE /widgets/{widgetID}` |
| Experimental | Location sharing, message effects, super reactions, summaries, transcription, whiteboards, code snippets, recordings, kanban under `/channels/{channelID}/experimental/*` |

## Voice

| Method | Path |
|---|---|
| `POST` | `/api/v1/voice/{channelID}/join` |
| `POST` | `/api/v1/voice/{channelID}/leave` |
| `GET` | `/api/v1/voice/{channelID}/states` |
| `POST` | `/api/v1/voice/{channelID}/members/{userID}/mute` |
| `POST` | `/api/v1/voice/{channelID}/members/{userID}/deafen` |
| `POST` | `/api/v1/voice/{channelID}/members/{userID}/move` |
| `GET` | `/api/v1/voice/preferences` |
| `PATCH` | `/api/v1/voice/preferences` |
| `POST` | `/api/v1/voice/{channelID}/input-mode` |
| `POST` | `/api/v1/voice/{channelID}/priority-speaker` |
| `POST` | `/api/v1/voice/{channelID}/members/{userID}/priority` |
| `GET/POST/DELETE` | `/api/v1/voice/{channelID}/soundboard` and `/soundboard/{soundID}` |
| `GET/PATCH` | `/api/v1/voice/{channelID}/soundboard/config` |
| `POST/DELETE/GET` | `/api/v1/voice/{channelID}/broadcast` |
| `POST/DELETE/PATCH` | `/api/v1/voice/{channelID}/screen-share` |
| `GET` | `/api/v1/voice/{channelID}/screen-shares` |

## Moderation and Issues

| Method | Path |
|---|---|
| `POST` | `/api/v1/issues` |
| `GET` | `/api/v1/moderation/stats` |
| `GET` | `/api/v1/moderation/user-reports` |
| `PATCH` | `/api/v1/moderation/user-reports/{reportID}` |
| `GET` | `/api/v1/moderation/message-reports` |
| `PATCH` | `/api/v1/moderation/message-reports/{reportID}` |
| `GET` | `/api/v1/moderation/issues/export` |
| `GET` | `/api/v1/moderation/issues/tokens` |
| `POST` | `/api/v1/moderation/issues/tokens` |
| `DELETE` | `/api/v1/moderation/issues/tokens/{tokenID}` |
| `GET` | `/api/v1/moderation/issues` |
| `PATCH` | `/api/v1/moderation/issues/{issueID}` |
| `GET` | `/api/v1/support/issues` |
| `PATCH` | `/api/v1/support/issues/{issueID}` |
| `GET` | `/api/v1/ban-lists/public` |

The support routes are intended for external maintainers/tools using a time-limited issue access token. The moderation routes are for local authenticated users with appropriate privileges.

## Bots, Webhooks, Stickers, Themes, Plugins

| Area | Endpoints |
|---|---|
| Bots | `/api/v1/bots/{botID}`, tokens, commands, guild permissions, presence, rate limit, subscriptions |
| Webhook tools | `GET /webhooks/templates`, `POST /webhooks/preview`, `GET /webhooks/outgoing-events` |
| Webhook execution | `POST /api/v1/webhooks/{webhookID}/{token}` |
| Stickers | User packs, pack share/unshare, shared pack get/clone |
| Themes | `GET/POST /themes/`, get by share code, like/unlike/delete |
| Widgets | Guild widget get/update and channel widget CRUD |
| Plugins | List/get/install plugins plus guild install update/delete |
| Key backup | `POST/GET/DELETE /encryption/key-backup/`, download, recovery codes |

## Activities and Social Features

| Area | Endpoints |
|---|---|
| Activities | `GET/POST /activities/`, get/rate activity, session start/active/join/leave/end/state |
| Games | `POST /games/`, join, move, get session, leaderboard |
| Watch together | `POST /watch-together/`, sync |
| Music party | `POST /music-party/`, queue |
| Insights/boosts | Guild insights and boosts |
| Vanity | Claim, release, check availability |
| Achievements | Global achievements, user achievements, check user achievements |
| Leveling | Guild leveling config, level roles, leaderboard, member XP |
| Starboard | Config and entries |
| Welcome | Guild welcome config |
| Auto roles | Guild auto-role CRUD |

## Media, Search, Notifications, Invites, Encryption

| Area | Endpoints |
|---|---|
| Invites | `GET/POST/DELETE /api/v1/invites/{code}` |
| Files | `POST /api/v1/files/upload`, `GET /api/v1/files/{fileID}`, patch/delete file, tag/untag |
| Federation media proxy | `GET /api/v1/federation/media/{instanceId}/{fileId}` |
| Encryption | Key packages, claim/delete packages, channel welcome, welcome ack/list, group state, commits |
| Notifications | List/update/delete, mark-all-read, clear, search, unread count, type preferences, global/channel preferences, VAPID key, push subscriptions |
| Search | `GET /api/v1/search/messages`, `/search/users`, `/search/guilds` |
| Giphy | `GET /api/v1/giphy/search`, `/giphy/trending`, `/giphy/categories` |
| Announcements | `GET /api/v1/announcements` |

## Admin API

All `/api/v1/admin` routes are instance administration routes, except first-run setup status/completion which are mounted in the admin group but have setup-specific behavior.

| Area | Endpoints |
|---|---|
| Setup | `GET /admin/setup/status`, `POST /admin/setup/complete` |
| Instance | `GET/PATCH /admin/instance`, `GET /admin/stats` |
| Users | `GET /admin/users`, suspend/unsuspend, set-admin, set-globalmod, instance-ban/unban, user guild list |
| Guilds | `GET /admin/guilds`, `GET/DELETE /admin/guilds/{guildID}` |
| Registration | `GET/PATCH /admin/registration`, registration token create/list/delete |
| Features | `GET /admin/features`, `PATCH /admin/features/{featureKey}` |
| Transcription | `GET/PATCH /admin/transcription` |
| Announcements | Create/list/update/delete |
| Reports | `GET /admin/reports` |
| Bots | `GET /admin/bots` |
| Rate limits | Stats, log, config update |
| Content scan | Rule CRUD and scan log |
| Captcha | `GET/PATCH /admin/captcha` |
| Federation | Peers, peer controls, approvals/rejections, dashboard, key audit, delivery receipts, search config, protocol config, blocklist, allowlist, profiles, federated user profile |
| Updates | Check, set latest, dismiss, get/update update config |
| Health/storage | Health dashboard/history, storage dashboard |
| Retention | Policy CRUD and run |
| Domains | Custom domain list/create/verify/delete |
| Backups | Schedule CRUD, history, trigger |
| Bridges | Bridge CRUD, mappings, virtual users |
| Media | Admin media list/delete |

## Federation API

Peer-to-peer routes are mounted outside `/api/v1`.

| Method | Path |
|---|---|
| `GET` | `/.well-known/amityvox` |
| `POST` | `/federation/v1/handshake` |
| `POST` | `/federation/v1/inbox` |
| `POST` | `/federation/v1/sync` |
| `GET` | `/federation/v1/users/lookup` |
| `POST` | `/federation/v1/users/{userID}/profile` |
| `POST` | `/federation/v1/dm/create` |
| `POST` | `/federation/v1/dm/message` |
| `POST` | `/federation/v1/dm/message/update` |
| `POST` | `/federation/v1/dm/message/delete` |
| `POST` | `/federation/v1/dm/reaction/add` |
| `POST` | `/federation/v1/dm/reaction/remove` |
| `POST` | `/federation/v1/dm/recipient-add` |
| `POST` | `/federation/v1/dm/recipient-remove` |
| `GET` | `/federation/v1/guilds/{guildID}/preview` |
| `POST` | `/federation/v1/guilds/{guildID}/join` |
| `POST` | `/federation/v1/guilds/{guildID}/leave` |
| `POST` | `/federation/v1/guilds/invite-accept` |
| `POST` | `/federation/v1/guilds/{guildID}/channels/{channelID}/messages` |
| `POST` | `/federation/v1/guilds/{guildID}/channels/{channelID}/messages/create` |
| `POST` | `/federation/v1/guilds/{guildID}/members` |
| `POST` | `/federation/v1/guilds/{guildID}/channels/{channelID}/messages/{messageID}/reactions` |
| `POST` | `/federation/v1/guilds/{guildID}/channels/{channelID}/messages/{messageID}/reactions/remove` |
| `POST` | `/federation/v1/guilds/{guildID}/channels/{channelID}/typing` |
| `POST` | `/federation/v1/guilds/{guildID}/manage` |
| `GET` | `/federation/v1/invites/{code}` |
| `POST` | `/federation/v1/invites/{code}/accept` |
| `POST` | `/federation/v1/guilds/discover` |
| `POST` | `/federation/v1/voice/token` |
| `GET` | `/federation/v1/guilds/{guildID}/channels/{channelID}/mls/key-packages/{userID}` |
| `POST` | `/federation/v1/guilds/{guildID}/channels/{channelID}/mls/key-packages/{userID}/claim` |
| `POST` | `/federation/v1/guilds/{guildID}/channels/{channelID}/mls/welcome` |
| `POST` | `/federation/v1/guilds/{guildID}/channels/{channelID}/mls/commits` |
| `GET` | `/federation/v1/guilds/{guildID}/channels/{channelID}/mls/group-state` |
| `GET` | `/federation/v1/guilds/{guildID}/channels/{channelID}/mls/commits` |

Authenticated local federation proxy routes:

| Area | Endpoints |
|---|---|
| Remote guilds | `POST /api/v1/federation/guilds/join`, leave, messages get/post, members, reactions, typing |
| Remote users | `POST /api/v1/federation/users/ensure`, `GET /api/v1/federation/users/{instanceID}/{userID}/profile` |
| Peers/discovery | `GET /api/v1/federation/peers/public`, peer guild discovery, aggregated discover |
| Invites | `POST /api/v1/federation/invites/resolve` |
| Voice | `POST /api/v1/federation/voice/join`, `POST /api/v1/federation/voice/guild-join` |

## WebSocket Gateway

Gateway URL:

```text
/ws
```

Wire format:

```json
{ "op": 2, "d": { "token": "session-token" } }
```

Opcodes:

| Op | Name |
|---|---|
| 0 | Dispatch |
| 1 | Heartbeat |
| 2 | Identify |
| 3 | Presence update |
| 4 | Voice state update |
| 5 | Resume |
| 6 | Reconnect |
| 7 | Request members |
| 8 | Typing |
| 9 | Subscribe |
| 10 | Hello |
| 11 | Heartbeat ACK |

The server sends HELLO first. Clients identify with a session token, send heartbeats at the advertised interval, and receive dispatch events with sequence numbers.
