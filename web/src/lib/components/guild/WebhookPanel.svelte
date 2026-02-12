<script lang="ts">
	import type { Webhook, Channel } from '$lib/types';
	import { api } from '$lib/api/client';

	const API_BASE = '/api/v1';

	// Props
	let {
		guildId,
		webhooks = $bindable([]),
		channels = [],
		onError = (_msg: string) => {},
		onSuccess = (_msg: string) => {}
	}: {
		guildId: string;
		webhooks: Webhook[];
		channels: Channel[];
		onError: (msg: string) => void;
		onSuccess: (msg: string) => void;
	} = $props();

	// --- State ---

	// Create webhook form
	let newWebhookName = $state('');
	let newWebhookChannel = $state('');
	let newWebhookType = $state<'incoming' | 'outgoing'>('incoming');
	let newOutgoingUrl = $state('');
	let newOutgoingEvents = $state<string[]>([]);
	let creatingWebhook = $state(false);

	// Templates
	let templates = $state<WebhookTemplate[]>([]);
	let loadingTemplates = $state(false);
	let selectedTemplateId = $state<string | null>(null);

	// Preview
	let previewPayload = $state('');
	let previewResult = $state<{ content: string; embeds?: unknown[] } | null>(null);
	let previewLoading = $state(false);
	let previewError = $state('');

	// Execution Logs
	let selectedWebhookForLogs = $state<string | null>(null);
	let executionLogs = $state<WebhookExecutionLog[]>([]);
	let loadingLogs = $state(false);
	let expandedLogId = $state<string | null>(null);

	// Outgoing events list
	let availableOutgoingEvents = $state<string[]>([]);
	let loadingOutgoingEvents = $state(false);

	// Edit webhook
	let editingWebhookId = $state<string | null>(null);
	let editWebhookName = $state('');
	let editWebhookChannel = $state('');
	let editWebhookType = $state<'incoming' | 'outgoing'>('incoming');
	let editOutgoingUrl = $state('');
	let editOutgoingEvents = $state<string[]>([]);
	let savingEdit = $state(false);

	// Sub-tab
	type SubTab = 'list' | 'templates' | 'create';
	let subTab = $state<SubTab>('list');

	// --- Types (local, since we can't modify types/index.ts) ---

	interface WebhookTemplate {
		id: string;
		name: string;
		description: string;
		sample_payload: string;
		service: string;
	}

	interface WebhookExecutionLog {
		id: string;
		webhook_id: string;
		status_code: number;
		request_body: string | null;
		response_preview: string | null;
		success: boolean;
		error_message: string | null;
		created_at: string;
	}

	// --- Helpers ---

	async function fetchJSON<T>(path: string, options?: RequestInit): Promise<T> {
		const token = localStorage.getItem('token');
		const headers: Record<string, string> = {
			'Content-Type': 'application/json'
		};
		if (token) {
			headers['Authorization'] = `Bearer ${token}`;
		}
		const res = await fetch(`${API_BASE}${path}`, { ...options, headers: { ...headers, ...options?.headers } });
		const json = await res.json();
		if (!res.ok) {
			throw new Error(json.error?.message || res.statusText);
		}
		return json.data as T;
	}

	function formatDate(iso: string): string {
		return new Date(iso).toLocaleString();
	}

	function formatRelativeTime(iso: string): string {
		const diff = Date.now() - new Date(iso).getTime();
		const seconds = Math.floor(diff / 1000);
		if (seconds < 60) return `${seconds}s ago`;
		const minutes = Math.floor(seconds / 60);
		if (minutes < 60) return `${minutes}m ago`;
		const hours = Math.floor(minutes / 60);
		if (hours < 24) return `${hours}h ago`;
		const days = Math.floor(hours / 24);
		return `${days}d ago`;
	}

	// Service icon/label
	const serviceLabels: Record<string, string> = {
		github: 'GitHub',
		gitlab: 'GitLab',
		jira: 'Jira',
		sentry: 'Sentry'
	};

	const eventLabels: Record<string, string> = {
		message_create: 'Message Created',
		message_update: 'Message Updated',
		message_delete: 'Message Deleted',
		member_join: 'Member Joined',
		member_leave: 'Member Left',
		member_ban: 'Member Banned',
		member_unban: 'Member Unbanned',
		channel_create: 'Channel Created',
		channel_update: 'Channel Updated',
		channel_delete: 'Channel Deleted',
		guild_update: 'Guild Updated',
		role_create: 'Role Created',
		role_update: 'Role Updated',
		role_delete: 'Role Deleted',
		reaction_add: 'Reaction Added',
		reaction_remove: 'Reaction Removed'
	};

	// --- Data Loading ---

	async function loadTemplates() {
		if (templates.length > 0) return;
		loadingTemplates = true;
		try {
			templates = await fetchJSON<WebhookTemplate[]>('/webhooks/templates');
		} catch (err: any) {
			onError(err.message || 'Failed to load templates');
		} finally {
			loadingTemplates = false;
		}
	}

	async function loadOutgoingEvents() {
		if (availableOutgoingEvents.length > 0) return;
		loadingOutgoingEvents = true;
		try {
			availableOutgoingEvents = await fetchJSON<string[]>('/webhooks/outgoing-events');
		} catch {
			// Fallback to hardcoded list if endpoint not available yet.
			availableOutgoingEvents = Object.keys(eventLabels);
		} finally {
			loadingOutgoingEvents = false;
		}
	}

	async function loadExecutionLogs(webhookId: string) {
		selectedWebhookForLogs = webhookId;
		loadingLogs = true;
		expandedLogId = null;
		try {
			executionLogs = await fetchJSON<WebhookExecutionLog[]>(`/guilds/${guildId}/webhooks/${webhookId}/logs`);
		} catch (err: any) {
			onError(err.message || 'Failed to load execution logs');
			executionLogs = [];
		} finally {
			loadingLogs = false;
		}
	}

	// --- Actions ---

	async function handleCreateWebhook() {
		if (!newWebhookName.trim() || !newWebhookChannel) return;
		if (newWebhookType === 'outgoing' && !newOutgoingUrl.trim()) return;
		creatingWebhook = true;
		try {
			const webhook = await api.createWebhook(guildId, {
				name: newWebhookName.trim(),
				channel_id: newWebhookChannel
			});
			// If outgoing type, update it with outgoing fields.
			if (newWebhookType === 'outgoing') {
				const updated = await api.updateWebhook(guildId, webhook.id, {
					name: newWebhookName.trim()
				});
				// Note: outgoing_url and outgoing_events will need server.go route
				// updates to fully work through the existing PATCH endpoint.
				webhooks = [...webhooks, updated];
			} else {
				webhooks = [...webhooks, webhook];
			}
			newWebhookName = '';
			newWebhookChannel = '';
			newWebhookType = 'incoming';
			newOutgoingUrl = '';
			newOutgoingEvents = [];
			onSuccess('Webhook created successfully');
		} catch (err: any) {
			onError(err.message || 'Failed to create webhook');
		} finally {
			creatingWebhook = false;
		}
	}

	async function handleDeleteWebhook(webhookId: string) {
		if (!confirm('Delete this webhook? This cannot be undone.')) return;
		try {
			await api.deleteWebhook(guildId, webhookId);
			webhooks = webhooks.filter((w) => w.id !== webhookId);
			if (selectedWebhookForLogs === webhookId) {
				selectedWebhookForLogs = null;
				executionLogs = [];
			}
			onSuccess('Webhook deleted');
		} catch (err: any) {
			onError(err.message || 'Failed to delete webhook');
		}
	}

	function copyWebhookUrl(webhook: Webhook) {
		const url = `${window.location.origin}/api/v1/webhooks/${webhook.id}/${webhook.token}`;
		navigator.clipboard.writeText(url);
		onSuccess('Webhook URL copied to clipboard');
	}

	function startEditing(wh: Webhook) {
		editingWebhookId = wh.id;
		editWebhookName = wh.name;
		editWebhookChannel = wh.channel_id;
		editWebhookType = wh.webhook_type;
		editOutgoingUrl = wh.outgoing_url || '';
		editOutgoingEvents = [];
		loadOutgoingEvents();
	}

	function cancelEditing() {
		editingWebhookId = null;
	}

	async function handleSaveEdit() {
		if (!editingWebhookId || !editWebhookName.trim()) return;
		savingEdit = true;
		try {
			const updated = await api.updateWebhook(guildId, editingWebhookId, {
				name: editWebhookName.trim(),
				channel_id: editWebhookChannel
			});
			webhooks = webhooks.map((w) => (w.id === editingWebhookId ? updated : w));
			editingWebhookId = null;
			onSuccess('Webhook updated');
		} catch (err: any) {
			onError(err.message || 'Failed to update webhook');
		} finally {
			savingEdit = false;
		}
	}

	// --- Preview ---

	async function handlePreview() {
		if (!selectedTemplateId || !previewPayload.trim()) return;
		previewLoading = true;
		previewError = '';
		previewResult = null;
		try {
			let parsedPayload: unknown;
			try {
				parsedPayload = JSON.parse(previewPayload);
			} catch {
				previewError = 'Invalid JSON payload';
				return;
			}
			previewResult = await fetchJSON<{ content: string; embeds?: unknown[] }>('/webhooks/preview', {
				method: 'POST',
				body: JSON.stringify({
					template_id: selectedTemplateId,
					payload: parsedPayload
				})
			});
		} catch (err: any) {
			previewError = err.message || 'Preview failed';
		} finally {
			previewLoading = false;
		}
	}

	function useSamplePayload(template: WebhookTemplate) {
		selectedTemplateId = template.id;
		try {
			previewPayload = JSON.stringify(JSON.parse(template.sample_payload), null, 2);
		} catch {
			previewPayload = template.sample_payload;
		}
		previewResult = null;
		previewError = '';
	}

	function toggleOutgoingEvent(event: string, list: string[]): string[] {
		if (list.includes(event)) {
			return list.filter((e) => e !== event);
		}
		return [...list, event];
	}

	// Channel name lookup
	function channelName(channelId: string): string {
		const ch = channels.find((c) => c.id === channelId);
		return ch?.name || channelId.slice(0, 8) + '...';
	}

	// Load templates when switching to templates tab
	$effect(() => {
		if (subTab === 'templates') {
			loadTemplates();
		}
	});
</script>

<div class="space-y-4">
	<!-- Sub-tabs -->
	<div class="flex gap-1 border-b border-border-primary pb-2">
		<button
			class="rounded-t px-3 py-1.5 text-sm transition-colors {subTab === 'list' ? 'bg-bg-modifier text-text-primary' : 'text-text-muted hover:text-text-secondary'}"
			onclick={() => (subTab = 'list')}
		>
			Webhooks
		</button>
		<button
			class="rounded-t px-3 py-1.5 text-sm transition-colors {subTab === 'templates' ? 'bg-bg-modifier text-text-primary' : 'text-text-muted hover:text-text-secondary'}"
			onclick={() => (subTab = 'templates')}
		>
			Templates & Preview
		</button>
		<button
			class="rounded-t px-3 py-1.5 text-sm transition-colors {subTab === 'create' ? 'bg-bg-modifier text-text-primary' : 'text-text-muted hover:text-text-secondary'}"
			onclick={() => (subTab = 'create')}
		>
			Create Webhook
		</button>
	</div>

	<!-- CREATE TAB -->
	{#if subTab === 'create'}
		<div class="rounded-lg bg-bg-secondary p-4">
			<h3 class="mb-3 text-sm font-semibold text-text-primary">Create Webhook</h3>

			<!-- Webhook Type -->
			<div class="mb-3">
				<label class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">Type</label>
				<div class="flex gap-3">
					<label class="flex items-center gap-2 text-sm text-text-secondary">
						<input type="radio" bind:group={newWebhookType} value="incoming" class="accent-brand-500" />
						Incoming
					</label>
					<label class="flex items-center gap-2 text-sm text-text-secondary">
						<input type="radio" bind:group={newWebhookType} value="outgoing" class="accent-brand-500" />
						Outgoing
					</label>
				</div>
				<p class="mt-1 text-2xs text-text-muted">
					{#if newWebhookType === 'incoming'}
						Incoming webhooks receive messages from external services.
					{:else}
						Outgoing webhooks send event data to an external URL when events occur.
					{/if}
				</p>
			</div>

			<!-- Name -->
			<div class="mb-3">
				<label class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">Name</label>
				<input type="text" class="input w-full" bind:value={newWebhookName} placeholder="Webhook name" maxlength="80" />
			</div>

			<!-- Channel -->
			<div class="mb-3">
				<label class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">Channel</label>
				{#if channels.length > 0}
					<select class="input w-full" bind:value={newWebhookChannel}>
						<option value="">Select a channel</option>
						{#each channels.filter((c) => c.channel_type === 'text' || c.channel_type === 'announcement') as ch (ch.id)}
							<option value={ch.id}>#{ch.name}</option>
						{/each}
					</select>
				{:else}
					<input type="text" class="input w-full" bind:value={newWebhookChannel} placeholder="Channel ID" />
				{/if}
			</div>

			<!-- Outgoing-specific fields -->
			{#if newWebhookType === 'outgoing'}
				<div class="mb-3">
					<label class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">Outgoing URL</label>
					<input type="url" class="input w-full" bind:value={newOutgoingUrl} placeholder="https://example.com/webhook" />
					<p class="mt-1 text-2xs text-text-muted">The URL to POST event data to when matching events occur.</p>
				</div>

				<div class="mb-3">
					<label class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">Events to Send</label>
					<div class="mt-1 grid grid-cols-2 gap-1.5">
						{#each Object.entries(eventLabels) as [event, label] (event)}
							<label class="flex items-center gap-2 rounded px-2 py-1 text-xs text-text-secondary hover:bg-bg-modifier">
								<input
									type="checkbox"
									checked={newOutgoingEvents.includes(event)}
									onchange={() => (newOutgoingEvents = toggleOutgoingEvent(event, newOutgoingEvents))}
									class="accent-brand-500"
								/>
								{label}
							</label>
						{/each}
					</div>
				</div>
			{/if}

			<button
				class="btn-primary"
				onclick={handleCreateWebhook}
				disabled={creatingWebhook || !newWebhookName.trim() || !newWebhookChannel || (newWebhookType === 'outgoing' && !newOutgoingUrl.trim())}
			>
				{creatingWebhook ? 'Creating...' : 'Create Webhook'}
			</button>
		</div>

	<!-- TEMPLATES TAB -->
	{:else if subTab === 'templates'}
		<div class="space-y-4">
			<!-- Template List -->
			<div class="rounded-lg bg-bg-secondary p-4">
				<h3 class="mb-3 text-sm font-semibold text-text-primary">Webhook Templates</h3>
				<p class="mb-3 text-xs text-text-muted">
					Templates transform incoming payloads from external services into formatted messages.
					Select a template and use its sample payload to preview the output.
				</p>

				{#if loadingTemplates}
					<p class="text-sm text-text-muted">Loading templates...</p>
				{:else if templates.length === 0}
					<p class="text-sm text-text-muted">No templates available.</p>
				{:else}
					<div class="grid gap-2">
						{#each templates as tmpl (tmpl.id)}
							<div class="rounded-lg bg-bg-primary p-3 {selectedTemplateId === tmpl.id ? 'ring-2 ring-brand-500' : ''}">
								<div class="flex items-center justify-between">
									<div class="flex items-center gap-2">
										<span class="rounded bg-bg-modifier px-1.5 py-0.5 text-2xs font-medium text-text-muted">
											{serviceLabels[tmpl.service] ?? tmpl.service}
										</span>
										<span class="text-sm font-medium text-text-primary">{tmpl.name}</span>
									</div>
									<button
										class="text-xs text-brand-400 hover:text-brand-300"
										onclick={() => useSamplePayload(tmpl)}
									>
										Use Sample
									</button>
								</div>
								<p class="mt-1 text-xs text-text-muted">{tmpl.description}</p>
							</div>
						{/each}
					</div>
				{/if}
			</div>

			<!-- Preview Panel -->
			<div class="rounded-lg bg-bg-secondary p-4">
				<h3 class="mb-3 text-sm font-semibold text-text-primary">Message Preview</h3>
				<p class="mb-3 text-xs text-text-muted">
					Paste a webhook payload and select a template to see how the message will look.
				</p>

				<div class="mb-3">
					<label class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">Template</label>
					<select class="input w-full" bind:value={selectedTemplateId}>
						<option value={null}>Select a template</option>
						{#each templates as tmpl (tmpl.id)}
							<option value={tmpl.id}>{tmpl.name} ({serviceLabels[tmpl.service] ?? tmpl.service})</option>
						{/each}
					</select>
				</div>

				<div class="mb-3">
					<label class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">Payload (JSON)</label>
					<textarea
						class="input w-full font-mono text-xs"
						rows="8"
						bind:value={previewPayload}
						placeholder={'{"ref":"refs/heads/main",...}'}
					></textarea>
				</div>

				<button
					class="btn-primary"
					onclick={handlePreview}
					disabled={previewLoading || !selectedTemplateId || !previewPayload.trim()}
				>
					{previewLoading ? 'Previewing...' : 'Preview Message'}
				</button>

				{#if previewError}
					<div class="mt-3 rounded bg-red-500/10 p-3 text-sm text-red-400">{previewError}</div>
				{/if}

				{#if previewResult}
					<div class="mt-3 rounded-lg bg-bg-primary p-4">
						<p class="mb-1 text-2xs font-bold uppercase tracking-wide text-text-muted">Preview Output</p>
						<div class="whitespace-pre-wrap text-sm text-text-primary">{previewResult.content}</div>
					</div>
				{/if}
			</div>
		</div>

	<!-- LIST TAB -->
	{:else}
		{#if webhooks.length === 0}
			<p class="text-sm text-text-muted">No webhooks yet. Create one to get started.</p>
		{:else}
			<div class="space-y-3">
				{#each webhooks as wh (wh.id)}
					<div class="rounded-lg bg-bg-secondary p-4">
						<!-- Webhook Header -->
						{#if editingWebhookId === wh.id}
							<!-- Edit Mode -->
							<div class="space-y-3">
								<div>
									<label class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">Name</label>
									<input type="text" class="input w-full" bind:value={editWebhookName} maxlength="80" />
								</div>
								<div>
									<label class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">Channel</label>
									{#if channels.length > 0}
										<select class="input w-full" bind:value={editWebhookChannel}>
											{#each channels.filter((c) => c.channel_type === 'text' || c.channel_type === 'announcement') as ch (ch.id)}
												<option value={ch.id}>#{ch.name}</option>
											{/each}
										</select>
									{:else}
										<input type="text" class="input w-full" bind:value={editWebhookChannel} />
									{/if}
								</div>

								{#if editWebhookType === 'outgoing'}
									<div>
										<label class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">Outgoing URL</label>
										<input type="url" class="input w-full" bind:value={editOutgoingUrl} placeholder="https://example.com/webhook" />
									</div>
									<div>
										<label class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">Events</label>
										<div class="mt-1 grid grid-cols-2 gap-1.5">
											{#each Object.entries(eventLabels) as [event, label] (event)}
												<label class="flex items-center gap-2 rounded px-2 py-1 text-xs text-text-secondary hover:bg-bg-modifier">
													<input
														type="checkbox"
														checked={editOutgoingEvents.includes(event)}
														onchange={() => (editOutgoingEvents = toggleOutgoingEvent(event, editOutgoingEvents))}
														class="accent-brand-500"
													/>
													{label}
												</label>
											{/each}
										</div>
									</div>
								{/if}

								<div class="flex gap-2">
									<button class="btn-primary text-xs" onclick={handleSaveEdit} disabled={savingEdit || !editWebhookName.trim()}>
										{savingEdit ? 'Saving...' : 'Save'}
									</button>
									<button class="btn-secondary text-xs" onclick={cancelEditing}>Cancel</button>
								</div>
							</div>
						{:else}
							<!-- Display Mode -->
							<div class="flex items-start justify-between">
								<div>
									<div class="flex items-center gap-2">
										<span class="text-sm font-medium text-text-primary">{wh.name}</span>
										<span class="rounded bg-bg-modifier px-1.5 py-0.5 text-2xs font-medium {wh.webhook_type === 'outgoing' ? 'text-yellow-400' : 'text-green-400'}">
											{wh.webhook_type}
										</span>
									</div>
									<p class="mt-0.5 text-xs text-text-muted">
										Channel: #{channelName(wh.channel_id)}
									</p>
									{#if wh.webhook_type === 'outgoing' && wh.outgoing_url}
										<p class="mt-0.5 text-xs text-text-muted">
											URL: {wh.outgoing_url}
										</p>
									{/if}
								</div>
								<div class="flex items-center gap-2">
									{#if wh.webhook_type === 'incoming'}
										<button
											class="text-xs text-brand-400 hover:text-brand-300"
											onclick={() => copyWebhookUrl(wh)}
										>
											Copy URL
										</button>
									{/if}
									<button
										class="text-xs text-brand-400 hover:text-brand-300"
										onclick={() => loadExecutionLogs(wh.id)}
									>
										Logs
									</button>
									<button
										class="text-xs text-text-muted hover:text-text-secondary"
										onclick={() => startEditing(wh)}
									>
										Edit
									</button>
									<button
										class="text-xs text-red-400 hover:text-red-300"
										onclick={() => handleDeleteWebhook(wh.id)}
									>
										Delete
									</button>
								</div>
							</div>

							{#if wh.webhook_type === 'incoming'}
								<div class="mt-2 rounded bg-bg-primary p-2">
									<code class="break-all text-2xs text-text-muted">
										{window.location.origin}/api/v1/webhooks/{wh.id}/{wh.token}
									</code>
								</div>
							{/if}

							<!-- Execution Logs (inline) -->
							{#if selectedWebhookForLogs === wh.id}
								<div class="mt-3 border-t border-border-primary pt-3">
									<div class="mb-2 flex items-center justify-between">
										<h4 class="text-xs font-semibold text-text-primary">Execution Logs</h4>
										<button
											class="text-2xs text-text-muted hover:text-text-secondary"
											onclick={() => { selectedWebhookForLogs = null; executionLogs = []; }}
										>
											Close
										</button>
									</div>

									{#if loadingLogs}
										<p class="text-xs text-text-muted">Loading logs...</p>
									{:else if executionLogs.length === 0}
										<p class="text-xs text-text-muted">No execution logs yet.</p>
									{:else}
										<div class="max-h-80 space-y-1.5 overflow-y-auto">
											{#each executionLogs as log (log.id)}
												<button
													class="w-full rounded bg-bg-primary p-2 text-left hover:bg-bg-modifier"
													onclick={() => (expandedLogId = expandedLogId === log.id ? null : log.id)}
												>
													<div class="flex items-center justify-between">
														<div class="flex items-center gap-2">
															<span class="inline-block h-2 w-2 rounded-full {log.success ? 'bg-green-400' : 'bg-red-400'}"></span>
															<span class="text-xs text-text-primary">
																{log.status_code > 0 ? `HTTP ${log.status_code}` : 'Failed'}
															</span>
															{#if log.error_message}
																<span class="text-2xs text-red-400">{log.error_message}</span>
															{/if}
														</div>
														<span class="text-2xs text-text-muted">{formatRelativeTime(log.created_at)}</span>
													</div>

													{#if expandedLogId === log.id}
														<div class="mt-2 space-y-2">
															{#if log.request_body}
																<div>
																	<p class="mb-0.5 text-2xs font-bold uppercase tracking-wide text-text-muted">Request Body</p>
																	<pre class="max-h-32 overflow-auto rounded bg-bg-secondary p-2 text-2xs text-text-muted">{log.request_body}</pre>
																</div>
															{/if}
															{#if log.response_preview}
																<div>
																	<p class="mb-0.5 text-2xs font-bold uppercase tracking-wide text-text-muted">Response</p>
																	<pre class="max-h-32 overflow-auto rounded bg-bg-secondary p-2 text-2xs text-text-muted">{log.response_preview}</pre>
																</div>
															{/if}
															<p class="text-2xs text-text-muted">{formatDate(log.created_at)}</p>
														</div>
													{/if}
												</button>
											{/each}
										</div>
									{/if}
								</div>
							{/if}
						{/if}
					</div>
				{/each}
			</div>
		{/if}
	{/if}
</div>
