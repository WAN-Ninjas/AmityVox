<script lang="ts">
	import { page } from '$app/stores';
	import { currentGuild, updateGuild } from '$lib/stores/guilds';
	import { currentUser } from '$lib/stores/auth';
	import { api } from '$lib/api/client';
	import { goto } from '$app/navigation';
	import Avatar from '$components/common/Avatar.svelte';
	import type { Role, Invite, Ban, AuditLogEntry, CustomEmoji, Webhook, Category, Channel, AutoModRule, AutoModAction, MemberWarning, MessageReport, RaidConfig, OnboardingConfig, OnboardingPrompt } from '$lib/types';

	type Tab = 'overview' | 'roles' | 'categories' | 'invites' | 'bans' | 'emoji' | 'webhooks' | 'audit' | 'automod' | 'moderation' | 'raid' | 'onboarding';
	let currentTab = $state<Tab>('overview');

	// --- Overview ---
	let name = $state('');
	let description = $state('');
	let verificationLevel = $state(0);
	let iconFile = $state<File | null>(null);
	let iconPreview = $state<string | null>(null);
	let guildTags = $state<string[]>([]);
	let newTag = $state('');
	let saving = $state(false);
	let error = $state('');
	let success = $state('');
	let deleteConfirm = $state('');

	const availableTags = ['Gaming', 'Music', 'Education', 'Science & Tech', 'Entertainment', 'Art & Creative', 'Community', 'Other'];

	// --- Roles ---
	let roles = $state<Role[]>([]);
	let loadingRoles = $state(false);
	let newRoleName = $state('');
	let creatingRole = $state(false);

	// --- Invites ---
	let invites = $state<Invite[]>([]);
	let loadingInvites = $state(false);
	let creatingInvite = $state(false);
	let newInviteMaxUses = $state(0);
	let newInviteExpiry = $state(86400); // 24h default

	// --- Bans ---
	let bans = $state<Ban[]>([]);
	let loadingBans = $state(false);

	// --- Emoji ---
	let emoji = $state<CustomEmoji[]>([]);
	let loadingEmoji = $state(false);
	let emojiFile = $state<File | null>(null);
	let emojiName = $state('');
	let uploadingEmoji = $state(false);

	// --- Webhooks ---
	let webhooks = $state<Webhook[]>([]);
	let loadingWebhooks = $state(false);
	let newWebhookName = $state('');
	let newWebhookChannel = $state('');
	let creatingWebhook = $state(false);

	// --- Categories ---
	let categories = $state<Category[]>([]);
	let loadingCategories = $state(false);
	let newCategoryName = $state('');
	let creatingCategory = $state(false);
	let editingCategoryId = $state<string | null>(null);
	let editingCategoryName = $state('');

	// --- Audit Log ---
	let auditLog = $state<AuditLogEntry[]>([]);
	let loadingAudit = $state(false);

	// --- AutoMod ---
	let automodRules = $state<AutoModRule[]>([]);
	let automodActions = $state<AutoModAction[]>([]);
	let loadingAutomod = $state(false);
	let creatingRule = $state(false);
	let newRuleType = $state<string>('word_filter');
	let newRuleName = $state('');
	let newRuleAction = $state<string>('delete');
	let newRuleEnabled = $state(true);
	let newRuleExemptRoles = $state<string[]>([]);
	let newRuleExemptChannels = $state<string[]>([]);

	// AutoMod: guild roles & channels available for exemption selection
	let automodGuildRoles = $state<Role[]>([]);
	let automodGuildChannels = $state<Channel[]>([]);
	let loadingAutomodMeta = $state(false);

	// AutoMod: editing exemptions on an existing rule
	let editingExemptRuleId = $state<string | null>(null);
	let editingExemptRoles = $state<string[]>([]);
	let editingExemptChannels = $state<string[]>([]);

	// --- Moderation ---
	let warnings = $state<MemberWarning[]>([]);
	let reports = $state<MessageReport[]>([]);
	let loadingWarnings = $state(false);
	let loadingReports = $state(false);
	let reportFilter = $state<string>('open');

	// --- Raid Config ---
	let raidConfig = $state<RaidConfig | null>(null);
	let loadingRaid = $state(false);
	let savingRaid = $state(false);

	// --- Onboarding ---
	let onboardingConfig = $state<OnboardingConfig | null>(null);
	let loadingOnboarding = $state(false);
	let savingOnboarding = $state(false);
	let onboardingChannels = $state<Channel[]>([]);
	let onboardingRoles = $state<Role[]>([]);
	let newRuleText = $state('');
	let newPromptTitle = $state('');
	let newPromptRequired = $state(false);
	let newPromptSingleSelect = $state(false);
	let creatingPrompt = $state(false);
	// Editing prompt inline
	let editingPromptId = $state<string | null>(null);
	let editingPromptTitle = $state('');
	let editingPromptRequired = $state(false);
	let editingPromptSingleSelect = $state(false);
	// Adding option to a prompt
	let addingOptionToPromptId = $state<string | null>(null);
	let newOptionLabel = $state('');
	let newOptionDescription = $state('');
	let newOptionEmoji = $state('');
	let newOptionRoleIds = $state<string[]>([]);
	let newOptionChannelIds = $state<string[]>([]);

	const isOwner = $derived($currentGuild?.owner_id === $currentUser?.id);

	$effect(() => {
		if ($currentGuild) {
			name = $currentGuild.name;
			description = $currentGuild.description ?? '';
			verificationLevel = $currentGuild.verification_level ?? 0;
			guildTags = [...($currentGuild.tags ?? [])];
		}
	});

	// Load data when switching tabs.
	$effect(() => {
		if (!$currentGuild) return;
		const gId = $currentGuild.id;
		if (currentTab === 'roles' && roles.length === 0) loadRoles(gId);
		if (currentTab === 'categories' && categories.length === 0) loadCategories(gId);
		if (currentTab === 'invites' && invites.length === 0) loadInvites(gId);
		if (currentTab === 'bans' && bans.length === 0) loadBans(gId);
		if (currentTab === 'emoji' && emoji.length === 0) loadEmoji(gId);
		if (currentTab === 'webhooks' && webhooks.length === 0) loadWebhooks(gId);
		if (currentTab === 'audit' && auditLog.length === 0) loadAudit(gId);
		if (currentTab === 'automod' && automodRules.length === 0) loadAutomod(gId);
		if (currentTab === 'moderation' && reports.length === 0) loadReports(gId);
		if (currentTab === 'raid' && !raidConfig) loadRaid(gId);
		if (currentTab === 'onboarding' && !onboardingConfig) loadOnboarding(gId);
	});

	// --- Data loading ---

	async function loadRoles(guildId: string) {
		loadingRoles = true;
		try { roles = await api.getRoles(guildId); } catch {}
		finally { loadingRoles = false; }
	}

	async function loadInvites(guildId: string) {
		loadingInvites = true;
		try { invites = await api.getGuildInvites(guildId); } catch {}
		finally { loadingInvites = false; }
	}

	async function loadBans(guildId: string) {
		loadingBans = true;
		try { bans = await api.getGuildBans(guildId); } catch {}
		finally { loadingBans = false; }
	}

	async function loadEmoji(guildId: string) {
		loadingEmoji = true;
		try { emoji = await api.getGuildEmoji(guildId); } catch {}
		finally { loadingEmoji = false; }
	}

	async function loadWebhooks(guildId: string) {
		loadingWebhooks = true;
		try { webhooks = await api.getGuildWebhooks(guildId); } catch {}
		finally { loadingWebhooks = false; }
	}

	async function loadCategories(guildId: string) {
		loadingCategories = true;
		try { categories = await api.getCategories(guildId); } catch {}
		finally { loadingCategories = false; }
	}

	async function loadAudit(guildId: string) {
		loadingAudit = true;
		try { auditLog = await api.getAuditLog(guildId, { limit: 50 }); } catch {}
		finally { loadingAudit = false; }
	}

	async function loadAutomod(guildId: string) {
		loadingAutomod = true;
		loadingAutomodMeta = true;
		try {
			const [rules, actions, guildRoles, guildChannels] = await Promise.all([
				api.getAutoModRules(guildId),
				api.getAutoModActions(guildId),
				api.getRoles(guildId),
				api.getGuildChannels(guildId)
			]);
			automodRules = rules;
			automodActions = actions;
			automodGuildRoles = guildRoles;
			automodGuildChannels = guildChannels;
		} catch {}
		finally {
			loadingAutomod = false;
			loadingAutomodMeta = false;
		}
	}

	async function loadReports(guildId: string) {
		loadingReports = true;
		try { reports = await api.getReports(guildId, { status: reportFilter }); } catch {}
		finally { loadingReports = false; }
	}

	async function loadRaid(guildId: string) {
		loadingRaid = true;
		try { raidConfig = await api.getRaidConfig(guildId); } catch {}
		finally { loadingRaid = false; }
	}

	async function loadOnboarding(guildId: string) {
		loadingOnboarding = true;
		try {
			const [config, guildChannels, guildRoles] = await Promise.all([
				api.getOnboarding(guildId),
				api.getGuildChannels(guildId),
				api.getRoles(guildId)
			]);
			onboardingConfig = config;
			onboardingChannels = guildChannels;
			onboardingRoles = guildRoles;
		} catch {
			// Onboarding may not exist yet; initialize defaults.
			onboardingConfig = { enabled: false, welcome_message: '', rules: [], default_channel_ids: [], prompts: [] };
			try {
				onboardingChannels = await api.getGuildChannels(guildId);
				onboardingRoles = await api.getRoles(guildId);
			} catch {}
		}
		finally { loadingOnboarding = false; }
	}

	// --- Onboarding actions ---

	async function handleSaveOnboarding() {
		if (!$currentGuild || !onboardingConfig) return;
		savingOnboarding = true;
		error = '';
		try {
			onboardingConfig = await api.updateOnboarding($currentGuild.id, {
				enabled: onboardingConfig.enabled,
				welcome_message: onboardingConfig.welcome_message,
				rules: onboardingConfig.rules,
				default_channel_ids: onboardingConfig.default_channel_ids
			});
			success = 'Onboarding settings saved!';
			setTimeout(() => (success = ''), 3000);
		} catch (err: any) {
			error = err.message || 'Failed to save onboarding';
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
		onboardingConfig = { ...onboardingConfig, rules: onboardingConfig.rules.filter((_, i) => i !== index) };
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
		if (ids.includes(channelId)) {
			onboardingConfig = { ...onboardingConfig, default_channel_ids: ids.filter((id) => id !== channelId) };
		} else {
			onboardingConfig = { ...onboardingConfig, default_channel_ids: [...ids, channelId] };
		}
	}

	async function handleCreatePrompt() {
		if (!$currentGuild || !newPromptTitle.trim()) return;
		creatingPrompt = true;
		error = '';
		try {
			const prompt = await api.createOnboardingPrompt($currentGuild.id, {
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
		} catch (err: any) {
			error = err.message || 'Failed to create prompt';
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
		if (!$currentGuild || !editingPromptId || !editingPromptTitle.trim()) return;
		error = '';
		try {
			await api.updateOnboardingPrompt($currentGuild.id, editingPromptId, {
				title: editingPromptTitle.trim(),
				required: editingPromptRequired,
				single_select: editingPromptSingleSelect
			});
			if (onboardingConfig) {
				onboardingConfig = {
					...onboardingConfig,
					prompts: onboardingConfig.prompts.map((p) =>
						p.id === editingPromptId
							? { ...p, title: editingPromptTitle.trim(), required: editingPromptRequired, single_select: editingPromptSingleSelect }
							: p
					)
				};
			}
			editingPromptId = null;
			editingPromptTitle = '';
		} catch (err: any) {
			error = err.message || 'Failed to update prompt';
		}
	}

	async function handleDeletePrompt(promptId: string) {
		if (!$currentGuild || !confirm('Delete this onboarding prompt?')) return;
		error = '';
		try {
			await api.deleteOnboardingPrompt($currentGuild.id, promptId);
			if (onboardingConfig) {
				onboardingConfig = { ...onboardingConfig, prompts: onboardingConfig.prompts.filter((p) => p.id !== promptId) };
			}
			if (editingPromptId === promptId) editingPromptId = null;
			if (addingOptionToPromptId === promptId) addingOptionToPromptId = null;
		} catch (err: any) {
			error = err.message || 'Failed to delete prompt';
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

	async function handleAddOption() {
		if (!$currentGuild || !addingOptionToPromptId || !newOptionLabel.trim()) return;
		error = '';
		try {
			// We update the prompt with the new option appended. The API will handle creating the option.
			const prompt = onboardingConfig?.prompts.find((p) => p.id === addingOptionToPromptId);
			if (!prompt) return;
			const newOptions = [
				...prompt.options.map((o) => ({ label: o.label, description: o.description, emoji: o.emoji, role_ids: o.role_ids, channel_ids: o.channel_ids })),
				{
					label: newOptionLabel.trim(),
					description: newOptionDescription.trim() || undefined,
					emoji: newOptionEmoji.trim() || undefined,
					role_ids: newOptionRoleIds,
					channel_ids: newOptionChannelIds
				}
			];
			await api.updateOnboardingPrompt($currentGuild.id, addingOptionToPromptId, { options: newOptions as any });
			// Reload onboarding to get server-generated option IDs.
			await loadOnboarding($currentGuild.id);
			cancelAddingOption();
			success = 'Option added!';
			setTimeout(() => (success = ''), 2000);
		} catch (err: any) {
			error = err.message || 'Failed to add option';
		}
	}

	async function handleRemoveOption(promptId: string, optionId: string) {
		if (!$currentGuild || !onboardingConfig) return;
		error = '';
		try {
			const prompt = onboardingConfig.prompts.find((p) => p.id === promptId);
			if (!prompt) return;
			const newOptions = prompt.options
				.filter((o) => o.id !== optionId)
				.map((o) => ({ label: o.label, description: o.description, emoji: o.emoji, role_ids: o.role_ids, channel_ids: o.channel_ids }));
			await api.updateOnboardingPrompt($currentGuild.id, promptId, { options: newOptions as any });
			await loadOnboarding($currentGuild.id);
		} catch (err: any) {
			error = err.message || 'Failed to remove option';
		}
	}

	function getOnboardingChannelName(channelId: string): string {
		const ch = onboardingChannels.find((c) => c.id === channelId);
		return ch?.name ?? channelId.slice(0, 8) + '...';
	}

	function getOnboardingRoleName(roleId: string): string {
		const r = onboardingRoles.find((role) => role.id === roleId);
		return r?.name ?? roleId.slice(0, 8) + '...';
	}

	// --- Overview actions ---

	function handleIconSelect(e: Event) {
		const file = (e.target as HTMLInputElement).files?.[0];
		if (!file?.type.startsWith('image/')) return;
		iconFile = file;
		iconPreview = URL.createObjectURL(file);
	}

	async function handleSave() {
		if (!$currentGuild) return;
		saving = true;
		error = '';
		success = '';

		try {
			let iconId: string | undefined;
			if (iconFile) {
				const uploaded = await api.uploadFile(iconFile);
				iconId = uploaded.id;
			}

			const payload: Record<string, unknown> = {
				name, description: description || undefined,
				verification_level: verificationLevel,
				tags: guildTags
			};
			if (iconId) payload.icon_id = iconId;

			const updated = await api.updateGuild($currentGuild.id, payload as any);
			updateGuild(updated);
			iconFile = null;
			iconPreview = null;
			success = 'Guild updated!';
			setTimeout(() => (success = ''), 3000);
		} catch (err: any) {
			error = err.message || 'Failed to save';
		} finally {
			saving = false;
		}
	}

	async function handleDelete() {
		if (!$currentGuild) return;
		if (deleteConfirm !== $currentGuild.name) {
			error = 'Type the guild name to confirm deletion.';
			return;
		}
		try {
			await api.deleteGuild($currentGuild.id);
			goto('/app');
		} catch (err: any) {
			error = err.message || 'Failed to delete';
		}
	}

	// --- Role actions ---

	async function handleCreateRole() {
		if (!$currentGuild || !newRoleName.trim()) return;
		creatingRole = true;
		try {
			const role = await api.createRole($currentGuild.id, newRoleName.trim());
			roles = [...roles, role];
			newRoleName = '';
		} catch (err: any) {
			error = err.message || 'Failed to create role';
		} finally {
			creatingRole = false;
		}
	}

	// --- Invite actions ---

	async function handleCreateInvite() {
		if (!$currentGuild) return;
		creatingInvite = true;
		try {
			const opts: { max_uses?: number; max_age_seconds?: number } = {};
			if (newInviteMaxUses > 0) opts.max_uses = newInviteMaxUses;
			if (newInviteExpiry > 0) opts.max_age_seconds = newInviteExpiry;
			const invite = await api.createInvite($currentGuild.id, opts);
			invites = [invite, ...invites];
		} catch (err: any) {
			error = err.message || 'Failed to create invite';
		} finally {
			creatingInvite = false;
		}
	}

	async function handleRevokeInvite(code: string) {
		try {
			await api.deleteInvite(code);
			invites = invites.filter((i) => i.code !== code);
		} catch (err: any) {
			error = err.message || 'Failed to revoke invite';
		}
	}

	function copyInviteLink(code: string) {
		navigator.clipboard.writeText(`${window.location.origin}/invite/${code}`);
		success = 'Invite link copied!';
		setTimeout(() => (success = ''), 2000);
	}

	// --- Ban actions ---

	async function handleUnban(userId: string) {
		if (!$currentGuild) return;
		try {
			await api.unbanUser($currentGuild.id, userId);
			bans = bans.filter((b) => b.user_id !== userId);
		} catch (err: any) {
			error = err.message || 'Failed to unban';
		}
	}

	// --- Emoji actions ---

	async function handleDeleteEmoji(emojiId: string) {
		if (!$currentGuild || !confirm('Delete this emoji?')) return;
		try {
			await api.deleteGuildEmoji($currentGuild.id, emojiId);
			emoji = emoji.filter((e) => e.id !== emojiId);
		} catch (err: any) {
			error = err.message || 'Failed to delete emoji';
		}
	}

	// --- Emoji upload ---

	function handleEmojiFileSelect(e: Event) {
		const file = (e.target as HTMLInputElement).files?.[0];
		if (!file?.type.startsWith('image/')) return;
		emojiFile = file;
		if (!emojiName) emojiName = file.name.replace(/\.[^.]+$/, '').replace(/[^a-zA-Z0-9_]/g, '_').slice(0, 32);
	}

	async function handleUploadEmoji() {
		if (!$currentGuild || !emojiFile || !emojiName.trim()) return;
		uploadingEmoji = true;
		try {
			const newEmoji = await api.uploadEmoji($currentGuild.id, emojiName.trim(), emojiFile);
			emoji = [...emoji, newEmoji];
			emojiFile = null;
			emojiName = '';
		} catch (err: any) {
			error = err.message || 'Failed to upload emoji';
		} finally {
			uploadingEmoji = false;
		}
	}

	// --- Webhook actions ---

	async function handleCreateWebhook() {
		if (!$currentGuild || !newWebhookName.trim() || !newWebhookChannel) return;
		creatingWebhook = true;
		try {
			const webhook = await api.createWebhook($currentGuild.id, {
				name: newWebhookName.trim(),
				channel_id: newWebhookChannel
			});
			webhooks = [...webhooks, webhook];
			newWebhookName = '';
			newWebhookChannel = '';
		} catch (err: any) {
			error = err.message || 'Failed to create webhook';
		} finally {
			creatingWebhook = false;
		}
	}

	async function handleDeleteWebhook(webhookId: string) {
		if (!$currentGuild || !confirm('Delete this webhook?')) return;
		try {
			await api.deleteWebhook($currentGuild.id, webhookId);
			webhooks = webhooks.filter((w) => w.id !== webhookId);
		} catch (err: any) {
			error = err.message || 'Failed to delete webhook';
		}
	}

	function copyWebhookUrl(webhook: Webhook) {
		navigator.clipboard.writeText(`${window.location.origin}/api/v1/webhooks/${webhook.id}/${webhook.token}`);
		success = 'Webhook URL copied!';
		setTimeout(() => (success = ''), 2000);
	}

	// --- Category actions ---

	async function handleCreateCategory() {
		if (!$currentGuild || !newCategoryName.trim()) return;
		creatingCategory = true;
		try {
			const cat = await api.createCategory($currentGuild.id, newCategoryName.trim());
			categories = [...categories, cat];
			newCategoryName = '';
		} catch (err: any) {
			error = err.message || 'Failed to create category';
		} finally {
			creatingCategory = false;
		}
	}

	async function handleRenameCategory() {
		if (!$currentGuild || !editingCategoryId || !editingCategoryName.trim()) return;
		try {
			const updated = await api.updateCategory($currentGuild.id, editingCategoryId, { name: editingCategoryName.trim() });
			categories = categories.map((c) => (c.id === editingCategoryId ? updated : c));
			editingCategoryId = null;
			editingCategoryName = '';
		} catch (err: any) {
			error = err.message || 'Failed to rename category';
		}
	}

	async function handleDeleteCategory(categoryId: string) {
		if (!$currentGuild || !confirm('Delete this category? Channels in it will become uncategorized.')) return;
		try {
			await api.deleteCategory($currentGuild.id, categoryId);
			categories = categories.filter((c) => c.id !== categoryId);
		} catch (err: any) {
			error = err.message || 'Failed to delete category';
		}
	}

	// --- Role actions (edit/delete) ---

	async function handleDeleteRole(roleId: string) {
		if (!$currentGuild || !confirm('Delete this role?')) return;
		try {
			await api.deleteRole($currentGuild.id, roleId);
			roles = roles.filter((r) => r.id !== roleId);
		} catch (err: any) {
			error = err.message || 'Failed to delete role';
		}
	}

	// --- AutoMod actions ---

	async function handleCreateAutomodRule() {
		if (!$currentGuild || !newRuleName.trim()) return;
		creatingRule = true;
		try {
			const rule = await api.createAutoModRule($currentGuild.id, {
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
		} catch (err: any) {
			error = err.message || 'Failed to create AutoMod rule';
		} finally {
			creatingRule = false;
		}
	}

	async function handleToggleAutomodRule(rule: AutoModRule) {
		if (!$currentGuild) return;
		try {
			const updated = await api.updateAutoModRule($currentGuild.id, rule.id, { enabled: !rule.enabled });
			automodRules = automodRules.map(r => r.id === rule.id ? updated : r);
		} catch (err: any) {
			error = err.message || 'Failed to update rule';
		}
	}

	async function handleDeleteAutomodRule(ruleId: string) {
		if (!$currentGuild || !confirm('Delete this AutoMod rule?')) return;
		try {
			await api.deleteAutoModRule($currentGuild.id, ruleId);
			automodRules = automodRules.filter(r => r.id !== ruleId);
			if (editingExemptRuleId === ruleId) editingExemptRuleId = null;
		} catch (err: any) {
			error = err.message || 'Failed to delete rule';
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
		if (!$currentGuild || !editingExemptRuleId) return;
		try {
			const updated = await api.updateAutoModRule($currentGuild.id, editingExemptRuleId, {
				exempt_roles: editingExemptRoles,
				exempt_channels: editingExemptChannels
			});
			automodRules = automodRules.map(r => r.id === editingExemptRuleId ? updated : r);
			editingExemptRuleId = null;
			editingExemptRoles = [];
			editingExemptChannels = [];
			success = 'Exemptions updated!';
			setTimeout(() => (success = ''), 3000);
		} catch (err: any) {
			error = err.message || 'Failed to update exemptions';
		}
	}

	function toggleArrayItem(arr: string[], item: string): string[] {
		if (arr.includes(item)) {
			return arr.filter(i => i !== item);
		}
		return [...arr, item];
	}

	function getRoleName(roleId: string): string {
		const role = automodGuildRoles.find(r => r.id === roleId);
		return role?.name ?? roleId.slice(0, 8) + '...';
	}

	function getChannelName(channelId: string): string {
		const channel = automodGuildChannels.find(c => c.id === channelId);
		return channel?.name ?? channelId.slice(0, 8) + '...';
	}

	// --- Moderation actions ---

	async function handleResolveReport(reportId: string, status: 'resolved' | 'dismissed') {
		if (!$currentGuild) return;
		try {
			const updated = await api.resolveReport($currentGuild.id, reportId, status);
			reports = reports.map(r => r.id === reportId ? updated : r);
		} catch (err: any) {
			error = err.message || 'Failed to resolve report';
		}
	}

	async function handleFilterReports() {
		if (!$currentGuild) return;
		loadingReports = true;
		try { reports = await api.getReports($currentGuild.id, { status: reportFilter }); } catch {}
		finally { loadingReports = false; }
	}

	// --- Raid actions ---

	async function handleSaveRaid() {
		if (!$currentGuild || !raidConfig) return;
		savingRaid = true;
		try {
			raidConfig = await api.updateRaidConfig($currentGuild.id, {
				enabled: raidConfig.enabled,
				join_rate_limit: raidConfig.join_rate_limit,
				join_rate_window: raidConfig.join_rate_window,
				min_account_age: raidConfig.min_account_age,
				lockdown_active: raidConfig.lockdown_active
			});
			success = 'Raid protection settings saved!';
			setTimeout(() => (success = ''), 3000);
		} catch (err: any) {
			error = err.message || 'Failed to save raid config';
		} finally {
			savingRaid = false;
		}
	}

	// --- Helpers ---

	const tabs: { id: Tab; label: string }[] = [
		{ id: 'overview', label: 'Overview' },
		{ id: 'roles', label: 'Roles' },
		{ id: 'categories', label: 'Categories' },
		{ id: 'invites', label: 'Invites' },
		{ id: 'bans', label: 'Bans' },
		{ id: 'emoji', label: 'Emoji' },
		{ id: 'webhooks', label: 'Webhooks' },
		{ id: 'audit', label: 'Audit Log' },
		{ id: 'automod', label: 'AutoMod' },
		{ id: 'moderation', label: 'Moderation' },
		{ id: 'raid', label: 'Raid Protection' },
		{ id: 'onboarding', label: 'Onboarding' }
	];

	const expiryOptions = [
		{ label: '30 minutes', value: 1800 },
		{ label: '1 hour', value: 3600 },
		{ label: '6 hours', value: 21600 },
		{ label: '12 hours', value: 43200 },
		{ label: '1 day', value: 86400 },
		{ label: '7 days', value: 604800 },
		{ label: 'Never', value: 0 }
	];

	function formatDate(iso: string): string {
		return new Date(iso).toLocaleString();
	}

	function formatRelative(iso: string | null): string {
		if (!iso) return 'Never';
		const diff = new Date(iso).getTime() - Date.now();
		if (diff <= 0) return 'Expired';
		const hours = Math.floor(diff / 3600000);
		if (hours < 1) return `${Math.floor(diff / 60000)}m`;
		if (hours < 24) return `${hours}h`;
		return `${Math.floor(hours / 24)}d`;
	}

	const actionTypeLabels: Record<string, string> = {
		guild_update: 'Guild Updated',
		channel_create: 'Channel Created',
		channel_update: 'Channel Updated',
		channel_delete: 'Channel Deleted',
		role_create: 'Role Created',
		role_update: 'Role Updated',
		role_delete: 'Role Deleted',
		member_kick: 'Member Kicked',
		member_ban: 'Member Banned',
		member_unban: 'Member Unbanned',
		invite_create: 'Invite Created',
		invite_delete: 'Invite Deleted',
		message_pin: 'Message Pinned',
		message_unpin: 'Message Unpinned',
		emoji_create: 'Emoji Created',
		emoji_delete: 'Emoji Deleted'
	};
</script>

<svelte:head>
	<title>Guild Settings — AmityVox</title>
</svelte:head>

<div class="flex h-full">
	<nav class="w-52 shrink-0 overflow-y-auto bg-bg-secondary p-4">
		<h3 class="mb-2 text-xs font-bold uppercase tracking-wide text-text-muted">Guild Settings</h3>
		<ul class="space-y-0.5">
			{#each tabs as tab (tab.id)}
				<li>
					<button
						class="w-full rounded px-2 py-1.5 text-left text-sm transition-colors {currentTab === tab.id ? 'bg-bg-modifier text-text-primary' : 'text-text-muted hover:bg-bg-modifier hover:text-text-secondary'}"
						onclick={() => (currentTab = tab.id)}
					>
						{tab.label}
					</button>
				</li>
			{/each}
		</ul>
		<div class="my-2 border-t border-bg-modifier"></div>
		<button
			class="w-full rounded px-2 py-1.5 text-left text-sm text-text-muted hover:bg-bg-modifier hover:text-text-secondary"
			onclick={() => goto(`/app/guilds/${$page.params.guildId}`)}
		>
			Back to guild
		</button>
	</nav>

	<div class="flex-1 overflow-y-auto bg-bg-tertiary p-8">
		<div class="max-w-xl">
			{#if error}
				<div class="mb-4 rounded bg-red-500/10 px-3 py-2 text-sm text-red-400">{error}</div>
			{/if}
			{#if success}
				<div class="mb-4 rounded bg-green-500/10 px-3 py-2 text-sm text-green-400">{success}</div>
			{/if}

			<!-- ==================== OVERVIEW ==================== -->
			{#if currentTab === 'overview'}
				<h1 class="mb-6 text-xl font-bold text-text-primary">Guild Overview</h1>

				<!-- Icon + name -->
				<div class="mb-6 flex items-center gap-4">
					<div class="relative">
						<Avatar
							name={$currentGuild?.name ?? '?'}
							src={iconPreview ?? ($currentGuild?.icon_id ? `/api/v1/files/${$currentGuild.icon_id}` : null)}
							size="lg"
						/>
						<label class="absolute inset-0 flex cursor-pointer items-center justify-center rounded-full bg-black/50 opacity-0 transition-opacity hover:opacity-100">
							<svg class="h-6 w-6 text-white" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
								<path d="M3 9a2 2 0 012-2h.93a2 2 0 001.664-.89l.812-1.22A2 2 0 0110.07 4h3.86a2 2 0 011.664.89l.812 1.22A2 2 0 0018.07 7H19a2 2 0 012 2v9a2 2 0 01-2 2H5a2 2 0 01-2-2V9z" />
								<circle cx="12" cy="13" r="3" />
							</svg>
							<input type="file" accept="image/*" class="hidden" onchange={handleIconSelect} />
						</label>
					</div>
					<div class="text-sm text-text-muted">
						Click the icon to upload a new guild image.
					</div>
				</div>

				<div class="mb-4">
					<label for="guildName" class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">Guild Name</label>
					<input id="guildName" type="text" bind:value={name} class="input w-full" maxlength="100" />
				</div>

				<div class="mb-6">
					<label for="guildDesc" class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">Description</label>
					<textarea id="guildDesc" bind:value={description} class="input w-full" rows="3" maxlength="1024"></textarea>
				</div>

				<div class="mb-6">
					<label for="verificationLevel" class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">Verification Level</label>
					<select id="verificationLevel" bind:value={verificationLevel} class="input w-full">
						<option value={0}>None</option>
						<option value={1}>Low — Verified email</option>
						<option value={2}>Medium — Registered 5+ min</option>
						<option value={3}>High — Member 10+ min</option>
						<option value={4}>Highest — Phone verified</option>
					</select>
					<p class="mt-1 text-xs text-text-muted">
						{#if verificationLevel === 0}
							No verification required to participate. Anyone can join and send messages immediately.
						{:else if verificationLevel === 1}
							Members must have a verified email address on their account.
						{:else if verificationLevel === 2}
							Members must be registered on this instance for at least 5 minutes.
						{:else if verificationLevel === 3}
							Members must have been a member of this guild for at least 10 minutes before they can participate.
						{:else}
							Members must have a verified phone number linked to their account.
						{/if}
					</p>
				</div>

				<!-- Tags (for discovery) -->
				<div class="mb-6">
					<label class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">Category Tags</label>
					<p class="mb-2 text-xs text-text-muted">Add tags to help users find your server in discovery. Max 5 tags.</p>
					<div class="mb-2 flex flex-wrap gap-1.5">
						{#each guildTags as tag, i}
							<span class="flex items-center gap-1 rounded-full bg-brand-500/15 px-2.5 py-1 text-xs text-brand-400">
								{tag}
								<button class="ml-0.5 text-brand-400/70 hover:text-brand-400" onclick={() => { guildTags = guildTags.filter((_, idx) => idx !== i); }}>
									<svg class="h-3 w-3" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24"><path d="M6 18L18 6M6 6l12 12" /></svg>
								</button>
							</span>
						{/each}
					</div>
					{#if guildTags.length < 5}
						<div class="flex flex-wrap gap-1.5">
							{#each availableTags.filter(t => !guildTags.includes(t)) as tag}
								<button
									class="rounded-full bg-bg-modifier px-2.5 py-1 text-xs text-text-muted transition-colors hover:bg-bg-tertiary hover:text-text-primary"
									onclick={() => { guildTags = [...guildTags, tag]; }}
								>
									+ {tag}
								</button>
							{/each}
						</div>
					{/if}
				</div>

				<button class="btn-primary" onclick={handleSave} disabled={saving}>
					{saving ? 'Saving...' : 'Save Changes'}
				</button>

				{#if isOwner}
					<div class="mt-12 border-t border-bg-modifier pt-6">
						<h2 class="mb-2 text-lg font-semibold text-red-400">Danger Zone</h2>
						<p class="mb-3 text-sm text-text-muted">
							Deleting a guild is permanent and cannot be undone. Type <strong class="text-text-primary">{$currentGuild?.name}</strong> to confirm.
						</p>
						<input type="text" class="input mb-3 w-full" bind:value={deleteConfirm} placeholder="Type guild name to confirm..." />
						<button
							class="rounded bg-red-600 px-4 py-2 text-sm font-medium text-white hover:bg-red-700 disabled:opacity-50"
							onclick={handleDelete}
							disabled={deleteConfirm !== $currentGuild?.name}
						>
							Delete Guild
						</button>
					</div>
				{/if}

			<!-- ==================== ROLES ==================== -->
			{:else if currentTab === 'roles'}
				<h1 class="mb-6 text-xl font-bold text-text-primary">Roles</h1>

				<div class="mb-6 flex gap-2">
					<input
						type="text" class="input flex-1" placeholder="New role name..."
						bind:value={newRoleName} maxlength="100"
						onkeydown={(e) => e.key === 'Enter' && handleCreateRole()}
					/>
					<button class="btn-primary" onclick={handleCreateRole} disabled={creatingRole || !newRoleName.trim()}>
						{creatingRole ? 'Creating...' : 'Create Role'}
					</button>
				</div>

				{#if loadingRoles}
					<p class="text-sm text-text-muted">Loading roles...</p>
				{:else if roles.length === 0}
					<p class="text-sm text-text-muted">No custom roles yet.</p>
				{:else}
					<div class="space-y-2">
						{#each roles as role (role.id)}
							<div class="flex items-center justify-between rounded-lg bg-bg-secondary p-3">
								<div class="flex items-center gap-3">
									<div class="h-3 w-3 rounded-full" style="background-color: {role.color ?? '#99aab5'}"></div>
									<span class="text-sm font-medium text-text-primary">{role.name}</span>
								</div>
								<div class="flex items-center gap-2 text-xs text-text-muted">
									{#if role.hoist}<span class="rounded bg-bg-modifier px-1.5 py-0.5">Hoisted</span>{/if}
									{#if role.mentionable}<span class="rounded bg-bg-modifier px-1.5 py-0.5">Mentionable</span>{/if}
									<span>Pos: {role.position}</span>
									<button
										class="text-red-400 hover:text-red-300"
										onclick={() => handleDeleteRole(role.id)}
										title="Delete role"
									>
										<svg class="h-3.5 w-3.5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
											<path d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
										</svg>
									</button>
								</div>
							</div>
						{/each}
					</div>
				{/if}

			<!-- ==================== CATEGORIES ==================== -->
			{:else if currentTab === 'categories'}
				<h1 class="mb-6 text-xl font-bold text-text-primary">Channel Categories</h1>

				<div class="mb-6 flex gap-2">
					<input
						type="text" class="input flex-1" placeholder="New category name..."
						bind:value={newCategoryName} maxlength="100"
						onkeydown={(e) => e.key === 'Enter' && handleCreateCategory()}
					/>
					<button class="btn-primary" onclick={handleCreateCategory} disabled={creatingCategory || !newCategoryName.trim()}>
						{creatingCategory ? 'Creating...' : 'Create Category'}
					</button>
				</div>

				{#if loadingCategories}
					<p class="text-sm text-text-muted">Loading categories...</p>
				{:else if categories.length === 0}
					<p class="text-sm text-text-muted">No categories yet. Channels will appear uncategorized.</p>
				{:else}
					<div class="space-y-2">
						{#each categories as cat (cat.id)}
							<div class="flex items-center justify-between rounded-lg bg-bg-secondary p-3">
								{#if editingCategoryId === cat.id}
									<div class="flex flex-1 items-center gap-2">
										<input
											type="text" class="input flex-1" bind:value={editingCategoryName}
											onkeydown={(e) => e.key === 'Enter' && handleRenameCategory()}
										/>
										<button class="btn-primary text-xs" onclick={handleRenameCategory}>Save</button>
										<button class="btn-secondary text-xs" onclick={() => (editingCategoryId = null)}>Cancel</button>
									</div>
								{:else}
									<div class="flex items-center gap-3">
										<svg class="h-4 w-4 text-text-muted" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
											<path d="M3 7v10a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-6l-2-2H5a2 2 0 00-2 2z" />
										</svg>
										<span class="text-sm font-medium text-text-primary">{cat.name}</span>
									</div>
									<div class="flex items-center gap-2">
										<span class="text-xs text-text-muted">Pos: {cat.position}</span>
										<button
											class="text-xs text-brand-400 hover:text-brand-300"
											onclick={() => { editingCategoryId = cat.id; editingCategoryName = cat.name; }}
										>
											Rename
										</button>
										<button
											class="text-xs text-red-400 hover:text-red-300"
											onclick={() => handleDeleteCategory(cat.id)}
										>
											Delete
										</button>
									</div>
								{/if}
							</div>
						{/each}
					</div>
				{/if}

			<!-- ==================== INVITES ==================== -->
			{:else if currentTab === 'invites'}
				<h1 class="mb-6 text-xl font-bold text-text-primary">Invites</h1>

				<div class="mb-6 rounded-lg bg-bg-secondary p-4">
					<h3 class="mb-3 text-sm font-semibold text-text-primary">Create Invite</h3>
					<div class="mb-3 flex gap-4">
						<div class="flex-1">
							<label class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">Expire After</label>
							<select bind:value={newInviteExpiry} class="input w-full">
								{#each expiryOptions as opt}
									<option value={opt.value}>{opt.label}</option>
								{/each}
							</select>
						</div>
						<div class="flex-1">
							<label class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">Max Uses (0 = unlimited)</label>
							<input type="number" min="0" max="100" bind:value={newInviteMaxUses} class="input w-full" />
						</div>
					</div>
					<button class="btn-primary" onclick={handleCreateInvite} disabled={creatingInvite}>
						{creatingInvite ? 'Creating...' : 'Create Invite'}
					</button>
				</div>

				{#if loadingInvites}
					<p class="text-sm text-text-muted">Loading invites...</p>
				{:else if invites.length === 0}
					<p class="text-sm text-text-muted">No active invites.</p>
				{:else}
					<div class="space-y-2">
						{#each invites as invite (invite.code)}
							<div class="flex items-center justify-between rounded-lg bg-bg-secondary p-3">
								<div>
									<code class="text-sm font-medium text-text-primary">{invite.code}</code>
									<p class="text-xs text-text-muted">
										Uses: {invite.uses}{invite.max_uses ? `/${invite.max_uses}` : ''} &middot;
										Expires: {formatRelative(invite.expires_at)}
									</p>
								</div>
								<div class="flex items-center gap-2">
									<button
										class="text-xs text-brand-400 hover:text-brand-300"
										onclick={() => copyInviteLink(invite.code)}
									>
										Copy Link
									</button>
									<button
										class="text-xs text-red-400 hover:text-red-300"
										onclick={() => handleRevokeInvite(invite.code)}
									>
										Revoke
									</button>
								</div>
							</div>
						{/each}
					</div>
				{/if}

			<!-- ==================== BANS ==================== -->
			{:else if currentTab === 'bans'}
				<h1 class="mb-6 text-xl font-bold text-text-primary">Bans</h1>

				{#if loadingBans}
					<p class="text-sm text-text-muted">Loading bans...</p>
				{:else if bans.length === 0}
					<p class="text-sm text-text-muted">No banned users.</p>
				{:else}
					<div class="space-y-2">
						{#each bans as ban (ban.user_id)}
							<div class="flex items-center justify-between rounded-lg bg-bg-secondary p-3">
								<div class="flex items-center gap-3">
									<Avatar name={ban.user?.display_name ?? ban.user?.username ?? '?'} size="sm" />
									<div>
										<span class="text-sm font-medium text-text-primary">
											{ban.user?.display_name ?? ban.user?.username ?? ban.user_id}
										</span>
										{#if ban.reason}
											<p class="text-xs text-text-muted">Reason: {ban.reason}</p>
										{/if}
									</div>
								</div>
								<button
									class="text-xs text-red-400 hover:text-red-300"
									onclick={() => handleUnban(ban.user_id)}
								>
									Unban
								</button>
							</div>
						{/each}
					</div>
				{/if}

			<!-- ==================== EMOJI ==================== -->
			{:else if currentTab === 'emoji'}
				<h1 class="mb-6 text-xl font-bold text-text-primary">Custom Emoji</h1>

				<!-- Upload form -->
				<div class="mb-6 rounded-lg bg-bg-secondary p-4">
					<h3 class="mb-3 text-sm font-semibold text-text-primary">Upload Emoji</h3>
					<div class="mb-3 flex gap-3">
						<div>
							<label class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">Image</label>
							<input type="file" accept="image/png,image/gif,image/jpeg,image/webp" onchange={handleEmojiFileSelect} class="text-sm text-text-muted" />
						</div>
						<div class="flex-1">
							<label class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">Name</label>
							<input type="text" class="input w-full" bind:value={emojiName} placeholder="emoji_name" maxlength="32" pattern="[a-zA-Z0-9_]+" />
						</div>
					</div>
					<button class="btn-primary" onclick={handleUploadEmoji} disabled={uploadingEmoji || !emojiFile || !emojiName.trim()}>
						{uploadingEmoji ? 'Uploading...' : 'Upload Emoji'}
					</button>
				</div>

				{#if loadingEmoji}
					<p class="text-sm text-text-muted">Loading emoji...</p>
				{:else if emoji.length === 0}
					<p class="text-sm text-text-muted">No custom emoji yet. Upload one above!</p>
				{:else}
					<div class="grid grid-cols-4 gap-3">
						{#each emoji as e (e.id)}
							<div class="flex flex-col items-center gap-1 rounded-lg bg-bg-secondary p-3">
								<img src="/api/v1/files/{e.id}" alt={e.name} class="h-8 w-8" />
								<span class="text-xs text-text-muted">:{e.name}:</span>
								<button
									class="text-2xs text-red-400 hover:text-red-300"
									onclick={() => handleDeleteEmoji(e.id)}
								>
									Delete
								</button>
							</div>
						{/each}
					</div>
				{/if}

			<!-- ==================== WEBHOOKS ==================== -->
			{:else if currentTab === 'webhooks'}
				<h1 class="mb-6 text-xl font-bold text-text-primary">Webhooks</h1>

				<div class="mb-6 rounded-lg bg-bg-secondary p-4">
					<h3 class="mb-3 text-sm font-semibold text-text-primary">Create Webhook</h3>
					<div class="mb-3">
						<label class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">Name</label>
						<input type="text" class="input w-full" bind:value={newWebhookName} placeholder="Webhook name" maxlength="80" />
					</div>
					<div class="mb-3">
						<label class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">Channel ID</label>
						<input type="text" class="input w-full" bind:value={newWebhookChannel} placeholder="Channel ID to post in" />
					</div>
					<button class="btn-primary" onclick={handleCreateWebhook} disabled={creatingWebhook || !newWebhookName.trim() || !newWebhookChannel}>
						{creatingWebhook ? 'Creating...' : 'Create Webhook'}
					</button>
				</div>

				{#if loadingWebhooks}
					<p class="text-sm text-text-muted">Loading webhooks...</p>
				{:else if webhooks.length === 0}
					<p class="text-sm text-text-muted">No webhooks yet.</p>
				{:else}
					<div class="space-y-2">
						{#each webhooks as wh (wh.id)}
							<div class="rounded-lg bg-bg-secondary p-3">
								<div class="flex items-center justify-between">
									<div>
										<span class="text-sm font-medium text-text-primary">{wh.name}</span>
										<p class="text-xs text-text-muted">
											Type: {wh.webhook_type} &middot; Channel: {wh.channel_id.slice(0, 8)}...
										</p>
									</div>
									<div class="flex items-center gap-2">
										<button
											class="text-xs text-brand-400 hover:text-brand-300"
											onclick={() => copyWebhookUrl(wh)}
										>
											Copy URL
										</button>
										<button
											class="text-xs text-red-400 hover:text-red-300"
											onclick={() => handleDeleteWebhook(wh.id)}
										>
											Delete
										</button>
									</div>
								</div>
								<div class="mt-2 rounded bg-bg-primary p-2">
									<code class="break-all text-2xs text-text-muted">
										{window.location.origin}/api/v1/webhooks/{wh.id}/{wh.token}
									</code>
								</div>
							</div>
						{/each}
					</div>
				{/if}

			<!-- ==================== AUDIT LOG ==================== -->
			{:else if currentTab === 'audit'}
				<h1 class="mb-6 text-xl font-bold text-text-primary">Audit Log</h1>

				{#if loadingAudit}
					<p class="text-sm text-text-muted">Loading audit log...</p>
				{:else if auditLog.length === 0}
					<p class="text-sm text-text-muted">No audit log entries.</p>
				{:else}
					<div class="space-y-2">
						{#each auditLog as entry (entry.id)}
							<div class="rounded-lg bg-bg-secondary p-3">
								<div class="flex items-center justify-between">
									<div class="flex items-center gap-2">
										<span class="text-sm font-medium text-text-primary">
											{entry.actor?.display_name ?? entry.actor?.username ?? entry.actor_id.slice(0, 8)}
										</span>
										<span class="rounded bg-bg-modifier px-1.5 py-0.5 text-xs text-text-muted">
											{actionTypeLabels[entry.action_type] ?? entry.action_type}
										</span>
									</div>
									<span class="text-xs text-text-muted">{formatDate(entry.created_at)}</span>
								</div>
								{#if entry.reason}
									<p class="mt-1 text-xs text-text-muted">Reason: {entry.reason}</p>
								{/if}
							</div>
						{/each}
					</div>
				{/if}

			<!-- ==================== AUTOMOD ==================== -->
			{:else if currentTab === 'automod'}
				<h1 class="mb-6 text-xl font-bold text-text-primary">AutoMod Rules</h1>

				<div class="mb-6 rounded-lg bg-bg-secondary p-4">
					<h3 class="mb-3 text-sm font-semibold text-text-primary">Create Rule</h3>
					<div class="mb-3 grid grid-cols-2 gap-3">
						<div>
							<label class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">Rule Name</label>
							<input type="text" class="input w-full" bind:value={newRuleName} placeholder="My rule" maxlength="100" />
						</div>
						<div>
							<label class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">Type</label>
							<select class="input w-full" bind:value={newRuleType}>
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
							<label class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">Action</label>
							<select class="input w-full" bind:value={newRuleAction}>
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

					<!-- Exempt Roles -->
					<div class="mb-3">
						<label class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">Exempt Roles</label>
						<p class="mb-1.5 text-xs text-text-muted">Members with these roles will not be affected by this rule.</p>
						{#if loadingAutomodMeta}
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
											<svg class="h-3 w-3" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24"><path d="M5 13l4 4L19 7" /></svg>
										{/if}
									</button>
								{/each}
							</div>
						{/if}
					</div>

					<!-- Exempt Channels -->
					<div class="mb-3">
						<label class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">Exempt Channels</label>
						<p class="mb-1.5 text-xs text-text-muted">Messages in these channels will not be checked by this rule.</p>
						{#if loadingAutomodMeta}
							<p class="text-xs text-text-muted">Loading channels...</p>
						{:else if automodGuildChannels.length === 0}
							<p class="text-xs text-text-muted">No channels available.</p>
						{:else}
							<div class="flex flex-wrap gap-1.5">
								{#each automodGuildChannels.filter(c => c.channel_type === 'text' || c.channel_type === 'announcement' || c.channel_type === 'forum') as channel (channel.id)}
									<button
										type="button"
										class="flex items-center gap-1.5 rounded-full px-2.5 py-1 text-xs transition-colors {newRuleExemptChannels.includes(channel.id) ? 'bg-brand-500/20 text-brand-400 ring-1 ring-brand-500/40' : 'bg-bg-modifier text-text-muted hover:bg-bg-tertiary hover:text-text-secondary'}"
										onclick={() => { newRuleExemptChannels = toggleArrayItem(newRuleExemptChannels, channel.id); }}
									>
										<span class="text-text-muted">#</span>
										{channel.name ?? 'unnamed'}
										{#if newRuleExemptChannels.includes(channel.id)}
											<svg class="h-3 w-3" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24"><path d="M5 13l4 4L19 7" /></svg>
										{/if}
									</button>
								{/each}
							</div>
						{/if}
					</div>

					<button class="btn-primary" onclick={handleCreateAutomodRule} disabled={creatingRule || !newRuleName.trim()}>
						{creatingRule ? 'Creating...' : 'Create Rule'}
					</button>
				</div>

				{#if loadingAutomod}
					<p class="text-sm text-text-muted">Loading AutoMod rules...</p>
				{:else if automodRules.length === 0}
					<p class="text-sm text-text-muted">No AutoMod rules configured.</p>
				{:else}
					<div class="space-y-2">
						{#each automodRules as rule (rule.id)}
							<div class="rounded-lg bg-bg-secondary p-3">
								<div class="flex items-center justify-between">
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
										<button
											class="text-xs text-brand-400 hover:text-brand-300"
											onclick={() => startEditingExemptions(rule)}
										>
											{editingExemptRuleId === rule.id ? 'Editing...' : 'Exemptions'}
										</button>
										<button class="text-xs text-red-400 hover:text-red-300" onclick={() => handleDeleteAutomodRule(rule.id)}>
											Delete
										</button>
									</div>
								</div>

								<!-- Show current exemptions summary (when not editing) -->
								{#if editingExemptRuleId !== rule.id && ((rule.exempt_roles && rule.exempt_roles.length > 0) || (rule.exempt_channels && rule.exempt_channels.length > 0))}
									<div class="mt-2 border-t border-bg-modifier pt-2">
										{#if rule.exempt_roles && rule.exempt_roles.length > 0}
											<div class="mb-1 flex flex-wrap items-center gap-1">
												<span class="text-xs text-text-muted">Exempt roles:</span>
												{#each rule.exempt_roles as roleId}
													<span class="rounded-full bg-bg-modifier px-2 py-0.5 text-xs text-text-secondary">
														{getRoleName(roleId)}
													</span>
												{/each}
											</div>
										{/if}
										{#if rule.exempt_channels && rule.exempt_channels.length > 0}
											<div class="flex flex-wrap items-center gap-1">
												<span class="text-xs text-text-muted">Exempt channels:</span>
												{#each rule.exempt_channels as channelId}
													<span class="rounded-full bg-bg-modifier px-2 py-0.5 text-xs text-text-secondary">
														#{getChannelName(channelId)}
													</span>
												{/each}
											</div>
										{/if}
									</div>
								{/if}

								<!-- Inline editing of exemptions -->
								{#if editingExemptRuleId === rule.id}
									<div class="mt-3 border-t border-bg-modifier pt-3">
										<!-- Exempt Roles Editor -->
										<div class="mb-3">
											<label class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">Exempt Roles</label>
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
																<svg class="h-3 w-3" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24"><path d="M5 13l4 4L19 7" /></svg>
															{/if}
														</button>
													{/each}
												</div>
											{/if}
										</div>

										<!-- Exempt Channels Editor -->
										<div class="mb-3">
											<label class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">Exempt Channels</label>
											{#if automodGuildChannels.length === 0}
												<p class="text-xs text-text-muted">No channels available.</p>
											{:else}
												<div class="flex flex-wrap gap-1.5">
													{#each automodGuildChannels.filter(c => c.channel_type === 'text' || c.channel_type === 'announcement' || c.channel_type === 'forum') as channel (channel.id)}
														<button
															type="button"
															class="flex items-center gap-1.5 rounded-full px-2.5 py-1 text-xs transition-colors {editingExemptChannels.includes(channel.id) ? 'bg-brand-500/20 text-brand-400 ring-1 ring-brand-500/40' : 'bg-bg-modifier text-text-muted hover:bg-bg-tertiary hover:text-text-secondary'}"
															onclick={() => { editingExemptChannels = toggleArrayItem(editingExemptChannels, channel.id); }}
														>
															<span class="text-text-muted">#</span>
															{channel.name ?? 'unnamed'}
															{#if editingExemptChannels.includes(channel.id)}
																<svg class="h-3 w-3" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24"><path d="M5 13l4 4L19 7" /></svg>
															{/if}
														</button>
													{/each}
												</div>
											{/if}
										</div>

										<div class="flex gap-2">
											<button class="btn-primary text-xs" onclick={handleSaveExemptions}>
												Save Exemptions
											</button>
											<button class="btn-secondary text-xs" onclick={cancelEditingExemptions}>
												Cancel
											</button>
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

			<!-- ==================== MODERATION ==================== -->
			{:else if currentTab === 'moderation'}
				<h1 class="mb-6 text-xl font-bold text-text-primary">Moderation</h1>

				<div class="mb-6">
					<h2 class="mb-3 text-lg font-semibold text-text-primary">Message Reports</h2>
					<div class="mb-3 flex items-center gap-2">
						<select class="input" bind:value={reportFilter} onchange={handleFilterReports}>
							<option value="open">Open</option>
							<option value="resolved">Resolved</option>
							<option value="dismissed">Dismissed</option>
							<option value="">All</option>
						</select>
					</div>

					{#if loadingReports}
						<p class="text-sm text-text-muted">Loading reports...</p>
					{:else if reports.length === 0}
						<p class="text-sm text-text-muted">No reports found.</p>
					{:else}
						<div class="space-y-2">
							{#each reports as report (report.id)}
								<div class="rounded-lg bg-bg-secondary p-3">
									<div class="flex items-center justify-between">
										<div>
											<span class="text-sm text-text-primary">{report.reason}</span>
											<div class="mt-1 flex gap-2 text-xs text-text-muted">
												<span class="rounded px-1.5 py-0.5 {report.status === 'open' ? 'bg-yellow-500/20 text-yellow-400' : report.status === 'resolved' ? 'bg-green-500/20 text-green-400' : 'bg-text-muted/20'}">
													{report.status}
												</span>
												<span>Channel: {report.channel_id.slice(0, 8)}...</span>
												<span>{formatDate(report.created_at)}</span>
											</div>
										</div>
										{#if report.status === 'open'}
											<div class="flex gap-2">
												<button class="text-xs text-green-400 hover:text-green-300" onclick={() => handleResolveReport(report.id, 'resolved')}>
													Resolve
												</button>
												<button class="text-xs text-text-muted hover:text-text-secondary" onclick={() => handleResolveReport(report.id, 'dismissed')}>
													Dismiss
												</button>
											</div>
										{/if}
									</div>
								</div>
							{/each}
						</div>
					{/if}
				</div>

			<!-- ==================== RAID PROTECTION ==================== -->
			{:else if currentTab === 'raid'}
				<h1 class="mb-6 text-xl font-bold text-text-primary">Raid Protection</h1>

				{#if loadingRaid}
					<p class="text-sm text-text-muted">Loading raid configuration...</p>
				{:else if raidConfig}
					<div class="space-y-6">
						<label class="flex items-center gap-3">
							<input type="checkbox" bind:checked={raidConfig.enabled} class="rounded" />
							<div>
								<span class="text-sm font-medium text-text-primary">Enable Raid Protection</span>
								<p class="text-xs text-text-muted">Automatically detect and respond to join raids</p>
							</div>
						</label>

						{#if raidConfig.enabled}
							<div class="grid grid-cols-2 gap-4">
								<div>
									<label class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">Max Joins Per Window</label>
									<input type="number" class="input w-full" bind:value={raidConfig.join_rate_limit} min="1" max="100" />
								</div>
								<div>
									<label class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">Window (seconds)</label>
									<input type="number" class="input w-full" bind:value={raidConfig.join_rate_window} min="5" max="300" />
								</div>
								<div>
									<label class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">Min Account Age (seconds)</label>
									<input type="number" class="input w-full" bind:value={raidConfig.min_account_age} min="0" max="604800" />
									<p class="mt-1 text-xs text-text-muted">0 = no requirement. 300 = 5 minutes.</p>
								</div>
							</div>
						{/if}

						<div class="border-t border-bg-modifier pt-4">
							<label class="flex items-center gap-3">
								<input type="checkbox" bind:checked={raidConfig.lockdown_active} class="rounded" />
								<div>
									<span class="text-sm font-medium text-text-primary {raidConfig.lockdown_active ? 'text-red-400' : ''}">
										{raidConfig.lockdown_active ? 'Lockdown Active' : 'Manual Lockdown'}
									</span>
									<p class="text-xs text-text-muted">When enabled, new joins are blocked and invites are paused</p>
									{#if raidConfig.lockdown_active && raidConfig.lockdown_started_at}
										<p class="text-xs text-red-400">Started: {formatDate(raidConfig.lockdown_started_at)}</p>
									{/if}
								</div>
							</label>
						</div>

						<button class="btn-primary" onclick={handleSaveRaid} disabled={savingRaid}>
							{savingRaid ? 'Saving...' : 'Save Raid Settings'}
						</button>
					</div>
				{/if}

			<!-- ==================== ONBOARDING ==================== -->
			{:else if currentTab === 'onboarding'}
				<h1 class="mb-6 text-xl font-bold text-text-primary">Onboarding</h1>
				<p class="mb-6 text-sm text-text-muted">
					Configure the onboarding flow that new members see when they join your server. You can set a welcome message, rules, and custom prompts to personalize their experience.
				</p>

				{#if loadingOnboarding}
					<p class="text-sm text-text-muted">Loading onboarding configuration...</p>
				{:else if onboardingConfig}
					<!-- Enable/Disable Toggle -->
					<label class="mb-6 flex items-center gap-3">
						<input type="checkbox" bind:checked={onboardingConfig.enabled} class="rounded" />
						<div>
							<span class="text-sm font-medium text-text-primary">Enable Onboarding</span>
							<p class="text-xs text-text-muted">When enabled, new members will see the onboarding flow after joining</p>
						</div>
					</label>

					<!-- Welcome Message -->
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

					<!-- Rules -->
					<div class="mb-6">
						<label class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">Server Rules</label>
						<p class="mb-2 text-xs text-text-muted">New members must accept these rules during onboarding.</p>

						{#if onboardingConfig.rules.length > 0}
							<div class="mb-3 space-y-2">
								{#each onboardingConfig.rules as rule, i}
									<div class="flex items-center gap-2 rounded-lg bg-bg-primary p-2.5">
										<span class="flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-brand-600/20 text-xs font-bold text-brand-400">
											{i + 1}
										</span>
										<span class="min-w-0 flex-1 text-sm text-text-primary">{rule}</span>
										<div class="flex shrink-0 items-center gap-1">
											{#if i > 0}
												<button
													class="text-text-muted hover:text-text-primary"
													onclick={() => moveOnboardingRule(i, 'up')}
													title="Move up"
												>
													<svg class="h-3.5 w-3.5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24"><path d="M18 15l-6-6-6 6" /></svg>
												</button>
											{/if}
											{#if i < onboardingConfig.rules.length - 1}
												<button
													class="text-text-muted hover:text-text-primary"
													onclick={() => moveOnboardingRule(i, 'down')}
													title="Move down"
												>
													<svg class="h-3.5 w-3.5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24"><path d="M6 9l6 6 6-6" /></svg>
												</button>
											{/if}
											<button
												class="text-red-400 hover:text-red-300"
												onclick={() => removeOnboardingRule(i)}
												title="Remove rule"
											>
												<svg class="h-3.5 w-3.5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24"><path d="M6 18L18 6M6 6l12 12" /></svg>
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
								onkeydown={(e) => e.key === 'Enter' && addOnboardingRule()}
							/>
							<button class="btn-primary" onclick={addOnboardingRule} disabled={!newRuleText.trim()}>
								Add
							</button>
						</div>
					</div>

					<!-- Default Channels -->
					<div class="mb-6">
						<label class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">Default Channels</label>
						<p class="mb-2 text-xs text-text-muted">Channels that new members are automatically added to after onboarding.</p>
						{#if onboardingChannels.length === 0}
							<p class="text-xs text-text-muted">No channels available.</p>
						{:else}
							<div class="flex flex-wrap gap-1.5">
								{#each onboardingChannels.filter((c) => c.channel_type === 'text' || c.channel_type === 'announcement') as channel (channel.id)}
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
											<svg class="h-3 w-3" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24"><path d="M5 13l4 4L19 7" /></svg>
										{/if}
									</button>
								{/each}
							</div>
						{/if}
					</div>

					<button class="btn-primary mb-8" onclick={handleSaveOnboarding} disabled={savingOnboarding}>
						{savingOnboarding ? 'Saving...' : 'Save Onboarding Settings'}
					</button>

					<!-- Prompts Section -->
					<div class="border-t border-bg-modifier pt-6">
						<h2 class="mb-2 text-lg font-semibold text-text-primary">Prompts</h2>
						<p class="mb-4 text-sm text-text-muted">
							Prompts let new members customize their experience by choosing roles and channels. Each prompt is shown as a separate step during onboarding.
						</p>

						<!-- Create Prompt -->
						<div class="mb-6 rounded-lg bg-bg-primary p-4">
							<h3 class="mb-3 text-sm font-semibold text-text-primary">Create Prompt</h3>
							<div class="mb-3">
								<label class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">Title</label>
								<input
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

						<!-- Existing Prompts -->
						{#if onboardingConfig.prompts.length === 0}
							<p class="text-sm text-text-muted">No prompts configured yet. Create one above to get started.</p>
						{:else}
							<div class="space-y-4">
								{#each onboardingConfig.prompts.slice().sort((a, b) => a.position - b.position) as prompt (prompt.id)}
									<div class="rounded-lg bg-bg-primary p-4">
										<!-- Prompt Header -->
										{#if editingPromptId === prompt.id}
											<div class="mb-3">
												<input
													type="text"
													class="input mb-2 w-full"
													bind:value={editingPromptTitle}
													onkeydown={(e) => e.key === 'Enter' && handleSavePrompt()}
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
											<div class="mb-3 flex items-center justify-between">
												<div>
													<h3 class="text-sm font-semibold text-text-primary">{prompt.title}</h3>
													<div class="mt-0.5 flex gap-2 text-xs text-text-muted">
														{#if prompt.required}
															<span class="rounded bg-brand-500/15 px-1.5 py-0.5 text-brand-400">Required</span>
														{/if}
														<span class="rounded bg-bg-modifier px-1.5 py-0.5">{prompt.single_select ? 'Single select' : 'Multi select'}</span>
														<span class="rounded bg-bg-modifier px-1.5 py-0.5">Pos: {prompt.position}</span>
													</div>
												</div>
												<div class="flex items-center gap-2">
													<button
														class="text-xs text-brand-400 hover:text-brand-300"
														onclick={() => startEditingPrompt(prompt)}
													>
														Edit
													</button>
													<button
														class="text-xs text-red-400 hover:text-red-300"
														onclick={() => handleDeletePrompt(prompt.id)}
													>
														Delete
													</button>
												</div>
											</div>
										{/if}

										<!-- Options -->
										<div class="mb-2 text-xs font-bold uppercase tracking-wide text-text-muted">Options ({prompt.options.length})</div>
										{#if prompt.options.length > 0}
											<div class="mb-3 space-y-1.5">
												{#each prompt.options as option (option.id)}
													<div class="flex items-center justify-between rounded-md bg-bg-secondary p-2.5">
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
														<button
															class="shrink-0 text-xs text-red-400 hover:text-red-300"
															onclick={() => handleRemoveOption(prompt.id, option.id)}
														>
															Remove
														</button>
													</div>
												{/each}
											</div>
										{:else}
											<p class="mb-3 text-xs text-text-muted">No options yet. Add one below.</p>
										{/if}

										<!-- Add Option Form -->
										{#if addingOptionToPromptId === prompt.id}
											<div class="rounded-md border border-bg-modifier bg-bg-secondary p-3">
												<h4 class="mb-2 text-xs font-bold uppercase tracking-wide text-text-muted">New Option</h4>
												<div class="mb-2 grid grid-cols-2 gap-2">
													<div>
														<label class="mb-1 block text-xs text-text-muted">Label</label>
														<input type="text" class="input w-full" bind:value={newOptionLabel} placeholder="Option label" maxlength="100" />
													</div>
													<div>
														<label class="mb-1 block text-xs text-text-muted">Emoji (optional)</label>
														<input type="text" class="input w-full" bind:value={newOptionEmoji} placeholder="e.g. a single emoji" maxlength="4" />
													</div>
												</div>
												<div class="mb-2">
													<label class="mb-1 block text-xs text-text-muted">Description (optional)</label>
													<input type="text" class="input w-full" bind:value={newOptionDescription} placeholder="Short description" maxlength="200" />
												</div>

												<!-- Role assignment -->
												<div class="mb-2">
													<label class="mb-1 block text-xs text-text-muted">Assign Roles</label>
													<div class="flex flex-wrap gap-1">
														{#each onboardingRoles as role (role.id)}
															<button
																type="button"
																class="flex items-center gap-1 rounded-full px-2 py-0.5 text-xs transition-colors {newOptionRoleIds.includes(role.id)
																	? 'bg-brand-500/20 text-brand-400 ring-1 ring-brand-500/40'
																	: 'bg-bg-modifier text-text-muted hover:bg-bg-tertiary'}"
																onclick={() => {
																	newOptionRoleIds = newOptionRoleIds.includes(role.id)
																		? newOptionRoleIds.filter((id) => id !== role.id)
																		: [...newOptionRoleIds, role.id];
																}}
															>
																<span class="h-2 w-2 rounded-full" style="background-color: {role.color ?? '#99aab5'}"></span>
																{role.name}
															</button>
														{/each}
													</div>
												</div>

												<!-- Channel assignment -->
												<div class="mb-3">
													<label class="mb-1 block text-xs text-text-muted">Grant Channel Access</label>
													<div class="flex flex-wrap gap-1">
														{#each onboardingChannels.filter((c) => c.channel_type === 'text' || c.channel_type === 'announcement') as channel (channel.id)}
															<button
																type="button"
																class="flex items-center gap-1 rounded-full px-2 py-0.5 text-xs transition-colors {newOptionChannelIds.includes(channel.id)
																	? 'bg-brand-500/20 text-brand-400 ring-1 ring-brand-500/40'
																	: 'bg-bg-modifier text-text-muted hover:bg-bg-tertiary'}"
																onclick={() => {
																	newOptionChannelIds = newOptionChannelIds.includes(channel.id)
																		? newOptionChannelIds.filter((id) => id !== channel.id)
																		: [...newOptionChannelIds, channel.id];
																}}
															>
																# {channel.name ?? 'unnamed'}
															</button>
														{/each}
													</div>
												</div>

												<div class="flex gap-2">
													<button class="btn-primary text-xs" onclick={handleAddOption} disabled={!newOptionLabel.trim()}>
														Add Option
													</button>
													<button class="btn-secondary text-xs" onclick={cancelAddingOption}>
														Cancel
													</button>
												</div>
											</div>
										{:else}
											<button
												class="text-xs text-brand-400 hover:text-brand-300"
												onclick={() => startAddingOption(prompt.id)}
											>
												+ Add option
											</button>
										{/if}
									</div>
								{/each}
							</div>
						{/if}
					</div>
				{/if}
			{/if}
		</div>
	</div>
</div>
