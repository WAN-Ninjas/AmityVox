<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api/client';
	import { addToast } from '$lib/stores/toast';
	import { createAsyncOp } from '$lib/utils/asyncOp';

	interface UpdateInfo {
		current_version: string;
		latest_version: string;
		update_available: boolean;
		release_notes: string;
		release_url: string;
		last_checked: string;
		dismissed: boolean;
	}

	interface UpdateConfig {
		auto_check: boolean;
		channel: string;
		notify_admins: boolean;
	}

	let loading = $state(true);
	let updateInfo = $state<UpdateInfo | null>(null);
	let config = $state<UpdateConfig>({ auto_check: true, channel: 'stable', notify_admins: true });
	let saveOp = $state(createAsyncOp());
	let checkOp = $state(createAsyncOp());
	let dismissOp = $state(createAsyncOp());

	async function loadUpdateInfo() {
		try {
			updateInfo = await api.getAdminUpdates();
		} catch {
			addToast('Failed to load update info', 'error');
		}
	}

	async function loadConfig() {
		try {
			const data = await api.getAdminUpdatesConfig();
			config = {
				auto_check: data.auto_check ?? true,
				channel: data.channel ?? 'stable',
				notify_admins: data.notify_admins ?? true
			};
		} catch {
			addToast('Failed to load update config', 'error');
		} finally {
			loading = false;
		}
	}

	async function saveConfig() {
		await saveOp.run(() => api.updateAdminUpdatesConfig(config), msg => addToast(msg, 'error'));
		if (!saveOp.error) addToast('Update settings saved', 'success');
	}

	async function checkNow() {
		const result = await checkOp.run(() => api.getAdminUpdates(), msg => addToast(msg, 'error'));
		if (!checkOp.error) {
			updateInfo = result!;
			addToast('Update check complete', 'success');
		}
	}

	async function dismissUpdate() {
		await dismissOp.run(() => api.dismissAdminUpdate(), msg => addToast(msg, 'error'));
		if (!dismissOp.error) {
			if (updateInfo) {
				updateInfo = { ...updateInfo, update_available: false, dismissed: true };
			}
			addToast('Update notification dismissed', 'success');
		}
	}

	onMount(() => {
		loadUpdateInfo();
		loadConfig();
	});
</script>

<h1 class="mb-6 text-2xl font-bold text-text-primary">Update Notifications</h1>
<p class="mb-6 text-sm text-text-muted">
	Check for new versions of AmityVox and configure automatic update notifications.
</p>

{#if loading}
	<p class="text-sm text-text-muted">Loading update information...</p>
{:else}
	<div class="space-y-6">
		<!-- Current Version Info -->
		<div class="rounded-lg bg-bg-secondary p-4">
			<h3 class="mb-3 text-xs font-bold uppercase tracking-wide text-text-muted">Version Status</h3>
			<div class="grid gap-2 text-sm">
				<div class="flex justify-between">
					<span class="text-text-muted">Current Version</span>
					<span class="font-mono text-text-primary">{updateInfo?.current_version ?? 'Unknown'}</span>
				</div>
				<div class="flex justify-between">
					<span class="text-text-muted">Latest Version</span>
					<span class="font-mono text-text-primary">{updateInfo?.latest_version || 'Not checked'}</span>
				</div>
				<div class="flex justify-between">
					<span class="text-text-muted">Last Checked</span>
					<span class="text-text-primary">
						{updateInfo?.last_checked ? new Date(updateInfo.last_checked).toLocaleString() : 'Never'}
					</span>
				</div>
			</div>

			{#if updateInfo?.update_available}
				<div class="mt-4 rounded-lg bg-brand-500/10 p-3">
					<div class="flex items-center gap-2">
						<span class="rounded bg-brand-500/20 px-2 py-0.5 text-xs font-bold text-brand-400">Update Available</span>
						<span class="font-mono text-sm text-text-primary">{updateInfo.latest_version}</span>
					</div>
					{#if updateInfo.release_notes}
						<p class="mt-2 text-sm text-text-secondary">{updateInfo.release_notes}</p>
					{/if}
					<div class="mt-3 flex gap-2">
						{#if updateInfo.release_url}
							<a
								href={updateInfo.release_url}
								target="_blank"
								rel="noopener noreferrer"
								class="btn-primary text-sm"
							>
								View Release
							</a>
						{/if}
						<button
							class="btn-secondary text-sm"
							onclick={dismissUpdate}
							disabled={dismissOp.loading}
						>
							{dismissOp.loading ? 'Dismissing...' : 'Dismiss'}
						</button>
					</div>
				</div>
			{:else if updateInfo?.dismissed}
				<p class="mt-3 text-xs text-text-muted">Update notification dismissed for current latest version.</p>
			{:else if updateInfo?.latest_version}
				<p class="mt-3 text-xs text-green-400">You are running the latest version.</p>
			{/if}
		</div>

		<button class="btn-secondary text-sm" onclick={checkNow} disabled={checkOp.loading}>
			{checkOp.loading ? 'Checking...' : 'Check for Updates Now'}
		</button>

		<!-- Update Configuration -->
		<div class="rounded-lg bg-bg-secondary p-4">
			<h3 class="mb-4 text-xs font-bold uppercase tracking-wide text-text-muted">Update Settings</h3>
			<div class="space-y-4">
				<label class="flex items-center gap-3 text-sm text-text-secondary">
					<input type="checkbox" bind:checked={config.auto_check} class="rounded accent-brand-500" />
					<div>
						<span class="font-medium text-text-primary">Automatic Update Checks</span>
						<p class="text-xs text-text-muted">Periodically check for new versions in the background.</p>
					</div>
				</label>

				<div>
					<label class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">Release Channel</label>
					<select class="input w-full" bind:value={config.channel}>
						<option value="stable">Stable</option>
						<option value="beta">Beta</option>
						<option value="nightly">Nightly</option>
					</select>
					<p class="mt-1 text-xs text-text-muted">Which release channel to check for updates.</p>
				</div>

				<label class="flex items-center gap-3 text-sm text-text-secondary">
					<input type="checkbox" bind:checked={config.notify_admins} class="rounded accent-brand-500" />
					<div>
						<span class="font-medium text-text-primary">Notify Admins</span>
						<p class="text-xs text-text-muted">Send a notification to all admins when a new version is available.</p>
					</div>
				</label>
			</div>
		</div>

		<button class="btn-primary" onclick={saveConfig} disabled={saveOp.loading}>
			{saveOp.loading ? 'Saving...' : 'Save Update Settings'}
		</button>
	</div>
{/if}
