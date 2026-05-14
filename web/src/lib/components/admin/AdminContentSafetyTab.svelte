<script lang="ts">
	import { onMount } from 'svelte';
	import { api, type ContentScanLogEntry, type ContentScanRule } from '$lib/api/client';
	import { addToast } from '$lib/stores/toast';
	import { confirmAction } from '$lib/stores/confirm';
	import Modal from '$components/common/Modal.svelte';

	let contentScanRules = $state<ContentScanRule[]>([]);
	let contentScanLog = $state<ContentScanLogEntry[]>([]);
	let loadingContentRules = $state(false);
	let loadingContentLog = $state(false);
	let createRuleModalOpen = $state(false);
	let editRuleModalOpen = $state(false);
	let editingRule = $state<ContentScanRule | null>(null);
	let ruleName = $state('');
	let rulePattern = $state('');
	let ruleAction = $state<'block' | 'flag' | 'log'>('log');
	let ruleTarget = $state<'filename' | 'content_type' | 'text_content'>('filename');
	let ruleEnabled = $state(true);
	let savingRule = $state(false);
	let contentLogSubTab = $state<'rules' | 'log'>('rules');

	onMount(() => {
		loadContentScanRules();
	});

	async function loadContentScanRules() {
		loadingContentRules = true;
		try {
			contentScanRules = await api.getContentScanRules();
		} catch {
			contentScanRules = [];
		} finally {
			loadingContentRules = false;
		}
	}

	async function loadContentScanLog() {
		loadingContentLog = true;
		try {
			contentScanLog = await api.getContentScanLog({ limit: 50 });
		} catch {
			contentScanLog = [];
		} finally {
			loadingContentLog = false;
		}
	}

	function openCreateRuleModal() {
		ruleName = '';
		rulePattern = '';
		ruleAction = 'log';
		ruleTarget = 'filename';
		ruleEnabled = true;
		createRuleModalOpen = true;
	}

	function openEditRuleModal(rule: ContentScanRule) {
		editingRule = rule;
		ruleName = rule.name;
		rulePattern = rule.pattern;
		ruleAction = rule.action as 'block' | 'flag' | 'log';
		ruleTarget = rule.target as 'filename' | 'content_type' | 'text_content';
		ruleEnabled = rule.enabled;
		editRuleModalOpen = true;
	}

	async function handleCreateRule() {
		if (!ruleName.trim() || !rulePattern.trim()) return;
		savingRule = true;
		try {
			const rule = await api.createContentScanRule({
				name: ruleName.trim(),
				pattern: rulePattern.trim(),
				action: ruleAction,
				target: ruleTarget,
				enabled: ruleEnabled
			});
			contentScanRules = [rule, ...contentScanRules];
			createRuleModalOpen = false;
			addToast('Content scan rule created', 'success');
		} catch (err: any) {
			addToast(err.message ?? 'Failed to create rule', 'error');
		} finally {
			savingRule = false;
		}
	}

	async function handleUpdateRule() {
		if (!editingRule) return;
		savingRule = true;
		try {
			const updated = await api.updateContentScanRule(editingRule.id, {
				name: ruleName.trim(),
				pattern: rulePattern.trim(),
				action: ruleAction,
				target: ruleTarget,
				enabled: ruleEnabled
			});
			contentScanRules = contentScanRules.map((rule) => rule.id === editingRule?.id ? updated : rule);
			editRuleModalOpen = false;
			editingRule = null;
			addToast('Content scan rule updated', 'success');
		} catch (err: any) {
			addToast(err.message ?? 'Failed to update rule', 'error');
		} finally {
			savingRule = false;
		}
	}

	async function handleToggleRule(rule: ContentScanRule) {
		try {
			await api.updateContentScanRule(rule.id, { enabled: !rule.enabled });
			contentScanRules = contentScanRules.map((value) => value.id === rule.id ? { ...value, enabled: !value.enabled } : value);
			addToast(rule.enabled ? 'Rule disabled' : 'Rule enabled', 'success');
		} catch {
			addToast('Failed to toggle rule', 'error');
		}
	}

	async function handleDeleteRule(id: string) {
		if (!(await confirmAction({ title: 'Delete Content Scan Rule', message: 'Delete this content scan rule? Associated log entries will also be deleted.', confirmLabel: 'Delete' }))) return;
		try {
			await api.deleteContentScanRule(id);
			contentScanRules = contentScanRules.filter((rule) => rule.id !== id);
			addToast('Rule deleted', 'success');
		} catch {
			addToast('Failed to delete rule', 'error');
		}
	}

	function actionClasses(action: string): string {
		switch (action) {
			case 'block': return 'bg-red-500/20 text-red-400';
			case 'flag': return 'bg-yellow-500/20 text-yellow-400';
			case 'log': return 'bg-blue-500/20 text-blue-400';
			default: return 'bg-gray-500/20 text-gray-400';
		}
	}

	function targetLabel(target: string): string {
		switch (target) {
			case 'filename': return 'Filename';
			case 'content_type': return 'Content Type';
			case 'text_content': return 'Text Content';
			default: return target;
		}
	}
</script>

<div class="mb-6 flex items-center justify-between">
	<h1 class="text-2xl font-bold text-text-primary">Content Safety</h1>
	<div class="flex gap-2">
		<button
			class="text-sm {contentLogSubTab === 'rules' ? 'btn-primary' : 'btn-secondary'}"
			onclick={() => (contentLogSubTab = 'rules')}
		>
			Rules
		</button>
		<button
			class="text-sm {contentLogSubTab === 'log' ? 'btn-primary' : 'btn-secondary'}"
			onclick={() => { contentLogSubTab = 'log'; if (contentScanLog.length === 0) loadContentScanLog(); }}
		>
			Scan Log
		</button>
	</div>
</div>

{#if contentLogSubTab === 'rules'}
	<div class="mb-4 flex items-center justify-between">
		<p class="text-sm text-text-muted">
			Define regex patterns to scan uploads and messages. Matched content can be blocked, flagged, or logged.
		</p>
		<button class="btn-primary text-sm" onclick={openCreateRuleModal}>
			Create Rule
		</button>
	</div>

	{#if loadingContentRules}
		<p class="text-sm text-text-muted">Loading content scan rules...</p>
	{:else if contentScanRules.length === 0}
		<div class="rounded-lg bg-bg-secondary p-6 text-center">
			<p class="text-sm text-text-muted">No content scan rules configured.</p>
			<p class="mt-1 text-xs text-text-muted">Create a rule to start scanning uploads and messages.</p>
		</div>
	{:else}
		<div class="space-y-3">
			{#each contentScanRules as rule (rule.id)}
				<div class="rounded-lg bg-bg-secondary p-4">
					<div class="flex items-start justify-between">
						<div class="flex-1">
							<div class="mb-1 flex items-center gap-2">
								<h3 class="text-sm font-semibold text-text-primary">{rule.name}</h3>
								<span class="rounded px-1.5 py-0.5 text-2xs font-bold {actionClasses(rule.action)}">
									{rule.action}
								</span>
								<span class="rounded bg-bg-modifier px-1.5 py-0.5 text-2xs text-text-muted">
									{targetLabel(rule.target)}
								</span>
								<span class="rounded px-1.5 py-0.5 text-2xs font-bold {rule.enabled ? 'bg-green-500/20 text-green-400' : 'bg-gray-500/20 text-gray-400'}">
									{rule.enabled ? 'Enabled' : 'Disabled'}
								</span>
							</div>
							<code class="text-xs text-text-muted">{rule.pattern}</code>
							<p class="mt-1 text-xs text-text-muted">Created {new Date(rule.created_at).toLocaleString()}</p>
						</div>
						<div class="flex items-center gap-2">
							<button
								class="text-xs text-brand-400 hover:text-brand-300"
								onclick={() => openEditRuleModal(rule)}
							>
								Edit
							</button>
							<button
								class="text-xs {rule.enabled ? 'text-yellow-400 hover:text-yellow-300' : 'text-green-400 hover:text-green-300'}"
								onclick={() => handleToggleRule(rule)}
							>
								{rule.enabled ? 'Disable' : 'Enable'}
							</button>
							<button
								class="text-xs text-red-400 hover:text-red-300"
								onclick={() => handleDeleteRule(rule.id)}
							>
								Delete
							</button>
						</div>
					</div>
				</div>
			{/each}
		</div>
	{/if}
{:else}
	<!-- Scan Log Sub-tab -->
	<div class="mb-4 flex items-center justify-between">
		<p class="text-sm text-text-muted">Recent content scan matches from all rules.</p>
		<button class="btn-secondary text-sm" onclick={loadContentScanLog} disabled={loadingContentLog}>
			{loadingContentLog ? 'Loading...' : 'Refresh'}
		</button>
	</div>

	{#if loadingContentLog}
		<p class="text-sm text-text-muted">Loading scan log...</p>
	{:else if contentScanLog.length === 0}
		<div class="rounded-lg bg-bg-secondary p-6 text-center">
			<p class="text-sm text-text-muted">No content scan matches recorded.</p>
			<p class="mt-1 text-xs text-text-muted">Matches will appear here when content triggers a scan rule.</p>
		</div>
	{:else}
		<div class="overflow-hidden rounded-lg border border-bg-modifier">
			<table class="w-full text-left text-sm">
				<thead class="bg-bg-secondary">
					<tr>
						<th class="px-4 py-3 text-xs font-bold uppercase tracking-wide text-text-muted">Rule</th>
						<th class="px-4 py-3 text-xs font-bold uppercase tracking-wide text-text-muted">User</th>
						<th class="px-4 py-3 text-xs font-bold uppercase tracking-wide text-text-muted">Matched</th>
						<th class="px-4 py-3 text-xs font-bold uppercase tracking-wide text-text-muted">Action</th>
						<th class="px-4 py-3 text-xs font-bold uppercase tracking-wide text-text-muted">Time</th>
					</tr>
				</thead>
				<tbody class="divide-y divide-bg-modifier">
					{#each contentScanLog as entry (entry.id)}
						<tr class="hover:bg-bg-secondary/50">
							<td class="px-4 py-3 text-text-primary text-xs">{entry.rule_name}</td>
							<td class="px-4 py-3 text-text-secondary text-xs">@{entry.username}</td>
							<td class="px-4 py-3">
								<code class="rounded bg-bg-modifier px-1.5 py-0.5 text-xs text-text-muted">{entry.content_matched.length > 50 ? entry.content_matched.slice(0, 50) + '...' : entry.content_matched}</code>
							</td>
							<td class="px-4 py-3">
								<span class="rounded px-1.5 py-0.5 text-2xs font-bold {actionClasses(entry.action_taken)}">
									{entry.action_taken}
								</span>
							</td>
							<td class="px-4 py-3 text-text-muted text-xs">{new Date(entry.created_at).toLocaleString()}</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
{/if}

<!-- Create Content Scan Rule Modal -->
<Modal open={createRuleModalOpen} title="Create Content Scan Rule" onclose={() => (createRuleModalOpen = false)}>
	<div class="space-y-4">
		<div>
			<label for="admin-create-rule-name" class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">Rule Name</label>
			<input id="admin-create-rule-name" type="text" class="input w-full" bind:value={ruleName} maxlength="200" placeholder="e.g., Block executable files" />
		</div>
		<div>
			<label for="admin-create-rule-pattern" class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">Regex Pattern</label>
			<input id="admin-create-rule-pattern" type="text" class="input w-full font-mono text-sm" bind:value={rulePattern} placeholder="e.g., \.(exe|bat|cmd|ps1)$" />
			<p class="mt-1 text-xs text-text-muted">Regular expression pattern to match against the target field.</p>
		</div>
		<div>
			<label for="admin-create-rule-target" class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">Target</label>
			<select id="admin-create-rule-target" class="input w-full" bind:value={ruleTarget}>
				<option value="filename">Filename</option>
				<option value="content_type">Content Type (MIME)</option>
				<option value="text_content">Text Content (message body)</option>
			</select>
		</div>
		<div>
			<p class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">Action</p>
			<div class="flex gap-4">
				<label class="flex items-center gap-2 text-sm text-text-secondary">
					<input type="radio" bind:group={ruleAction} value="log" class="accent-blue-500" />
					Log Only
				</label>
				<label class="flex items-center gap-2 text-sm text-text-secondary">
					<input type="radio" bind:group={ruleAction} value="flag" class="accent-yellow-500" />
					Flag for Review
				</label>
				<label class="flex items-center gap-2 text-sm text-text-secondary">
					<input type="radio" bind:group={ruleAction} value="block" class="accent-red-500" />
					Block Upload
				</label>
			</div>
		</div>
		<div>
			<label class="flex items-center gap-2 text-sm text-text-secondary">
				<input type="checkbox" bind:checked={ruleEnabled} class="accent-brand-500" />
				Enabled
			</label>
		</div>
		<div class="flex justify-end gap-2">
			<button class="btn-secondary text-sm" onclick={() => (createRuleModalOpen = false)}>Cancel</button>
			<button
				class="btn-primary text-sm"
				onclick={handleCreateRule}
				disabled={savingRule || !ruleName.trim() || !rulePattern.trim()}
			>
				{savingRule ? 'Creating...' : 'Create Rule'}
			</button>
		</div>
	</div>
</Modal>

<!-- Edit Content Scan Rule Modal -->
<Modal open={editRuleModalOpen} title="Edit Content Scan Rule" onclose={() => (editRuleModalOpen = false)}>
	<div class="space-y-4">
		<div>
			<label for="admin-edit-rule-name" class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">Rule Name</label>
			<input id="admin-edit-rule-name" type="text" class="input w-full" bind:value={ruleName} maxlength="200" />
		</div>
		<div>
			<label for="admin-edit-rule-pattern" class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">Regex Pattern</label>
			<input id="admin-edit-rule-pattern" type="text" class="input w-full font-mono text-sm" bind:value={rulePattern} />
			<p class="mt-1 text-xs text-text-muted">Regular expression pattern to match against the target field.</p>
		</div>
		<div>
			<label for="admin-edit-rule-target" class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">Target</label>
			<select id="admin-edit-rule-target" class="input w-full" bind:value={ruleTarget}>
				<option value="filename">Filename</option>
				<option value="content_type">Content Type (MIME)</option>
				<option value="text_content">Text Content (message body)</option>
			</select>
		</div>
		<div>
			<p class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">Action</p>
			<div class="flex gap-4">
				<label class="flex items-center gap-2 text-sm text-text-secondary">
					<input type="radio" bind:group={ruleAction} value="log" class="accent-blue-500" />
					Log Only
				</label>
				<label class="flex items-center gap-2 text-sm text-text-secondary">
					<input type="radio" bind:group={ruleAction} value="flag" class="accent-yellow-500" />
					Flag for Review
				</label>
				<label class="flex items-center gap-2 text-sm text-text-secondary">
					<input type="radio" bind:group={ruleAction} value="block" class="accent-red-500" />
					Block Upload
				</label>
			</div>
		</div>
		<div>
			<label class="flex items-center gap-2 text-sm text-text-secondary">
				<input type="checkbox" bind:checked={ruleEnabled} class="accent-brand-500" />
				Enabled
			</label>
		</div>
		<div class="flex justify-end gap-2">
			<button class="btn-secondary text-sm" onclick={() => (editRuleModalOpen = false)}>Cancel</button>
			<button
				class="btn-primary text-sm"
				onclick={handleUpdateRule}
				disabled={savingRule || !ruleName.trim() || !rulePattern.trim()}
			>
				{savingRule ? 'Saving...' : 'Save Changes'}
			</button>
		</div>
	</div>
</Modal>
