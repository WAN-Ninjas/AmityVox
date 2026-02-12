<script lang="ts">
	interface Props {
		label: string;
		danger?: boolean;
		disabled?: boolean;
		onclick?: () => void;
	}

	let { label, danger = false, disabled = false, onclick }: Props = $props();

	const classes = $derived.by(() => {
		const base = 'flex w-full items-center rounded px-2 py-1.5 text-left text-sm transition-colors';
		if (disabled) return `${base} text-text-muted cursor-not-allowed`;
		if (danger) return `${base} text-red-400 hover:bg-red-500 hover:text-white`;
		return `${base} text-text-primary hover:bg-brand-500 hover:text-white`;
	});
</script>

<button
	class={classes}
	disabled={disabled}
	onclick={() => { if (!disabled) onclick?.(); }}
	role="menuitem"
>
	{label}
</button>
