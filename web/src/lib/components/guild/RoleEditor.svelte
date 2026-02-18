<script lang="ts">
	import type { Role } from '$lib/types';
	import { api } from '$lib/api/client';
	import { createAsyncOp } from '$lib/utils/asyncOp';

	let {
		guildId,
		roles = $bindable([]),
		onError = (_msg: string) => {},
		onSuccess = (_msg: string) => {}
	}: {
		guildId: string;
		roles: Role[];
		onError: (msg: string) => void;
		onSuccess: (msg: string) => void;
	} = $props();

	// --- Permission definitions ---

	interface PermDef {
		key: string;
		label: string;
		bit: bigint;
	}

	interface PermGroup {
		name: string;
		perms: PermDef[];
	}

	const permissionGroups: PermGroup[] = [
		{
			name: 'Server',
			perms: [
				{ key: 'ManageGuild', label: 'Manage Server', bit: 1n << 1n },
				{ key: 'ManageChannels', label: 'Manage Channels', bit: 1n << 0n },
				{ key: 'ManageEmoji', label: 'Manage Emoji', bit: 1n << 4n },
				{ key: 'ManageWebhooks', label: 'Manage Webhooks', bit: 1n << 5n },
				{ key: 'CreateInvites', label: 'Create Invites', bit: 1n << 37n },
			]
		},
		{
			name: 'Members',
			perms: [
				{ key: 'KickMembers', label: 'Kick Members', bit: 1n << 6n },
				{ key: 'BanMembers', label: 'Ban Members', bit: 1n << 7n },
				{ key: 'TimeoutMembers', label: 'Timeout Members', bit: 1n << 8n },
				{ key: 'ManageRoles', label: 'Manage Roles', bit: 1n << 3n },
				{ key: 'AssignRoles', label: 'Assign Roles', bit: 1n << 9n },
				{ key: 'ManageNicknames', label: 'Manage Nicknames', bit: 1n << 11n },
				{ key: 'RemoveAvatars', label: 'Remove Avatars', bit: 1n << 13n },
			]
		},
		{
			name: 'Information',
			perms: [
				{ key: 'ViewAuditLog', label: 'View Audit Log', bit: 1n << 14n },
				{ key: 'ViewGuildInsights', label: 'View Insights', bit: 1n << 15n },
				{ key: 'MentionEveryone', label: 'Mention @everyone', bit: 1n << 16n },
				{ key: 'ManagePermissions', label: 'Manage Permissions', bit: 1n << 2n },
			]
		},
		{
			name: 'Channel',
			perms: [
				{ key: 'ViewChannel', label: 'View Channels', bit: 1n << 20n },
				{ key: 'ReadHistory', label: 'Read History', bit: 1n << 21n },
				{ key: 'SendMessages', label: 'Send Messages', bit: 1n << 22n },
				{ key: 'ManageMessages', label: 'Manage Messages', bit: 1n << 23n },
				{ key: 'EmbedLinks', label: 'Embed Links', bit: 1n << 24n },
				{ key: 'UploadFiles', label: 'Upload Files', bit: 1n << 25n },
				{ key: 'AddReactions', label: 'Add Reactions', bit: 1n << 26n },
				{ key: 'UseExternalEmoji', label: 'External Emoji', bit: 1n << 27n },
				{ key: 'Masquerade', label: 'Masquerade', bit: 1n << 36n },
				{ key: 'ManageThreads', label: 'Manage Threads', bit: 1n << 38n },
				{ key: 'CreateThreads', label: 'Create Threads', bit: 1n << 39n },
			]
		},
		{
			name: 'Voice',
			perms: [
				{ key: 'Connect', label: 'Connect', bit: 1n << 28n },
				{ key: 'Speak', label: 'Speak', bit: 1n << 29n },
				{ key: 'MuteMembers', label: 'Mute Members', bit: 1n << 30n },
				{ key: 'DeafenMembers', label: 'Deafen Members', bit: 1n << 31n },
				{ key: 'MoveMembers', label: 'Move Members', bit: 1n << 32n },
				{ key: 'UseVAD', label: 'Voice Activity', bit: 1n << 33n },
				{ key: 'PrioritySpeaker', label: 'Priority Speaker', bit: 1n << 34n },
				{ key: 'Stream', label: 'Stream', bit: 1n << 35n },
			]
		},
		{
			name: 'Special',
			perms: [
				{ key: 'ChangeNickname', label: 'Change Own Nickname', bit: 1n << 10n },
				{ key: 'ChangeAvatar', label: 'Change Own Avatar', bit: 1n << 12n },
				{ key: 'Administrator', label: 'Administrator', bit: 1n << 63n },
			]
		}
	];

	// --- State ---

	let selectedRoleId = $state<string | null>(null);
	let editName = $state('');
	let editColor = $state('#99aab5');
	let editHoist = $state(false);
	let editMentionable = $state(false);
	let editAllow = $state(0n);
	let editDeny = $state(0n);
	let saveOp = $state(createAsyncOp());
	let reorderOp = $state(createAsyncOp());

	let newRoleName = $state('');
	let newRoleColor = $state('#99aab5');
	let createRoleOp = $state(createAsyncOp());

	const sortedRoles = $derived([...roles].sort((a, b) => b.position - a.position));
	const selectedRole = $derived(roles.find((r) => r.id === selectedRoleId) ?? null);
	const isEveryone = $derived(selectedRole?.name === '@everyone' && selectedRole?.position === 0);

	// --- Permission helpers (exported for testing) ---

	export function permState(allow: bigint, deny: bigint, bit: bigint): 'allow' | 'deny' | 'neutral' {
		if ((allow & bit) === bit) return 'allow';
		if ((deny & bit) === bit) return 'deny';
		return 'neutral';
	}

	export function toggleAllow(allow: bigint, deny: bigint, bit: bigint): { allow: bigint; deny: bigint } {
		if ((allow & bit) === bit) {
			return { allow: allow & ~bit, deny };
		}
		return { allow: allow | bit, deny: deny & ~bit };
	}

	export function toggleDeny(allow: bigint, deny: bigint, bit: bigint): { allow: bigint; deny: bigint } {
		if ((deny & bit) === bit) {
			return { allow, deny: deny & ~bit };
		}
		return { allow: allow & ~bit, deny: deny | bit };
	}

	// --- Selection ---

	function selectRole(role: Role) {
		selectedRoleId = role.id;
		editName = role.name;
		editColor = role.color ?? '#99aab5';
		editHoist = role.hoist;
		editMentionable = role.mentionable;
		editAllow = BigInt(role.permissions_allow);
		editDeny = BigInt(role.permissions_deny);
	}

	// --- Reorder helpers ---

	let draggedRoleId = $state<string | null>(null);
	let dropTargetIdx = $state<number | null>(null);

	async function moveRoleUp(role: Role) {
		const sorted = sortedRoles;
		const idx = sorted.findIndex((r) => r.id === role.id);
		if (idx <= 0) return;
		await reorderSwap(role, sorted[idx - 1]);
	}

	async function moveRoleDown(role: Role) {
		const sorted = sortedRoles;
		const idx = sorted.findIndex((r) => r.id === role.id);
		if (idx < 0 || idx >= sorted.length - 1) return;
		const below = sorted[idx + 1];
		if (below.name === '@everyone' && below.position === 0) return;
		await reorderSwap(role, below);
	}

	async function reorderSwap(a: Role, b: Role) {
		const updated = await reorderOp.run(
			() => api.reorderRoles(guildId, [
				{ id: a.id, position: b.position },
				{ id: b.id, position: a.position },
			]),
			msg => onError(msg)
		);
		if (updated) {
			roles = updated;
			onSuccess('Roles reordered');
		}
	}

	function canMoveUp(role: Role): boolean {
		const idx = sortedRoles.findIndex((r) => r.id === role.id);
		return idx > 0;
	}

	function canMoveDown(role: Role): boolean {
		if (role.name === '@everyone' && role.position === 0) return false;
		const idx = sortedRoles.findIndex((r) => r.id === role.id);
		if (idx < 0 || idx >= sortedRoles.length - 1) return false;
		const below = sortedRoles[idx + 1];
		return !(below.name === '@everyone' && below.position === 0);
	}

	// --- Drag-and-drop ---

	function handleDragStart(e: DragEvent, roleId: string) {
		draggedRoleId = roleId;
		if (e.dataTransfer) {
			e.dataTransfer.effectAllowed = 'move';
			e.dataTransfer.setData('text/plain', roleId);
		}
	}

	function handleDragOver(e: DragEvent, idx: number) {
		e.preventDefault();
		if (e.dataTransfer) e.dataTransfer.dropEffect = 'move';
		// Don't allow dropping onto @everyone
		const targetRole = sortedRoles[idx];
		if (targetRole?.name === '@everyone' && targetRole?.position === 0) return;
		dropTargetIdx = idx;
	}

	function handleDragLeave() {
		dropTargetIdx = null;
	}

	function handleDragEnd() {
		draggedRoleId = null;
		dropTargetIdx = null;
	}

	async function handleDrop(e: DragEvent, targetIdx: number) {
		e.preventDefault();
		if (!draggedRoleId) return;
		const sorted = sortedRoles;
		const sourceIdx = sorted.findIndex((r) => r.id === draggedRoleId);
		if (sourceIdx < 0 || sourceIdx === targetIdx) {
			handleDragEnd();
			return;
		}

		// Don't allow dropping onto @everyone
		const targetRole = sorted[targetIdx];
		if (targetRole?.name === '@everyone' && targetRole?.position === 0) {
			handleDragEnd();
			return;
		}

		// Build new position assignments for all affected roles
		const reordered = [...sorted];
		const [moved] = reordered.splice(sourceIdx, 1);
		reordered.splice(targetIdx, 0, moved);

		// Assign positions: highest index in sorted = highest position
		const updates: { id: string; position: number }[] = [];
		for (let i = 0; i < reordered.length; i++) {
			const newPos = reordered.length - 1 - i;
			// Skip @everyone (always position 0)
			if (reordered[i].name === '@everyone' && reordered[i].position === 0) continue;
			if (reordered[i].position !== newPos) {
				updates.push({ id: reordered[i].id, position: newPos });
			}
		}

		handleDragEnd();
		if (updates.length === 0) return;

		const updated = await reorderOp.run(
			() => api.reorderRoles(guildId, updates),
			msg => onError(msg)
		);
		if (updated) {
			roles = updated;
			onSuccess('Roles reordered');
		}
	}

	// --- Actions ---

	async function handleCreateRole() {
		if (!newRoleName.trim()) return;
		const role = await createRoleOp.run(
			() => api.createRole(guildId, newRoleName.trim(), {
				color: newRoleColor !== '#99aab5' ? newRoleColor : undefined
			}),
			msg => onError(msg)
		);
		if (role) {
			roles = [...roles, role];
			newRoleName = '';
			newRoleColor = '#99aab5';
			onSuccess('Role created');
		}
	}

	async function handleSave() {
		if (!selectedRoleId || !editName.trim()) return;
		const data: Record<string, any> = {
			color: editColor,
			hoist: editHoist,
			mentionable: editMentionable,
			permissions_allow: editAllow.toString(),
			permissions_deny: editDeny.toString()
		};
		// Don't allow renaming @everyone
		if (!isEveryone) {
			data.name = editName.trim();
		}
		const roleId = selectedRoleId;
		const updated = await saveOp.run(
			() => api.updateRole(guildId, roleId, data),
			msg => onError(msg)
		);
		if (updated) {
			roles = roles.map((r) => (r.id === roleId ? updated : r));
			onSuccess('Role updated');
		}
	}

	async function handleDelete() {
		if (!selectedRoleId || !confirm('Delete this role? This cannot be undone.')) return;
		try {
			await api.deleteRole(guildId, selectedRoleId);
			roles = roles.filter((r) => r.id !== selectedRoleId);
			selectedRoleId = null;
			onSuccess('Role deleted');
		} catch (err: any) {
			onError(err.message || 'Failed to delete role');
		}
	}

	function handlePermToggle(bit: bigint, type: 'allow' | 'deny') {
		if (type === 'allow') {
			const result = toggleAllow(editAllow, editDeny, bit);
			editAllow = result.allow;
			editDeny = result.deny;
		} else {
			const result = toggleDeny(editAllow, editDeny, bit);
			editAllow = result.allow;
			editDeny = result.deny;
		}
	}
</script>

<div class="flex gap-4" style="min-height: 500px;">
	<!-- Left column: role list -->
	<div class="w-64 shrink-0 space-y-3">
		<!-- Create role form -->
		<div class="space-y-2">
			<div class="flex gap-2">
				<input
					type="text" class="input flex-1" placeholder="New role name..."
					bind:value={newRoleName} maxlength="100"
					onkeydown={(e) => e.key === 'Enter' && handleCreateRole()}
				/>
				<input type="color" class="h-9 w-9 cursor-pointer rounded border border-border-primary bg-bg-secondary" bind:value={newRoleColor} title="Role color" />
			</div>
			<button class="btn-primary w-full text-sm" onclick={handleCreateRole} disabled={createRoleOp.loading || !newRoleName.trim()}>
				{createRoleOp.loading ? 'Creating...' : 'Create Role'}
			</button>
		</div>

		<!-- Role list -->
		<div class="space-y-0.5">
			{#if sortedRoles.length === 0}
				<p class="py-4 text-center text-sm text-text-muted">No custom roles yet.</p>
			{/if}
			{#each sortedRoles as role, idx (role.id)}
				{@const isEveryoneRole = role.name === '@everyone' && role.position === 0}
				{#if isEveryoneRole}
					<!-- @everyone is always pinned at the bottom, no reorderOp.loading -->
					<div class="mt-2 border-t border-bg-modifier pt-2">
						<button
							class="flex w-full items-center gap-2.5 rounded-lg px-3 py-2 text-left text-sm transition-colors {selectedRoleId === role.id ? 'bg-brand-500/20 text-text-primary' : 'text-text-secondary hover:bg-bg-modifier'}"
							onclick={() => selectRole(role)}
						>
							<div class="h-3 w-3 shrink-0 rounded-full" style="background-color: {role.color ?? '#99aab5'}"></div>
							<span class="truncate">{role.name}</span>
							<span class="ml-auto text-2xs text-text-muted">Base</span>
						</button>
					</div>
				{:else}
					{@const isDragging = draggedRoleId === role.id}
					{@const isDropTarget = dropTargetIdx === idx && draggedRoleId !== role.id}
					<!-- svelte-ignore a11y_no_static_element_interactions -->
					<div
						class="flex items-center gap-0.5 rounded-lg transition-all {isDragging ? 'opacity-40' : ''} {isDropTarget ? 'border-t-2 border-brand-500' : 'border-t-2 border-transparent'}"
						draggable={true}
						ondragstart={(e) => handleDragStart(e, role.id)}
						ondragover={(e) => handleDragOver(e, idx)}
						ondragleave={handleDragLeave}
						ondrop={(e) => handleDrop(e, idx)}
						ondragend={handleDragEnd}
					>
						<!-- Drag handle -->
						<div class="flex shrink-0 cursor-grab items-center px-1 text-text-muted/50 hover:text-text-muted active:cursor-grabbing">
							<svg class="h-4 w-4" fill="currentColor" viewBox="0 0 24 24">
								<circle cx="9" cy="6" r="1.5" />
								<circle cx="15" cy="6" r="1.5" />
								<circle cx="9" cy="12" r="1.5" />
								<circle cx="15" cy="12" r="1.5" />
								<circle cx="9" cy="18" r="1.5" />
								<circle cx="15" cy="18" r="1.5" />
							</svg>
						</div>

						<button
							class="flex flex-1 items-center gap-2.5 rounded-lg px-2 py-2 text-left text-sm transition-colors {selectedRoleId === role.id ? 'bg-brand-500/20 text-text-primary' : 'text-text-secondary hover:bg-bg-modifier'}"
							onclick={() => selectRole(role)}
						>
							<div class="h-3 w-3 shrink-0 rounded-full" style="background-color: {role.color ?? '#99aab5'}"></div>
							<span class="truncate">{role.name}</span>
						</button>

						<!-- Reorder arrows -->
						<div class="flex shrink-0 flex-col">
							<button
								class="rounded p-0.5 text-text-muted hover:text-text-primary disabled:opacity-25 disabled:hover:text-text-muted"
								title="Move up one"
								disabled={reorderOp.loading || !canMoveUp(role)}
								onclick={() => moveRoleUp(role)}
							>
								<svg class="h-3.5 w-3.5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
									<path d="M5 15l7-7 7 7" />
								</svg>
							</button>
							<button
								class="rounded p-0.5 text-text-muted hover:text-text-primary disabled:opacity-25 disabled:hover:text-text-muted"
								title="Move down one"
								disabled={reorderOp.loading || !canMoveDown(role)}
								onclick={() => moveRoleDown(role)}
							>
								<svg class="h-3.5 w-3.5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
									<path d="M19 9l-7 7-7-7" />
								</svg>
							</button>
						</div>
					</div>
				{/if}
			{/each}
		</div>
	</div>

	<!-- Right column: edit panel -->
	<div class="flex-1 overflow-y-auto">
		{#if !selectedRole}
			<div class="flex h-full items-center justify-center">
				<p class="text-sm text-text-muted">Select a role to edit its properties and permissions.</p>
			</div>
		{:else}
			<div class="space-y-6">
				<!-- Basic properties -->
				<div class="rounded-lg bg-bg-secondary p-4">
					<h3 class="mb-3 text-sm font-semibold text-text-primary">Role Settings</h3>
					<div class="grid grid-cols-2 gap-4">
						<div>
							<label class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">Name</label>
							{#if isEveryone}
								<input type="text" class="input w-full cursor-not-allowed opacity-60" value="@everyone" disabled />
							{:else}
								<input type="text" class="input w-full" bind:value={editName} maxlength="100" />
							{/if}
						</div>
						<div>
							<label class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">Color</label>
							<div class="flex items-center gap-2">
								<input type="color" class="h-9 w-9 cursor-pointer rounded border border-border-primary bg-bg-secondary" bind:value={editColor} />
								<input type="text" class="input flex-1 font-mono text-xs" bind:value={editColor} maxlength="7" />
							</div>
						</div>
					</div>
					<div class="mt-3 flex gap-6">
						<label class="flex items-center gap-2 text-sm text-text-secondary">
							<input type="checkbox" bind:checked={editHoist} class="accent-brand-500" />
							Display separately (hoist)
						</label>
						<label class="flex items-center gap-2 text-sm text-text-secondary">
							<input type="checkbox" bind:checked={editMentionable} class="accent-brand-500" />
							Mentionable
						</label>
					</div>
				</div>

				<!-- Permissions grid -->
				<div class="rounded-lg bg-bg-secondary p-4">
					<h3 class="mb-3 text-sm font-semibold text-text-primary">Permissions</h3>
					<p class="mb-4 text-xs text-text-muted">
						Toggle Allow (green) to grant or Deny (red) to explicitly deny a permission. Unchecking both leaves it neutral/inherited.
					</p>

					<div class="space-y-4">
						{#each permissionGroups as group (group.name)}
							<div>
								<h4 class="mb-2 text-xs font-bold uppercase tracking-wide text-text-muted">{group.name}</h4>
								<div class="space-y-1">
									{#each group.perms as perm (perm.key)}
										{@const state = permState(editAllow, editDeny, perm.bit)}
										<div class="flex items-center justify-between rounded px-3 py-1.5 hover:bg-bg-modifier">
											<span class="text-sm text-text-secondary">{perm.label}</span>
											<div class="flex items-center gap-3">
												<!-- Allow checkbox -->
												<label class="flex items-center gap-1 text-xs" title="Allow">
													<input
														type="checkbox"
														checked={state === 'allow'}
														onchange={() => handlePermToggle(perm.bit, 'allow')}
														class="accent-green-500"
													/>
													<span class="text-green-400">Allow</span>
												</label>
												<!-- Deny checkbox -->
												<label class="flex items-center gap-1 text-xs" title="Deny">
													<input
														type="checkbox"
														checked={state === 'deny'}
														onchange={() => handlePermToggle(perm.bit, 'deny')}
														class="accent-red-500"
													/>
													<span class="text-red-400">Deny</span>
												</label>
											</div>
										</div>
									{/each}
								</div>
							</div>
						{/each}
					</div>
				</div>

				<!-- Action buttons -->
				<div class="flex items-center justify-between">
					<button class="btn-primary" onclick={handleSave} disabled={saveOp.loading || (!isEveryone && !editName.trim())}>
						{saveOp.loading ? 'Saving...' : 'Save Changes'}
					</button>
					{#if !isEveryone}
						<button class="text-sm text-red-400 hover:text-red-300" onclick={handleDelete}>
							Delete Role
						</button>
					{/if}
				</div>
			</div>
		{/if}
	</div>
</div>
