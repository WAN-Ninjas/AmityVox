<!-- ActivityFrame.svelte — Iframe-based activity container for embedded activities and games. -->
<script lang="ts">
	import { api } from '$lib/api/client';
	import { currentUser } from '$lib/stores/auth';

	interface ActivitySession {
		id: string;
		activity_id: string;
		activity_name: string;
		activity_url: string;
		activity_icon?: string;
		channel_id: string;
		guild_id?: string;
		host_user_id: string;
		state: Record<string, unknown>;
		config: Record<string, unknown>;
		status: string;
		started_at: string;
	}

	interface Participant {
		user_id: string;
		role: string;
		joined_at: string;
		username: string;
		display_name?: string;
		avatar_id?: string;
	}

	interface Activity {
		id: string;
		name: string;
		description?: string;
		activity_type: string;
		icon_url?: string;
		url: string;
		max_participants: number;
		min_participants: number;
		category: string;
		verified: boolean;
		rating_avg: number;
		rating_count: number;
	}

	interface Props {
		channelId: string;
		onclose?: () => void;
	}

	let { channelId, onclose }: Props = $props();

	let session = $state<ActivitySession | null>(null);
	let participants = $state<Participant[]>([]);
	let activities = $state<Activity[]>([]);
	let loading = $state(false);
	let error = $state('');
	let showBrowser = $state(false);
	let selectedCategory = $state('all');
	let joining = $state(false);
	let iframeEl = $state<HTMLIFrameElement | null>(null);

	const categories = [
		{ id: 'all', label: 'All' },
		{ id: 'entertainment', label: 'Entertainment' },
		{ id: 'games', label: 'Games' },
		{ id: 'productivity', label: 'Productivity' },
		{ id: 'other', label: 'Other' }
	];

	const isHost = $derived(session?.host_user_id === $currentUser?.id);
	const isParticipant = $derived(participants.some((p) => p.user_id === $currentUser?.id));

	async function loadActiveSession() {
		loading = true;
		error = '';
		try {
			const data = await api.getActiveSession<{ session: ActivitySession; participants: Participant[] }>(channelId);
			if (data && data.session) {
				session = data.session;
				participants = data.participants ?? [];
			} else {
				session = null;
				participants = [];
				showBrowser = true;
			}
		} catch {
			showBrowser = true;
		} finally {
			loading = false;
		}
	}

	async function browseActivities() {
		loading = true;
		try {
			const category = selectedCategory === 'all' ? undefined : selectedCategory;
			const data = await api.listActivities<Activity[]>(category);
			activities = data ?? [];
		} catch (err: any) {
			error = err.message || 'Failed to load activities';
		} finally {
			loading = false;
		}
	}

	async function startActivity(activityId: string) {
		joining = true;
		error = '';
		try {
			const result = await api.createActivitySession<ActivitySession>(channelId, { activity_id: activityId, config: {} });
			if (result) {
				session = result;
				showBrowser = false;
				await loadActiveSession();
			}
		} catch (err: any) {
			error = err.message || 'Failed to start activity';
		} finally {
			joining = false;
		}
	}

	async function joinSession() {
		if (!session) return;
		joining = true;
		try {
			await api.joinActivitySession(channelId, session.id);
			await loadActiveSession();
		} catch (err: any) {
			error = err.message || 'Failed to join';
		} finally {
			joining = false;
		}
	}

	async function leaveSession() {
		if (!session) return;
		try {
			await api.leaveActivitySession(channelId, session.id);
			await loadActiveSession();
		} catch {
			// Ignore.
		}
	}

	async function endSession() {
		if (!session) return;
		try {
			await api.endActivitySession(channelId, session.id);
			session = null;
			participants = [];
			showBrowser = true;
		} catch (err: any) {
			error = err.message || 'Failed to end session';
		}
	}

	// Build the iframe URL with context parameters.
	function buildActivityUrl(baseUrl: string): string {
		if (baseUrl.startsWith('builtin://')) return '';
		const url = new URL(baseUrl);
		url.searchParams.set('session_id', session?.id ?? '');
		url.searchParams.set('channel_id', channelId);
		url.searchParams.set('user_id', $currentUser?.id ?? '');
		return url.toString();
	}

	function renderStars(rating: number): string {
		const full = Math.floor(rating);
		const half = rating - full >= 0.5;
		let stars = '';
		for (let i = 0; i < full; i++) stars += '★';
		if (half) stars += '½';
		for (let i = stars.length; i < 5; i++) stars += '☆';
		return stars;
	}

	$effect(() => {
		if (channelId) {
			loadActiveSession();
		}
	});

	$effect(() => {
		if (showBrowser) {
			browseActivities();
		}
	});
</script>

<div class="flex flex-col h-full bg-bg-primary">
	{#if loading && !session && !showBrowser}
		<div class="flex items-center justify-center h-64 text-text-muted">Loading activity...</div>
	{:else if session && !showBrowser}
		<!-- Active session -->
		<div class="flex items-center justify-between px-4 py-2 bg-bg-secondary border-b border-border-primary">
			<div class="flex items-center gap-2">
				{#if session.activity_icon}
					<img src={session.activity_icon} alt="" class="w-6 h-6 rounded" />
				{:else}
					<div class="w-6 h-6 rounded bg-brand-500/20 flex items-center justify-center">
						<svg class="w-4 h-4 text-brand-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
							<path stroke-linecap="round" stroke-linejoin="round" d="M14.752 11.168l-3.197-2.132A1 1 0 0010 9.87v4.263a1 1 0 001.555.832l3.197-2.132a1 1 0 000-1.664z" />
							<path stroke-linecap="round" stroke-linejoin="round" d="M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
						</svg>
					</div>
				{/if}
				<div>
					<span class="text-text-primary text-sm font-medium">{session.activity_name}</span>
					<span class="text-text-muted text-xs ml-2">{participants.length} participant{participants.length !== 1 ? 's' : ''}</span>
				</div>
			</div>

			<div class="flex items-center gap-2">
				<!-- Participant avatars -->
				<div class="flex -space-x-2">
					{#each participants.slice(0, 5) as p}
						<div
							class="w-6 h-6 rounded-full bg-bg-tertiary border border-bg-secondary flex items-center justify-center text-xs text-text-primary"
							title={p.display_name ?? p.username}
						>
							{(p.display_name ?? p.username).charAt(0).toUpperCase()}
						</div>
					{/each}
					{#if participants.length > 5}
						<div class="w-6 h-6 rounded-full bg-bg-tertiary border border-bg-secondary flex items-center justify-center text-xs text-text-muted">
							+{participants.length - 5}
						</div>
					{/if}
				</div>

				{#if !isParticipant}
					<button type="button" class="btn-primary text-xs px-3 py-1 rounded" disabled={joining} onclick={joinSession}>
						{joining ? 'Joining...' : 'Join'}
					</button>
				{:else}
					<button type="button" class="btn-secondary text-xs px-3 py-1 rounded" onclick={leaveSession}>Leave</button>
				{/if}

				{#if isHost}
					<button type="button" class="text-xs px-3 py-1 rounded text-red-400 hover:bg-red-500/10" onclick={endSession}>End</button>
				{/if}

				{#if onclose}
					<button type="button" class="text-text-muted hover:text-text-primary p-1" aria-label="Close activity" onclick={onclose}>
						<svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
							<path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
						</svg>
					</button>
				{/if}
			</div>
		</div>

		<!-- Activity iframe -->
		<div class="flex-1 relative">
			{#if session.activity_url && !session.activity_url.startsWith('builtin://')}
				<iframe
					bind:this={iframeEl}
					src={buildActivityUrl(session.activity_url)}
					title={session.activity_name}
					class="w-full h-full border-none"
					sandbox="allow-scripts allow-same-origin allow-popups allow-forms"
					allow="camera; microphone; fullscreen"
				></iframe>
			{:else}
				<!-- Built-in activity placeholder -->
				<div class="flex flex-col items-center justify-center h-full text-text-muted">
					<svg class="w-16 h-16 mb-4 text-brand-400/30" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1">
						<path stroke-linecap="round" stroke-linejoin="round" d="M14.752 11.168l-3.197-2.132A1 1 0 0010 9.87v4.263a1 1 0 001.555.832l3.197-2.132a1 1 0 000-1.664z" />
						<path stroke-linecap="round" stroke-linejoin="round" d="M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
					</svg>
					<p class="text-lg font-medium text-text-primary">{session.activity_name}</p>
					<p class="text-sm mt-1">Built-in activity running</p>
					<p class="text-xs mt-2">{participants.length} participants connected</p>
				</div>
			{/if}
		</div>

		{#if error}
			<div class="px-4 py-2 bg-red-500/10 text-red-400 text-sm">{error}</div>
		{/if}
	{:else}
		<!-- Activity browser -->
		<div class="flex items-center justify-between px-4 py-3 border-b border-border-primary">
			<h2 class="text-text-primary font-medium">Activities</h2>
			{#if onclose}
				<button type="button" class="text-text-muted hover:text-text-primary" aria-label="Close activity browser" onclick={onclose}>
					<svg class="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
						<path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
					</svg>
				</button>
			{/if}
		</div>

		<!-- Category tabs -->
		<div class="flex items-center gap-1 px-4 py-2 border-b border-border-primary overflow-x-auto">
			{#each categories as cat}
				<button
					type="button"
					class="text-sm px-3 py-1 rounded-full whitespace-nowrap transition-colors
						{selectedCategory === cat.id ? 'bg-brand-500 text-white' : 'text-text-muted hover:text-text-primary hover:bg-bg-tertiary'}"
					onclick={() => (selectedCategory = cat.id)}
				>
					{cat.label}
				</button>
			{/each}
		</div>

		{#if error}
			<div class="mx-4 mt-2 p-2 bg-red-500/10 border border-red-500/20 rounded text-red-400 text-sm">{error}</div>
		{/if}

		<!-- Activity list -->
		<div class="flex-1 overflow-y-auto p-4">
			{#if loading}
				<div class="text-text-muted text-sm">Loading activities...</div>
			{:else if activities.length === 0}
				<div class="text-center text-text-muted py-8">
					<p class="text-sm">No activities available yet.</p>
					<p class="text-xs mt-1">Create a custom activity with the Activity SDK!</p>
				</div>
			{:else}
				<div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
					{#each activities as activity (activity.id)}
						<div class="bg-bg-secondary border border-border-primary rounded-lg p-4 hover:border-brand-500/30 transition-colors">
							<div class="flex items-start gap-3">
								{#if activity.icon_url}
									<img src={activity.icon_url} alt="" class="w-12 h-12 rounded-lg shrink-0" />
								{:else}
									<div class="w-12 h-12 rounded-lg bg-brand-500/10 flex items-center justify-center shrink-0">
										<span class="text-brand-400 text-lg">
											{activity.category === 'games' ? '🎮' : activity.category === 'entertainment' ? '🎬' : '🧩'}
										</span>
									</div>
								{/if}
								<div class="min-w-0 flex-1">
									<div class="flex items-center gap-2">
										<h3 class="text-text-primary text-sm font-medium truncate">{activity.name}</h3>
										{#if activity.verified}
											<svg class="w-4 h-4 text-brand-400 shrink-0" fill="currentColor" viewBox="0 0 24 24">
												<path d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
											</svg>
										{/if}
									</div>
									{#if activity.description}
										<p class="text-text-muted text-xs mt-0.5 line-clamp-2">{activity.description}</p>
									{/if}
									<div class="flex items-center gap-3 mt-2 text-xs text-text-muted">
										{#if activity.rating_count > 0}
											<span class="text-yellow-400">{renderStars(activity.rating_avg)}</span>
											<span>({activity.rating_count})</span>
										{/if}
										<span>{activity.min_participants}-{activity.max_participants || '∞'} players</span>
									</div>
								</div>
							</div>
							<button
								type="button"
								class="btn-primary w-full text-sm py-1.5 rounded mt-3"
								disabled={joining}
								onclick={() => startActivity(activity.id)}
							>
								{joining ? 'Starting...' : 'Start Activity'}
							</button>
						</div>
					{/each}
				</div>
			{/if}
		</div>
	{/if}
</div>
