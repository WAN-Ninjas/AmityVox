<script lang="ts">
	import { page } from '$app/stores';
	import { currentGuild } from '$lib/stores/guilds';
	import { currentUser } from '$lib/stores/auth';
	import { goto } from '$app/navigation';
	import SoundboardSettings from '$lib/components/guild/SoundboardSettings.svelte';
	import AutoRoleSettings from '$lib/components/guild/AutoRoleSettings.svelte';
	import LevelingSettings from '$lib/components/guild/LevelingSettings.svelte';
	import StarboardSettings from '$lib/components/guild/StarboardSettings.svelte';
	import WelcomeSettings from '$lib/components/guild/WelcomeSettings.svelte';
	import BoostPanel from '$lib/components/guild/BoostPanel.svelte';
	import GuildInsights from '$lib/components/guild/GuildInsights.svelte';
	import GuildRetentionSettings from '$lib/components/guild/GuildRetentionSettings.svelte';
	import GuildInvitesSettings from '$lib/components/guild/GuildInvitesSettings.svelte';
	import GuildBansSettings from '$lib/components/guild/GuildBansSettings.svelte';
	import GuildCategoriesSettings from '$lib/components/guild/GuildCategoriesSettings.svelte';
	import GuildEmojiSettings from '$lib/components/guild/GuildEmojiSettings.svelte';
	import GuildStickersSettings from '$lib/components/guild/GuildStickersSettings.svelte';
	import GuildWebhooksSettings from '$lib/components/guild/GuildWebhooksSettings.svelte';
	import GuildAuditSettings from '$lib/components/guild/GuildAuditSettings.svelte';
	import GuildAutomodSettings from '$lib/components/guild/GuildAutomodSettings.svelte';
	import GuildModerationSettings from '$lib/components/guild/GuildModerationSettings.svelte';
	import GuildRaidSettings from '$lib/components/guild/GuildRaidSettings.svelte';
	import GuildOnboardingSettings from '$lib/components/guild/GuildOnboardingSettings.svelte';
	import GuildBanListsSettings from '$lib/components/guild/GuildBanListsSettings.svelte';
	import GuildChannelTemplatesSettings from '$lib/components/guild/GuildChannelTemplatesSettings.svelte';
	import GuildRolesSettings from '$lib/components/guild/GuildRolesSettings.svelte';
	import GuildOverviewSettings from '$lib/components/guild/GuildOverviewSettings.svelte';
	import GuildMembersSettings from '$lib/components/guild/GuildMembersSettings.svelte';
	import { canManageGuild, canManageRoles, canBanMembers, canKickMembers, canViewAuditLog } from '$lib/stores/permissions';

	type Tab = 'overview' | 'boosts' | 'roles' | 'auto-roles' | 'members' | 'categories' | 'invites' | 'bans' | 'emoji' | 'soundboard' | 'stickers' | 'webhooks' | 'audit' | 'insights' | 'automod' | 'moderation' | 'leveling' | 'raid' | 'onboarding' | 'starboard' | 'welcome' | 'ban-lists' | 'templates' | 'retention';
	let currentTab = $state<Tab>('overview');

	const routeGuildId = $derived($page.params.guildId);
	const isOwner = $derived($currentGuild?.owner_id === $currentUser?.id);

	// --- Helpers ---

	const allTabs: { id: Tab; label: string }[] = [
		{ id: 'overview', label: 'Overview' },
		{ id: 'boosts', label: 'Boosts' },
		{ id: 'roles', label: 'Roles' },
		{ id: 'members', label: 'Members' },
		{ id: 'auto-roles', label: 'Auto Roles' },
		{ id: 'categories', label: 'Categories' },
		{ id: 'invites', label: 'Invites' },
		{ id: 'bans', label: 'Bans' },
		{ id: 'emoji', label: 'Emoji' },
		{ id: 'soundboard', label: 'Soundboard' },
		{ id: 'stickers', label: 'Stickers' },
		{ id: 'webhooks', label: 'Webhooks' },
		{ id: 'audit', label: 'Audit Log' },
		{ id: 'insights', label: 'Insights' },
		{ id: 'automod', label: 'AutoMod' },
		{ id: 'moderation', label: 'Moderation' },
		{ id: 'leveling', label: 'Leveling' },
		{ id: 'raid', label: 'Raid Protection' },
		{ id: 'onboarding', label: 'Onboarding' },
		{ id: 'starboard', label: 'Starboard' },
		{ id: 'welcome', label: 'Welcome' },
		{ id: 'ban-lists', label: 'Ban Lists' },
		{ id: 'templates', label: 'Templates' },
		{ id: 'retention', label: 'Message Retention' }
	];

	// Permission-gated tabs: only show tabs the user has permissions for.
	const permissionGatedTabs: Record<string, () => boolean> = {
		'roles': () => isOwner || $canManageRoles,
		'members': () => isOwner || $canManageRoles || $canKickMembers,
		'bans': () => isOwner || $canBanMembers,
		'ban-lists': () => isOwner || $canBanMembers,
		'audit': () => isOwner || $canViewAuditLog,
	};

	const tabs = $derived(allTabs.filter((tab) => {
		const gate = permissionGatedTabs[tab.id];
		return !gate || gate();
	}));

	// Tabs that need full width instead of max-w-xl.
	const wideContentTabs = new Set<Tab>(['roles', 'members', 'webhooks', 'audit', 'automod', 'moderation', 'ban-lists', 'onboarding']);

</script>

<svelte:head>
	<title>Server Settings — AmityVox</title>
</svelte:head>

{#if isOwner || $canManageGuild}
<div class="flex h-full">
	<nav class="w-52 shrink-0 overflow-y-auto bg-bg-secondary p-4">
		<h3 class="mb-2 text-xs font-bold uppercase tracking-wide text-text-muted">Server Settings</h3>
		<ul class="space-y-0.5">
			{#each tabs as tab (tab.id)}
				<li>
					<button
						class="w-full rounded px-2 py-1.5 text-left text-sm transition-colors {currentTab === tab.id ? 'bg-bg-modifier text-text-primary' : 'text-text-muted hover:bg-bg-modifier hover:text-text-secondary'}"
						onclick={() => (currentTab = tab.id)}
					>
						{tab.label}
					</button>
				</li>
			{/each}
		</ul>
		<div class="my-2 border-t border-bg-modifier"></div>
		<button
			class="w-full rounded px-2 py-1.5 text-left text-sm text-text-muted hover:bg-bg-modifier hover:text-text-secondary"
			onclick={() => routeGuildId && goto(`/app/guilds/${routeGuildId}`)}
		>
			Back to server
		</button>
	</nav>

	<div class="flex-1 overflow-y-auto bg-bg-tertiary p-8">
		<div class={wideContentTabs.has(currentTab) ? '' : 'max-w-xl'}>
			<!-- ==================== OVERVIEW ==================== -->
			{#if currentTab === 'overview'}
				{#if $currentGuild}
					<GuildOverviewSettings guild={$currentGuild} {isOwner} />
				{/if}

			<!-- ==================== ROLES ==================== -->
			{:else if currentTab === 'roles'}
				{#if $currentGuild}
					<GuildRolesSettings guildId={$currentGuild.id} />
				{/if}

			<!-- ==================== MEMBERS ==================== -->
			{:else if currentTab === 'members'}
				{#if $currentGuild}
					<GuildMembersSettings guildId={$currentGuild.id} />
				{/if}

			<!-- ==================== CATEGORIES ==================== -->
			{:else if currentTab === 'categories'}
				{#if $currentGuild}
					<GuildCategoriesSettings guildId={$currentGuild.id} />
				{/if}

			<!-- ==================== INVITES ==================== -->
			{:else if currentTab === 'invites'}
				{#if $currentGuild}
					<GuildInvitesSettings guildId={$currentGuild.id} />
				{/if}

			<!-- ==================== BANS ==================== -->
			{:else if currentTab === 'bans'}
				{#if $currentGuild}
					<GuildBansSettings guildId={$currentGuild.id} />
				{/if}

			<!-- ==================== EMOJI ==================== -->
			{:else if currentTab === 'emoji'}
				{#if $currentGuild}
					<GuildEmojiSettings guildId={$currentGuild.id} instanceId={$currentGuild.instance_id} />
				{/if}

			<!-- ==================== STICKERS ==================== -->
			{:else if currentTab === 'stickers'}
				{#if $currentGuild}
					<GuildStickersSettings guildId={$currentGuild.id} instanceId={$currentGuild.instance_id} />
				{/if}

			<!-- ==================== WEBHOOKS ==================== -->
			{:else if currentTab === 'webhooks'}
				{#if $currentGuild}
					<GuildWebhooksSettings guildId={$currentGuild.id} />
				{/if}

			<!-- ==================== AUDIT LOG ==================== -->
			{:else if currentTab === 'audit'}
				{#if $currentGuild}
					<GuildAuditSettings guildId={$currentGuild.id} />
				{/if}

			<!-- ==================== AUTOMOD ==================== -->
			{:else if currentTab === 'automod'}
				{#if $currentGuild}
					<GuildAutomodSettings guildId={$currentGuild.id} />
				{/if}

			<!-- ==================== MODERATION ==================== -->
			{:else if currentTab === 'moderation'}
				{#if $currentGuild}
					<GuildModerationSettings guildId={$currentGuild.id} />
				{/if}

			<!-- ==================== RAID PROTECTION ==================== -->
			{:else if currentTab === 'raid'}
				{#if $currentGuild}
					<GuildRaidSettings guildId={$currentGuild.id} />
				{/if}

			<!-- ==================== ONBOARDING ==================== -->
			{:else if currentTab === 'onboarding'}
				{#if $currentGuild}
					<GuildOnboardingSettings guildId={$currentGuild.id} />
				{/if}

			<!-- ==================== BAN LISTS ==================== -->
			{:else if currentTab === 'ban-lists'}
				{#if $currentGuild}
					<GuildBanListsSettings guildId={$currentGuild.id} />
				{/if}

			<!-- ==================== SOUNDBOARD ==================== -->
			{:else if currentTab === 'soundboard'}
				{#if routeGuildId}
					<SoundboardSettings guildId={routeGuildId} />
				{/if}

			<!-- ==================== AUTO ROLES ==================== -->
			{:else if currentTab === 'auto-roles'}
				{#if routeGuildId}
					<AutoRoleSettings guildId={routeGuildId} />
				{/if}

			<!-- ==================== LEVELING ==================== -->
			{:else if currentTab === 'leveling'}
				{#if routeGuildId}
					<LevelingSettings guildId={routeGuildId} />
				{/if}

			<!-- ==================== STARBOARD ==================== -->
			{:else if currentTab === 'starboard'}
				{#if routeGuildId}
					<StarboardSettings guildId={routeGuildId} />
				{/if}

			<!-- ==================== WELCOME ==================== -->
			{:else if currentTab === 'welcome'}
				{#if routeGuildId}
					<WelcomeSettings guildId={routeGuildId} />
				{/if}

			<!-- ==================== BOOSTS ==================== -->
			{:else if currentTab === 'boosts'}
				{#if routeGuildId}
					<BoostPanel guildId={routeGuildId} />
				{/if}

			<!-- ==================== INSIGHTS ==================== -->
			{:else if currentTab === 'insights'}
				{#if routeGuildId}
					<GuildInsights guildId={routeGuildId} />
				{/if}

			<!-- ==================== TEMPLATES ==================== -->
			{:else if currentTab === 'templates'}
				{#if routeGuildId}
					<GuildChannelTemplatesSettings guildId={routeGuildId} />
				{/if}

			<!-- ==================== MESSAGE RETENTION ==================== -->
			{:else if currentTab === 'retention'}
				{#if routeGuildId}
					<GuildRetentionSettings guildId={routeGuildId} />
				{/if}
			{/if}
		</div>
	</div>
</div>
{:else}
<div class="flex h-full items-center justify-center">
	<p class="text-text-muted">You don't have permission to view server settings.</p>
</div>
{/if}
