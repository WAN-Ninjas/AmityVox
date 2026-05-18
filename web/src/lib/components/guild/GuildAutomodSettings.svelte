<script lang="ts">
	import { api } from '$lib/api/client';
	import { confirmAction } from '$lib/stores/confirm';
	import { addToast } from '$lib/stores/toast';
	import { createAsyncOp } from '$lib/utils/asyncOp';
	import { getErrorMessage } from '$lib/utils/apiError';
	import type { AutoModAction, AutoModRule, Channel, Role } from '$lib/types';

	interface Props {
		guildId: string;
	}

	type AutoModRuleType = AutoModRule['rule_type'];
	type AutoModRuleAction = AutoModRule['action'];

	let { guildId }: Props = $props();

	let automodRules = $state<AutoModRule[]>([]);
	let automodActions = $state<AutoModAction[]>([]);
	let automodGuildRoles = $state<Role[]>([]);
	let automodGuildChannels = $state<Channel[]>([]);
	let loadedGuildId = $state<string | null>(null);

	let newRuleType = $state<AutoModRuleType>('word_filter');
	let newRuleName = $state('');
	let newRuleAction = $state<AutoModRuleAction>('delete');
	let newRuleEnabled = $state(true);
	let newRuleExemptRoles = $state<string[]>([]);
	let newRuleExemptChannels = $state<string[]>([]);

	let editingExemptRuleId = $state<string | null>(null);
	let editingExemptRoles = $state<string[]>([]);
	let editingExemptChannels = $state<string[]>([]);

	let testRuleType = $state<AutoModRuleType>('word_filter');
	let testRuleConfigText = $state('');
	let testSampleText = $state('');
	let testResult = $state<{ matched: boolean; matched_content: string | null } | null>(null);
	let testError = $state('');
	let loadOp = $state(createAsyncOp());
	let createOp = $state(createAsyncOp());
	let testOp = $state(createAsyncOp());

	const textChannels = $derived(
		automodGuildChannels.filter((channel) =>
			channel.channel_type === 'text' ||
			channel.channel_type === 'announcement' ||
			channel.channel_type === 'forum'
		)
	);

	$effect(() => {
		if (guildId && loadedGuildId !== guildId && !loadOp.loading) {
			loadAutomod();
		}
	});

	async function loadAutomod() {
		await loadOp.run(async () => {
			const [rules, actions, roles, channels] = await Promise.all([
				api.getAutoModRules(guildId),
				api.getAutoModActions(guildId),
				api.getRoles(guildId),
				api.getGuildChannels(guildId)
			]);
			automodRules = rules;
			automodActions = actions;
			automodGuildRoles = roles;
			automodGuildChannels = channels;
			loadedGuildId = guildId;
		}, msg => addToast(msg, 'error'), 'Failed to load AutoMod settings');
	}

	async function handleCreateAutomodRule() {
		if (!newRuleName.trim()) return;
		await createOp.run(async () => {
			const rule = await api.createAutoModRule(guildId, {
				name: newRuleName.trim(),
				rule_type: newRuleType,
				action: newRuleAction,
				enabled: newRuleEnabled,
				config: {},
				exempt_roles: newRuleExemptRoles,
				exempt_channels: newRuleExemptChannels,
				timeout_duration: 0
			});
			automodRules = [...automodRules, rule];
			newRuleName = '';
			newRuleExemptRoles = [];
			newRuleExemptChannels = [];
			addToast('AutoMod rule created', 'success');
		}, msg => addToast(msg, 'error'), 'Failed to create AutoMod rule');
	}

	async function handleToggleAutomodRule(rule: AutoModRule) {
		try {
			const updated = await api.updateAutoModRule(guildId, rule.id, { enabled: !rule.enabled });
			automodRules = automodRules.map((candidate) => candidate.id === rule.id ? updated : candidate);
		} catch (err: unknown) {
			addToast(getErrorMessage(err, 'Failed to update rule'), 'error');
		}
	}

	async function handleDeleteAutomodRule(ruleId: string) {
		if (!(await confirmAction({ title: 'Delete AutoMod Rule', message: 'Delete this AutoMod rule?', confirmLabel: 'Delete' }))) return;
		try {
			await api.deleteAutoModRule(guildId, ruleId);
			automodRules = automodRules.filter((rule) => rule.id !== ruleId);
			if (editingExemptRuleId === ruleId) cancelEditingExemptions();
			addToast('AutoMod rule deleted', 'success');
		} catch (err: unknown) {
			addToast(getErrorMessage(err, 'Failed to delete rule'), 'error');
		}
	}

	function startEditingExemptions(rule: AutoModRule) {
		editingExemptRuleId = rule.id;
		editingExemptRoles = [...(rule.exempt_roles ?? [])];
		editingExemptChannels = [...(rule.exempt_channels ?? [])];
	}

	function cancelEditingExemptions() {
		editingExemptRuleId = null;
		editingExemptRoles = [];
		editingExemptChannels = [];
	}

	async function handleSaveExemptions() {
		if (!editingExemptRuleId) return;
		try {
			const updated = await api.updateAutoModRule(guildId, editingExemptRuleId, {
				exempt_roles: editingExemptRoles,
				exempt_channels: editingExemptChannels
			});
			automodRules = automodRules.map((rule) => rule.id === editingExemptRuleId ? updated : rule);
			cancelEditingExemptions();
			addToast('Exemptions updated', 'success');
		} catch (err: unknown) {
			addToast(getErrorMessage(err, 'Failed to update exemptions'), 'error');
		}
	}

	function toggleArrayItem(items: string[], item: string): string[] {
		return items.includes(item) ? items.filter((candidate) => candidate !== item) : [...items, item];
	}

	function getRoleName(roleId: string): string {
		const role = automodGuildRoles.find((candidate) => candidate.id === roleId);
		return role?.name ?? roleId.slice(0, 8) + '...';
	}

	function getChannelName(channelId: string): string {
		const channel = automodGuildChannels.find((candidate) => candidate.id === channelId);
		return channel?.name ?? channelId.slice(0, 8) + '...';
	}

	function buildTestConfig(): Record<string, unknown> {
		const text = testRuleConfigText.trim();
		if (!text) return {};
		switch (testRuleType) {
			case 'word_filter':
				return { words: text.split(',').map((word) => word.trim()).filter(Boolean) };
			case 'regex_filter':
				return { patterns: text.split(',').map((pattern) => pattern.trim()).filter(Boolean) };
			case 'mention_spam': {
				const maxMentions = parseInt(text, 10);
				return { max_mentions: isNaN(maxMentions) ? 5 : maxMentions };
			}
			case 'caps_filter': {
				const maxCapsPercent = parseInt(text, 10);
				return { max_caps_percent: isNaN(maxCapsPercent) ? 70 : maxCapsPercent };
			}
			case 'link_filter':
				return { blocked_domains: text.split(',').map((domain) => domain.trim()).filter(Boolean) };
			case 'invite_filter':
			default:
				return {};
		}
	}

	async function handleTestAutoModRule() {
		if (!testSampleText.trim()) return;
		testResult = null;
		testError = '';
		await testOp.run(async () => {
			testResult = await api.testAutoModRule(guildId, {
				rule_type: testRuleType,
				config: buildTestConfig(),
				sample_text: testSampleText.trim()
			});
		}, msg => {
			testError = msg;
		}, 'Failed to test rule');
	}

	function formatDate(iso: string): string {
		return new Date(iso).toLocaleString();
	}
</script>

<h1 class="mb-6 text-xl font-bold text-text-primary">AutoMod Rules</h1>

<div class="mb-6 rounded-lg bg-bg-secondary p-4">
	<h3 class="mb-3 text-sm font-semibold text-text-primary">Create Rule</h3>
	<div class="mb-3 grid grid-cols-2 gap-3">
		<div>
			<label for="newRuleName" class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">Rule Name</label>
			<input id="newRuleName" type="text" class="input w-full" bind:value={newRuleName} placeholder="My rule" maxlength="100" />
		</div>
		<div>
			<label for="newRuleType" class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">Type</label>
			<select id="newRuleType" class="input w-full" bind:value={newRuleType}>
				<option value="word_filter">Word Filter</option>
				<option value="regex_filter">Regex Filter</option>
				<option value="invite_filter">Invite Links</option>
				<option value="mention_spam">Mention Spam</option>
				<option value="caps_filter">Caps Filter</option>
				<option value="spam_filter">Spam Filter</option>
				<option value="link_filter">Link Filter</option>
			</select>
		</div>
		<div>
			<label for="newRuleAction" class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">Action</label>
			<select id="newRuleAction" class="input w-full" bind:value={newRuleAction}>
				<option value="delete">Delete Message</option>
				<option value="warn">Warn User</option>
				<option value="timeout">Timeout User</option>
				<option value="log">Log Only</option>
			</select>
		</div>
		<div class="flex items-end">
			<label class="flex items-center gap-2 text-sm text-text-muted">
				<input type="checkbox" bind:checked={newRuleEnabled} class="rounded" />
				Enabled
			</label>
		</div>
	</div>

	<div class="mb-3">
		<div class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">Exempt Roles</div>
		<p class="mb-1.5 text-xs text-text-muted">Members with these roles will not be affected by this rule.</p>
		{#if loadOp.loading}
			<p class="text-xs text-text-muted">Loading roles...</p>
		{:else if automodGuildRoles.length === 0}
			<p class="text-xs text-text-muted">No roles available.</p>
		{:else}
			<div class="flex flex-wrap gap-1.5">
				{#each automodGuildRoles as role (role.id)}
					<button
						type="button"
						class="flex items-center gap-1.5 rounded-full px-2.5 py-1 text-xs transition-colors {newRuleExemptRoles.includes(role.id) ? 'bg-brand-500/20 text-brand-400 ring-1 ring-brand-500/40' : 'bg-bg-modifier text-text-muted hover:bg-bg-tertiary hover:text-text-secondary'}"
						onclick={() => { newRuleExemptRoles = toggleArrayItem(newRuleExemptRoles, role.id); }}
					>
						<span class="h-2 w-2 rounded-full" style="background-color: {role.color ?? '#99aab5'}"></span>
						{role.name}
						{#if newRuleExemptRoles.includes(role.id)}
							<span aria-hidden="true">Selected</span>
						{/if}
					</button>
				{/each}
			</div>
		{/if}
	</div>

	<div class="mb-3">
		<div class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">Exempt Channels</div>
		<p class="mb-1.5 text-xs text-text-muted">Messages in these channels will not be checked by this rule.</p>
		{#if loadOp.loading}
			<p class="text-xs text-text-muted">Loading channels...</p>
		{:else if textChannels.length === 0}
			<p class="text-xs text-text-muted">No channels available.</p>
		{:else}
			<div class="flex flex-wrap gap-1.5">
				{#each textChannels as channel (channel.id)}
					<button
						type="button"
						class="flex items-center gap-1.5 rounded-full px-2.5 py-1 text-xs transition-colors {newRuleExemptChannels.includes(channel.id) ? 'bg-brand-500/20 text-brand-400 ring-1 ring-brand-500/40' : 'bg-bg-modifier text-text-muted hover:bg-bg-tertiary hover:text-text-secondary'}"
						onclick={() => { newRuleExemptChannels = toggleArrayItem(newRuleExemptChannels, channel.id); }}
					>
						<span class="text-text-muted">#</span>
						{channel.name ?? 'unnamed'}
						{#if newRuleExemptChannels.includes(channel.id)}
							<span aria-hidden="true">Selected</span>
						{/if}
					</button>
				{/each}
			</div>
		{/if}
	</div>

	<button class="btn-primary" onclick={handleCreateAutomodRule} disabled={createOp.loading || !newRuleName.trim()}>
		{createOp.loading ? 'Creating...' : 'Create Rule'}
	</button>
</div>

{#if loadOp.loading}
	<p class="text-sm text-text-muted">Loading AutoMod rules...</p>
{:else if automodRules.length === 0}
	<p class="text-sm text-text-muted">No AutoMod rules configured.</p>
{:else}
	<div class="space-y-2">
		{#each automodRules as rule (rule.id)}
			<div class="rounded-lg bg-bg-secondary p-3">
				<div class="flex items-center justify-between gap-3">
					<div class="flex items-center gap-3">
						<button
							class="h-4 w-4 rounded border {rule.enabled ? 'border-green-500 bg-green-500' : 'border-text-muted'}"
							onclick={() => handleToggleAutomodRule(rule)}
							title={rule.enabled ? 'Disable' : 'Enable'}
						></button>
						<div>
							<span class="text-sm font-medium text-text-primary">{rule.name}</span>
							<div class="flex gap-2 text-xs text-text-muted">
								<span class="rounded bg-bg-modifier px-1.5 py-0.5">{rule.rule_type.replace('_', ' ')}</span>
								<span class="rounded bg-bg-modifier px-1.5 py-0.5">{rule.action}</span>
							</div>
						</div>
					</div>
					<div class="flex items-center gap-2">
						<button class="text-xs text-brand-400 hover:text-brand-300" onclick={() => startEditingExemptions(rule)}>
							{editingExemptRuleId === rule.id ? 'Editing...' : 'Exemptions'}
						</button>
						<button class="text-xs text-red-400 hover:text-red-300" onclick={() => handleDeleteAutomodRule(rule.id)}>
							Delete
						</button>
					</div>
				</div>

				{#if editingExemptRuleId !== rule.id && ((rule.exempt_roles && rule.exempt_roles.length > 0) || (rule.exempt_channels && rule.exempt_channels.length > 0))}
					<div class="mt-2 border-t border-bg-modifier pt-2">
						{#if rule.exempt_roles && rule.exempt_roles.length > 0}
							<div class="mb-1 flex flex-wrap items-center gap-1">
								<span class="text-xs text-text-muted">Exempt roles:</span>
								{#each rule.exempt_roles as roleId}
									<span class="rounded-full bg-bg-modifier px-2 py-0.5 text-xs text-text-secondary">{getRoleName(roleId)}</span>
								{/each}
							</div>
						{/if}
						{#if rule.exempt_channels && rule.exempt_channels.length > 0}
							<div class="flex flex-wrap items-center gap-1">
								<span class="text-xs text-text-muted">Exempt channels:</span>
								{#each rule.exempt_channels as channelId}
									<span class="rounded-full bg-bg-modifier px-2 py-0.5 text-xs text-text-secondary">#{getChannelName(channelId)}</span>
								{/each}
							</div>
						{/if}
					</div>
				{/if}

				{#if editingExemptRuleId === rule.id}
					<div class="mt-3 border-t border-bg-modifier pt-3">
						<div class="mb-3">
							<div class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">Exempt Roles</div>
							{#if automodGuildRoles.length === 0}
								<p class="text-xs text-text-muted">No roles available.</p>
							{:else}
								<div class="flex flex-wrap gap-1.5">
									{#each automodGuildRoles as role (role.id)}
										<button
											type="button"
											class="flex items-center gap-1.5 rounded-full px-2.5 py-1 text-xs transition-colors {editingExemptRoles.includes(role.id) ? 'bg-brand-500/20 text-brand-400 ring-1 ring-brand-500/40' : 'bg-bg-modifier text-text-muted hover:bg-bg-tertiary hover:text-text-secondary'}"
											onclick={() => { editingExemptRoles = toggleArrayItem(editingExemptRoles, role.id); }}
										>
											<span class="h-2 w-2 rounded-full" style="background-color: {role.color ?? '#99aab5'}"></span>
											{role.name}
											{#if editingExemptRoles.includes(role.id)}
												<span aria-hidden="true">Selected</span>
											{/if}
										</button>
									{/each}
								</div>
							{/if}
						</div>

						<div class="mb-3">
							<div class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">Exempt Channels</div>
							{#if textChannels.length === 0}
								<p class="text-xs text-text-muted">No channels available.</p>
							{:else}
								<div class="flex flex-wrap gap-1.5">
									{#each textChannels as channel (channel.id)}
										<button
											type="button"
											class="flex items-center gap-1.5 rounded-full px-2.5 py-1 text-xs transition-colors {editingExemptChannels.includes(channel.id) ? 'bg-brand-500/20 text-brand-400 ring-1 ring-brand-500/40' : 'bg-bg-modifier text-text-muted hover:bg-bg-tertiary hover:text-text-secondary'}"
											onclick={() => { editingExemptChannels = toggleArrayItem(editingExemptChannels, channel.id); }}
										>
											<span class="text-text-muted">#</span>
											{channel.name ?? 'unnamed'}
											{#if editingExemptChannels.includes(channel.id)}
												<span aria-hidden="true">Selected</span>
											{/if}
										</button>
									{/each}
								</div>
							{/if}
						</div>

						<div class="flex gap-2">
							<button class="btn-primary text-xs" onclick={handleSaveExemptions}>Save Exemptions</button>
							<button class="btn-secondary text-xs" onclick={cancelEditingExemptions}>Cancel</button>
						</div>
					</div>
				{/if}
			</div>
		{/each}
	</div>
{/if}

{#if automodActions.length > 0}
	<h2 class="mb-3 mt-8 text-lg font-semibold text-text-primary">Recent Actions</h2>
	<div class="space-y-2">
		{#each automodActions.slice(0, 20) as action (action.id)}
			<div class="rounded-lg bg-bg-secondary p-3">
				<div class="flex items-center justify-between">
					<div>
						<span class="text-sm text-text-primary">{action.rule_name}</span>
						<span class="ml-2 text-xs text-text-muted">({action.action_taken})</span>
					</div>
					<span class="text-xs text-text-muted">{formatDate(action.created_at)}</span>
				</div>
				{#if action.matched_content}
					<p class="mt-1 truncate text-xs text-text-muted">Matched: {action.matched_content}</p>
				{/if}
			</div>
		{/each}
	</div>
{/if}

<div class="mt-8 rounded-lg bg-bg-secondary p-4">
	<h2 class="mb-3 text-lg font-semibold text-text-primary">Test Rule</h2>
	<p class="mb-3 text-xs text-text-muted">Preview what messages would match a rule before enabling it. Enter your rule configuration and sample text to test.</p>

	<div class="mb-3 grid grid-cols-2 gap-3">
		<div>
			<label for="testRuleType" class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">Rule Type</label>
			<select id="testRuleType" class="input w-full" bind:value={testRuleType}>
				<option value="word_filter">Word Filter</option>
				<option value="regex_filter">Regex Filter</option>
				<option value="invite_filter">Invite Links</option>
				<option value="mention_spam">Mention Spam</option>
				<option value="caps_filter">Caps Filter</option>
				<option value="link_filter">Link Filter</option>
			</select>
		</div>
		<div>
			<label for="testRuleConfigText" class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">
				{#if testRuleType === 'word_filter'}
					Blocked Words (comma-separated)
				{:else if testRuleType === 'regex_filter'}
					Patterns (comma-separated)
				{:else if testRuleType === 'mention_spam'}
					Max Mentions (number)
				{:else if testRuleType === 'caps_filter'}
					Max Caps Percent (number)
				{:else if testRuleType === 'link_filter'}
					Blocked Domains (comma-separated)
				{:else}
					Config (not needed)
				{/if}
			</label>
			<input
				id="testRuleConfigText"
				type="text"
				class="input w-full"
				bind:value={testRuleConfigText}
				placeholder={
					testRuleType === 'word_filter' ? 'spam, badword, test' :
					testRuleType === 'regex_filter' ? '\\btest\\b, spam\\d+' :
					testRuleType === 'mention_spam' ? '5' :
					testRuleType === 'caps_filter' ? '70' :
					testRuleType === 'link_filter' ? 'evil.com, spam.net' :
					'N/A'
				}
				disabled={testRuleType === 'invite_filter'}
			/>
		</div>
	</div>

	<div class="mb-3">
		<label for="testSampleText" class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">Sample Text</label>
		<textarea
			id="testSampleText"
			class="input w-full resize-y"
			bind:value={testSampleText}
			rows="3"
			placeholder="Enter a sample message to test against the rule..."
		></textarea>
	</div>

	<button class="btn-primary" onclick={handleTestAutoModRule} disabled={testOp.loading || !testSampleText.trim()}>
		{testOp.loading ? 'Testing...' : 'Test Rule'}
	</button>

	{#if testError}
		<div class="mt-3 rounded border border-red-500/30 bg-red-500/10 p-3 text-sm text-red-400">
			{testError}
		</div>
	{/if}

	{#if testResult}
		<div class="mt-3 rounded border p-3 text-sm {testResult.matched ? 'border-red-500/30 bg-red-500/10' : 'border-green-500/30 bg-green-500/10'}">
			{#if testResult.matched}
				<p class="font-semibold text-red-400">Matched</p>
				{#if testResult.matched_content}
					<p class="mt-0.5 text-xs text-red-300/80">Reason: {testResult.matched_content}</p>
				{/if}
			{:else}
				<p class="font-semibold text-green-400">No match -- this text would not trigger the rule.</p>
			{/if}
		</div>
	{/if}
</div>
