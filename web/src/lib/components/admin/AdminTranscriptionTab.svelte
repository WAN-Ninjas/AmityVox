<script lang="ts">
	import { api, type AdminTranscriptionConfig } from '$lib/api/client';
	import { addToast } from '$lib/stores/toast';
	import { createAsyncOp } from '$lib/utils/asyncOp';

	let config = $state<AdminTranscriptionConfig>({
		engine_type: 'none',
		engine_endpoint: '',
		save_enabled: false
	});
	let loadOp = $state(createAsyncOp(true));
	let saveOp = $state(createAsyncOp());

	$effect(() => {
		loadOp.run(async () => {
			config = await api.getAdminTranscriptionConfig();
		}, (message) => addToast(message, 'error'));
	});

	async function saveConfig() {
		await saveOp.run(async () => {
			config = await api.updateAdminTranscriptionConfig(config);
			addToast('Transcription settings saved', 'success');
		}, (message) => addToast(message, 'error'));
	}
</script>

<h1 class="mb-6 text-2xl font-bold text-text-primary">Voice Transcription</h1>

{#if loadOp.loading}
	<p class="text-sm text-text-muted">Loading transcription settings...</p>
{:else}
	<div class="max-w-xl space-y-5">
		<div>
			<label for="transcription-engine" class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">Engine Type</label>
			<select id="transcription-engine" class="input w-full" bind:value={config.engine_type}>
				<option value="none">Disabled / no engine</option>
				<option value="whisper">Whisper-compatible</option>
				<option value="llm">LLM transcription service</option>
				<option value="custom">Custom HTTP service</option>
			</select>
		</div>

		<div>
			<label for="transcription-endpoint" class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">Engine Endpoint</label>
			<input
				id="transcription-endpoint"
				class="input w-full"
				type="url"
				bind:value={config.engine_endpoint}
				placeholder="https://transcribe.example.com"
				disabled={config.engine_type === 'none'}
			/>
		</div>

		<label class="flex items-start gap-3 rounded border border-bg-modifier bg-bg-secondary p-3">
			<input type="checkbox" class="mt-1 accent-brand-500" bind:checked={config.save_enabled} />
			<span>
				<span class="block text-sm font-medium text-text-primary">Allow saved transcripts</span>
				<span class="block text-xs text-text-muted">When off, transcript text must be treated as ephemeral and should not be retained after the call/session closes.</span>
			</span>
		</label>

		<button class="btn-primary" onclick={saveConfig} disabled={saveOp.loading}>
			{saveOp.loading ? 'Saving...' : 'Save Settings'}
		</button>
	</div>
{/if}
