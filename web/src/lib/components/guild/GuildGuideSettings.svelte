<script lang="ts">
	import { api, type GuideStep } from '$lib/api/client';
	import { addToast } from '$lib/stores/toast';
	import { createAsyncOp } from '$lib/utils/asyncOp';
	import type { Channel } from '$lib/types';

	interface Props {
		guildId: string;
		channels?: Channel[];
	}

	let { guildId, channels = [] }: Props = $props();

	let steps = $state<GuideStep[]>([]);
	let loadedGuildId = $state<string | null>(null);
	let loadOp = $state(createAsyncOp());
	let saveOp = $state(createAsyncOp());

	const textChannels = $derived(channels.filter((channel) =>
		channel.channel_type === 'text' || channel.channel_type === 'announcement' || channel.channel_type === 'forum' || channel.channel_type === 'gallery'
	));

	$effect(() => {
		if (guildId && loadedGuildId !== guildId && !loadOp.loading) {
			loadGuide();
		}
	});

	async function loadGuide() {
		const result = await loadOp.run(
			() => api.getServerGuide(guildId),
			msg => addToast(msg, 'error'),
			'Failed to load server guide'
		);
		if (result) {
			steps = result;
			loadedGuildId = guildId;
		}
	}

	function addStep() {
		steps = [
			...steps,
			{
				id: crypto.randomUUID(),
				guild_id: guildId,
				title: '',
				content: '',
				position: steps.length,
				channel_id: null,
				created_at: new Date().toISOString()
			}
		];
	}

	function removeStep(index: number) {
		steps = steps.filter((_, i) => i !== index).map((step, i) => ({ ...step, position: i }));
	}

	async function saveGuide() {
		const payload = steps
			.map((step, index) => ({
				title: step.title.trim(),
				content: step.content.trim(),
				position: index,
				channel_id: step.channel_id || null
			}))
			.filter((step) => step.title && step.content);
		const result = await saveOp.run(
			() => api.updateServerGuide(guildId, payload),
			msg => addToast(msg, 'error'),
			'Failed to save server guide'
		);
		if (result) {
			steps = result;
			addToast('Server guide saved', 'success');
		}
	}
</script>

<div class="mb-6 flex items-center justify-between gap-3">
	<div>
		<h1 class="text-xl font-bold text-text-primary">Server Guide</h1>
		<p class="mt-1 text-sm text-text-muted">Create the walkthrough shown on the server home page.</p>
	</div>
	<button class="btn-secondary text-sm" onclick={addStep} disabled={steps.length >= 20}>Add Step</button>
</div>

{#if loadOp.loading}
	<p class="text-sm text-text-muted">Loading guide...</p>
{:else if loadOp.error}
	<div class="rounded bg-red-500/10 px-3 py-2 text-sm text-red-400">{loadOp.error}</div>
{:else}
	<div class="space-y-3">
		{#each steps as step, index (step.id)}
			<div class="rounded border border-bg-floating bg-bg-secondary p-4">
				<div class="mb-3 flex items-center justify-between">
					<h2 class="text-sm font-semibold text-text-primary">Step {index + 1}</h2>
					<button class="text-xs text-red-400 hover:text-red-300" onclick={() => removeStep(index)}>Remove</button>
				</div>
				<div class="grid gap-3 md:grid-cols-2">
					<label class="block">
						<span class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">Title</span>
						<input class="input w-full" maxlength="100" bind:value={step.title} />
					</label>
					<label class="block">
						<span class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">Channel</span>
						<select class="input w-full" bind:value={step.channel_id}>
							<option value={null}>No channel link</option>
							{#each textChannels as channel (channel.id)}
								<option value={channel.id}>#{channel.name}</option>
							{/each}
						</select>
					</label>
				</div>
				<label class="mt-3 block">
					<span class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">Content</span>
					<textarea class="input min-h-24 w-full resize-y" maxlength="1000" bind:value={step.content}></textarea>
				</label>
			</div>
		{/each}
	</div>

	{#if steps.length === 0}
		<p class="rounded border border-bg-floating bg-bg-secondary p-4 text-sm text-text-muted">No guide steps yet.</p>
	{/if}

	<div class="mt-4 flex justify-end">
		<button class="btn-primary" onclick={saveGuide} disabled={saveOp.loading}>
			{saveOp.loading ? 'Saving...' : 'Save Guide'}
		</button>
	</div>
{/if}
