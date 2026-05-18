<script lang="ts">
	interface Props {
		open: boolean;
		name: string;
		color: string;
		creating: boolean;
		onclose: () => void;
		oncreate: () => void;
	}

	let { open, name = $bindable(), color = $bindable(), creating, onclose, oncreate }: Props = $props();
</script>

{#if open}
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div
		class="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
		onclick={onclose}
		onkeydown={(e) => e.key === 'Escape' && onclose()}
		role="dialog"
		aria-modal="true"
		tabindex="-1"
	>
		<!-- svelte-ignore a11y_no_static_element_interactions, a11y_no_noninteractive_element_interactions -->
		<div
			class="w-full max-w-sm rounded-lg bg-bg-floating p-5 shadow-xl"
			onclick={(e) => e.stopPropagation()}
			onkeydown={(e) => e.stopPropagation()}
			role="document"
			tabindex="-1"
		>
			<h3 class="mb-4 text-base font-semibold text-text-primary">Create Channel Group</h3>

			<div class="mb-3">
				<label for="groupName" class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">
					Group Name
				</label>
				<input
					id="groupName"
					type="text"
					class="input w-full"
					bind:value={name}
					placeholder="My Favorites"
					maxlength="64"
					onkeydown={(e) => e.key === 'Enter' && oncreate()}
				/>
			</div>

			<div class="mb-4">
				<label for="groupColor" class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">
					Color
				</label>
				<div class="flex items-center gap-2">
					<input
						id="groupColor"
						type="color"
						class="h-8 w-8 cursor-pointer rounded border-0 bg-transparent p-0"
						bind:value={color}
					/>
					<span class="text-xs text-text-muted">{color}</span>
				</div>
			</div>

			<div class="flex justify-end gap-2">
				<button class="btn-secondary" onclick={onclose}>Cancel</button>
				<button class="btn-primary" onclick={oncreate} disabled={creating || !name.trim()}>
					{creating ? 'Creating...' : 'Create'}
				</button>
			</div>
		</div>
	</div>
{/if}
