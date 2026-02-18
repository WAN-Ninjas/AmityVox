<script lang="ts">
	import { api } from '$lib/api/client';
	import { addToast } from '$lib/stores/toast';
	import { createAsyncOp } from '$lib/utils/asyncOp';

	interface Props {
		channelId: string;
	}

	let { channelId }: Props = $props();

	let joined = $state(false);
	let joinOp = $state(createAsyncOp());
	let leaveOp = $state(createAsyncOp());
	let voiceToken = $state<string | null>(null);
	let voiceUrl = $state<string | null>(null);

	// Placeholder speaker/audience lists. In production these would be populated
	// via WebSocket presence events from the voice server.
	let speakers = $state<{ id: string; name: string }[]>([]);
	let audience = $state<{ id: string; name: string }[]>([]);

	// Reset state when the channelId changes
	$effect(() => {
		const _id = channelId;
		joined = false;
		voiceToken = null;
		voiceUrl = null;
		speakers = [];
		audience = [];
	});

	async function joinStage() {
		if (joinOp.loading) return;
		const result = await joinOp.run(
			() => api.joinVoice(channelId),
			msg => addToast(msg, 'error')
		);
		if (!joinOp.error) {
			voiceToken = result!.token;
			voiceUrl = result!.url;
			joined = true;
			addToast('Joined the stage', 'success');
		}
	}

	async function leaveStage() {
		if (leaveOp.loading) return;
		await leaveOp.run(
			() => api.leaveVoice(channelId),
			msg => addToast(msg, 'error')
		);
		if (!leaveOp.error) {
			joined = false;
			voiceToken = null;
			voiceUrl = null;
			speakers = [];
			audience = [];
			addToast('Left the stage', 'info');
		}
	}

	function getInitials(name: string): string {
		return name
			.split(' ')
			.map((w) => w[0])
			.join('')
			.slice(0, 2)
			.toUpperCase();
	}
</script>

<div class="flex h-full flex-col bg-bg-primary">
	<!-- Header -->
	<div class="flex h-12 items-center gap-2 border-b border-bg-floating px-4">
		<svg class="h-5 w-5 text-text-muted" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
			<path d="M19 11a7 7 0 01-7 7m0 0a7 7 0 01-7-7m7 7v4m0 0H8m4 0h4m-4-8a3 3 0 01-3-3V5a3 3 0 116 0v6a3 3 0 01-3 3z" />
		</svg>
		<h2 class="text-sm font-semibold text-text-primary">Stage</h2>
		{#if joined}
			<span class="ml-auto flex items-center gap-1.5 text-xs text-status-online">
				<span class="inline-block h-2 w-2 rounded-full bg-status-online"></span>
				Connected
			</span>
		{/if}
	</div>

	<!-- Main stage area -->
	<div class="flex flex-1 flex-col overflow-y-auto">
		<!-- Stage visual -->
		<div class="flex flex-col items-center justify-center px-6 py-10">
			{#if !joined}
				<!-- Not joined state -->
				<div class="flex flex-col items-center">
					<div class="mb-4 flex h-20 w-20 items-center justify-center rounded-full bg-bg-modifier">
						<svg class="h-10 w-10 text-text-muted" fill="none" stroke="currentColor" stroke-width="1.5" viewBox="0 0 24 24">
							<path d="M19 11a7 7 0 01-7 7m0 0a7 7 0 01-7-7m7 7v4m0 0H8m4 0h4m-4-8a3 3 0 01-3-3V5a3 3 0 116 0v6a3 3 0 01-3 3z" />
						</svg>
					</div>
					<p class="mb-1 text-base font-semibold text-text-primary">Stage Channel</p>
					<p class="mb-6 text-sm text-text-muted">Join to listen or speak on this stage.</p>
					<button
						class="rounded-md bg-brand-500 px-6 py-2 text-sm font-medium text-white transition-colors hover:bg-brand-600 disabled:opacity-50"
						onclick={joinStage}
						disabled={joinOp.loading}
					>
						{#if joinOp.loading}
							<span class="flex items-center gap-2">
								<span class="inline-block h-4 w-4 animate-spin rounded-full border-2 border-white border-t-transparent"></span>
								Joining...
							</span>
						{:else}
							Join Stage
						{/if}
					</button>
				</div>
			{:else}
				<!-- Joined state -->
				<!-- Speaker area -->
				<div class="mb-8 w-full max-w-lg">
					<h3 class="mb-3 text-2xs font-bold uppercase tracking-wide text-text-muted">
						Speakers — {speakers.length}
					</h3>
					<div class="rounded-lg border border-bg-modifier bg-bg-secondary p-4">
						{#if speakers.length === 0}
							<p class="text-center text-sm text-text-muted">No one is speaking yet</p>
						{:else}
							<div class="flex flex-wrap gap-4">
								{#each speakers as speaker (speaker.id)}
									<div class="flex flex-col items-center gap-1">
										<div class="flex h-14 w-14 items-center justify-center rounded-full bg-brand-600 text-sm font-semibold text-white ring-2 ring-status-online">
											{getInitials(speaker.name)}
										</div>
										<span class="max-w-[70px] truncate text-xs text-text-secondary">{speaker.name}</span>
									</div>
								{/each}
							</div>
						{/if}
					</div>
				</div>

				<!-- Audience area -->
				<div class="w-full max-w-lg">
					<h3 class="mb-3 text-2xs font-bold uppercase tracking-wide text-text-muted">
						Audience — {audience.length}
					</h3>
					<div class="rounded-lg border border-bg-modifier bg-bg-secondary p-4">
						{#if audience.length === 0}
							<p class="text-center text-sm text-text-muted">No audience members</p>
						{:else}
							<div class="flex flex-wrap gap-3">
								{#each audience as member (member.id)}
									<div class="flex flex-col items-center gap-1">
										<div class="flex h-10 w-10 items-center justify-center rounded-full bg-bg-modifier text-xs font-medium text-text-secondary">
											{getInitials(member.name)}
										</div>
										<span class="max-w-[60px] truncate text-2xs text-text-muted">{member.name}</span>
									</div>
								{/each}
							</div>
						{/if}
					</div>
				</div>
			{/if}
		</div>
	</div>

	<!-- Bottom controls (shown when joined) -->
	{#if joined}
		<div class="border-t border-bg-floating bg-bg-secondary px-4 py-3">
			<div class="flex items-center justify-between">
				<div class="flex items-center gap-3">
					<span class="flex items-center gap-1.5 text-xs text-text-muted">
						<svg class="h-4 w-4 text-status-online" fill="currentColor" viewBox="0 0 24 24">
							<path d="M12 14c1.66 0 3-1.34 3-3V5c0-1.66-1.34-3-3-3S9 3.34 9 5v6c0 1.66 1.34 3 3 3z" />
							<path d="M17 11c0 2.76-2.24 5-5 5s-5-2.24-5-5H5c0 3.53 2.61 6.43 6 6.92V21h2v-3.08c3.39-.49 6-3.39 6-6.92h-2z" />
						</svg>
						Stage Connected
					</span>
					{#if voiceUrl}
						<span class="text-2xs text-text-muted">({voiceUrl})</span>
					{/if}
				</div>
				<button
					class="rounded-md bg-red-500 px-4 py-1.5 text-xs font-medium text-white transition-colors hover:bg-red-600 disabled:opacity-50"
					onclick={leaveStage}
					disabled={leaveOp.loading}
				>
					{#if leaveOp.loading}
						Leaving...
					{:else}
						Leave Stage
					{/if}
				</button>
			</div>
		</div>
	{/if}
</div>
