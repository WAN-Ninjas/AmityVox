<script lang="ts">
	import { api } from '$lib/api/client';
	import { addToast } from '$lib/stores/toast';
	import { getErrorMessage } from '$lib/utils/apiError';
	import type { MessageComponent } from '$lib/types';

	interface Props {
		channelId: string;
		messageId: string;
		components?: MessageComponent[] | null;
	}

	let { channelId, messageId, components = [] }: Props = $props();
	let pending = $state<Set<string>>(new Set());

	type ComponentOption = {
		label?: string;
		value?: string;
		description?: string;
	};

	const visibleComponents = $derived((components ?? []).filter((component) => component.component_type !== 'action_row'));

	function optionsFor(component: MessageComponent): ComponentOption[] {
		return Array.isArray(component.options) ? component.options as ComponentOption[] : [];
	}

	function componentLabel(component: MessageComponent): string {
		return component.label || component.placeholder || 'Interact';
	}

	function buttonClass(component: MessageComponent): string {
		const style = component.style ?? 'secondary';
		if (style === 'primary') return 'border-brand-500 bg-brand-500 text-white hover:bg-brand-600';
		if (style === 'success') return 'border-green-600 bg-green-600 text-white hover:bg-green-700';
		if (style === 'danger') return 'border-red-600 bg-red-600 text-white hover:bg-red-700';
		if (style === 'link') return 'border-bg-modifier bg-bg-tertiary text-text-link hover:bg-bg-modifier';
		return 'border-bg-modifier bg-bg-tertiary text-text-secondary hover:bg-bg-modifier hover:text-text-primary';
	}

	function setPending(componentId: string, value: boolean) {
		const next = new Set(pending);
		if (value) next.add(componentId);
		else next.delete(componentId);
		pending = next;
	}

	async function interact(component: MessageComponent, values: string[] = []) {
		if (component.disabled || pending.has(component.id)) return;
		setPending(component.id, true);
		try {
			await api.interactMessageComponent(channelId, messageId, component.id, values);
			addToast('Interaction sent', 'success');
		} catch (err: unknown) {
			addToast(getErrorMessage(err, 'Failed to send interaction'), 'error');
		} finally {
			setPending(component.id, false);
		}
	}
</script>

{#if visibleComponents.length > 0}
	<div class="mt-2 flex max-w-xl flex-wrap gap-2">
		{#each visibleComponents as component (component.id)}
			{#if component.component_type === 'button'}
				{#if component.url}
					<a
						class="inline-flex h-8 items-center rounded border px-3 text-xs font-medium transition-colors {buttonClass(component)}"
						href={component.url}
						target="_blank"
						rel="noreferrer"
					>
						{componentLabel(component)}
					</a>
				{:else}
					<button
						class="inline-flex h-8 items-center rounded border px-3 text-xs font-medium transition-colors disabled:cursor-not-allowed disabled:opacity-50 {buttonClass(component)}"
						onclick={() => interact(component)}
						disabled={component.disabled || pending.has(component.id)}
					>
						{pending.has(component.id) ? 'Sending...' : componentLabel(component)}
					</button>
				{/if}
			{:else if component.component_type === 'select_menu'}
				<select
					class="{component.max_values && component.max_values > 1 ? 'min-h-20 py-1' : 'h-8'} max-w-full rounded border border-bg-modifier bg-bg-tertiary px-2 text-xs text-text-secondary outline-none focus:border-brand-500 disabled:cursor-not-allowed disabled:opacity-50"
					disabled={component.disabled || pending.has(component.id)}
					aria-label={component.placeholder || componentLabel(component)}
					multiple={!!component.max_values && component.max_values > 1}
					onchange={(event) => {
						const select = event.currentTarget as HTMLSelectElement;
						const values = Array.from(select.selectedOptions).map((option) => option.value).filter(Boolean);
						if (values.length) interact(component, values);
						for (const option of select.options) option.selected = false;
					}}
				>
					{#if !component.max_values || component.max_values <= 1}
						<option value="">{pending.has(component.id) ? 'Sending...' : component.placeholder || componentLabel(component)}</option>
					{/if}
					{#each optionsFor(component) as option}
						<option value={option.value ?? option.label ?? ''}>
							{option.label ?? option.value}
							{option.description ? ` - ${option.description}` : ''}
						</option>
					{/each}
				</select>
			{/if}
		{/each}
	</div>
{/if}
