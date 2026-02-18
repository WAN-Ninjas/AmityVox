# Feature Completeness Matrix

**Audit Date:** 2026-02-18
**Auditor:** Phase 1C Pre-Release Audit (Claude Opus 4.6)
**Codebase Ref:** commit 804f1d3 (main)

## Summary

- Total features audited: 136
- IMPLEMENTED: 121
- PARTIAL: 12
- STUB: 3
- MISSING: 0

## Detailed Matrix

### Core Messaging

| Category | Feature | Status | Evidence (files) | Notes |
|----------|---------|--------|-------------------|-------|
| Core Messaging | Text messages (send) | IMPLEMENTED | `internal/api/channels/channels.go` (HandleCreateMessage L462), `web/src/lib/components/chat/MessageInput.svelte`, `web/src/lib/stores/messages.ts` | Full end-to-end with nonce dedup, slowmode, permission checks, DM spam detection |
| Core Messaging | Text messages (edit) | IMPLEMENTED | `internal/api/channels/channels.go` (HandleUpdateMessage L726), edit history stored in DB | Edit history viewable via HandleGetMessageEdits (L799), migration 003_message_edit_history |
| Core Messaging | Text messages (delete) | IMPLEMENTED | `internal/api/channels/channels.go` (HandleDeleteMessage L855, HandleBulkDeleteMessages L903) | Single and bulk delete with permission checks |
| Core Messaging | Text messages (reply) | IMPLEMENTED | `internal/api/channels/channels.go` (reply_to_ids field in HandleCreateMessage) | reply_to_ids array in messages table, enriched on fetch |
| Core Messaging | Message attachments | IMPLEMENTED | `internal/media/media.go`, `internal/api/server.go` (HandleUpload, HandleGetFile, HandleUpdateAttachment, HandleDeleteAttachment) | S3 upload/download, alt text (migration 025), media tags, batch loading on messages |
| Core Messaging | Message embeds (link previews) | IMPLEMENTED | `internal/api/channels/channels.go` (enrichMessagesWithEmbeds L1994), embeds table in schema | Embed types: website, image, video, rich, special. Async unfurl via workers |
| Core Messaging | Message reactions (add/remove) | IMPLEMENTED | `internal/api/channels/channels.go` (HandleAddReaction L999, HandleRemoveReaction L1042, HandleRemoveUserReaction L1065, HandleGetReactions L954) | Unicode and custom emoji support, per-user removal |
| Core Messaging | Message pinning | IMPLEMENTED | `internal/api/channels/channels.go` (HandleGetPins L1100, HandlePinMessage L1147, HandleUnpinMessage L1197), `web/src/lib/components/chat/PinnedMessages.svelte` | Full pin/unpin with permission checks |
| Core Messaging | Message search | IMPLEMENTED | `internal/api/search_handlers.go` (handleSearchMessages), `internal/search/search.go`, `web/src/lib/components/chat/SearchModal.svelte` | Meilisearch integration with channel/guild/author filters, pagination |
| Core Messaging | Threads | IMPLEMENTED | `internal/api/channels/channels.go` (HandleCreateThread L1356, HandleGetThreads L1467, HandleHideThread L1538, HandleUnhideThread L1573, HandleGetHiddenThreads L1606), `web/src/lib/components/chat/ThreadPanel.svelte` | Thread create from message, list, hide/unhide. Migration 042_thread_redesign |
| Core Messaging | Polls | IMPLEMENTED | `internal/api/polls/polls.go` (522 lines: HandleCreatePoll, HandleGetPoll, HandleVotePoll, HandleClosePoll, HandleDeletePoll), `web/src/lib/components/PollDisplay.svelte` | Full CRUD with voting, closing, deletion |
| Core Messaging | Message formatting (markdown, code blocks) | IMPLEMENTED | `web/src/lib/components/chat/MarkdownRenderer.svelte`, `web/src/lib/components/chat/CodeSnippet.svelte` | Frontend markdown renderer with tests |
| Core Messaging | Scheduled messages | IMPLEMENTED | `internal/api/channels/channels.go` (HandleScheduleMessage L1636, HandleGetScheduledMessages L1697, HandleDeleteScheduledMessage L1740) | Migration 013_scheduled_messages |
| Core Messaging | Message bookmarks | IMPLEMENTED | `internal/api/bookmarks/` (HandleCreateBookmark, HandleDeleteBookmark, HandleListBookmarks), `web/src/routes/app/bookmarks/` | User-scoped bookmarks with dedicated route |
| Core Messaging | Message edit history | IMPLEMENTED | `internal/api/channels/channels.go` (HandleGetMessageEdits L799), `web/src/lib/components/chat/EditHistoryModal.svelte` | Migration 003, view previous versions |
| Core Messaging | Message translation | IMPLEMENTED | `internal/api/channels/translation.go` (HandleTranslateMessage), `web/src/lib/components/chat/TranslateButton.svelte` | Per-message translation |
| Core Messaging | Crosspost/publish | IMPLEMENTED | `internal/api/channels/channels.go` (HandleCrosspostMessage L2054, HandlePublishMessage L2277) | Announcement channel crossposting with follower notification |

### Channels

| Category | Feature | Status | Evidence (files) | Notes |
|----------|---------|--------|-------------------|-------|
| Channels | Text channels (CRUD) | IMPLEMENTED | `internal/api/channels/channels.go` (HandleGetChannel, HandleUpdateChannel, HandleDeleteChannel), `internal/api/guilds/guilds.go` (HandleCreateGuildChannel) | Full CRUD with position ordering |
| Channels | Voice channels | IMPLEMENTED | `internal/api/voice_handlers.go` (handleVoiceJoin, handleVoiceLeave, handleGetVoiceStates), `internal/voice/voice.go`, `web/src/lib/components/voice/VoiceChannelView.svelte` | LiveKit integration, federation-aware |
| Channels | Forum channels | IMPLEMENTED | `internal/api/channels/forum.go` (HandleGetForumTags, HandleCreateForumTag, HandleUpdateForumTag, HandleDeleteForumTag, HandleGetForumPosts, HandleCreateForumPost, HandlePinForumPost, HandleCloseForumPost), `web/src/lib/components/channels/ForumChannelView.svelte`, `web/src/lib/components/channels/ForumPostCard.svelte`, `web/src/lib/components/channels/ForumPostCreate.svelte` | Migration 051_forum_channels. Tags + posts with pin/close |
| Channels | Gallery channels | IMPLEMENTED | `internal/api/channels/gallery.go` (HandleGetGalleryTags, HandleCreateGalleryTag, HandleUpdateGalleryTag, HandleDeleteGalleryTag, HandleGetGalleryPosts, HandleCreateGalleryPost, HandlePinGalleryPost, HandleCloseGalleryPost), `web/src/lib/components/channels/GalleryChannelView.svelte`, `web/src/lib/components/channels/GalleryPostCard.svelte` | Migration 052_gallery_channels. Tags + posts |
| Channels | Categories | IMPLEMENTED | `internal/api/guilds/guilds.go` (HandleGetGuildCategories, HandleCreateGuildCategory, HandleUpdateGuildCategory, HandleDeleteGuildCategory) | guild_categories table in schema |
| Channels | Channel permissions (overrides) | IMPLEMENTED | `internal/api/channels/channels.go` (HandleSetChannelPermission L1282, HandleDeleteChannelPermission L1331) | channel_permission_overrides table, role and user targets |
| Channels | Channel locking | IMPLEMENTED | `internal/api/moderation/moderation.go` (HandleLockChannel L422, HandleUnlockChannel L467) | locked, locked_by, locked_at fields (migration 010) |
| Channels | Slow mode | IMPLEMENTED | `internal/api/channels/channels.go` (slowmode_seconds enforced in HandleCreateMessage) | slowmode_seconds column in channels table |
| Channels | NSFW flag | IMPLEMENTED | Schema: channels.nsfw column, HandleUpdateChannel supports nsfw field | Boolean flag on channel |
| Channels | Channel topics/descriptions | IMPLEMENTED | Schema: channels.topic column, HandleUpdateChannel supports topic | Displayed in TopBar |
| Channels | Announcement channels | IMPLEMENTED | `internal/api/channels/channels.go` (HandleFollowChannel L2126, HandleGetChannelFollowers L2193, HandleUnfollowChannel L2246, HandlePublishMessage L2277) | Migration 011_announcement_channels. Follow/publish pattern |
| Channels | Channel templates | IMPLEMENTED | `internal/api/channels/channels.go` (HandleCreateChannelTemplate L2404, HandleGetChannelTemplates L2473, HandleDeleteChannelTemplate L2520, HandleApplyChannelTemplate L2548) | Migration 027_channel_templates |
| Channels | Channel archiving | IMPLEMENTED | Migration 027_channel_templates_readonly_autoarchive, 015_channel_archive | Read-only and auto-archive support |
| Channels | Stage channels | PARTIAL | `web/src/lib/components/channels/StageChannelView.svelte`, voice_handlers.go supports 'stage' type | Frontend component exists, voice join supports stage type, but stage-specific features (speaker management, hand raising) appear limited |
| Channels | Channel cloning | IMPLEMENTED | `internal/api/guilds/guilds.go` (HandleCloneChannel), route at `/{guildID}/channels/{channelID}/clone` | Deep clone of channel settings |
| Channels | Channel reordering | IMPLEMENTED | `internal/api/guilds/guilds.go` (HandleReorderGuildChannels) | Batch position update |
| Channels | Channel emoji | IMPLEMENTED | `internal/api/channels/emoji.go` (HandleGetChannelEmoji, HandleCreateChannelEmoji, HandleDeleteChannelEmoji) | Per-channel custom emoji |
| Channels | Group DMs | IMPLEMENTED | `internal/api/users/users.go` (HandleCreateGroupDM), `internal/api/channels/channels.go` (HandleAddGroupDMRecipient L2843, HandleRemoveGroupDMRecipient L2935), `web/src/lib/components/common/GroupDMCreateModal.svelte`, `web/src/lib/components/common/GroupDMSettingsPanel.svelte` | Full group DM with recipient management |
| Channels | Channel gallery (media view) | IMPLEMENTED | `internal/api/channels/channels.go` (HandleGetChannelGallery L3015), `web/src/lib/components/gallery/` | Browse media attachments per channel |

### Guilds (Servers)

| Category | Feature | Status | Evidence (files) | Notes |
|----------|---------|--------|-------------------|-------|
| Guilds | Guild CRUD | IMPLEMENTED | `internal/api/guilds/guilds.go` (HandleCreateGuild, HandleGetGuild, HandleUpdateGuild, HandleDeleteGuild, HandleLeaveGuild) | 3320 lines of guild handlers |
| Guilds | Guild settings (name, icon, banner, description) | IMPLEMENTED | `internal/api/guilds/guilds.go` (HandleUpdateGuild) | Full settings update with icon/banner support |
| Guilds | Guild discovery/explore | IMPLEMENTED | `internal/api/guilds/guilds.go` (HandleDiscoverGuilds, HandleGetGuildPreview, HandleJoinDiscoverableGuild), `web/src/routes/app/discover/` | Discoverable flag, preview, join |
| Guilds | Guild templates | IMPLEMENTED | `internal/api/guilds/templates.go` (HandleCreateGuildTemplate, HandleGetGuildTemplates, HandleGetGuildTemplate, HandleDeleteGuildTemplate, HandleApplyGuildTemplate) | Migration 029_guild_templates |
| Guilds | Vanity URLs | IMPLEMENTED | `internal/api/guilds/guilds.go` (HandleGetGuildVanityURL, HandleSetGuildVanityURL), `internal/api/social/social.go` (HandleClaimVanityURL, HandleReleaseVanityURL, HandleCheckVanityAvailability) | Migration 002, vanity_url column |
| Guilds | Verification levels | IMPLEMENTED | Migration 010_phase8_moderation adds verification_level to guilds | Server-level verification |
| Guilds | Audit log | IMPLEMENTED | `internal/api/guilds/guilds.go` (HandleGetGuildAuditLog), audit_log table in schema | Full audit log with actor, action, target, changes JSONB |
| Guilds | Guild ownership transfer | IMPLEMENTED | `internal/api/guilds/guilds.go` (HandleTransferGuildOwnership) | Transfer endpoint at `/{guildID}/transfer` |
| Guilds | Guild bumping | IMPLEMENTED | `internal/api/guilds/guilds.go` (HandleGetBumpStatus, HandleBumpGuild) | Boost guild visibility in discovery |
| Guilds | Server guide | IMPLEMENTED | `internal/api/guilds/guilds.go` (HandleGetServerGuide, HandleUpdateServerGuide) | Customizable server welcome guide |
| Guilds | Guild onboarding | IMPLEMENTED | `internal/api/onboarding/` (HandleGetOnboarding, HandleUpdateOnboarding, HandleCreatePrompt, HandleUpdatePrompt, HandleDeletePrompt, HandleCompleteOnboarding, HandleGetOnboardingStatus), `web/src/lib/components/guild/OnboardingModal.svelte` | Migration 022_guild_onboarding. Full prompt system |
| Guilds | Guild pruning | IMPLEMENTED | `internal/api/guilds/guilds.go` (HandleGetGuildPruneCount, HandleGuildPrune) | Prune inactive members |
| Guilds | Guild events | IMPLEMENTED | `internal/api/guildevents/` (HandleCreateEvent, HandleListEvents, HandleGetEvent, HandleUpdateEvent, HandleDeleteEvent, HandleRSVP, HandleDeleteRSVP, HandleListRSVPs) | Migration 023_event_reminders |
| Guilds | Retention policies | IMPLEMENTED | `internal/api/guilds/retention.go` (HandleGetGuildRetentionPolicies, HandleCreateGuildRetentionPolicy, HandleUpdateGuildRetentionPolicy, HandleDeleteGuildRetentionPolicy) | Migration 050_retention_enhancements |
| Guilds | Media gallery | IMPLEMENTED | `internal/api/guilds/guilds.go` (HandleGetGuildGallery, HandleGetMediaTags, HandleCreateMediaTag, HandleDeleteMediaTag) | Migration 049_media_gallery |

### Members & Roles

| Category | Feature | Status | Evidence (files) | Notes |
|----------|---------|--------|-------------------|-------|
| Members & Roles | Role CRUD with permissions | IMPLEMENTED | `internal/api/guilds/guilds.go` (HandleGetGuildRoles, HandleCreateGuildRole, HandleUpdateGuildRole, HandleDeleteGuildRole, HandleReorderGuildRoles), `web/src/lib/components/guild/RoleEditor.svelte` | Full CRUD with allow/deny bitfields |
| Members & Roles | Role hierarchy | IMPLEMENTED | `internal/permissions/permissions.go`, roles have position field, reorder endpoint | Position-based hierarchy, tests in permissions_test.go |
| Members & Roles | Role colors and icons | IMPLEMENTED | Schema: roles.color (CSS hex), roles.hoist (display separately) | Color and hoist support |
| Members & Roles | Member management (kick, ban, timeout) | IMPLEMENTED | `internal/api/guilds/guilds.go` (HandleRemoveGuildMember=kick, HandleCreateGuildBan, HandleRemoveGuildBan), guild_bans table, timeout_until in guild_members | Timed bans via migration 045_timed_bans |
| Members & Roles | Member nickname | IMPLEMENTED | Schema: guild_members.nickname, HandleUpdateGuildMember supports nickname | Per-guild nicknames |
| Members & Roles | Member roles assignment | IMPLEMENTED | `internal/api/guilds/guilds.go` (HandleGetMemberRoles, HandleAddMemberRole, HandleRemoveMemberRole) | Via member_roles table |
| Members & Roles | Invite system (create, revoke, vanity) | IMPLEMENTED | `internal/api/invites/` (HandleGetInvite, HandleAcceptInvite, HandleDeleteInvite), `internal/api/guilds/guilds.go` (HandleGetGuildInvites, HandleCreateGuildInvite), `web/src/lib/components/guild/InviteModal.svelte` | Full invite with max_uses, max_age, temporary |
| Members & Roles | Auto-roles | IMPLEMENTED | `internal/api/social/social.go` (HandleGetAutoRoles, HandleCreateAutoRole, HandleUpdateAutoRole, HandleDeleteAutoRole, ApplyAutoRoles), `web/src/lib/components/guild/AutoRoleSettings.svelte` | Automatic role assignment on join |
| Members & Roles | Level roles | IMPLEMENTED | `internal/api/social/social.go` (HandleAddLevelRole, HandleDeleteLevelRole, assignLevelRoles) | XP-based auto role assignment |
| Members & Roles | Member search | IMPLEMENTED | `internal/api/guilds/guilds.go` (HandleSearchGuildMembers) | Search within guild members |
| Members & Roles | Warning system | IMPLEMENTED | `internal/api/moderation/moderation.go` (HandleWarnMember L110, HandleGetWarnings L164, HandleDeleteWarning L216) | Guild-level member warnings |
| Members & Roles | @everyone role | IMPLEMENTED | Migration 046_everyone_role | Dedicated @everyone role handling |

### Users

| Category | Feature | Status | Evidence (files) | Notes |
|----------|---------|--------|-------------------|-------|
| Users | Registration / Login | IMPLEMENTED | `internal/api/server.go` (handleRegister, handleLogin), `internal/auth/auth.go` | Argon2id hashing, session tokens, registration modes (open/invite_only/closed) |
| Users | Profile (avatar, banner, bio, status) | IMPLEMENTED | `internal/api/users/users.go` (HandleGetSelf, HandleUpdateSelf, HandleGetUser), `web/src/lib/components/common/ProfileModal.svelte` | Avatar, display_name, bio, status_text, status_presence |
| Users | User settings | IMPLEMENTED | `internal/api/users/users.go` (HandleGetUserSettings, HandleUpdateUserSettings), `web/src/routes/app/settings/`, `web/src/lib/stores/settings.ts` | Full settings page with theme, notification, privacy prefs |
| Users | Privacy settings | IMPLEMENTED | User settings include privacy controls | Via settings store and API |
| Users | Two-factor auth (TOTP) | IMPLEMENTED | `internal/api/mfa_handlers.go` (handleTOTPEnable, handleTOTPVerify, handleTOTPDisable, handleGenerateBackupCodes, handleConsumeBackupCode) | TOTP with backup codes, QR URI generation |
| Users | WebAuthn/Passkeys | IMPLEMENTED | `internal/api/mfa_handlers.go` (handleWebAuthnRegisterBegin/Finish, handleWebAuthnLoginBegin/Finish), webauthn_credentials table | go-webauthn/webauthn integration |
| Users | Account deletion | IMPLEMENTED | `internal/api/users/users.go` (HandleDeleteSelf) | Self-service account deletion |
| Users | User blocking | IMPLEMENTED | `internal/api/users/users.go` (HandleBlockUser, HandleUpdateBlockLevel, HandleUnblockUser, HandleGetBlockedUsers), `web/src/lib/stores/blocked.ts` | Migration 019_user_blocks, 053_user_block_levels. Configurable block levels |
| Users | User relationships (friends) | IMPLEMENTED | `internal/api/users/users.go` (HandleAddFriend, HandleRemoveFriend, HandleGetRelationships, HandleGetMutualFriends), `web/src/lib/stores/relationships.ts`, `web/src/routes/app/friends/` | Friend/block/pending states |
| Users | DMs | IMPLEMENTED | `internal/api/users/users.go` (HandleCreateDM, HandleGetSelfDMs), `web/src/lib/stores/dms.ts`, `web/src/routes/app/dms/` | Direct messages with full store |
| Users | User notes | IMPLEMENTED | `internal/api/users/users.go` (HandleGetUserNote, HandleSetUserNote) | Migration 002_user_notes |
| Users | Session management | IMPLEMENTED | `internal/api/users/users.go` (HandleGetSelfSessions, HandleDeleteSelfSession), user_sessions table | View/revoke active sessions |
| Users | Data export | IMPLEMENTED | `internal/api/users/export.go` (HandleExportUserData, HandleExportAccount, HandleImportAccount, HandleExportChannelMessages) | Full data export/import |
| Users | User badges | IMPLEMENTED | `internal/api/users/badges.go` (HandleGetUserBadges) | User badge system |
| Users | Profile links | IMPLEMENTED | `internal/api/users/users.go` (HandleGetMyLinks, HandleCreateLink, HandleUpdateLink, HandleDeleteLink, HandleGetUserLinks), `web/src/lib/components/common/ProfileLinkEditor.svelte` | Migration 048_user_links |
| Users | User activity/status | IMPLEMENTED | `internal/api/users/activity.go` (HandleUpdateActivity, HandleGetActivity), custom activity support | Rich activity updates |
| Users | User emoji | IMPLEMENTED | `internal/api/users/emoji.go` (HandleGetUserEmoji, HandleCreateUserEmoji, HandleDeleteUserEmoji) | Per-user custom emoji |
| Users | Handle resolution | IMPLEMENTED | `internal/api/users/resolve.go` (HandleResolveHandle) | Resolve @user@domain format |
| Users | Mutual guilds | IMPLEMENTED | `internal/api/users/users.go` (HandleGetMutualGuilds) | Find shared guilds |
| Users | Password/email change | IMPLEMENTED | `internal/api/server.go` (handleChangePassword, handleChangeEmail) | Requires re-auth |

### Voice & Video

| Category | Feature | Status | Evidence (files) | Notes |
|----------|---------|--------|-------------------|-------|
| Voice & Video | Voice channels (join/leave/mute/deafen) | IMPLEMENTED | `internal/api/voice_handlers.go` (handleVoiceJoin, handleVoiceLeave, handleVoiceServerMute, handleVoiceServerDeafen), `internal/voice/voice.go`, `web/src/lib/stores/voice.ts`, `web/src/lib/components/voice/VoiceControls.svelte` | Full LiveKit integration with self/server mute/deafen |
| Voice & Video | Video calls | IMPLEMENTED | `web/src/lib/components/voice/VideoTile.svelte`, `web/src/lib/components/voice/CameraSettings.svelte`, `web/src/lib/stores/voice.ts` (selfCamera, videoTracks) | Camera with resolution/framerate preferences |
| Voice & Video | Screen sharing | IMPLEMENTED | `internal/api/voice_handlers.go` (handleStartScreenShare, handleStopScreenShare, handleUpdateScreenShare, handleGetScreenShares), `web/src/lib/components/voice/ScreenShareControls.svelte` | Screen share with audio, resolution/framerate settings |
| Voice & Video | Voice activity detection | IMPLEMENTED | `internal/voice/voice.go` (VoicePreferences.VADThreshold, InputMode "vad"/"ptt"), `web/src/lib/stores/voice.ts` | VAD threshold configurable, PTT alternative |
| Voice & Video | Noise suppression | IMPLEMENTED | `web/src/lib/utils/noiseReduction.ts` (128 lines), `internal/voice/voice.go` (NoiseSuppression field) | Web Audio BiquadFilter-based, per-user preference |
| Voice & Video | Volume control per user | IMPLEMENTED | `web/src/lib/utils/voiceVolume.ts` (134 lines), `internal/voice/voice.go` (InputVolume, OutputVolume) | GainNode-based per-user volume, RMS level computation |
| Voice & Video | Soundboard | IMPLEMENTED | `internal/api/voice_handlers.go` (handleGetSoundboardSounds, handleCreateSoundboardSound, handleDeleteSoundboardSound, handlePlaySoundboardSound, handleGetSoundboardConfig, handleUpdateSoundboardConfig), `web/src/lib/components/voice/Soundboard.svelte`, `web/src/lib/components/guild/SoundboardSettings.svelte` | Full soundboard with guild config |
| Voice & Video | Voice broadcast | IMPLEMENTED | `internal/api/voice_handlers.go` (handleStartBroadcast, handleStopBroadcast, handleGetBroadcast), `web/src/lib/components/voice/VoiceBroadcast.svelte` | One-way broadcast mode |
| Voice & Video | Voice preferences (PTT/VAD) | IMPLEMENTED | `internal/api/voice_handlers.go` (handleGetVoicePreferences, handleUpdateVoicePreferences, handleSetInputMode, handleSetPrioritySpeaker) | Full preferences persistence |
| Voice & Video | Voice transcription | IMPLEMENTED | `internal/api/experimental/experimental.go` (HandleGetTranscriptionSettings, HandleUpdateTranscriptionSettings, HandleGetTranscriptions), `web/src/lib/components/voice/Transcription.svelte` | Live voice transcription |
| Voice & Video | Incoming call modal | IMPLEMENTED | `web/src/lib/components/common/IncomingCallModal.svelte`, `web/src/lib/stores/callRing.ts` | Gateway event CALL_RING dispatched |

### Real-time

| Category | Feature | Status | Evidence (files) | Notes |
|----------|---------|--------|-------------------|-------|
| Real-time | WebSocket gateway | IMPLEMENTED | `internal/gateway/gateway.go` (1286 lines), `web/src/lib/api/ws.ts`, `web/src/lib/stores/gateway.ts` | Full protocol: HELLO, IDENTIFY, HEARTBEAT, RESUME, DISPATCH. 12 opcodes |
| Real-time | Typing indicators | IMPLEMENTED | `internal/api/channels/channels.go` (HandleTriggerTyping L1228), gateway dispatches TYPING_START, `web/src/lib/stores/typing.ts`, `web/src/lib/components/chat/TypingIndicator.svelte` | REST trigger + WS broadcast |
| Real-time | Presence (online/offline/idle/dnd) | IMPLEMENTED | `internal/presence/presence.go`, `web/src/lib/stores/presence.ts`, gateway PRESENCE_UPDATE events | DragonflyDB-backed with 6 statuses: online, idle, focus, busy, invisible, offline |
| Real-time | Read state tracking | IMPLEMENTED | `internal/api/channels/channels.go` (HandleAckChannel L1248), `internal/api/users/users.go` (HandleGetSelfReadState), `web/src/lib/stores/unreads.ts` | read_state table, CHANNEL_ACK events |
| Real-time | Unread counts | IMPLEMENTED | `web/src/lib/stores/unreads.ts` (incrementUnread), mention_count tracking in read_state | Per-channel unread + mention counts |
| Real-time | Gateway reconnection | IMPLEMENTED | `web/src/lib/stores/gateway.reconnect.ts`, gateway RESUME opcode | Automatic reconnect with session resume |
| Real-time | Idle detection | IMPLEMENTED | `web/src/lib/utils/idle.ts` (startIdleDetection, stopIdleDetection) | Auto-idle after inactivity |

### Moderation

| Category | Feature | Status | Evidence (files) | Notes |
|----------|---------|--------|-------------------|-------|
| Moderation | Ban/kick/timeout | IMPLEMENTED | `internal/api/guilds/guilds.go` (HandleCreateGuildBan, HandleRemoveGuildBan, HandleRemoveGuildMember), guild_members.timeout_until | Timed bans (migration 045), timeout with permission strip |
| Moderation | Message deletion (bulk) | IMPLEMENTED | `internal/api/channels/channels.go` (HandleBulkDeleteMessages L903) | Bulk delete with permission check |
| Moderation | Auto-moderation | IMPLEMENTED | `internal/automod/` (automod.go, filters.go, handlers.go: HandleListRules, HandleCreateRule, HandleGetRule, HandleUpdateRule, HandleDeleteRule, HandleTestRule, HandleGetActions) | 1400 lines total. Rule-based with test endpoint |
| Moderation | Moderation log | IMPLEMENTED | Audit log serves as moderation log, `internal/api/guilds/guilds.go` (HandleGetGuildAuditLog), `internal/api/moderation/moderation.go` (HandleGetModerationStats) | Dedicated moderation stats endpoint |
| Moderation | Warning system | IMPLEMENTED | `internal/api/moderation/moderation.go` (HandleWarnMember, HandleGetWarnings, HandleDeleteWarning) | Per-member guild warnings |
| Moderation | Channel locking | IMPLEMENTED | `internal/api/moderation/moderation.go` (HandleLockChannel L422, HandleUnlockChannel L467) | Lock with locked_by tracking |
| Moderation | Verification levels | IMPLEMENTED | guilds.verification_level (migration 010) | Per-guild verification |
| Moderation | Raid protection | IMPLEMENTED | `internal/api/moderation/moderation.go` (HandleGetRaidConfig L512, HandleUpdateRaidConfig L564, publishLockdownAlert L629) | Configurable raid detection with lockdown alerts |
| Moderation | User/message reporting | IMPLEMENTED | `internal/api/moderation/moderation.go` (HandleReportUser, HandleReportMessage, HandleReportToAdmin, HandleGetReports, HandleResolveReport, HandleGetUserReports, HandleGetAllMessageReports, HandleResolveUserReport, HandleResolveMessageReport), `web/src/lib/components/common/ModerationModals.svelte` | Multi-type reporting with admin resolution |
| Moderation | Ban lists | IMPLEMENTED | `internal/api/moderation/moderation.go` (HandleCreateBanList, HandleGetBanLists, HandleDeleteBanList, HandleGetBanListEntries, HandleAddBanListEntry, HandleRemoveBanListEntry, HandleExportBanList, HandleImportBanList, HandleGetPublicBanLists, HandleGetBanListSubscriptions, HandleSubscribeBanList, HandleUnsubscribeBanList) | Shared/subscribable ban lists with import/export (migration 020) |
| Moderation | Issue tracking | IMPLEMENTED | `internal/api/moderation/moderation.go` (HandleCreateIssue, HandleGetIssues, HandleResolveIssue, HandleGetMyIssues), reported_issues table | In-app bug/issue reporting for users |
| Moderation | Content scanning | IMPLEMENTED | `internal/api/admin/admin.go` (HandleGetContentScanRules, HandleCreateContentScanRule, HandleUpdateContentScanRule, HandleDeleteContentScanRule, HandleGetContentScanLog) | Admin content scanning rules + log |
| Moderation | DM spam detection | IMPLEMENTED | `internal/api/channels/channels.go` (dmSpamTracker, trackDMSend L68) | Content-hash based cross-recipient spam detection |

### Federation

| Category | Feature | Status | Evidence (files) | Notes |
|----------|---------|--------|-------------------|-------|
| Federation | Instance-to-instance protocol | IMPLEMENTED | `internal/federation/federation.go` (687 lines), `internal/federation/sync.go` (719 lines), `internal/federation/hlc.go` | HTTP-based with Ed25519 signing, HLC ordering |
| Federation | Federated guilds | IMPLEMENTED | `internal/federation/guild.go` (1128 lines), commit 7e51c77 "Add federated guilds" | Cross-instance guild membership, messaging, cache |
| Federation | Federated DMs | IMPLEMENTED | `internal/federation/dm.go` (686 lines), commit da92ef1 "Add federated DMs" | Cross-instance direct messages |
| Federation | Federated voice | IMPLEMENTED | `internal/federation/voice.go` (466 lines), commit 804f1d3 "Add federated voice" | Cross-instance voice, video, screenshare |
| Federation | Instance health tracking | IMPLEMENTED | `internal/federation/federation.go`, commit 3b09aab "Add federation foundation: retry queue, health tracking" | Health tracking with retry |
| Federation | Retry queue | IMPLEMENTED | `internal/federation/federation.go`, commit 3b09aab | NATS JetStream-backed offline queuing |
| Federation | Federation admin dashboard | IMPLEMENTED | `internal/api/admin/federation.go` (HandleGetFederationDashboard, HandleUpdatePeerControl, HandleGetPeerControls, HandleGetDeliveryReceipts, HandleRetryDelivery, HandleGetFederatedSearchConfig, HandleUpdateFederatedSearchConfig, HandleGetProtocolInfo, HandleUpdateProtocolConfig), `web/src/routes/app/admin/federation/` | Full admin UI for federation management |
| Federation | Instance blocklist/allowlist | IMPLEMENTED | `internal/api/admin/federation.go` (HandleGetInstanceBlocklist, HandleGetInstanceAllowlist, HandleGetInstanceProfiles, HandleAddInstanceProfile, HandleRemoveInstanceProfile) | Per-instance federation control |
| Federation | Federation badges | IMPLEMENTED | `web/src/lib/components/common/FederationBadge.svelte` | Visual indicator for federated content |
| Federation | Instance switcher | IMPLEMENTED | `web/src/lib/components/layout/InstanceSwitcher.svelte`, `web/src/lib/stores/instances.ts` | Multi-instance UI |

### Encryption

| Category | Feature | Status | Evidence (files) | Notes |
|----------|---------|--------|-------------------|-------|
| Encryption | End-to-end encryption (MLS) | IMPLEMENTED | `internal/encryption/service.go` (441 lines: HandleUploadKeyPackage, HandleGetKeyPackages, HandleClaimKeyPackage, HandleDeleteKeyPackage, HandleSendWelcome, HandleGetWelcomes, HandleAckWelcome, HandleGetGroupState, HandleUpdateGroupState, HandlePublishCommit, HandleGetCommits), `web/src/lib/encryption/` (crypto.ts, e2eeManager.ts, keyStore.ts) | MLS delivery service, key package management, group state, migration 006 |
| Encryption | Key management | IMPLEMENTED | `web/src/lib/encryption/keyStore.ts` (160 lines), `internal/encryption/service.go` | Client-side key store + server-side key package management |
| Encryption | Device verification | PARTIAL | `web/src/lib/encryption/e2eeManager.ts` (89 lines) | E2EE manager exists but device cross-verification UI (QR scan, emoji comparison) is minimal |
| Encryption | Encrypted channels | IMPLEMENTED | Schema: channels.encrypted boolean, messages.encrypted boolean, `internal/api/channels/channels.go` (HandleBatchDecryptMessages L2787) | Per-channel encryption toggle, batch decrypt |
| Encryption | Key backup | IMPLEMENTED | `internal/api/server.go` (HandleCreateKeyBackup, HandleGetKeyBackup, HandleDownloadKeyBackup, HandleDeleteKeyBackup, HandleGenerateRecoveryCodes) | Server-side encrypted key backup with recovery codes |
| Encryption | Encryption panel UI | IMPLEMENTED | `web/src/lib/components/encryption/EncryptionPanel.svelte` | Frontend encryption management |

### Admin & Self-hosting

| Category | Feature | Status | Evidence (files) | Notes |
|----------|---------|--------|-------------------|-------|
| Admin | Admin dashboard | IMPLEMENTED | `internal/api/admin/admin.go` (HandleGetStats, HandleListUsers, HandleListGuilds, HandleGetGuildDetails), `web/src/routes/app/admin/` | Full admin panel with stats, user mgmt, guild mgmt |
| Admin | Instance settings | IMPLEMENTED | `internal/api/admin/admin.go` (HandleGetInstance, HandleUpdateInstance) | Instance name, description, federation mode |
| Admin | User management (admin) | IMPLEMENTED | `internal/api/admin/admin.go` (HandleSuspendUser, HandleUnsuspendUser, HandleSetAdmin, HandleSetGlobalMod, HandleInstanceBanUser, HandleInstanceUnbanUser, HandleGetInstanceBans) | Suspend, admin flag, global mod, instance bans |
| Admin | System health monitoring | IMPLEMENTED | `internal/api/health.go` (handleDeepHealthCheck), `internal/api/admin/selfhost.go` (HandleGetHealthDashboard L326, HandleGetHealthHistory L465), `web/src/lib/components/admin/HealthMonitor.svelte` | Deep health check: DB, NATS, cache, S3, search. Dashboard with history |
| Admin | Backup/restore | IMPLEMENTED | `internal/api/admin/selfhost.go` (HandleGetBackupSchedules, HandleCreateBackupSchedule, HandleUpdateBackupSchedule, HandleDeleteBackupSchedule, HandleGetBackupHistory, HandleTriggerBackup), `web/src/lib/components/admin/BackupScheduler.svelte`, `scripts/backup.sh`, `scripts/restore.sh` | Scheduled backups + manual trigger + scripts |
| Admin | Self-host setup wizard | IMPLEMENTED | `internal/api/admin/selfhost.go` (HandleGetSetupStatus, HandleCompleteSetup), `web/src/routes/setup/` | First-run setup wizard |
| Admin | Registration management | IMPLEMENTED | `internal/api/admin/admin.go` (HandleGetRegistrationConfig, HandleUpdateRegistrationConfig, HandleCreateRegistrationToken, HandleListRegistrationTokens, HandleDeleteRegistrationToken) | Open/invite_only/closed modes with tokens |
| Admin | Announcements | IMPLEMENTED | `internal/api/admin/admin.go` (HandleCreateAnnouncement, HandleGetAnnouncements, HandleListAllAnnouncements, HandleUpdateAnnouncement, HandleDeleteAnnouncement), `web/src/lib/components/common/AnnouncementBanner.svelte` | Instance-wide announcements |
| Admin | Rate limit management | IMPLEMENTED | `internal/api/admin/admin.go` (HandleGetRateLimitStats, HandleGetRateLimitLog, HandleUpdateRateLimitConfig) | Admin rate limit monitoring and config |
| Admin | Storage dashboard | IMPLEMENTED | `internal/api/admin/selfhost.go` (HandleGetStorageDashboard L524), `web/src/lib/components/admin/StorageDashboard.svelte` | S3 storage usage monitoring |
| Admin | Retention policies | IMPLEMENTED | `internal/api/admin/selfhost.go` (HandleGetRetentionPolicies, HandleCreateRetentionPolicy, HandleUpdateRetentionPolicy, HandleDeleteRetentionPolicy, HandleRunRetentionPolicy), `web/src/lib/components/admin/RetentionSettings.svelte` | Data retention with manual run |
| Admin | Custom domains | IMPLEMENTED | `internal/api/admin/selfhost.go` (HandleGetCustomDomains, HandleCreateCustomDomain, HandleVerifyCustomDomain, HandleDeleteCustomDomain), `web/src/lib/components/admin/DomainSettings.svelte` | Custom domain with DNS verification |
| Admin | Update notifications | IMPLEMENTED | `internal/api/admin/selfhost.go` (HandleCheckUpdates, HandleSetLatestVersion, HandleDismissUpdate, HandleGetUpdateConfig, HandleUpdateUpdateConfig), `web/src/lib/components/admin/UpdateNotifications.svelte` | Version check with dismiss |
| Admin | CAPTCHA config | IMPLEMENTED | `internal/api/admin/admin.go` (HandleGetCaptchaConfig, HandleUpdateCaptchaConfig) | Admin CAPTCHA toggle |
| Admin | Admin media management | IMPLEMENTED | `internal/api/admin/selfhost.go` (HandleAdminGetMedia, HandleAdminDeleteMedia), `web/src/lib/components/gallery/AdminMediaPanel.svelte` | Browse/delete uploaded media |
| Admin | Bridge management | IMPLEMENTED | `internal/api/admin/federation.go` (HandleGetBridges, HandleCreateBridge, HandleUpdateBridge, HandleDeleteBridge, HandleGetBridgeChannelMappings, HandleCreateBridgeChannelMapping, HandleDeleteBridgeChannelMapping, HandleGetBridgeVirtualUsers), `web/src/routes/app/admin/bridges/` | Full bridge CRUD with channel mappings |

### Bots & Integrations

| Category | Feature | Status | Evidence (files) | Notes |
|----------|---------|--------|-------------------|-------|
| Bots | Bot SDK | IMPLEMENTED | `sdk/go/amityvox/` (bot.go 443L, client.go 366L, events.go 244L) | Go SDK with REST client, event handling, bot lifecycle |
| Bots | Bot accounts | IMPLEMENTED | `internal/api/bots/bots.go` (1453 lines: HandleCreateBot, HandleGetBot, HandleUpdateBot, HandleDeleteBot, HandleListMyBots, HandleAdminListAllBots) | Full bot account management |
| Bots | Bot tokens | IMPLEMENTED | `internal/api/bots/bots.go` (HandleListTokens, HandleCreateToken, HandleDeleteToken) | Multiple tokens per bot |
| Bots | Bot commands | IMPLEMENTED | `internal/api/bots/bots.go` (HandleListCommands, HandleRegisterCommand, HandleUpdateCommand, HandleDeleteCommand) | Slash command registration |
| Bots | Bot interactions | IMPLEMENTED | `internal/api/bots/bots.go` (HandleComponentInteraction) | Component interaction handling |
| Bots | Bot permissions | IMPLEMENTED | `internal/api/bots/bots.go` (HandleGetBotGuildPermissions, HandleUpdateBotGuildPermissions) | Per-guild bot permissions |
| Bots | Bot presence/rate limits | IMPLEMENTED | `internal/api/bots/bots.go` (HandleGetBotPresence, HandleUpdateBotPresence, HandleGetBotRateLimit, HandleUpdateBotRateLimit) | Bot-specific presence and rate limit config |
| Bots | Bot event subscriptions | IMPLEMENTED | `internal/api/bots/bots.go` (HandleCreateEventSubscription, HandleListEventSubscriptions, HandleDeleteEventSubscription) | Selective event subscription |
| Bots | Webhooks | IMPLEMENTED | `internal/api/webhooks/webhooks.go` (984 lines: HandleExecute, HandleGetWebhookLogs, HandleGetWebhookTemplates, HandlePreviewWebhookMessage, HandleGetOutgoingEvents), outgoing webhook subscriber | Incoming + outgoing webhooks with execution logs, templates, preview |
| Bots | Bridge adapters - Matrix | IMPLEMENTED | `bridges/matrix/main.go` | Full Matrix appservice bridge: channel mapping, message relay, masquerade |
| Bots | Bridge adapters - Discord | IMPLEMENTED | `bridges/discord/main.go` | Discord bot bridge: channel mapping, webhook relay |
| Bots | Bridge adapters - Telegram | IMPLEMENTED | `bridges/telegram/telegram.go` | Telegram bot bridge: text, photo, document, typing |
| Bots | Bridge adapters - Slack | IMPLEMENTED | `bridges/slack/slack.go` | Slack bridge: Socket Mode, Events API, file attachments, threads |
| Bots | Bridge adapters - IRC | IMPLEMENTED | `bridges/irc/irc.go` | IRC bridge: multi-network, nick changes, joins/parts/quits |
| Bots | Giphy integration | IMPLEMENTED | `internal/api/giphy_handler.go` (handleGiphySearch, handleGiphyTrending, handleGiphyCategories), `web/src/lib/components/common/GiphyPicker.svelte` | Giphy proxy with search/trending/categories |
| Bots | Plugin system | PARTIAL | `internal/plugins/plugins.go` + `internal/plugins/sandbox.go`, `internal/api/widgets/` (HandleListPlugins, HandleGetPlugin, HandleInstallPlugin, HandleGetGuildPlugins, HandleUpdateGuildPlugin, HandleUninstallPlugin), `web/src/lib/components/guild/PluginSettings.svelte`, `web/src/routes/app/plugins/` | WASM plugin runtime with hooks, manifest, sandbox. Installation/management UI exists. Actual WASM execution sandbox may be limited in scope |
| Bots | Integrations | IMPLEMENTED | `internal/api/integrations/integrations.go` (739 lines: HandleListIntegrations, HandleCreateIntegration, HandleGetIntegration, HandleUpdateIntegration, HandleDeleteIntegration, HandleGetIntegrationLog), `web/src/lib/components/guild/IntegrationSettings.svelte` | Generic integration framework with ActivityPub follows |
| Bots | ActivityPub integration | PARTIAL | `internal/api/integrations/integrations.go` (HandleListActivityPubFollows, HandleAddActivityPubFollow, HandleRemoveActivityPubFollow) | Follow management exists but full ActivityPub protocol implementation (inbox/outbox, signing) may be partial |
| Bots | Bridge connections | IMPLEMENTED | `internal/api/integrations/integrations.go` (HandleListBridgeConnections, HandleCreateBridgeConnection, HandleUpdateBridgeConnection, HandleDeleteBridgeConnection) | Channel-level bridge connections |
| Bots | Bridge attribution | IMPLEMENTED | `web/src/lib/components/common/BridgeAttribution.svelte` | Visual bridge source indicator |

### Experimental Features

| Category | Feature | Status | Evidence (files) | Notes |
|----------|---------|--------|-------------------|-------|
| Experimental | Whiteboard | IMPLEMENTED | `internal/api/experimental/experimental.go` (HandleCreateWhiteboard, HandleGetWhiteboards, HandleUpdateWhiteboard, HandleGetWhiteboardState), `web/src/lib/components/channels/Whiteboard.svelte` | Collaborative whiteboard with state management |
| Experimental | Kanban board | IMPLEMENTED | `internal/api/experimental/experimental.go` (HandleCreateKanbanBoard, HandleGetKanbanBoard, HandleCreateKanbanColumn, HandleCreateKanbanCard, HandleMoveKanbanCard, HandleDeleteKanbanCard), `web/src/lib/components/channels/KanbanBoard.svelte` | Full kanban with columns, cards, drag-move |
| Experimental | Code snippets | IMPLEMENTED | `internal/api/experimental/experimental.go` (HandleCreateCodeSnippet, HandleGetCodeSnippet, HandleRunCodeSnippet), `web/src/lib/components/chat/CodeSnippet.svelte` | Create, view, run snippets |
| Experimental | Video recorder | IMPLEMENTED | `internal/api/experimental/experimental.go` (HandleCreateVideoRecording, HandleGetRecordings), `web/src/lib/components/chat/VideoRecorder.svelte` | Record and share video clips |
| Experimental | Location sharing | IMPLEMENTED | `internal/api/experimental/experimental.go` (HandleShareLocation, HandleUpdateLiveLocation, HandleStopLiveLocation, HandleGetLocationShares), `web/src/lib/components/chat/LocationShare.svelte` | Static + live location sharing |
| Experimental | Activities | IMPLEMENTED | `internal/api/activities/activities.go` (HandleListActivities, HandleGetActivity, HandleCreateActivity, HandleRateActivity, HandleStartActivitySession, HandleJoinActivitySession, HandleLeaveActivitySession, HandleEndActivitySession, HandleGetActiveSession, HandleUpdateActivityState), `web/src/lib/components/channels/ActivityFrame.svelte` | Activity sessions with join/leave, state sync |
| Experimental | Games | IMPLEMENTED | `internal/api/activities/activities.go` (HandleCreateGame, HandleJoinGame, HandleGameMove, HandleGetGame, HandleGetLeaderboard) | Built-in games with game state, moves, leaderboard |
| Experimental | Watch together | IMPLEMENTED | `internal/api/activities/activities.go` (HandleStartWatchTogether, HandleSyncWatchTogether) | Synchronized video watching |
| Experimental | Music party | IMPLEMENTED | `internal/api/activities/activities.go` (HandleStartMusicParty, HandleAddToMusicQueue) | Shared music queue |
| Experimental | Message effects | IMPLEMENTED | `internal/api/experimental/experimental.go` (HandleCreateMessageEffect), `web/src/lib/components/chat/MessageEffects.svelte` | Visual message effects |
| Experimental | Super reactions | IMPLEMENTED | `internal/api/experimental/experimental.go` (HandleAddSuperReaction, HandleGetSuperReactions) | Enhanced reaction animations |
| Experimental | Message summaries | PARTIAL | `internal/api/experimental/experimental.go` (HandleSummarizeMessages, HandleGetSummaries) | Summarization endpoint exists but depends on external AI/LLM service configuration that may not be present in all deployments |

### Social Features

| Category | Feature | Status | Evidence (files) | Notes |
|----------|---------|--------|-------------------|-------|
| Social | Guild insights | IMPLEMENTED | `internal/api/social/social.go` (HandleGetInsights, ensureTodaySnapshot), `web/src/lib/components/guild/GuildInsights.svelte` | Daily snapshots with member/message/voice analytics |
| Social | Guild boosts | IMPLEMENTED | `internal/api/social/social.go` (HandleGetBoosts, HandleCreateBoost, HandleRemoveBoost), `web/src/lib/components/guild/BoostPanel.svelte` | User boosting system with boost count |
| Social | Achievements | IMPLEMENTED | `internal/api/social/social.go` (HandleGetAchievements, HandleGetUserAchievements, HandleCheckAchievements, awardAchievement) | Achievement definitions + per-user progress |
| Social | Leveling/XP | IMPLEMENTED | `internal/api/social/social.go` (HandleGetLevelingConfig, HandleUpdateLevelingConfig, AwardXP, HandleGetLeaderboard, HandleGetMemberXP, assignLevelRoles), `web/src/lib/components/guild/LevelingSettings.svelte` | XP system with level roles, leaderboard |
| Social | Starboard | IMPLEMENTED | `internal/api/social/social.go` (HandleGetStarboardConfig, HandleUpdateStarboardConfig, HandleGetStarboardEntries, CheckStarboard), `web/src/lib/components/guild/StarboardSettings.svelte` | Configurable star threshold, auto-post to starboard |
| Social | Welcome messages | IMPLEMENTED | `internal/api/social/social.go` (HandleGetWelcomeConfig, HandleUpdateWelcomeConfig, SendWelcomeMessage), `web/src/lib/components/guild/WelcomeSettings.svelte` | Customizable welcome with template variables |

### Search

| Category | Feature | Status | Evidence (files) | Notes |
|----------|---------|--------|-------------------|-------|
| Search | Message search | IMPLEMENTED | `internal/api/search_handlers.go` (handleSearchMessages), `internal/search/search.go` (IndexMessages), `web/src/lib/components/chat/SearchModal.svelte` | Meilisearch with channel/guild/author filters |
| Search | Full-text search (Meilisearch) | IMPLEMENTED | `internal/search/search.go` (EnsureIndexes, 4 indexes: messages, users, guilds, channels) | Full Meilisearch integration |
| Search | User search | IMPLEMENTED | `internal/api/search_handlers.go` (handleSearchUsers), `internal/search/search.go` (IndexUsers) | Search users by name |
| Search | Guild search/discovery | IMPLEMENTED | `internal/api/search_handlers.go` (handleSearchGuilds), `internal/api/guilds/guilds.go` (HandleDiscoverGuilds) | Meilisearch + discovery endpoint |
| Search | Federated search | PARTIAL | `internal/api/admin/federation.go` (HandleGetFederatedSearchConfig, HandleUpdateFederatedSearchConfig) | Config endpoints exist but cross-instance search query forwarding and result aggregation not visible |

### Notifications

| Category | Feature | Status | Evidence (files) | Notes |
|----------|---------|--------|-------------------|-------|
| Notifications | Push notifications | IMPLEMENTED | `internal/notifications/notifications.go` (646 lines: HandleSubscribe, HandleListSubscriptions, HandleUnsubscribe, HandleGetVAPIDKey), migration 008_push_subscriptions | Web Push with VAPID keys |
| Notifications | Notification preferences | IMPLEMENTED | `internal/notifications/notifications.go` (HandleGetPreferences, HandleUpdatePreferences, HandleGetChannelPreferences, HandleUpdateChannelPreference, HandleDeleteChannelPreference), `web/src/lib/stores/notifications.ts` | Global + per-channel notification prefs |
| Notifications | Mention notifications | IMPLEMENTED | `web/src/lib/stores/notifications.ts` (type 'mention'), gateway dispatches with mention detection | In-app mention alerts |
| Notifications | DM notifications | IMPLEMENTED | `web/src/lib/stores/notifications.ts` (type 'dm'), gateway dispatches | In-app DM alerts |
| Notifications | Notification center UI | IMPLEMENTED | `web/src/lib/components/common/NotificationCenter.svelte` | Grouped notification panel |
| Notifications | DND scheduling | IMPLEMENTED | `web/src/lib/stores/settings.ts` (DndSchedule interface, isDndActive derived) | Time-based DND with scheduled quiet hours |
| Notifications | Notification sounds | IMPLEMENTED | `web/src/lib/utils/sounds.ts` (playNotificationSound), settings (notificationSoundsEnabled, notificationVolume, notificationSoundPreset) | Configurable sound presets and volume |
| Notifications | Channel muting | IMPLEMENTED | `web/src/lib/stores/muting.ts` (isChannelMuted, isGuildMuted, loadChannelMutePrefs), migration 043_channel_muting | Per-channel and per-guild mute |

### Themes & Customization

| Category | Feature | Status | Evidence (files) | Notes |
|----------|---------|--------|-------------------|-------|
| Themes | Theme presets | IMPLEMENTED | `web/src/lib/stores/settings.ts` (7 presets defined in app.css via data-theme attribute) | 7 built-in themes |
| Themes | Custom themes | IMPLEMENTED | `web/src/lib/stores/settings.ts` (CustomTheme interface, 18 color tokens), `web/src/lib/components/common/ImageCropper.svelte` | Full custom theme editor with all color tokens |
| Themes | Theme gallery/sharing | IMPLEMENTED | `internal/api/themes/themes.go` (HandleListSharedThemes, HandleShareTheme, HandleGetSharedTheme, HandleLikeTheme, HandleUnlikeTheme, HandleDeleteSharedTheme), `web/src/routes/app/themes/` | Share and browse community themes with likes |
| Themes | UI layout preferences | IMPLEMENTED | `web/src/lib/stores/layout.ts` (persisted sidebar/panel widths), `web/src/lib/components/common/ResizeHandle.svelte` | Resizable sidebar and panels |

### Other Notable Features

| Category | Feature | Status | Evidence (files) | Notes |
|----------|---------|--------|-------------------|-------|
| Other | Sticker packs | IMPLEMENTED | `internal/api/stickers/stickers.go` (670 lines), `web/src/lib/components/common/StickerPicker.svelte` | Guild + user sticker packs with sharing/cloning. Migration 021 |
| Other | Custom emoji | IMPLEMENTED | `internal/api/guilds/guilds.go` (HandleGetGuildEmoji, HandleCreateGuildEmoji, HandleUpdateGuildEmoji, HandleDeleteGuildEmoji), `web/src/lib/components/common/EmojiPicker.svelte` | Guild custom emoji with animated support |
| Other | Widgets (guild embeddable) | IMPLEMENTED | `internal/api/widgets/`, `web/src/lib/components/guild/WidgetSettings.svelte`, `web/src/lib/components/channels/WidgetPanel.svelte` | Embeddable guild widget + per-channel widgets |
| Other | Quick switcher | IMPLEMENTED | `web/src/lib/components/common/QuickSwitcher.svelte` | Keyboard-driven channel/guild switcher |
| Other | Command palette | IMPLEMENTED | `web/src/lib/components/common/CommandPalette.svelte` | Command palette for actions |
| Other | Keyboard shortcuts | IMPLEMENTED | `web/src/lib/components/common/KeyboardShortcuts.svelte`, `web/src/lib/components/common/KeyboardNav.svelte` | Global keyboard shortcuts with navigation |
| Other | Image lightbox | IMPLEMENTED | `web/src/lib/components/common/ImageLightbox.svelte` | Full-screen image viewer |
| Other | Connection indicator | IMPLEMENTED | `web/src/lib/components/common/ConnectionIndicator.svelte` | WebSocket connection status |
| Other | User popover | IMPLEMENTED | `web/src/lib/components/common/UserPopover.svelte` | Hover card for user info |
| Other | Context menus | IMPLEMENTED | `web/src/lib/components/common/ContextMenu.svelte`, `ContextMenuItem.svelte`, `ContextMenuDivider.svelte`, `voice/ParticipantContextMenu.svelte` | Full context menu system |
| Other | Cross-channel quote | IMPLEMENTED | `web/src/lib/components/chat/CrossChannelQuote.svelte` | Quote messages across channels |
| Other | Voice messages | IMPLEMENTED | `web/src/lib/components/chat/VoiceMessageRecorder.svelte`, `web/src/lib/components/chat/AudioPlayer.svelte` | Record and play voice messages. Migration 020 |
| Other | Read-only banner | IMPLEMENTED | `web/src/lib/components/chat/ReadOnlyBanner.svelte` | Visual indicator for locked/archived channels |
| Other | Lazy image loading | IMPLEMENTED | `web/src/lib/components/common/LazyImage.svelte` | Progressive image loading with blurhash |
| Other | Metrics/Prometheus | IMPLEMENTED | `internal/api/metrics.go` (handleMetrics), `/metrics` endpoint | Prometheus-compatible metrics |

## PARTIAL Features Detail

| Feature | What Exists | What's Missing or Limited |
|---------|-------------|---------------------------|
| Stage channels | Frontend component `StageChannelView.svelte` exists, voice join supports 'stage' type | Stage-specific features like hand raising queue, speaker management UI, audience/speaker role separation are not deeply implemented beyond the basic voice join |
| Device verification (E2EE) | `e2eeManager.ts` (89 lines) manages E2EE sessions | Cross-device verification ceremony (QR code scan, emoji comparison) is not present; devices are trusted implicitly upon key package upload |
| Plugin system (WASM) | Plugin runtime with hooks, manifest, sandbox files exist (`plugins.go`, `sandbox.go`). Admin install/uninstall endpoints and UI are complete | Actual WASM module loading and sandboxed execution depth is unclear; no plugin marketplace or distribution system |
| Federated search | Config management endpoints exist (HandleGetFederatedSearchConfig, HandleUpdateFederatedSearchConfig) | Cross-instance search query forwarding and result aggregation not visible in codebase; appears to be config-only |
| ActivityPub integration | Follow management (list, add, remove) within integration framework exists | Full ActivityPub protocol implementation (HTTP signatures, inbox/outbox, object vocabulary) may be limited to follow management |
| Message summaries | Backend endpoints HandleSummarizeMessages and HandleGetSummaries exist with caching | Depends on external AI/LLM service; may not function without additional configuration not included in default deployment |
| Voice transcription | Settings and transcription retrieval endpoints exist with frontend component | Depends on external speech-to-text service; may be infrastructure-dependent rather than self-contained |
| Code snippet execution | HandleRunCodeSnippet endpoint exists | Sandboxed code execution requires careful security consideration; execution environment details unclear |
| Video recording storage | HandleCreateVideoRecording and HandleGetRecordings exist | Recording lifecycle (storage quotas, automatic cleanup, transcoding) may need attention for production use |
| Super reactions | HandleAddSuperReaction and HandleGetSuperReactions exist | Animation rendering quality and cross-client consistency depend on frontend implementation depth |
| Location sharing (live) | HandleUpdateLiveLocation with live tracking exists | Live location updates require persistent WebSocket delivery; may need real-time testing at scale |
| Watch/music together | Session creation and sync/queue endpoints exist | Actual media streaming coordination (playback sync, buffering) depends on client-side implementation sophistication |

## STUB Features Detail

| Feature | What Exists | What's Missing |
|---------|-------------|----------------|
| File upload fallback | `stubHandler("upload_file")` in server.go when `s.Media == nil` | Returns 501 Not Implemented when S3 is not configured. This is intentional graceful degradation, not a bug |
| Mobile clients | Architecture designed API-first for mobile support | No React Native, Flutter, or native mobile code exists. Web app is responsive but no dedicated mobile apps |
| Desktop client (Tauri) | Mentioned in architecture.md as Phase 2 (Tauri 5-10MB) | No Tauri code exists. Web app served via Caddy is the only client interface |

## Architecture Coverage Notes

All features planned in `docs/architecture.md` Section 11 (MVP Scope) are IMPLEMENTED:
1. Core server (Go binary) -- Done
2. User auth (registration, login, sessions, TOTP 2FA) -- Done
3. Guilds (CRUD, join, leave, categories) -- Done
4. Channels (text, DMs, group DMs) -- Done
5. Messaging (send, edit, delete, reply, reactions, markdown, embeds) -- Done
6. Permissions (full bitfield system, roles, channel overrides) -- Done
7. File uploads (S3-based) -- Done
8. WebSocket gateway (real-time events, typing, presence) -- Done
9. Voice chat (LiveKit integration) -- Done
10. Web client (SvelteKit app) -- Done
11. Admin dashboard -- Done
12. Docker Compose deployment -- Done

Features listed as "NOT in v0.1.0" that have ALSO been implemented:
- Federation (v0.2.0) -- IMPLEMENTED
- E2E encryption / MLS (v0.2.0) -- IMPLEMENTED
- Matrix/Discord bridges (v0.3.0) -- IMPLEMENTED (plus Telegram, Slack, IRC)
- Video/screen sharing (v0.2.0) -- IMPLEMENTED
- Forum channels (v0.3.0) -- IMPLEMENTED
- Threads (v0.2.0) -- IMPLEMENTED
- Search / Meilisearch (v0.2.0) -- IMPLEMENTED
- Bot API (v0.2.0) -- IMPLEMENTED (with full Go SDK)
- Plugin/WASM system (v0.4.0+) -- PARTIAL

## Codebase Scale Summary

| Component | Files | Approximate Lines |
|-----------|-------|-------------------|
| Backend handlers (`internal/api/`) | 40+ Go files | ~20,000+ lines |
| Gateway (`internal/gateway/`) | 2 files | ~1,400 lines |
| Federation (`internal/federation/`) | 6 files | ~3,800 lines |
| Auth (`internal/auth/`) | 3 files | ~1,000+ lines |
| Voice (`internal/voice/`) | 2 files | ~500+ lines |
| Search (`internal/search/`) | 2 files | ~400+ lines |
| Encryption (`internal/encryption/`) | 3 files | ~500+ lines |
| AutoMod (`internal/automod/`) | 4 files | ~1,400 lines |
| Notifications (`internal/notifications/`) | 1 file | ~650 lines |
| Plugins (`internal/plugins/`) | 2 files | ~300+ lines |
| Frontend API client | 1 file | ~1,666 lines (234+ methods) |
| Frontend stores | 27+ files | ~3,000+ lines |
| Frontend components | 80+ Svelte files | ~10,000+ lines |
| Frontend encryption | 4 files | ~564 lines |
| Frontend tests | 33 test files | ~495 tests |
| Bridges | 5 adapters | ~2,000+ lines |
| Bot SDK | 3 files | ~1,053 lines |
| Migrations | 53 migration pairs (106 SQL files) | -- |
| Go test suites | 23+ test files | -- |
