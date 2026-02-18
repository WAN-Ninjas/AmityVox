# Frontend-to-Backend Wiring Map

**Audit Date:** 2026-02-18
**Frontend:** `web/src/lib/api/client.ts` (1,666 lines, 163 API methods)
**Backend:** `internal/api/server.go` (1,477 lines) + `cmd/amityvox/main.go` (federation routes)

---

## Summary

| Metric | Count |
|---|---|
| Total frontend API methods | 163 |
| Verified matches | 155 |
| Mismatches found | 6 |
| Orphaned frontend methods (no backend) | 0 |
| Note: conditional backend routes | 3 (Giphy, Encryption, Notifications - gated by config) |

---

## Mismatch Details

### MISMATCH 1: `ackWelcome` - Wrong HTTP Method and URL Path

- **Frontend method:** `ackWelcome(welcomeId)`
- **Frontend call:** `POST /api/v1/encryption/welcome/${welcomeId}/ack`
- **Backend route:** `DELETE /api/v1/encryption/welcome/{welcomeID}`
- **Severity:** HIGH - Two issues:
  1. HTTP method mismatch: frontend sends POST, backend expects DELETE
  2. URL path mismatch: frontend appends `/ack` suffix, backend route has no `/ack` suffix
- **Impact:** This call will always return 404/405. Encryption welcome acknowledgment is broken.
- **Fix:** Change frontend to `this.del(\`/encryption/welcome/${welcomeId}\`)` or change backend to register `POST /welcome/{welcomeID}/ack`

### MISMATCH 2: `deleteKeyPackage` - URL Parameter Name Mismatch

- **Frontend method:** `deleteKeyPackage(keyPackageId)`
- **Frontend call:** `DELETE /api/v1/encryption/key-packages/${keyPackageId}`
- **Backend route:** `DELETE /api/v1/encryption/key-packages/{packageID}`
- **Severity:** LOW - Chi router uses `{packageID}` as the parameter name; the actual URL path works fine since chi matches any segment. However, if the backend handler uses `chi.URLParam(r, "packageID")`, the frontend-supplied value in the URL segment will be correctly extracted regardless of the JavaScript variable name. **No runtime issue, but naming inconsistency should be documented.**
- **Impact:** None at runtime. Cosmetic naming discrepancy.

### MISMATCH 3: `getActiveSession` - URL Path Mismatch (Channel-Scoped vs Activity-Scoped)

- **Frontend method:** `getActiveSession(channelId)`
- **Frontend call:** `GET /api/v1/channels/${channelId}/activities/sessions/active`
- **Backend route:** `GET /api/v1/activities/{activityID}/sessions/active`
- **Severity:** HIGH - The frontend sends a channel-scoped URL (`/channels/{channelID}/activities/...`) but the backend registers the route under `/activities/{activityID}/...`. These are entirely different route trees.
- **Impact:** This call will return 404. Active session lookup by channel is broken.
- **Fix:** Either add a channel-scoped route on the backend, or change the frontend to use the activity-scoped URL.

### MISMATCH 4: `createActivitySession` - URL Path Mismatch (Channel-Scoped vs Activity-Scoped)

- **Frontend method:** `createActivitySession(channelId, body)`
- **Frontend call:** `POST /api/v1/channels/${channelId}/activities/sessions`
- **Backend route:** `POST /api/v1/activities/{activityID}/sessions`
- **Severity:** HIGH - Same issue as MISMATCH 3. The frontend uses channel-scoped URL, backend uses activity-scoped URL.
- **Impact:** This call will return 404. Activity session creation is broken.
- **Fix:** Same as MISMATCH 3.

### MISMATCH 5: `joinActivitySession` / `leaveActivitySession` / `endActivitySession` - URL Path Mismatch

- **Frontend methods:** `joinActivitySession(channelId, sessionId)`, `leaveActivitySession(channelId, sessionId)`, `endActivitySession(channelId, sessionId)`
- **Frontend calls:**
  - `POST /api/v1/channels/${channelId}/activities/sessions/${sessionId}/join`
  - `POST /api/v1/channels/${channelId}/activities/sessions/${sessionId}/leave`
  - `POST /api/v1/channels/${channelId}/activities/sessions/${sessionId}/end`
- **Backend routes:**
  - `POST /api/v1/activities/sessions/{sessionID}/join`
  - `POST /api/v1/activities/sessions/{sessionID}/leave`
  - `POST /api/v1/activities/sessions/{sessionID}/end`
- **Severity:** HIGH - Frontend nests these under `/channels/{channelID}/activities/...` but backend registers them directly under `/activities/sessions/...`.
- **Impact:** All three calls will return 404.
- **Fix:** Align URL structures.

### MISMATCH 6: `listActivities` - URL Path Discrepancy (Minor)

- **Frontend method:** `listActivities(category?)`
- **Frontend call:** `GET /api/v1/activities` or `GET /api/v1/activities?category=...`
- **Backend route:** `GET /api/v1/activities/`
- **Severity:** NONE - Chi router handles trailing slash normalization. This matches correctly.
- **Status:** VERIFIED OK (included for completeness)

---

## Verified Wiring Table

### Auth

| Frontend Method | HTTP | URL | Backend Handler | Status |
|---|---|---|---|---|
| `register` | POST | `/auth/register` | `s.handleRegister` | OK |
| `login` | POST | `/auth/login` | `s.handleLogin` | OK |
| `logout` | POST | `/auth/logout` | `s.handleLogout` | OK |
| `changePassword` | POST | `/auth/password` | `s.handleChangePassword` | OK |
| `enableTOTP` | POST | `/auth/totp/enable` | `s.handleTOTPEnable` | OK |
| `verifyTOTP` | POST | `/auth/totp/verify` | `s.handleTOTPVerify` | OK |
| `disableTOTP` | DELETE | `/auth/totp` | `s.handleTOTPDisable` | OK |
| `generateBackupCodes` | POST | `/auth/backup-codes` | `s.handleGenerateBackupCodes` | OK |

### Users

| Frontend Method | HTTP | URL | Backend Handler | Status |
|---|---|---|---|---|
| `getMe` | GET | `/users/@me` | `userH.HandleGetSelf` | OK |
| `updateMe` | PATCH | `/users/@me` | `userH.HandleUpdateSelf` | OK |
| `getUser` | GET | `/users/{userID}` | `userH.HandleGetUser` | OK |
| `getUserBadges` | GET | `/users/{userID}/badges` | `userH.HandleGetUserBadges` | OK |
| `getUserLinks` | GET | `/users/{userID}/links` | `userH.HandleGetUserLinks` | OK |
| `getMyLinks` | GET | `/users/@me/links` | `userH.HandleGetMyLinks` | OK |
| `createLink` | POST | `/users/@me/links` | `userH.HandleCreateLink` | OK |
| `updateLink` | PATCH | `/users/@me/links/{linkID}` | `userH.HandleUpdateLink` | OK |
| `deleteLink` | DELETE | `/users/@me/links/{linkID}` | `userH.HandleDeleteLink` | OK |
| `getMyGuilds` | GET | `/users/@me/guilds` | `userH.HandleGetSelfGuilds` | OK |
| `getMyDMs` | GET | `/users/@me/dms` | `userH.HandleGetSelfDMs` | OK |
| `createDM` | POST | `/users/{userID}/dm` | `userH.HandleCreateDM` | OK |
| `createGroupDM` | POST | `/users/@me/group-dms` | `userH.HandleCreateGroupDM` | OK |
| `resolveHandle` | GET | `/users/resolve?handle=...` | `userH.HandleResolveHandle` | OK |
| `getUserNote` | GET | `/users/{userID}/note` | `userH.HandleGetUserNote` | OK |
| `setUserNote` | PUT | `/users/{userID}/note` | `userH.HandleSetUserNote` | OK |
| `getMutualFriends` | GET | `/users/{userID}/mutual-friends` | `userH.HandleGetMutualFriends` | OK |
| `getMutualGuilds` | GET | `/users/{userID}/mutual-guilds` | `userH.HandleGetMutualGuilds` | OK |
| `getUserSettings` | GET | `/users/@me/settings` | `userH.HandleGetUserSettings` | OK |
| `updateUserSettings` | PATCH | `/users/@me/settings` | `userH.HandleUpdateUserSettings` | OK |

### Friends / Blocks

| Frontend Method | HTTP | URL | Backend Handler | Status |
|---|---|---|---|---|
| `getFriends` | GET | `/users/@me/relationships` | `userH.HandleGetRelationships` | OK |
| `addFriend` | PUT | `/users/{userID}/friend` | `userH.HandleAddFriend` | OK |
| `removeFriend` | DELETE | `/users/{userID}/friend` | `userH.HandleRemoveFriend` | OK |
| `blockUser` | PUT | `/users/{userID}/block` | `userH.HandleBlockUser` | OK |
| `updateBlockLevel` | PATCH | `/users/{userID}/block` | `userH.HandleUpdateBlockLevel` | OK |
| `unblockUser` | DELETE | `/users/{userID}/block` | `userH.HandleUnblockUser` | OK |
| `getBlockedUsers` | GET | `/users/@me/blocked` | `userH.HandleGetBlockedUsers` | OK |

### Sessions

| Frontend Method | HTTP | URL | Backend Handler | Status |
|---|---|---|---|---|
| `getSessions` | GET | `/users/@me/sessions` | `userH.HandleGetSelfSessions` | OK |
| `deleteSession` | DELETE | `/users/@me/sessions/{sessionID}` | `userH.HandleDeleteSelfSession` | OK |

### Read State

| Frontend Method | HTTP | URL | Backend Handler | Status |
|---|---|---|---|---|
| `getReadState` | GET | `/users/@me/read-state` | `userH.HandleGetSelfReadState` | OK |
| `ackChannel` | POST | `/channels/{channelID}/ack` | `channelH.HandleAckChannel` | OK |

### Guilds

| Frontend Method | HTTP | URL | Backend Handler | Status |
|---|---|---|---|---|
| `createGuild` | POST | `/guilds` | `guildH.HandleCreateGuild` | OK |
| `getGuild` | GET | `/guilds/{guildID}` | `guildH.HandleGetGuild` | OK |
| `updateGuild` | PATCH | `/guilds/{guildID}` | `guildH.HandleUpdateGuild` | OK |
| `deleteGuild` | DELETE | `/guilds/{guildID}` | `guildH.HandleDeleteGuild` | OK |
| `leaveGuild` | POST | `/guilds/{guildID}/leave` | `guildH.HandleLeaveGuild` | OK |
| `getMyPermissions` | GET | `/guilds/{guildID}/members/@me/permissions` | `guildH.HandleGetMyPermissions` | OK |
| `discoverGuilds` | GET | `/guilds/discover` | `guildH.HandleDiscoverGuilds` | OK |
| `getGuildPreview` | GET | `/guilds/{guildID}/preview` | `guildH.HandleGetGuildPreview` | OK |
| `joinGuild` | POST | `/guilds/{guildID}/join` | `guildH.HandleJoinDiscoverableGuild` | OK |

### Channels

| Frontend Method | HTTP | URL | Backend Handler | Status |
|---|---|---|---|---|
| `getGuildChannels` | GET | `/guilds/{guildID}/channels` | `guildH.HandleGetGuildChannels` | OK |
| `createChannel` | POST | `/guilds/{guildID}/channels` | `guildH.HandleCreateGuildChannel` | OK |
| `getChannel` | GET | `/channels/{channelID}` | `channelH.HandleGetChannel` | OK |
| `updateChannel` | PATCH | `/channels/{channelID}` | `channelH.HandleUpdateChannel` | OK |
| `deleteChannel` | DELETE | `/channels/{channelID}` | `channelH.HandleDeleteChannel` | OK |
| `cloneChannel` | POST | `/guilds/{guildID}/channels/{channelID}/clone` | `guildH.HandleCloneChannel` | OK |

### Messages

| Frontend Method | HTTP | URL | Backend Handler | Status |
|---|---|---|---|---|
| `getMessages` | GET | `/channels/{channelID}/messages` | `channelH.HandleGetMessages` | OK |
| `sendMessage` | POST | `/channels/{channelID}/messages` | `channelH.HandleCreateMessage` | OK |
| `editMessage` | PATCH | `/channels/{channelID}/messages/{messageID}` | `channelH.HandleUpdateMessage` | OK |
| `deleteMessage` | DELETE | `/channels/{channelID}/messages/{messageID}` | `channelH.HandleDeleteMessage` | OK |
| `bulkDeleteMessages` | POST | `/channels/{channelID}/messages/bulk-delete` | `channelH.HandleBulkDeleteMessages` | OK |
| `batchDecryptMessages` | POST | `/channels/{channelID}/decrypt-messages` | `channelH.HandleBatchDecryptMessages` | OK |
| `getMessageEdits` | GET | `/channels/{channelID}/messages/{messageID}/edits` | `channelH.HandleGetMessageEdits` | OK |
| `translateMessage` | POST | `/channels/{channelID}/messages/{messageID}/translate` | `channelH.HandleTranslateMessage` | OK |
| `forwardMessage` | POST | `/channels/{channelID}/messages/{messageID}/crosspost` | `channelH.HandleCrosspostMessage` | OK |

### Scheduled Messages

| Frontend Method | HTTP | URL | Backend Handler | Status |
|---|---|---|---|---|
| `scheduleMessage` | POST | `/channels/{channelID}/scheduled-messages` | `channelH.HandleScheduleMessage` | OK |
| `getScheduledMessages` | GET | `/channels/{channelID}/scheduled-messages` | `channelH.HandleGetScheduledMessages` | OK |
| `deleteScheduledMessage` | DELETE | `/channels/{channelID}/scheduled-messages/{messageID}` | `channelH.HandleDeleteScheduledMessage` | OK |

### Pins

| Frontend Method | HTTP | URL | Backend Handler | Status |
|---|---|---|---|---|
| `getPins` | GET | `/channels/{channelID}/pins` | `channelH.HandleGetPins` | OK |
| `pinMessage` | PUT | `/channels/{channelID}/pins/{messageID}` | `channelH.HandlePinMessage` | OK |
| `unpinMessage` | DELETE | `/channels/{channelID}/pins/{messageID}` | `channelH.HandleUnpinMessage` | OK |

### Reactions

| Frontend Method | HTTP | URL | Backend Handler | Status |
|---|---|---|---|---|
| `addReaction` | PUT | `/channels/{channelID}/messages/{messageID}/reactions/{emoji}` | `channelH.HandleAddReaction` | OK |
| `removeReaction` | DELETE | `/channels/{channelID}/messages/{messageID}/reactions/{emoji}` | `channelH.HandleRemoveReaction` | OK |

### Typing

| Frontend Method | HTTP | URL | Backend Handler | Status |
|---|---|---|---|---|
| `sendTyping` | POST | `/channels/{channelID}/typing` | `channelH.HandleTriggerTyping` | OK |

### Threads

| Frontend Method | HTTP | URL | Backend Handler | Status |
|---|---|---|---|---|
| `createThread` | POST | `/channels/{channelID}/messages/{messageID}/threads` | `channelH.HandleCreateThread` | OK |
| `getThreads` | GET | `/channels/{channelID}/threads` | `channelH.HandleGetThreads` | OK |
| `hideThread` | POST | `/channels/{channelID}/threads/{threadID}/hide` | `channelH.HandleHideThread` | OK |
| `unhideThread` | DELETE | `/channels/{channelID}/threads/{threadID}/hide` | `channelH.HandleUnhideThread` | OK |
| `getHiddenThreads` | GET | `/users/@me/hidden-threads` | `channelH.HandleGetHiddenThreads` | OK |

### Members

| Frontend Method | HTTP | URL | Backend Handler | Status |
|---|---|---|---|---|
| `getMembers` | GET | `/guilds/{guildID}/members` | `guildH.HandleGetGuildMembers` | OK |
| `getMember` | GET | `/guilds/{guildID}/members/{memberID}` | `guildH.HandleGetGuildMember` | OK |
| `updateMember` | PATCH | `/guilds/{guildID}/members/{memberID}` | `guildH.HandleUpdateGuildMember` | OK |
| `kickMember` | DELETE | `/guilds/{guildID}/members/{memberID}` | `guildH.HandleRemoveGuildMember` | OK |
| `getMemberRoles` | GET | `/guilds/{guildID}/members/{memberID}/roles` | `guildH.HandleGetMemberRoles` | OK |

### Bans

| Frontend Method | HTTP | URL | Backend Handler | Status |
|---|---|---|---|---|
| `getGuildBans` | GET | `/guilds/{guildID}/bans` | `guildH.HandleGetGuildBans` | OK |
| `banUser` | PUT | `/guilds/{guildID}/bans/{userID}` | `guildH.HandleCreateGuildBan` | OK |
| `unbanUser` | DELETE | `/guilds/{guildID}/bans/{userID}` | `guildH.HandleRemoveGuildBan` | OK |

### Roles

| Frontend Method | HTTP | URL | Backend Handler | Status |
|---|---|---|---|---|
| `getRoles` | GET | `/guilds/{guildID}/roles` | `guildH.HandleGetGuildRoles` | OK |
| `createRole` | POST | `/guilds/{guildID}/roles` | `guildH.HandleCreateGuildRole` | OK |
| `updateRole` | PATCH | `/guilds/{guildID}/roles/{roleID}` | `guildH.HandleUpdateGuildRole` | OK |
| `deleteRole` | DELETE | `/guilds/{guildID}/roles/{roleID}` | `guildH.HandleDeleteGuildRole` | OK |
| `assignRole` | PUT | `/guilds/{guildID}/members/{memberID}/roles/{roleID}` | `guildH.HandleAddMemberRole` | OK |
| `removeRole` | DELETE | `/guilds/{guildID}/members/{memberID}/roles/{roleID}` | `guildH.HandleRemoveMemberRole` | OK |
| `reorderRoles` | PATCH | `/guilds/{guildID}/roles` | `guildH.HandleReorderGuildRoles` | OK |

### Invites

| Frontend Method | HTTP | URL | Backend Handler | Status |
|---|---|---|---|---|
| `getGuildInvites` | GET | `/guilds/{guildID}/invites` | `guildH.HandleGetGuildInvites` | OK |
| `createInvite` | POST | `/guilds/{guildID}/invites` | `guildH.HandleCreateGuildInvite` | OK |
| `getInvite` | GET | `/invites/{code}` | `inviteH.HandleGetInvite` | OK |
| `acceptInvite` | POST | `/invites/{code}` | `inviteH.HandleAcceptInvite` | OK |
| `deleteInvite` | DELETE | `/invites/{code}` | `inviteH.HandleDeleteInvite` | OK |

### Categories

| Frontend Method | HTTP | URL | Backend Handler | Status |
|---|---|---|---|---|
| `getCategories` | GET | `/guilds/{guildID}/categories` | `guildH.HandleGetGuildCategories` | OK |
| `createCategory` | POST | `/guilds/{guildID}/categories` | `guildH.HandleCreateGuildCategory` | OK |
| `updateCategory` | PATCH | `/guilds/{guildID}/categories/{categoryID}` | `guildH.HandleUpdateGuildCategory` | OK |
| `deleteCategory` | DELETE | `/guilds/{guildID}/categories/{categoryID}` | `guildH.HandleDeleteGuildCategory` | OK |

### Audit Log

| Frontend Method | HTTP | URL | Backend Handler | Status |
|---|---|---|---|---|
| `getAuditLog` | GET | `/guilds/{guildID}/audit-log` | `guildH.HandleGetGuildAuditLog` | OK |

### Emoji

| Frontend Method | HTTP | URL | Backend Handler | Status |
|---|---|---|---|---|
| `getGuildEmoji` | GET | `/guilds/{guildID}/emoji` | `guildH.HandleGetGuildEmoji` | OK |
| `uploadEmoji` | POST | `/guilds/{guildID}/emoji` | `guildH.HandleCreateGuildEmoji` | OK |
| `deleteGuildEmoji` | DELETE | `/guilds/{guildID}/emoji/{emojiID}` | `guildH.HandleDeleteGuildEmoji` | OK |

### Webhooks

| Frontend Method | HTTP | URL | Backend Handler | Status |
|---|---|---|---|---|
| `getGuildWebhooks` | GET | `/guilds/{guildID}/webhooks` | `guildH.HandleGetGuildWebhooks` | OK |
| `getChannelWebhooks` | GET | `/channels/{channelID}/webhooks` | `channelH.HandleGetChannelWebhooks` | OK |
| `createWebhook` | POST | `/guilds/{guildID}/webhooks` | `guildH.HandleCreateGuildWebhook` | OK |
| `updateWebhook` | PATCH | `/guilds/{guildID}/webhooks/{webhookID}` | `guildH.HandleUpdateGuildWebhook` | OK |
| `deleteWebhook` | DELETE | `/guilds/{guildID}/webhooks/{webhookID}` | `guildH.HandleDeleteGuildWebhook` | OK |

### Voice

| Frontend Method | HTTP | URL | Backend Handler | Status |
|---|---|---|---|---|
| `joinVoice` | POST | `/voice/{channelID}/join` | `s.handleVoiceJoin` | OK |
| `leaveVoice` | POST | `/voice/{channelID}/leave` | `s.handleVoiceLeave` | OK |
| `getVoicePreferences` | GET | `/voice/preferences` | `s.handleGetVoicePreferences` | OK |
| `updateVoicePreferences` | PATCH | `/voice/preferences` | `s.handleUpdateVoicePreferences` | OK |

### Federation Voice (registered in cmd/amityvox/main.go)

| Frontend Method | HTTP | URL | Backend Handler | Status |
|---|---|---|---|---|
| `joinFederatedVoice` | POST | `/federation/voice/join` | `syncSvc.HandleProxyFederatedVoiceJoin` | OK |
| `joinFederatedVoiceByGuild` | POST | `/federation/voice/guild-join` | `syncSvc.HandleProxyFederatedVoiceJoinByGuild` | OK |

### Federation Guilds (registered in cmd/amityvox/main.go)

| Frontend Method | HTTP | URL | Backend Handler | Status |
|---|---|---|---|---|
| `joinFederatedGuild` | POST | `/federation/guilds/join` | `syncSvc.HandleProxyJoinFederatedGuild` | OK |
| `leaveFederatedGuild` | POST | `/federation/guilds/{guildID}/leave` | `syncSvc.HandleProxyLeaveFederatedGuild` | OK |
| `getFederatedGuildMessages` | GET | `/federation/guilds/{guildID}/channels/{channelID}/messages` | `syncSvc.HandleProxyGetFederatedGuildMessages` | OK |
| `sendFederatedGuildMessage` | POST | `/federation/guilds/{guildID}/channels/{channelID}/messages` | `syncSvc.HandleProxyPostFederatedGuildMessage` | OK |

### File Upload / Media

| Frontend Method | HTTP | URL | Backend Handler | Status |
|---|---|---|---|---|
| `uploadFile` | POST | `/files/upload` | `s.Media.HandleUpload` | OK (conditional on Media != nil) |
| `updateAttachment` | PATCH | `/files/{fileID}` | `s.Media.HandleUpdateAttachment` | OK |
| `deleteAttachment` | DELETE | `/files/{fileID}` | `s.Media.HandleDeleteAttachment` | OK |
| `tagAttachment` | PUT | `/files/{fileID}/tags/{tagID}` | `s.Media.HandleTagAttachment` | OK |
| `untagAttachment` | DELETE | `/files/{fileID}/tags/{tagID}` | `s.Media.HandleUntagAttachment` | OK |

### Gallery

| Frontend Method | HTTP | URL | Backend Handler | Status |
|---|---|---|---|---|
| `getChannelGallery` | GET | `/channels/{channelID}/gallery` | `channelH.HandleGetChannelGallery` | OK |
| `getGuildGallery` | GET | `/guilds/{guildID}/gallery` | `guildH.HandleGetGuildGallery` | OK |
| `getMediaTags` | GET | `/guilds/{guildID}/media-tags` | `guildH.HandleGetMediaTags` | OK |
| `createMediaTag` | POST | `/guilds/{guildID}/media-tags` | `guildH.HandleCreateMediaTag` | OK |
| `deleteMediaTag` | DELETE | `/guilds/{guildID}/media-tags/{tagID}` | `guildH.HandleDeleteMediaTag` | OK |

### Search

| Frontend Method | HTTP | URL | Backend Handler | Status |
|---|---|---|---|---|
| `searchMessages` | GET | `/search/messages?q=...` | `s.handleSearchMessages` | OK |

### Giphy (conditional - only if Giphy enabled)

| Frontend Method | HTTP | URL | Backend Handler | Status |
|---|---|---|---|---|
| `searchGiphy` | GET | `/giphy/search?q=...` | `s.handleGiphySearch` | OK |
| `getTrendingGiphy` | GET | `/giphy/trending` | `s.handleGiphyTrending` | OK |
| `getGiphyCategories` | GET | `/giphy/categories` | `s.handleGiphyCategories` | OK |

### Encryption/MLS (conditional - only if Encryption != nil)

| Frontend Method | HTTP | URL | Backend Handler | Status |
|---|---|---|---|---|
| `uploadKeyPackage` | POST | `/encryption/key-packages` | `s.Encryption.HandleUploadKeyPackage` | OK |
| `getKeyPackages` | GET | `/encryption/key-packages/{userID}` | `s.Encryption.HandleGetKeyPackages` | OK |
| `claimKeyPackage` | POST | `/encryption/key-packages/{userID}/claim` | `s.Encryption.HandleClaimKeyPackage` | OK |
| `deleteKeyPackage` | DELETE | `/encryption/key-packages/{packageID}` | `s.Encryption.HandleDeleteKeyPackage` | OK (naming note) |
| `getWelcomeMessages` | GET | `/encryption/welcome` | `s.Encryption.HandleGetWelcomes` | OK |
| `sendWelcome` | POST | `/encryption/channels/{channelID}/welcome` | `s.Encryption.HandleSendWelcome` | OK |
| `getGroupState` | GET | `/encryption/channels/{channelID}/group-state` | `s.Encryption.HandleGetGroupState` | OK |
| `updateGroupState` | PUT | `/encryption/channels/{channelID}/group-state` | `s.Encryption.HandleUpdateGroupState` | OK |
| `ackWelcome` | POST | `/encryption/welcome/{welcomeId}/ack` | **MISMATCH** - see details above | FAIL |

### Notifications (conditional - only if Notifications != nil)

| Frontend Method | HTTP | URL | Backend Handler | Status |
|---|---|---|---|---|
| `getNotificationPreferences` | GET | `/notifications/preferences` | `s.Notifications.HandleGetPreferences` | OK |
| `updateNotificationPreferences` | PATCH | `/notifications/preferences` | `s.Notifications.HandleUpdatePreferences` | OK |
| `getChannelNotificationPreferences` | GET | `/notifications/preferences/channels` | `s.Notifications.HandleGetChannelPreferences` | OK |
| `updateChannelNotificationPreference` | PATCH | `/notifications/preferences/channels` | `s.Notifications.HandleUpdateChannelPreference` | OK |
| `deleteChannelNotificationPreference` | DELETE | `/notifications/preferences/channels/{channelID}` | `s.Notifications.HandleDeleteChannelPreference` | OK |
| `getVapidKey` | GET | `/notifications/vapid-key` | `s.Notifications.HandleGetVAPIDKey` | OK (conditional on Enabled()) |
| `subscribePush` | POST | `/notifications/subscriptions` | `s.Notifications.HandleSubscribe` | OK (conditional) |
| `getPushSubscriptions` | GET | `/notifications/subscriptions` | `s.Notifications.HandleListSubscriptions` | OK (conditional) |
| `deletePushSubscription` | DELETE | `/notifications/subscriptions/{subscriptionID}` | `s.Notifications.HandleUnsubscribe` | OK (conditional) |

### Polls

| Frontend Method | HTTP | URL | Backend Handler | Status |
|---|---|---|---|---|
| `createPoll` | POST | `/channels/{channelID}/polls` | `pollH.HandleCreatePoll` | OK |
| `getPoll` | GET | `/channels/{channelID}/polls/{pollID}` | `pollH.HandleGetPoll` | OK |
| `votePoll` | POST | `/channels/{channelID}/polls/{pollID}/votes` | `pollH.HandleVotePoll` | OK |
| `closePoll` | POST | `/channels/{channelID}/polls/{pollID}/close` | `pollH.HandleClosePoll` | OK |
| `deletePoll` | DELETE | `/channels/{channelID}/polls/{pollID}` | `pollH.HandleDeletePoll` | OK |

### Bookmarks

| Frontend Method | HTTP | URL | Backend Handler | Status |
|---|---|---|---|---|
| `createBookmark` | PUT | `/messages/{messageID}/bookmark` | `bookmarkH.HandleCreateBookmark` | OK |
| `deleteBookmark` | DELETE | `/messages/{messageID}/bookmark` | `bookmarkH.HandleDeleteBookmark` | OK |
| `getBookmarks` | GET | `/users/@me/bookmarks` | `bookmarkH.HandleListBookmarks` | OK |

### Guild Events

| Frontend Method | HTTP | URL | Backend Handler | Status |
|---|---|---|---|---|
| `createGuildEvent` | POST | `/guilds/{guildID}/events` | `guildEventH.HandleCreateEvent` | OK |
| `getGuildEvents` | GET | `/guilds/{guildID}/events` | `guildEventH.HandleListEvents` | OK |
| `getGuildEvent` | GET | `/guilds/{guildID}/events/{eventID}` | `guildEventH.HandleGetEvent` | OK |
| `updateGuildEvent` | PATCH | `/guilds/{guildID}/events/{eventID}` | `guildEventH.HandleUpdateEvent` | OK |
| `deleteGuildEvent` | DELETE | `/guilds/{guildID}/events/{eventID}` | `guildEventH.HandleDeleteEvent` | OK |
| `rsvpEvent` | POST | `/guilds/{guildID}/events/{eventID}/rsvp` | `guildEventH.HandleRSVP` | OK |
| `deleteRsvp` | DELETE | `/guilds/{guildID}/events/{eventID}/rsvp` | `guildEventH.HandleDeleteRSVP` | OK |
| `getEventRsvps` | GET | `/guilds/{guildID}/events/{eventID}/rsvps` | `guildEventH.HandleListRSVPs` | OK |

### Moderation: Warnings

| Frontend Method | HTTP | URL | Backend Handler | Status |
|---|---|---|---|---|
| `warnMember` | POST | `/guilds/{guildID}/members/{memberID}/warn` | `modH.HandleWarnMember` | OK |
| `getMemberWarnings` | GET | `/guilds/{guildID}/members/{memberID}/warnings` | `modH.HandleGetWarnings` | OK |
| `deleteWarning` | DELETE | `/guilds/{guildID}/warnings/{warningID}` | `modH.HandleDeleteWarning` | OK |

### Moderation: Reports

| Frontend Method | HTTP | URL | Backend Handler | Status |
|---|---|---|---|---|
| `reportMessage` | POST | `/channels/{channelID}/messages/{messageID}/report` | `modH.HandleReportMessage` | OK |
| `getReports` | GET | `/guilds/{guildID}/reports` | `modH.HandleGetReports` | OK |
| `resolveReport` | PATCH | `/guilds/{guildID}/reports/{reportID}` | `modH.HandleResolveReport` | OK |

### Moderation: Channel Lock

| Frontend Method | HTTP | URL | Backend Handler | Status |
|---|---|---|---|---|
| `lockChannel` | POST | `/channels/{channelID}/lock` | `modH.HandleLockChannel` | OK |
| `unlockChannel` | POST | `/channels/{channelID}/unlock` | `modH.HandleUnlockChannel` | OK |

### Moderation: Raid Config

| Frontend Method | HTTP | URL | Backend Handler | Status |
|---|---|---|---|---|
| `getRaidConfig` | GET | `/guilds/{guildID}/raid-config` | `modH.HandleGetRaidConfig` | OK |
| `updateRaidConfig` | PATCH | `/guilds/{guildID}/raid-config` | `modH.HandleUpdateRaidConfig` | OK |

### AutoMod (conditional - only if AutoMod != nil)

| Frontend Method | HTTP | URL | Backend Handler | Status |
|---|---|---|---|---|
| `getAutoModRules` | GET | `/guilds/{guildID}/automod/rules` | `s.AutoMod.HandleListRules` | OK |
| `createAutoModRule` | POST | `/guilds/{guildID}/automod/rules` | `s.AutoMod.HandleCreateRule` | OK |
| `getAutoModRule` | GET | `/guilds/{guildID}/automod/rules/{ruleID}` | `s.AutoMod.HandleGetRule` | OK |
| `updateAutoModRule` | PATCH | `/guilds/{guildID}/automod/rules/{ruleID}` | `s.AutoMod.HandleUpdateRule` | OK |
| `deleteAutoModRule` | DELETE | `/guilds/{guildID}/automod/rules/{ruleID}` | `s.AutoMod.HandleDeleteRule` | OK |
| `getAutoModActions` | GET | `/guilds/{guildID}/automod/actions` | `s.AutoMod.HandleGetActions` | OK |
| `testAutoModRule` | POST | `/guilds/{guildID}/automod/rules/test` | `s.AutoMod.HandleTestRule` | OK |

### Onboarding

| Frontend Method | HTTP | URL | Backend Handler | Status |
|---|---|---|---|---|
| `getOnboarding` | GET | `/guilds/{guildID}/onboarding` | `onboardH.HandleGetOnboarding` | OK |
| `updateOnboarding` | PUT | `/guilds/{guildID}/onboarding` | `onboardH.HandleUpdateOnboarding` | OK |
| `createOnboardingPrompt` | POST | `/guilds/{guildID}/onboarding/prompts` | `onboardH.HandleCreatePrompt` | OK |
| `updateOnboardingPrompt` | PUT | `/guilds/{guildID}/onboarding/prompts/{promptID}` | `onboardH.HandleUpdatePrompt` | OK |
| `deleteOnboardingPrompt` | DELETE | `/guilds/{guildID}/onboarding/prompts/{promptID}` | `onboardH.HandleDeletePrompt` | OK |
| `completeOnboarding` | POST | `/guilds/{guildID}/onboarding/complete` | `onboardH.HandleCompleteOnboarding` | OK |
| `getOnboardingStatus` | GET | `/guilds/{guildID}/onboarding/status` | `onboardH.HandleGetOnboardingStatus` | OK |

### Ban Lists

| Frontend Method | HTTP | URL | Backend Handler | Status |
|---|---|---|---|---|
| `getBanLists` | GET | `/guilds/{guildID}/ban-lists` | `modH.HandleGetBanLists` | OK |
| `createBanList` | POST | `/guilds/{guildID}/ban-lists` | `modH.HandleCreateBanList` | OK |
| `deleteBanList` | DELETE | `/guilds/{guildID}/ban-lists/{listID}` | `modH.HandleDeleteBanList` | OK |
| `getBanListEntries` | GET | `/guilds/{guildID}/ban-lists/{listID}/entries` | `modH.HandleGetBanListEntries` | OK |
| `addBanListEntry` | POST | `/guilds/{guildID}/ban-lists/{listID}/entries` | `modH.HandleAddBanListEntry` | OK |
| `removeBanListEntry` | DELETE | `/guilds/{guildID}/ban-lists/{listID}/entries/{entryID}` | `modH.HandleRemoveBanListEntry` | OK |
| `exportBanList` | GET | `/guilds/{guildID}/ban-lists/{listID}/export` | `modH.HandleExportBanList` | OK |
| `importBanList` | POST | `/guilds/{guildID}/ban-lists/{listID}/import` | `modH.HandleImportBanList` | OK |
| `getBanListSubscriptions` | GET | `/guilds/{guildID}/ban-list-subscriptions` | `modH.HandleGetBanListSubscriptions` | OK |
| `subscribeBanList` | POST | `/guilds/{guildID}/ban-list-subscriptions` | `modH.HandleSubscribeBanList` | OK |
| `unsubscribeBanList` | DELETE | `/guilds/{guildID}/ban-list-subscriptions/{subID}` | `modH.HandleUnsubscribeBanList` | OK |
| `getPublicBanLists` | GET | `/ban-lists/public` | `modH.HandleGetPublicBanLists` | OK |

### Channel Followers

| Frontend Method | HTTP | URL | Backend Handler | Status |
|---|---|---|---|---|
| `followChannel` | POST | `/channels/{channelID}/followers` | `channelH.HandleFollowChannel` | OK |
| `getChannelFollowers` | GET | `/channels/{channelID}/followers` | `channelH.HandleGetChannelFollowers` | OK |
| `unfollowChannel` | DELETE | `/channels/{channelID}/followers/{followerID}` | `channelH.HandleUnfollowChannel` | OK |

### Group DM Recipients

| Frontend Method | HTTP | URL | Backend Handler | Status |
|---|---|---|---|---|
| `addGroupDMRecipient` | PUT | `/channels/{channelID}/recipients/{userID}` | `channelH.HandleAddGroupDMRecipient` | OK |
| `removeGroupDMRecipient` | DELETE | `/channels/{channelID}/recipients/{userID}` | `channelH.HandleRemoveGroupDMRecipient` | OK |

### Bots

| Frontend Method | HTTP | URL | Backend Handler | Status |
|---|---|---|---|---|
| `getMyBots` | GET | `/users/@me/bots` | `botH.HandleListMyBots` | OK |
| `createBot` | POST | `/users/@me/bots` | `botH.HandleCreateBot` | OK |
| `getBot` | GET | `/bots/{botID}` | `botH.HandleGetBot` | OK |
| `updateBot` | PATCH | `/bots/{botID}` | `botH.HandleUpdateBot` | OK |
| `deleteBot` | DELETE | `/bots/{botID}` | `botH.HandleDeleteBot` | OK |
| `getBotTokens` | GET | `/bots/{botID}/tokens` | `botH.HandleListTokens` | OK |
| `createBotToken` | POST | `/bots/{botID}/tokens` | `botH.HandleCreateToken` | OK |
| `deleteBotToken` | DELETE | `/bots/{botID}/tokens/{tokenID}` | `botH.HandleDeleteToken` | OK |
| `getBotCommands` | GET | `/bots/{botID}/commands` | `botH.HandleListCommands` | OK |
| `registerBotCommand` | POST | `/bots/{botID}/commands` | `botH.HandleRegisterCommand` | OK |
| `updateBotCommand` | PATCH | `/bots/{botID}/commands/{commandID}` | `botH.HandleUpdateCommand` | OK |
| `deleteBotCommand` | DELETE | `/bots/{botID}/commands/{commandID}` | `botH.HandleDeleteCommand` | OK |

### Sticker Packs

| Frontend Method | HTTP | URL | Backend Handler | Status |
|---|---|---|---|---|
| `getGuildStickerPacks` | GET | `/guilds/{guildID}/sticker-packs` | `stickerH.HandleGetGuildPacks` | OK |
| `createGuildStickerPack` | POST | `/guilds/{guildID}/sticker-packs` | `stickerH.HandleCreateGuildPack` | OK |
| `deleteGuildStickerPack` | DELETE | `/guilds/{guildID}/sticker-packs/{packID}` | `stickerH.HandleDeletePack` | OK |
| `getPackStickers` | GET | `/guilds/{guildID}/sticker-packs/{packID}/stickers` | `stickerH.HandleGetPackStickers` | OK |
| `addStickerToGuildPack` | POST | `/guilds/{guildID}/sticker-packs/{packID}/stickers` | `stickerH.HandleAddSticker` | OK |
| `deleteStickerFromGuildPack` | DELETE | `/guilds/{guildID}/sticker-packs/{packID}/stickers/{stickerID}` | `stickerH.HandleDeleteSticker` | OK |
| `getUserStickerPacks` | GET | `/stickers/my-packs` | `stickerH.HandleGetUserPacks` | OK |
| `createUserStickerPack` | POST | `/stickers/my-packs` | `stickerH.HandleCreateUserPack` | OK |

### Announcements

| Frontend Method | HTTP | URL | Backend Handler | Status |
|---|---|---|---|---|
| `getActiveAnnouncements` | GET | `/announcements` | `adminH.HandleGetAnnouncements` | OK |

### Admin

| Frontend Method | HTTP | URL | Backend Handler | Status |
|---|---|---|---|---|
| `getAdminStats` | GET | `/admin/stats` | `adminH.HandleGetStats` | OK |
| `getAdminInstance` | GET | `/admin/instance` | `adminH.HandleGetInstance` | OK |
| `updateAdminInstance` | PATCH | `/admin/instance` | `adminH.HandleUpdateInstance` | OK |
| `getAdminUsers` | GET | `/admin/users` | `adminH.HandleListUsers` | OK |
| `suspendUser` | POST | `/admin/users/{userID}/suspend` | `adminH.HandleSuspendUser` | OK |
| `unsuspendUser` | POST | `/admin/users/{userID}/unsuspend` | `adminH.HandleUnsuspendUser` | OK |
| `setAdmin` | POST | `/admin/users/{userID}/set-admin` | `adminH.HandleSetAdmin` | OK |
| `setGlobalMod` | POST | `/admin/users/{userID}/set-globalmod` | `adminH.HandleSetGlobalMod` | OK |
| `instanceBanUser` | POST | `/admin/users/{userID}/instance-ban` | `adminH.HandleInstanceBanUser` | OK |
| `instanceUnbanUser` | POST | `/admin/users/{userID}/instance-unban` | `adminH.HandleInstanceUnbanUser` | OK |
| `getInstanceBans` | GET | `/admin/instance-bans` | `adminH.HandleGetInstanceBans` | OK |
| `getAdminGuilds` | GET | `/admin/guilds` | `adminH.HandleListGuilds` | OK |
| `getAdminGuildDetails` | GET | `/admin/guilds/{guildID}` | `adminH.HandleGetGuildDetails` | OK |
| `adminDeleteGuild` | DELETE | `/admin/guilds/{guildID}` | `adminH.HandleAdminDeleteGuild` | OK |
| `getAdminUserGuilds` | GET | `/admin/users/{userID}/guilds` | `adminH.HandleGetUserGuilds` | OK |
| `getAdminMedia` | GET | `/admin/media` | `adminH.HandleAdminGetMedia` | OK |
| `deleteAdminMedia` | DELETE | `/admin/media/{fileID}` | `adminH.HandleAdminDeleteMedia` | OK |

### Admin Federation

| Frontend Method | HTTP | URL | Backend Handler | Status |
|---|---|---|---|---|
| `getFederationPeers` | GET | `/admin/federation/peers` | `adminH.HandleGetFederationPeers` | OK |
| `addFederationPeer` | POST | `/admin/federation/peers` | `adminH.HandleAddFederationPeer` | OK |
| `removeFederationPeer` | DELETE | `/admin/federation/peers/{peerID}` | `adminH.HandleRemoveFederationPeer` | OK |

### Admin Registration

| Frontend Method | HTTP | URL | Backend Handler | Status |
|---|---|---|---|---|
| `getRegistrationSettings` | GET | `/admin/registration` | `adminH.HandleGetRegistrationConfig` | OK |
| `updateRegistrationSettings` | PATCH | `/admin/registration` | `adminH.HandleUpdateRegistrationConfig` | OK |
| `createRegistrationToken` | POST | `/admin/registration/tokens` | `adminH.HandleCreateRegistrationToken` | OK |
| `getRegistrationTokens` | GET | `/admin/registration/tokens` | `adminH.HandleListRegistrationTokens` | OK |
| `deleteRegistrationToken` | DELETE | `/admin/registration/tokens/{tokenID}` | `adminH.HandleDeleteRegistrationToken` | OK |

### Admin Announcements

| Frontend Method | HTTP | URL | Backend Handler | Status |
|---|---|---|---|---|
| `createAnnouncement` | POST | `/admin/announcements` | `adminH.HandleCreateAnnouncement` | OK |
| `getAdminAnnouncements` | GET | `/admin/announcements` | `adminH.HandleListAllAnnouncements` | OK |
| `updateAnnouncement` | PATCH | `/admin/announcements/{announcementID}` | `adminH.HandleUpdateAnnouncement` | OK |
| `deleteAnnouncement` | DELETE | `/admin/announcements/{announcementID}` | `adminH.HandleDeleteAnnouncement` | OK |

### Global Moderation

| Frontend Method | HTTP | URL | Backend Handler | Status |
|---|---|---|---|---|
| `reportUser` | POST | `/users/{userID}/report` | `modH.HandleReportUser` | OK |
| `reportMessageToAdmins` | POST | `/channels/{channelID}/messages/{messageID}/report-admin` | `modH.HandleReportToAdmin` | OK |
| `createIssue` | POST | `/issues` | `modH.HandleCreateIssue` | OK |
| `getMyIssues` | GET | `/users/@me/issues` | `modH.HandleGetMyIssues` | OK |
| `getModerationStats` | GET | `/moderation/stats` | `modH.HandleGetModerationStats` | OK |
| `getModerationUserReports` | GET | `/moderation/user-reports` | `modH.HandleGetUserReports` | OK |
| `resolveModerationUserReport` | PATCH | `/moderation/user-reports/{reportID}` | `modH.HandleResolveUserReport` | OK |
| `getModerationMessageReports` | GET | `/moderation/message-reports` | `modH.HandleGetAllMessageReports` | OK |
| `resolveModerationMessageReport` | PATCH | `/moderation/message-reports/{reportID}` | `modH.HandleResolveMessageReport` | OK |
| `getModerationIssues` | GET | `/moderation/issues` | `modH.HandleGetIssues` | OK |
| `resolveModerationIssue` | PATCH | `/moderation/issues/{issueID}` | `modH.HandleResolveIssue` | OK |

### Retention Policies

| Frontend Method | HTTP | URL | Backend Handler | Status |
|---|---|---|---|---|
| `getGuildRetentionPolicies` | GET | `/guilds/{guildID}/retention` | `guildH.HandleGetGuildRetentionPolicies` | OK |
| `createGuildRetentionPolicy` | POST | `/guilds/{guildID}/retention` | `guildH.HandleCreateGuildRetentionPolicy` | OK |
| `updateGuildRetentionPolicy` | PATCH | `/guilds/{guildID}/retention/{policyID}` | `guildH.HandleUpdateGuildRetentionPolicy` | OK |
| `deleteGuildRetentionPolicy` | DELETE | `/guilds/{guildID}/retention/{policyID}` | `guildH.HandleDeleteGuildRetentionPolicy` | OK |

### Forum Tags and Posts

| Frontend Method | HTTP | URL | Backend Handler | Status |
|---|---|---|---|---|
| `getForumTags` | GET | `/channels/{channelID}/tags` | `channelH.HandleGetForumTags` | OK |
| `createForumTag` | POST | `/channels/{channelID}/tags` | `channelH.HandleCreateForumTag` | OK |
| `updateForumTag` | PATCH | `/channels/{channelID}/tags/{tagID}` | `channelH.HandleUpdateForumTag` | OK |
| `deleteForumTag` | DELETE | `/channels/{channelID}/tags/{tagID}` | `channelH.HandleDeleteForumTag` | OK |
| `getForumPosts` | GET | `/channels/{channelID}/posts` | `channelH.HandleGetForumPosts` | OK |
| `createForumPost` | POST | `/channels/{channelID}/posts` | `channelH.HandleCreateForumPost` | OK |
| `pinForumPost` | POST | `/channels/{channelID}/posts/{postID}/pin` | `channelH.HandlePinForumPost` | OK |
| `closeForumPost` | POST | `/channels/{channelID}/posts/{postID}/close` | `channelH.HandleCloseForumPost` | OK |

### Gallery Tags and Posts

| Frontend Method | HTTP | URL | Backend Handler | Status |
|---|---|---|---|---|
| `getGalleryTags` | GET | `/channels/{channelID}/gallery-tags` | `channelH.HandleGetGalleryTags` | OK |
| `createGalleryTag` | POST | `/channels/{channelID}/gallery-tags` | `channelH.HandleCreateGalleryTag` | OK |
| `updateGalleryTag` | PATCH | `/channels/{channelID}/gallery-tags/{tagID}` | `channelH.HandleUpdateGalleryTag` | OK |
| `deleteGalleryTag` | DELETE | `/channels/{channelID}/gallery-tags/{tagID}` | `channelH.HandleDeleteGalleryTag` | OK |
| `getGalleryPosts` | GET | `/channels/{channelID}/gallery-posts` | `channelH.HandleGetGalleryPosts` | OK |
| `createGalleryPost` | POST | `/channels/{channelID}/gallery-posts` | `channelH.HandleCreateGalleryPost` | OK |
| `pinGalleryPost` | POST | `/channels/{channelID}/gallery-posts/{postID}/pin` | `channelH.HandlePinGalleryPost` | OK |
| `closeGalleryPost` | POST | `/channels/{channelID}/gallery-posts/{postID}/close` | `channelH.HandleCloseGalleryPost` | OK |

### Activities (MISMATCHES - see details above)

| Frontend Method | HTTP | URL | Backend Route | Status |
|---|---|---|---|---|
| `listActivities` | GET | `/activities` | `GET /activities/` | OK |
| `getActiveSession` | GET | `/channels/{channelId}/activities/sessions/active` | `GET /activities/{activityID}/sessions/active` | **FAIL** |
| `createActivitySession` | POST | `/channels/{channelId}/activities/sessions` | `POST /activities/{activityID}/sessions` | **FAIL** |
| `joinActivitySession` | POST | `/channels/{channelId}/activities/sessions/{sessionId}/join` | `POST /activities/sessions/{sessionID}/join` | **FAIL** |
| `leaveActivitySession` | POST | `/channels/{channelId}/activities/sessions/{sessionId}/leave` | `POST /activities/sessions/{sessionID}/leave` | **FAIL** |
| `endActivitySession` | POST | `/channels/{channelId}/activities/sessions/{sessionId}/end` | `POST /activities/sessions/{sessionID}/end` | **FAIL** |

### Kanban (under experimental)

| Frontend Method | HTTP | URL | Backend Handler | Status |
|---|---|---|---|---|
| `createKanbanBoard` | POST | `/channels/{channelID}/experimental/kanban` | `experimentalH.HandleCreateKanbanBoard` | OK |
| `getKanbanBoard` | GET | `/channels/{channelID}/experimental/kanban/{boardID}` | `experimentalH.HandleGetKanbanBoard` | OK |
| `createKanbanColumn` | POST | `/channels/{channelID}/experimental/kanban/{boardID}/columns` | `experimentalH.HandleCreateKanbanColumn` | OK |
| `createKanbanCard` | POST | `/channels/{channelID}/experimental/kanban/{boardID}/columns/{columnID}/cards` | `experimentalH.HandleCreateKanbanCard` | OK |
| `moveKanbanCard` | PATCH | `/channels/{channelID}/experimental/kanban/{boardID}/cards/{cardID}/move` | `experimentalH.HandleMoveKanbanCard` | OK |
| `deleteKanbanCard` | DELETE | `/channels/{channelID}/experimental/kanban/{boardID}/cards/{cardID}` | `experimentalH.HandleDeleteKanbanCard` | OK |

---

## Recommended Fixes (Priority Order)

### P0 - Broken at Runtime

1. **Activities URL mismatch (5 methods):** The frontend routes activity session calls through `/channels/{channelId}/activities/...` but the backend registers them under `/activities/{activityID}/...`. Either:
   - (a) Add channel-scoped proxy routes on the backend under `/channels/{channelID}/activities/...`, or
   - (b) Change the frontend `getActiveSession`, `createActivitySession`, `joinActivitySession`, `leaveActivitySession`, and `endActivitySession` to use the `/activities/...` URL structure.

2. **`ackWelcome` (1 method):** Frontend sends `POST /encryption/welcome/{id}/ack` but backend expects `DELETE /encryption/welcome/{id}`. Change the frontend to:
   ```typescript
   ackWelcome(welcomeId: string): Promise<void> {
       return this.del(`/encryption/welcome/${welcomeId}`);
   }
   ```

### P1 - Cosmetic / Documentation

3. **`deleteKeyPackage` parameter naming:** Frontend variable is `keyPackageId` but backend chi parameter is `packageID`. No runtime impact but inconsistent naming. Consider renaming the backend parameter to `keyPackageID` for clarity.

---

## Backend Routes With No Frontend Caller

The following backend routes exist but have no corresponding frontend API method in `client.ts`. These are intentional (used by other clients, admin CLI, federation peers, bots, etc.) and are listed for completeness:

- `POST /auth/email` - Change email
- `POST /auth/backup-codes/verify` - Consume backup code
- `POST /auth/webauthn/*` - WebAuthn registration/login (4 routes)
- `DELETE /users/@me` - Delete own account
- `GET /users/@me/export` - Export user data
- `GET /users/@me/export-account` - Export account
- `POST /users/@me/import-account` - Import account
- `PUT /users/@me/activity` - Update activity
- `GET /users/@me/activity` - Get activity
- `GET /users/@me/emoji` - Get user emoji
- `POST /users/@me/emoji` - Create user emoji
- `DELETE /users/@me/emoji/{emojiID}` - Delete user emoji
- `GET /users/@me/channel-groups/*` - Channel group management (6 routes)
- `GET /channels/{channelID}/messages/{messageID}` - Get single message
- `GET /channels/{channelID}/messages/{messageID}/reactions` - Get reactions
- `DELETE /channels/{channelID}/messages/{messageID}/reactions/{emoji}/{targetUserID}` - Remove user reaction
- `PUT /channels/{channelID}/permissions/{overrideID}` - Set channel permission
- `DELETE /channels/{channelID}/permissions/{overrideID}` - Delete channel permission
- `POST /channels/{channelID}/messages/{messageID}/publish` - Publish message
- `GET /channels/{channelID}/export` - Export channel messages
- `GET /channels/{channelID}/templates/*` - Channel template routes (4 routes)
- `GET /channels/{channelID}/emoji` - Channel emoji routes (3 routes)
- `PATCH /guilds/{guildID}/channels` - Reorder channels
- `POST /guilds/{guildID}/transfer` - Transfer guild ownership
- `GET /guilds/{guildID}/guide` - Get server guide
- `PUT /guilds/{guildID}/guide` - Update server guide
- `GET /guilds/{guildID}/bump` - Get bump status
- `POST /guilds/{guildID}/bump` - Bump guild
- `GET /guilds/{guildID}/templates/*` - Guild template routes (5 routes)
- `GET /guilds/{guildID}/members/search` - Search members
- `GET /guilds/{guildID}/prune` - Get prune count
- `POST /guilds/{guildID}/prune` - Execute prune
- `PATCH /guilds/{guildID}/emoji/{emojiID}` - Update emoji
- `GET /guilds/{guildID}/webhooks/{webhookID}/logs` - Webhook logs
- `GET /guilds/{guildID}/vanity-url` - Get vanity URL
- `PATCH /guilds/{guildID}/vanity-url` - Set vanity URL
- `GET /guilds/vanity/{code}` - Resolve vanity URL
- `GET /search/users` - Search users
- `GET /search/guilds` - Search guilds
- Voice: soundboard, broadcast, screen-share, server mute/deafen/move, input-mode, priority-speaker routes
- Experimental: location sharing, message effects, super reactions, summaries, transcription, whiteboards, code snippets, video recordings
- Activities: `POST /activities` (create), rate, game/watch-together/music-party routes
- Social: insights, boosts, vanity-claim, achievements, leveling, leaderboard, starboard, welcome config, auto-roles
- Integrations: all guild integration and bridge connection routes
- Widgets/plugins: all widget and plugin routes
- Encryption: key-backup, commits
- Themes: all theme gallery routes
- Webhooks: templates, preview, outgoing-events
- Stickers: sharing, clone
- Admin: reports, bots, rate-limits, content-scan, captcha, federation dashboard, blocklist/allowlist, profiles, setup, updates, health, storage, retention, domains, backups, bridges
