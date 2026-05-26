<script lang="ts">
	import { api } from '$lib/api/client';
	import { addToast } from '$lib/stores/toast';
	import { createAsyncOp } from '$lib/utils/asyncOp';
	import { getErrorMessage } from '$lib/utils/apiError';
	import type { ScheduledMessage } from '$lib/types';

	interface Props {
		channelId: string;
		onclose?: () => void;
	}

	let { channelId, onclose }: Props = $props();

	let messages = $state<ScheduledMessage[]>([]);
	let loadedChannelId = $state<string | null>(null);
	let loadOp = $state(createAsyncOp());
	let cancelingId = $state<string | null>(null);

	$effect(() => {
		if (channelId && loadedChannelId !== channelId && !loadOp.loading) {
			loadMessages();
		}
	});

	async function loadMessages() {
		const result = await loadOp.run(
			() => api.getScheduledMessages(channelId),
			msg => addToast(msg, 'error'),
			'Failed to load scheduled messages'
		);
		if (result) {
			messages = result;
			loadedChannelId = channelId;
		}
	}

	async function cancelMessage(messageId: string) {
		cancelingId = messageId;
		try {
			await api.deleteScheduledMessage(channelId, messageId);
			messages = messages.filter((message) => message.id !== messageId);
			addToast('Scheduled message canceled', 'success');
		} catch (err: unknown) {
			addToast(getErrorMessage(err, 'Failed to cancel scheduled message'), 'error');
		} finally {
			cancelingId = null;
		}
	}

	function formatDate(value: string) {
		return new Date(value).toLocaleString([], {
			month: 'short',
			day: 'numeric',
			hour: 'numeric',
			minute: '2-digit'
		});
	}
</script>

<section class="border-b border-bg-floating bg-bg-secondary p-4">
	<div class="mb-3 flex items-center justify-between">
		<div>
			<h2 class="text-sm font-semibold text-text-primary">Scheduled Messages</h2>
			<p class="text-xs text-text-muted">Messages you have queued for this channel.</p>
		</div>
		<div class="flex items-center gap-2">
			<button class="btn-secondary text-xs" onclick={loadMessages} disabled={loadOp.loading}>
				{loadOp.loading ? 'Refreshing...' : 'Refresh'}
			</button>
			{#if onclose}
				<button class="btn-secondary text-xs" onclick={onclose}>Close</button>
			{/if}
		</div>
	</div>

	{#if loadOp.loading && messages.length === 0}
		<p class="text-sm text-text-muted">Loading scheduled messages...</p>
	{:else if loadOp.error}
		<p class="text-sm text-red-400">{loadOp.error}</p>
	{:else if messages.length === 0}
		<p class="text-sm text-text-muted">No scheduled messages in this channel.</p>
	{:else}
		<div class="space-y-2">
			{#each messages as message (message.id)}
				<div class="flex items-start justify-between gap-3 rounded border border-bg-modifier bg-bg-primary p-3">
					<div class="min-w-0">
						<div class="text-xs font-medium text-text-muted">{formatDate(message.scheduled_for)}</div>
						<p class="mt-1 whitespace-pre-wrap break-words text-sm text-text-primary">
							{message.content || `${message.attachment_ids.length} attachment${message.attachment_ids.length === 1 ? '' : 's'}`}
						</p>
					</div>
					<button
						class="btn-secondary shrink-0 text-xs text-red-400 hover:text-red-300"
						onclick={() => cancelMessage(message.id)}
						disabled={cancelingId === message.id}
					>
						{cancelingId === message.id ? 'Canceling...' : 'Cancel'}
					</button>
				</div>
			{/each}
		</div>
	{/if}
</section>
