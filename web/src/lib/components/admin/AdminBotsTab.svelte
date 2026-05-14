<script lang="ts">
	import { api, type AdminBotWithDetails } from '$lib/api/client';
	import Avatar from '$components/common/Avatar.svelte';
	import type { User } from '$lib/types';

	let allBots = $state<AdminBotWithDetails[]>([]);
	let botsLoaded = $state(false);
	let loadingAllBots = $state(false);
	let expandedBotId = $state<string | null>(null);

	$effect(() => {
		if (!botsLoaded && !loadingAllBots) {
			loadAllBots();
		}
	});

	async function loadAllBots() {
		loadingAllBots = true;
		try {
			allBots = await api.getAdminBots();
			botsLoaded = true;
		} catch {
			try {
				const allUsers = await api.getAdminUsers({ limit: 200, query: '' });
				allBots = allUsers.filter((user: User) => (user.flags & 8) !== 0).map((user: User) => ({
					...user,
					guild_permissions: [],
					event_subscriptions: [],
					rate_limit: null,
					presence: null
				}));
				botsLoaded = true;
			} catch {
				allBots = [];
			}
		} finally {
			loadingAllBots = false;
		}
	}

	function toggleBotExpand(botId: string) {
		expandedBotId = expandedBotId === botId ? null : botId;
	}

	function formatScopes(scopes: string[]): string {
		if (!scopes || scopes.length === 0) return 'None';
		return scopes.map((scope) => scope.replace('.', ' ')).join(', ');
	}

	function presenceStatusColor(status: string): string {
		switch (status) {
			case 'online': return 'bg-green-500/20 text-green-400';
			case 'idle': return 'bg-yellow-500/20 text-yellow-400';
			case 'dnd': return 'bg-red-500/20 text-red-400';
			default: return 'bg-gray-500/20 text-gray-400';
		}
	}
</script>

<div class="mb-6">
	<h1 class="text-2xl font-bold text-text-primary">Bot Management</h1>
	<p class="mt-1 text-sm text-text-muted">Review bot accounts, guild permissions, event subscriptions, and limits.</p>
</div>

{#if loadingAllBots}
	<p class="text-sm text-text-muted">Loading bots...</p>
{:else if allBots.length === 0}
	<p class="text-sm text-text-muted">No bots found.</p>
{:else}
	<div class="space-y-3">
		{#each allBots as bot (bot.id)}
			<div class="rounded-lg bg-bg-secondary">
				<button class="flex w-full items-center gap-3 p-4 text-left hover:bg-bg-modifier" onclick={() => toggleBotExpand(bot.id)}>
					<Avatar name={bot.display_name ?? bot.username} size="md" />
					<div class="min-w-0 flex-1">
						<div class="flex items-center gap-2">
							<span class="font-semibold text-text-primary">@{bot.username}</span>
							{#if bot.presence}
								<span class="rounded px-2 py-0.5 text-xs {presenceStatusColor(bot.presence.status)}">{bot.presence.status}</span>
							{/if}
						</div>
						<p class="text-xs text-text-muted">{bot.id}</p>
					</div>
					<span class="text-text-muted">{expandedBotId === bot.id ? 'Hide' : 'Details'}</span>
				</button>
				{#if expandedBotId === bot.id}
					<div class="grid gap-4 border-t border-bg-modifier p-4 md:grid-cols-3">
						<div>
							<h4 class="mb-2 text-sm font-semibold text-text-primary">Guild Permissions</h4>
							<p class="text-xs text-text-muted">{bot.guild_permissions.length} grant{bot.guild_permissions.length === 1 ? '' : 's'}</p>
							{#each bot.guild_permissions.slice(0, 5) as permission}
								<p class="mt-1 truncate text-xs text-text-secondary">{permission.guild_id}: {formatScopes(permission.scopes)}</p>
							{/each}
						</div>
						<div>
							<h4 class="mb-2 text-sm font-semibold text-text-primary">Event Subscriptions</h4>
							<p class="text-xs text-text-muted">{bot.event_subscriptions.length} subscription{bot.event_subscriptions.length === 1 ? '' : 's'}</p>
							{#each bot.event_subscriptions.slice(0, 5) as subscription}
								<p class="mt-1 truncate text-xs text-text-secondary">{subscription.guild_id}: {subscription.event_types.join(', ')}</p>
							{/each}
						</div>
						<div>
							<h4 class="mb-2 text-sm font-semibold text-text-primary">Rate Limit</h4>
							{#if bot.rate_limit}
								<p class="text-xs text-text-secondary">{bot.rate_limit.requests_per_second}/sec, burst {bot.rate_limit.burst}</p>
							{:else}
								<p class="text-xs text-text-muted">No custom limit</p>
							{/if}
						</div>
					</div>
				{/if}
			</div>
		{/each}
	</div>
{/if}
