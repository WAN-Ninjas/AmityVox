<script lang="ts">
	import { api, type ChannelTemplate } from '$lib/api/client';
	import { addToast } from '$lib/stores/toast';
	import { createAsyncOp } from '$lib/utils/asyncOp';
	import { getErrorMessage } from '$lib/utils/apiError';
	import GuildTemplates from '$lib/components/guild/GuildTemplates.svelte';

	interface Props {
		guildId: string;
	}

	let { guildId }: Props = $props();

	let channelTemplates = $state<ChannelTemplate[]>([]);
	let loadedGuildId = $state<string | null>(null);

	let newTemplateName = $state('');
	let newTemplateChannelType = $state('text');
	let newTemplateTopic = $state('');
	let newTemplateSlowmode = $state(0);
	let newTemplateNsfw = $state(false);
	let applyingTemplateId = $state<string | null>(null);
	let applyChannelName = $state('');

	let loadOp = $state(createAsyncOp());
	let createOp = $state(createAsyncOp());
	let applyOp = $state(createAsyncOp());

	$effect(() => {
		if (guildId && loadedGuildId !== guildId && !loadOp.loading) {
			loadChannelTemplates();
		}
	});

	async function loadChannelTemplates() {
		await loadOp.run(async () => {
			channelTemplates = await api.getChannelTemplates(guildId);
			loadedGuildId = guildId;
		}, (message) => addToast(message, 'error'), 'Failed to load templates');
	}

	async function handleCreateTemplate() {
		if (!newTemplateName.trim()) return;
		await createOp.run(async () => {
			const template = await api.createChannelTemplate(guildId, {
				name: newTemplateName.trim(),
				channel_type: newTemplateChannelType,
				topic: newTemplateTopic.trim() || null,
				slowmode_seconds: newTemplateSlowmode,
				nsfw: newTemplateNsfw
			});
			channelTemplates = [...channelTemplates, template];
			newTemplateName = '';
			newTemplateTopic = '';
			newTemplateSlowmode = 0;
			newTemplateNsfw = false;
			newTemplateChannelType = 'text';
			addToast('Channel template created', 'success');
		}, (message) => addToast(message, 'error'), 'Failed to create template');
	}

	async function handleDeleteTemplate(templateId: string) {
		try {
			await api.deleteChannelTemplate(guildId, templateId);
			channelTemplates = channelTemplates.filter((template) => template.id !== templateId);
			addToast('Template deleted', 'success');
		} catch (err: unknown) {
			addToast(getErrorMessage(err, 'Failed to delete template'), 'error');
		}
	}

	async function handleApplyTemplate() {
		if (!applyingTemplateId || !applyChannelName.trim()) return;
		const templateId = applyingTemplateId;
		await applyOp.run(async () => {
			const channel = await api.applyChannelTemplate(guildId, templateId, {
				name: applyChannelName.trim()
			});
			applyingTemplateId = null;
			applyChannelName = '';
			addToast(`Channel "${channel.name}" created from template`, 'success');
		}, (message) => addToast(message, 'error'), 'Failed to apply template');
	}

	function cancelApplyTemplate() {
		applyingTemplateId = null;
		applyChannelName = '';
	}

	function formatDate(iso: string): string {
		return new Date(iso).toLocaleString();
	}
</script>

<h1 class="mb-6 text-xl font-bold text-text-primary">Channel Templates</h1>
<p class="mb-4 text-sm text-text-muted">
	Save channel configurations as reusable templates. When creating new channels, apply a template to instantly configure type, topic, slowmode, and permissions.
</p>

<div class="mb-6 rounded-lg bg-bg-secondary p-4">
	<h3 class="mb-3 text-sm font-semibold text-text-primary">Create Template</h3>
	<div class="space-y-3">
		<div>
			<label for="newTemplateName" class="mb-1 block text-xs text-text-muted">Template Name</label>
			<input
				id="newTemplateName"
				type="text"
				class="input w-full"
				placeholder="e.g. Announcement Channel"
				bind:value={newTemplateName}
				maxlength="100"
			/>
		</div>
		<div class="flex gap-3">
			<div class="flex-1">
				<label for="newTemplateChannelType" class="mb-1 block text-xs text-text-muted">Channel Type</label>
				<select id="newTemplateChannelType" class="input w-full" bind:value={newTemplateChannelType}>
					<option value="text">Text</option>
					<option value="voice">Voice</option>
					<option value="announcement">Announcement</option>
					<option value="forum">Forum</option>
					<option value="stage">Stage</option>
				</select>
			</div>
			<div class="flex-1">
				<label for="newTemplateSlowmode" class="mb-1 block text-xs text-text-muted">Slowmode (seconds)</label>
				<input
					id="newTemplateSlowmode"
					type="number"
					class="input w-full"
					bind:value={newTemplateSlowmode}
					min="0"
					max="21600"
				/>
			</div>
		</div>
		<div>
			<label for="newTemplateTopic" class="mb-1 block text-xs text-text-muted">Topic</label>
			<input
				id="newTemplateTopic"
				type="text"
				class="input w-full"
				placeholder="Channel topic (optional)"
				bind:value={newTemplateTopic}
			/>
		</div>
		<div class="flex items-center gap-2">
			<label class="flex items-center gap-2 text-sm text-text-muted">
				<input type="checkbox" bind:checked={newTemplateNsfw} class="rounded" />
				NSFW
			</label>
		</div>
		<button class="btn-primary text-sm" onclick={handleCreateTemplate} disabled={createOp.loading || !newTemplateName.trim()}>
			{createOp.loading ? 'Creating...' : 'Create Template'}
		</button>
	</div>
</div>

{#if loadOp.loading}
	<p class="text-sm text-text-muted">Loading templates...</p>
{:else if channelTemplates.length === 0}
	<p class="text-sm text-text-muted">No templates yet. Create one above.</p>
{:else}
	<div class="space-y-3">
		{#each channelTemplates as tmpl (tmpl.id)}
			<div class="rounded-lg bg-bg-secondary p-4">
				<div class="flex items-start justify-between gap-3">
					<div>
						<h4 class="text-sm font-semibold text-text-primary">{tmpl.name}</h4>
						<div class="mt-1 flex flex-wrap gap-2">
							<span class="rounded bg-bg-modifier px-1.5 py-0.5 text-xs text-text-muted">{tmpl.channel_type}</span>
							{#if tmpl.nsfw}
								<span class="rounded bg-red-500/20 px-1.5 py-0.5 text-xs text-red-400">NSFW</span>
							{/if}
							{#if tmpl.slowmode_seconds > 0}
								<span class="rounded bg-bg-modifier px-1.5 py-0.5 text-xs text-text-muted">Slowmode: {tmpl.slowmode_seconds}s</span>
							{/if}
						</div>
						{#if tmpl.topic}
							<p class="mt-1 text-xs text-text-muted">{tmpl.topic}</p>
						{/if}
						<p class="mt-1 text-xs text-text-muted">Created {formatDate(tmpl.created_at)}</p>
					</div>
					<div class="flex gap-2">
						{#if applyingTemplateId === tmpl.id}
							<div class="flex items-center gap-2">
								<input
									type="text"
									class="input text-xs"
									placeholder="New channel name"
									bind:value={applyChannelName}
									maxlength="100"
								/>
								<button class="btn-primary text-xs" onclick={handleApplyTemplate} disabled={applyOp.loading || !applyChannelName.trim()}>
									{applyOp.loading ? 'Creating...' : 'Create'}
								</button>
								<button class="btn-secondary text-xs" onclick={cancelApplyTemplate}>
									Cancel
								</button>
							</div>
						{:else}
							<button class="btn-primary text-xs" onclick={() => { applyingTemplateId = tmpl.id; applyChannelName = ''; }}>
								Apply
							</button>
						{/if}
						<button class="text-xs text-red-400 hover:text-red-300" onclick={() => handleDeleteTemplate(tmpl.id)}>
							Delete
						</button>
					</div>
				</div>
			</div>
		{/each}
	</div>
{/if}

<hr class="my-6 border-bg-modifier" />
<GuildTemplates guildId={guildId} />
