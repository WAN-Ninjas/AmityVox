<script lang="ts">
	import { api } from '$lib/api/client';
	import { confirmAction } from '$lib/stores/confirm';
	import { addToast } from '$lib/stores/toast';
	import { createAsyncOp } from '$lib/utils/asyncOp';
	import { getErrorMessage } from '$lib/utils/apiError';
	import type { BanList, BanListEntry, BanListSubscription } from '$lib/types';

	interface Props {
		guildId: string;
	}

	let { guildId }: Props = $props();

	let banLists = $state<BanList[]>([]);
	let banListEntries = $state<Map<string, BanListEntry[]>>(new Map());
	let banListSubscriptions = $state<BanListSubscription[]>([]);
	let publicBanLists = $state<BanList[]>([]);
	let loadedGuildId = $state<string | null>(null);

	let newBanListName = $state('');
	let newBanListDescription = $state('');
	let newBanListPublic = $state(false);
	let expandedBanListId = $state<string | null>(null);

	let newEntryUserId = $state('');
	let newEntryReason = $state('');

	let showSubscribePanel = $state(false);
	let subscribingListId = $state('');
	let subscribingAutoBan = $state(false);

	let importingListId = $state<string | null>(null);
	let importData = $state('');

	let loadOp = $state(createAsyncOp());
	let loadEntriesOp = $state(createAsyncOp());
	let createOp = $state(createAsyncOp());
	let addEntryOp = $state(createAsyncOp());
	let importOp = $state(createAsyncOp());
	let subscribeOp = $state(createAsyncOp());

	const subscribablePublicBanLists = $derived(
		publicBanLists.filter((publicList) => !banListSubscriptions.some((subscription) => subscription.list_id === publicList.id))
	);

	$effect(() => {
		if (guildId && loadedGuildId !== guildId && !loadOp.loading) {
			loadBanLists();
		}
	});

	async function loadBanLists() {
		await loadOp.run(async () => {
			const [lists, subscriptions, publicLists] = await Promise.all([
				api.getBanLists(guildId),
				api.getBanListSubscriptions(guildId),
				api.getPublicBanLists()
			]);
			banLists = lists;
			banListSubscriptions = subscriptions;
			publicBanLists = publicLists;
			loadedGuildId = guildId;
		}, (message) => addToast(message, 'error'), 'Failed to load ban lists');
	}

	async function loadBanListEntriesFor(listId: string) {
		await loadEntriesOp.run(async () => {
			const entries = await api.getBanListEntries(guildId, listId);
			banListEntries = new Map(banListEntries);
			banListEntries.set(listId, entries);
		}, (message) => addToast(message, 'error'), 'Failed to load ban list entries');
	}

	async function handleCreateBanList() {
		if (!newBanListName.trim()) return;
		await createOp.run(async () => {
			const list = await api.createBanList(guildId, {
				name: newBanListName.trim(),
				description: newBanListDescription.trim() || undefined,
				public: newBanListPublic
			});
			banLists = [...banLists, list];
			newBanListName = '';
			newBanListDescription = '';
			newBanListPublic = false;
			addToast('Ban list created', 'success');
		}, (message) => addToast(message, 'error'), 'Failed to create ban list');
	}

	async function handleDeleteBanList(listId: string) {
		if (!(await confirmAction({ title: 'Delete Ban List', message: 'Delete this ban list? All entries will be removed.', confirmLabel: 'Delete' }))) return;
		try {
			await api.deleteBanList(guildId, listId);
			banLists = banLists.filter((list) => list.id !== listId);
			if (expandedBanListId === listId) expandedBanListId = null;
			if (importingListId === listId) importingListId = null;
			addToast('Ban list deleted', 'success');
		} catch (err: unknown) {
			addToast(getErrorMessage(err, 'Failed to delete ban list'), 'error');
		}
	}

	async function toggleExpandBanList(listId: string) {
		if (expandedBanListId === listId) {
			expandedBanListId = null;
			return;
		}
		expandedBanListId = listId;
		if (!banListEntries.has(listId)) {
			await loadBanListEntriesFor(listId);
		}
	}

	async function handleAddBanListEntry() {
		if (!expandedBanListId || !newEntryUserId.trim()) return;
		const listId = expandedBanListId;
		await addEntryOp.run(async () => {
			const entry = await api.addBanListEntry(guildId, listId, {
				user_id: newEntryUserId.trim(),
				reason: newEntryReason.trim() || undefined
			});
			const existing = banListEntries.get(listId) ?? [];
			banListEntries = new Map(banListEntries);
			banListEntries.set(listId, [...existing, entry]);
			banLists = banLists.map((list) => list.id === listId ? { ...list, entry_count: list.entry_count + 1 } : list);
			newEntryUserId = '';
			newEntryReason = '';
			addToast('Ban list entry added', 'success');
		}, (message) => addToast(message, 'error'), 'Failed to add entry');
	}

	async function handleRemoveBanListEntry(listId: string, entryId: string) {
		try {
			await api.removeBanListEntry(guildId, listId, entryId);
			const existing = banListEntries.get(listId) ?? [];
			banListEntries = new Map(banListEntries);
			banListEntries.set(listId, existing.filter((entry) => entry.id !== entryId));
			banLists = banLists.map((list) => list.id === listId ? { ...list, entry_count: Math.max(0, list.entry_count - 1) } : list);
			addToast('Ban list entry removed', 'success');
		} catch (err: unknown) {
			addToast(getErrorMessage(err, 'Failed to remove entry'), 'error');
		}
	}

	async function handleExportBanList(listId: string) {
		try {
			const data = await api.exportBanList(guildId, listId);
			const json = JSON.stringify(data, null, 2);
			const blob = new Blob([json], { type: 'application/json' });
			const url = URL.createObjectURL(blob);
			const anchor = document.createElement('a');
			anchor.href = url;
			anchor.download = `ban-list-${listId}.json`;
			document.body.appendChild(anchor);
			anchor.click();
			document.body.removeChild(anchor);
			URL.revokeObjectURL(url);
			addToast('Ban list exported', 'success');
		} catch (err: unknown) {
			addToast(getErrorMessage(err, 'Failed to export ban list'), 'error');
		}
	}

	async function handleImportBanList() {
		if (!importingListId || !importData.trim()) return;
		const listId = importingListId;
		await importOp.run(async () => {
			const parsed = JSON.parse(importData);
			await api.importBanList(guildId, listId, parsed);
			await loadBanListEntriesFor(listId);
			banLists = await api.getBanLists(guildId);
			importingListId = null;
			importData = '';
			addToast('Ban list imported', 'success');
		}, (message) => addToast(message, 'error'), 'Failed to import ban list (check JSON format)');
	}

	async function handleSubscribeBanList() {
		if (!subscribingListId) return;
		await subscribeOp.run(async () => {
			const subscription = await api.subscribeBanList(guildId, {
				list_id: subscribingListId,
				auto_ban: subscribingAutoBan
			});
			banListSubscriptions = [...banListSubscriptions, subscription];
			subscribingListId = '';
			subscribingAutoBan = false;
			showSubscribePanel = false;
			addToast('Subscribed to ban list', 'success');
		}, (message) => addToast(message, 'error'), 'Failed to subscribe to ban list');
	}

	async function handleUnsubscribeBanList(subId: string) {
		if (!(await confirmAction({ title: 'Unsubscribe Ban List', message: 'Unsubscribe from this ban list?', confirmLabel: 'Unsubscribe' }))) return;
		try {
			await api.unsubscribeBanList(guildId, subId);
			banListSubscriptions = banListSubscriptions.filter((subscription) => subscription.id !== subId);
			addToast('Unsubscribed from ban list', 'success');
		} catch (err: unknown) {
			addToast(getErrorMessage(err, 'Failed to unsubscribe'), 'error');
		}
	}

	function toggleImportPanel(listId: string) {
		importingListId = importingListId === listId ? null : listId;
		importData = '';
	}

	function closeSubscribePanel() {
		showSubscribePanel = false;
		subscribingListId = '';
		subscribingAutoBan = false;
	}

	function formatDate(iso: string): string {
		return new Date(iso).toLocaleString();
	}
</script>

<h1 class="mb-6 text-xl font-bold text-text-primary">Ban Lists</h1>
<p class="mb-4 text-sm text-text-muted">
	Create and manage shared ban lists. Public lists can be subscribed to by other servers.
</p>

<div class="mb-6 rounded-lg bg-bg-secondary p-4">
	<h2 class="mb-3 text-sm font-bold uppercase tracking-wide text-text-muted">Create Ban List</h2>
	<div class="mb-2">
		<input
			type="text"
			class="input w-full"
			placeholder="Ban list name..."
			bind:value={newBanListName}
			maxlength="100"
		/>
	</div>
	<div class="mb-2">
		<textarea
			class="input w-full"
			placeholder="Description (optional)..."
			rows="2"
			bind:value={newBanListDescription}
			maxlength="500"
		></textarea>
	</div>
	<div class="mb-3 flex items-center gap-2">
		<label class="flex items-center gap-2 text-sm text-text-muted">
			<input type="checkbox" bind:checked={newBanListPublic} class="rounded" />
			Make public (other servers can discover and subscribe)
		</label>
	</div>
	<button class="btn-primary" onclick={handleCreateBanList} disabled={createOp.loading || !newBanListName.trim()}>
		{createOp.loading ? 'Creating...' : 'Create Ban List'}
	</button>
</div>

{#if loadOp.loading}
	<p class="text-sm text-text-muted">Loading ban lists...</p>
{:else if banLists.length === 0}
	<p class="text-sm text-text-muted">No ban lists yet. Create one above.</p>
{:else}
	<div class="mb-6 space-y-2">
		{#each banLists as list (list.id)}
			<div class="rounded-lg bg-bg-secondary">
				<div class="flex items-center justify-between p-3">
					<button class="flex min-w-0 flex-1 items-center gap-3 text-left" onclick={() => toggleExpandBanList(list.id)}>
						<span class="h-4 w-4 shrink-0 text-text-muted transition-transform {expandedBanListId === list.id ? 'rotate-90' : ''}">›</span>
						<div class="min-w-0 flex-1">
							<div class="flex items-center gap-2">
								<span class="text-sm font-medium text-text-primary">{list.name}</span>
								{#if list.public}
									<span class="rounded bg-green-500/15 px-1.5 py-0.5 text-2xs text-green-400">Public</span>
								{/if}
								<span class="rounded bg-bg-modifier px-1.5 py-0.5 text-2xs text-text-muted">
									{list.entry_count} {list.entry_count === 1 ? 'entry' : 'entries'}
								</span>
							</div>
							{#if list.description}
								<p class="mt-0.5 text-xs text-text-muted">{list.description}</p>
							{/if}
						</div>
					</button>
					<div class="flex items-center gap-2">
						<button class="text-xs text-brand-400 hover:text-brand-300" onclick={() => handleExportBanList(list.id)} title="Export ban list">
							Export
						</button>
						<button class="text-xs text-brand-400 hover:text-brand-300" onclick={() => toggleImportPanel(list.id)} title="Import entries">
							Import
						</button>
						<button class="text-xs text-red-400 hover:text-red-300" onclick={() => handleDeleteBanList(list.id)} title="Delete ban list">
							Delete
						</button>
					</div>
				</div>

				{#if importingListId === list.id}
					<div class="border-t border-bg-modifier px-3 py-3">
						<h4 class="mb-2 text-xs font-bold uppercase tracking-wide text-text-muted">Import Entries (JSON)</h4>
						<textarea
							class="input mb-2 w-full font-mono text-xs"
							rows="4"
							bind:value={importData}
							placeholder="Paste exported ban list JSON here..."
						></textarea>
						<div class="flex gap-2">
							<button class="btn-primary text-xs" onclick={handleImportBanList} disabled={importOp.loading || !importData.trim()}>
								{importOp.loading ? 'Importing...' : 'Import'}
							</button>
							<button class="btn-secondary text-xs" onclick={() => toggleImportPanel(list.id)}>
								Cancel
							</button>
						</div>
					</div>
				{/if}

				{#if expandedBanListId === list.id}
					<div class="border-t border-bg-modifier px-3 py-3">
						{#if loadEntriesOp.loading}
							<p class="text-xs text-text-muted">Loading entries...</p>
						{:else}
							<div class="mb-3 flex gap-2">
								<input type="text" class="input flex-1 text-sm" placeholder="User ID..." bind:value={newEntryUserId} />
								<input type="text" class="input flex-1 text-sm" placeholder="Reason (optional)..." bind:value={newEntryReason} />
								<button class="btn-primary text-xs" onclick={handleAddBanListEntry} disabled={addEntryOp.loading || !newEntryUserId.trim()}>
									{addEntryOp.loading ? 'Adding...' : 'Add'}
								</button>
							</div>

							{@const entries = banListEntries.get(list.id) ?? []}
							{#if entries.length === 0}
								<p class="text-xs text-text-muted">No entries in this ban list.</p>
							{:else}
								<div class="space-y-1.5">
									{#each entries as entry (entry.id)}
										<div class="flex items-center justify-between rounded-md bg-bg-primary p-2">
											<div class="min-w-0 flex-1">
												<div class="flex items-center gap-2">
													<span class="text-sm text-text-primary">{entry.username ?? entry.user_id}</span>
												</div>
												{#if entry.reason}
													<p class="mt-0.5 text-xs text-text-muted">Reason: {entry.reason}</p>
												{/if}
												<p class="mt-0.5 text-2xs text-text-muted">Added {formatDate(entry.created_at)}</p>
											</div>
											<button class="shrink-0 text-xs text-red-400 hover:text-red-300" onclick={() => handleRemoveBanListEntry(list.id, entry.id)}>
												Remove
											</button>
										</div>
									{/each}
								</div>
							{/if}
						{/if}
					</div>
				{/if}
			</div>
		{/each}
	</div>
{/if}

<div class="mb-6">
	<h2 class="mb-3 text-sm font-bold uppercase tracking-wide text-text-muted">Subscriptions</h2>
	<p class="mb-3 text-xs text-text-muted">
		Subscribe to ban lists from other servers. When auto-ban is enabled, users on subscribed lists are automatically banned.
	</p>

	{#if banListSubscriptions.length === 0}
		<p class="mb-3 text-sm text-text-muted">No subscriptions yet.</p>
	{:else}
		<div class="mb-3 space-y-2">
			{#each banListSubscriptions as sub (sub.id)}
				<div class="flex items-center justify-between rounded-lg bg-bg-secondary p-3">
					<div>
						<span class="text-sm font-medium text-text-primary">{sub.list_name}</span>
						<div class="mt-0.5 flex gap-2 text-xs text-text-muted">
							{#if sub.auto_ban}
								<span class="rounded bg-red-500/15 px-1.5 py-0.5 text-red-400">Auto-ban enabled</span>
							{:else}
								<span class="rounded bg-bg-modifier px-1.5 py-0.5">Manual review</span>
							{/if}
							<span>Since {formatDate(sub.created_at)}</span>
						</div>
					</div>
					<button class="text-xs text-red-400 hover:text-red-300" onclick={() => handleUnsubscribeBanList(sub.id)}>
						Unsubscribe
					</button>
				</div>
			{/each}
		</div>
	{/if}

	{#if !showSubscribePanel}
		<button class="btn-secondary text-sm" onclick={() => (showSubscribePanel = true)}>
			Subscribe to a Ban List
		</button>
	{:else}
		<div class="rounded-lg bg-bg-secondary p-4">
			<h3 class="mb-2 text-xs font-bold uppercase tracking-wide text-text-muted">Subscribe to Public Ban List</h3>
			{#if publicBanLists.length === 0}
				<p class="mb-2 text-xs text-text-muted">No public ban lists available.</p>
			{:else}
				<div class="mb-2">
					<select class="input w-full" bind:value={subscribingListId}>
						<option value="">Select a ban list...</option>
						{#each subscribablePublicBanLists as pubList (pubList.id)}
							<option value={pubList.id}>
								{pubList.name} ({pubList.entry_count} entries)
								{#if pubList.description} - {pubList.description}{/if}
							</option>
						{/each}
					</select>
				</div>
				<div class="mb-3 flex items-center gap-2">
					<label class="flex items-center gap-2 text-sm text-text-muted">
						<input type="checkbox" bind:checked={subscribingAutoBan} class="rounded" />
						Auto-ban users on this list
					</label>
				</div>
			{/if}
			<div class="flex gap-2">
				<button class="btn-primary text-xs" onclick={handleSubscribeBanList} disabled={subscribeOp.loading || !subscribingListId}>
					{subscribeOp.loading ? 'Subscribing...' : 'Subscribe'}
				</button>
				<button class="btn-secondary text-xs" onclick={closeSubscribePanel}>
					Cancel
				</button>
			</div>
		</div>
	{/if}
</div>
