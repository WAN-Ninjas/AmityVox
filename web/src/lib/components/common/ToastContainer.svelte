<script lang="ts">
	import { toasts, dismissToast } from '$lib/stores/toast';
	import { fly, fade } from 'svelte/transition';

	const typeStyles: Record<string, string> = {
		info: 'bg-brand-500',
		success: 'bg-green-600',
		error: 'bg-red-600',
		warning: 'bg-yellow-600'
	};

	const typeIcons: Record<string, string> = {
		info: 'M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z',
		success: 'M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z',
		error: 'M10 14l2-2m0 0l2-2m-2 2l-2-2m2 2l2 2m7-2a9 9 0 11-18 0 9 9 0 0118 0z',
		warning: 'M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-2.5L13.732 4.5c-.77-.833-1.964-.833-2.732 0L3.34 16.5c-.77.833.192 2.5 1.732 2.5z'
	};
</script>

<div class="fixed bottom-4 right-4 z-[200] flex flex-col gap-2" aria-live="polite">
	{#each $toasts as toast (toast.id)}
		<div
			class="flex items-center gap-3 rounded-lg px-4 py-3 text-white shadow-lg {typeStyles[toast.type] ?? typeStyles.info}"
			in:fly={{ x: 100, duration: 200 }}
			out:fade={{ duration: 150 }}
			role="alert"
		>
			<svg class="h-5 w-5 shrink-0" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
				<path d={typeIcons[toast.type] ?? typeIcons.info} />
			</svg>
			<span class="text-sm">{toast.message}</span>
			<button
				class="ml-2 shrink-0 rounded p-0.5 opacity-70 hover:opacity-100"
				onclick={() => dismissToast(toast.id)}
				aria-label="Dismiss"
			>
				<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
					<path d="M6 18L18 6M6 6l12 12" />
				</svg>
			</button>
		</div>
	{/each}
</div>
