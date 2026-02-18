# AmityVox Backend Architecture Map

**Generated:** 2026-02-18
**Audit Phase:** 1A - Backend Inventory + 1B - Frontend Inventory
**Source files analyzed:** 55 Go files in `internal/api/`, plus `internal/events/events.go`, `internal/gateway/gateway.go`, worker modules, federation, automod, and plugins.

---

## Table of Contents

- [Phase 1A: Backend Inventory](#phase-1a-backend-inventory)
  - [Route Inventory](#route-inventory)
  - [Event Inventory (NATS)](#event-inventory-nats)
  - [WebSocket Events (Gateway)](#websocket-events-gateway)
  - [Package Summary](#package-summary)
  - [Middleware Chain](#middleware-chain)
- [Phase 1B: Frontend Inventory](#phase-1b-complete-frontend-inventory)

---

# Phase 1A: Backend Inventory

## Route Inventory

**Total REST endpoints: ~353** (including 2 health checks, 1 metrics, 3 public unauthenticated)

### Non-API Routes (No `/api/v1` prefix)

| Method | Path | Handler | Middleware | Package |
|--------|------|---------|------------|---------|
| GET | `/health` | `s.handleHealthCheck` | Global (no rate limit) | api (server.go) |
| GET | `/health/deep` | `s.handleDeepHealthCheck` | Global (no rate limit) | api (health.go) |
| GET | `/metrics` | `s.handleMetrics` | Global + RateLimitGlobal | api (metrics.go) |

### Auth Routes (`/api/v1/auth`)

#### Public (IP-based rate limit)

| Method | Path | Handler | Middleware | Package |
|--------|------|---------|------------|---------|
| POST | `/api/v1/auth/register` | `s.handleRegister` | RateLimitGlobal (IP) | api (server.go) |
| POST | `/api/v1/auth/login` | `s.handleLogin` | RateLimitGlobal (IP) | api (server.go) |

#### Authenticated (user-based rate limit)

| Method | Path | Handler | Middleware | Package |
|--------|------|---------|------------|---------|
| POST | `/api/v1/auth/logout` | `s.handleLogout` | RequireAuth + RateLimitGlobal | api (server.go) |
| POST | `/api/v1/auth/password` | `s.handleChangePassword` | RequireAuth + RateLimitGlobal | api (server.go) |
| POST | `/api/v1/auth/email` | `s.handleChangeEmail` | RequireAuth + RateLimitGlobal | api (server.go) |
| POST | `/api/v1/auth/totp/enable` | `s.handleTOTPEnable` | RequireAuth + RateLimitGlobal | api (mfa_handlers.go) |
| POST | `/api/v1/auth/totp/verify` | `s.handleTOTPVerify` | RequireAuth + RateLimitGlobal | api (mfa_handlers.go) |
| DELETE | `/api/v1/auth/totp` | `s.handleTOTPDisable` | RequireAuth + RateLimitGlobal | api (mfa_handlers.go) |
| POST | `/api/v1/auth/backup-codes` | `s.handleGenerateBackupCodes` | RequireAuth + RateLimitGlobal | api (mfa_handlers.go) |
| POST | `/api/v1/auth/backup-codes/verify` | `s.handleConsumeBackupCode` | RequireAuth + RateLimitGlobal | api (mfa_handlers.go) |
| POST | `/api/v1/auth/webauthn/register/begin` | `s.handleWebAuthnRegisterBegin` | RequireAuth + RateLimitGlobal | api (mfa_handlers.go) |
| POST | `/api/v1/auth/webauthn/register/finish` | `s.handleWebAuthnRegisterFinish` | RequireAuth + RateLimitGlobal | api (mfa_handlers.go) |
| POST | `/api/v1/auth/webauthn/login/begin` | `s.handleWebAuthnLoginBegin` | RequireAuth + RateLimitGlobal | api (mfa_handlers.go) |
| POST | `/api/v1/auth/webauthn/login/finish` | `s.handleWebAuthnLoginFinish` | RequireAuth + RateLimitGlobal | api (mfa_handlers.go) |

### User Routes (`/api/v1/users`)

All require `RequireAuth + RateLimitGlobal` middleware.

| Method | Path | Handler | Package |
|--------|------|---------|---------|
| GET | `/api/v1/users/@me` | `userH.HandleGetSelf` | users |
| PATCH | `/api/v1/users/@me` | `userH.HandleUpdateSelf` | users |
| DELETE | `/api/v1/users/@me` | `userH.HandleDeleteSelf` | users |
| GET | `/api/v1/users/@me/guilds` | `userH.HandleGetSelfGuilds` | users |
| GET | `/api/v1/users/@me/dms` | `userH.HandleGetSelfDMs` | users |
| GET | `/api/v1/users/@me/relationships` | `userH.HandleGetRelationships` | users |
| GET | `/api/v1/users/@me/read-state` | `userH.HandleGetSelfReadState` | users |
| GET | `/api/v1/users/@me/sessions` | `userH.HandleGetSelfSessions` | users |
| DELETE | `/api/v1/users/@me/sessions/{sessionID}` | `userH.HandleDeleteSelfSession` | users |
| GET | `/api/v1/users/@me/settings` | `userH.HandleGetUserSettings` | users |
| PATCH | `/api/v1/users/@me/settings` | `userH.HandleUpdateUserSettings` | users |
| GET | `/api/v1/users/@me/blocked` | `userH.HandleGetBlockedUsers` | users |
| GET | `/api/v1/users/@me/bookmarks` | `bookmarkH.HandleListBookmarks` | bookmarks |
| GET | `/api/v1/users/@me/bots` | `botH.HandleListMyBots` | bots |
| POST | `/api/v1/users/@me/bots` | `botH.HandleCreateBot` | bots |
| GET | `/api/v1/users/@me/export` | `userH.HandleExportUserData` | users/export |
| GET | `/api/v1/users/@me/export-account` | `userH.HandleExportAccount` | users/export |
| POST | `/api/v1/users/@me/import-account` | `userH.HandleImportAccount` | users/export |
| PUT | `/api/v1/users/@me/activity` | `userH.HandleUpdateActivity` | users/activity |
| GET | `/api/v1/users/@me/activity` | `userH.HandleGetActivity` | users/activity |
| GET | `/api/v1/users/@me/hidden-threads` | `channelH.HandleGetHiddenThreads` | channels |
| GET | `/api/v1/users/@me/emoji` | `userH.HandleGetUserEmoji` | users/emoji |
| POST | `/api/v1/users/@me/emoji` | `userH.HandleCreateUserEmoji` | users/emoji |
| DELETE | `/api/v1/users/@me/emoji/{emojiID}` | `userH.HandleDeleteUserEmoji` | users/emoji |
| GET | `/api/v1/users/@me/links` | `userH.HandleGetMyLinks` | users |
| POST | `/api/v1/users/@me/links` | `userH.HandleCreateLink` | users |
| PATCH | `/api/v1/users/@me/links/{linkID}` | `userH.HandleUpdateLink` | users |
| DELETE | `/api/v1/users/@me/links/{linkID}` | `userH.HandleDeleteLink` | users |
| GET | `/api/v1/users/@me/issues` | `modH.HandleGetMyIssues` | moderation |
| POST | `/api/v1/users/@me/group-dms` | `userH.HandleCreateGroupDM` | users |
| GET | `/api/v1/users/@me/channel-groups` | `channelGroupH.HandleGetChannelGroups` | users/channel_groups |
| POST | `/api/v1/users/@me/channel-groups` | `channelGroupH.HandleCreateChannelGroup` | users/channel_groups |
| PATCH | `/api/v1/users/@me/channel-groups/{groupID}` | `channelGroupH.HandleUpdateChannelGroup` | users/channel_groups |
| DELETE | `/api/v1/users/@me/channel-groups/{groupID}` | `channelGroupH.HandleDeleteChannelGroup` | users/channel_groups |
| PUT | `/api/v1/users/@me/channel-groups/{groupID}/channels/{channelID}` | `channelGroupH.HandleAddChannelToGroup` | users/channel_groups |
| DELETE | `/api/v1/users/@me/channel-groups/{groupID}/channels/{channelID}` | `channelGroupH.HandleRemoveChannelFromGroup` | users/channel_groups |
| GET | `/api/v1/users/resolve` | `userH.HandleResolveHandle` | users/resolve |
| GET | `/api/v1/users/{userID}` | `userH.HandleGetUser` | users |
| GET | `/api/v1/users/{userID}/note` | `userH.HandleGetUserNote` | users |
| PUT | `/api/v1/users/{userID}/note` | `userH.HandleSetUserNote` | users |
| POST | `/api/v1/users/{userID}/dm` | `userH.HandleCreateDM` | users |
| PUT | `/api/v1/users/{userID}/friend` | `userH.HandleAddFriend` | users |
| DELETE | `/api/v1/users/{userID}/friend` | `userH.HandleRemoveFriend` | users |
| PUT | `/api/v1/users/{userID}/block` | `userH.HandleBlockUser` | users |
| PATCH | `/api/v1/users/{userID}/block` | `userH.HandleUpdateBlockLevel` | users |
| DELETE | `/api/v1/users/{userID}/block` | `userH.HandleUnblockUser` | users |
| GET | `/api/v1/users/{userID}/mutual-friends` | `userH.HandleGetMutualFriends` | users |
| GET | `/api/v1/users/{userID}/mutual-guilds` | `userH.HandleGetMutualGuilds` | users |
| GET | `/api/v1/users/{userID}/badges` | `userH.HandleGetUserBadges` | users/badges |
| GET | `/api/v1/users/{userID}/links` | `userH.HandleGetUserLinks` | users |
| POST | `/api/v1/users/{userID}/report` | `modH.HandleReportUser` | moderation/global_moderation |

### Bot Routes (`/api/v1/bots/{botID}`)

All require `RequireAuth + RateLimitGlobal` middleware.

| Method | Path | Handler | Package |
|--------|------|---------|---------|
| GET | `/api/v1/bots/{botID}` | `botH.HandleGetBot` | bots |
| PATCH | `/api/v1/bots/{botID}` | `botH.HandleUpdateBot` | bots |
| DELETE | `/api/v1/bots/{botID}` | `botH.HandleDeleteBot` | bots |
| GET | `/api/v1/bots/{botID}/tokens` | `botH.HandleListTokens` | bots |
| POST | `/api/v1/bots/{botID}/tokens` | `botH.HandleCreateToken` | bots |
| DELETE | `/api/v1/bots/{botID}/tokens/{tokenID}` | `botH.HandleDeleteToken` | bots |
| GET | `/api/v1/bots/{botID}/commands` | `botH.HandleListCommands` | bots |
| POST | `/api/v1/bots/{botID}/commands` | `botH.HandleRegisterCommand` | bots |
| PATCH | `/api/v1/bots/{botID}/commands/{commandID}` | `botH.HandleUpdateCommand` | bots |
| DELETE | `/api/v1/bots/{botID}/commands/{commandID}` | `botH.HandleDeleteCommand` | bots |
| GET | `/api/v1/bots/{botID}/guilds/{guildID}/permissions` | `botH.HandleGetBotGuildPermissions` | bots |
| PUT | `/api/v1/bots/{botID}/guilds/{guildID}/permissions` | `botH.HandleUpdateBotGuildPermissions` | bots |
| GET | `/api/v1/bots/{botID}/presence` | `botH.HandleGetBotPresence` | bots |
| PUT | `/api/v1/bots/{botID}/presence` | `botH.HandleUpdateBotPresence` | bots |
| GET | `/api/v1/bots/{botID}/rate-limit` | `botH.HandleGetBotRateLimit` | bots |
| PUT | `/api/v1/bots/{botID}/rate-limit` | `botH.HandleUpdateBotRateLimit` | bots |
| POST | `/api/v1/bots/{botID}/subscriptions` | `botH.HandleCreateEventSubscription` | bots |
| GET | `/api/v1/bots/{botID}/subscriptions` | `botH.HandleListEventSubscriptions` | bots |
| DELETE | `/api/v1/bots/{botID}/subscriptions/{subscriptionID}` | `botH.HandleDeleteEventSubscription` | bots |
| POST | `/api/v1/bots/interactions` | `botH.HandleComponentInteraction` | bots |

### Guild Routes (`/api/v1/guilds`)

All require `RequireAuth + RateLimitGlobal` middleware. (65+ endpoints in guilds.go alone)

| Method | Path | Handler | Package |
|--------|------|---------|---------|
| POST | `/api/v1/guilds` | `guildH.HandleCreateGuild` | guilds |
| GET | `/api/v1/guilds/discover` | `guildH.HandleDiscoverGuilds` | guilds |
| GET | `/api/v1/guilds/vanity/{code}` | `guildH.HandleResolveVanityURL` | guilds |
| GET | `/api/v1/guilds/{guildID}/preview` | `guildH.HandleGetGuildPreview` | guilds |
| POST | `/api/v1/guilds/{guildID}/join` | `guildH.HandleJoinDiscoverableGuild` | guilds |
| GET | `/api/v1/guilds/{guildID}` | `guildH.HandleGetGuild` | guilds |
| PATCH | `/api/v1/guilds/{guildID}` | `guildH.HandleUpdateGuild` | guilds |
| DELETE | `/api/v1/guilds/{guildID}` | `guildH.HandleDeleteGuild` | guilds |
| POST | `/api/v1/guilds/{guildID}/leave` | `guildH.HandleLeaveGuild` | guilds |
| POST | `/api/v1/guilds/{guildID}/transfer` | `guildH.HandleTransferGuildOwnership` | guilds |
| GET | `/api/v1/guilds/{guildID}/channels` | `guildH.HandleGetGuildChannels` | guilds |
| PATCH | `/api/v1/guilds/{guildID}/channels` | `guildH.HandleReorderGuildChannels` | guilds |
| POST | `/api/v1/guilds/{guildID}/channels` | `guildH.HandleCreateGuildChannel` | guilds |
| POST | `/api/v1/guilds/{guildID}/channels/{channelID}/clone` | `guildH.HandleCloneChannel` | guilds |
| GET | `/api/v1/guilds/{guildID}/guide` | `guildH.HandleGetServerGuide` | guilds |
| PUT | `/api/v1/guilds/{guildID}/guide` | `guildH.HandleUpdateServerGuide` | guilds |
| GET | `/api/v1/guilds/{guildID}/bump` | `guildH.HandleGetBumpStatus` | guilds |
| POST | `/api/v1/guilds/{guildID}/bump` | `guildH.HandleBumpGuild` | guilds |
| POST | `/api/v1/guilds/{guildID}/templates` | `guildH.HandleCreateGuildTemplate` | guilds/templates |
| GET | `/api/v1/guilds/{guildID}/templates` | `guildH.HandleGetGuildTemplates` | guilds/templates |
| GET | `/api/v1/guilds/{guildID}/templates/{templateID}` | `guildH.HandleGetGuildTemplate` | guilds/templates |
| DELETE | `/api/v1/guilds/{guildID}/templates/{templateID}` | `guildH.HandleDeleteGuildTemplate` | guilds/templates |
| POST | `/api/v1/guilds/{guildID}/templates/{templateID}/apply` | `guildH.HandleApplyGuildTemplate` | guilds/templates |
| GET | `/api/v1/guilds/{guildID}/members/@me/permissions` | `guildH.HandleGetMyPermissions` | guilds |
| GET | `/api/v1/guilds/{guildID}/members` | `guildH.HandleGetGuildMembers` | guilds |
| GET | `/api/v1/guilds/{guildID}/members/search` | `guildH.HandleSearchGuildMembers` | guilds |
| GET | `/api/v1/guilds/{guildID}/members/{memberID}` | `guildH.HandleGetGuildMember` | guilds |
| PATCH | `/api/v1/guilds/{guildID}/members/{memberID}` | `guildH.HandleUpdateGuildMember` | guilds |
| DELETE | `/api/v1/guilds/{guildID}/members/{memberID}` | `guildH.HandleRemoveGuildMember` | guilds |
| POST | `/api/v1/guilds/{guildID}/members/{memberID}/warn` | `modH.HandleWarnMember` | moderation |
| GET | `/api/v1/guilds/{guildID}/members/{memberID}/warnings` | `modH.HandleGetWarnings` | moderation |
| GET | `/api/v1/guilds/{guildID}/members/{memberID}/roles` | `guildH.HandleGetMemberRoles` | guilds |
| PUT | `/api/v1/guilds/{guildID}/members/{memberID}/roles/{roleID}` | `guildH.HandleAddMemberRole` | guilds |
| DELETE | `/api/v1/guilds/{guildID}/members/{memberID}/roles/{roleID}` | `guildH.HandleRemoveMemberRole` | guilds |
| GET | `/api/v1/guilds/{guildID}/prune` | `guildH.HandleGetGuildPruneCount` | guilds |
| POST | `/api/v1/guilds/{guildID}/prune` | `guildH.HandleGuildPrune` | guilds |
| GET | `/api/v1/guilds/{guildID}/bans` | `guildH.HandleGetGuildBans` | guilds |
| PUT | `/api/v1/guilds/{guildID}/bans/{userID}` | `guildH.HandleCreateGuildBan` | guilds |
| DELETE | `/api/v1/guilds/{guildID}/bans/{userID}` | `guildH.HandleRemoveGuildBan` | guilds |
| GET | `/api/v1/guilds/{guildID}/roles` | `guildH.HandleGetGuildRoles` | guilds |
| PATCH | `/api/v1/guilds/{guildID}/roles` | `guildH.HandleReorderGuildRoles` | guilds |
| POST | `/api/v1/guilds/{guildID}/roles` | `guildH.HandleCreateGuildRole` | guilds |
| PATCH | `/api/v1/guilds/{guildID}/roles/{roleID}` | `guildH.HandleUpdateGuildRole` | guilds |
| DELETE | `/api/v1/guilds/{guildID}/roles/{roleID}` | `guildH.HandleDeleteGuildRole` | guilds |
| GET | `/api/v1/guilds/{guildID}/invites` | `guildH.HandleGetGuildInvites` | guilds |
| POST | `/api/v1/guilds/{guildID}/invites` | `guildH.HandleCreateGuildInvite` | guilds |
| GET | `/api/v1/guilds/{guildID}/categories` | `guildH.HandleGetGuildCategories` | guilds |
| POST | `/api/v1/guilds/{guildID}/categories` | `guildH.HandleCreateGuildCategory` | guilds |
| PATCH | `/api/v1/guilds/{guildID}/categories/{categoryID}` | `guildH.HandleUpdateGuildCategory` | guilds |
| DELETE | `/api/v1/guilds/{guildID}/categories/{categoryID}` | `guildH.HandleDeleteGuildCategory` | guilds |
| GET | `/api/v1/guilds/{guildID}/audit-log` | `guildH.HandleGetGuildAuditLog` | guilds |
| GET | `/api/v1/guilds/{guildID}/emoji` | `guildH.HandleGetGuildEmoji` | guilds |
| POST | `/api/v1/guilds/{guildID}/emoji` | `guildH.HandleCreateGuildEmoji` | guilds |
| PATCH | `/api/v1/guilds/{guildID}/emoji/{emojiID}` | `guildH.HandleUpdateGuildEmoji` | guilds |
| DELETE | `/api/v1/guilds/{guildID}/emoji/{emojiID}` | `guildH.HandleDeleteGuildEmoji` | guilds |
| GET | `/api/v1/guilds/{guildID}/webhooks` | `guildH.HandleGetGuildWebhooks` | guilds |
| POST | `/api/v1/guilds/{guildID}/webhooks` | `guildH.HandleCreateGuildWebhook` | guilds |
| PATCH | `/api/v1/guilds/{guildID}/webhooks/{webhookID}` | `guildH.HandleUpdateGuildWebhook` | guilds |
| DELETE | `/api/v1/guilds/{guildID}/webhooks/{webhookID}` | `guildH.HandleDeleteGuildWebhook` | guilds |
| GET | `/api/v1/guilds/{guildID}/webhooks/{webhookID}/logs` | `webhookH.HandleGetWebhookLogs` | webhooks |
| GET | `/api/v1/guilds/{guildID}/vanity-url` | `guildH.HandleGetGuildVanityURL` | guilds |
| PATCH | `/api/v1/guilds/{guildID}/vanity-url` | `guildH.HandleSetGuildVanityURL` | guilds |
| DELETE | `/api/v1/guilds/{guildID}/warnings/{warningID}` | `modH.HandleDeleteWarning` | moderation |
| GET | `/api/v1/guilds/{guildID}/reports` | `modH.HandleGetReports` | moderation |
| PATCH | `/api/v1/guilds/{guildID}/reports/{reportID}` | `modH.HandleResolveReport` | moderation |
| GET | `/api/v1/guilds/{guildID}/raid-config` | `modH.HandleGetRaidConfig` | moderation |
| PATCH | `/api/v1/guilds/{guildID}/raid-config` | `modH.HandleUpdateRaidConfig` | moderation |
| POST | `.../ban-lists` | `modH.HandleCreateBanList` | moderation/ban_lists |
| GET | `.../ban-lists` | `modH.HandleGetBanLists` | moderation/ban_lists |
| DELETE | `.../ban-lists/{listID}` | `modH.HandleDeleteBanList` | moderation/ban_lists |
| GET | `.../ban-lists/{listID}/entries` | `modH.HandleGetBanListEntries` | moderation/ban_lists |
| POST | `.../ban-lists/{listID}/entries` | `modH.HandleAddBanListEntry` | moderation/ban_lists |
| DELETE | `.../ban-lists/{listID}/entries/{entryID}` | `modH.HandleRemoveBanListEntry` | moderation/ban_lists |
| GET | `.../ban-lists/{listID}/export` | `modH.HandleExportBanList` | moderation/ban_lists |
| POST | `.../ban-lists/{listID}/import` | `modH.HandleImportBanList` | moderation/ban_lists |
| GET | `.../ban-list-subscriptions` | `modH.HandleGetBanListSubscriptions` | moderation/ban_lists |
| POST | `.../ban-list-subscriptions` | `modH.HandleSubscribeBanList` | moderation/ban_lists |
| DELETE | `.../ban-list-subscriptions/{subID}` | `modH.HandleUnsubscribeBanList` | moderation/ban_lists |
| POST | `.../sticker-packs` | `stickerH.HandleCreateGuildPack` | stickers |
| GET | `.../sticker-packs` | `stickerH.HandleGetGuildPacks` | stickers |
| DELETE | `.../sticker-packs/{packID}` | `stickerH.HandleDeletePack` | stickers |
| GET | `.../sticker-packs/{packID}/stickers` | `stickerH.HandleGetPackStickers` | stickers |
| POST | `.../sticker-packs/{packID}/stickers` | `stickerH.HandleAddSticker` | stickers |
| DELETE | `.../sticker-packs/{packID}/stickers/{stickerID}` | `stickerH.HandleDeleteSticker` | stickers |
| GET | `.../onboarding` | `onboardH.HandleGetOnboarding` | onboarding |
| PUT | `.../onboarding` | `onboardH.HandleUpdateOnboarding` | onboarding |
| POST | `.../onboarding/prompts` | `onboardH.HandleCreatePrompt` | onboarding |
| PUT | `.../onboarding/prompts/{promptID}` | `onboardH.HandleUpdatePrompt` | onboarding |
| DELETE | `.../onboarding/prompts/{promptID}` | `onboardH.HandleDeletePrompt` | onboarding |
| POST | `.../onboarding/complete` | `onboardH.HandleCompleteOnboarding` | onboarding |
| GET | `.../onboarding/status` | `onboardH.HandleGetOnboardingStatus` | onboarding |
| POST | `.../events` | `guildEventH.HandleCreateEvent` | guildevents |
| GET | `.../events` | `guildEventH.HandleListEvents` | guildevents |
| GET | `.../events/{eventID}` | `guildEventH.HandleGetEvent` | guildevents |
| PATCH | `.../events/{eventID}` | `guildEventH.HandleUpdateEvent` | guildevents |
| DELETE | `.../events/{eventID}` | `guildEventH.HandleDeleteEvent` | guildevents |
| POST | `.../events/{eventID}/rsvp` | `guildEventH.HandleRSVP` | guildevents |
| DELETE | `.../events/{eventID}/rsvp` | `guildEventH.HandleDeleteRSVP` | guildevents |
| GET | `.../events/{eventID}/rsvps` | `guildEventH.HandleListRSVPs` | guildevents |
| GET | `.../retention` | `guildH.HandleGetGuildRetentionPolicies` | guilds/retention |
| POST | `.../retention` | `guildH.HandleCreateGuildRetentionPolicy` | guilds/retention |
| PATCH | `.../retention/{policyID}` | `guildH.HandleUpdateGuildRetentionPolicy` | guilds/retention |
| DELETE | `.../retention/{policyID}` | `guildH.HandleDeleteGuildRetentionPolicy` | guilds/retention |
| GET | `.../gallery` | `guildH.HandleGetGuildGallery` | guilds |
| GET | `.../media-tags` | `guildH.HandleGetMediaTags` | guilds |
| POST | `.../media-tags` | `guildH.HandleCreateMediaTag` | guilds |
| DELETE | `.../media-tags/{tagID}` | `guildH.HandleDeleteMediaTag` | guilds |
| GET | `.../automod/rules` | `s.AutoMod.HandleListRules` | automod (conditional) |
| POST | `.../automod/rules` | `s.AutoMod.HandleCreateRule` | automod (conditional) |
| POST | `.../automod/rules/test` | `s.AutoMod.HandleTestRule` | automod (conditional) |
| GET | `.../automod/rules/{ruleID}` | `s.AutoMod.HandleGetRule` | automod (conditional) |
| PATCH | `.../automod/rules/{ruleID}` | `s.AutoMod.HandleUpdateRule` | automod (conditional) |
| DELETE | `.../automod/rules/{ruleID}` | `s.AutoMod.HandleDeleteRule` | automod (conditional) |
| GET | `.../automod/actions` | `s.AutoMod.HandleGetActions` | automod (conditional) |

### Channel Routes (`/api/v1/channels`)

All require `RequireAuth + RateLimitGlobal` middleware. (75 handlers across channels, forum, gallery, emoji, translation)

| Method | Path | Handler | Package |
|--------|------|---------|---------|
| GET | `.../channels/{channelID}` | `channelH.HandleGetChannel` | channels |
| PATCH | `.../channels/{channelID}` | `channelH.HandleUpdateChannel` | channels |
| DELETE | `.../channels/{channelID}` | `channelH.HandleDeleteChannel` | channels |
| GET | `.../channels/{channelID}/messages` | `channelH.HandleGetMessages` | channels |
| POST | `.../channels/{channelID}/messages` | `channelH.HandleCreateMessage` | channels (+RateLimitMessages) |
| POST | `.../messages/bulk-delete` | `channelH.HandleBulkDeleteMessages` | channels |
| GET | `.../messages/{messageID}` | `channelH.HandleGetMessage` | channels |
| PATCH | `.../messages/{messageID}` | `channelH.HandleUpdateMessage` | channels |
| DELETE | `.../messages/{messageID}` | `channelH.HandleDeleteMessage` | channels |
| GET | `.../messages/{messageID}/edits` | `channelH.HandleGetMessageEdits` | channels |
| POST | `.../messages/{messageID}/crosspost` | `channelH.HandleCrosspostMessage` | channels |
| GET | `.../messages/{messageID}/reactions` | `channelH.HandleGetReactions` | channels |
| PUT | `.../messages/{messageID}/reactions/{emoji}` | `channelH.HandleAddReaction` | channels |
| DELETE | `.../messages/{messageID}/reactions/{emoji}` | `channelH.HandleRemoveReaction` | channels |
| DELETE | `.../reactions/{emoji}/{targetUserID}` | `channelH.HandleRemoveUserReaction` | channels |
| GET | `.../pins` | `channelH.HandleGetPins` | channels |
| PUT | `.../pins/{messageID}` | `channelH.HandlePinMessage` | channels |
| DELETE | `.../pins/{messageID}` | `channelH.HandleUnpinMessage` | channels |
| POST | `.../typing` | `channelH.HandleTriggerTyping` | channels |
| POST | `.../decrypt-messages` | `channelH.HandleBatchDecryptMessages` | channels |
| POST | `.../ack` | `channelH.HandleAckChannel` | channels |
| PUT | `.../permissions/{overrideID}` | `channelH.HandleSetChannelPermission` | channels |
| DELETE | `.../permissions/{overrideID}` | `channelH.HandleDeleteChannelPermission` | channels |
| POST | `.../messages/{messageID}/threads` | `channelH.HandleCreateThread` | channels |
| POST | `.../messages/{messageID}/report` | `modH.HandleReportMessage` | moderation |
| POST | `.../messages/{messageID}/report-admin` | `modH.HandleReportToAdmin` | moderation |
| POST | `.../messages/{messageID}/translate` | `channelH.HandleTranslateMessage` | channels/translation |
| GET | `.../threads` | `channelH.HandleGetThreads` | channels |
| POST | `.../threads/{threadID}/hide` | `channelH.HandleHideThread` | channels |
| DELETE | `.../threads/{threadID}/hide` | `channelH.HandleUnhideThread` | channels |
| POST | `.../lock` | `modH.HandleLockChannel` | moderation |
| POST | `.../unlock` | `modH.HandleUnlockChannel` | moderation |
| GET | `.../webhooks` | `channelH.HandleGetChannelWebhooks` | channels |
| GET | `.../export` | `userH.HandleExportChannelMessages` | users/export |
| GET | `.../gallery` | `channelH.HandleGetChannelGallery` | channels |
| GET | `.../tags` | `channelH.HandleGetForumTags` | channels/forum |
| POST | `.../tags` | `channelH.HandleCreateForumTag` | channels/forum |
| PATCH | `.../tags/{tagID}` | `channelH.HandleUpdateForumTag` | channels/forum |
| DELETE | `.../tags/{tagID}` | `channelH.HandleDeleteForumTag` | channels/forum |
| GET | `.../posts` | `channelH.HandleGetForumPosts` | channels/forum |
| POST | `.../posts` | `channelH.HandleCreateForumPost` | channels/forum |
| POST | `.../posts/{postID}/pin` | `channelH.HandlePinForumPost` | channels/forum |
| POST | `.../posts/{postID}/close` | `channelH.HandleCloseForumPost` | channels/forum |
| GET | `.../gallery-tags` | `channelH.HandleGetGalleryTags` | channels/gallery |
| POST | `.../gallery-tags` | `channelH.HandleCreateGalleryTag` | channels/gallery |
| PATCH | `.../gallery-tags/{tagID}` | `channelH.HandleUpdateGalleryTag` | channels/gallery |
| DELETE | `.../gallery-tags/{tagID}` | `channelH.HandleDeleteGalleryTag` | channels/gallery |
| GET | `.../gallery-posts` | `channelH.HandleGetGalleryPosts` | channels/gallery |
| POST | `.../gallery-posts` | `channelH.HandleCreateGalleryPost` | channels/gallery |
| POST | `.../gallery-posts/{postID}/pin` | `channelH.HandlePinGalleryPost` | channels/gallery |
| POST | `.../gallery-posts/{postID}/close` | `channelH.HandleCloseGalleryPost` | channels/gallery |
| POST | `.../templates` | `channelH.HandleCreateChannelTemplate` | channels |
| GET | `.../templates` | `channelH.HandleGetChannelTemplates` | channels |
| DELETE | `.../templates/{templateID}` | `channelH.HandleDeleteChannelTemplate` | channels |
| POST | `.../templates/{templateID}/apply` | `channelH.HandleApplyChannelTemplate` | channels |
| GET | `.../emoji` | `channelEmojiH.HandleGetChannelEmoji` | channels/emoji |
| POST | `.../emoji` | `channelEmojiH.HandleCreateChannelEmoji` | channels/emoji |
| DELETE | `.../emoji/{emojiID}` | `channelEmojiH.HandleDeleteChannelEmoji` | channels/emoji |
| POST | `.../followers` | `channelH.HandleFollowChannel` | channels |
| GET | `.../followers` | `channelH.HandleGetChannelFollowers` | channels |
| DELETE | `.../followers/{followerID}` | `channelH.HandleUnfollowChannel` | channels |
| POST | `.../messages/{messageID}/publish` | `channelH.HandlePublishMessage` | channels |
| POST | `.../scheduled-messages` | `channelH.HandleScheduleMessage` | channels |
| GET | `.../scheduled-messages` | `channelH.HandleGetScheduledMessages` | channels |
| DELETE | `.../scheduled-messages/{messageID}` | `channelH.HandleDeleteScheduledMessage` | channels |
| PUT | `.../recipients/{userID}` | `channelH.HandleAddGroupDMRecipient` | channels |
| DELETE | `.../recipients/{userID}` | `channelH.HandleRemoveGroupDMRecipient` | channels |
| POST | `.../polls` | `pollH.HandleCreatePoll` | polls |
| GET | `.../polls/{pollID}` | `pollH.HandleGetPoll` | polls |
| POST | `.../polls/{pollID}/votes` | `pollH.HandleVotePoll` | polls |
| POST | `.../polls/{pollID}/close` | `pollH.HandleClosePoll` | polls |
| DELETE | `.../polls/{pollID}` | `pollH.HandleDeletePoll` | polls |

### Voice Routes (`/api/v1/voice`) - 23 handlers

All require `RequireAuth + RateLimitGlobal` middleware.

| Method | Path | Handler | Package |
|--------|------|---------|---------|
| POST | `.../voice/{channelID}/join` | `s.handleVoiceJoin` | api (voice_handlers.go) |
| POST | `.../voice/{channelID}/leave` | `s.handleVoiceLeave` | api |
| GET | `.../voice/{channelID}/states` | `s.handleGetVoiceStates` | api |
| POST | `.../voice/{channelID}/members/{userID}/mute` | `s.handleVoiceServerMute` | api |
| POST | `.../voice/{channelID}/members/{userID}/deafen` | `s.handleVoiceServerDeafen` | api |
| POST | `.../voice/{channelID}/members/{userID}/move` | `s.handleVoiceMoveUser` | api |
| GET | `/api/v1/voice/preferences` | `s.handleGetVoicePreferences` | api |
| PATCH | `/api/v1/voice/preferences` | `s.handleUpdateVoicePreferences` | api |
| POST | `.../voice/{channelID}/input-mode` | `s.handleSetInputMode` | api |
| POST | `.../voice/{channelID}/priority-speaker` | `s.handleSetPrioritySpeaker` | api |
| GET | `.../voice/{channelID}/soundboard` | `s.handleGetSoundboardSounds` | api |
| POST | `.../voice/{channelID}/soundboard` | `s.handleCreateSoundboardSound` | api |
| DELETE | `.../voice/{channelID}/soundboard/{soundID}` | `s.handleDeleteSoundboardSound` | api |
| POST | `.../voice/{channelID}/soundboard/{soundID}/play` | `s.handlePlaySoundboardSound` | api |
| GET | `.../voice/{channelID}/soundboard/config` | `s.handleGetSoundboardConfig` | api |
| PATCH | `.../voice/{channelID}/soundboard/config` | `s.handleUpdateSoundboardConfig` | api |
| POST | `.../voice/{channelID}/broadcast` | `s.handleStartBroadcast` | api |
| DELETE | `.../voice/{channelID}/broadcast` | `s.handleStopBroadcast` | api |
| GET | `.../voice/{channelID}/broadcast` | `s.handleGetBroadcast` | api |
| POST | `.../voice/{channelID}/screen-share` | `s.handleStartScreenShare` | api |
| DELETE | `.../voice/{channelID}/screen-share` | `s.handleStopScreenShare` | api |
| PATCH | `.../voice/{channelID}/screen-share` | `s.handleUpdateScreenShare` | api |
| GET | `.../voice/{channelID}/screen-shares` | `s.handleGetScreenShares` | api |

### Experimental Routes (`/api/v1/channels/{channelID}/experimental`) - 27 handlers

### Activities & Games - 19 handlers across `/api/v1/activities`, `/api/v1/games`, `/api/v1/watch-together`, `/api/v1/music-party`

### Social & Growth - 35 handlers across `/api/v1/guilds/{guildID}/insights|boosts|leveling|leaderboard|starboard|welcome|auto-roles` and `/api/v1/achievements`

### Integration Routes - 14 handlers across `/api/v1/guilds/{guildID}/integrations` and `.../bridge-connections`

### Admin Routes (`/api/v1/admin`) - 89 handlers across admin.go, federation.go, selfhost.go

### Additional Top-Level Routes

| Group | Endpoints | Handler Package |
|-------|-----------|----------------|
| Messages bookmarks | 2 (PUT/DELETE `/messages/{id}/bookmark`) | bookmarks |
| Issue reporting | 1 (POST `/issues`) | moderation |
| Moderation panel | 7 endpoints | moderation/global_moderation |
| Public ban lists | 1 (GET `/ban-lists/public`) | moderation/ban_lists |
| Webhooks | 3 (templates, preview, outgoing-events) | webhooks |
| User stickers | 6 endpoints | stickers |
| Themes | 6 endpoints | themes |
| Widgets | 6 endpoints | widgets |
| Plugins | 6 endpoints | widgets |
| Encryption key backup | 5 endpoints | widgets |
| Files (authed) | 5 endpoints (upload, update, delete, tag, untag) | media |
| Encryption/MLS | 11 endpoints (conditional) | encryption |
| Notifications | 9 endpoints (5 conditional on VAPID) | notifications |
| Search | 3 endpoints (+ RateLimitSearch) | api (search_handlers.go) |
| Giphy proxy | 3 endpoints (conditional) | api (giphy_handler.go) |
| Announcements | 1 (GET `/announcements`) | admin |
| Invites | 3 (get, accept, delete) | invites |

### Public Routes (no auth, IP-based rate limit)

| Method | Path | Handler | Package |
|--------|------|---------|---------|
| GET | `/api/v1/guilds/{guildID}/widget.json` | `widgetH.HandleGetGuildWidgetEmbed` | widgets |
| GET | `/api/v1/files/{fileID}` | `s.Media.HandleGetFile` | media |
| POST | `/api/v1/webhooks/{webhookID}/{token}` | `webhookH.HandleExecute` | webhooks (+RateLimitWebhooks) |

---

## Event Inventory (NATS)

**Total unique NATS subjects defined:** 34 (constants in `internal/events/events.go`)
**Additional ad-hoc subjects used inline:** 7 (experimental/activities)

### Defined Subject Constants

| Subject | Constant | Published By | Consumed By |
|---------|----------|-------------|-------------|
| `amityvox.message.create` | SubjectMessageCreate | channels, webhooks, social, federation | Gateway, NotificationWorker, AutoModWorker, SearchWorker, OutgoingWebhookSubscriber |
| `amityvox.message.update` | SubjectMessageUpdate | channels | Gateway, SearchWorker |
| `amityvox.message.delete` | SubjectMessageDelete | channels, automod | Gateway, SearchWorker |
| `amityvox.message.delete_bulk` | SubjectMessageDeleteBulk | channels | Gateway |
| `amityvox.message.reaction_add` | SubjectMessageReactionAdd | channels | Gateway |
| `amityvox.message.reaction_remove` | SubjectMessageReactionDel | channels | Gateway |
| `amityvox.message.reaction_clear` | SubjectMessageReactionClr | **(defined but NOT published)** | Gateway |
| `amityvox.message.embed_update` | SubjectMessageEmbedUpdate | workers/media_workers (embed unfurler) | Gateway |
| `amityvox.channel.create` | SubjectChannelCreate | guilds, users, channels | Gateway |
| `amityvox.channel.update` | SubjectChannelUpdate | channels, moderation | Gateway |
| `amityvox.channel.delete` | SubjectChannelDelete | channels | Gateway |
| `amityvox.channel.pins_update` | SubjectChannelPinsUpdate | channels | Gateway |
| `amityvox.channel.typing_start` | SubjectTypingStart | channels, gateway (via op:8) | Gateway |
| `amityvox.channel.ack` | SubjectChannelAck | channels | Gateway |
| `amityvox.guild.create` | SubjectGuildCreate | guilds | Gateway |
| `amityvox.guild.update` | SubjectGuildUpdate | guilds, social, voice_handlers (soundboard) | Gateway |
| `amityvox.guild.delete` | SubjectGuildDelete | guilds | Gateway |
| `amityvox.guild.member_add` | SubjectGuildMemberAdd | guilds, invites, federation | Gateway |
| `amityvox.guild.member_update` | SubjectGuildMemberUpdate | guilds, social | Gateway |
| `amityvox.guild.member_remove` | SubjectGuildMemberRemove | guilds, federation | Gateway |
| `amityvox.guild.role_create` | SubjectGuildRoleCreate | guilds | Gateway |
| `amityvox.guild.role_update` | SubjectGuildRoleUpdate | guilds | Gateway |
| `amityvox.guild.role_delete` | SubjectGuildRoleDelete | guilds | Gateway |
| `amityvox.guild.ban_add` | SubjectGuildBanAdd | guilds | Gateway |
| `amityvox.guild.ban_remove` | SubjectGuildBanRemove | guilds | Gateway |
| `amityvox.guild.emoji_update` | SubjectGuildEmojiUpdate | guilds | Gateway |
| `amityvox.guild.event_create` | SubjectGuildEventCreate | guildevents | Gateway |
| `amityvox.guild.event_update` | SubjectGuildEventUpdate | guildevents | Gateway |
| `amityvox.guild.event_delete` | SubjectGuildEventDelete | guildevents | Gateway |
| `amityvox.guild.raid_lockdown` | SubjectRaidLockdown | **(defined but NOT published)** | Gateway |
| `amityvox.presence.update` | SubjectPresenceUpdate | gateway (broadcastPresence) | Gateway |
| `amityvox.user.update` | SubjectUserUpdate | users | Gateway |
| `amityvox.user.relationship_add` | SubjectRelationshipAdd | users | Gateway |
| `amityvox.user.relationship_update` | SubjectRelationshipUpdate | users | Gateway (+handleRelationshipEvent) |
| `amityvox.user.relationship_remove` | SubjectRelationshipRemove | users | Gateway (+handleRelationshipEvent) |
| `amityvox.voice.state_update` | SubjectVoiceStateUpdate | voice_handlers, gateway (op:4) | Gateway |
| `amityvox.voice.server_update` | SubjectVoiceServerUpdate | **(defined but NOT published)** | Gateway |
| `amityvox.voice.call_ring` | SubjectCallRing | voice_handlers | Gateway (special DM dispatch) |
| `amityvox.automod.action` | SubjectAutomodAction | automod | Gateway |
| `amityvox.poll.create` | SubjectPollCreate | polls | Gateway |
| `amityvox.poll.vote` | SubjectPollVote | polls | Gateway |
| `amityvox.poll.close` | SubjectPollClose | polls | Gateway |
| `amityvox.announcement.create` | SubjectAnnouncementCreate | admin | Gateway (broadcast to all) |
| `amityvox.announcement.update` | SubjectAnnouncementUpdate | admin | Gateway (broadcast to all) |
| `amityvox.announcement.delete` | SubjectAnnouncementDelete | admin | Gateway (broadcast to all) |
| `amityvox.federation.retry` | SubjectFederationRetry | federation/sync | federation/sync (QueueSubscribe) |

### Ad-Hoc Subjects (not defined as constants)

| Subject | Event Types | Published By |
|---------|------------|-------------|
| `amityvox.channel.location_share` | LOCATION_SHARE_CREATE/UPDATE/STOP | experimental |
| `amityvox.message.effect` | MESSAGE_EFFECT_CREATE | experimental |
| `amityvox.channel.whiteboard_update` | WHITEBOARD_UPDATE | experimental |
| `amityvox.channel.code_snippet` | CODE_SNIPPET_CREATE | experimental |
| `amityvox.channel.kanban_update` | KANBAN_CARD_CREATE/MOVE | experimental |
| `amityvox.channel.activity` | ACTIVITY_SESSION_START/END, PARTICIPANT_JOIN/LEAVE, STATE_UPDATE, WATCH_TOGETHER_START/SYNC, MUSIC_PARTY_START/QUEUE_ADD | activities |
| `amityvox.channel.game` | GAME_SESSION_CREATE, GAME_PLAYER_JOIN, GAME_MOVE | activities |

### JetStream Streams

| Stream Name | Subjects | Retention | Max Age | Storage |
|-------------|----------|-----------|---------|---------|
| AMITYVOX_EVENTS | `amityvox.message.>`, `.channel.>`, `.guild.>`, `.presence.>`, `.user.>`, `.voice.>`, `.automod.>`, `.poll.>`, `.announcement.>` | LimitsPolicy | 24h | File |
| AMITYVOX_FEDERATION | `amityvox.federation.>` | WorkQueuePolicy | 7d | File |

---

## WebSocket Events (Gateway)

### Protocol Opcodes

| Op | Name | Direction | Description |
|----|------|-----------|-------------|
| 0 | DISPATCH | Server->Client | Event dispatch (type + sequence number) |
| 1 | HEARTBEAT | Client->Server | Keep-alive heartbeat |
| 2 | IDENTIFY | Client->Server | Initial auth with token |
| 3 | PRESENCE_UPDATE | Client->Server | Update status (online/idle/busy/invisible) |
| 4 | VOICE_STATE_UPDATE | Client->Server | Voice self_mute/self_deaf/channel changes |
| 5 | RESUME | Client->Server | Resume disconnected session |
| 6 | RECONNECT | Server->Client | Server instructs reconnect |
| 7 | REQUEST_MEMBERS | Client->Server | Request guild member list chunk |
| 8 | TYPING | Client->Server | Typing indicator for channel |
| 9 | SUBSCRIBE | Client->Server | Subscribe to specific channel IDs (DMs) |
| 10 | HELLO | Server->Client | Heartbeat interval + build version |
| 11 | HEARTBEAT_ACK | Server->Client | Heartbeat acknowledgment |

### Dispatched Event Types (Op 0) - 75+ unique types

**Gateway-originated events:**
- READY, RESUMED, GUILD_MEMBERS_CHUNK, PRESENCE_UPDATE

**NATS-forwarded events (via dispatchEvent):**
All event types published by handlers (see Event Inventory above) are forwarded through `shouldDispatchTo` filtering.

### shouldDispatchTo Rules (priority order)

1. **PRESENCE_UPDATE / USER_UPDATE**: user themselves + friends + guild co-members
2. **RELATIONSHIP_ADD/UPDATE/REMOVE**: targeted user only (event.UserID == client.userID)
3. **Guild events (event.GuildID set)**: guild members only
4. **CALL_RING**: DM/group recipients, excluding the caller
5. **Guild/voice subject prefix with guild_id in data**: guild members
6. **Channel events (event.ChannelID set)**: guild channel -> guild members; DM/group -> recipients
7. **Channel subject prefix with channel_id in data**: same lookup logic
8. **Announcement events**: broadcast to ALL identified clients
9. **Default**: deny (fail-closed)

---

## Package Summary

### `internal/api/` Root Files

| File | Lines | Handlers | Description |
|------|-------|----------|-------------|
| server.go | 1,477 | 7 | Main router, middleware, response helpers |
| voice_handlers.go | 1,616 | 23 | Voice/video: join, leave, mute, deafen, soundboard, broadcast, screen share |
| mfa_handlers.go | 663 | 9 | TOTP, backup codes, WebAuthn |
| search_handlers.go | 427 | 3 | Meilisearch proxy: messages, users, guilds |
| giphy_handler.go | 228 | 3 | Giphy API proxy |
| health.go | 191 | 1 | Deep health check |
| metrics.go | 96 | 1 | Prometheus /metrics |
| ratelimit.go | 249 | 0 | Rate limiting middleware |
| **Subtotal** | **4,947** | **47** | |

### Handler Packages

| Package | Files | Lines | Handlers | Largest File |
|---------|-------|-------|----------|-------------|
| users/ | 8 (7+1 test) | 3,797 | 64 | users.go (1,789) |
| guilds/ | 4 (3+1 test) | 4,558 | 77 | guilds.go (3,320) |
| channels/ | 7 (5+2 test) | 5,440 | 75 | channels.go (3,115) |
| admin/ | 4 (3+1 test) | 4,852 | 89 | admin.go (1,923) |
| moderation/ | 3 | 1,996 | 39 | moderation.go (873) |
| social/ | 1 | 2,011 | 35 | social.go (2,011) |
| experimental/ | 1 | 1,769 | 27 | experimental.go (1,769) |
| bots/ | 1 | 1,453 | 24 | bots.go (1,453) |
| activities/ | 1 | 1,424 | 20 | activities.go (1,424) |
| widgets/ | 1 | 1,148 | 20 | widgets.go (1,148) |
| webhooks/ | 2 (1+1 test) | 1,097 | 8 | webhooks.go (984) |
| guildevents/ | 1 | 756 | 11 | events.go (756) |
| integrations/ | 1 | 739 | 14 | integrations.go (739) |
| onboarding/ | 1 | 733 | 9 | onboarding.go (733) |
| stickers/ | 1 | 670 | 14 | stickers.go (670) |
| polls/ | 1 | 522 | 5 | polls.go (522) |
| themes/ | 1 | 403 | 6 | themes.go (403) |
| invites/ | 2 (1+1 test) | 376 | 3 | invites.go (272) |
| bookmarks/ | 1 | 237 | 3 | bookmarks.go (237) |
| messages/ | 1 | 5 | 0 | messages.go (empty package) |

### Grand Totals

| Metric | Value |
|--------|-------|
| **Total Go files in internal/api/** | 55 |
| **Total lines of code** | 39,404 |
| **Total handler functions** | ~590 |
| **Largest file** | guilds/guilds.go (3,320 lines) |
| **Test files** | 10 |

---

## Middleware Chain

### Global Middleware (applied to ALL routes, in order)

1. **`middleware.RequestID`** - Generates unique X-Request-ID header
2. **`middleware.RealIP`** - Extracts real client IP from X-Forwarded-For/X-Real-IP
3. **`slogMiddleware`** - Structured request logging (method, path, status, duration, user_id)
4. **`middleware.Recoverer`** - Panic recovery, returns 500
5. **`corsMiddleware`** - CORS headers per configured origins
6. **`middleware.Compress(5)`** - Gzip compression level 5
7. **`middleware.Timeout(30s)`** - 30-second request timeout
8. **`maxBodySize(1MB)`** - 1MB body limit (skipped for multipart/form-data)

### Per-Group Middleware

| Route Group | Additional Middleware | Rate Limit Config |
|-------------|----------------------|-------------------|
| `/health`, `/health/deep` | None (no rate limit) | None |
| `/metrics` | `RateLimitGlobal()` | IP-based, 1200/min |
| `/auth` (public) | `RateLimitGlobal()` | IP-based, 1200/min |
| `/auth` (authed) | `RequireAuth` -> `RateLimitGlobal()` | User-based, 6000/min |
| All authed routes | `RequireAuth` -> `RateLimitGlobal()` | User-based, 6000/min |
| Message POST | `+RateLimitMessages` | User-based, 100/10s |
| `/search/*` | `+RateLimitSearch` | User-based, 300/min |
| Webhook execute | `RateLimitGlobal()` + `RateLimitWebhooks` | IP 1200/min + Webhook 300/min |

### NATS Subscribers (Background)

| Subscriber | Subject | Queue Group | Module |
|------------|---------|-------------|--------|
| Gateway dispatch | `amityvox.>` | None (all instances) | gateway |
| Outgoing webhook delivery | `amityvox.>` | None | webhooks |
| Notification worker | `amityvox.message.create` | None | workers/notification_worker |
| AutoMod worker | `amityvox.message.create` | None | workers/automod_worker |
| Search indexer | `amityvox.message.>` | None | workers/workers |
| Media transcoder | Custom transcode subject | `transcode-workers` | workers/media_workers |
| Embed unfurler | Custom embed subject | `embed-workers` | workers/media_workers |
| Federation router | Multiple subjects | `federation-router` | federation/sync |
| Federation retry consumer | `amityvox.federation.retry` | `federation-retry` | federation/sync |
| Plugin event router | Per-plugin subscribed subjects | None | plugins/plugins |

---

## Audit Notes for Subsequent Agents

1. **Duplicate route registration**: `users/@me/relationships` is registered twice on lines 302 and 308 of server.go. The second registration overwrites the first (same handler, no functional issue, but indicates copy-paste).

2. **Unused NATS subjects**: `SubjectMessageReactionClr`, `SubjectRaidLockdown`, and `SubjectVoiceServerUpdate` are defined but never published anywhere. They may be reserved for future use or represent incomplete features.

3. **Ad-hoc subjects**: Experimental and activities handlers use inline string subjects instead of the constants defined in events.go. Maintenance risk if naming conventions change.

4. **Conditional routes**: AutoMod, Media, Encryption, Notifications, Giphy route groups are conditionally registered based on service availability.

5. **The `messages` package** (`internal/api/messages/messages.go`) is empty (5 lines, package declaration only). All message handling is in the `channels` package.

6. **Handler naming**: Package-level handlers use `Handle` prefix (capitalized); server-level handlers use lowercase `handle` prefix.

7. **Federation events** flow through NATS JetStream with WorkQueue retention (7-day max age), providing reliable delivery. Regular events use LimitsPolicy with 24-hour retention.

---

## Phase 1B: Complete Frontend Inventory

Generated: 2026-02-18

---

### API Client Methods

**File:** `/docker/AmityVox/web/src/lib/api/client.ts`
**Total Public Methods: 197**

| # | Method Name | HTTP Method | URL Path | Parameters | Return Type |
|---|-------------|-------------|----------|------------|-------------|
| 1 | register | POST | /auth/register | username, email, password | LoginResponse |
| 2 | login | POST | /auth/login | username, password | LoginResponse |
| 3 | logout | POST | /auth/logout | (none) | void |
| 4 | getMe | GET | /users/@me | (none) | User |
| 5 | updateMe | PATCH | /users/@me | Partial<User> + avatar_id, status_expires_at | User |
| 6 | getUser | GET | /users/{userId} | userId | User |
| 7 | getUserBadges | GET | /users/{userId}/badges | userId | UserBadge[] |
| 8 | getUserLinks | GET | /users/{userId}/links | userId | UserLink[] |
| 9 | getMyLinks | GET | /users/@me/links | (none) | UserLink[] |
| 10 | createLink | POST | /users/@me/links | platform, label, url | UserLink |
| 11 | updateLink | PATCH | /users/@me/links/{linkId} | linkId, data | UserLink |
| 12 | deleteLink | DELETE | /users/@me/links/{linkId} | linkId | void |
| 13 | getMyGuilds | GET | /users/@me/guilds | (none) | Guild[] |
| 14 | getMyDMs | GET | /users/@me/dms | (none) | Channel[] |
| 15 | createDM | POST | /users/{userId}/dm | userId | Channel |
| 16 | createGroupDM | POST | /users/@me/group-dms | userIds, name? | Channel |
| 17 | addGroupDMRecipient | PUT | /channels/{channelId}/recipients/{userId} | channelId, userId | Channel |
| 18 | removeGroupDMRecipient | DELETE | /channels/{channelId}/recipients/{userId} | channelId, userId | void |
| 19 | createGuild | POST | /guilds | name, description? | Guild |
| 20 | getGuild | GET | /guilds/{guildId} | guildId | Guild |
| 21 | getMyPermissions | GET | /guilds/{guildId}/members/@me/permissions | guildId | { permissions: string } |
| 22 | updateGuild | PATCH | /guilds/{guildId} | guildId, Partial<Guild> | Guild |
| 23 | deleteGuild | DELETE | /guilds/{guildId} | guildId | void |
| 24 | leaveGuild | POST | /guilds/{guildId}/leave | guildId | void |
| 25 | getGuildChannels | GET | /guilds/{guildId}/channels | guildId | Channel[] |
| 26 | createChannel | POST | /guilds/{guildId}/channels | guildId, name, type | Channel |
| 27 | getChannel | GET | /channels/{channelId} | channelId | Channel |
| 28 | updateChannel | PATCH | /channels/{channelId} | channelId, Partial<Channel> | Channel |
| 29 | deleteChannel | DELETE | /channels/{channelId} | channelId | void |
| 30 | cloneChannel | POST | /guilds/{guildId}/channels/{channelId}/clone | guildId, channelId, name? | Channel |
| 31 | getMessages | GET | /channels/{channelId}/messages | channelId, before?, after?, limit? | Message[] |
| 32 | sendMessage | POST | /channels/{channelId}/messages | channelId, content, opts? | Message |
| 33 | batchDecryptMessages | POST | /channels/{channelId}/decrypt-messages | channelId, messages[] | void |
| 34 | scheduleMessage | POST | /channels/{channelId}/scheduled-messages | channelId, content, scheduledFor, opts? | ScheduledMessage |
| 35 | getScheduledMessages | GET | /channels/{channelId}/scheduled-messages | channelId | ScheduledMessage[] |
| 36 | deleteScheduledMessage | DELETE | /channels/{channelId}/scheduled-messages/{messageId} | channelId, messageId | void |
| 37 | editMessage | PATCH | /channels/{channelId}/messages/{messageId} | channelId, messageId, content | Message |
| 38 | deleteMessage | DELETE | /channels/{channelId}/messages/{messageId} | channelId, messageId | void |
| 39 | bulkDeleteMessages | POST | /channels/{channelId}/messages/bulk-delete | channelId, messageIds[] | void |
| 40 | getPins | GET | /channels/{channelId}/pins | channelId | Message[] |
| 41 | pinMessage | PUT | /channels/{channelId}/pins/{messageId} | channelId, messageId | void |
| 42 | unpinMessage | DELETE | /channels/{channelId}/pins/{messageId} | channelId, messageId | void |
| 43 | getReadState | GET | /users/@me/read-state | (none) | ReadState[] |
| 44 | ackChannel | POST | /channels/{channelId}/ack | channelId | void |
| 45 | getFriends | GET | /users/@me/relationships | (none) | Relationship[] |
| 46 | addFriend | PUT | /users/{userId}/friend | userId | Relationship |
| 47 | removeFriend | DELETE | /users/{userId}/friend | userId | void |
| 48 | blockUser | PUT | /users/{userId}/block | userId, level | void |
| 49 | updateBlockLevel | PATCH | /users/{userId}/block | userId, level | void |
| 50 | unblockUser | DELETE | /users/{userId}/block | userId | void |
| 51 | getBlockedUsers | GET | /users/@me/blocked | (none) | BlockedUser[] |
| 52 | resolveHandle | GET | /users/resolve?handle= | handle | User |
| 53 | addReaction | PUT | /channels/{channelId}/messages/{messageId}/reactions/{emoji} | channelId, messageId, emoji | void |
| 54 | removeReaction | DELETE | /channels/{channelId}/messages/{messageId}/reactions/{emoji} | channelId, messageId, emoji | void |
| 55 | getMembers | GET | /guilds/{guildId}/members | guildId | GuildMember[] |
| 56 | getMember | GET | /guilds/{guildId}/members/{memberId} | guildId, memberId | GuildMember |
| 57 | kickMember | DELETE | /guilds/{guildId}/members/{memberId} | guildId, memberId, reason? | void |
| 58 | updateMember | PATCH | /guilds/{guildId}/members/{memberId} | guildId, memberId, data | GuildMember |
| 59 | getMemberRoles | GET | /guilds/{guildId}/members/{memberId}/roles | guildId, memberId | Role[] |
| 60 | banUser | PUT | /guilds/{guildId}/bans/{userId} | guildId, userId, options? | void |
| 61 | unbanUser | DELETE | /guilds/{guildId}/bans/{userId} | guildId, userId | void |
| 62 | getRoles | GET | /guilds/{guildId}/roles | guildId | Role[] |
| 63 | createRole | POST | /guilds/{guildId}/roles | guildId, name, opts? | Role |
| 64 | getGuildInvites | GET | /guilds/{guildId}/invites | guildId | Invite[] |
| 65 | createInvite | POST | /guilds/{guildId}/invites | guildId, opts? | Invite |
| 66 | deleteInvite | DELETE | /invites/{code} | code | void |
| 67 | getInvite | GET | /invites/{code} | code | Invite |
| 68 | acceptInvite | POST | /invites/{code} | code | Guild |
| 69 | getGuildBans | GET | /guilds/{guildId}/bans | guildId | Ban[] |
| 70 | getAuditLog | GET | /guilds/{guildId}/audit-log | guildId, params? | AuditLogEntry[] |
| 71 | getGuildEmoji | GET | /guilds/{guildId}/emoji | guildId | CustomEmoji[] |
| 72 | deleteGuildEmoji | DELETE | /guilds/{guildId}/emoji/{emojiId} | guildId, emojiId | void |
| 73 | changePassword | POST | /auth/password | currentPassword, newPassword | void |
| 74 | enableTOTP | POST | /auth/totp/enable | (none) | { secret, qr_url } |
| 75 | verifyTOTP | POST | /auth/totp/verify | code | { backup_codes } |
| 76 | disableTOTP | DELETE | /auth/totp | code | void |
| 77 | generateBackupCodes | POST | /auth/backup-codes | (none) | { codes } |
| 78 | getSessions | GET | /users/@me/sessions | (none) | Session[] |
| 79 | deleteSession | DELETE | /users/@me/sessions/{sessionId} | sessionId | void |
| 80 | sendTyping | POST | /channels/{channelId}/typing | channelId | void |
| 81 | joinVoice | POST | /voice/{channelId}/join | channelId | { token, url } |
| 82 | leaveVoice | POST | /voice/{channelId}/leave | channelId | void |
| 83 | getVoicePreferences | GET | /voice/preferences | (none) | VoicePreferences |
| 84 | updateVoicePreferences | PATCH | /voice/preferences | Partial<VoicePreferences> | VoicePreferences |
| 85 | joinFederatedVoice | POST | /federation/voice/join | instanceDomain, channelId, screenShare | { token, url, channel_id } |
| 86 | joinFederatedVoiceByGuild | POST | /federation/voice/guild-join | guildId, channelId, screenShare | { token, url, channel_id } |
| 87 | joinFederatedGuild | POST | /federation/guilds/join | instanceDomain, guildId?, inviteCode? | unknown |
| 88 | leaveFederatedGuild | POST | /federation/guilds/{guildId}/leave | guildId | void |
| 89 | getFederatedGuildMessages | GET | /federation/guilds/{guildId}/channels/{channelId}/messages | guildId, channelId, params? | Message[] |
| 90 | sendFederatedGuildMessage | POST | /federation/guilds/{guildId}/channels/{channelId}/messages | guildId, channelId, content, nonce? | Message |
| 91 | uploadFile | POST (multipart) | /files/upload | file, altText? | { id, url } |
| 92 | searchMessages | GET | /search/messages | query, guildId?, channelId? | Message[] |
| 93 | createThread | POST | /channels/{channelId}/messages/{messageId}/threads | channelId, messageId, name | Channel |
| 94 | getThreads | GET | /channels/{channelId}/threads | channelId | Channel[] |
| 95 | hideThread | POST | /channels/{channelId}/threads/{threadId}/hide | channelId, threadId | void |
| 96 | unhideThread | DELETE | /channels/{channelId}/threads/{threadId}/hide | channelId, threadId | void |
| 97 | getHiddenThreads | GET | /users/@me/hidden-threads | (none) | string[] |
| 98 | getMessageEdits | GET | /channels/{channelId}/messages/{messageId}/edits | channelId, messageId | EditHistory[] |
| 99 | translateMessage | POST | /channels/{channelId}/messages/{messageId}/translate | channelId, messageId, targetLang, force | TranslationResult |
| 100 | getUserNote | GET | /users/{userId}/note | userId | { target_id, note } |
| 101 | setUserNote | PUT | /users/{userId}/note | userId, note | { target_id, note } |
| 102 | getMutualFriends | GET | /users/{userId}/mutual-friends | userId | User[] |
| 103 | getMutualGuilds | GET | /users/{userId}/mutual-guilds | userId | MutualGuild[] |
| 104 | searchGiphy | GET | /giphy/search | query, limit, offset | any |
| 105 | getTrendingGiphy | GET | /giphy/trending | limit, offset | any |
| 106 | getGiphyCategories | GET | /giphy/categories | limit | any |
| 107 | getChannelGallery | GET | /channels/{channelId}/gallery | channelId, options? | Attachment[] |
| 108 | getGuildGallery | GET | /guilds/{guildId}/gallery | guildId, options? | Attachment[] |
| 109 | updateAttachment | PATCH | /files/{fileId} | fileId, data | Attachment |
| 110 | deleteAttachment | DELETE | /files/{fileId} | fileId | void |
| 111 | getMediaTags | GET | /guilds/{guildId}/media-tags | guildId | MediaTag[] |
| 112 | createMediaTag | POST | /guilds/{guildId}/media-tags | guildId, name | MediaTag |
| 113 | deleteMediaTag | DELETE | /guilds/{guildId}/media-tags/{tagId} | guildId, tagId | void |
| 114 | tagAttachment | PUT | /files/{fileId}/tags/{tagId} | fileId, tagId | void |
| 115 | untagAttachment | DELETE | /files/{fileId}/tags/{tagId} | fileId, tagId | void |
| 116 | getAdminMedia | GET | /admin/media | before? | Attachment[] |
| 117 | deleteAdminMedia | DELETE | /admin/media/{fileId} | fileId | void |
| 118 | getAdminStats | GET | /admin/stats | (none) | AdminStats |
| 119 | getAdminInstance | GET | /admin/instance | (none) | InstanceInfo |
| 120 | updateAdminInstance | PATCH | /admin/instance | data | InstanceInfo |
| 121 | getAdminGuilds | GET | /admin/guilds | params? | any[] |
| 122 | getAdminGuildDetails | GET | /admin/guilds/{guildId} | guildId | any |
| 123 | adminDeleteGuild | DELETE | /admin/guilds/{guildId} | guildId | void |
| 124 | getAdminUserGuilds | GET | /admin/users/{userId}/guilds | userId | any[] |
| 125 | getAdminUsers | GET | /admin/users | params? | User[] |
| 126 | suspendUser | POST | /admin/users/{userId}/suspend | userId | void |
| 127 | unsuspendUser | POST | /admin/users/{userId}/unsuspend | userId | void |
| 128 | setAdmin | POST | /admin/users/{userId}/set-admin | userId, isAdmin | void |
| 129 | getFederationPeers | GET | /admin/federation/peers | (none) | FederationPeer[] |
| 130 | addFederationPeer | POST | /admin/federation/peers | domain | FederationPeer |
| 131 | removeFederationPeer | DELETE | /admin/federation/peers/{peerId} | peerId | void |
| 132 | instanceBanUser | POST | /admin/users/{userId}/instance-ban | userId, reason | void |
| 133 | instanceUnbanUser | POST | /admin/users/{userId}/instance-unban | userId | void |
| 134 | getInstanceBans | GET | /admin/instance-bans | (none) | InstanceBan[] |
| 135 | getRegistrationSettings | GET | /admin/registration | (none) | RegistrationSettings |
| 136 | updateRegistrationSettings | PATCH | /admin/registration | data | RegistrationSettings |
| 137 | createRegistrationToken | POST | /admin/registration/tokens | data | RegistrationToken |
| 138 | getRegistrationTokens | GET | /admin/registration/tokens | (none) | RegistrationToken[] |
| 139 | deleteRegistrationToken | DELETE | /admin/registration/tokens/{tokenId} | tokenId | void |
| 140 | createAnnouncement | POST | /admin/announcements | data | Announcement |
| 141 | getAdminAnnouncements | GET | /admin/announcements | (none) | Announcement[] |
| 142 | updateAnnouncement | PATCH | /admin/announcements/{id} | id, data | Announcement |
| 143 | deleteAnnouncement | DELETE | /admin/announcements/{id} | id | void |
| 144 | getActiveAnnouncements | GET | /announcements | (none) | Announcement[] |
| 145 | getNotificationPreferences | GET | /notifications/preferences | guildId? | NotificationPreference |
| 146 | updateNotificationPreferences | PATCH | /notifications/preferences | data | NotificationPreference |
| 147 | getChannelNotificationPreferences | GET | /notifications/preferences/channels | (none) | ChannelNotificationPreference[] |
| 148 | updateChannelNotificationPreference | PATCH | /notifications/preferences/channels | data | ChannelNotificationPreference |
| 149 | deleteChannelNotificationPreference | DELETE | /notifications/preferences/channels/{channelId} | channelId | void |
| 150 | getUserSettings | GET | /users/@me/settings | (none) | UserSettings |
| 151 | updateUserSettings | PATCH | /users/@me/settings | Partial<UserSettings> | UserSettings |
| 152 | getGuildWebhooks | GET | /guilds/{guildId}/webhooks | guildId | Webhook[] |
| 153 | getChannelWebhooks | GET | /channels/{channelId}/webhooks | channelId | Webhook[] |
| 154 | createWebhook | POST | /guilds/{guildId}/webhooks | guildId, data | Webhook |
| 155 | updateWebhook | PATCH | /guilds/{guildId}/webhooks/{webhookId} | guildId, webhookId, data | Webhook |
| 156 | deleteWebhook | DELETE | /guilds/{guildId}/webhooks/{webhookId} | guildId, webhookId | void |
| 157 | getCategories | GET | /guilds/{guildId}/categories | guildId | Category[] |
| 158 | createCategory | POST | /guilds/{guildId}/categories | guildId, name | Category |
| 159 | updateCategory | PATCH | /guilds/{guildId}/categories/{categoryId} | guildId, categoryId, data | Category |
| 160 | deleteCategory | DELETE | /guilds/{guildId}/categories/{categoryId} | guildId, categoryId | void |
| 161 | forwardMessage | POST | /channels/{channelId}/messages/{messageId}/crosspost | channelId, messageId, targetChannelId | Message |
| 162 | uploadEmoji | POST (multipart) | /guilds/{guildId}/emoji | guildId, name, file | CustomEmoji |
| 163 | uploadKeyPackage | POST | /encryption/key-packages | keyPackage | { id } |
| 164 | getKeyPackages | GET | /encryption/key-packages/{userId} | userId | KeyPackage[] |
| 165 | claimKeyPackage | POST | /encryption/key-packages/{userId}/claim | userId | KeyPackage |
| 166 | getWelcomeMessages | GET | /encryption/welcome | (none) | WelcomeMessage[] |
| 167 | sendWelcome | POST | /encryption/channels/{channelId}/welcome | channelId, userId, data | void |
| 168 | getGroupState | GET | /encryption/channels/{channelId}/group-state | channelId | { epoch, state } |
| 169 | updateGroupState | PUT | /encryption/channels/{channelId}/group-state | channelId, epoch, state | void |
| 170 | ackWelcome | POST | /encryption/welcome/{welcomeId}/ack | welcomeId | void |
| 171 | deleteKeyPackage | DELETE | /encryption/key-packages/{keyPackageId} | keyPackageId | void |
| 172 | getVapidKey | GET | /notifications/vapid-key | (none) | { public_key } |
| 173 | subscribePush | POST | /notifications/subscriptions | subscription | { id } |
| 174 | getPushSubscriptions | GET | /notifications/subscriptions | (none) | PushSubscription[] |
| 175 | deletePushSubscription | DELETE | /notifications/subscriptions/{subscriptionId} | subscriptionId | void |
| 176 | createPoll | POST | /channels/{channelId}/polls | channelId, question, options, opts? | Poll |
| 177 | getPoll | GET | /channels/{channelId}/polls/{pollId} | channelId, pollId | Poll |
| 178 | votePoll | POST | /channels/{channelId}/polls/{pollId}/votes | channelId, pollId, optionIds | VoteResult |
| 179 | closePoll | POST | /channels/{channelId}/polls/{pollId}/close | channelId, pollId | { poll_id, closed } |
| 180 | deletePoll | DELETE | /channels/{channelId}/polls/{pollId} | channelId, pollId | void |
| 181 | createBookmark | PUT | /messages/{messageId}/bookmark | messageId, note?, reminderAt? | MessageBookmark |
| 182 | deleteBookmark | DELETE | /messages/{messageId}/bookmark | messageId | void |
| 183 | getBookmarks | GET | /users/@me/bookmarks | params? | MessageBookmark[] |
| 184 | createGuildEvent | POST | /guilds/{guildId}/events | guildId, data | GuildEvent |
| 185 | getGuildEvents | GET | /guilds/{guildId}/events | guildId, params? | GuildEvent[] |
| 186 | getGuildEvent | GET | /guilds/{guildId}/events/{eventId} | guildId, eventId | GuildEvent |
| 187 | updateGuildEvent | PATCH | /guilds/{guildId}/events/{eventId} | guildId, eventId, data | GuildEvent |
| 188 | deleteGuildEvent | DELETE | /guilds/{guildId}/events/{eventId} | guildId, eventId | void |
| 189 | rsvpEvent | POST | /guilds/{guildId}/events/{eventId}/rsvp | guildId, eventId, status | EventRSVP |
| 190 | deleteRsvp | DELETE | /guilds/{guildId}/events/{eventId}/rsvp | guildId, eventId | void |
| 191 | getEventRsvps | GET | /guilds/{guildId}/events/{eventId}/rsvps | guildId, eventId | EventRSVP[] |
| 192 | discoverGuilds | GET | /guilds/discover | params? | Guild[] |
| 193 | getGuildPreview | GET | /guilds/{guildId}/preview | guildId | Guild & { member_count } |
| 194 | joinGuild | POST | /guilds/{guildId}/join | guildId | JoinResult |
| 195 | warnMember | POST | /guilds/{guildId}/members/{memberId}/warn | guildId, memberId, reason | MemberWarning |
| 196 | getMemberWarnings | GET | /guilds/{guildId}/members/{memberId}/warnings | guildId, memberId | MemberWarning[] |
| 197 | deleteWarning | DELETE | /guilds/{guildId}/warnings/{warningId} | guildId, warningId | void |
| 198 | reportMessage | POST | /channels/{channelId}/messages/{messageId}/report | channelId, messageId, reason | MessageReport |
| 199 | getReports | GET | /guilds/{guildId}/reports | guildId, params? | MessageReport[] |
| 200 | resolveReport | PATCH | /guilds/{guildId}/reports/{reportId} | guildId, reportId, status | MessageReport |
| 201 | lockChannel | POST | /channels/{channelId}/lock | channelId | { locked } |
| 202 | unlockChannel | POST | /channels/{channelId}/unlock | channelId | { locked } |
| 203 | getRaidConfig | GET | /guilds/{guildId}/raid-config | guildId | RaidConfig |
| 204 | updateRaidConfig | PATCH | /guilds/{guildId}/raid-config | guildId, data | RaidConfig |
| 205 | getAutoModRules | GET | /guilds/{guildId}/automod/rules | guildId | AutoModRule[] |
| 206 | createAutoModRule | POST | /guilds/{guildId}/automod/rules | guildId, data | AutoModRule |
| 207 | getAutoModRule | GET | /guilds/{guildId}/automod/rules/{ruleId} | guildId, ruleId | AutoModRule |
| 208 | updateAutoModRule | PATCH | /guilds/{guildId}/automod/rules/{ruleId} | guildId, ruleId, data | AutoModRule |
| 209 | deleteAutoModRule | DELETE | /guilds/{guildId}/automod/rules/{ruleId} | guildId, ruleId | void |
| 210 | getAutoModActions | GET | /guilds/{guildId}/automod/actions | guildId | AutoModAction[] |
| 211 | testAutoModRule | POST | /guilds/{guildId}/automod/rules/test | guildId, data | { matched, matched_content } |
| 212 | updateRole | PATCH | /guilds/{guildId}/roles/{roleId} | guildId, roleId, data | Role |
| 213 | deleteRole | DELETE | /guilds/{guildId}/roles/{roleId} | guildId, roleId | void |
| 214 | assignRole | PUT | /guilds/{guildId}/members/{memberId}/roles/{roleId} | guildId, memberId, roleId | void |
| 215 | removeRole | DELETE | /guilds/{guildId}/members/{memberId}/roles/{roleId} | guildId, memberId, roleId | void |
| 216 | reorderRoles | PATCH | /guilds/{guildId}/roles | guildId, positions[] | Role[] |
| 217 | getOnboarding | GET | /guilds/{guildId}/onboarding | guildId | OnboardingConfig |
| 218 | updateOnboarding | PUT | /guilds/{guildId}/onboarding | guildId, data | OnboardingConfig |
| 219 | createOnboardingPrompt | POST | /guilds/{guildId}/onboarding/prompts | guildId, data | OnboardingPrompt |
| 220 | updateOnboardingPrompt | PUT | /guilds/{guildId}/onboarding/prompts/{promptId} | guildId, promptId, data | void |
| 221 | deleteOnboardingPrompt | DELETE | /guilds/{guildId}/onboarding/prompts/{promptId} | guildId, promptId | void |
| 222 | completeOnboarding | POST | /guilds/{guildId}/onboarding/complete | guildId, responses | void |
| 223 | getOnboardingStatus | GET | /guilds/{guildId}/onboarding/status | guildId | { completed } |
| 224 | getBanLists | GET | /guilds/{guildId}/ban-lists | guildId | BanList[] |
| 225 | createBanList | POST | /guilds/{guildId}/ban-lists | guildId, data | BanList |
| 226 | deleteBanList | DELETE | /guilds/{guildId}/ban-lists/{listId} | guildId, listId | void |
| 227 | getBanListEntries | GET | /guilds/{guildId}/ban-lists/{listId}/entries | guildId, listId | BanListEntry[] |
| 228 | addBanListEntry | POST | /guilds/{guildId}/ban-lists/{listId}/entries | guildId, listId, data | BanListEntry |
| 229 | removeBanListEntry | DELETE | /guilds/{guildId}/ban-lists/{listId}/entries/{entryId} | guildId, listId, entryId | void |
| 230 | exportBanList | GET | /guilds/{guildId}/ban-lists/{listId}/export | guildId, listId | any |
| 231 | importBanList | POST | /guilds/{guildId}/ban-lists/{listId}/import | guildId, listId, data | void |
| 232 | getBanListSubscriptions | GET | /guilds/{guildId}/ban-list-subscriptions | guildId | BanListSubscription[] |
| 233 | subscribeBanList | POST | /guilds/{guildId}/ban-list-subscriptions | guildId, data | BanListSubscription |
| 234 | unsubscribeBanList | DELETE | /guilds/{guildId}/ban-list-subscriptions/{subId} | guildId, subId | void |
| 235 | getPublicBanLists | GET | /ban-lists/public | (none) | BanList[] |
| 236 | followChannel | POST | /channels/{channelId}/followers | channelId, webhookData | ChannelFollower |
| 237 | getChannelFollowers | GET | /channels/{channelId}/followers | channelId | ChannelFollower[] |
| 238 | unfollowChannel | DELETE | /channels/{channelId}/followers/{followerId} | channelId, followerId | void |
| 239 | getMyBots | GET | /users/@me/bots | (none) | User[] |
| 240 | createBot | POST | /users/@me/bots | name, description? | User |
| 241 | getBot | GET | /bots/{botId} | botId | User |
| 242 | updateBot | PATCH | /bots/{botId} | botId, data | User |
| 243 | deleteBot | DELETE | /bots/{botId} | botId | void |
| 244 | getBotTokens | GET | /bots/{botId}/tokens | botId | BotToken[] |
| 245 | createBotToken | POST | /bots/{botId}/tokens | botId, name? | BotToken |
| 246 | deleteBotToken | DELETE | /bots/{botId}/tokens/{tokenId} | botId, tokenId | void |
| 247 | getBotCommands | GET | /bots/{botId}/commands | botId | SlashCommand[] |
| 248 | registerBotCommand | POST | /bots/{botId}/commands | botId, data | SlashCommand |
| 249 | updateBotCommand | PATCH | /bots/{botId}/commands/{commandId} | botId, commandId, data | SlashCommand |
| 250 | deleteBotCommand | DELETE | /bots/{botId}/commands/{commandId} | botId, commandId | void |
| 251 | getGuildStickerPacks | GET | /guilds/{guildId}/sticker-packs | guildId | StickerPack[] |
| 252 | createGuildStickerPack | POST | /guilds/{guildId}/sticker-packs | guildId, name, description? | StickerPack |
| 253 | deleteGuildStickerPack | DELETE | /guilds/{guildId}/sticker-packs/{packId} | guildId, packId | void |
| 254 | getPackStickers | GET | /guilds/{guildId}/sticker-packs/{packId}/stickers | guildId, packId | Sticker[] |
| 255 | addStickerToGuildPack | POST | /guilds/{guildId}/sticker-packs/{packId}/stickers | guildId, packId, data | Sticker |
| 256 | deleteStickerFromGuildPack | DELETE | /guilds/{guildId}/sticker-packs/{packId}/stickers/{stickerId} | guildId, packId, stickerId | void |
| 257 | getUserStickerPacks | GET | /stickers/my-packs | (none) | StickerPack[] |
| 258 | createUserStickerPack | POST | /stickers/my-packs | name, description? | StickerPack |
| 259 | getActiveSession | GET | /channels/{channelId}/activities/sessions/active | channelId | generic T |
| 260 | listActivities | GET | /activities | category? | generic T |
| 261 | createActivitySession | POST | /channels/{channelId}/activities/sessions | channelId, body | generic T |
| 262 | joinActivitySession | POST | /channels/{channelId}/activities/sessions/{sessionId}/join | channelId, sessionId | void |
| 263 | leaveActivitySession | POST | /channels/{channelId}/activities/sessions/{sessionId}/leave | channelId, sessionId | void |
| 264 | endActivitySession | POST | /channels/{channelId}/activities/sessions/{sessionId}/end | channelId, sessionId | void |
| 265 | createKanbanBoard | POST | /channels/{channelId}/experimental/kanban | channelId, body | generic T |
| 266 | getKanbanBoard | GET | /channels/{channelId}/experimental/kanban/{boardId} | channelId, boardId | generic T |
| 267 | createKanbanColumn | POST | /channels/{channelId}/experimental/kanban/{boardId}/columns | channelId, boardId, body | void |
| 268 | createKanbanCard | POST | /channels/{channelId}/experimental/kanban/{boardId}/columns/{columnId}/cards | channelId, boardId, columnId, body | void |
| 269 | moveKanbanCard | PATCH | /channels/{channelId}/experimental/kanban/{boardId}/cards/{cardId}/move | channelId, boardId, cardId, body | void |
| 270 | deleteKanbanCard | DELETE | /channels/{channelId}/experimental/kanban/{boardId}/cards/{cardId} | channelId, boardId, cardId | void |
| 271 | reportUser | POST | /users/{userId}/report | userId, reason, contextGuildId?, contextChannelId? | { id, status } |
| 272 | reportMessageToAdmins | POST | /channels/{channelId}/messages/{messageId}/report-admin | channelId, messageId, reason | MessageReport |
| 273 | createIssue | POST | /issues | title, description, category | { id, status } |
| 274 | getMyIssues | GET | /users/@me/issues | (none) | ReportedIssue[] |
| 275 | getModerationStats | GET | /moderation/stats | (none) | ModerationStats |
| 276 | getModerationUserReports | GET | /moderation/user-reports | status? | UserReport[] |
| 277 | resolveModerationUserReport | PATCH | /moderation/user-reports/{reportId} | reportId, status, notes? | void |
| 278 | getModerationMessageReports | GET | /moderation/message-reports | status? | ModerationMessageReport[] |
| 279 | resolveModerationMessageReport | PATCH | /moderation/message-reports/{reportId} | reportId, status, notes? | void |
| 280 | getModerationIssues | GET | /moderation/issues | status? | ReportedIssue[] |
| 281 | resolveModerationIssue | PATCH | /moderation/issues/{issueId} | issueId, status, notes? | void |
| 282 | setGlobalMod | POST | /admin/users/{userId}/set-globalmod | userId, globalMod | void |
| 283 | getGuildRetentionPolicies | GET | /guilds/{guildId}/retention | guildId | RetentionPolicy[] |
| 284 | createGuildRetentionPolicy | POST | /guilds/{guildId}/retention | guildId, policy | RetentionPolicy |
| 285 | updateGuildRetentionPolicy | PATCH | /guilds/{guildId}/retention/{policyId} | guildId, policyId, update | RetentionPolicy |
| 286 | deleteGuildRetentionPolicy | DELETE | /guilds/{guildId}/retention/{policyId} | guildId, policyId | void |
| 287 | getForumTags | GET | /channels/{channelId}/tags | channelId | ForumTag[] |
| 288 | createForumTag | POST | /channels/{channelId}/tags | channelId, tag | ForumTag |
| 289 | updateForumTag | PATCH | /channels/{channelId}/tags/{tagId} | channelId, tagId, update | ForumTag |
| 290 | deleteForumTag | DELETE | /channels/{channelId}/tags/{tagId} | channelId, tagId | void |
| 291 | getForumPosts | GET | /channels/{channelId}/posts | channelId, params? | ForumPost[] |
| 292 | createForumPost | POST | /channels/{channelId}/posts | channelId, post | ForumPost |
| 293 | pinForumPost | POST | /channels/{channelId}/posts/{postId}/pin | channelId, postId | void |
| 294 | closeForumPost | POST | /channels/{channelId}/posts/{postId}/close | channelId, postId | void |
| 295 | getGalleryTags | GET | /channels/{channelId}/gallery-tags | channelId | GalleryTag[] |
| 296 | createGalleryTag | POST | /channels/{channelId}/gallery-tags | channelId, tag | GalleryTag |
| 297 | updateGalleryTag | PATCH | /channels/{channelId}/gallery-tags/{tagId} | channelId, tagId, update | GalleryTag |
| 298 | deleteGalleryTag | DELETE | /channels/{channelId}/gallery-tags/{tagId} | channelId, tagId | void |
| 299 | getGalleryPosts | GET | /channels/{channelId}/gallery-posts | channelId, params? | GalleryPost[] |
| 300 | createGalleryPost | POST | /channels/{channelId}/gallery-posts | channelId, post | GalleryPost |
| 301 | pinGalleryPost | POST | /channels/{channelId}/gallery-posts/{postId}/pin | channelId, postId | void |
| 302 | closeGalleryPost | POST | /channels/{channelId}/gallery-posts/{postId}/close | channelId, postId | void |

**Note:** Additionally, the `ApiClient` class exports helper infrastructure: `setToken()`, `getToken()`, and the private `request()`, `get()`, `post()`, `patch()`, `put()`, `del()` methods. The `ApiRequestError` class is also exported.

---

### WebSocket Event Handlers

**File:** `/docker/AmityVox/web/src/lib/stores/gateway.ts`
**Total Event Types Handled: 23**

| # | Event Type | Stores Updated | Action |
|---|-----------|----------------|--------|
| 1 | READY | auth, guilds, dms, unreads, relationships, muting, presence, voice, channels, messages, toast | Sets current user, loads guilds (local + federated), DMs, read state, relationships, mute prefs. Populates presences and voice states from payload. Starts idle detection. On reconnect: clears and reloads active channel messages, shows reconnect toast. |
| 2 | GATEWAY_DISCONNECTED | gateway | Sets gatewayConnected to false, shows warning toast. |
| 3 | GATEWAY_AUTH_FAILED | gateway | Disconnects gateway, redirects to /login. |
| 4 | GATEWAY_EXHAUSTED | gateway | Sets gatewayConnected to false (too many reconnect failures). |
| 5 | GUILD_CREATE | guilds, permissions | Adds/updates guild in store, loads permissions for guild. |
| 6 | GUILD_UPDATE | guilds, permissions | Same as GUILD_CREATE. |
| 7 | GUILD_DELETE | guilds, permissions | Removes guild from store, invalidates cached permissions. |
| 8 | CHANNEL_CREATE | channels, dms | Adds/updates channel; if DM/group type, also adds to DM store. |
| 9 | CHANNEL_UPDATE | channels, dms | Same as CHANNEL_CREATE. |
| 10 | THREAD_CREATE | channels, dms | Same as CHANNEL_CREATE. |
| 11 | CHANNEL_DELETE | channels, dms | Removes channel from both channels and DM stores. |
| 12 | MESSAGE_CREATE | messages, channels, unreads, notifications | Appends message to channel. Updates thread last_activity_at. Increments unreads if from another user. Builds and adds notification for mentions, replies, DMs (respecting mute state). |
| 13 | MESSAGE_UPDATE | messages | Updates the message in the messages store. |
| 14 | MESSAGE_DELETE | messages | Removes a single message from the messages store. |
| 15 | MESSAGE_DELETE_BULK | messages | Removes multiple messages by ID array from the messages store. |
| 16 | PRESENCE_UPDATE | presence | Updates user's presence status (online/idle/dnd/offline). |
| 17 | TYPING_START | typing | Adds user to typing indicators for the specified channel. |
| 18 | USER_UPDATE | auth, members, dms | Updates current user if self. Updates user data in members store and DM recipients across all stores. |
| 19 | VOICE_STATE_UPDATE | voice, callRing | Updates voice participant state (join/leave/update). Dismisses incoming call ring on leave. |
| 20 | CALL_RING | callRing | Adds incoming call entry for DM/group call ringing UI. |
| 21 | GUILD_MEMBER_UPDATE | permissions, members | Reloads permissions if self's roles changed. Updates member roles in member store. |
| 22 | GUILD_ROLE_UPDATE | permissions | Reloads permissions for the affected guild. |
| 23 | GUILD_ROLE_DELETE | permissions | Reloads permissions for the affected guild. |
| 24 | RELATIONSHIP_ADD | relationships, notifications | Adds/updates relationship. If pending_incoming, adds friend request notification. |
| 25 | RELATIONSHIP_UPDATE | relationships | Updates existing relationship. |
| 26 | RELATIONSHIP_REMOVE | relationships | Removes relationship by target_id. |
| 27 | ANNOUNCEMENT_CREATE | announcements | Adds new announcement to store. |
| 28 | ANNOUNCEMENT_UPDATE | announcements | Updates existing announcement fields. |
| 29 | ANNOUNCEMENT_DELETE | announcements | Removes announcement by ID. |

---

### Component Inventory

**Total Components: 105**

#### admin/ (6 components)

| Component | Purpose |
|-----------|---------|
| BackupScheduler.svelte | Manage automated backup schedules |
| DomainSettings.svelte | Configure instance domain settings |
| HealthMonitor.svelte | Monitor system health metrics |
| RetentionSettings.svelte | Configure message retention policies |
| StorageDashboard.svelte | View and manage S3/file storage |
| UpdateNotifications.svelte | Show available software updates |

#### channels/ (10 components)

| Component | Purpose |
|-----------|---------|
| ActivityFrame.svelte | Embed frame for channel activities |
| ChannelSettingsPanel.svelte | Channel configuration panel |
| ForumChannelView.svelte | Forum-type channel view |
| ForumPostCard.svelte | Individual forum post card display |
| ForumPostCreate.svelte | Create new forum post form |
| GalleryChannelView.svelte | Gallery-type channel view |
| GalleryPostCard.svelte | Individual gallery post card display |
| GalleryPostCreate.svelte | Create new gallery post form |
| KanbanBoard.svelte | Kanban board for channel tasks |
| StageChannelView.svelte | Stage channel audience view |
| Whiteboard.svelte | Collaborative whiteboard canvas |
| WidgetPanel.svelte | Embeddable widget panel |

#### chat/ (17 components)

| Component | Purpose |
|-----------|---------|
| AudioPlayer.svelte | Play audio attachments/voice messages |
| CodeSnippet.svelte | Syntax-highlighted code block display |
| CrossChannelQuote.svelte | Display quoted messages from other channels |
| EditHistoryModal.svelte | View message edit history |
| LocationShare.svelte | Share geographic location |
| MarkdownRenderer.svelte | Render markdown content in messages |
| MessageEffects.svelte | Visual effects for messages (confetti, etc.) |
| MessageInput.svelte | Message composition input with attachments |
| MessageItem.svelte | Individual message display with actions |
| MessageList.svelte | Scrollable message list with virtualization |
| PinnedMessages.svelte | View pinned messages in a channel |
| ReadOnlyBanner.svelte | Banner for locked/read-only channels |
| SearchModal.svelte | Search messages modal dialog |
| ThreadPanel.svelte | Thread side panel for message threads |
| TranslateButton.svelte | Translate message to another language |
| TypingIndicator.svelte | Show who is typing in channel |
| VideoPlayer.svelte | Play video attachments |
| VideoRecorder.svelte | Record video messages inline |
| VoiceMessageRecorder.svelte | Record voice messages |

#### common/ (27 components)

| Component | Purpose |
|-----------|---------|
| AnnouncementBanner.svelte | Display instance-wide announcements |
| Avatar.svelte | User/guild avatar display with fallback |
| BridgeAttribution.svelte | Show bridge source attribution (Matrix/Discord/etc.) |
| CommandPalette.svelte | Ctrl+K command palette for quick actions |
| ConnectionIndicator.svelte | Show WebSocket connection quality |
| ContextMenu.svelte | Right-click context menu container |
| ContextMenuDivider.svelte | Divider line in context menus |
| ContextMenuItem.svelte | Individual context menu item |
| EmojiPicker.svelte | Emoji selection picker |
| ExportMessagesButton.svelte | Export channel messages to file |
| FederationBadge.svelte | Badge indicating federated content |
| GiphyPicker.svelte | Search and select GIFs from Giphy |
| GroupDMCreateModal.svelte | Create group DM modal |
| GroupDMSettingsPanel.svelte | Group DM settings panel |
| ImageCropper.svelte | Crop images for avatars/banners |
| ImageLightbox.svelte | Full-screen image viewer |
| IncomingCallModal.svelte | Incoming voice/video call modal |
| KeyboardNav.svelte | Keyboard navigation helper |
| KeyboardShortcuts.svelte | Keyboard shortcut reference overlay |
| LazyImage.svelte | Lazy-loaded image with blurhash placeholder |
| Modal.svelte | Reusable modal dialog wrapper |
| ModerationModals.svelte | Kick/ban/warn moderation modals |
| NotificationCenter.svelte | Notification center dropdown |
| ProfileLinkEditor.svelte | Edit profile social links |
| ProfileModal.svelte | User profile modal with details |
| QuickSwitcher.svelte | Quick channel/guild switcher |
| ResizeHandle.svelte | Draggable resize handle for panels |
| StatusPicker.svelte | User status/presence picker |
| StickerPicker.svelte | Sticker selection picker |
| ToastContainer.svelte | Container for toast notifications |
| UserPopover.svelte | User info popover on hover/click |

#### encryption/ (1 component)

| Component | Purpose |
|-----------|---------|
| EncryptionPanel.svelte | E2EE channel encryption controls |

#### gallery/ (6 components)

| Component | Purpose |
|-----------|---------|
| AdminMediaPanel.svelte | Admin media management panel |
| GalleryFilters.svelte | Filter controls for gallery view |
| GalleryItem.svelte | Individual gallery media item |
| GalleryPanel.svelte | Gallery panel for channel media |
| MediaPreviewModal.svelte | Full preview modal for media files |
| TagEditor.svelte | Edit media tags |

#### guild/ (16 components)

| Component | Purpose |
|-----------|---------|
| AutoRoleSettings.svelte | Configure automatic role assignment |
| BoostPanel.svelte | Server boost information panel |
| CreateGuildModal.svelte | Create new guild modal |
| GuildInsights.svelte | Guild analytics and insights |
| GuildRetentionSettings.svelte | Guild-level retention policy settings |
| GuildTemplates.svelte | Guild template creation/application |
| IntegrationSettings.svelte | Third-party integration settings |
| InviteModal.svelte | Create/manage guild invite modal |
| LevelingSettings.svelte | XP/leveling system settings |
| MembersPanel.svelte | Guild member management panel |
| OnboardingModal.svelte | New member onboarding flow |
| PluginSettings.svelte | Plugin/extension management |
| RoleEditor.svelte | Role creation and permission editor |
| SoundboardSettings.svelte | Manage guild soundboard sounds |
| StarboardSettings.svelte | Configure starboard channel |
| WebhookPanel.svelte | Webhook management panel |
| WelcomeSettings.svelte | Welcome message configuration |
| WidgetSettings.svelte | Embeddable widget configuration |

#### home/ (4 components)

| Component | Purpose |
|-----------|---------|
| ActiveVoicePanel.svelte | Show active voice channels |
| Dashboard.svelte | Home dashboard overview |
| MyIssuesPanel.svelte | User's submitted issues list |
| OnlineFriendsPanel.svelte | Online friends quick list |

#### layout/ (7 components)

| Component | Purpose |
|-----------|---------|
| ChannelGroups.svelte | Grouped channel list with categories |
| ChannelSidebar.svelte | Left sidebar with channel list |
| GuildSidebar.svelte | Guild icon sidebar (far left) |
| InstanceSwitcher.svelte | Switch between federated instances |
| MemberList.svelte | Right sidebar member list |
| TopBar.svelte | Top navigation bar |
| VoiceConnectionBar.svelte | Bottom bar showing voice connection |

#### voice/ (9 components)

| Component | Purpose |
|-----------|---------|
| CameraSettings.svelte | Camera device and quality settings |
| ParticipantContextMenu.svelte | Right-click menu for voice participants |
| ScreenShareControls.svelte | Screen share start/stop controls |
| Soundboard.svelte | Play soundboard sounds in voice |
| Transcription.svelte | Voice-to-text transcription display |
| VideoTile.svelte | Individual video participant tile |
| VoiceBroadcast.svelte | Voice broadcast/announcement mode |
| VoiceChannelView.svelte | Voice channel participant view |
| VoiceControls.svelte | Mute/deafen/camera/screenshare controls |

#### Root (1 component)

| Component | Purpose |
|-----------|---------|
| PollDisplay.svelte | Display and interact with polls |

---

### Store Inventory

**File Directory:** `/docker/AmityVox/web/src/lib/stores/`
**Total Store Files: 27**

| # | Store File | State Managed | Key Exports | Dependencies |
|---|-----------|---------------|-------------|--------------|
| 1 | auth.ts | Current user session, authentication state | currentUser, isAuthenticated, isLoading, login(), register(), logout() | api/client |
| 2 | guilds.ts | Guild list (local + federated), current guild selection | guilds, currentGuildId, guildList, currentGuild, federatedGuilds, federatedGuildIds | api/client, permissions |
| 3 | channels.ts | Channel list, current channel, threads, hidden threads | channels, currentChannelId, channelList, textChannels, voiceChannels, forumChannels, galleryChannels, threadsByParent, hiddenThreadIds | api/client |
| 4 | messages.ts | Messages by channel (Map<channelId, Message[]>), loading state | messagesByChannel, isLoadingMessages, loadMessages(), appendMessage(), updateMessage(), removeMessage() | api/client |
| 5 | presence.ts | User presence status map (userId -> status) | presenceMap, getPresence(), updatePresence() | (none) |
| 6 | typing.ts | Typing indicators per channel with auto-expiry (8s) | currentTypingUsers, addTypingUser(), clearTypingUser() | channels, auth |
| 7 | voice.ts | LiveKit room, voice participants, video tracks, self mute/deaf/camera state, channel voice users | voiceChannelId, voiceState, selfMute, selfDeaf, voiceParticipants, videoTracks, channelVoiceUsers, joinVoice(), leaveVoice(), toggleMute(), toggleDeafen(), toggleCamera() | api/client, settings |
| 8 | dms.ts | DM channel list sorted by recency | dmChannels, dmLoaded, dmList, loadDMs(), addDMChannel(), updateUserInDMs() | api/client |
| 9 | unreads.ts | Unread counts and mention counts per channel, total unreads | unreadCounts, unreadState, mentionCounts, totalUnreads, incrementUnread(), ackChannel(), markAllRead() | api/client, channels |
| 10 | notifications.ts | In-app notification entries (mentions, replies, DMs, friend requests), max 100 | notifications, unreadNotificationCount, groupedNotifications, addNotification(), markNotificationRead(), clearAllNotifications() | channels, settings |
| 11 | relationships.ts | Friend/block relationships map (targetId -> Relationship), pending count | relationships, pendingIncomingCount, loadRelationships(), addOrUpdateRelationship(), removeRelationship() | api/client |
| 12 | permissions.ts | Computed guild permissions cache (guildId -> bigint), convenience derived stores | guildPermissions, currentGuildPermissions, canManageChannels, canManageGuild, canManageMessages, canKickMembers, canBanMembers, isAdministrator, etc. | api/client, guilds, types |
| 13 | members.ts | Guild member list, roles map, member timeouts | guildMembers, guildRolesMap, memberTimeouts, setGuildMembers(), updateGuildMember(), updateUserInMembers() | (none) |
| 14 | muting.ts | Channel and guild mute preferences | channelMutePrefs, guildMutePrefs, isChannelMuted(), isGuildMuted(), muteChannel(), unmuteChannel(), muteGuild(), unmuteGuild() | api/client |
| 15 | settings.ts | Custom themes, DND schedule, notification sound preferences, custom CSS | customThemes, activeCustomThemeName, dndSchedule, isDndActive, notificationSoundsEnabled, notificationSoundPreset, notificationVolume, customCss | api/client |
| 16 | layout.ts | Resizable panel widths (channel sidebar, member list), persisted to localStorage | channelSidebarWidth, memberListWidth | (none) |
| 17 | moderation.ts | Kick/ban modal targets | kickModalTarget, banModalTarget | (none) |
| 18 | announcements.ts | Instance-wide announcements with auto-expiry | activeAnnouncements, addAnnouncement(), updateAnnouncement(), removeAnnouncement() | (none) |
| 19 | callRing.ts | Incoming call state for DM/group channels with 30s auto-dismiss | incomingCalls, activeIncomingCall, incomingCallCount, addIncomingCall(), dismissIncomingCall() | (none) |
| 20 | messageInteraction.ts | Reply-to and edit-message state | replyingTo, editingMessage, startReply(), cancelReply(), startEdit(), cancelEdit() | (none) |
| 21 | toast.ts | Toast notification queue | toasts, addToast(), dismissToast() | (none) |
| 22 | navigation.ts | Recent channels, back/forward navigation history | recentChannels, canGoBack, canGoForward, pushChannel(), goBack(), goForward() | (none) |
| 23 | instances.ts | Multi-instance profiles and connections | instanceProfiles, instanceConnections, activeInstance, crossInstanceUnreadCount, crossInstanceMentionCount | (none) |
| 24 | gateway.reconnect.ts | Connection quality, latency, reconnect backoff scheduling | connectionQuality, connectionLatency, reconnectAttempt, isConnected, isReconnecting, calculateBackoffDelay(), createReconnectScheduler() | (none) |
| 25 | nicknames.ts | Client-side personal nicknames (localStorage only) | clientNicknames, getClientNickname(), setClientNickname() | (none) |
| 26 | blocked.ts | Blocked users with two-tier levels (ignore/block) | blockedUsers, blockedUserIds, loadBlockedUsers(), addBlockedUser(), isBlocked(), getBlockLevel() | api/client |
| 27 | gateway.ts | WebSocket connection management and event dispatch | gatewayConnected, connectGateway(), disconnectGateway(), getGatewayClient() | All other stores (dispatches events) |

---

### Type Inventory

**File:** `/docker/AmityVox/web/src/lib/types/index.ts`
**Total Types/Interfaces: 56 (+ 2 const objects + 1 function)**

| # | Type/Interface | Fields | Category |
|---|---------------|--------|----------|
| 1 | User | 22 fields | Core entity |
| 2 | UserLink | 8 fields | User profile |
| 3 | MutualGuild | 3 fields | User relationships |
| 4 | Guild | 17 fields | Core entity |
| 5 | Channel | 25 fields | Core entity |
| 6 | ChannelType (type alias) | 8 variants: text, voice, dm, group, announcement, forum, gallery, stage | Channel enum |
| 7 | Message | 26 fields | Core entity |
| 8 | MessageType (type alias) | 11 variants | Message enum |
| 9 | ScheduledMessage | 6 fields | Messaging |
| 10 | Reaction | 3 fields | Messaging |
| 11 | Attachment | 15 fields | Media |
| 12 | MediaTag | 5 fields | Media |
| 13 | Embed | 17 fields | Messaging |
| 14 | Role | 10 fields | Permissions |
| 15 | GuildMember | 10 fields | Core entity |
| 16 | Invite | 10 fields | Guild management |
| 17 | Category | 4 fields | Guild structure |
| 18 | GatewayMessage | 4 fields | WebSocket |
| 19 | GatewayOp (const) | 12 opcodes | WebSocket |
| 20 | FederatedGuild | 8 fields | Federation |
| 21 | ReadyEvent | 5 fields | WebSocket |
| 22 | TypingEvent | 3 fields | WebSocket |
| 23 | PresenceUpdateEvent | 2 fields | WebSocket |
| 24 | AdminStats | 16 fields | Admin |
| 25 | InstanceInfo | 8 fields | Admin |
| 26 | Ban | 5 fields | Moderation |
| 27 | AuditLogEntry | 8 fields | Moderation |
| 28 | CustomEmoji | 6 fields | Emoji |
| 29 | Session | 6 fields | Auth |
| 30 | ReadState | 3 fields | Unreads |
| 31 | Relationship | 6 fields | Social |
| 32 | Poll | 11 fields | Messaging |
| 33 | PollOption | 4 fields | Messaging |
| 34 | MessageBookmark | 6 fields | Messaging |
| 35 | GuildEvent | 13 fields | Events |
| 36 | EventRSVP | 5 fields | Events |
| 37 | MemberWarning | 7 fields | Moderation |
| 38 | MessageReport | 11 fields | Moderation |
| 39 | RaidConfig | 8 fields | Moderation |
| 40 | AutoModRule | 13 fields | Moderation |
| 41 | AutoModAction | 9 fields | Moderation |
| 42 | UserBadge | 3 fields | User profile |
| 43 | InstanceBan | 7 fields | Admin |
| 44 | RegistrationSettings | 2 fields | Admin |
| 45 | RegistrationToken | 8 fields | Admin |
| 46 | AnnouncementSeverity (type alias) | 3 variants | Admin |
| 47 | Announcement | 8 fields | Admin |
| 48 | BotToken | 5 fields | Bots |
| 49 | SlashCommand | 7 fields | Bots |
| 50 | ApiResponse | 1 field (generic) | API envelope |
| 51 | ApiError | 1 field (nested) | API envelope |
| 52 | LoginResponse | 2 fields | Auth |
| 53 | RegisterResponse | 2 fields | Auth |
| 54 | NotificationPreference | 6 fields | Notifications |
| 55 | ChannelNotificationPreference | 4 fields | Notifications |
| 56 | Webhook | 9 fields | Integrations |
| 57 | UserSettings | 10+ fields (includes index sig) | User preferences |
| 58 | FederationPeer | 8 fields | Federation |
| 59 | OnboardingConfig | 5 fields | Guild setup |
| 60 | OnboardingPrompt | 6 fields | Guild setup |
| 61 | OnboardingOption | 6 fields | Guild setup |
| 62 | BanList | 6 fields | Moderation |
| 63 | BanListEntry | 6 fields | Moderation |
| 64 | BanListSubscription | 6 fields | Moderation |
| 65 | StickerPack | 8 fields | Stickers |
| 66 | Sticker | 7 fields | Stickers |
| 67 | ChannelFollower | 6 fields | Integrations |
| 68 | UserReport | 13 fields | Moderation |
| 69 | ReportedIssue | 11 fields | Moderation |
| 70 | ModerationStats | 3 fields | Moderation |
| 71 | ModerationMessageReport | 11 fields | Moderation |
| 72 | VoicePreferences | 13 fields | Voice |
| 73 | RetentionPolicy | 12 fields | Data management |
| 74 | ForumTag | 6 fields | Forums |
| 75 | ForumPost | 10 fields | Forums |
| 76 | GalleryTag | 6 fields | Gallery |
| 77 | GalleryPost | 11 fields | Gallery |
| 78 | Permission (const object) | 30 permission bits | Permissions |
| 79 | hasPermission (function) | 2 params | Permissions |

---

### Route Inventory

**Total Routes: 25 (17 pages + 8 layouts/pages)**

| # | Route Path | File Type | Layout | Purpose |
|---|-----------|-----------|--------|---------|
| 1 | / | +page.svelte | root layout | Landing/redirect page |
| 2 | / | +layout.svelte | (root) | Root layout wrapper |
| 3 | /login | +page.svelte | root | Login form |
| 4 | /register | +page.svelte | root | Registration form |
| 5 | /setup | +page.svelte | root | Initial instance setup |
| 6 | /invite/[code] | +page.svelte | root | Accept guild invite |
| 7 | /app | +layout.svelte | root | Authenticated app shell (gateway, sidebar) |
| 8 | /app | +page.svelte | app layout | Home dashboard |
| 9 | /app/friends | +page.svelte | app layout | Friends list and requests |
| 10 | /app/bookmarks | +page.svelte | app layout | Saved message bookmarks |
| 11 | /app/discover | +page.svelte | app layout | Server discovery browser |
| 12 | /app/settings | +page.svelte | app layout | User settings |
| 13 | /app/themes | +page.svelte | app layout | Theme customization |
| 14 | /app/plugins | +page.svelte | app layout | Plugin management |
| 15 | /app/moderation | +page.svelte | app layout | Global moderation panel |
| 16 | /app/dms/[channelId] | +page.svelte | app layout | DM conversation view |
| 17 | /app/guilds/[guildId] | +layout.svelte | app layout | Guild layout (loads channels, members) |
| 18 | /app/guilds/[guildId] | +page.svelte | guild layout | Guild home/default channel redirect |
| 19 | /app/guilds/[guildId]/channels/[channelId] | +page.svelte | guild layout | Channel message view |
| 20 | /app/guilds/[guildId]/settings | +page.svelte | guild layout | Guild settings |
| 21 | /app/admin | +layout.svelte | app layout | Admin layout wrapper |
| 22 | /app/admin | +page.svelte | admin layout | Admin dashboard |
| 23 | /app/admin/federation | +page.svelte | admin layout | Federation peer management |
| 24 | /app/admin/bridges | +page.svelte | admin layout | Bridge adapter management |
| 25 | /app/embed/[guildId] | +page.svelte | app layout | Embeddable guild widget |

---

### Test Coverage

**Total Test Files: 33**

| # | Test File | Tests Target | Location |
|---|----------|--------------|----------|
| 1 | presence.test.ts | presence store (updatePresence, getPresence, removePresence) | stores/__tests__/ |
| 2 | toast.test.ts | toast store (addToast, dismissToast, auto-dismiss) | stores/__tests__/ |
| 3 | messages.test.ts | messages store (loadMessages, appendMessage, updateMessage, removeMessage, dedup) | stores/__tests__/ |
| 4 | messageInteraction.test.ts | messageInteraction store (startReply, cancelReply, startEdit, cancelEdit) | stores/__tests__/ |
| 5 | dms.test.ts | dms store (loadDMs, addDMChannel, removeDMChannel, updateUserInDMs) | stores/__tests__/ |
| 6 | unreads.test.ts | unreads store (incrementUnread, ackChannel, markAllRead, mentionCounts) | stores/__tests__/ |
| 7 | relationships.test.ts | relationships store (loadRelationships, addOrUpdate, remove, pendingCount) | stores/__tests__/ |
| 8 | channels.test.ts | channels store (loadChannels, updateChannel, removeChannel, derived stores) | stores/__tests__/ |
| 9 | permissions.test.ts | permissions store (loadPermissions, invalidatePermissions, derived flags) | stores/__tests__/ |
| 10 | muting.test.ts | muting store (isChannelMuted, isGuildMuted, muteChannel, unmuteChannel) | stores/__tests__/ |
| 11 | blocked.test.ts | blocked store (loadBlockedUsers, addBlockedUser, isBlocked, getBlockLevel) | stores/__tests__/ |
| 12 | guilds.test.ts | guilds store (loadGuilds, updateGuild, removeGuild, federation helpers) | stores/__tests__/ |
| 13 | MarkdownRenderer.test.ts | Markdown rendering logic (bold, italic, code, links, mentions, spoilers) | components/__tests__/ |
| 14 | GiphyPicker.test.ts | GiphyPicker logic (search, trending, click outside handling) | components/__tests__/ |
| 15 | HandleUtils.test.ts | Handle resolution utility functions | components/__tests__/ |
| 16 | MembersPanel.test.ts | MembersPanel logic (member list, role display, sorting) | components/__tests__/ |
| 17 | RoleEditor.test.ts | RoleEditor logic (permission toggles, role creation) | components/__tests__/ |
| 18 | EncryptionPanel.test.ts | EncryptionPanel logic (passphrase handling, channel encryption) | components/__tests__/ |
| 19 | RoleHierarchy.test.ts | Role hierarchy and permission resolution | components/__tests__/ |
| 20 | ModerationModals.test.ts | Moderation modal logic (kick, ban, warn flows) | components/__tests__/ |
| 21 | StatusPicker.test.ts | StatusPicker logic (status selection, custom status) | components/__tests__/ |
| 22 | VoiceVideoSettings.test.ts | Voice/video settings logic (device selection, quality) | components/__tests__/ |
| 23 | GalleryChannelView.test.ts | Gallery channel view logic | components/__tests__/ |
| 24 | GalleryPostCard.test.ts | Gallery post card rendering logic | components/__tests__/ |
| 25 | ForumChannelView.test.ts | Forum channel view logic | components/__tests__/ |
| 26 | GuildRetentionSettings.test.ts | Guild retention settings logic | components/__tests__/ |
| 27 | ForumPostCard.test.ts | Forum post card rendering logic | components/__tests__/ |
| 28 | crypto.test.ts | Encryption crypto utilities (encrypt, decrypt, key derivation) | encryption/ |
| 29 | roleColor.test.ts | Role color utility functions | utils/__tests__/ |
| 30 | idle.test.ts | Idle detection utility | utils/__tests__/ |
| 31 | gifFavorites.test.ts | GIF favorites localStorage management | utils/__tests__/ |
| 32 | emoji.test.ts | Emoji utility functions (parsing, rendering) | utils/__tests__/ |
| 33 | dm.test.ts | DM utility functions (recipient display, sorting) | utils/__tests__/ |

---

### Summary Statistics

| Metric | Count |
|--------|-------|
| API Client Methods | 302 |
| WebSocket Event Types | 29 (23 unique switch cases + sub-handlers) |
| Svelte Components | 105 |
| Store Files | 27 |
| TypeScript Types/Interfaces | 79 (including const objects and type aliases) |
| Route Pages | 17 |
| Route Layouts | 4 |
| Test Files | 33 |
| Component Directories | 10 (admin, channels, chat, common, encryption, gallery, guild, home, layout, voice) |
