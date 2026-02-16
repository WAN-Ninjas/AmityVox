<script lang="ts">
	import type { Role } from '$lib/types';
	import { api } from '$lib/api/client';

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
	let saving = $state(false);

	let newRoleName = $state('');
	let newRoleColor = $state('#99aab5');
	let creatingRole = $state(false);

	const sortedRoles = $derived([...roles].sort((a, b) => b.position - a.position));
	const selectedRole = $derived(roles.find((r) => r.id === selectedRoleId) ?? null);

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

	// --- Actions ---

	async function handleCreateRole() {
		if (!newRoleName.trim()) return;
		creatingRole = true;
		try {
			const role = await api.createRole(guildId, newRoleName.trim(), {
				color: newRoleColor !== '#99aab5' ? newRoleColor : undefined
			});
			roles = [...roles, role];
			newRoleName = '';
			newRoleColor = '#99aab5';
			onSuccess('Role created');
		} catch (err: any) {
			onError(err.message || 'Failed to create role');
		} finally {
			creatingRole = false;
		}
	}

	async function handleSave() {
		if (!selectedRoleId || !editName.trim()) return;
		saving = true;
		try {
			const updated = await api.updateRole(guildId, selectedRoleId, {
				name: editName.trim(),
				color: editColor,
				hoist: editHoist,
				mentionable: editMentionable,
				permissions_allow: editAllow.toString() as any,
				permissions_deny: editDeny.toString() as any
			});
			roles = roles.map((r) => (r.id === selectedRoleId ? updated : r));
			onSuccess('Role updated');
		} catch (err: any) {
			onError(err.message || 'Failed to update role');
		} finally {
			saving = false;
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
			<button class="btn-primary w-full text-sm" onclick={handleCreateRole} disabled={creatingRole || !newRoleName.trim()}>
				{creatingRole ? 'Creating...' : 'Create Role'}
			</button>
		</div>

		<!-- Role list -->
		<div class="space-y-1">
			{#if sortedRoles.length === 0}
				<p class="py-4 text-center text-sm text-text-muted">No custom roles yet.</p>
			{/if}
			{#each sortedRoles as role (role.id)}
				<button
					class="flex w-full items-center gap-2.5 rounded-lg px-3 py-2 text-left text-sm transition-colors {selectedRoleId === role.id ? 'bg-brand-500/20 text-text-primary' : 'text-text-secondary hover:bg-bg-modifier'}"
					onclick={() => selectRole(role)}
				>
					<div class="h-3 w-3 shrink-0 rounded-full" style="background-color: {role.color ?? '#99aab5'}"></div>
					<span class="truncate">{role.name}</span>
				</button>
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
							<input type="text" class="input w-full" bind:value={editName} maxlength="100" />
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
					<button class="btn-primary" onclick={handleSave} disabled={saving || !editName.trim()}>
						{saving ? 'Saving...' : 'Save Changes'}
					</button>
					<button class="text-sm text-red-400 hover:text-red-300" onclick={handleDelete}>
						Delete Role
					</button>
				</div>
			</div>
		{/if}
	</div>
</div>
