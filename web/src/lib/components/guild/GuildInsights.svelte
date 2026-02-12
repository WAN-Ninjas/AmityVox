<script lang="ts">
	import { api } from '$lib/api/client';

	let { guildId }: { guildId: string } = $props();

	interface DailyInsight {
		date: string;
		member_count: number;
		members_joined: number;
		members_left: number;
		messages_sent: number;
		reactions_added: number;
		voice_minutes: number;
		active_members: number;
	}

	interface HourlyInsight {
		hour: number;
		messages: number;
	}

	interface InsightsData {
		daily: DailyInsight[];
		peak_hours: HourlyInsight[];
		total_members: number;
		total_messages: number;
		growth_rate: number;
	}

	let loading = $state(false);
	let error = $state('');
	let insights = $state<InsightsData | null>(null);
	let days = $state(30);
	let activeChart = $state<'members' | 'messages' | 'activity'>('members');

	async function loadInsights() {
		loading = true;
		error = '';
		try {
			insights = await api.request<InsightsData>('GET', `/guilds/${guildId}/insights?days=${days}`);
		} catch (err: any) {
			error = err.message || 'Failed to load insights';
		} finally {
			loading = false;
		}
	}

	$effect(() => {
		if (guildId) loadInsights();
	});

	function maxValue(arr: number[]): number {
		return Math.max(...arr, 1);
	}

	function formatDate(dateStr: string): string {
		const d = new Date(dateStr);
		return d.toLocaleDateString(undefined, { month: 'short', day: 'numeric' });
	}

	function formatHour(hour: number): string {
		const suffix = hour >= 12 ? 'PM' : 'AM';
		const h = hour % 12 || 12;
		return `${h}${suffix}`;
	}

	let memberData = $derived(insights?.daily.map(d => d.member_count) ?? []);
	let messageData = $derived(insights?.daily.map(d => d.messages_sent) ?? []);
	let activeData = $derived(insights?.daily.map(d => d.active_members) ?? []);
	let dateLabels = $derived(insights?.daily.map(d => formatDate(d.date)) ?? []);

	let currentChartData = $derived(
		activeChart === 'members' ? memberData :
		activeChart === 'messages' ? messageData : activeData
	);

	let chartMax = $derived(maxValue(currentChartData));

	let peakHourMax = $derived(maxValue(insights?.peak_hours.map(h => h.messages) ?? []));

	let totalJoined = $derived(insights?.daily.reduce((sum, d) => sum + d.members_joined, 0) ?? 0);
	let totalLeft = $derived(insights?.daily.reduce((sum, d) => sum + d.members_left, 0) ?? 0);
</script>

<div class="space-y-6">
	<div class="flex items-center justify-between">
		<h2 class="text-lg font-semibold text-text-primary">Server Insights</h2>
		<select class="input w-32" bind:value={days} onchange={() => loadInsights()}>
			<option value={7}>7 days</option>
			<option value={14}>14 days</option>
			<option value={30}>30 days</option>
			<option value={60}>60 days</option>
			<option value={90}>90 days</option>
		</select>
	</div>

	{#if loading}
		<div class="flex items-center justify-center py-12">
			<div class="h-8 w-8 animate-spin rounded-full border-2 border-brand-500 border-t-transparent"></div>
		</div>
	{:else if error}
		<div class="rounded bg-red-500/10 px-4 py-3 text-sm text-red-400">{error}</div>
	{:else if insights}
		<!-- Summary Cards -->
		<div class="grid grid-cols-2 gap-3 lg:grid-cols-4">
			<div class="rounded-lg bg-bg-secondary p-4">
				<div class="text-xs font-bold uppercase tracking-wide text-text-muted">Members</div>
				<div class="mt-1 text-2xl font-bold text-text-primary">{insights.total_members.toLocaleString()}</div>
			</div>
			<div class="rounded-lg bg-bg-secondary p-4">
				<div class="text-xs font-bold uppercase tracking-wide text-text-muted">Messages ({days}d)</div>
				<div class="mt-1 text-2xl font-bold text-text-primary">{insights.total_messages.toLocaleString()}</div>
			</div>
			<div class="rounded-lg bg-bg-secondary p-4">
				<div class="text-xs font-bold uppercase tracking-wide text-text-muted">Joined / Left</div>
				<div class="mt-1 text-2xl font-bold text-text-primary">
					<span class="text-green-400">+{totalJoined}</span>
					<span class="text-text-muted">/</span>
					<span class="text-red-400">-{totalLeft}</span>
				</div>
			</div>
			<div class="rounded-lg bg-bg-secondary p-4">
				<div class="text-xs font-bold uppercase tracking-wide text-text-muted">Growth Rate</div>
				<div class="mt-1 text-2xl font-bold" class:text-green-400={insights.growth_rate >= 0} class:text-red-400={insights.growth_rate < 0}>
					{insights.growth_rate >= 0 ? '+' : ''}{insights.growth_rate}%
				</div>
			</div>
		</div>

		<!-- Chart Tabs -->
		<div class="rounded-lg bg-bg-secondary p-4">
			<div class="mb-4 flex gap-2">
				<button
					class="rounded px-3 py-1 text-sm font-medium transition-colors"
					class:bg-brand-500={activeChart === 'members'}
					class:text-white={activeChart === 'members'}
					class:text-text-muted={activeChart !== 'members'}
					onclick={() => (activeChart = 'members')}
				>Members</button>
				<button
					class="rounded px-3 py-1 text-sm font-medium transition-colors"
					class:bg-brand-500={activeChart === 'messages'}
					class:text-white={activeChart === 'messages'}
					class:text-text-muted={activeChart !== 'messages'}
					onclick={() => (activeChart = 'messages')}
				>Messages</button>
				<button
					class="rounded px-3 py-1 text-sm font-medium transition-colors"
					class:bg-brand-500={activeChart === 'activity'}
					class:text-white={activeChart === 'activity'}
					class:text-text-muted={activeChart !== 'activity'}
					onclick={() => (activeChart = 'activity')}
				>Active Members</button>
			</div>

			<!-- Bar Chart -->
			{#if currentChartData.length > 0}
				<div class="flex items-end gap-px" style="height: 160px;">
					{#each currentChartData as value, i}
						<div class="group relative flex flex-1 flex-col items-center justify-end">
							<div
								class="w-full min-w-1 rounded-t bg-brand-500 transition-all hover:bg-brand-400"
								style="height: {Math.max((value / chartMax) * 140, 2)}px"
							></div>
							<!-- Tooltip -->
							<div class="pointer-events-none absolute -top-8 z-10 hidden rounded bg-bg-tertiary px-2 py-1 text-xs text-text-primary shadow-lg group-hover:block">
								{value.toLocaleString()}
								{#if dateLabels[i]}
									<div class="text-text-muted">{dateLabels[i]}</div>
								{/if}
							</div>
						</div>
					{/each}
				</div>
				<!-- X-axis labels (sparse) -->
				<div class="mt-1 flex justify-between text-xs text-text-muted">
					{#if dateLabels.length > 0}
						<span>{dateLabels[0]}</span>
						{#if dateLabels.length > 1}
							<span>{dateLabels[dateLabels.length - 1]}</span>
						{/if}
					{/if}
				</div>
			{:else}
				<div class="flex items-center justify-center py-8 text-sm text-text-muted">
					No data available for this period
				</div>
			{/if}
		</div>

		<!-- Peak Hours -->
		<div class="rounded-lg bg-bg-secondary p-4">
			<h3 class="mb-3 text-sm font-semibold text-text-primary">Peak Hours (Last 7 days)</h3>
			{#if insights.peak_hours.length > 0}
				<div class="flex items-end gap-1" style="height: 100px;">
					{#each Array(24) as _, hour}
						{@const hourData = insights.peak_hours.find(h => h.hour === hour)}
						{@const value = hourData?.messages ?? 0}
						<div class="group relative flex flex-1 flex-col items-center justify-end">
							<div
								class="w-full min-w-1 rounded-t transition-all"
								class:bg-yellow-500={value === Math.max(...insights.peak_hours.map(h => h.messages))}
								class:bg-brand-500={value !== Math.max(...insights.peak_hours.map(h => h.messages))}
								class:hover:bg-brand-400={true}
								style="height: {Math.max((value / peakHourMax) * 80, 2)}px"
							></div>
							<div class="pointer-events-none absolute -top-8 z-10 hidden rounded bg-bg-tertiary px-2 py-1 text-xs text-text-primary shadow-lg group-hover:block">
								{value} msgs at {formatHour(hour)}
							</div>
						</div>
					{/each}
				</div>
				<div class="mt-1 flex justify-between text-xs text-text-muted">
					<span>12AM</span>
					<span>6AM</span>
					<span>12PM</span>
					<span>6PM</span>
					<span>11PM</span>
				</div>
			{:else}
				<div class="py-4 text-center text-sm text-text-muted">No hourly data yet</div>
			{/if}
		</div>
	{/if}
</div>
