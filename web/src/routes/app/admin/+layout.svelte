<script lang="ts">
	import type { Snippet } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/stores';

	interface Props {
		children: Snippet;
	}

	let { children }: Props = $props();

	type Tab = 'dashboard' | 'settings' | 'federation' | 'users';
	let currentTab = $state<Tab>('dashboard');

	const tabs: { id: Tab; label: string }[] = [
		{ id: 'dashboard', label: 'Dashboard' },
		{ id: 'settings', label: 'Instance Settings' },
		{ id: 'federation', label: 'Federation' },
		{ id: 'users', label: 'Users' }
	];
</script>

<div class="flex h-full">
	<!-- Admin sidebar -->
	<nav class="w-52 shrink-0 overflow-y-auto bg-bg-secondary p-4">
		<h3 class="mb-2 text-xs font-bold uppercase tracking-wide text-text-muted">Admin</h3>
		<ul class="space-y-0.5">
			{#each tabs as tab (tab.id)}
				<li>
					<button
						class="w-full rounded px-2 py-1.5 text-left text-sm transition-colors"
						class:bg-bg-modifier={currentTab === tab.id}
						class:text-text-primary={currentTab === tab.id}
						class:text-text-muted={currentTab !== tab.id}
						class:hover:bg-bg-modifier={currentTab !== tab.id}
						class:hover:text-text-secondary={currentTab !== tab.id}
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
			onclick={() => goto('/app')}
		>
			Back to app
		</button>
	</nav>

	<div class="flex-1 overflow-y-auto bg-bg-tertiary">
		{@render children()}
	</div>
</div>
