# Large Svelte File Inventory

Last updated: 2026-05-17

The repo's 200-line target is a quality guardrail. The active bug backlog no
longer treats every legacy file above 200 lines as a blocking defect; this file
tracks the remaining refactor inventory so it can be reduced deliberately.

Highest-priority split candidates:

- `web/src/lib/components/layout/ChannelSidebar.svelte` - 1078 lines
- `web/src/lib/components/chat/MessageItem.svelte` - 989 lines
- `web/src/lib/components/chat/MessageInput.svelte` - 976 lines
- `web/src/lib/components/layout/ChannelGroups.svelte` - 905 lines
- `web/src/lib/components/settings/SettingsAppearanceTab.svelte` - 876 lines
- `web/src/routes/app/admin/federation/+page.svelte` - 849 lines
- `web/src/lib/components/guild/SoundboardSettings.svelte` - 811 lines
- `web/src/lib/components/guild/WebhookPanel.svelte` - 689 lines
- `web/src/routes/app/guilds/[guildId]/+page.svelte` - 633 lines
- `web/src/lib/components/guild/GuildOnboardingSettings.svelte` - 587 lines

Recent reduction:

- `ChannelSidebar.svelte` dropped from 1697 to 1078 lines by extracting the report issue modal, user panel, create/edit channel modals, and channel/DM/guild/thread context menus.
- New extracted sidebar files are below the 200-line target: `ChannelContextMenu.svelte` 180, `EditChannelModal.svelte` 157, `GuildContextMenu.svelte` 97, `CreateChannelModal.svelte` 87, `ThreadContextMenu.svelte` 77, `UserPanel.svelte` 61, `ReportIssueModal.svelte` 59, `DMContextMenu.svelte` 52.
- `MessageInput.svelte` dropped from 1170 to 976 lines by extracting pending-file preview, encrypted passphrase prompt, status bars, and schedule picker. New extracted files are below the 200-line target.
- `MessageItem.svelte` dropped from 1172 to 989 lines by extracting attachment rendering, embed/reaction rendering, and the message context menu. New extracted files are below the 200-line target.

Recommended next split order:

1. `ChannelSidebar.svelte`: continue with channel tree, thread context menu, and guild section controls.
2. `MessageInput.svelte`: continue with picker actions, mobile action tray, and send orchestration helpers.
3. `MessageItem.svelte`: continue with hover action bar, forward/quote/report modals, and moderation helpers.
4. `SettingsAppearanceTab.svelte`: extract theme editor, preview, import/export, and connected accounts.
5. Admin federation/bridge routes: split into dashboard, health, controls, delivery, and mapping tabs.
