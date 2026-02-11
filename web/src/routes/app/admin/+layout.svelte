<script lang="ts">
	import type { Snippet } from 'svelte';

	interface Props {
		children: Snippet;
	}

	let { children }: Props = $props();
</script>

<div class="flex h-full">
	<!-- Admin sidebar -->
	<nav class="w-52 shrink-0 overflow-y-auto bg-bg-secondary p-4">
		<h3 class="mb-2 text-xs font-bold uppercase tracking-wide text-text-muted">Admin</h3>
		<ul class="space-y-0.5">
			<li>
				<a
					href="/app/admin"
					class="block w-full rounded px-2 py-1.5 text-left text-sm text-text-primary bg-bg-modifier"
				>
					Dashboard
				</a>
			</li>
			<li>
				<button class="w-full rounded px-2 py-1.5 text-left text-sm text-text-muted hover:bg-bg-modifier hover:text-text-secondary">
					Instance Settings
				</button>
			</li>
			<li>
				<button class="w-full rounded px-2 py-1.5 text-left text-sm text-text-muted hover:bg-bg-modifier hover:text-text-secondary">
					Federation
				</button>
			</li>
			<li>
				<button class="w-full rounded px-2 py-1.5 text-left text-sm text-text-muted hover:bg-bg-modifier hover:text-text-secondary">
					Users
				</button>
			</li>
		</ul>
	</nav>

	<div class="flex-1 overflow-y-auto bg-bg-tertiary">
		{@render children()}
	</div>
</div>
