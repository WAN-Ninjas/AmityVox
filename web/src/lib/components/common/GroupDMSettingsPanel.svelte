<script lang="ts">
	import type { Channel, User } from '$lib/types';
	import { api } from '$lib/api/client';
	import { currentUser } from '$lib/stores/auth';
	import { addToast } from '$lib/stores/toast';
	import { removeDMChannel, addDMChannel } from '$lib/stores/dms';
	import { goto } from '$app/navigation';
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
	const recipients = $derived(channel.recipients ?? []);

	async function removeMember(userId: string) {
		try {
			await api.removeGroupDMRecipient(channel.id, userId);
			addToast('Member removed', 'success');
		} catch (err: any) {
			addToast(err.message || 'Failed to remove member', 'error');
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
		} catch (err: any) {
			addToast(err.message || 'Failed to leave group', 'error');
		}
	}
</script>

<Modal bind:open title="Group Settings" {onclose}>
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
							src={member.avatar_id ? `/api/v1/files/${member.avatar_id}` : null}
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

		<!-- Encryption -->
		<div>
			<h4 class="mb-2 text-xs font-medium text-text-muted">Encryption</h4>
			<EncryptionPanel
				channelId={channel.id}
				encrypted={channel.encrypted ?? false}
				onchange={() => { onclose(); }}
			/>
		</div>

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
