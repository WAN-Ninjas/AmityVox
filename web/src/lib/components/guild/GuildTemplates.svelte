<script lang="ts">
	import { api } from '$lib/api/client';
	import { createAsyncOp } from '$lib/utils/asyncOp';

	let { guildId }: { guildId: string } = $props();

	interface GuildTemplate {
		id: string;
		guild_id: string;
		name: string;
		description?: string;
		template_data: unknown;
		creator_id: string;
		created_at: string;
	}

	let templates = $state<GuildTemplate[]>([]);
	let error = $state('');
	let success = $state('');

	let loadOp = $state(createAsyncOp());
	let createOp = $state(createAsyncOp());
	let applyOp = $state(createAsyncOp());

	// Create template form.
	let showCreateForm = $state(false);
	let newTemplateName = $state('');
	let newTemplateDescription = $state('');

	// Apply template form.
	let applyingTemplateId = $state<string | null>(null);
	let applyNewGuildName = $state('');
	let showApplyDialog = $state(false);
	let applyMode = $state<'new' | 'existing'>('new');

	$effect(() => {
		if (guildId) {
			loadTemplates();
		}
	});

	async function loadTemplates() {
		error = '';
		const result = await loadOp.run(
			() => api.getGuildTemplates(guildId) as Promise<GuildTemplate[]>,
			msg => (error = msg)
		);
		if (result) templates = result;
	}

	async function handleCreateTemplate() {
		if (!newTemplateName.trim()) {
			error = 'Please enter a template name.';
			return;
		}
		error = '';
		success = '';
		const newTemplate = await createOp.run(
			() => api.createGuildTemplate(guildId, {
				name: newTemplateName.trim(),
				description: newTemplateDescription.trim() || undefined
			}) as Promise<GuildTemplate>,
			msg => (error = msg)
		);
		if (!createOp.error) {
			templates = [newTemplate!, ...templates];
			newTemplateName = '';
			newTemplateDescription = '';
			showCreateForm = false;
			success = 'Template created successfully!';
			setTimeout(() => (success = ''), 3000);
		}
	}

	async function handleDeleteTemplate(templateId: string) {
		if (!confirm('Are you sure you want to delete this template?')) return;
		error = '';
		try {
			await api.deleteGuildTemplate(guildId, templateId);
			templates = templates.filter((t) => t.id !== templateId);
			success = 'Template deleted.';
			setTimeout(() => (success = ''), 3000);
		} catch (err: any) {
			error = err.message || 'Failed to delete template';
		}
	}

	function openApplyDialog(templateId: string) {
		applyingTemplateId = templateId;
		applyNewGuildName = '';
		applyMode = 'new';
		showApplyDialog = true;
		error = '';
	}

	async function handleApplyTemplate() {
		if (!applyingTemplateId) return;
		if (applyMode === 'new' && !applyNewGuildName.trim()) {
			error = 'Please enter a server name.';
			return;
		}
		error = '';
		success = '';
		const payload: Record<string, string> = {
			template_id: applyingTemplateId
		};
		if (applyMode === 'new') {
			payload.guild_name = applyNewGuildName.trim();
		}
		await applyOp.run(
			() => api.applyGuildTemplate(guildId, applyingTemplateId!, payload),
			msg => (error = msg)
		);
		if (!applyOp.error) {
			showApplyDialog = false;
			applyingTemplateId = null;
			if (applyMode === 'new') {
				success = 'New server created from template!';
			} else {
				success = 'Template applied to this server!';
			}
			setTimeout(() => (success = ''), 5000);
		}
	}

	function handleExportTemplate(template: GuildTemplate) {
		const blob = new Blob([JSON.stringify(template.template_data, null, 2)], { type: 'application/json' });
		const url = URL.createObjectURL(blob);
		const a = document.createElement('a');
		a.href = url;
		a.download = `template-${template.name.replace(/\s+/g, '-').toLowerCase()}.json`;
		a.click();
		URL.revokeObjectURL(url);
	}

	function formatDate(iso: string): string {
		return new Date(iso).toLocaleDateString(undefined, {
			year: 'numeric',
			month: 'short',
			day: 'numeric'
		});
	}
</script>

<div class="space-y-4">
	{#if error}
		<div class="rounded bg-red-500/10 px-3 py-2 text-sm text-red-400">{error}</div>
	{/if}
	{#if success}
		<div class="rounded bg-green-500/10 px-3 py-2 text-sm text-green-400">{success}</div>
	{/if}

	<!-- Create Template -->
	{#if showCreateForm}
		<div class="rounded-lg bg-bg-secondary p-4">
			<h4 class="mb-3 text-sm font-semibold text-text-primary">Create Template</h4>
			<p class="mb-3 text-xs text-text-muted">
				Save the current server structure (roles, channels, categories, settings) as a reusable template.
			</p>
			<div class="mb-3">
				<label for="templateName" class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">
					Template Name
				</label>
				<input
					id="templateName"
					type="text"
					class="input w-full"
					placeholder="e.g., Community Server"
					maxlength="100"
					bind:value={newTemplateName}
				/>
			</div>
			<div class="mb-3">
				<label for="templateDesc" class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">
					Description (optional)
				</label>
				<input
					id="templateDesc"
					type="text"
					class="input w-full"
					placeholder="A brief description of what this template sets up"
					maxlength="500"
					bind:value={newTemplateDescription}
				/>
			</div>
			<div class="flex items-center gap-2">
				<button class="btn-primary" onclick={handleCreateTemplate} disabled={createOp.loading || !newTemplateName.trim()}>
					{createOp.loading ? 'Creating...' : 'Create Template'}
				</button>
				<button class="btn-secondary" onclick={() => (showCreateForm = false)}>Cancel</button>
			</div>
		</div>
	{:else}
		<button class="btn-primary" onclick={() => (showCreateForm = true)}>
			Create Template from Current Structure
		</button>
	{/if}

	<!-- Template List -->
	{#if loadOp.loading}
		<div class="flex items-center gap-2 py-4">
			<div class="h-4 w-4 animate-spin rounded-full border-2 border-brand-500 border-t-transparent"></div>
			<span class="text-sm text-text-muted">Loading templates...</span>
		</div>
	{:else if templates.length === 0}
		<div class="rounded-lg bg-bg-secondary p-6 text-center">
			<p class="text-sm text-text-muted">No templates yet.</p>
			<p class="text-xs text-text-muted">Create one above to save this server's structure as a reusable template.</p>
		</div>
	{:else}
		<div class="space-y-3">
			{#each templates as template (template.id)}
				<div class="rounded-lg bg-bg-secondary p-4">
					<div class="flex items-start justify-between">
						<div>
							<h4 class="text-sm font-semibold text-text-primary">{template.name}</h4>
							{#if template.description}
								<p class="mt-0.5 text-xs text-text-muted">{template.description}</p>
							{/if}
							<p class="mt-1 text-2xs text-text-muted">Created {formatDate(template.created_at)}</p>
						</div>
						<div class="flex items-center gap-2">
							<button
								class="text-xs text-brand-400 hover:text-brand-300"
								onclick={() => openApplyDialog(template.id)}
								title="Apply this template"
							>
								Apply
							</button>
							<button
								class="text-xs text-text-muted hover:text-text-primary"
								onclick={() => handleExportTemplate(template)}
								title="Download template data as JSON"
							>
								Export
							</button>
							<button
								class="text-xs text-red-400 hover:text-red-300"
								onclick={() => handleDeleteTemplate(template.id)}
								title="Delete this template"
							>
								Delete
							</button>
						</div>
					</div>
				</div>
			{/each}
		</div>
	{/if}

	<!-- Apply Template Dialog -->
	{#if showApplyDialog}
		<!-- svelte-ignore a11y_no_static_element_interactions -->
		<div
			class="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
			onkeydown={(e) => { if (e.key === 'Escape') showApplyDialog = false; }}
			onclick={() => (showApplyDialog = false)}
		>
			<!-- svelte-ignore a11y_click_events_have_key_events -->
			<!-- svelte-ignore a11y_no_static_element_interactions -->
			<div class="w-full max-w-md rounded-lg bg-bg-secondary p-6 shadow-xl" onclick={(e) => e.stopPropagation()}>
				<h3 class="mb-4 text-lg font-bold text-text-primary">Apply Template</h3>

				<div class="mb-4 space-y-3">
					<label class="flex items-center gap-2">
						<input type="radio" name="applyMode" value="new" bind:group={applyMode} class="accent-brand-500" />
						<span class="text-sm text-text-secondary">Create a new server from this template</span>
					</label>
					<label class="flex items-center gap-2">
						<input type="radio" name="applyMode" value="existing" bind:group={applyMode} class="accent-brand-500" />
						<span class="text-sm text-text-secondary">Apply to this server (adds missing roles/channels)</span>
					</label>
				</div>

				{#if applyMode === 'new'}
					<div class="mb-4">
						<label for="newGuildName" class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">
							New Server Name
						</label>
						<input
							id="newGuildName"
							type="text"
							class="input w-full"
							placeholder="My New Server"
							maxlength="100"
							bind:value={applyNewGuildName}
						/>
					</div>
				{:else}
					<div class="mb-4 rounded bg-yellow-500/10 px-3 py-2 text-xs text-yellow-300">
						This will add roles and channels from the template that don't already exist in this server.
						Existing roles and channels will not be modified or removed.
					</div>
				{/if}

				<div class="flex items-center justify-end gap-2">
					<button class="btn-secondary" onclick={() => (showApplyDialog = false)}>Cancel</button>
					<button
						class="btn-primary"
						onclick={handleApplyTemplate}
						disabled={applyOp.loading || (applyMode === 'new' && !applyNewGuildName.trim())}
					>
						{applyOp.loading ? 'Applying...' : applyMode === 'new' ? 'Create Server' : 'Apply Template'}
					</button>
				</div>
			</div>
		</div>
	{/if}
</div>
