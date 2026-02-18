<script lang="ts">
	import { api } from '$lib/api/client';

	let { channelId }: { channelId: string } = $props();

	let exporting = $state(false);
	let error = $state('');
	let success = $state('');

	async function handleExport() {
		exporting = true;
		error = '';
		success = '';
		try {
			const exportData = await api.exportChannelMessages(channelId, 'json');
			const blob = new Blob([JSON.stringify(exportData, null, 2)], { type: 'application/json' });
			const url = URL.createObjectURL(blob);
			const a = document.createElement('a');
			a.href = url;
			a.download = `channel-${channelId}-export-${new Date().toISOString().slice(0, 10)}.json`;
			a.click();
			URL.revokeObjectURL(url);
			success = 'Messages exported!';
			setTimeout(() => (success = ''), 3000);
		} catch (err: any) {
			error = err.message || 'Failed to export messages';
			setTimeout(() => (error = ''), 5000);
		} finally {
			exporting = false;
		}
	}
</script>

<div class="inline-flex flex-col items-start gap-1">
	<button
		class="flex items-center gap-1.5 rounded px-2 py-1 text-xs text-text-muted transition-colors hover:bg-bg-modifier hover:text-text-secondary"
		onclick={handleExport}
		disabled={exporting}
		title="Export all messages in this channel as JSON"
	>
		<svg class="h-3.5 w-3.5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
			<path d="M12 10v6m0 0l-3-3m3 3l3-3m2 8H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
		</svg>
		{exporting ? 'Exporting...' : 'Export Messages'}
	</button>
	{#if success}
		<span class="text-2xs text-green-400">{success}</span>
	{/if}
	{#if error}
		<span class="text-2xs text-red-400">{error}</span>
	{/if}
</div>
