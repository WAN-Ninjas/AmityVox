<script lang="ts">
	import Modal from './Modal.svelte';
	import { pendingConfirm, resolveConfirm } from '$lib/stores/confirm';
</script>

<Modal
	open={!!$pendingConfirm}
	title={$pendingConfirm?.title ?? ''}
	onclose={() => resolveConfirm(false)}
>
	{#if $pendingConfirm}
		<div class="space-y-4">
			<p class="text-sm leading-relaxed text-text-secondary">{$pendingConfirm.message}</p>
			<div class="flex justify-end gap-2">
				<button class="btn-secondary text-sm" onclick={() => resolveConfirm(false)}>
					{$pendingConfirm.cancelLabel}
				</button>
				<button
					class={$pendingConfirm.variant === 'danger' ? 'btn-danger text-sm' : 'btn-primary text-sm'}
					onclick={() => resolveConfirm(true)}
				>
					{$pendingConfirm.confirmLabel}
				</button>
			</div>
		</div>
	{/if}
</Modal>
