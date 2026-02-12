<script lang="ts">
	import { api } from '$lib/api/client';
	import Modal from '$components/common/Modal.svelte';

	interface Props {
		open?: boolean;
		channelId: string;
		messageId: string;
		onclose: () => void;
	}

	let { open = false, channelId, messageId, onclose }: Props = $props();

	let edits = $state<{ content: string; edited_at: string }[]>([]);
	let loading = $state(true);
	let error = $state('');

	$effect(() => {
		if (open && channelId && messageId) {
			loading = true;
			error = '';
			api.getMessageEdits(channelId, messageId)
				.then((data) => (edits = data))
				.catch((e) => (error = e.message || 'Failed to load edit history'))
				.finally(() => (loading = false));
		}
	});

	function formatTime(ts: string): string {
		return new Date(ts).toLocaleString();
	}
</script>

<Modal {open} title="Edit History" {onclose}>
	{#if loading}
		<div class="flex items-center justify-center py-8">
			<div class="h-5 w-5 animate-spin rounded-full border-2 border-brand-500 border-t-transparent"></div>
		</div>
	{:else if error}
		<p class="py-4 text-center text-sm text-red-400">{error}</p>
	{:else if edits.length === 0}
		<p class="py-4 text-center text-sm text-text-muted">No edit history available.</p>
	{:else}
		<div class="max-h-80 space-y-3 overflow-y-auto">
			{#each edits as edit, i (i)}
				<div class="rounded bg-bg-primary p-3">
					<div class="mb-1 flex items-center justify-between text-xs text-text-muted">
						<span>Version {edits.length - i}</span>
						<time>{formatTime(edit.edited_at)}</time>
					</div>
					<p class="whitespace-pre-wrap text-sm text-text-secondary">{edit.content}</p>
				</div>
			{/each}
		</div>
	{/if}
</Modal>
