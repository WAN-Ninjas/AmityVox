<script lang="ts">
	import { page } from '$app/stores';
	import { currentGuild, updateGuild } from '$lib/stores/guilds';
	import { currentUser } from '$lib/stores/auth';
	import { api } from '$lib/api/client';
	import { goto } from '$app/navigation';
	import Avatar from '$components/common/Avatar.svelte';
	import type { Role, Invite, Ban, AuditLogEntry, CustomEmoji, Webhook, Category } from '$lib/types';

	type Tab = 'overview' | 'roles' | 'categories' | 'invites' | 'bans' | 'emoji' | 'webhooks' | 'audit';
	let currentTab = $state<Tab>('overview');

	// --- Overview ---
	let name = $state('');
	let description = $state('');
	let iconFile = $state<File | null>(null);
	let iconPreview = $state<string | null>(null);
	let saving = $state(false);
	let error = $state('');
	let success = $state('');
	let deleteConfirm = $state('');

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

	const isOwner = $derived($currentGuild?.owner_id === $currentUser?.id);

	$effect(() => {
		if ($currentGuild) {
			name = $currentGuild.name;
			description = $currentGuild.description ?? '';
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
				name, description: description || undefined
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

	// --- Helpers ---

	const tabs: { id: Tab; label: string }[] = [
		{ id: 'overview', label: 'Overview' },
		{ id: 'roles', label: 'Roles' },
		{ id: 'categories', label: 'Categories' },
		{ id: 'invites', label: 'Invites' },
		{ id: 'bans', label: 'Bans' },
		{ id: 'emoji', label: 'Emoji' },
		{ id: 'webhooks', label: 'Webhooks' },
		{ id: 'audit', label: 'Audit Log' }
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
			{/if}
		</div>
	</div>
</div>
