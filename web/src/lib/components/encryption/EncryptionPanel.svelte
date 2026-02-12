<script lang="ts">
	import { api } from '$lib/api/client';
	import { addToast } from '$lib/stores/toast';

	interface Props {
		channelId: string;
		encrypted: boolean;
	}

	let { channelId, encrypted }: Props = $props();

	let groupEpoch = $state<number | null>(null);
	let groupState = $state<string | null>(null);
	let loadingState = $state(false);
	let stateError = $state('');
	let uploading = $state(false);

	// Fetch group state when channelId changes and channel is encrypted
	$effect(() => {
		const id = channelId;
		const isEncrypted = encrypted;
		groupEpoch = null;
		groupState = null;
		stateError = '';

		if (isEncrypted) {
			loadingState = true;
			api.getGroupState(id)
				.then((result) => {
					groupEpoch = result.epoch;
					groupState = result.state;
				})
				.catch((err) => {
					stateError = err.message || 'Failed to load group state';
				})
				.finally(() => {
					loadingState = false;
				});
		}
	});

	async function handleUploadKeyPackage() {
		uploading = true;
		try {
			// Generate a placeholder key package. In a real implementation this
			// would come from the MLS library running in the client. For now,
			// we send an empty string and let the server handle key generation
			// or the user can paste a key package in DeviceVerification.
			const result = await api.uploadKeyPackage('');
			addToast(`Key package uploaded (ID: ${result.id})`, 'success');
		} catch (err: any) {
			addToast(err.message || 'Failed to upload key package', 'error');
		} finally {
			uploading = false;
		}
	}

	async function refreshGroupState() {
		loadingState = true;
		stateError = '';
		try {
			const result = await api.getGroupState(channelId);
			groupEpoch = result.epoch;
			groupState = result.state;
		} catch (err: any) {
			stateError = err.message || 'Failed to refresh group state';
		} finally {
			loadingState = false;
		}
	}
</script>

<div class="flex flex-col gap-4 rounded-lg border border-bg-modifier bg-bg-secondary p-4">
	<!-- Encryption status -->
	<div class="flex items-center gap-3">
		{#if encrypted}
			<div class="flex h-10 w-10 shrink-0 items-center justify-center rounded-full bg-status-online/15">
				<svg class="h-5 w-5 text-status-online" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
					<path d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z" />
				</svg>
			</div>
			<div>
				<p class="text-sm font-semibold text-text-primary">End-to-End Encrypted</p>
				<p class="text-xs text-text-muted">Messages in this channel are encrypted with MLS.</p>
			</div>
		{:else}
			<div class="flex h-10 w-10 shrink-0 items-center justify-center rounded-full bg-bg-modifier">
				<svg class="h-5 w-5 text-text-muted" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
					<path d="M8 11V7a4 4 0 118 0m-4 8v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2z" />
				</svg>
			</div>
			<div>
				<p class="text-sm font-semibold text-text-primary">Not Encrypted</p>
				<p class="text-xs text-text-muted">Messages in this channel are not end-to-end encrypted.</p>
			</div>
		{/if}
	</div>

	{#if encrypted}
		<!-- Divider -->
		<div class="border-t border-bg-modifier"></div>

		<!-- Group state info -->
		<div>
			<div class="flex items-center justify-between">
				<h4 class="text-xs font-bold uppercase tracking-wide text-text-muted">Group State</h4>
				<button
					class="text-xs text-text-muted transition-colors hover:text-text-primary"
					onclick={refreshGroupState}
					disabled={loadingState}
					title="Refresh group state"
				>
					<svg
						class="h-4 w-4 {loadingState ? 'animate-spin' : ''}"
						fill="none"
						stroke="currentColor"
						stroke-width="2"
						viewBox="0 0 24 24"
					>
						<path d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
					</svg>
				</button>
			</div>
			{#if loadingState}
				<div class="mt-2 flex items-center gap-2 text-xs text-text-muted">
					<span class="inline-block h-3 w-3 animate-spin rounded-full border-2 border-brand-500 border-t-transparent"></span>
					Loading group state...
				</div>
			{:else if stateError}
				<p class="mt-2 text-xs text-red-400">{stateError}</p>
			{:else if groupEpoch !== null}
				<div class="mt-2 space-y-1">
					<div class="flex items-center justify-between rounded bg-bg-primary px-3 py-2">
						<span class="text-xs text-text-muted">Epoch</span>
						<span class="font-mono text-xs text-text-primary">{groupEpoch}</span>
					</div>
					{#if groupState}
						<div class="flex items-center justify-between rounded bg-bg-primary px-3 py-2">
							<span class="text-xs text-text-muted">State</span>
							<span class="max-w-[180px] truncate font-mono text-xs text-text-secondary" title={groupState}>
								{groupState}
							</span>
						</div>
					{/if}
				</div>
			{:else}
				<p class="mt-2 text-xs text-text-muted">No group state available. The encryption group may not be initialized yet.</p>
			{/if}
		</div>

		<!-- Divider -->
		<div class="border-t border-bg-modifier"></div>

		<!-- Key package upload -->
		<div>
			<h4 class="mb-2 text-xs font-bold uppercase tracking-wide text-text-muted">Key Management</h4>
			<p class="mb-3 text-xs text-text-muted">
				Upload a key package so other members can add you to encrypted channels.
			</p>
			<button
				class="flex items-center gap-2 rounded-md bg-brand-500 px-4 py-2 text-xs font-medium text-white transition-colors hover:bg-brand-600 disabled:opacity-50"
				onclick={handleUploadKeyPackage}
				disabled={uploading}
			>
				{#if uploading}
					<span class="inline-block h-3 w-3 animate-spin rounded-full border-2 border-white border-t-transparent"></span>
					Uploading...
				{:else}
					<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
						<path d="M7 16a4 4 0 01-.88-7.903A5 5 0 1115.9 6L16 6a5 5 0 011 9.9M15 13l-3-3m0 0l-3 3m3-3v12" />
					</svg>
					Upload Key Package
				{/if}
			</button>
		</div>
	{/if}
</div>
