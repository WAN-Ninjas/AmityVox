<script lang="ts">
	import { currentGuild } from '$lib/stores/guilds';
	import { textChannels } from '$lib/stores/channels';
	import { presenceMap } from '$lib/stores/presence';
	import { page } from '$app/stores';
	import { api } from '$lib/api/client';
	import type { GuildEvent, OnboardingConfig } from '$lib/types';
	import OnboardingModal from '$lib/components/guild/OnboardingModal.svelte';

	let events = $state<GuildEvent[]>([]);
	let eventsLoading = $state(true);
	let eventsError = $state(false);
	let members = $state<{ total: number; online: number }>({ total: 0, online: 0 });

	// --- Onboarding ---
	let showOnboarding = $state(false);
	let onboardingConfig = $state<OnboardingConfig | null>(null);

	// --- Server Guide ---
	interface GuideStep {
		id: string;
		guild_id: string;
		title: string;
		content: string;
		position: number;
		channel_id: string | null;
		created_at: string;
	}

	let guideSteps = $state<GuideStep[]>([]);
	let guideLoading = $state(true);
	let guideCurrentStep = $state(0);
	let guideDismissed = $state(false);

	// --- Bump System ---
	interface BumpStatus {
		can_bump: boolean;
		next_bump_at: string | null;
		last_bump: string | null;
		bump_count_24h: number;
	}

	let bumpStatus = $state<BumpStatus | null>(null);
	let bumpLoading = $state(false);
	let bumpMessage = $state<string | null>(null);
	let bumpCooldownText = $state('');
	let bumpCooldownInterval: ReturnType<typeof setInterval> | null = null;

	// Check onboarding status whenever guildId changes.
	$effect(() => {
		const guildId = $page.params.guildId;
		if (!guildId) return;

		showOnboarding = false;
		onboardingConfig = null;

		api.getOnboardingStatus(guildId)
			.then((status) => {
				if (!status.completed) {
					return api.getOnboarding(guildId);
				}
				return null;
			})
			.then((config) => {
				if (config && config.enabled) {
					onboardingConfig = config;
					showOnboarding = true;
				}
			})
			.catch(() => {
				// Onboarding not available or error; silently ignore.
			});
	});

	function handleOnboardingComplete() {
		showOnboarding = false;
		onboardingConfig = null;
	}

	// Load guild events whenever guildId changes.
	$effect(() => {
		const guildId = $page.params.guildId;
		if (!guildId) return;

		eventsLoading = true;
		eventsError = false;

		api
			.getGuildEvents(guildId, { status: 'scheduled', limit: 5 })
			.then((data) => {
				events = data;
			})
			.catch(() => {
				eventsError = true;
				events = [];
			})
			.finally(() => {
				eventsLoading = false;
			});
	});

	// Load server guide whenever guildId changes.
	$effect(() => {
		const guildId = $page.params.guildId;
		if (!guildId) return;

		guideLoading = true;
		guideCurrentStep = 0;
		guideDismissed = false;

		// Check localStorage to see if the user already dismissed this guild's guide.
		const dismissKey = `guide_dismissed_${guildId}`;
		if (typeof window !== 'undefined' && localStorage.getItem(dismissKey) === 'true') {
			guideDismissed = true;
		}

		fetch(`/api/v1/guilds/${guildId}/guide`, {
			headers: {
				Authorization: `Bearer ${getToken()}`
			}
		})
			.then((res) => {
				if (!res.ok) throw new Error('Failed to load guide');
				return res.json();
			})
			.then((json) => {
				guideSteps = json.data ?? [];
			})
			.catch(() => {
				guideSteps = [];
			})
			.finally(() => {
				guideLoading = false;
			});
	});

	// Load bump status whenever guildId changes.
	$effect(() => {
		const guildId = $page.params.guildId;
		const guild = $currentGuild;
		if (!guildId || !guild?.discoverable) {
			bumpStatus = null;
			return;
		}

		fetch(`/api/v1/guilds/${guildId}/bump`, {
			headers: {
				Authorization: `Bearer ${getToken()}`
			}
		})
			.then((res) => {
				if (!res.ok) throw new Error('Failed to load bump status');
				return res.json();
			})
			.then((json) => {
				bumpStatus = json.data;
				updateBumpCooldown();
			})
			.catch(() => {
				bumpStatus = null;
			});
	});

	// Update bump cooldown timer.
	$effect(() => {
		if (bumpCooldownInterval) {
			clearInterval(bumpCooldownInterval);
			bumpCooldownInterval = null;
		}

		if (bumpStatus && !bumpStatus.can_bump && bumpStatus.next_bump_at) {
			updateBumpCooldown();
			bumpCooldownInterval = setInterval(updateBumpCooldown, 1000);
		}

		return () => {
			if (bumpCooldownInterval) {
				clearInterval(bumpCooldownInterval);
				bumpCooldownInterval = null;
			}
		};
	});

	function updateBumpCooldown() {
		if (!bumpStatus?.next_bump_at) {
			bumpCooldownText = '';
			return;
		}

		const next = new Date(bumpStatus.next_bump_at).getTime();
		const now = Date.now();
		const diff = next - now;

		if (diff <= 0) {
			bumpStatus = { ...bumpStatus, can_bump: true, next_bump_at: null };
			bumpCooldownText = '';
			if (bumpCooldownInterval) {
				clearInterval(bumpCooldownInterval);
				bumpCooldownInterval = null;
			}
			return;
		}

		const hours = Math.floor(diff / (1000 * 60 * 60));
		const minutes = Math.floor((diff % (1000 * 60 * 60)) / (1000 * 60));
		const seconds = Math.floor((diff % (1000 * 60)) / 1000);

		if (hours > 0) {
			bumpCooldownText = `${hours}h ${minutes}m`;
		} else if (minutes > 0) {
			bumpCooldownText = `${minutes}m ${seconds}s`;
		} else {
			bumpCooldownText = `${seconds}s`;
		}
	}

	function getToken(): string {
		if (typeof document === 'undefined') return '';
		const match = document.cookie.match(/(?:^|;\s*)token=([^;]*)/);
		if (match) return decodeURIComponent(match[1]);
		return localStorage.getItem('token') ?? '';
	}

	async function handleBump() {
		const guildId = $page.params.guildId;
		if (!guildId || bumpLoading) return;

		bumpLoading = true;
		bumpMessage = null;

		try {
			const res = await fetch(`/api/v1/guilds/${guildId}/bump`, {
				method: 'POST',
				headers: {
					'Content-Type': 'application/json',
					Authorization: `Bearer ${getToken()}`
				}
			});

			const json = await res.json();

			if (res.ok) {
				const data = json.data;
				bumpMessage = data.bump_message;
				bumpStatus = {
					can_bump: false,
					next_bump_at: data.next_bump_at,
					last_bump: new Date().toISOString(),
					bump_count_24h: (bumpStatus?.bump_count_24h ?? 0) + 1
				};
			} else {
				bumpMessage = json.error?.message ?? 'Failed to bump guild';
			}
		} catch {
			bumpMessage = 'Failed to bump guild';
		} finally {
			bumpLoading = false;
			// Clear the message after a few seconds.
			setTimeout(() => {
				bumpMessage = null;
			}, 5000);
		}
	}

	function dismissGuide() {
		const guildId = $page.params.guildId;
		guideDismissed = true;
		if (typeof window !== 'undefined' && guildId) {
			localStorage.setItem(`guide_dismissed_${guildId}`, 'true');
		}
	}

	function guideNext() {
		if (guideCurrentStep < guideSteps.length - 1) {
			guideCurrentStep++;
		}
	}

	function guidePrev() {
		if (guideCurrentStep > 0) {
			guideCurrentStep--;
		}
	}

	// Derive member counts from guild data and presence map.
	$effect(() => {
		const guild = $currentGuild;
		const presences = $presenceMap;

		if (guild) {
			let onlineCount = 0;
			for (const status of presences.values()) {
				if (status === 'online' || status === 'idle' || status === 'dnd') {
					onlineCount++;
				}
			}
			members = { total: guild.member_count, online: onlineCount };
		}
	});

	const featuredChannels = $derived($textChannels.slice(0, 5));

	const createdDate = $derived(
		$currentGuild?.created_at
			? new Date($currentGuild.created_at).toLocaleDateString('en-US', {
					year: 'numeric',
					month: 'long',
					day: 'numeric'
				})
			: null
	);

	function formatEventDate(dateStr: string): string {
		const date = new Date(dateStr);
		const now = new Date();
		const diffMs = date.getTime() - now.getTime();
		const diffDays = Math.ceil(diffMs / (1000 * 60 * 60 * 24));

		const timeStr = date.toLocaleTimeString('en-US', {
			hour: 'numeric',
			minute: '2-digit'
		});

		if (diffDays === 0) {
			return `Today at ${timeStr}`;
		} else if (diffDays === 1) {
			return `Tomorrow at ${timeStr}`;
		} else if (diffDays > 1 && diffDays <= 7) {
			const dayName = date.toLocaleDateString('en-US', { weekday: 'long' });
			return `${dayName} at ${timeStr}`;
		} else {
			return date.toLocaleDateString('en-US', {
				month: 'short',
				day: 'numeric',
				year: 'numeric',
				hour: 'numeric',
				minute: '2-digit'
			});
		}
	}
</script>

<svelte:head>
	<title>{$currentGuild?.name ?? 'Guild'} — AmityVox</title>
</svelte:head>

<div class="flex h-full flex-col overflow-y-auto bg-bg-tertiary">
	{#if $currentGuild}
		<!-- Guild Hero Section -->
		<div class="relative">
			{#if $currentGuild.banner_id}
				<div class="h-40 w-full overflow-hidden">
					<img
						src="/api/v1/files/{$currentGuild.banner_id}"
						alt="{$currentGuild.name} banner"
						class="h-full w-full object-cover"
					/>
					<div class="absolute inset-0 bg-gradient-to-b from-transparent to-bg-tertiary"></div>
				</div>
			{:else}
				<div class="h-32 w-full bg-gradient-to-br from-brand-600/30 via-brand-500/20 to-bg-tertiary"></div>
			{/if}
		</div>

		<div class="mx-auto w-full max-w-3xl px-6 pb-8">
			<!-- Guild Identity -->
			<div class="relative -mt-12 mb-6 flex items-end gap-5">
				<div class="shrink-0 rounded-2xl bg-bg-secondary p-1.5 shadow-lg">
					{#if $currentGuild.icon_id}
						<img
							src="/api/v1/files/{$currentGuild.icon_id}"
							alt={$currentGuild.name}
							class="h-20 w-20 rounded-xl object-cover"
						/>
					{:else}
						<div class="flex h-20 w-20 items-center justify-center rounded-xl bg-brand-600 text-2xl font-bold text-white">
							{$currentGuild.name
								.split(' ')
								.map((w) => w[0])
								.join('')
								.slice(0, 2)
								.toUpperCase()}
						</div>
					{/if}
				</div>
				<div class="mb-1 min-w-0 flex-1">
					<h1 class="truncate text-2xl font-bold text-text-primary">{$currentGuild.name}</h1>
					{#if $currentGuild.description}
						<p class="mt-1 text-sm text-text-muted">{$currentGuild.description}</p>
					{/if}
				</div>
			</div>

			<!-- Stats Bar -->
			<div class="mb-6 flex flex-wrap items-center gap-6 rounded-lg bg-bg-secondary px-5 py-3">
				<div class="flex items-center gap-2">
					<svg class="h-4 w-4 text-text-muted" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
						<path d="M17 21v-2a4 4 0 00-4-4H5a4 4 0 00-4 4v2" />
						<circle cx="9" cy="7" r="4" />
						<path d="M23 21v-2a4 4 0 00-3-3.87" />
						<path d="M16 3.13a4 4 0 010 7.75" />
					</svg>
					<span class="text-sm font-medium text-text-primary">{members.total.toLocaleString()}</span>
					<span class="text-sm text-text-muted">Members</span>
				</div>
				<div class="flex items-center gap-2">
					<span class="h-2.5 w-2.5 rounded-full bg-status-online"></span>
					<span class="text-sm font-medium text-text-primary">{members.online.toLocaleString()}</span>
					<span class="text-sm text-text-muted">Online</span>
				</div>
				{#if createdDate}
					<div class="flex items-center gap-2">
						<svg class="h-4 w-4 text-text-muted" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
							<rect x="3" y="4" width="18" height="18" rx="2" ry="2" />
							<line x1="16" y1="2" x2="16" y2="6" />
							<line x1="8" y1="2" x2="8" y2="6" />
							<line x1="3" y1="10" x2="21" y2="10" />
						</svg>
						<span class="text-sm text-text-muted">Created {createdDate}</span>
					</div>
				{/if}

				<!-- Bump Button (only for discoverable guilds) -->
				{#if $currentGuild.discoverable && bumpStatus}
					<div class="ml-auto flex items-center gap-2">
						{#if bumpStatus.bump_count_24h > 0}
							<span class="text-xs text-text-muted">
								{bumpStatus.bump_count_24h} bump{bumpStatus.bump_count_24h !== 1 ? 's' : ''} today
							</span>
						{/if}
						<button
							onclick={handleBump}
							disabled={!bumpStatus.can_bump || bumpLoading}
							class="flex items-center gap-1.5 rounded-md px-3 py-1.5 text-sm font-medium transition-colors
								{bumpStatus.can_bump
									? 'bg-brand-600 text-white hover:bg-brand-700'
									: 'cursor-not-allowed bg-bg-primary text-text-muted'}"
						>
							{#if bumpLoading}
								<svg class="h-3.5 w-3.5 animate-spin" fill="none" viewBox="0 0 24 24">
									<circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
									<path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
								</svg>
							{:else}
								<svg class="h-3.5 w-3.5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
									<path d="M12 19V5M5 12l7-7 7 7" />
								</svg>
							{/if}
							{#if bumpStatus.can_bump}
								Bump
							{:else}
								{bumpCooldownText}
							{/if}
						</button>
					</div>
				{/if}
			</div>

			<!-- Bump Feedback Message -->
			{#if bumpMessage}
				<div class="mb-4 rounded-lg border border-brand-600/30 bg-brand-600/10 px-4 py-2.5 text-sm text-brand-400">
					{bumpMessage}
				</div>
			{/if}

			<!-- Server Guide Walkthrough -->
			{#if guideSteps.length > 0 && !guideDismissed && !guideLoading}
				{@const step = guideSteps[guideCurrentStep]}
				<div class="mb-6 rounded-lg border border-brand-600/20 bg-bg-secondary p-5">
					<div class="mb-3 flex items-center justify-between">
						<h2 class="flex items-center gap-2 text-sm font-semibold uppercase tracking-wide text-text-muted">
							<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
								<path d="M12 6.253v13m0-13C10.832 5.477 9.246 5 7.5 5S4.168 5.477 3 6.253v13C4.168 18.477 5.754 18 7.5 18s3.332.477 4.5 1.253m0-13C13.168 5.477 14.754 5 16.5 5c1.747 0 3.332.477 4.5 1.253v13C19.832 18.477 18.247 18 16.5 18c-1.746 0-3.332.477-4.5 1.253" />
							</svg>
							Server Guide
						</h2>
						<button
							onclick={dismissGuide}
							class="rounded p-1 text-text-muted transition-colors hover:bg-bg-primary hover:text-text-primary"
							title="Dismiss guide"
						>
							<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
								<path d="M6 18L18 6M6 6l12 12" />
							</svg>
						</button>
					</div>

					<!-- Step indicator -->
					<div class="mb-4 flex items-center gap-1">
						{#each guideSteps as _, i}
							<button
								onclick={() => (guideCurrentStep = i)}
								class="h-1.5 flex-1 rounded-full transition-colors {i === guideCurrentStep
									? 'bg-brand-500'
									: i < guideCurrentStep
										? 'bg-brand-500/40'
										: 'bg-bg-primary'}"
							></button>
						{/each}
					</div>

					<!-- Step content -->
					{#if step}
						<div class="mb-4">
							<h3 class="mb-2 text-lg font-semibold text-text-primary">{step.title}</h3>
							<div class="prose prose-sm text-text-secondary">
								<p class="whitespace-pre-wrap text-sm leading-relaxed text-text-muted">{step.content}</p>
							</div>
							{#if step.channel_id}
								<a
									href="/app/guilds/{$page.params.guildId}/channels/{step.channel_id}"
									class="mt-3 inline-flex items-center gap-1.5 rounded-md bg-brand-600/10 px-3 py-1.5 text-sm font-medium text-brand-400 transition-colors hover:bg-brand-600/20"
								>
									<span>#</span>
									Go to channel
								</a>
							{/if}
						</div>
					{/if}

					<!-- Navigation -->
					<div class="flex items-center justify-between">
						<span class="text-xs text-text-muted">
							Step {guideCurrentStep + 1} of {guideSteps.length}
						</span>
						<div class="flex items-center gap-2">
							{#if guideCurrentStep > 0}
								<button
									onclick={guidePrev}
									class="rounded-md border border-bg-primary px-3 py-1.5 text-sm font-medium text-text-primary transition-colors hover:bg-bg-primary"
								>
									Previous
								</button>
							{/if}
							{#if guideCurrentStep < guideSteps.length - 1}
								<button
									onclick={guideNext}
									class="rounded-md bg-brand-600 px-3 py-1.5 text-sm font-medium text-white transition-colors hover:bg-brand-700"
								>
									Next
								</button>
							{:else}
								<button
									onclick={dismissGuide}
									class="rounded-md bg-brand-600 px-3 py-1.5 text-sm font-medium text-white transition-colors hover:bg-brand-700"
								>
									Done
								</button>
							{/if}
						</div>
					</div>
				</div>
			{/if}

			<div class="grid gap-6 md:grid-cols-2">
				<!-- Featured Channels -->
				<div class="rounded-lg bg-bg-secondary p-5">
					<h2 class="mb-3 flex items-center gap-2 text-sm font-semibold uppercase tracking-wide text-text-muted">
						<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
							<path d="M21 15a2 2 0 01-2 2H7l-4 4V5a2 2 0 012-2h14a2 2 0 012 2z" />
						</svg>
						Text Channels
					</h2>
					{#if featuredChannels.length > 0}
						<ul class="space-y-1">
							{#each featuredChannels as channel (channel.id)}
								<li>
									<a
										href="/app/guilds/{$page.params.guildId}/channels/{channel.id}"
										class="group flex items-center gap-2.5 rounded-md px-3 py-2 transition-colors hover:bg-bg-primary/50"
									>
										<span class="text-text-muted group-hover:text-text-primary">#</span>
										<span class="truncate text-sm font-medium text-text-primary">{channel.name}</span>
										{#if channel.topic}
											<span class="ml-auto hidden truncate text-xs text-text-muted md:inline">{channel.topic}</span>
										{/if}
									</a>
								</li>
							{/each}
						</ul>
					{:else}
						<p class="py-4 text-center text-sm text-text-muted">No text channels yet</p>
					{/if}
				</div>

				<!-- Scheduled Events -->
				<div class="rounded-lg bg-bg-secondary p-5">
					<h2 class="mb-3 flex items-center gap-2 text-sm font-semibold uppercase tracking-wide text-text-muted">
						<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
							<rect x="3" y="4" width="18" height="18" rx="2" ry="2" />
							<line x1="16" y1="2" x2="16" y2="6" />
							<line x1="8" y1="2" x2="8" y2="6" />
							<line x1="3" y1="10" x2="21" y2="10" />
						</svg>
						Upcoming Events
					</h2>
					{#if eventsLoading}
						<div class="flex items-center justify-center py-6">
							<svg class="h-5 w-5 animate-spin text-text-muted" fill="none" viewBox="0 0 24 24">
								<circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
								<path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
							</svg>
						</div>
					{:else if events.length > 0}
						<ul class="space-y-2">
							{#each events as event (event.id)}
								<li class="rounded-md border border-bg-primary/50 bg-bg-primary/30 px-3 py-2.5">
									<div class="flex items-start justify-between gap-2">
										<div class="min-w-0">
											<p class="truncate text-sm font-medium text-text-primary">{event.name}</p>
											<p class="mt-0.5 text-xs text-text-muted">
												{formatEventDate(event.scheduled_start)}
											</p>
											{#if event.description}
												<p class="mt-1 line-clamp-2 text-xs text-text-muted">{event.description}</p>
											{/if}
										</div>
										{#if event.interested_count > 0}
											<span class="shrink-0 rounded-full bg-brand-600/20 px-2 py-0.5 text-xs font-medium text-brand-400">
												{event.interested_count} interested
											</span>
										{/if}
									</div>
									{#if event.location}
										<div class="mt-1.5 flex items-center gap-1 text-xs text-text-muted">
											<svg class="h-3 w-3" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
												<path d="M21 10c0 7-9 13-9 13s-9-6-9-13a9 9 0 0118 0z" />
												<circle cx="12" cy="10" r="3" />
											</svg>
											{event.location}
										</div>
									{/if}
								</li>
							{/each}
						</ul>
					{:else if eventsError}
						<p class="py-4 text-center text-sm text-text-muted">Could not load events</p>
					{:else}
						<div class="flex flex-col items-center py-6 text-center">
							<svg class="mb-2 h-8 w-8 text-text-muted/50" fill="none" stroke="currentColor" stroke-width="1.5" viewBox="0 0 24 24">
								<rect x="3" y="4" width="18" height="18" rx="2" ry="2" />
								<line x1="16" y1="2" x2="16" y2="6" />
								<line x1="8" y1="2" x2="8" y2="6" />
								<line x1="3" y1="10" x2="21" y2="10" />
							</svg>
							<p class="text-sm text-text-muted">No upcoming events</p>
						</div>
					{/if}
				</div>
			</div>

			<!-- Welcome Tip -->
			{#if featuredChannels.length > 0}
				<div class="mt-6 rounded-lg border border-brand-600/20 bg-brand-600/5 px-5 py-4 text-center">
					<p class="text-sm text-text-muted">
						Pick a channel to start chatting, or explore the server at your own pace.
					</p>
				</div>
			{/if}
		</div>
	{:else}
		<div class="flex h-full items-center justify-center">
			<div class="flex items-center gap-3">
				<svg class="h-5 w-5 animate-spin text-text-muted" fill="none" viewBox="0 0 24 24">
					<circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
					<path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
				</svg>
				<p class="text-text-muted">Loading guild...</p>
			</div>
		</div>
	{/if}
</div>

{#if onboardingConfig && $currentGuild}
	<OnboardingModal
		bind:open={showOnboarding}
		guildId={$page.params.guildId}
		guildName={$currentGuild.name}
		onboarding={onboardingConfig}
		onComplete={handleOnboardingComplete}
	/>
{/if}
