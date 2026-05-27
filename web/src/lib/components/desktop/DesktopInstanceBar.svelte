<script lang="ts">
	import { onMount } from 'svelte';
	import {
		desktopInstances,
		initDesktopInstances,
		normalizeInstanceUrl,
		removeInstanceUrl,
		setActiveInstanceUrl
	} from '$lib/desktop/instances';

	let newInstanceUrl = $state('');
	let error = $state('');

	onMount(() => {
		initDesktopInstances();
	});

	function switchInstance(url: string) {
		error = '';
		setActiveInstanceUrl(url);
		location.assign('/app');
	}

	function addInstance() {
		error = '';
		try {
			const url = normalizeInstanceUrl(newInstanceUrl);
			setActiveInstanceUrl(url);
			newInstanceUrl = '';
			location.assign('/app');
		} catch (err) {
			error = err instanceof Error ? err.message : 'Invalid instance URL';
		}
	}

	function removeInstance(event: Event, url: string) {
		event.stopPropagation();
		error = '';
	removeInstanceUrl(url);
	location.assign('/app');
	}
</script>

{#if $desktopInstances.enabled}
	<div class="fixed inset-x-0 top-0 z-50 flex h-11 items-center gap-2 border-b border-border bg-bg-secondary px-2 shadow-lg">
		<div class="flex min-w-0 flex-1 items-center gap-1 overflow-x-auto">
			{#each $desktopInstances.instances as instance}
				<div
					class="flex h-8 max-w-56 shrink-0 items-center rounded text-sm transition"
					class:bg-brand-500={instance.url === $desktopInstances.activeUrl}
					class:text-white={instance.url === $desktopInstances.activeUrl}
					class:bg-bg-tertiary={instance.url !== $desktopInstances.activeUrl}
					class:text-text-secondary={instance.url !== $desktopInstances.activeUrl}
					title={instance.url}
				>
					<button
						type="button"
						class="min-w-0 flex-1 truncate px-3"
						onclick={() => switchInstance(instance.url)}
					>
						{instance.name}
					</button>
					{#if $desktopInstances.instances.length > 1}
						<button
							type="button"
							class="mr-1 rounded px-1 text-xs opacity-70 hover:bg-black/20 hover:opacity-100"
							aria-label={`Remove ${instance.name}`}
							onclick={(event) => removeInstance(event, instance.url)}
						>
							x
						</button>
					{/if}
				</div>
			{/each}
		</div>

		<form class="flex shrink-0 items-center gap-2" onsubmit={(event) => { event.preventDefault(); addInstance(); }}>
			<input
				class="h-8 w-64 rounded border border-border bg-bg-primary px-2 text-sm text-text-primary placeholder:text-text-muted"
				bind:value={newInstanceUrl}
				placeholder="https://instance.example"
				aria-label="Instance URL"
			/>
			<button type="submit" class="btn-secondary h-8 px-3 text-sm">Add</button>
		</form>

		{#if error}
			<div class="max-w-64 truncate text-xs text-red-400" title={error}>{error}</div>
		{/if}
	</div>
{/if}
