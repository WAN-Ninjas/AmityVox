<script lang="ts">
	import { currentUser } from '$lib/stores/auth';
	import { updatePresence } from '$lib/stores/presence';
	import { getGatewayClient } from '$lib/stores/gateway';
	import { api } from '$lib/api/client';
	import { addToast } from '$lib/stores/toast';
	import { setManualStatus } from '$lib/utils/idle';

	interface Props {
		open: boolean;
		onclose: () => void;
	}

	let { open = $bindable(), onclose }: Props = $props();

	const statusOptions = [
		{ value: 'online', label: 'Online', description: 'You are available', colorClass: 'bg-status-online' },
		{ value: 'idle', label: 'Idle', description: 'You may be away', colorClass: 'bg-status-idle' },
		{ value: 'busy', label: 'Do Not Disturb', description: 'Suppress notifications', colorClass: 'bg-status-dnd' },
		{ value: 'invisible', label: 'Invisible', description: 'Appear offline to others', colorClass: 'bg-status-offline' }
	] as const;

	const expiryOptions = [
		{ label: "Don't clear", value: null },
		{ label: '30 minutes', value: 30 * 60 * 1000 },
		{ label: '1 hour', value: 60 * 60 * 1000 },
		{ label: '4 hours', value: 4 * 60 * 60 * 1000 },
		{ label: 'Today', value: -1 }
	] as const;

	let showCustomStatus = $state(false);
	let customText = $state('');
	let expiryMs = $state<number | null>(null);
	let saving = $state(false);

	const currentStatus = $derived(
		$currentUser?.status_presence ?? 'online'
	);

	async function selectStatus(status: string) {
		if (!$currentUser) return;

		try {
			// Update backend via REST
			await api.updateMe({ status_presence: status as any });

			// Update local presence
			updatePresence($currentUser.id, status === 'invisible' ? 'offline' : status);

			// Send via WebSocket
			const client = getGatewayClient();
			client?.updatePresence(status);

			// Update current user store
			currentUser.update((u) => u ? { ...u, status_presence: status as any } : u);

			// Track manual status for idle detection
			setManualStatus(status);

			onclose();
		} catch (err: any) {
			addToast(err.message || 'Failed to update status', 'error');
		}
	}

	async function saveCustomStatus() {
		if (!$currentUser) return;
		saving = true;

		try {
			const update: Record<string, unknown> = {
				status_text: customText.trim() || null
			};

			if (expiryMs === null) {
				update.status_expires_at = null;
			} else if (expiryMs === -1) {
				// "Today" — end of day
				const endOfDay = new Date();
				endOfDay.setHours(23, 59, 59, 999);
				update.status_expires_at = endOfDay.toISOString();
			} else {
				update.status_expires_at = new Date(Date.now() + expiryMs).toISOString();
			}

			const user = await api.updateMe(update as any);
			currentUser.set(user);
			showCustomStatus = false;
			addToast('Custom status updated', 'success');
		} catch (err: any) {
			addToast(err.message || 'Failed to set custom status', 'error');
		} finally {
			saving = false;
		}
	}

	async function clearCustomStatus() {
		if (!$currentUser) return;
		saving = true;

		try {
			const user = await api.updateMe({ status_text: null, status_expires_at: null } as any);
			currentUser.set(user);
			customText = '';
			expiryMs = null;
			showCustomStatus = false;
		} catch (err: any) {
			addToast(err.message || 'Failed to clear custom status', 'error');
		} finally {
			saving = false;
		}
	}

	// Initialize custom text from current user
	$effect(() => {
		if (open && $currentUser?.status_text) {
			customText = $currentUser.status_text;
		}
	});
</script>

{#if open}
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div
		class="absolute bottom-full left-0 z-50 mb-2 w-64 rounded-lg bg-bg-floating shadow-lg"
		onclick={(e) => e.stopPropagation()}
		onkeydown={(e) => e.key === 'Escape' && onclose()}
		role="listbox"
		aria-label="Set status"
		aria-expanded="true"
	>
		<!-- Status options -->
		<div class="p-1.5">
			{#each statusOptions as option (option.value)}
				<button
					class="flex w-full items-center gap-3 rounded-md px-3 py-2 text-left transition-colors hover:bg-bg-modifier {currentStatus === option.value ? 'bg-bg-modifier' : ''}"
					onclick={() => selectStatus(option.value)}
				>
					<span class="block h-3 w-3 shrink-0 rounded-full ring-2 ring-bg-floating {option.colorClass}"></span>
					<div class="min-w-0 flex-1">
						<p class="text-sm font-medium text-text-primary">{option.label}</p>
						<p class="text-xs text-text-muted">{option.description}</p>
					</div>
					{#if currentStatus === option.value}
						<svg class="h-4 w-4 shrink-0 text-brand-500" fill="currentColor" viewBox="0 0 20 20">
							<path fill-rule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clip-rule="evenodd" />
						</svg>
					{/if}
				</button>
			{/each}
		</div>

		<!-- Divider -->
		<div class="border-t border-bg-modifier"></div>

		<!-- Custom status section -->
		<div class="p-1.5">
			{#if !showCustomStatus}
				<button
					class="flex w-full items-center gap-3 rounded-md px-3 py-2 text-left text-sm text-text-secondary transition-colors hover:bg-bg-modifier"
					onclick={() => (showCustomStatus = true)}
				>
					<svg class="h-4 w-4 shrink-0" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
						<circle cx="12" cy="12" r="9" />
						<path d="M8.5 14.5c1 1.5 5.5 1.5 7 0" stroke-linecap="round" />
						<circle cx="9" cy="10" r="1" fill="currentColor" stroke="none" />
						<circle cx="15" cy="10" r="1" fill="currentColor" stroke="none" />
					</svg>
					{$currentUser?.status_text ? 'Edit Custom Status' : 'Set Custom Status'}
				</button>
			{:else}
				<div class="space-y-2 px-2 py-1">
					<label class="block text-xs font-bold uppercase tracking-wide text-text-muted">
						Status Message
					</label>
					<input
						type="text"
						class="input w-full text-sm"
						bind:value={customText}
						placeholder="What's on your mind?"
						maxlength="128"
					/>

					<label class="block text-xs font-bold uppercase tracking-wide text-text-muted">
						Clear After
					</label>
					<select class="input w-full text-sm" bind:value={expiryMs}>
						{#each expiryOptions as opt (opt.label)}
							<option value={opt.value}>{opt.label}</option>
						{/each}
					</select>

					<div class="flex gap-2 pt-1">
						{#if $currentUser?.status_text}
							<button
								class="btn-secondary flex-1 text-xs"
								onclick={clearCustomStatus}
								disabled={saving}
							>
								Clear
							</button>
						{/if}
						<button
							class="btn-primary flex-1 text-xs"
							onclick={saveCustomStatus}
							disabled={saving || !customText.trim()}
						>
							{saving ? 'Saving...' : 'Save'}
						</button>
					</div>
				</div>
			{/if}
		</div>
	</div>
{/if}
