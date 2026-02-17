<script lang="ts">
	import Modal from '$components/common/Modal.svelte';
	import { api } from '$lib/api/client';
	import { kickModalTarget, banModalTarget } from '$lib/stores/moderation';
	import { addToast } from '$lib/stores/toast';

	// --- Kick Modal ---
	let kickReason = $state('');
	let kickSubmitting = $state(false);

	$effect(() => {
		if ($kickModalTarget) {
			kickReason = '';
			kickSubmitting = false;
		}
	});

	async function submitKick() {
		const target = $kickModalTarget;
		if (!target || !kickReason.trim()) return;
		kickSubmitting = true;
		try {
			await api.kickMember(target.guildId, target.userId, kickReason.trim());
			addToast(`Kicked ${target.displayName}`, 'success');
			$kickModalTarget = null;
		} catch (err: any) {
			addToast(err.message || 'Failed to kick member', 'error');
		} finally {
			kickSubmitting = false;
		}
	}

	// --- Ban Modal ---
	let banReason = $state('');
	let banDuration = $state('permanent');
	let banCustomMinutes = $state('');
	let banCleanup = $state('0');
	let banSubmitting = $state(false);

	$effect(() => {
		if ($banModalTarget) {
			banReason = '';
			banDuration = 'permanent';
			banCustomMinutes = '';
			banCleanup = '0';
			banSubmitting = false;
		}
	});

	const banDurationOptions = [
		{ label: 'Permanent', value: 'permanent' },
		{ label: '5 minutes', value: '300' },
		{ label: '15 minutes', value: '900' },
		{ label: '30 minutes', value: '1800' },
		{ label: '1 hour', value: '3600' },
		{ label: '1 day', value: '86400' },
		{ label: '7 days', value: '604800' },
		{ label: 'Custom', value: 'custom' },
	];

	const cleanupOptions = [
		{ label: "Don't delete", value: '0' },
		{ label: 'Last hour', value: '3600' },
		{ label: 'Last 6 hours', value: '21600' },
		{ label: 'Last 12 hours', value: '43200' },
		{ label: 'Last 24 hours', value: '86400' },
		{ label: 'Last 3 days', value: '259200' },
		{ label: 'Last 7 days', value: '604800' },
	];

	function getBanDurationSeconds(): number | undefined {
		if (banDuration === 'permanent') return undefined;
		if (banDuration === 'custom') {
			const mins = parseInt(banCustomMinutes, 10);
			if (!mins || mins <= 0) return -1; // sentinel: invalid custom duration
			return mins * 60;
		}
		return parseInt(banDuration, 10) || undefined;
	}

	async function submitBan() {
		const target = $banModalTarget;
		if (!target || !banReason.trim()) return;
		const durationSeconds = getBanDurationSeconds();
		if (durationSeconds === -1) {
			addToast('Please enter a valid number of minutes for the custom ban duration', 'error');
			return;
		}
		banSubmitting = true;
		try {
			const deleteMessageSeconds = parseInt(banCleanup, 10) || undefined;
			await api.banUser(target.guildId, target.userId, {
				reason: banReason.trim(),
				duration_seconds: durationSeconds,
				delete_message_seconds: deleteMessageSeconds,
			});
			const durLabel = durationSeconds ? banDurationOptions.find(o => o.value === banDuration)?.label ?? 'timed' : 'permanently';
			addToast(`Banned ${target.displayName} ${durationSeconds ? `for ${durLabel}` : 'permanently'}`, 'success');
			$banModalTarget = null;
		} catch (err: any) {
			addToast(err.message || 'Failed to ban user', 'error');
		} finally {
			banSubmitting = false;
		}
	}
</script>

<!-- Kick Modal -->
<Modal open={!!$kickModalTarget} title="Kick Member" onclose={() => ($kickModalTarget = null)}>
	<p class="mb-3 text-sm text-text-muted">
		Kick <strong class="text-text-primary">{$kickModalTarget?.displayName}</strong> from this server. They can rejoin with a new invite.
	</p>
	<label class="mb-1 block text-xs font-medium text-text-muted">Reason</label>
	<textarea
		class="mb-4 w-full rounded-md border border-bg-modifier bg-bg-primary p-2 text-sm text-text-primary placeholder:text-text-muted focus:border-brand-500 focus:outline-none"
		placeholder="Why is this member being kicked?"
		rows="3"
		bind:value={kickReason}
	></textarea>
	<div class="flex justify-end gap-2">
		<button
			class="rounded-md px-3 py-1.5 text-sm text-text-muted hover:text-text-primary"
			onclick={() => ($kickModalTarget = null)}
		>Cancel</button>
		<button
			class="rounded-md bg-red-500 px-3 py-1.5 text-sm font-medium text-white hover:bg-red-600 disabled:opacity-50"
			disabled={!kickReason.trim() || kickSubmitting}
			onclick={submitKick}
		>{kickSubmitting ? 'Kicking...' : 'Kick'}</button>
	</div>
</Modal>

<!-- Ban Modal -->
<Modal open={!!$banModalTarget} title="Ban User" onclose={() => ($banModalTarget = null)}>
	<p class="mb-3 text-sm text-text-muted">
		Ban <strong class="text-text-primary">{$banModalTarget?.displayName}</strong> from this server.
	</p>

	<label class="mb-1 block text-xs font-medium text-text-muted">Reason</label>
	<textarea
		class="mb-3 w-full rounded-md border border-bg-modifier bg-bg-primary p-2 text-sm text-text-primary placeholder:text-text-muted focus:border-brand-500 focus:outline-none"
		placeholder="Why is this user being banned?"
		rows="3"
		bind:value={banReason}
	></textarea>

	<label class="mb-1 block text-xs font-medium text-text-muted">Ban Duration</label>
	<select
		class="mb-3 w-full rounded-md border border-bg-modifier bg-bg-primary p-2 text-sm text-text-primary focus:border-brand-500 focus:outline-none"
		bind:value={banDuration}
	>
		{#each banDurationOptions as opt}
			<option value={opt.value}>{opt.label}</option>
		{/each}
	</select>

	{#if banDuration === 'custom'}
		<label class="mb-1 block text-xs font-medium text-text-muted">Duration (minutes)</label>
		<input
			type="number"
			min="1"
			class="mb-3 w-full rounded-md border border-bg-modifier bg-bg-primary p-2 text-sm text-text-primary placeholder:text-text-muted focus:border-brand-500 focus:outline-none"
			placeholder="Enter minutes"
			bind:value={banCustomMinutes}
		/>
	{/if}

	<label class="mb-1 block text-xs font-medium text-text-muted">Message Cleanup</label>
	<select
		class="mb-3 w-full rounded-md border border-bg-modifier bg-bg-primary p-2 text-sm text-text-primary focus:border-brand-500 focus:outline-none"
		bind:value={banCleanup}
	>
		{#each cleanupOptions as opt}
			<option value={opt.value}>{opt.label}</option>
		{/each}
	</select>

	<div class="flex justify-end gap-2">
		<button
			class="rounded-md px-3 py-1.5 text-sm text-text-muted hover:text-text-primary"
			onclick={() => ($banModalTarget = null)}
		>Cancel</button>
		<button
			class="rounded-md bg-red-500 px-3 py-1.5 text-sm font-medium text-white hover:bg-red-600 disabled:opacity-50"
			disabled={!banReason.trim() || banSubmitting}
			onclick={submitBan}
		>{banSubmitting ? 'Banning...' : 'Ban'}</button>
	</div>
</Modal>
