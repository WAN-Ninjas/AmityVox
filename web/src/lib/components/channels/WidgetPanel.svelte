<script lang="ts">
	import { api } from '$lib/api/client';
	import { addToast } from '$lib/stores/toast';
	import { createAsyncOp } from '$lib/utils/asyncOp';

	interface ChannelWidget {
		id: string;
		channel_id: string;
		guild_id: string;
		widget_type: string;
		title: string;
		config: Record<string, unknown>;
		creator_id: string;
		position: number;
		active: boolean;
		created_at: string;
		updated_at: string;
	}

	interface Props {
		channelId: string;
		guildId: string;
	}

	let { channelId, guildId }: Props = $props();

	let widgets = $state<ChannelWidget[]>([]);
	let loadOp = $state(createAsyncOp());
	let showAddForm = $state(false);
	let newTitle = $state('');
	let newType = $state('notes');
	let createOp = $state(createAsyncOp());

	const widgetTypeLabels: Record<string, string> = {
		notes: 'Collaborative Notes',
		youtube: 'YouTube Player',
		countdown: 'Countdown Timer',
		custom_iframe: 'Custom Embed'
	};

	const widgetTypeIcons: Record<string, string> = {
		notes: 'M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z',
		youtube: 'M14.752 11.168l-3.197-2.132A1 1 0 0010 9.87v4.263a1 1 0 001.555.832l3.197-2.132a1 1 0 000-1.664zM21 12a9 9 0 11-18 0 9 9 0 0118 0z',
		countdown: 'M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z',
		custom_iframe: 'M10 20l4-16m4 4l4 4-4 4M6 16l-4-4 4-4'
	};

	$effect(() => {
		const id = channelId;
		if (id) loadWidgets(id);
	});

	async function loadWidgets(chId: string) {
		const result = await loadOp.run(() => api.getChannelWidgets(chId));
		if (!loadOp.error) {
			widgets = result!;
		} else {
			widgets = [];
		}
	}

	async function addWidget() {
		if (!newTitle.trim()) {
			addToast('Widget title is required', 'error');
			return;
		}
		const widget = await createOp.run(
			() => api.createChannelWidget(channelId, {
				widget_type: newType,
				title: newTitle.trim(),
				config: {}
			}),
			msg => addToast(msg, 'error')
		);
		if (!createOp.error) {
			widgets = [...widgets, widget!];
			showAddForm = false;
			newTitle = '';
			addToast('Widget added', 'success');
		}
	}

	async function removeWidget(widgetId: string) {
		try {
			await api.deleteChannelWidget(channelId, widgetId);
			widgets = widgets.filter((w) => w.id !== widgetId);
			addToast('Widget removed', 'success');
		} catch (err: any) {
			addToast(err.message || 'Failed to remove widget', 'error');
		}
	}

	async function toggleWidget(widget: ChannelWidget) {
		try {
			await api.updateChannelWidget(channelId, widget.id, {
				active: !widget.active
			});
			widgets = widgets.map((w) =>
				w.id === widget.id ? { ...w, active: !w.active } : w
			);
		} catch (err: any) {
			addToast(err.message || 'Failed to toggle widget', 'error');
		}
	}
</script>

<div class="flex flex-col gap-3 p-4">
	<div class="flex items-center justify-between">
		<h3 class="text-sm font-semibold text-text-primary">Channel Widgets</h3>
		<button
			class="rounded bg-brand-500 px-3 py-1 text-xs font-medium text-white transition-colors hover:bg-brand-600"
			onclick={() => (showAddForm = !showAddForm)}
		>
			{showAddForm ? 'Cancel' : 'Add Widget'}
		</button>
	</div>

	{#if showAddForm}
		<div class="rounded-lg border border-bg-modifier bg-bg-secondary p-4">
			<div class="mb-3">
				<label class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">
					Widget Type
				</label>
				<select class="input w-full" bind:value={newType}>
					{#each Object.entries(widgetTypeLabels) as [value, label]}
						<option {value}>{label}</option>
					{/each}
				</select>
			</div>
			<div class="mb-3">
				<label class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">
					Title
				</label>
				<input
					type="text"
					class="input w-full"
					bind:value={newTitle}
					placeholder="Widget title..."
					maxlength="100"
				/>
			</div>
			<button
				class="btn-primary w-full"
				onclick={addWidget}
				disabled={createOp.loading || !newTitle.trim()}
			>
				{createOp.loading ? 'Creating...' : 'Create Widget'}
			</button>
		</div>
	{/if}

	{#if loadOp.loading}
		<div class="flex items-center justify-center py-8">
			<span class="inline-block h-5 w-5 animate-spin rounded-full border-2 border-brand-500 border-t-transparent"></span>
		</div>
	{:else if loadOp.error}
		<div class="rounded bg-red-500/10 px-3 py-2 text-sm text-red-400">{loadOp.error}</div>
	{:else if widgets.length === 0}
		<p class="py-4 text-center text-sm text-text-muted">
			No widgets in this channel. Add one to get started.
		</p>
	{:else}
		<div class="flex flex-col gap-2">
			{#each widgets as widget (widget.id)}
				<div
					class="flex items-center gap-3 rounded-lg border border-bg-modifier bg-bg-secondary p-3 transition-colors hover:border-brand-500/30"
					class:opacity-50={!widget.active}
				>
					<div class="flex h-8 w-8 shrink-0 items-center justify-center rounded-md bg-bg-tertiary">
						<svg class="h-4 w-4 text-text-muted" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
							<path d={widgetTypeIcons[widget.widget_type] || widgetTypeIcons.notes} />
						</svg>
					</div>
					<div class="min-w-0 flex-1">
						<p class="truncate text-sm font-medium text-text-primary">{widget.title}</p>
						<p class="text-xs text-text-muted">{widgetTypeLabels[widget.widget_type] || widget.widget_type}</p>
					</div>
					<div class="flex items-center gap-1">
						<button
							class="rounded p-1 text-text-muted transition-colors hover:bg-bg-modifier hover:text-text-primary"
							onclick={() => toggleWidget(widget)}
							title={widget.active ? 'Disable' : 'Enable'}
						>
							{#if widget.active}
								<svg class="h-4 w-4 text-status-online" fill="currentColor" viewBox="0 0 20 20">
									<path d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" />
								</svg>
							{:else}
								<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
									<circle cx="12" cy="12" r="10" />
								</svg>
							{/if}
						</button>
						<button
							class="rounded p-1 text-text-muted transition-colors hover:bg-red-500/10 hover:text-red-400"
							onclick={() => removeWidget(widget.id)}
							title="Remove widget"
						>
							<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
								<path d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
							</svg>
						</button>
					</div>
				</div>
			{/each}
		</div>
	{/if}
</div>
