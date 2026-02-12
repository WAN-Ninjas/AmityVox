<script lang="ts">
	import { api } from '$lib/api/client';

	let { guildId }: { guildId: string } = $props();

	interface BoostInfo {
		id: string;
		guild_id: string;
		user_id: string;
		tier: number;
		started_at: string;
		expires_at: string | null;
		active: boolean;
		username: string;
	}

	interface BoostSummary {
		boost_count: number;
		boost_tier: number;
		boosters: BoostInfo[];
		user_boosted: boolean;
	}

	let loading = $state(false);
	let error = $state('');
	let success = $state('');
	let summary = $state<BoostSummary | null>(null);
	let boosting = $state(false);

	async function loadBoosts() {
		loading = true;
		error = '';
		try {
			summary = await api.request<BoostSummary>('GET', `/guilds/${guildId}/boosts`);
		} catch (err: any) {
			error = err.message || 'Failed to load boosts';
		} finally {
			loading = false;
		}
	}

	async function toggleBoost() {
		boosting = true;
		error = '';
		success = '';
		try {
			if (summary?.user_boosted) {
				await api.request('DELETE', `/guilds/${guildId}/boosts`);
				success = 'Boost removed';
			} else {
				await api.request('POST', `/guilds/${guildId}/boosts`);
				success = 'Server boosted!';
			}
			await loadBoosts();
			setTimeout(() => (success = ''), 3000);
		} catch (err: any) {
			error = err.message || 'Failed to update boost';
		} finally {
			boosting = false;
		}
	}

	$effect(() => {
		if (guildId) loadBoosts();
	});

	const tierNames = ['No Tier', 'Tier 1', 'Tier 2', 'Tier 3'];
	const tierColors = ['text-text-muted', 'text-pink-400', 'text-purple-400', 'text-yellow-400'];
	const tierPerks = [
		[],
		['50 custom emoji slots', 'Better audio quality', 'Custom invite background'],
		['100 custom emoji slots', 'Upload limit: 50MB', 'Server banner'],
		['250 custom emoji slots', 'Upload limit: 100MB', 'Vanity URL', 'Animated icon']
	];

	function tierProgressText(count: number): string {
		if (count >= 14) return 'Max tier reached!';
		if (count >= 7) return `${14 - count} more boosts for Tier 3`;
		if (count >= 2) return `${7 - count} more boosts for Tier 2`;
		return `${2 - count} more boosts for Tier 1`;
	}

	function tierProgressPercent(count: number): number {
		if (count >= 14) return 100;
		if (count >= 7) return ((count - 7) / 7) * 100;
		if (count >= 2) return ((count - 2) / 5) * 100;
		return (count / 2) * 100;
	}
</script>

<div class="space-y-6">
	<div class="flex items-center justify-between">
		<h2 class="text-lg font-semibold text-text-primary">Server Boosts</h2>
	</div>

	{#if loading}
		<div class="flex items-center justify-center py-12">
			<div class="h-8 w-8 animate-spin rounded-full border-2 border-brand-500 border-t-transparent"></div>
		</div>
	{:else if error}
		<div class="rounded bg-red-500/10 px-4 py-3 text-sm text-red-400">{error}</div>
	{:else if summary}
		{#if success}
			<div class="rounded bg-green-500/10 px-4 py-3 text-sm text-green-400">{success}</div>
		{/if}

		<!-- Tier Display -->
		<div class="rounded-lg bg-bg-secondary p-6 text-center">
			<div class="mb-2 text-4xl font-bold {tierColors[summary.boost_tier]}">
				{tierNames[summary.boost_tier]}
			</div>
			<div class="text-sm text-text-muted">
				{summary.boost_count} {summary.boost_count === 1 ? 'boost' : 'boosts'}
			</div>

			<!-- Progress bar -->
			<div class="mx-auto mt-4 max-w-sm">
				<div class="h-2 overflow-hidden rounded-full bg-bg-tertiary">
					<div
						class="h-full rounded-full bg-gradient-to-r from-pink-500 to-purple-500 transition-all duration-500"
						style="width: {tierProgressPercent(summary.boost_count)}%"
					></div>
				</div>
				<div class="mt-1 text-xs text-text-muted">
					{tierProgressText(summary.boost_count)}
				</div>
			</div>

			<!-- Boost Button -->
			<button
				class="mt-4 rounded-lg px-6 py-2 font-medium text-white transition-colors"
				class:bg-pink-500={!summary.user_boosted}
				class:hover:bg-pink-600={!summary.user_boosted}
				class:bg-red-500={summary.user_boosted}
				class:hover:bg-red-600={summary.user_boosted}
				onclick={toggleBoost}
				disabled={boosting}
			>
				{#if boosting}
					Processing...
				{:else if summary.user_boosted}
					Remove Boost
				{:else}
					Boost This Server
				{/if}
			</button>
		</div>

		<!-- Tier Perks -->
		{#if summary.boost_tier > 0}
			<div class="rounded-lg bg-bg-secondary p-4">
				<h3 class="mb-3 text-sm font-semibold text-text-primary">Current Perks</h3>
				<ul class="space-y-1">
					{#each tierPerks[summary.boost_tier] as perk}
						<li class="flex items-center gap-2 text-sm text-text-secondary">
							<span class="text-green-400">&#10003;</span>
							{perk}
						</li>
					{/each}
				</ul>
			</div>
		{/if}

		<!-- Boosters List -->
		{#if summary.boosters.length > 0}
			<div class="rounded-lg bg-bg-secondary p-4">
				<h3 class="mb-3 text-sm font-semibold text-text-primary">
					Boosters ({summary.boosters.length})
				</h3>
				<div class="space-y-2">
					{#each summary.boosters as booster}
						<div class="flex items-center justify-between rounded bg-bg-tertiary px-3 py-2">
							<span class="text-sm text-text-primary">{booster.username}</span>
							<span class="text-xs text-text-muted">
								Since {new Date(booster.started_at).toLocaleDateString()}
							</span>
						</div>
					{/each}
				</div>
			</div>
		{/if}
	{/if}
</div>
