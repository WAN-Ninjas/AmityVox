<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api/client';
	import { addToast } from '$lib/stores/toast';
	import { confirmAction } from '$lib/stores/confirm';
	import type { FederationPeer } from '$lib/types';

	let peers = $state<FederationPeer[]>([]);
	let loadingPeers = $state(false);
	let newPeerDomain = $state('');
	let addingPeer = $state(false);

	onMount(() => {
		loadPeers();
	});

	async function loadPeers() {
		loadingPeers = true;
		try {
			peers = await api.getFederationPeers();
		} catch {
			peers = [];
		} finally {
			loadingPeers = false;
		}
	}

	async function handleAddPeer() {
		if (!newPeerDomain.trim()) return;
		addingPeer = true;
		try {
			const peer = await api.addFederationPeer(newPeerDomain.trim());
			peers = [...peers, peer];
			newPeerDomain = '';
			addToast('Peer added', 'success');
		} catch {
			addToast('Failed to add peer', 'error');
		} finally {
			addingPeer = false;
		}
	}

	async function handleRemovePeer(peerId: string) {
		if (!(await confirmAction({ title: 'Remove Federation Peer', message: 'Remove this federation peer?', confirmLabel: 'Remove' }))) return;
		try {
			await api.removeFederationPeer(peerId);
			peers = peers.filter((peer) => peer.id !== peerId);
			addToast('Peer removed', 'success');
		} catch {
			addToast('Failed to remove peer', 'error');
		}
	}
</script>

<h1 class="mb-6 text-2xl font-bold text-text-primary">Federation Peers</h1>

<a
	href="/app/admin/federation"
	class="mb-6 inline-flex items-center gap-2 rounded-lg bg-bg-tertiary px-4 py-2.5 text-sm font-medium text-text-primary transition-colors hover:bg-bg-quaternary"
>
	Open Federation Dashboard <span aria-hidden="true">&rarr;</span>
</a>

<div class="mb-6 flex gap-2">
	<input
		type="text" class="input flex-1" aria-label="Federation peer domain" placeholder="Enter domain (e.g., chat.example.com)..."
		bind:value={newPeerDomain}
		onkeydown={(e) => e.key === 'Enter' && handleAddPeer()}
	/>
	<button class="btn-primary" onclick={handleAddPeer} disabled={addingPeer || !newPeerDomain.trim()}>
		{addingPeer ? 'Adding...' : 'Add Peer'}
	</button>
</div>

{#if loadingPeers}
	<p class="text-sm text-text-muted">Loading peers...</p>
{:else if peers.length === 0}
	<div class="rounded-lg bg-bg-secondary p-6 text-center">
		<p class="text-sm text-text-muted">No federation peers configured.</p>
		<p class="mt-1 text-xs text-text-muted">Add a peer domain above to begin federating.</p>
	</div>
{:else}
	<div class="space-y-2">
		{#each peers as peer (peer.id)}
			<div class="flex items-center justify-between rounded-lg bg-bg-secondary p-3">
				<div>
					<div class="flex items-center gap-2">
						<span class="text-sm font-medium text-text-primary">{peer.domain}</span>
						<span class="rounded px-1.5 py-0.5 text-2xs font-bold {peer.status === 'active' ? 'bg-green-500/20 text-green-400' : 'bg-yellow-500/20 text-yellow-400'}">
							{peer.status}
						</span>
					</div>
					<p class="text-xs text-text-muted">
						{peer.software ?? 'Unknown'} {peer.software_version ?? ''} &middot;
						{peer.last_seen_at ? `Last seen ${new Date(peer.last_seen_at).toLocaleString()}` : 'Never seen'}
					</p>
				</div>
				<button
					class="text-xs text-red-400 hover:text-red-300"
					onclick={() => handleRemovePeer(peer.id)}
				>
					Remove
				</button>
			</div>
		{/each}
	</div>
{/if}
