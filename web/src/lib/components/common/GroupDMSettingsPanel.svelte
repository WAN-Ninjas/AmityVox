<script lang="ts">
	import type { Channel, User } from '$lib/types';
	import { api } from '$lib/api/client';
	import { currentUser } from '$lib/stores/auth';
	import { addToast } from '$lib/stores/toast';
	import { removeDMChannel, updateDMChannel } from '$lib/stores/dms';
	import { relationships } from '$lib/stores/relationships';
	import { clientConfig, isFeatureEnabled } from '$lib/stores/clientConfig';
	import { goto } from '$app/navigation';
	import { avatarUrl } from '$lib/utils/avatar';
	import { getErrorMessage } from '$lib/utils/apiError';
	import Avatar from './Avatar.svelte';
	import Modal from './Modal.svelte';
	import EncryptionPanel from '$components/encryption/EncryptionPanel.svelte';

	interface Props {
		channel: Channel;
		open: boolean;
		onclose: () => void;
	}

	let { channel, open = $bindable(), onclose }: Props = $props();

	const isOwner = $derived($currentUser?.id === channel.owner_id);
	const hasE2EE = $derived(isFeatureEnabled($clientConfig, 'e2ee'));
	const recipients = $derived(channel.recipients ?? []);
	const recipientIds = $derived(new Set(recipients.map((member) => member.id)));
	let memberSearch = $state('');
	let addingUserId = $state<string | null>(null);

	const addableFriends = $derived.by(() => {
		const query = memberSearch.trim().toLowerCase();
		const list: User[] = [];
		for (const [targetId, rel] of $relationships) {
			if (rel.type !== 'friend' || recipientIds.has(targetId) || !rel.user) continue;
			const name = rel.user.display_name ?? rel.user.username;
			if (query && !name.toLowerCase().includes(query) && !rel.user.username.toLowerCase().includes(query)) continue;
			list.push(rel.user);
		}
		return list.sort((a, b) => (a.display_name ?? a.username).localeCompare(b.display_name ?? b.username));
	});

	async function removeMember(userId: string) {
		try {
			await api.removeGroupDMRecipient(channel.id, userId);
			updateDMChannel({ ...channel, recipients: recipients.filter((member) => member.id !== userId) });
			addToast('Member removed', 'success');
		} catch (err: unknown) {
			addToast(getErrorMessage(err, 'Failed to remove member'), 'error');
		}
	}

	async function addMember(user: User) {
		if (addingUserId || recipients.length >= 10) return;
		addingUserId = user.id;
		try {
			const updated = await api.addGroupDMRecipient(channel.id, user.id);
			updateDMChannel(updated);
			memberSearch = '';
			addToast('Member added', 'success');
		} catch (err: unknown) {
			addToast(getErrorMessage(err, 'Failed to add member'), 'error');
		} finally {
			addingUserId = null;
		}
	}

	async function leaveGroup() {
		if (!$currentUser) return;
		try {
			await api.removeGroupDMRecipient(channel.id, $currentUser.id);
			removeDMChannel(channel.id);
			onclose();
			goto('/app');
			addToast('Left group', 'success');
		} catch (err: unknown) {
			addToast(getErrorMessage(err, 'Failed to leave group'), 'error');
		}
	}
</script>

<Modal {open} title="Group Settings" {onclose}>
	<div class="space-y-4">
		<!-- Group info -->
		<div>
			<h4 class="mb-1 text-xs font-medium text-text-muted">Group Name</h4>
			<p class="text-sm text-text-primary">{channel.name || 'Unnamed Group'}</p>
		</div>

		<!-- Members -->
		<div>
			<h4 class="mb-2 text-xs font-medium text-text-muted">Members ({recipients.length})</h4>
			<div class="max-h-48 space-y-1 overflow-y-auto rounded-md bg-bg-primary">
				{#each recipients as member (member.id)}
					<div class="flex items-center gap-2.5 px-3 py-2">
						<Avatar
							name={member.display_name ?? member.username}
							src={avatarUrl(member.avatar_id, member.instance_id || undefined)}
							size="sm"
						/>
						<span class="flex-1 truncate text-sm text-text-secondary">
							{member.display_name ?? member.username}
						</span>
						{#if member.id === channel.owner_id}
							<span class="rounded bg-brand-500/20 px-1.5 py-0.5 text-2xs font-medium text-brand-400">Owner</span>
						{/if}
						{#if isOwner && member.id !== $currentUser?.id}
							<button
								class="text-text-muted transition-colors hover:text-red-400"
								onclick={() => removeMember(member.id)}
								title="Remove from group"
							>
								<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
									<path d="M6 18L18 6M6 6l12 12" />
								</svg>
							</button>
						{/if}
					</div>
				{/each}
			</div>
		</div>

		{#if isOwner && (hasE2EE || channel.encrypted)}
			<div>
				<h4 class="mb-2 text-xs font-medium text-text-muted">Add Members</h4>
				<input
					class="input mb-2 w-full text-sm"
					placeholder={recipients.length >= 10 ? 'Group is full' : 'Search friends...'}
					bind:value={memberSearch}
					disabled={recipients.length >= 10}
				/>
				{#if recipients.length < 10}
					<div class="max-h-36 overflow-y-auto rounded-md bg-bg-primary">
						{#if addableFriends.length === 0}
							<p class="p-3 text-center text-sm text-text-muted">{memberSearch ? 'No friends match your search.' : 'No friends available to add.'}</p>
						{:else}
							{#each addableFriends as friend (friend.id)}
								<button
									class="flex w-full items-center gap-2.5 px-3 py-2 text-left transition-colors hover:bg-bg-modifier disabled:opacity-50"
									onclick={() => addMember(friend)}
									disabled={addingUserId === friend.id}
								>
									<Avatar
										name={friend.display_name ?? friend.username}
										src={avatarUrl(friend.avatar_id, friend.instance_id || undefined)}
										size="sm"
									/>
									<span class="flex-1 truncate text-sm text-text-secondary">
										{friend.display_name ?? friend.username}
									</span>
									<span class="text-xs text-text-muted">{addingUserId === friend.id ? 'Adding...' : 'Add'}</span>
								</button>
							{/each}
						{/if}
					</div>
				{/if}
			</div>
		{/if}

		<!-- Encryption (owner only) -->
		{#if isOwner}
			<div>
				<h4 class="mb-2 text-xs font-medium text-text-muted">Encryption</h4>
				<EncryptionPanel
					channelId={channel.id}
					encrypted={channel.encrypted ?? false}
					onchange={() => { onclose(); }}
				/>
			</div>
		{/if}

		<!-- Actions -->
		<div class="flex justify-between">
			<button
				class="rounded px-3 py-1.5 text-sm font-medium text-red-400 transition-colors hover:bg-red-500/10"
				onclick={leaveGroup}
			>
				Leave Group
			</button>
			<button class="btn-secondary text-sm" onclick={onclose}>Close</button>
		</div>
	</div>
</Modal>
