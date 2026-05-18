<script lang="ts">
	interface Props {
		passphrase: string;
		loading: boolean;
		onunlock: () => void;
	}

	let { passphrase = $bindable(''), loading, onunlock }: Props = $props();
</script>

<div class="mb-2 flex items-center gap-2 rounded-lg bg-yellow-500/10 border border-yellow-500/30 px-3 py-2">
	<svg class="h-4 w-4 shrink-0 text-yellow-400" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
		<path d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z" />
	</svg>
	<span class="text-xs text-yellow-400">Enter passphrase to send messages</span>
	<input
		type="password"
		class="ml-auto min-w-0 flex-1 max-w-48 rounded border border-bg-modifier bg-bg-primary px-2 py-1 text-xs text-text-primary placeholder:text-text-muted focus:border-brand-500 focus:outline-none"
		placeholder="Channel passphrase"
		bind:value={passphrase}
		onkeydown={(e) => e.key === 'Enter' && onunlock()}
	/>
	<button
		class="shrink-0 rounded bg-brand-500 px-2.5 py-1 text-xs font-medium text-white hover:bg-brand-600 disabled:opacity-50"
		onclick={onunlock}
		disabled={loading || !passphrase.trim()}
	>
		{loading ? '...' : 'Unlock'}
	</button>
</div>
