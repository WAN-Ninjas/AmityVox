<script lang="ts">
	import { api } from '$lib/api/client';
	import { confirmAction } from '$lib/stores/confirm';
	import { addToast } from '$lib/stores/toast';
	import type { Channel, OnboardingConfig, OnboardingPrompt, Role } from '$lib/types';

	interface Props {
		guildId: string;
	}

	type PromptOptionPayload = {
		label: string;
		description?: string;
		emoji?: string;
		role_ids: string[];
		channel_ids: string[];
	};

	let { guildId }: Props = $props();

	let onboardingConfig = $state<OnboardingConfig | null>(null);
	let onboardingChannels = $state<Channel[]>([]);
	let onboardingRoles = $state<Role[]>([]);
	let loadingOnboarding = $state(false);
	let savingOnboarding = $state(false);
	let loadedGuildId = $state<string | null>(null);

	let newRuleText = $state('');
	let newPromptTitle = $state('');
	let newPromptRequired = $state(false);
	let newPromptSingleSelect = $state(false);
	let creatingPrompt = $state(false);

	let editingPromptId = $state<string | null>(null);
	let editingPromptTitle = $state('');
	let editingPromptRequired = $state(false);
	let editingPromptSingleSelect = $state(false);

	let addingOptionToPromptId = $state<string | null>(null);
	let newOptionLabel = $state('');
	let newOptionDescription = $state('');
	let newOptionEmoji = $state('');
	let newOptionRoleIds = $state<string[]>([]);
	let newOptionChannelIds = $state<string[]>([]);

	const textChannels = $derived(
		onboardingChannels.filter((channel) =>
			channel.channel_type === 'text' || channel.channel_type === 'announcement'
		)
	);

	$effect(() => {
		if (guildId && loadedGuildId !== guildId && !loadingOnboarding) {
			loadOnboarding();
		}
	});

	async function loadOnboarding() {
		loadingOnboarding = true;
		try {
			const [config, channels, roles] = await Promise.all([
				api.getOnboarding(guildId),
				api.getGuildChannels(guildId),
				api.getRoles(guildId)
			]);
			onboardingConfig = config;
			onboardingChannels = channels;
			onboardingRoles = roles;
			loadedGuildId = guildId;
		} catch {
			onboardingConfig = { enabled: false, welcome_message: '', rules: [], default_channel_ids: [], prompts: [] };
			try {
				const [channels, roles] = await Promise.all([
					api.getGuildChannels(guildId),
					api.getRoles(guildId)
				]);
				onboardingChannels = channels;
				onboardingRoles = roles;
				loadedGuildId = guildId;
			} catch (err: any) {
				addToast(err.message || 'Failed to load onboarding metadata', 'error');
			}
		} finally {
			loadingOnboarding = false;
		}
	}

	async function handleSaveOnboarding() {
		if (!onboardingConfig) return;
		savingOnboarding = true;
		try {
			onboardingConfig = await api.updateOnboarding(guildId, {
				enabled: onboardingConfig.enabled,
				welcome_message: onboardingConfig.welcome_message,
				rules: onboardingConfig.rules,
				default_channel_ids: onboardingConfig.default_channel_ids
			});
			addToast('Onboarding settings saved', 'success');
		} catch (err: any) {
			addToast(err.message || 'Failed to save onboarding', 'error');
		} finally {
			savingOnboarding = false;
		}
	}

	function addOnboardingRule() {
		if (!onboardingConfig || !newRuleText.trim()) return;
		onboardingConfig = { ...onboardingConfig, rules: [...onboardingConfig.rules, newRuleText.trim()] };
		newRuleText = '';
	}

	function removeOnboardingRule(index: number) {
		if (!onboardingConfig) return;
		onboardingConfig = { ...onboardingConfig, rules: onboardingConfig.rules.filter((_, ruleIndex) => ruleIndex !== index) };
	}

	function moveOnboardingRule(index: number, direction: 'up' | 'down') {
		if (!onboardingConfig) return;
		const rules = [...onboardingConfig.rules];
		const newIndex = direction === 'up' ? index - 1 : index + 1;
		if (newIndex < 0 || newIndex >= rules.length) return;
		[rules[index], rules[newIndex]] = [rules[newIndex], rules[index]];
		onboardingConfig = { ...onboardingConfig, rules };
	}

	function toggleDefaultChannel(channelId: string) {
		if (!onboardingConfig) return;
		const ids = onboardingConfig.default_channel_ids;
		onboardingConfig = {
			...onboardingConfig,
			default_channel_ids: ids.includes(channelId)
				? ids.filter((id) => id !== channelId)
				: [...ids, channelId]
		};
	}

	async function handleCreatePrompt() {
		if (!newPromptTitle.trim()) return;
		creatingPrompt = true;
		try {
			const prompt = await api.createOnboardingPrompt(guildId, {
				title: newPromptTitle.trim(),
				required: newPromptRequired,
				single_select: newPromptSingleSelect,
				options: []
			});
			if (onboardingConfig) {
				onboardingConfig = { ...onboardingConfig, prompts: [...onboardingConfig.prompts, prompt] };
			}
			newPromptTitle = '';
			newPromptRequired = false;
			newPromptSingleSelect = false;
			addToast('Prompt created', 'success');
		} catch (err: any) {
			addToast(err.message || 'Failed to create prompt', 'error');
		} finally {
			creatingPrompt = false;
		}
	}

	function startEditingPrompt(prompt: OnboardingPrompt) {
		editingPromptId = prompt.id;
		editingPromptTitle = prompt.title;
		editingPromptRequired = prompt.required;
		editingPromptSingleSelect = prompt.single_select;
	}

	function cancelEditingPrompt() {
		editingPromptId = null;
		editingPromptTitle = '';
	}

	async function handleSavePrompt() {
		if (!editingPromptId || !editingPromptTitle.trim()) return;
		try {
			await api.updateOnboardingPrompt(guildId, editingPromptId, {
				title: editingPromptTitle.trim(),
				required: editingPromptRequired,
				single_select: editingPromptSingleSelect
			});
			if (onboardingConfig) {
				onboardingConfig = {
					...onboardingConfig,
					prompts: onboardingConfig.prompts.map((prompt) =>
						prompt.id === editingPromptId
							? { ...prompt, title: editingPromptTitle.trim(), required: editingPromptRequired, single_select: editingPromptSingleSelect }
							: prompt
					)
				};
			}
			cancelEditingPrompt();
			addToast('Prompt updated', 'success');
		} catch (err: any) {
			addToast(err.message || 'Failed to update prompt', 'error');
		}
	}

	async function handleDeletePrompt(promptId: string) {
		if (!(await confirmAction({ title: 'Delete Onboarding Prompt', message: 'Delete this onboarding prompt?', confirmLabel: 'Delete' }))) return;
		try {
			await api.deleteOnboardingPrompt(guildId, promptId);
			if (onboardingConfig) {
				onboardingConfig = { ...onboardingConfig, prompts: onboardingConfig.prompts.filter((prompt) => prompt.id !== promptId) };
			}
			if (editingPromptId === promptId) cancelEditingPrompt();
			if (addingOptionToPromptId === promptId) cancelAddingOption();
			addToast('Prompt deleted', 'success');
		} catch (err: any) {
			addToast(err.message || 'Failed to delete prompt', 'error');
		}
	}

	function startAddingOption(promptId: string) {
		addingOptionToPromptId = promptId;
		newOptionLabel = '';
		newOptionDescription = '';
		newOptionEmoji = '';
		newOptionRoleIds = [];
		newOptionChannelIds = [];
	}

	function cancelAddingOption() {
		addingOptionToPromptId = null;
		newOptionLabel = '';
		newOptionDescription = '';
		newOptionEmoji = '';
		newOptionRoleIds = [];
		newOptionChannelIds = [];
	}

	function optionPayloads(prompt: OnboardingPrompt): PromptOptionPayload[] {
		return prompt.options.map((option) => ({
			label: option.label,
			description: option.description,
			emoji: option.emoji,
			role_ids: option.role_ids,
			channel_ids: option.channel_ids
		}));
	}

	async function handleAddOption() {
		if (!addingOptionToPromptId || !newOptionLabel.trim()) return;
		try {
			const prompt = onboardingConfig?.prompts.find((candidate) => candidate.id === addingOptionToPromptId);
			if (!prompt) return;
			const newOptions: PromptOptionPayload[] = [
				...optionPayloads(prompt),
				{
					label: newOptionLabel.trim(),
					description: newOptionDescription.trim() || undefined,
					emoji: newOptionEmoji.trim() || undefined,
					role_ids: newOptionRoleIds,
					channel_ids: newOptionChannelIds
				}
			];
			await api.updateOnboardingPrompt(guildId, addingOptionToPromptId, { options: newOptions as any });
			await loadOnboarding();
			cancelAddingOption();
			addToast('Option added', 'success');
		} catch (err: any) {
			addToast(err.message || 'Failed to add option', 'error');
		}
	}

	async function handleRemoveOption(promptId: string, optionId: string) {
		if (!onboardingConfig) return;
		try {
			const prompt = onboardingConfig.prompts.find((candidate) => candidate.id === promptId);
			if (!prompt) return;
			const newOptions = optionPayloads(prompt).filter((_, index) => prompt.options[index]?.id !== optionId);
			await api.updateOnboardingPrompt(guildId, promptId, { options: newOptions as any });
			await loadOnboarding();
			addToast('Option removed', 'success');
		} catch (err: any) {
			addToast(err.message || 'Failed to remove option', 'error');
		}
	}

	function toggleRole(roleId: string) {
		newOptionRoleIds = newOptionRoleIds.includes(roleId)
			? newOptionRoleIds.filter((id) => id !== roleId)
			: [...newOptionRoleIds, roleId];
	}

	function toggleChannel(channelId: string) {
		newOptionChannelIds = newOptionChannelIds.includes(channelId)
			? newOptionChannelIds.filter((id) => id !== channelId)
			: [...newOptionChannelIds, channelId];
	}

	function getOnboardingChannelName(channelId: string): string {
		const channel = onboardingChannels.find((candidate) => candidate.id === channelId);
		return channel?.name ?? channelId.slice(0, 8) + '...';
	}

	function getOnboardingRoleName(roleId: string): string {
		const role = onboardingRoles.find((candidate) => candidate.id === roleId);
		return role?.name ?? roleId.slice(0, 8) + '...';
	}
</script>

<h1 class="mb-6 text-xl font-bold text-text-primary">Onboarding</h1>
<p class="mb-6 text-sm text-text-muted">
	Configure the onboarding flow that new members see when they join your server. You can set a welcome message, rules, and custom prompts to personalize their experience.
</p>

{#if loadingOnboarding}
	<p class="text-sm text-text-muted">Loading onboarding configuration...</p>
{:else if onboardingConfig}
	<label class="mb-6 flex items-center gap-3">
		<input type="checkbox" bind:checked={onboardingConfig.enabled} class="rounded" />
		<div>
			<span class="text-sm font-medium text-text-primary">Enable Onboarding</span>
			<p class="text-xs text-text-muted">When enabled, new members will see the onboarding flow after joining</p>
		</div>
	</label>

	<div class="mb-6">
		<label for="onboardingWelcome" class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">Welcome Message</label>
		<textarea
			id="onboardingWelcome"
			bind:value={onboardingConfig.welcome_message}
			class="input w-full"
			rows="3"
			maxlength="2000"
			placeholder="Write a welcome message for new members..."
		></textarea>
	</div>

	<div class="mb-6">
		<div class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">Server Rules</div>
		<p class="mb-2 text-xs text-text-muted">New members must accept these rules during onboarding.</p>

		{#if onboardingConfig.rules.length > 0}
			<div class="mb-3 space-y-2">
				{#each onboardingConfig.rules as rule, index}
					<div class="flex items-center gap-2 rounded-lg bg-bg-primary p-2.5">
						<span class="flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-brand-600/20 text-xs font-bold text-brand-400">
							{index + 1}
						</span>
						<span class="min-w-0 flex-1 text-sm text-text-primary">{rule}</span>
						<div class="flex shrink-0 items-center gap-1">
							<button
								class="text-xs text-text-muted hover:text-text-primary disabled:opacity-40"
								onclick={() => moveOnboardingRule(index, 'up')}
								disabled={index === 0}
							>
								Up
							</button>
							<button
								class="text-xs text-text-muted hover:text-text-primary disabled:opacity-40"
								onclick={() => moveOnboardingRule(index, 'down')}
								disabled={index === onboardingConfig.rules.length - 1}
							>
								Down
							</button>
							<button class="text-xs text-red-400 hover:text-red-300" onclick={() => removeOnboardingRule(index)}>
								Remove
							</button>
						</div>
					</div>
				{/each}
			</div>
		{/if}

		<div class="flex gap-2">
			<input
				type="text"
				class="input flex-1"
				bind:value={newRuleText}
				placeholder="Add a rule..."
				maxlength="500"
				onkeydown={(event) => event.key === 'Enter' && addOnboardingRule()}
			/>
			<button class="btn-primary" onclick={addOnboardingRule} disabled={!newRuleText.trim()}>Add</button>
		</div>
	</div>

	<div class="mb-6">
		<div class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">Default Channels</div>
		<p class="mb-2 text-xs text-text-muted">Channels that new members are automatically added to after onboarding.</p>
		{#if textChannels.length === 0}
			<p class="text-xs text-text-muted">No channels available.</p>
		{:else}
			<div class="flex flex-wrap gap-1.5">
				{#each textChannels as channel (channel.id)}
					<button
						type="button"
						class="flex items-center gap-1.5 rounded-full px-2.5 py-1 text-xs transition-colors {onboardingConfig.default_channel_ids.includes(channel.id)
							? 'bg-brand-500/20 text-brand-400 ring-1 ring-brand-500/40'
							: 'bg-bg-modifier text-text-muted hover:bg-bg-tertiary hover:text-text-secondary'}"
						onclick={() => toggleDefaultChannel(channel.id)}
					>
						<span class="text-text-muted">#</span>
						{channel.name ?? 'unnamed'}
						{#if onboardingConfig.default_channel_ids.includes(channel.id)}
							<span aria-hidden="true">Selected</span>
						{/if}
					</button>
				{/each}
			</div>
		{/if}
	</div>

	<button class="btn-primary mb-8" onclick={handleSaveOnboarding} disabled={savingOnboarding}>
		{savingOnboarding ? 'Saving...' : 'Save Onboarding Settings'}
	</button>

	<div class="border-t border-bg-modifier pt-6">
		<h2 class="mb-2 text-lg font-semibold text-text-primary">Prompts</h2>
		<p class="mb-4 text-sm text-text-muted">
			Prompts let new members customize their experience by choosing roles and channels. Each prompt is shown as a separate step during onboarding.
		</p>

		<div class="mb-6 rounded-lg bg-bg-primary p-4">
			<h3 class="mb-3 text-sm font-semibold text-text-primary">Create Prompt</h3>
			<div class="mb-3">
				<label for="newPromptTitle" class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">Title</label>
				<input
					id="newPromptTitle"
					type="text"
					class="input w-full"
					bind:value={newPromptTitle}
					placeholder="What are you interested in?"
					maxlength="200"
				/>
			</div>
			<div class="mb-3 flex gap-4">
				<label class="flex items-center gap-2 text-sm text-text-muted">
					<input type="checkbox" bind:checked={newPromptRequired} class="rounded" />
					Required
				</label>
				<label class="flex items-center gap-2 text-sm text-text-muted">
					<input type="checkbox" bind:checked={newPromptSingleSelect} class="rounded" />
					Single select
				</label>
			</div>
			<button class="btn-primary" onclick={handleCreatePrompt} disabled={creatingPrompt || !newPromptTitle.trim()}>
				{creatingPrompt ? 'Creating...' : 'Create Prompt'}
			</button>
		</div>

		{#if onboardingConfig.prompts.length === 0}
			<p class="text-sm text-text-muted">No prompts configured yet. Create one above to get started.</p>
		{:else}
			<div class="space-y-4">
				{#each onboardingConfig.prompts.slice().sort((a, b) => a.position - b.position) as prompt (prompt.id)}
					<div class="rounded-lg bg-bg-primary p-4">
						{#if editingPromptId === prompt.id}
							<div class="mb-3">
								<input
									type="text"
									class="input mb-2 w-full"
									bind:value={editingPromptTitle}
									onkeydown={(event) => event.key === 'Enter' && handleSavePrompt()}
								/>
								<div class="mb-2 flex gap-4">
									<label class="flex items-center gap-2 text-sm text-text-muted">
										<input type="checkbox" bind:checked={editingPromptRequired} class="rounded" />
										Required
									</label>
									<label class="flex items-center gap-2 text-sm text-text-muted">
										<input type="checkbox" bind:checked={editingPromptSingleSelect} class="rounded" />
										Single select
									</label>
								</div>
								<div class="flex gap-2">
									<button class="btn-primary text-xs" onclick={handleSavePrompt}>Save</button>
									<button class="btn-secondary text-xs" onclick={cancelEditingPrompt}>Cancel</button>
								</div>
							</div>
						{:else}
							<div class="mb-3 flex items-center justify-between gap-3">
								<div>
									<h3 class="text-sm font-semibold text-text-primary">{prompt.title}</h3>
									<div class="mt-0.5 flex flex-wrap gap-2 text-xs text-text-muted">
										{#if prompt.required}
											<span class="rounded bg-brand-500/15 px-1.5 py-0.5 text-brand-400">Required</span>
										{/if}
										<span class="rounded bg-bg-modifier px-1.5 py-0.5">{prompt.single_select ? 'Single select' : 'Multi select'}</span>
										<span class="rounded bg-bg-modifier px-1.5 py-0.5">Pos: {prompt.position}</span>
									</div>
								</div>
								<div class="flex items-center gap-2">
									<button class="text-xs text-brand-400 hover:text-brand-300" onclick={() => startEditingPrompt(prompt)}>Edit</button>
									<button class="text-xs text-red-400 hover:text-red-300" onclick={() => handleDeletePrompt(prompt.id)}>Delete</button>
								</div>
							</div>
						{/if}

						<div class="mb-2 text-xs font-bold uppercase tracking-wide text-text-muted">Options ({prompt.options.length})</div>
						{#if prompt.options.length > 0}
							<div class="mb-3 space-y-1.5">
								{#each prompt.options as option (option.id)}
									<div class="flex items-center justify-between gap-3 rounded-md bg-bg-secondary p-2.5">
										<div class="min-w-0 flex-1">
											<div class="flex items-center gap-2">
												{#if option.emoji}
													<span>{option.emoji}</span>
												{/if}
												<span class="text-sm font-medium text-text-primary">{option.label}</span>
											</div>
											{#if option.description}
												<p class="mt-0.5 text-xs text-text-muted">{option.description}</p>
											{/if}
											<div class="mt-1 flex flex-wrap gap-1">
												{#each option.role_ids as roleId}
													<span class="rounded-full bg-brand-500/10 px-1.5 py-0.5 text-2xs text-brand-400">
														@{getOnboardingRoleName(roleId)}
													</span>
												{/each}
												{#each option.channel_ids as channelId}
													<span class="rounded-full bg-bg-modifier px-1.5 py-0.5 text-2xs text-text-muted">
														#{getOnboardingChannelName(channelId)}
													</span>
												{/each}
											</div>
										</div>
										<button class="shrink-0 text-xs text-red-400 hover:text-red-300" onclick={() => handleRemoveOption(prompt.id, option.id)}>
											Remove
										</button>
									</div>
								{/each}
							</div>
						{:else}
							<p class="mb-3 text-xs text-text-muted">No options yet. Add one below.</p>
						{/if}

						{#if addingOptionToPromptId === prompt.id}
							<div class="rounded-md border border-bg-modifier bg-bg-secondary p-3">
								<h4 class="mb-2 text-xs font-bold uppercase tracking-wide text-text-muted">New Option</h4>
								<div class="mb-2 grid grid-cols-2 gap-2">
									<div>
										<label for={`newOptionLabel-${prompt.id}`} class="mb-1 block text-xs text-text-muted">Label</label>
										<input id={`newOptionLabel-${prompt.id}`} type="text" class="input w-full" bind:value={newOptionLabel} placeholder="Option label" maxlength="100" />
									</div>
									<div>
										<label for={`newOptionEmoji-${prompt.id}`} class="mb-1 block text-xs text-text-muted">Emoji (optional)</label>
										<input id={`newOptionEmoji-${prompt.id}`} type="text" class="input w-full" bind:value={newOptionEmoji} placeholder="e.g. a single emoji" maxlength="4" />
									</div>
								</div>
								<div class="mb-2">
									<label for={`newOptionDescription-${prompt.id}`} class="mb-1 block text-xs text-text-muted">Description (optional)</label>
									<input id={`newOptionDescription-${prompt.id}`} type="text" class="input w-full" bind:value={newOptionDescription} placeholder="Short description" maxlength="200" />
								</div>

								<div class="mb-2">
									<div class="mb-1 block text-xs text-text-muted">Assign Roles</div>
									<div class="flex flex-wrap gap-1">
										{#each onboardingRoles as role (role.id)}
											<button
												type="button"
												class="flex items-center gap-1 rounded-full px-2 py-0.5 text-xs transition-colors {newOptionRoleIds.includes(role.id)
													? 'bg-brand-500/20 text-brand-400 ring-1 ring-brand-500/40'
													: 'bg-bg-modifier text-text-muted hover:bg-bg-tertiary'}"
												onclick={() => toggleRole(role.id)}
											>
												<span class="h-2 w-2 rounded-full" style="background-color: {role.color ?? '#99aab5'}"></span>
												{role.name}
											</button>
										{/each}
									</div>
								</div>

								<div class="mb-3">
									<div class="mb-1 block text-xs text-text-muted">Grant Channel Access</div>
									<div class="flex flex-wrap gap-1">
										{#each textChannels as channel (channel.id)}
											<button
												type="button"
												class="flex items-center gap-1 rounded-full px-2 py-0.5 text-xs transition-colors {newOptionChannelIds.includes(channel.id)
													? 'bg-brand-500/20 text-brand-400 ring-1 ring-brand-500/40'
													: 'bg-bg-modifier text-text-muted hover:bg-bg-tertiary'}"
												onclick={() => toggleChannel(channel.id)}
											>
												# {channel.name ?? 'unnamed'}
											</button>
										{/each}
									</div>
								</div>

								<div class="flex gap-2">
									<button class="btn-primary text-xs" onclick={handleAddOption} disabled={!newOptionLabel.trim()}>Add Option</button>
									<button class="btn-secondary text-xs" onclick={cancelAddingOption}>Cancel</button>
								</div>
							</div>
						{:else}
							<button class="text-xs text-brand-400 hover:text-brand-300" onclick={() => startAddingOption(prompt.id)}>
								+ Add option
							</button>
						{/if}
					</div>
				{/each}
			</div>
		{/if}
	</div>
{/if}
