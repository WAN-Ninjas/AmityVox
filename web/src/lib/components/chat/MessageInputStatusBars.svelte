<script lang="ts">
	import type { Message } from '$lib/types';

	interface Props {
		replyingTo: Message | null;
		editingMessage: Message | null;
		silentMode: boolean;
		slowmodeRemainingSeconds: number;
		oncancelreply: () => void;
		oncanceledit: () => void;
	}

	let { replyingTo, editingMessage, silentMode = $bindable(false), slowmodeRemainingSeconds, oncancelreply, oncanceledit }: Props = $props();
</script>

{#if replyingTo}
	<div class="mb-2 flex items-center gap-2 rounded-t-lg bg-bg-secondary px-3 py-2 text-sm">
		<svg class="h-4 w-4 shrink-0 text-text-muted" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
			<path d="M3 10h10a5 5 0 015 5v6M3 10l6 6m-6-6l6-6" />
		</svg>
		<span class="text-text-muted">Replying to</span>
		<span class="font-medium text-text-primary">{replyingTo.author?.display_name ?? replyingTo.author?.username ?? 'Unknown'}</span>
		<span class="flex-1 truncate text-text-muted">{replyingTo.content?.slice(0, 60)}</span>
		<button class="shrink-0 text-text-muted hover:text-text-primary" onclick={oncancelreply} title="Cancel reply">
			<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
				<path d="M6 18L18 6M6 6l12 12" />
			</svg>
		</button>
	</div>
{/if}

{#if editingMessage}
	<div class="mb-2 flex items-center gap-2 rounded-t-lg bg-yellow-500/10 px-3 py-2 text-sm">
		<svg class="h-4 w-4 shrink-0 text-yellow-500" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
			<path d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" />
		</svg>
		<span class="text-yellow-500">Editing message</span>
		<span class="flex-1"></span>
		<button class="shrink-0 text-text-muted hover:text-text-primary" onclick={oncanceledit} title="Cancel edit">
			<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
				<path d="M6 18L18 6M6 6l12 12" />
			</svg>
		</button>
	</div>
{/if}

{#if silentMode && !editingMessage}
	<div class="mb-2 flex items-center gap-2 rounded-t-lg bg-bg-secondary px-3 py-1.5 text-xs text-text-muted">
		<svg class="h-3.5 w-3.5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
			<path d="M5.586 15H4a1 1 0 01-1-1v-4a1 1 0 011-1h1.586l4.707-4.707C10.923 3.663 12 4.109 12 5v14c0 .891-1.077 1.337-1.707.707L5.586 15z" />
			<path d="M17 14l2-2m0 0l2-2m-2 2l-2-2m2 2l2 2" />
		</svg>
		<span>Silent mode -- recipients will not be notified</span>
		<button class="ml-auto text-text-muted hover:text-text-primary" onclick={() => (silentMode = false)} title="Disable silent mode">
			<svg class="h-3.5 w-3.5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
				<path d="M6 18L18 6M6 6l12 12" />
			</svg>
		</button>
	</div>
{/if}

{#if slowmodeRemainingSeconds > 0}
	<div class="mb-2 flex items-center gap-2 rounded bg-bg-secondary px-3 py-2 text-xs text-text-muted">
		<svg class="h-4 w-4 shrink-0 text-yellow-400" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
			<path d="M12 8v4l3 3" />
			<circle cx="12" cy="12" r="9" />
		</svg>
		<span>Slowmode active. You can send again in {slowmodeRemainingSeconds}s.</span>
	</div>
{/if}
