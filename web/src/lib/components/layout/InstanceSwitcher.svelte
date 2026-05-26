<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api/client';
	import { addToast } from '$lib/stores/toast';
	import { confirmAction } from '$lib/stores/confirm';
	import { createAsyncOp } from '$lib/utils/asyncOp';
	import { getErrorMessage } from '$lib/utils/apiError';
	import {
		instanceProfiles,
		crossInstanceUnreadCount,
		crossInstanceMentionCount,
		setInstanceProfiles,
		upsertInstanceProfile,
		removeInstanceProfile,
		setActiveInstance,
		loadInstanceProfilesFromCache,
		restoreActiveInstance,
		type InstanceProfile
	} from '$lib/stores/instances';

	let expanded = $state(false);
	let showAddForm = $state(false);
	let newInstanceUrl = $state('');
	let newInstanceName = $state('');
	let addOp = $state(createAsyncOp());
	let loadOp = $state(createAsyncOp());
	let currentOrigin = $state('');

	// Derived state
	let profiles = $derived($instanceProfiles);
	let unreadBadge = $derived($crossInstanceUnreadCount);
	let mentionBadge = $derived($crossInstanceMentionCount);

	async function loadProfiles() {
		const data = await loadOp.run(() => api.getInstanceProfiles());
		if (!loadOp.error) {
			setInstanceProfiles(data!);
		} else {
			// Fall back to cache.
			loadInstanceProfilesFromCache();
		}
	}

	async function addInstance() {
		if (!newInstanceUrl.trim()) return;
		const result = await addOp.run(
			() => api.createInstanceProfile({
				instance_url: newInstanceUrl.trim(),
				instance_name: newInstanceName.trim() || undefined,
			}),
			msg => addToast('Failed to add instance: ' + msg, 'error')
		);
		if (!addOp.error) {
			upsertInstanceProfile({
				id: result!.id,
				instance_url: newInstanceUrl.trim(),
				instance_name: newInstanceName.trim() || null,
				instance_icon: null,
				is_primary: false,
				last_connected: null,
				created_at: new Date().toISOString(),
			});
			addToast('Instance added', 'success');
			showAddForm = false;
			newInstanceUrl = '';
			newInstanceName = '';
		}
	}

	async function removeInstance(profile: InstanceProfile) {
		if (!(await confirmAction({ title: 'Remove Instance', message: `Remove ${profile.instance_name || profile.instance_url}?`, confirmLabel: 'Remove' }))) return;
		try {
			await api.deleteInstanceProfile(profile.id);
			removeInstanceProfile(profile.instance_url);
			addToast('Instance removed', 'success');
		} catch (err: unknown) {
			addToast('Failed to remove instance: ' + getErrorMessage(err, 'Unknown error'), 'error');
		}
	}

	function switchToInstance(instanceUrl: string) {
		setActiveInstance(instanceUrl);
		expanded = false;
		try {
			window.location.href = new URL('/app', instanceUrl).toString();
		} catch {
			addToast('Invalid instance URL', 'error');
		}
	}

	function getInitials(name: string | null, url: string): string {
		if (name) {
			return name.split(' ').map(w => w[0]).join('').slice(0, 2).toUpperCase();
		}
		try {
			const hostname = new URL(url).hostname;
			return hostname.split('.')[0].slice(0, 2).toUpperCase();
		} catch {
			return 'AV';
		}
	}

	onMount(() => {
		currentOrigin = window.location.origin;
		restoreActiveInstance();
		loadProfiles();
	});
</script>

<div class="relative">
	<!-- Toggle Button -->
	<button
		class="relative flex h-9 w-9 items-center justify-center rounded-md border border-bg-modifier bg-bg-tertiary text-text-muted transition-colors hover:bg-bg-modifier hover:text-text-primary"
		onclick={() => expanded = !expanded}
		title="Switch Instance"
	>
		<svg xmlns="http://www.w3.org/2000/svg" class="h-5 w-5" viewBox="0 0 20 20" fill="currentColor">
			<path d="M10 3.5a1.5 1.5 0 013 0V4a1 1 0 001 1h3a1 1 0 011 1v3a1 1 0 01-1 1h-.5a1.5 1.5 0 000 3h.5a1 1 0 011 1v3a1 1 0 01-1 1h-3a1 1 0 01-1-1v-.5a1.5 1.5 0 00-3 0v.5a1 1 0 01-1 1H6a1 1 0 01-1-1v-3a1 1 0 00-1-1h-.5a1.5 1.5 0 010-3H4a1 1 0 001-1V6a1 1 0 011-1h3a1 1 0 001-1v-.5z" />
		</svg>

		<!-- Badge for cross-instance notifications -->
		{#if mentionBadge > 0}
			<span class="absolute -top-1 -right-1 min-w-[18px] h-[18px] bg-red-500 rounded-full text-white text-[10px] font-bold flex items-center justify-center px-1">
				{mentionBadge > 99 ? '99+' : mentionBadge}
			</span>
		{:else if unreadBadge > 0}
			<span class="absolute -top-1 -right-1 w-3 h-3 bg-text-primary rounded-full"></span>
		{/if}
	</button>

	<!-- Dropdown -->
	{#if expanded}
		<!-- Backdrop -->
		<button
			class="fixed inset-0 z-40"
			onclick={() => expanded = false}
			aria-label="Close instance switcher"
		></button>

		<div class="absolute bottom-0 left-full z-50 ml-2 w-72 rounded-lg border border-border-primary bg-bg-floating shadow-xl">
			<div class="p-3 border-b border-border-primary">
				<h3 class="text-sm font-semibold text-text-primary">Instances</h3>
				<p class="text-xs text-text-muted mt-0.5">Switch between connected AmityVox instances.</p>
			</div>

			<div class="max-h-64 overflow-y-auto p-2 space-y-1">
				{#if loadOp.loading}
					<div class="flex items-center justify-center py-4">
						<div class="animate-spin h-5 w-5 border-2 border-brand-500 border-t-transparent rounded-full"></div>
					</div>
				{:else if profiles.length === 0}
					<div class="text-center py-4 text-text-muted text-xs">
						No additional instances configured.
					</div>
				{:else}
					<!-- Current instance (this one) -->
					<div class="px-3 py-2 rounded bg-brand-500/10 border border-brand-500/20">
						<div class="flex items-center gap-3">
							<div class="w-8 h-8 rounded-full bg-brand-500/20 flex items-center justify-center text-brand-400 text-xs font-bold">
								AV
							</div>
							<div class="flex-1 min-w-0">
								<div class="text-sm font-medium text-text-primary truncate">This Instance</div>
								<div class="text-xs text-text-muted truncate">{currentOrigin}</div>
							</div>
							<span class="text-xs text-brand-400">Current</span>
						</div>
					</div>

					{#each profiles as profile}
						<div class="px-3 py-2 rounded hover:bg-bg-modifier group">
							<div class="flex items-center gap-3">
								<button
									class="w-8 h-8 rounded-full bg-bg-secondary flex items-center justify-center text-text-muted text-xs font-bold group-hover:text-text-primary transition-colors"
									onclick={() => switchToInstance(profile.instance_url)}
								>
									{getInitials(profile.instance_name, profile.instance_url)}
								</button>
								<button
									class="flex-1 min-w-0 text-left"
									onclick={() => switchToInstance(profile.instance_url)}
								>
									<div class="text-sm text-text-primary truncate">{profile.instance_name || profile.instance_url}</div>
									<div class="text-xs text-text-muted truncate">{profile.instance_url}</div>
								</button>
								<div class="flex items-center gap-1">
									{#if profile.is_primary}
										<span class="text-xs text-brand-400">Primary</span>
									{/if}
									<button
										class="opacity-0 group-hover:opacity-100 text-text-muted hover:text-red-400 transition-all p-1"
										onclick={() => removeInstance(profile)}
										title="Remove instance"
									>
										<svg xmlns="http://www.w3.org/2000/svg" class="h-3.5 w-3.5" viewBox="0 0 20 20" fill="currentColor">
											<path fill-rule="evenodd" d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z" clip-rule="evenodd" />
										</svg>
									</button>
								</div>
							</div>
						</div>
					{/each}
				{/if}
			</div>

			<!-- Add Instance -->
			{#if showAddForm}
				<div class="p-3 border-t border-border-primary space-y-2">
					<input
						type="url"
						class="input w-full text-sm"
						placeholder="https://other-instance.example.com"
						bind:value={newInstanceUrl}
					/>
					<input
						type="text"
						class="input w-full text-sm"
						placeholder="Display name (optional)"
						bind:value={newInstanceName}
					/>
					<div class="flex gap-2">
						<button
							class="btn-primary text-sm flex-1"
							onclick={addInstance}
							disabled={addOp.loading || !newInstanceUrl.trim()}
						>
							{addOp.loading ? 'Adding...' : 'Add'}
						</button>
						<button
							class="btn-secondary text-sm"
							onclick={() => { showAddForm = false; newInstanceUrl = ''; newInstanceName = ''; }}
						>
							Cancel
						</button>
					</div>
				</div>
			{:else}
				<div class="p-2 border-t border-border-primary">
					<button
						class="w-full px-3 py-2 text-sm text-text-muted hover:text-text-primary hover:bg-bg-modifier rounded transition-colors text-left"
						onclick={() => showAddForm = true}
					>
						+ Add another instance
					</button>
				</div>
			{/if}
		</div>
	{/if}
</div>
