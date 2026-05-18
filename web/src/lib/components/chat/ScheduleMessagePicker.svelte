<script lang="ts">
	interface Props {
		open: boolean;
		customDatetime: string;
		onpreset: (date: Date) => void;
		oncustom: () => void;
	}

	let { open = $bindable(false), customDatetime = $bindable(''), onpreset, oncustom }: Props = $props();

	function getSchedulePresets(): { label: string; getTime: () => Date }[] {
		return [
			{ label: 'In 15 minutes', getTime: () => new Date(Date.now() + 15 * 60 * 1000) },
			{ label: 'In 30 minutes', getTime: () => new Date(Date.now() + 30 * 60 * 1000) },
			{ label: 'In 1 hour', getTime: () => new Date(Date.now() + 60 * 60 * 1000) },
			{ label: 'In 4 hours', getTime: () => new Date(Date.now() + 4 * 60 * 60 * 1000) },
			{
				label: 'Tomorrow 9:00 AM',
				getTime: () => {
					const d = new Date();
					d.setDate(d.getDate() + 1);
					d.setHours(9, 0, 0, 0);
					return d;
				}
			}
		];
	}
</script>

{#if open}
	<div class="fixed inset-x-0 bottom-0 z-50 rounded-t-xl border-t border-bg-floating bg-bg-primary p-3 shadow-lg md:absolute md:inset-auto md:bottom-10 md:right-0 md:w-64 md:rounded-lg md:border">
		<div class="mb-2 text-xs font-semibold uppercase text-text-muted">Schedule Message</div>
		<div class="flex flex-col gap-1">
			{#each getSchedulePresets() as preset}
				<button class="rounded px-3 py-1.5 text-left text-sm text-text-primary hover:bg-bg-modifier" onclick={() => onpreset(preset.getTime())}>
					{preset.label}
				</button>
			{/each}
		</div>
		<div class="my-2 border-t border-bg-floating"></div>
		<div class="text-xs font-semibold uppercase text-text-muted mb-1.5">Custom</div>
		<input
			type="datetime-local"
			bind:value={customDatetime}
			class="mb-2 w-full rounded border border-bg-floating bg-bg-secondary px-2 py-1 text-sm text-text-primary outline-none focus:border-text-link"
		/>
		<button
			class="w-full rounded bg-text-link px-3 py-1.5 text-sm font-medium text-white hover:opacity-90 disabled:opacity-50"
			disabled={!customDatetime}
			onclick={oncustom}
		>
			Schedule
		</button>
	</div>
{/if}
