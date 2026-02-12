<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api/client';
	import type { Announcement } from '$lib/types';

	let announcements = $state<Announcement[]>([]);
	let dismissed = $state<Set<string>>(new Set());
	let loading = $state(true);

	onMount(async () => {
		try {
			const token = api.getToken();
			if (token) {
				announcements = await api.getActiveAnnouncements();
			}
		} catch {
			// Silently fail — announcements are non-critical
		} finally {
			loading = false;
		}
	});

	function dismiss(id: string) {
		dismissed = new Set([...dismissed, id]);
	}

	function severityBg(severity: string): string {
		switch (severity) {
			case 'info': return 'bg-blue-600';
			case 'warning': return 'bg-yellow-600';
			case 'critical': return 'bg-red-600';
			default: return 'bg-blue-600';
		}
	}

	function severityText(severity: string): string {
		switch (severity) {
			case 'info': return 'text-blue-50';
			case 'warning': return 'text-yellow-50';
			case 'critical': return 'text-red-50';
			default: return 'text-blue-50';
		}
	}

	function severityIcon(severity: string): string {
		switch (severity) {
			case 'info': return 'M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z';
			case 'warning': return 'M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-2.5L13.732 4.5c-.77-.833-2.694-.833-3.464 0L3.34 16.5c-.77.833.192 2.5 1.732 2.5z';
			case 'critical': return 'M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z';
			default: return 'M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z';
		}
	}

	const visibleAnnouncements = $derived(
		announcements.filter(a => !dismissed.has(a.id))
	);
</script>

{#if !loading && visibleAnnouncements.length > 0}
	<div class="flex flex-col">
		{#each visibleAnnouncements as announcement (announcement.id)}
			<div class="{severityBg(announcement.severity)} {severityText(announcement.severity)} relative px-4 py-2">
				<div class="mx-auto flex max-w-5xl items-center justify-between gap-3">
					<div class="flex items-center gap-2 min-w-0">
						<svg class="h-4 w-4 shrink-0" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" d={severityIcon(announcement.severity)} />
						</svg>
						<span class="text-sm font-semibold shrink-0">{announcement.title}</span>
						<span class="text-sm opacity-90 truncate">{announcement.content}</span>
					</div>
					<button
						class="shrink-0 rounded p-1 opacity-70 hover:opacity-100 transition-opacity"
						onclick={() => dismiss(announcement.id)}
						aria-label="Dismiss announcement"
					>
						<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
						</svg>
					</button>
				</div>
			</div>
		{/each}
	</div>
{/if}
