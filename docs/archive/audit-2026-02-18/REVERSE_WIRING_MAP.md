# Reverse Wiring Map - AmityVox Pre-Release Audit Phase 3B

Generated: 2026-02-18

---

## Part 1: Orphaned Backend Endpoints

Routes extracted from `internal/api/server.go` and `cmd/amityvox/main.go` (federation proxy routes).
Frontend consumer checked against `web/src/lib/api/client.ts`.

### Classification Key

- **ORPHANED** - Should have a frontend consumer but does not
- **INTERNAL-ONLY** - Legitimately has no frontend (health, metrics, webhook execute, file serve, federation server-to-server)
- **BOT-API-ONLY** - Only used by bot SDK, no frontend consumer expected

### Non-Versioned / Infrastructure Routes

| Method | Path | Handler | Classification | Notes |
|--------|------|---------|---------------|-------|
| GET | `/health` | `handleHealthCheck` | INTERNAL-ONLY | Docker healthcheck endpoint |
| GET | `/health/deep` | `handleDeepHealthCheck` | INTERNAL-ONLY | Deep health probe |
| GET | `/metrics` | `handleMetrics` | INTERNAL-ONLY | Prometheus metrics scraping |
| POST | `/webhooks/{webhookID}/{token}` | `webhookH.HandleExecute` | INTERNAL-ONLY | External webhook execution (no auth needed) |
| GET | `/files/{fileID}` | `Media.HandleGetFile` | INTERNAL-ONLY | Direct file serving via URL (browser/img tag, not API client) |
| GET | `/guilds/{guildID}/widget.json` | `widgetH.HandleGetGuildWidgetEmbed` | INTERNAL-ONLY | Public embed widget (consumed by external sites) |

### Federation Server-to-Server Routes (not consumed by frontend)

| Method | Path | Handler | Classification | Notes |
|--------|------|---------|---------------|-------|
| POST | `/federation/v1/message` | `syncSvc.HandleFederatedMessage` | INTERNAL-ONLY | Instance-to-instance message relay |
| POST | `/federation/v1/dm/create` | `syncSvc.HandleFederatedDMCreate` | INTERNAL-ONLY | Instance-to-instance DM creation |
| POST | `/federation/v1/dm/message` | `syncSvc.HandleFederatedDMMessage` | INTERNAL-ONLY | Instance-to-instance DM message relay |
| POST | `/federation/v1/dm/recipient-add` | `syncSvc.HandleFederatedDMRecipientAdd` | INTERNAL-ONLY | Instance-to-instance group DM add |
| POST | `/federation/v1/dm/recipient-remove` | `syncSvc.HandleFederatedDMRecipientRemove` | INTERNAL-ONLY | Instance-to-instance group DM remove |
| GET | `/federation/v1/guilds/{guildID}/preview` | `syncSvc.HandleFederatedGuildPreview` | INTERNAL-ONLY | Instance-to-instance guild preview |
| POST | `/federation/v1/guilds/{guildID}/join` | `syncSvc.HandleFederatedGuildJoin` | INTERNAL-ONLY | Instance-to-instance guild join |
| POST | `/federation/v1/guilds/{guildID}/leave` | `syncSvc.HandleFederatedGuildLeave` | INTERNAL-ONLY | Instance-to-instance guild leave |
| POST | `/federation/v1/guilds/invite-accept` | `syncSvc.HandleFederatedGuildInviteAccept` | INTERNAL-ONLY | Instance-to-instance invite accept |
| POST | `/federation/v1/guilds/{guildID}/channels/{channelID}/messages` | `syncSvc.HandleFederatedGuildMessages` | INTERNAL-ONLY | Instance-to-instance message fetch |
| POST | `/federation/v1/guilds/{guildID}/channels/{channelID}/messages/create` | `syncSvc.HandleFederatedGuildPostMessage` | INTERNAL-ONLY | Instance-to-instance message post |
| POST | `/federation/v1/voice/token` | `syncSvc.HandleFederatedVoiceToken` | INTERNAL-ONLY | Instance-to-instance voice token |

### Auth Routes

| Method | Path | Handler | Classification | Notes |
|--------|------|---------|---------------|-------|
| POST | `/api/v1/auth/register` | `handleRegister` | OK | `api.register()` |
| POST | `/api/v1/auth/login` | `handleLogin` | OK | `api.login()` |
| POST | `/api/v1/auth/logout` | `handleLogout` | OK | `api.logout()` |
| POST | `/api/v1/auth/password` | `handleChangePassword` | OK | `api.changePassword()` |
| POST | `/api/v1/auth/email` | `handleChangeEmail` | ORPHANED | No `changeEmail()` in client.ts. Settings page should expose this. |
| POST | `/api/v1/auth/totp/enable` | `handleTOTPEnable` | OK | `api.enableTOTP()` |
| POST | `/api/v1/auth/totp/verify` | `handleTOTPVerify` | OK | `api.verifyTOTP()` |
| DELETE | `/api/v1/auth/totp` | `handleTOTPDisable` | OK | `api.disableTOTP()` |
| POST | `/api/v1/auth/backup-codes` | `handleGenerateBackupCodes` | OK | `api.generateBackupCodes()` |
| POST | `/api/v1/auth/backup-codes/verify` | `handleConsumeBackupCode` | ORPHANED | No `consumeBackupCode()` in client.ts. Login flow needs this for 2FA recovery. |
| POST | `/api/v1/auth/webauthn/register/begin` | `handleWebAuthnRegisterBegin` | ORPHANED | No WebAuthn methods in client.ts at all |
| POST | `/api/v1/auth/webauthn/register/finish` | `handleWebAuthnRegisterFinish` | ORPHANED | No WebAuthn methods in client.ts |
| POST | `/api/v1/auth/webauthn/login/begin` | `handleWebAuthnLoginBegin` | ORPHANED | No WebAuthn methods in client.ts |
| POST | `/api/v1/auth/webauthn/login/finish` | `handleWebAuthnLoginFinish` | ORPHANED | No WebAuthn methods in client.ts |

### User Routes

| Method | Path | Handler | Classification | Notes |
|--------|------|---------|---------------|-------|
| GET | `/api/v1/users/@me` | `HandleGetSelf` | OK | `api.getMe()` |
| PATCH | `/api/v1/users/@me` | `HandleUpdateSelf` | OK | `api.updateMe()` |
| DELETE | `/api/v1/users/@me` | `HandleDeleteSelf` | ORPHANED | No `deleteAccount()` in client.ts |
| GET | `/api/v1/users/@me/guilds` | `HandleGetSelfGuilds` | OK | `api.getMyGuilds()` |
| GET | `/api/v1/users/@me/dms` | `HandleGetSelfDMs` | OK | `api.getMyDMs()` |
| GET | `/api/v1/users/@me/relationships` | `HandleGetRelationships` | OK | `api.getFriends()` |
| GET | `/api/v1/users/@me/read-state` | `HandleGetSelfReadState` | OK | `api.getReadState()` |
| GET | `/api/v1/users/@me/sessions` | `HandleGetSelfSessions` | OK | `api.getSessions()` |
| DELETE | `/api/v1/users/@me/sessions/{sessionID}` | `HandleDeleteSelfSession` | OK | `api.deleteSession()` |
| GET | `/api/v1/users/@me/settings` | `HandleGetUserSettings` | OK | `api.getUserSettings()` |
| PATCH | `/api/v1/users/@me/settings` | `HandleUpdateUserSettings` | OK | `api.updateUserSettings()` |
| GET | `/api/v1/users/@me/blocked` | `HandleGetBlockedUsers` | OK | `api.getBlockedUsers()` |
| GET | `/api/v1/users/@me/bookmarks` | `HandleListBookmarks` | OK | `api.getBookmarks()` |
| GET | `/api/v1/users/@me/bots` | `HandleListMyBots` | OK | `api.getMyBots()` |
| POST | `/api/v1/users/@me/bots` | `HandleCreateBot` | OK | `api.createBot()` |
| GET | `/api/v1/users/@me/export` | `HandleExportUserData` | ORPHANED | No `exportUserData()` in client.ts |
| GET | `/api/v1/users/@me/export-account` | `HandleExportAccount` | ORPHANED | No `exportAccount()` in client.ts |
| POST | `/api/v1/users/@me/import-account` | `HandleImportAccount` | ORPHANED | No `importAccount()` in client.ts |
| PUT | `/api/v1/users/@me/activity` | `HandleUpdateActivity` | ORPHANED | No `updateActivity()` in client.ts |
| GET | `/api/v1/users/@me/activity` | `HandleGetActivity` | ORPHANED | No `getActivity()` in client.ts |
| GET | `/api/v1/users/@me/hidden-threads` | `HandleGetHiddenThreads` | OK | `api.getHiddenThreads()` |
| GET | `/api/v1/users/@me/emoji` | `HandleGetUserEmoji` | ORPHANED | No `getUserEmoji()` in client.ts |
| POST | `/api/v1/users/@me/emoji` | `HandleCreateUserEmoji` | ORPHANED | No `createUserEmoji()` in client.ts |
| DELETE | `/api/v1/users/@me/emoji/{emojiID}` | `HandleDeleteUserEmoji` | ORPHANED | No `deleteUserEmoji()` in client.ts |
| GET | `/api/v1/users/@me/links` | `HandleGetMyLinks` | OK | `api.getMyLinks()` |
| POST | `/api/v1/users/@me/links` | `HandleCreateLink` | OK | `api.createLink()` |
| PATCH | `/api/v1/users/@me/links/{linkID}` | `HandleUpdateLink` | OK | `api.updateLink()` |
| DELETE | `/api/v1/users/@me/links/{linkID}` | `HandleDeleteLink` | OK | `api.deleteLink()` |
| GET | `/api/v1/users/@me/issues` | `HandleGetMyIssues` | OK | `api.getMyIssues()` |
| POST | `/api/v1/users/@me/group-dms` | `HandleCreateGroupDM` | OK | `api.createGroupDM()` |
| GET | `/api/v1/users/@me/channel-groups` | `HandleGetChannelGroups` | ORPHANED | No `getChannelGroups()` in client.ts |
| POST | `/api/v1/users/@me/channel-groups` | `HandleCreateChannelGroup` | ORPHANED | No `createChannelGroup()` in client.ts |
| PATCH | `/api/v1/users/@me/channel-groups/{groupID}` | `HandleUpdateChannelGroup` | ORPHANED | No frontend consumer |
| DELETE | `/api/v1/users/@me/channel-groups/{groupID}` | `HandleDeleteChannelGroup` | ORPHANED | No frontend consumer |
| PUT | `/api/v1/users/@me/channel-groups/{groupID}/channels/{channelID}` | `HandleAddChannelToGroup` | ORPHANED | No frontend consumer |
| DELETE | `/api/v1/users/@me/channel-groups/{groupID}/channels/{channelID}` | `HandleRemoveChannelFromGroup` | ORPHANED | No frontend consumer |
| GET | `/api/v1/users/resolve` | `HandleResolveHandle` | OK | `api.resolveHandle()` |
| GET | `/api/v1/users/{userID}` | `HandleGetUser` | OK | `api.getUser()` |
| GET | `/api/v1/users/{userID}/note` | `HandleGetUserNote` | OK | `api.getUserNote()` |
| PUT | `/api/v1/users/{userID}/note` | `HandleSetUserNote` | OK | `api.setUserNote()` |
| POST | `/api/v1/users/{userID}/dm` | `HandleCreateDM` | OK | `api.createDM()` |
| PUT | `/api/v1/users/{userID}/friend` | `HandleAddFriend` | OK | `api.addFriend()` |
| DELETE | `/api/v1/users/{userID}/friend` | `HandleRemoveFriend` | OK | `api.removeFriend()` |
| PUT | `/api/v1/users/{userID}/block` | `HandleBlockUser` | OK | `api.blockUser()` |
| PATCH | `/api/v1/users/{userID}/block` | `HandleUpdateBlockLevel` | OK | `api.updateBlockLevel()` |
| DELETE | `/api/v1/users/{userID}/block` | `HandleUnblockUser` | OK | `api.unblockUser()` |
| GET | `/api/v1/users/{userID}/mutual-friends` | `HandleGetMutualFriends` | OK | `api.getMutualFriends()` |
| GET | `/api/v1/users/{userID}/mutual-guilds` | `HandleGetMutualGuilds` | OK | `api.getMutualGuilds()` |
| GET | `/api/v1/users/{userID}/badges` | `HandleGetUserBadges` | OK | `api.getUserBadges()` |
| GET | `/api/v1/users/{userID}/links` | `HandleGetUserLinks` | OK | `api.getUserLinks()` |
| POST | `/api/v1/users/{userID}/report` | `HandleReportUser` | OK | `api.reportUser()` |

### Guild Routes

| Method | Path | Handler | Classification | Notes |
|--------|------|---------|---------------|-------|
| POST | `/api/v1/guilds` | `HandleCreateGuild` | OK | `api.createGuild()` |
| GET | `/api/v1/guilds/discover` | `HandleDiscoverGuilds` | OK | `api.discoverGuilds()` |
| GET | `/api/v1/guilds/vanity/{code}` | `HandleResolveVanityURL` | ORPHANED | No `resolveVanityURL()` in client.ts |
| GET | `/api/v1/guilds/{guildID}/preview` | `HandleGetGuildPreview` | OK | `api.getGuildPreview()` |
| POST | `/api/v1/guilds/{guildID}/join` | `HandleJoinDiscoverableGuild` | OK | `api.joinGuild()` |
| GET | `/api/v1/guilds/{guildID}` | `HandleGetGuild` | OK | `api.getGuild()` |
| PATCH | `/api/v1/guilds/{guildID}` | `HandleUpdateGuild` | OK | `api.updateGuild()` |
| DELETE | `/api/v1/guilds/{guildID}` | `HandleDeleteGuild` | OK | `api.deleteGuild()` |
| POST | `/api/v1/guilds/{guildID}/leave` | `HandleLeaveGuild` | OK | `api.leaveGuild()` |
| POST | `/api/v1/guilds/{guildID}/transfer` | `HandleTransferGuildOwnership` | ORPHANED | No `transferGuildOwnership()` in client.ts |
| GET | `/api/v1/guilds/{guildID}/channels` | `HandleGetGuildChannels` | OK | `api.getGuildChannels()` |
| PATCH | `/api/v1/guilds/{guildID}/channels` | `HandleReorderGuildChannels` | ORPHANED | No `reorderChannels()` in client.ts |
| POST | `/api/v1/guilds/{guildID}/channels` | `HandleCreateGuildChannel` | OK | `api.createChannel()` |
| POST | `/api/v1/guilds/{guildID}/channels/{channelID}/clone` | `HandleCloneChannel` | OK | `api.cloneChannel()` |
| GET | `/api/v1/guilds/{guildID}/guide` | `HandleGetServerGuide` | ORPHANED | No `getServerGuide()` in client.ts |
| PUT | `/api/v1/guilds/{guildID}/guide` | `HandleUpdateServerGuide` | ORPHANED | No `updateServerGuide()` in client.ts |
| GET | `/api/v1/guilds/{guildID}/bump` | `HandleGetBumpStatus` | ORPHANED | No frontend consumer |
| POST | `/api/v1/guilds/{guildID}/bump` | `HandleBumpGuild` | ORPHANED | No frontend consumer |
| POST | `/api/v1/guilds/{guildID}/templates` | `HandleCreateGuildTemplate` | ORPHANED | No frontend consumer |
| GET | `/api/v1/guilds/{guildID}/templates` | `HandleGetGuildTemplates` | ORPHANED | No frontend consumer |
| GET | `/api/v1/guilds/{guildID}/templates/{templateID}` | `HandleGetGuildTemplate` | ORPHANED | No frontend consumer |
| DELETE | `/api/v1/guilds/{guildID}/templates/{templateID}` | `HandleDeleteGuildTemplate` | ORPHANED | No frontend consumer |
| POST | `/api/v1/guilds/{guildID}/templates/{templateID}/apply` | `HandleApplyGuildTemplate` | ORPHANED | No frontend consumer |
| GET | `/api/v1/guilds/{guildID}/members/@me/permissions` | `HandleGetMyPermissions` | OK | `api.getMyPermissions()` |
| GET | `/api/v1/guilds/{guildID}/members` | `HandleGetGuildMembers` | OK | `api.getMembers()` |
| GET | `/api/v1/guilds/{guildID}/members/search` | `HandleSearchGuildMembers` | ORPHANED | No `searchGuildMembers()` in client.ts |
| GET | `/api/v1/guilds/{guildID}/members/{memberID}` | `HandleGetGuildMember` | OK | `api.getMember()` |
| PATCH | `/api/v1/guilds/{guildID}/members/{memberID}` | `HandleUpdateGuildMember` | OK | `api.updateMember()` |
| DELETE | `/api/v1/guilds/{guildID}/members/{memberID}` | `HandleRemoveGuildMember` | OK | `api.kickMember()` |
| GET | `/api/v1/guilds/{guildID}/prune` | `HandleGetGuildPruneCount` | ORPHANED | No frontend consumer |
| POST | `/api/v1/guilds/{guildID}/prune` | `HandleGuildPrune` | ORPHANED | No frontend consumer |
| PATCH | `/api/v1/guilds/{guildID}/emoji/{emojiID}` | `HandleUpdateGuildEmoji` | ORPHANED | No `updateGuildEmoji()` in client.ts |
| GET | `/api/v1/guilds/{guildID}/webhooks/{webhookID}/logs` | `HandleGetWebhookLogs` | ORPHANED | No `getWebhookLogs()` in client.ts |
| GET | `/api/v1/guilds/{guildID}/vanity-url` | `HandleGetGuildVanityURL` | ORPHANED | No frontend consumer |
| PATCH | `/api/v1/guilds/{guildID}/vanity-url` | `HandleSetGuildVanityURL` | ORPHANED | No frontend consumer |

### Channel Routes (selected ORPHANED only)

| Method | Path | Handler | Classification | Notes |
|--------|------|---------|---------------|-------|
| GET | `/api/v1/channels/{channelID}/messages/{messageID}` | `HandleGetMessage` | ORPHANED | No `getMessage()` in client.ts |
| GET | `/api/v1/channels/{channelID}/messages/{messageID}/reactions` | `HandleGetReactions` | ORPHANED | No `getReactions()` in client.ts |
| DELETE | `/api/v1/channels/{channelID}/messages/{messageID}/reactions/{emoji}/{targetUserID}` | `HandleRemoveUserReaction` | ORPHANED | No `removeUserReaction()` |
| PUT | `/api/v1/channels/{channelID}/permissions/{overrideID}` | `HandleSetChannelPermission` | ORPHANED | No `setChannelPermission()` |
| DELETE | `/api/v1/channels/{channelID}/permissions/{overrideID}` | `HandleDeleteChannelPermission` | ORPHANED | No `deleteChannelPermission()` |
| GET | `/api/v1/channels/{channelID}/export` | `HandleExportChannelMessages` | ORPHANED | No `exportChannelMessages()` |
| POST | `/api/v1/channels/{channelID}/messages/{messageID}/publish` | `HandlePublishMessage` | ORPHANED | No `publishMessage()` (announcement publish) |
| POST | `/api/v1/channels/{channelID}/templates` | `HandleCreateChannelTemplate` | ORPHANED | No frontend consumer |
| GET | `/api/v1/channels/{channelID}/templates` | `HandleGetChannelTemplates` | ORPHANED | No frontend consumer |
| DELETE | `/api/v1/channels/{channelID}/templates/{templateID}` | `HandleDeleteChannelTemplate` | ORPHANED | No frontend consumer |
| POST | `/api/v1/channels/{channelID}/templates/{templateID}/apply` | `HandleApplyChannelTemplate` | ORPHANED | No frontend consumer |
| GET | `/api/v1/channels/{channelID}/emoji` | `HandleGetChannelEmoji` | ORPHANED | No frontend consumer |
| POST | `/api/v1/channels/{channelID}/emoji` | `HandleCreateChannelEmoji` | ORPHANED | No frontend consumer |
| DELETE | `/api/v1/channels/{channelID}/emoji/{emojiID}` | `HandleDeleteChannelEmoji` | ORPHANED | No frontend consumer |

### Voice Routes (ALL ORPHANED except join/leave/preferences)

| Method | Path | Handler | Classification | Notes |
|--------|------|---------|---------------|-------|
| GET | `/api/v1/voice/{channelID}/states` | `handleGetVoiceStates` | ORPHANED | No frontend consumer |
| POST | `/api/v1/voice/{channelID}/members/{userID}/mute` | `handleVoiceServerMute` | ORPHANED | No frontend consumer |
| POST | `/api/v1/voice/{channelID}/members/{userID}/deafen` | `handleVoiceServerDeafen` | ORPHANED | No frontend consumer |
| POST | `/api/v1/voice/{channelID}/members/{userID}/move` | `handleVoiceMoveUser` | ORPHANED | No frontend consumer |
| POST | `/api/v1/voice/{channelID}/input-mode` | `handleSetInputMode` | ORPHANED | No frontend consumer |
| POST | `/api/v1/voice/{channelID}/priority-speaker` | `handleSetPrioritySpeaker` | ORPHANED | No frontend consumer |
| GET | `/api/v1/voice/{channelID}/soundboard` | `handleGetSoundboardSounds` | ORPHANED | 6 soundboard endpoints, none wired |
| POST | `/api/v1/voice/{channelID}/soundboard` | `handleCreateSoundboardSound` | ORPHANED | |
| DELETE | `/api/v1/voice/{channelID}/soundboard/{soundID}` | `handleDeleteSoundboardSound` | ORPHANED | |
| POST | `/api/v1/voice/{channelID}/soundboard/{soundID}/play` | `handlePlaySoundboardSound` | ORPHANED | |
| GET | `/api/v1/voice/{channelID}/soundboard/config` | `handleGetSoundboardConfig` | ORPHANED | |
| PATCH | `/api/v1/voice/{channelID}/soundboard/config` | `handleUpdateSoundboardConfig` | ORPHANED | |
| POST | `/api/v1/voice/{channelID}/broadcast` | `handleStartBroadcast` | ORPHANED | 3 broadcast endpoints, none wired |
| DELETE | `/api/v1/voice/{channelID}/broadcast` | `handleStopBroadcast` | ORPHANED | |
| GET | `/api/v1/voice/{channelID}/broadcast` | `handleGetBroadcast` | ORPHANED | |
| POST | `/api/v1/voice/{channelID}/screen-share` | `handleStartScreenShare` | ORPHANED | 4 screen-share endpoints, none wired |
| DELETE | `/api/v1/voice/{channelID}/screen-share` | `handleStopScreenShare` | ORPHANED | |
| PATCH | `/api/v1/voice/{channelID}/screen-share` | `handleUpdateScreenShare` | ORPHANED | |
| GET | `/api/v1/voice/{channelID}/screen-shares` | `handleGetScreenShares` | ORPHANED | |

### Experimental Routes (ORPHANED except kanban)

All 21 non-kanban experimental routes are ORPHANED: location sharing (4), message effects (1), super reactions (2), message summaries (2), voice transcription (3), whiteboards (4), code snippets (3), video recordings (2). Kanban routes (6) are wired.

### Activities / Games / Watch-Together / Music-Party (ALL ORPHANED)

All 18 routes under `/api/v1/activities`, `/api/v1/games`, `/api/v1/watch-together`, `/api/v1/music-party` have no frontend consumers. The client.ts activity methods use channel-scoped paths that do not match these route registrations.

### Social / Growth (ALL 28 ORPHANED)

All routes for: guild insights, boosts, vanity claims, achievements, leveling/XP, starboard, welcome config, auto-roles. Zero frontend consumers.

### Integrations (ALL 13 ORPHANED)

All routes for: guild integrations, ActivityPub follows, bridge connections. Zero frontend consumers.

### Widgets / Plugins (ALL 12 ORPHANED)

All routes for: guild widgets, channel widgets, plugin management. Zero frontend consumers.

### Encryption Key Backup (ALL 5 ORPHANED)

All routes for: key backup CRUD, recovery codes. Zero frontend consumers.

### Admin Routes (49 ORPHANED of 68)

All admin routes for: reports, bot listing, rate limits (3), content scan (5), captcha (2), federation dashboard/controls/deliveries/search/protocol/blocklist/allowlist/profiles/users (16), setup (2), updates (5), health dashboard/history (2), storage (1), retention (5), domains (4), backups (6), bridges (8). Zero frontend consumers.

### Orphaned Endpoint Summary

| Category | Total Routes | ORPHANED | INTERNAL-ONLY | BOT-API-ONLY | OK |
|----------|-------------|----------|---------------|--------------|-----|
| Infrastructure | 6 | 0 | 6 | 0 | 0 |
| Federation S2S | 12 | 0 | 12 | 0 | 0 |
| Auth | 14 | 5 | 0 | 0 | 9 |
| Users | 50 | 13 | 0 | 0 | 37 |
| Guilds | 55 | 14 | 0 | 0 | 41 |
| Channels | 52 | 14 | 0 | 0 | 38 |
| Voice | 23 | 21 | 0 | 0 | 2 |
| Bots | 20 | 2 | 0 | 8 | 10 |
| Experimental | 27 | 21 | 0 | 0 | 6 |
| Activities/Games | 18 | 18 | 0 | 0 | 0 |
| Social/Growth | 28 | 28 | 0 | 0 | 0 |
| Integrations | 13 | 13 | 0 | 0 | 0 |
| Widgets/Plugins | 12 | 12 | 0 | 0 | 0 |
| Encryption Backup | 5 | 5 | 0 | 0 | 0 |
| Admin | 68 | 49 | 0 | 0 | 19 |
| **TOTAL** | **~403** | **~215** | **18** | **8** | **~162** |

**53% of all backend routes have no frontend consumer.** The bulk of these are in social/growth (28), admin (49), voice (21), experimental (21), and activities (18) feature groups that were implemented on the backend but never wired to the SvelteKit frontend.

---

## Part 2: Orphaned Events

See `SOCKET_EVENT_PARITY.md` for the comprehensive WebSocket event coverage table.

---

## Part 3: Type Parity

### Go Model to TypeScript Type Mapping

| Go Model (models.go) | TS Type (types/index.ts) | Missing TS Fields | Notes |
|----------------------|-------------------------|-------------------|-------|
| `Instance` | `InstanceInfo` | `public_key`, `last_seen_at` | TS type has flattened subset |
| `User` | `User` | (none) | Parity OK |
| `SelfUser` | (no separate type) | N/A | TS `User` includes `email` directly |
| `UserLink` | `UserLink` | (none) | Parity OK |
| `UserSession` | `Session` | `device_name`, `expires_at` | TS has extra `current` field Go lacks |
| `UserRelationship` | `Relationship` | (none) | TS uses `type` instead of `status`; TS has `id` Go lacks |
| `UserBlock` | (inline in client.ts) | (none) | Used inline in `getBlockedUsers()` return |
| `WebAuthnCredential` | **(none)** | N/A | **ORPHANED** - WebAuthn not wired to frontend |
| `Guild` | `Guild` | `system_channel_join/leave/kick/ban`, `afk_channel_id`, `afk_timeout` | 6 fields backend sends but TS ignores |
| `GuildCategory` | `Category` | `created_at` | Minor omission |
| `Channel` | `Channel` | `default_permissions`, `read_only`, `read_only_role_ids`, `default_auto_archive_duration` | 4 fields backend sends but TS ignores |
| `ChannelRecipient` | (none) | N/A | Internal join table, OK |
| `Role` | `Role` | (none) | Type difference: Go `int64` vs TS `string` for permissions (bigint serialization) |
| `GuildMember` | `GuildMember` | (none) | Parity OK |
| `MemberRole` | (none) | N/A | Join table, OK |
| `ChannelPermissionOverride` | **(none)** | N/A | **ORPHANED** - No TS type despite backend endpoints |
| `Message` | `Message` | `components` | TS has extra `reactions` and `pinned` (computed) |
| `ScheduledMessage` | `ScheduledMessage` | (none) | Parity OK |
| `Attachment` | `Attachment` | (none) | Parity OK |
| `MediaTag` | `MediaTag` | (none) | Parity OK |
| `AttachmentTag` | (none) | N/A | Join table, OK |
| `Embed` | `Embed` | **Major divergence** | Go has `id`, `message_id`, `embed_type`, `site_name`, `icon_url`, `special_type/id`; TS has `thumbnail_*`, `author_*`, `provider_*`. Structures do not match. |
| `Reaction` | `Reaction` | N/A | Different shapes by design (Go=per-user row, TS=aggregated) |
| `Pin` | (none) | N/A | Returned as Messages, OK |
| `Invite` | `Invite` | (none) | Parity OK |
| `GuildBan` | `Ban` | `banned_by` | TS lacks `banned_by` field |
| `CustomEmoji` | `CustomEmoji` | `s3_key` | Internal field, correctly omitted |
| `Webhook` | `Webhook` | (none) | Parity OK |
| `AuditLogEntry` | `AuditLogEntry` | `target_type` | Field names differ: Go `action` vs TS `action_type` |
| `FederationPeer` | `FederationPeer` | (none) | Different shapes (TS denormalizes domain) |
| `ReadState` | `ReadState` | `user_id` | Implicit from auth context |
| `Poll` | `Poll` | (none) | Parity OK |
| `PollOption` | `PollOption` | (none) | Parity OK |
| `MessageBookmark` | `MessageBookmark` | (none) | Parity OK |
| `GuildEvent` | `GuildEvent` | (none) | Parity OK |
| `EventRSVP` | `EventRSVP` | (none) | Parity OK |
| `MemberWarning` | `MemberWarning` | (none) | Parity OK |
| `MessageReport` | `MessageReport` | (none) | Parity OK |
| `GuildRaidConfig` | `RaidConfig` | (none) | Parity OK |
| `ChannelFollower` | `ChannelFollower` | (none) | TS adds denormalized `guild_name`, `channel_name` |
| `BotToken` | `BotToken` | (none) | Parity OK |
| `SlashCommand` | `SlashCommand` | (none) | Parity OK |
| `ChannelTemplate` | **(none)** | N/A | **ORPHANED** - No TS type |
| `BotGuildPermission` | **(none)** | N/A | **ORPHANED** - No TS type |
| `MessageComponent` | **(none)** | N/A | **ORPHANED** - No TS type |
| `BotPresence` | (none) | N/A | BOT-API-ONLY |
| `BotEventSubscription` | (none) | N/A | BOT-API-ONLY |
| `BotRateLimit` | (none) | N/A | BOT-API-ONLY |
| `UserReport` | `UserReport` | (none) | Parity OK |
| `ReportedIssue` | `ReportedIssue` | (none) | Parity OK |
| `ForumTag` | `ForumTag` | (none) | Parity OK |
| `ForumPost` | `ForumPost` | (none) | Parity OK |
| `GalleryTag` | `GalleryTag` | (none) | Parity OK |
| `GalleryPost` | `GalleryPost` | (none) | Parity OK |
| `ModerationStats` | `ModerationStats` | (none) | Parity OK |

### Critical Parity Issues

1. **Embed type mismatch**: Go `Embed` and TS `Embed` have significantly different field structures. The Go struct stores DB fields (`id`, `message_id`, `embed_type`, `site_name`, `icon_url`, `special_type`, `special_id`) while the TS type uses OEmbed-style fields (`thumbnail_url/width/height`, `author_name/url`, `provider_name/url`). Either the handler transforms between formats or data is silently lost.

2. **Guild missing 6 fields**: TS `Guild` type lacks `system_channel_join`, `system_channel_leave`, `system_channel_kick`, `system_channel_ban`, `afk_channel_id`, and `afk_timeout`. These are sent by the backend but silently ignored by the frontend.

3. **Channel missing 4 fields**: TS `Channel` type lacks `default_permissions`, `read_only`, `read_only_role_ids`, `default_auto_archive_duration`.

4. **Session type mismatch**: Go `UserSession` has `device_name` and `expires_at` that TS `Session` lacks. TS has `current` Go lacks.

5. **5 Go model types with no TS equivalent**: `WebAuthnCredential`, `ChannelPermissionOverride`, `ChannelTemplate`, `BotGuildPermission`, `MessageComponent` -- all orphaned backend features.

6. **AuditLogEntry field naming**: Go uses `action` and has `target_type`; TS uses `action_type` and lacks `target_type`. May cause silent data loss on audit log display.
