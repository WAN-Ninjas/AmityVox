<script lang="ts">
	import { api } from '$lib/api/client';
	import Modal from '$components/common/Modal.svelte';
	import EncryptionPanel from '$components/encryption/EncryptionPanel.svelte';
	import { updateChannel } from '$lib/stores/channels';
	import { createAsyncOp } from '$lib/utils/asyncOp';
	import type { Channel } from '$lib/types';

	interface Props {
		open: boolean;
		channel: Channel | null;
		onclose: () => void;
	}

	let { open = $bindable(false), channel, onclose }: Props = $props();

	let loadedChannelId = $state('');
	let name = $state('');
	let topic = $state('');
	let nsfw = $state(false);
	let encrypted = $state(false);
	let channelType = $state<'text' | 'voice'>('text');
	let userLimit = $state(0);
	let bitrate = $state(64000);
	let error = $state('');
	let saveOp = $state(createAsyncOp());

	const userLimitOptions = [0, 5, 10, 15, 20, 25, 50, 99];
	const bitrateOptions = [32000, 64000, 96000, 128000, 192000, 256000, 384000];

	$effect(() => {
		if (!open || !channel || loadedChannelId === channel.id) return;
		loadedChannelId = channel.id;
		name = channel.name ?? '';
		topic = channel.topic ?? '';
		nsfw = channel.nsfw ?? false;
		encrypted = channel.encrypted ?? false;
		channelType = channel.channel_type === 'voice' ? 'voice' : 'text';
		userLimit = channel.user_limit ?? 0;
		bitrate = channel.bitrate ?? 64000;
		error = '';
	});

	async function saveChannel() {
		if (!channel || !name.trim()) return;
		error = '';
		const updated = await saveOp.run(async () => {
			const updateData: Record<string, unknown> = {
				name: name.trim(),
				topic: topic || undefined,
				nsfw
			};
			if (channelType === 'voice') {
				updateData.user_limit = userLimit;
				updateData.bitrate = bitrate;
			}
			return api.updateChannel(channel.id, updateData as any);
		});
		if (saveOp.error) {
			error = saveOp.error;
		} else if (updated) {
			updateChannel(updated);
			onclose();
		}
	}
</script>

<Modal {open} title="Edit Channel" {onclose}>
	{#if error}
		<div class="mb-4 rounded bg-red-500/10 px-3 py-2 text-sm text-red-400">{error}</div>
	{/if}

	<div class="mb-4">
		<label for="editName" class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">
			Channel Name
		</label>
		<input id="editName" type="text" class="input w-full" bind:value={name} maxlength="100" />
	</div>

	<div class="mb-4">
		<label for="editTopic" class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">
			Topic
		</label>
		<input
			id="editTopic"
			type="text"
			class="input w-full"
			bind:value={topic}
			placeholder="Set a channel topic"
			maxlength="1024"
		/>
	</div>

	<div class="mb-4">
		<label class="flex cursor-pointer items-center gap-3">
			<button
				type="button"
				role="switch"
				aria-checked={nsfw}
				aria-label="NSFW Channel"
				class="relative inline-flex h-6 w-11 shrink-0 rounded-full transition-colors {nsfw ? 'bg-red-500' : 'bg-bg-modifier'}"
				onclick={() => (nsfw = !nsfw)}
			>
				<span
					class="pointer-events-none inline-block h-5 w-5 translate-y-0.5 rounded-full bg-white shadow transition-transform {nsfw ? 'translate-x-5' : 'translate-x-0.5'}"
				></span>
			</button>
			<div>
				<span class="text-sm font-medium text-text-primary">NSFW Channel</span>
				<p class="text-xs text-text-muted">Mark this channel as age-restricted. Users will see a warning before viewing.</p>
			</div>
		</label>
	</div>

	{#if channelType === 'text' && channel}
		<div class="mb-4">
			<EncryptionPanel
				channelId={channel.id}
				{encrypted}
				onchange={onclose}
			/>
		</div>
	{/if}

	{#if channelType === 'voice'}
		<div class="mb-4">
			<label for="editUserLimit" class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">
				User Limit
			</label>
			<select id="editUserLimit" class="input w-full" bind:value={userLimit}>
				{#each userLimitOptions as limit}
					<option value={limit}>{limit === 0 ? 'No limit' : `${limit} users`}</option>
				{/each}
			</select>
			<p class="mt-1 text-xs text-text-muted">Maximum number of users that can join this voice channel. Set to "No limit" for unlimited.</p>
		</div>

		<div class="mb-4">
			<label for="editBitrate" class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">
				Bitrate
			</label>
			<select id="editBitrate" class="input w-full" bind:value={bitrate}>
				{#each bitrateOptions as rate}
					<option value={rate}>{Math.floor(rate / 1000)}kbps</option>
				{/each}
			</select>
			<p class="mt-1 text-xs text-text-muted">Higher bitrate means better audio quality but uses more bandwidth.</p>
		</div>
	{/if}

	<div class="flex justify-end gap-2">
		<button class="btn-secondary" onclick={onclose}>Cancel</button>
		<button class="btn-primary" onclick={saveChannel} disabled={saveOp.loading || !name.trim()}>
			{saveOp.loading ? 'Saving...' : 'Save'}
		</button>
	</div>
</Modal>
