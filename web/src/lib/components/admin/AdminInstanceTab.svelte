<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api/client';
	import { addToast } from '$lib/stores/toast';
	import type { InstanceInfo } from '$lib/types';

	let instance = $state<InstanceInfo | null>(null);
	let loadingInstance = $state(false);
	let instanceName = $state('');
	let instanceDesc = $state('');
	let instanceFedMode = $state('');
	let savingInstance = $state(false);

	onMount(() => {
		loadInstance();
	});

	async function loadInstance() {
		loadingInstance = true;
		try {
			instance = await api.getAdminInstance();
			instanceName = instance?.name ?? '';
			instanceDesc = instance?.description ?? '';
			const rawMode = instance?.federation_mode ?? 'closed';
			instanceFedMode =
				rawMode === 'allow' ? 'open' :
				rawMode === 'deny' || rawMode === 'disabled' ? 'closed' :
				rawMode;
		} catch {
			instance = null;
		} finally {
			loadingInstance = false;
		}
	}

	async function saveInstance() {
		savingInstance = true;
		try {
			instance = await api.updateAdminInstance({
				name: instanceName || undefined,
				description: instanceDesc || undefined,
				federation_mode: instanceFedMode || undefined
			});
			addToast('Instance settings saved', 'success');
		} catch {
			addToast('Failed to save instance settings', 'error');
		} finally {
			savingInstance = false;
		}
	}
</script>

<h1 class="mb-6 text-2xl font-bold text-text-primary">Instance Settings</h1>

{#if loadingInstance}
	<p class="text-sm text-text-muted">Loading instance settings...</p>
{:else}
	<div class="space-y-4">
		<div>
			<label for="admin-instance-name" class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">Instance Name</label>
			<input id="admin-instance-name" type="text" class="input w-full" bind:value={instanceName} maxlength="100" />
		</div>
		<div>
			<label for="admin-instance-description" class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">Description</label>
			<textarea id="admin-instance-description" class="input w-full" bind:value={instanceDesc} rows="3" maxlength="1024"></textarea>
		</div>
		<div>
			<label for="admin-instance-federation-mode" class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">Federation Mode</label>
			<select id="admin-instance-federation-mode" class="input w-full" bind:value={instanceFedMode}>
				<option value="open">Open (federate with all peers)</option>
				<option value="allowlist">Allowlist (federate only with approved peers)</option>
				<option value="closed">Closed (no federation)</option>
			</select>
		</div>

		{#if instance}
			<div class="rounded-lg bg-bg-secondary p-4">
				<div class="grid gap-2 text-sm">
					<div class="flex justify-between">
						<span class="text-text-muted">Instance ID</span>
						<code class="text-xs text-text-primary">{instance.id}</code>
					</div>
					<div class="flex justify-between">
						<span class="text-text-muted">Domain</span>
						<span class="text-text-primary">{instance.domain}</span>
					</div>
					<div class="flex justify-between">
						<span class="text-text-muted">Software</span>
						<span class="text-text-primary">{instance.software} {instance.software_version}</span>
					</div>
					<div class="flex justify-between">
						<span class="text-text-muted">Created</span>
						<span class="text-text-primary">{new Date(instance.created_at).toLocaleDateString()}</span>
					</div>
				</div>
			</div>
		{/if}

		<button class="btn-primary" onclick={saveInstance} disabled={savingInstance}>
			{savingInstance ? 'Saving...' : 'Save Settings'}
		</button>
	</div>
{/if}
