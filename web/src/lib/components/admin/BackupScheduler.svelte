<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api/client';
	import { addToast } from '$lib/stores/toast';

	interface BackupSchedule {
		id: string;
		name: string;
		frequency: string;
		retention_count: number;
		include_media: boolean;
		include_database: boolean;
		storage_path: string;
		enabled: boolean;
		last_run_at: string | null;
		last_run_status: string | null;
		last_run_size_bytes: number | null;
		next_run_at: string | null;
		created_by: string;
		created_at: string;
		updated_at: string;
		creator_name: string;
	}

	interface BackupEntry {
		id: string;
		schedule_id: string;
		status: string;
		size_bytes: number | null;
		file_path: string | null;
		error_message: string | null;
		started_at: string;
		completed_at: string | null;
		created_at: string;
	}

	let loading = $state(true);
	let schedules = $state<BackupSchedule[]>([]);
	let showCreateForm = $state(false);
	let creating = $state(false);
	let triggeringId = $state('');

	// History
	let historyScheduleId = $state('');
	let history = $state<BackupEntry[]>([]);
	let loadingHistory = $state(false);

	// Create form
	let newName = $state('');
	let newFrequency = $state('daily');
	let newRetentionCount = $state(7);
	let newIncludeMedia = $state(false);
	let newIncludeDatabase = $state(true);
	let newStoragePath = $state('/backups');
	let newEnabled = $state(true);

	async function loadSchedules() {
		loading = true;
		try {
			const res = await fetch('/api/v1/admin/backups', {
				headers: { 'Authorization': `Bearer ${api.getToken()}` }
			});
			const json = await res.json();
			if (res.ok) {
				schedules = json.data || [];
			}
		} catch {
			addToast('Failed to load backup schedules', 'error');
		}
		loading = false;
	}

	async function createSchedule() {
		if (!newName.trim()) {
			addToast('Schedule name is required', 'error');
			return;
		}
		creating = true;
		try {
			const res = await fetch('/api/v1/admin/backups', {
				method: 'POST',
				headers: {
					'Content-Type': 'application/json',
					'Authorization': `Bearer ${api.getToken()}`
				},
				body: JSON.stringify({
					name: newName,
					frequency: newFrequency,
					retention_count: newRetentionCount,
					include_media: newIncludeMedia,
					include_database: newIncludeDatabase,
					storage_path: newStoragePath,
					enabled: newEnabled
				})
			});
			const json = await res.json();
			if (res.ok) {
				addToast('Backup schedule created', 'success');
				showCreateForm = false;
				resetForm();
				await loadSchedules();
			} else {
				addToast(json.error?.message || 'Failed to create schedule', 'error');
			}
		} catch {
			addToast('Failed to create backup schedule', 'error');
		}
		creating = false;
	}

	async function toggleSchedule(schedule: BackupSchedule) {
		try {
			const res = await fetch(`/api/v1/admin/backups/${schedule.id}`, {
				method: 'PATCH',
				headers: {
					'Content-Type': 'application/json',
					'Authorization': `Bearer ${api.getToken()}`
				},
				body: JSON.stringify({ enabled: !schedule.enabled })
			});
			if (res.ok) {
				schedule.enabled = !schedule.enabled;
				schedules = [...schedules];
				addToast(`Schedule ${schedule.enabled ? 'enabled' : 'disabled'}`, 'success');
			}
		} catch {
			addToast('Failed to update schedule', 'error');
		}
	}

	async function deleteSchedule(scheduleId: string) {
		if (!confirm('Delete this backup schedule and all its history?')) return;
		try {
			const res = await fetch(`/api/v1/admin/backups/${scheduleId}`, {
				method: 'DELETE',
				headers: { 'Authorization': `Bearer ${api.getToken()}` }
			});
			if (res.ok) {
				schedules = schedules.filter(s => s.id !== scheduleId);
				if (historyScheduleId === scheduleId) {
					historyScheduleId = '';
					history = [];
				}
				addToast('Backup schedule deleted', 'success');
			}
		} catch {
			addToast('Failed to delete schedule', 'error');
		}
	}

	async function triggerBackup(scheduleId: string) {
		triggeringId = scheduleId;
		try {
			const res = await fetch(`/api/v1/admin/backups/${scheduleId}/run`, {
				method: 'POST',
				headers: { 'Authorization': `Bearer ${api.getToken()}` }
			});
			const json = await res.json();
			if (res.ok) {
				addToast('Backup triggered successfully', 'success');
				await loadSchedules();
				if (historyScheduleId === scheduleId) {
					await loadHistory(scheduleId);
				}
			} else {
				addToast(json.error?.message || 'Failed to trigger backup', 'error');
			}
		} catch {
			addToast('Failed to trigger backup', 'error');
		}
		triggeringId = '';
	}

	async function loadHistory(scheduleId: string) {
		historyScheduleId = scheduleId;
		loadingHistory = true;
		try {
			const res = await fetch(`/api/v1/admin/backups/${scheduleId}/history`, {
				headers: { 'Authorization': `Bearer ${api.getToken()}` }
			});
			const json = await res.json();
			if (res.ok) {
				history = json.data || [];
			}
		} catch {
			addToast('Failed to load backup history', 'error');
		}
		loadingHistory = false;
	}

	function resetForm() {
		newName = '';
		newFrequency = 'daily';
		newRetentionCount = 7;
		newIncludeMedia = false;
		newIncludeDatabase = true;
		newStoragePath = '/backups';
		newEnabled = true;
	}

	function formatDate(date: string | null): string {
		if (!date) return 'Never';
		return new Date(date).toLocaleDateString(undefined, {
			year: 'numeric', month: 'short', day: 'numeric',
			hour: '2-digit', minute: '2-digit'
		});
	}

	function formatBytes(bytes: number | null): string {
		if (bytes === null || bytes === 0) return '0 B';
		const k = 1024;
		const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
		const i = Math.floor(Math.log(bytes) / Math.log(k));
		return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i];
	}

	function statusBadge(status: string | null): { color: string; label: string } {
		switch (status) {
			case 'completed': return { color: 'bg-status-online/20 text-status-online', label: 'Completed' };
			case 'running': return { color: 'bg-brand-500/20 text-brand-400', label: 'Running' };
			case 'failed': return { color: 'bg-status-dnd/20 text-status-dnd', label: 'Failed' };
			default: return { color: 'bg-bg-modifier text-text-muted', label: status || 'Unknown' };
		}
	}

	onMount(() => {
		loadSchedules();
	});
</script>

<div class="space-y-6">
	<!-- Header -->
	<div class="flex items-center justify-between">
		<div>
			<h2 class="text-xl font-bold text-text-primary">Backup Scheduler</h2>
			<p class="text-text-muted text-sm">Automated database and media backups with configurable retention</p>
		</div>
		<button
			class="btn-primary text-sm px-4 py-2"
			onclick={() => showCreateForm = !showCreateForm}
		>
			{showCreateForm ? 'Cancel' : 'New Schedule'}
		</button>
	</div>

	<!-- Create Form -->
	{#if showCreateForm}
		<div class="bg-bg-tertiary rounded-lg p-5 border border-bg-modifier">
			<h3 class="text-sm font-semibold text-text-secondary mb-4">New Backup Schedule</h3>
			<div class="grid grid-cols-1 md:grid-cols-2 gap-4">
				<div>
					<label for="bk-name" class="block text-sm font-medium text-text-secondary mb-1">Name *</label>
					<input id="bk-name" type="text" class="input w-full" placeholder="Daily Database Backup" bind:value={newName} />
				</div>
				<div>
					<label for="bk-freq" class="block text-sm font-medium text-text-secondary mb-1">Frequency</label>
					<select id="bk-freq" class="input w-full" bind:value={newFrequency}>
						<option value="hourly">Hourly</option>
						<option value="daily">Daily</option>
						<option value="weekly">Weekly</option>
						<option value="monthly">Monthly</option>
					</select>
				</div>
				<div>
					<label for="bk-retention" class="block text-sm font-medium text-text-secondary mb-1">Retention (keep last N)</label>
					<input id="bk-retention" type="number" class="input w-full" min="1" max="365" bind:value={newRetentionCount} />
				</div>
				<div>
					<label for="bk-path" class="block text-sm font-medium text-text-secondary mb-1">Storage Path</label>
					<input id="bk-path" type="text" class="input w-full" placeholder="/backups" bind:value={newStoragePath} />
				</div>
			</div>

			<div class="flex gap-6 mt-4">
				<label class="flex items-center gap-2 text-sm text-text-primary">
					<input type="checkbox" bind:checked={newIncludeDatabase} />
					Include database
				</label>
				<label class="flex items-center gap-2 text-sm text-text-primary">
					<input type="checkbox" bind:checked={newIncludeMedia} />
					Include media files
				</label>
				<label class="flex items-center gap-2 text-sm text-text-primary">
					<input type="checkbox" bind:checked={newEnabled} />
					Enable immediately
				</label>
			</div>

			{#if newIncludeMedia}
				<div class="bg-status-idle/10 border border-status-idle/30 rounded-lg p-3 mt-3">
					<p class="text-xs text-text-secondary">
						Including media files can significantly increase backup size and duration.
						Consider using S3 bucket versioning instead for media backup.
					</p>
				</div>
			{/if}

			<div class="flex justify-end gap-3 mt-4">
				<button class="btn-secondary px-4 py-2 text-sm" onclick={() => { showCreateForm = false; resetForm(); }}>
					Cancel
				</button>
				<button class="btn-primary px-4 py-2 text-sm" onclick={createSchedule} disabled={creating}>
					{creating ? 'Creating...' : 'Create Schedule'}
				</button>
			</div>
		</div>
	{/if}

	<!-- Schedules List -->
	{#if loading && schedules.length === 0}
		<div class="flex justify-center py-12">
			<div class="animate-spin w-8 h-8 border-2 border-brand-500 border-t-transparent rounded-full"></div>
		</div>
	{:else if schedules.length === 0}
		<div class="bg-bg-tertiary rounded-lg p-8 text-center">
			<svg class="w-12 h-12 text-text-muted mx-auto mb-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M4 7v10c0 2.21 3.582 4 8 4s8-1.79 8-4V7M4 7c0 2.21 3.582 4 8 4s8-1.79 8-4M4 7c0-2.21 3.582-4 8-4s8 1.79 8 4m0 5c0 2.21-3.582 4-8 4s-8-1.79-8-4" />
			</svg>
			<p class="text-text-muted">No backup schedules configured.</p>
			<p class="text-text-muted text-sm mt-1">Create a schedule to automate backups.</p>
		</div>
	{:else}
		<div class="space-y-3">
			{#each schedules as schedule}
				<div class="bg-bg-tertiary rounded-lg p-4">
					<div class="flex items-start justify-between">
						<div class="flex-1">
							<div class="flex items-center gap-2 mb-1">
								<span class="text-text-primary font-medium">{schedule.name}</span>
								<span class="text-xs px-2 py-0.5 rounded-full {schedule.enabled ? 'bg-status-online/20 text-status-online' : 'bg-bg-modifier text-text-muted'}">
									{schedule.enabled ? 'Active' : 'Disabled'}
								</span>
								{#if schedule.last_run_status}
									{@const badge = statusBadge(schedule.last_run_status)}
									<span class="text-xs px-2 py-0.5 rounded-full {badge.color}">
										{badge.label}
									</span>
								{/if}
							</div>
							<div class="grid grid-cols-2 md:grid-cols-4 gap-x-4 gap-y-1 text-sm mt-2">
								<div>
									<span class="text-text-muted">Frequency:</span>
									<span class="text-text-secondary ml-1 capitalize">{schedule.frequency}</span>
								</div>
								<div>
									<span class="text-text-muted">Retention:</span>
									<span class="text-text-secondary ml-1">{schedule.retention_count} backups</span>
								</div>
								<div>
									<span class="text-text-muted">Last Run:</span>
									<span class="text-text-secondary ml-1">{formatDate(schedule.last_run_at)}</span>
								</div>
								<div>
									<span class="text-text-muted">Next Run:</span>
									<span class="text-text-secondary ml-1">{formatDate(schedule.next_run_at)}</span>
								</div>
							</div>
							<div class="flex gap-3 mt-1 text-xs text-text-muted">
								{#if schedule.include_database}<span>Database</span>{/if}
								{#if schedule.include_media}<span>Media</span>{/if}
								<span>Path: {schedule.storage_path}</span>
								{#if schedule.last_run_size_bytes}
									<span>Last size: {formatBytes(schedule.last_run_size_bytes)}</span>
								{/if}
								<span>By {schedule.creator_name}</span>
							</div>
						</div>
						<div class="flex items-center gap-2 ml-4">
							<button
								class="btn-secondary text-xs px-3 py-1"
								onclick={() => triggerBackup(schedule.id)}
								disabled={triggeringId === schedule.id}
							>
								{triggeringId === schedule.id ? 'Running...' : 'Run Now'}
							</button>
							<button
								class="btn-secondary text-xs px-3 py-1"
								onclick={() => loadHistory(schedule.id)}
							>
								History
							</button>
							<button
								class="text-xs px-3 py-1 rounded {schedule.enabled ? 'bg-status-idle/20 text-status-idle hover:bg-status-idle/30' : 'bg-status-online/20 text-status-online hover:bg-status-online/30'} transition-colors"
								onclick={() => toggleSchedule(schedule)}
							>
								{schedule.enabled ? 'Disable' : 'Enable'}
							</button>
							<button
								class="text-xs px-3 py-1 rounded bg-status-dnd/20 text-status-dnd hover:bg-status-dnd/30 transition-colors"
								onclick={() => deleteSchedule(schedule.id)}
							>
								Delete
							</button>
						</div>
					</div>
				</div>
			{/each}
		</div>
	{/if}

	<!-- Backup History -->
	{#if historyScheduleId}
		{@const historySchedule = schedules.find(s => s.id === historyScheduleId)}
		<div class="bg-bg-tertiary rounded-lg p-5">
			<div class="flex items-center justify-between mb-4">
				<h3 class="text-sm font-semibold text-text-secondary">
					Backup History
					{#if historySchedule}
						- {historySchedule.name}
					{/if}
				</h3>
				<button class="text-xs text-text-muted hover:text-text-secondary" onclick={() => { historyScheduleId = ''; history = []; }}>
					Close
				</button>
			</div>

			{#if loadingHistory}
				<div class="flex justify-center py-6">
					<div class="animate-spin w-6 h-6 border-2 border-brand-500 border-t-transparent rounded-full"></div>
				</div>
			{:else if history.length === 0}
				<p class="text-text-muted text-sm text-center py-4">No backup history yet.</p>
			{:else}
				<div class="space-y-2">
					{#each history as entry}
						{@const badge = statusBadge(entry.status)}
						<div class="flex items-center justify-between py-2 border-b border-bg-modifier last:border-0">
							<div class="flex items-center gap-3">
								<span class="text-xs px-2 py-0.5 rounded-full {badge.color}">{badge.label}</span>
								<span class="text-text-secondary text-sm">{formatDate(entry.started_at)}</span>
							</div>
							<div class="flex items-center gap-4 text-sm text-text-muted">
								{#if entry.size_bytes}
									<span>{formatBytes(entry.size_bytes)}</span>
								{/if}
								{#if entry.completed_at}
									{@const duration = new Date(entry.completed_at).getTime() - new Date(entry.started_at).getTime()}
									<span>{(duration / 1000).toFixed(1)}s</span>
								{/if}
								{#if entry.error_message}
									<span class="text-status-dnd text-xs" title={entry.error_message}>Error</span>
								{/if}
							</div>
						</div>
					{/each}
				</div>
			{/if}
		</div>
	{/if}
</div>
