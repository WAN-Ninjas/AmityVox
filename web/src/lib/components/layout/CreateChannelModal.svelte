<script lang="ts">
	import { api } from '$lib/api/client';
	import Modal from '$components/common/Modal.svelte';
	import { currentGuildId } from '$lib/stores/guilds';
	import { updateChannel } from '$lib/stores/channels';
	import { createAsyncOp } from '$lib/utils/asyncOp';

	type ChannelType = 'text' | 'announcement' | 'voice' | 'stage' | 'forum' | 'gallery';

	interface Props {
		open: boolean;
		onclose: () => void;
	}

	let { open = $bindable(false), onclose }: Props = $props();

	let name = $state('');
	let type = $state<ChannelType>('text');
	let error = $state('');
	let createOp = $state(createAsyncOp());

	function channelTypeButtonClass(candidate: ChannelType): string {
		const base = 'rounded-lg border-2 px-4 py-2 text-sm transition-colors';
		if (type === candidate) return `${base} border-brand-500 bg-brand-500/10 text-text-primary`;
		return `${base} border-bg-modifier text-text-muted`;
	}

	function placeholderForType(): string {
		if (type === 'announcement') return 'announcements';
		if (type === 'voice') return 'General';
		if (type === 'stage') return 'Town Hall';
		if (type === 'forum') return 'bug-reports';
		if (type === 'gallery') return 'screenshots';
		return 'new-channel';
	}

	async function createChannel() {
		const guildId = $currentGuildId;
		if (!guildId || !name.trim()) return;
		error = '';
		const channel = await createOp.run(() => api.createChannel(guildId, name.trim(), type));
		if (createOp.error) {
			error = createOp.error;
		} else if (channel) {
			updateChannel(channel);
			name = '';
			type = 'text';
			onclose();
		}
	}
</script>

<Modal {open} title="Create Channel" {onclose}>
	{#if error}
		<div class="mb-4 rounded bg-red-500/10 px-3 py-2 text-sm text-red-400">{error}</div>
	{/if}

	<div class="mb-4">
		<div id="channel-type-label" class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">Channel Type</div>
		<div class="flex flex-wrap gap-2" role="group" aria-labelledby="channel-type-label">
			<button class={channelTypeButtonClass('text')} onclick={() => (type = 'text')}># Text</button>
			<button class={channelTypeButtonClass('announcement')} onclick={() => (type = 'announcement')}>Announcement</button>
			<button class={channelTypeButtonClass('voice')} onclick={() => (type = 'voice')}>Voice</button>
			<button class={channelTypeButtonClass('stage')} onclick={() => (type = 'stage')}>Stage</button>
			<button class={channelTypeButtonClass('forum')} onclick={() => (type = 'forum')}>Forum</button>
			<button class={channelTypeButtonClass('gallery')} onclick={() => (type = 'gallery')}>Gallery</button>
		</div>
	</div>

	<div class="mb-4">
		<label for="channelName" class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">
			Channel Name
		</label>
		<input
			id="channelName"
			type="text"
			class="input w-full"
			bind:value={name}
			placeholder={placeholderForType()}
			maxlength="100"
			onkeydown={(event) => event.key === 'Enter' && createChannel()}
		/>
	</div>

	<div class="flex justify-end gap-2">
		<button class="btn-secondary" onclick={onclose}>Cancel</button>
		<button class="btn-primary" onclick={createChannel} disabled={createOp.loading || !name.trim()}>
			{createOp.loading ? 'Creating...' : 'Create'}
		</button>
	</div>
</Modal>
