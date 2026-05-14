# Large Svelte File Inventory

Last updated: 2026-05-14

The repo's 200-line target is a quality guardrail. The active bug backlog no
longer treats every legacy file above 200 lines as a blocking defect; this file
tracks the remaining refactor inventory so it can be reduced deliberately.

Highest-priority split candidates:

- `web/src/lib/components/layout/ChannelSidebar.svelte` - 1697 lines
- `web/src/lib/components/chat/MessageItem.svelte` - 1171 lines
- `web/src/lib/components/chat/MessageInput.svelte` - 1169 lines
- `web/src/lib/components/layout/ChannelGroups.svelte` - 904 lines
- `web/src/lib/components/settings/SettingsAppearanceTab.svelte` - 875 lines
- `web/src/routes/app/admin/federation/+page.svelte` - 841 lines
- `web/src/lib/components/guild/SoundboardSettings.svelte` - 810 lines
- `web/src/lib/components/guild/WebhookPanel.svelte` - 688 lines
- `web/src/routes/app/guilds/[guildId]/+page.svelte` - 632 lines
- `web/src/lib/components/guild/GuildOnboardingSettings.svelte` - 597 lines

Recommended next split order:

1. `ChannelSidebar.svelte`: extract channel tree, guild section controls, DM section, and sidebar modals.
2. `MessageInput.svelte`: extract attachment tray, picker actions, slowmode/status strip, and send orchestration helpers.
3. `MessageItem.svelte`: extract attachment rendering, action menu, embed rendering, and moderation actions.
4. `SettingsAppearanceTab.svelte`: extract theme editor, preview, import/export, and connected accounts.
5. Admin federation/bridge routes: split into dashboard, health, controls, delivery, and mapping tabs.
