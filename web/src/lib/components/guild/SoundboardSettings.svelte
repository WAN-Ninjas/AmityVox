<script lang="ts">
	import { api } from '$lib/api/client';

	let { guildId }: { guildId: string } = $props();

	interface SoundboardSound {
		id: string;
		guild_id: string;
		name: string;
		file_url: string;
		volume: number;
		duration_ms: number;
		emoji: string | null;
		creator_id: string;
		play_count: number;
		created_at: string;
	}

	interface SoundboardConfig {
		guild_id: string;
		enabled: boolean;
		max_sounds: number;
		cooldown_seconds: number;
		allow_external: boolean;
	}

	// State
	let sounds = $state<SoundboardSound[]>([]);
	let config = $state<SoundboardConfig>({
		guild_id: '',
		enabled: true,
		max_sounds: 8,
		cooldown_seconds: 5,
		allow_external: false
	});
	let loading = $state(true);
	let saving = $state(false);
	let uploading = $state(false);
	let error = $state<string | null>(null);
	let successMessage = $state<string | null>(null);

	// New sound form
	let showAddForm = $state(false);
	let newSoundName = $state('');
	let newSoundEmoji = $state('');
	let newSoundVolume = $state(1.0);
	let newSoundFileUrl = $state('');
	let newSoundDuration = $state(0);
	let selectedFile = $state<File | null>(null);

	// Confirmation state for deletes
	let deleteConfirm = $state<string | null>(null);

	$effect(() => {
		if (guildId) {
			loadData();
		}
	});

	async function loadData() {
		try {
			loading = true;
			error = null;

			// Load config and sounds in parallel
			const [configRes, soundsRes] = await Promise.all([
				fetch(`/api/v1/guilds/${guildId}/soundboard/config`, {
					headers: { 'Authorization': `Bearer ${api.getToken()}` }
				}),
				fetch(`/api/v1/guilds/${guildId}/soundboard/sounds`, {
					headers: { 'Authorization': `Bearer ${api.getToken()}` }
				})
			]);

			if (configRes.ok) {
				const configJson = await configRes.json();
				config = configJson.data;
			}
			if (soundsRes.ok) {
				const soundsJson = await soundsRes.json();
				sounds = soundsJson.data || [];
			}
		} catch (err) {
			error = 'Failed to load soundboard settings';
			console.error('Soundboard settings load error:', err);
		} finally {
			loading = false;
		}
	}

	async function saveConfig() {
		try {
			saving = true;
			error = null;
			successMessage = null;

			const res = await fetch(`/api/v1/guilds/${guildId}/soundboard/config`, {
				method: 'PATCH',
				headers: {
					'Content-Type': 'application/json',
					'Authorization': `Bearer ${api.getToken()}`
				},
				body: JSON.stringify({
					enabled: config.enabled,
					max_sounds: config.max_sounds,
					cooldown_seconds: config.cooldown_seconds,
					allow_external: config.allow_external
				})
			});

			if (!res.ok) {
				const errJson = await res.json();
				error = errJson.error?.message || 'Failed to save config';
				return;
			}

			const json = await res.json();
			config = json.data;
			successMessage = 'Settings saved successfully';
			setTimeout(() => successMessage = null, 3000);
		} catch (err) {
			error = 'Failed to save soundboard config';
		} finally {
			saving = false;
		}
	}

	async function handleFileSelect(event: Event) {
		const target = event.target as HTMLInputElement;
		const file = target.files?.[0];
		if (!file) return;

		// Validate file type
		if (!file.type.startsWith('audio/')) {
			error = 'Please select an audio file';
			return;
		}

		// Validate file size (max 500KB for short clips)
		if (file.size > 512 * 1024) {
			error = 'Audio file must be under 500KB';
			return;
		}

		selectedFile = file;

		// Get audio duration
		const audio = new Audio();
		audio.src = URL.createObjectURL(file);
		audio.onloadedmetadata = () => {
			const durationMs = Math.round(audio.duration * 1000);
			if (durationMs > 5000) {
				error = 'Sound must be 5 seconds or shorter';
				selectedFile = null;
				return;
			}
			newSoundDuration = durationMs;
			URL.revokeObjectURL(audio.src);
		};
	}

	async function uploadAndCreateSound() {
		if (!selectedFile || !newSoundName.trim()) {
			error = 'Name and audio file are required';
			return;
		}

		try {
			uploading = true;
			error = null;

			// Upload file first
			const formData = new FormData();
			formData.append('file', selectedFile);
			const uploadRes = await fetch('/api/v1/files/upload', {
				method: 'POST',
				headers: { 'Authorization': `Bearer ${api.getToken()}` },
				body: formData
			});

			if (!uploadRes.ok) {
				const errJson = await uploadRes.json();
				error = errJson.error?.message || 'Failed to upload audio file';
				return;
			}

			const uploadJson = await uploadRes.json();
			const fileId = uploadJson.data.id;

			// Create the sound
			const createRes = await fetch(`/api/v1/guilds/${guildId}/soundboard/sounds`, {
				method: 'POST',
				headers: {
					'Content-Type': 'application/json',
					'Authorization': `Bearer ${api.getToken()}`
				},
				body: JSON.stringify({
					name: newSoundName.trim(),
					file_url: fileId,
					volume: newSoundVolume,
					duration_ms: newSoundDuration,
					emoji: newSoundEmoji.trim() || null
				})
			});

			if (!createRes.ok) {
				const errJson = await createRes.json();
				error = errJson.error?.message || 'Failed to create sound';
				return;
			}

			const createJson = await createRes.json();
			sounds = [...sounds, createJson.data];

			// Reset form
			resetAddForm();
			successMessage = 'Sound added successfully';
			setTimeout(() => successMessage = null, 3000);
		} catch (err) {
			error = 'Failed to create sound';
			console.error('Sound creation error:', err);
		} finally {
			uploading = false;
		}
	}

	async function deleteSound(soundId: string) {
		try {
			error = null;
			const res = await fetch(`/api/v1/guilds/${guildId}/soundboard/sounds/${soundId}`, {
				method: 'DELETE',
				headers: { 'Authorization': `Bearer ${api.getToken()}` }
			});

			if (!res.ok) {
				const errJson = await res.json();
				error = errJson.error?.message || 'Failed to delete sound';
				return;
			}

			sounds = sounds.filter(s => s.id !== soundId);
			deleteConfirm = null;
			successMessage = 'Sound deleted';
			setTimeout(() => successMessage = null, 3000);
		} catch (err) {
			error = 'Failed to delete sound';
		}
	}

	function resetAddForm() {
		showAddForm = false;
		newSoundName = '';
		newSoundEmoji = '';
		newSoundVolume = 1.0;
		newSoundFileUrl = '';
		newSoundDuration = 0;
		selectedFile = null;
	}

	function formatDuration(ms: number): string {
		const seconds = (ms / 1000).toFixed(1);
		return `${seconds}s`;
	}

	function formatDate(dateStr: string): string {
		return new Date(dateStr).toLocaleDateString();
	}
</script>

<div class="soundboard-settings">
	<div class="section-header">
		<h3 class="section-title">Soundboard</h3>
		<p class="section-description">
			Configure sound clips that members can play in voice channels.
		</p>
	</div>

	{#if loading}
		<div class="loading-state">Loading soundboard settings...</div>
	{:else}
		<!-- Configuration Section -->
		<div class="config-section">
			<h4 class="subsection-title">Configuration</h4>

			<div class="config-grid">
				<label class="config-toggle">
					<input type="checkbox" bind:checked={config.enabled} />
					<span class="toggle-text">
						<strong>Enable Soundboard</strong>
						<span class="toggle-desc">Allow members to play sounds in voice channels</span>
					</span>
				</label>

				<div class="config-field">
					<label class="field-label">Maximum Sounds</label>
					<input
						type="number"
						class="input field-input"
						bind:value={config.max_sounds}
						min={1}
						max={100}
					/>
					<span class="field-hint">Number of sound slots available (1-100)</span>
				</div>

				<div class="config-field">
					<label class="field-label">Cooldown (seconds)</label>
					<input
						type="number"
						class="input field-input"
						bind:value={config.cooldown_seconds}
						min={0}
						max={300}
					/>
					<span class="field-hint">Time between plays per user (0 = no cooldown)</span>
				</div>

				<label class="config-toggle">
					<input type="checkbox" bind:checked={config.allow_external} />
					<span class="toggle-text">
						<strong>Allow External Sounds</strong>
						<span class="toggle-desc">Allow sounds from other guilds to be played here</span>
					</span>
				</label>
			</div>

			<div class="config-actions">
				<button
					class="btn-primary"
					onclick={saveConfig}
					disabled={saving}
				>
					{saving ? 'Saving...' : 'Save Config'}
				</button>
			</div>
		</div>

		<!-- Sounds List Section -->
		<div class="sounds-section">
			<div class="sounds-header">
				<h4 class="subsection-title">
					Sounds
					<span class="count">{sounds.length}/{config.max_sounds}</span>
				</h4>
				{#if sounds.length < config.max_sounds}
					<button
						class="btn-add"
						onclick={() => showAddForm = !showAddForm}
					>
						{showAddForm ? 'Cancel' : '+ Add Sound'}
					</button>
				{/if}
			</div>

			{#if showAddForm}
				<div class="add-form">
					<div class="form-row">
						<div class="form-field">
							<label class="field-label">Name</label>
							<input
								type="text"
								class="input"
								bind:value={newSoundName}
								placeholder="Sound name"
								maxlength={32}
							/>
						</div>
						<div class="form-field emoji-field">
							<label class="field-label">Emoji</label>
							<input
								type="text"
								class="input"
								bind:value={newSoundEmoji}
								placeholder="Optional"
								maxlength={4}
							/>
						</div>
					</div>

					<div class="form-row">
						<div class="form-field">
							<label class="field-label">Audio File</label>
							<input
								type="file"
								accept="audio/*"
								onchange={handleFileSelect}
								class="file-input"
							/>
							<span class="field-hint">Max 500KB, max 5 seconds. MP3, OGG, WAV supported.</span>
						</div>
					</div>

					{#if selectedFile}
						<div class="form-row">
							<div class="form-field">
								<label class="field-label">
									Volume
									<span class="field-value">{Math.round(newSoundVolume * 100)}%</span>
								</label>
								<input
									type="range"
									min="0.1"
									max="2"
									step="0.1"
									bind:value={newSoundVolume}
									class="slider"
								/>
							</div>
						</div>

						{#if newSoundDuration > 0}
							<div class="file-info">
								Selected: {selectedFile.name} ({formatDuration(newSoundDuration)})
							</div>
						{/if}
					{/if}

					<div class="form-actions">
						<button class="btn-secondary" onclick={resetAddForm}>
							Cancel
						</button>
						<button
							class="btn-primary"
							onclick={uploadAndCreateSound}
							disabled={uploading || !selectedFile || !newSoundName.trim()}
						>
							{uploading ? 'Uploading...' : 'Add Sound'}
						</button>
					</div>
				</div>
			{/if}

			{#if sounds.length === 0}
				<div class="empty-state">
					<p>No sounds added yet.</p>
					<p class="empty-hint">Add sound clips for members to play in voice channels.</p>
				</div>
			{:else}
				<div class="sounds-list">
					{#each sounds as sound (sound.id)}
						<div class="sound-item">
							<div class="sound-icon">
								{#if sound.emoji}
									<span class="emoji">{sound.emoji}</span>
								{:else}
									<svg width="20" height="20" viewBox="0 0 24 24" fill="currentColor">
										<path d="M3 9v6h4l5 5V4L7 9H3z"/>
									</svg>
								{/if}
							</div>
							<div class="sound-details">
								<span class="sound-name">{sound.name}</span>
								<span class="sound-meta">
									{formatDuration(sound.duration_ms)}
									- Volume: {Math.round(sound.volume * 100)}%
									- {sound.play_count} play{sound.play_count !== 1 ? 's' : ''}
									- Added {formatDate(sound.created_at)}
								</span>
							</div>
							<div class="sound-actions">
								{#if deleteConfirm === sound.id}
									<button
										class="btn-delete-confirm"
										onclick={() => deleteSound(sound.id)}
									>
										Confirm
									</button>
									<button
										class="btn-cancel-sm"
										onclick={() => deleteConfirm = null}
									>
										Cancel
									</button>
								{:else}
									<button
										class="btn-delete"
										onclick={() => deleteConfirm = sound.id}
										title="Delete sound"
									>
										<svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor">
											<path d="M6 19c0 1.1.9 2 2 2h8c1.1 0 2-.9 2-2V7H6v12zM19 4h-3.5l-1-1h-5l-1 1H5v2h14V4z"/>
										</svg>
									</button>
								{/if}
							</div>
						</div>
					{/each}
				</div>
			{/if}
		</div>
	{/if}

	{#if error}
		<div class="message error">{error}</div>
	{/if}
	{#if successMessage}
		<div class="message success">{successMessage}</div>
	{/if}
</div>

<style>
	.soundboard-settings {
		display: flex;
		flex-direction: column;
		gap: 24px;
	}

	.section-header {
		margin-bottom: 4px;
	}

	.section-title {
		margin: 0 0 4px;
		font-size: 18px;
		font-weight: 700;
		color: var(--text-primary);
	}

	.section-description {
		margin: 0;
		font-size: 14px;
		color: var(--text-muted);
	}

	.config-section, .sounds-section {
		display: flex;
		flex-direction: column;
		gap: 16px;
		padding: 16px;
		background: var(--bg-secondary);
		border-radius: 8px;
	}

	.subsection-title {
		margin: 0;
		font-size: 14px;
		font-weight: 600;
		color: var(--text-primary);
		display: flex;
		align-items: center;
		gap: 8px;
	}

	.count {
		font-size: 12px;
		font-weight: 400;
		color: var(--text-muted);
	}

	.config-grid {
		display: flex;
		flex-direction: column;
		gap: 14px;
	}

	.config-toggle {
		display: flex;
		align-items: flex-start;
		gap: 12px;
		cursor: pointer;
	}

	.toggle-text {
		display: flex;
		flex-direction: column;
		gap: 2px;
	}

	.toggle-text strong {
		font-size: 14px;
		color: var(--text-primary);
	}

	.toggle-desc {
		font-size: 12px;
		color: var(--text-muted);
	}

	.config-field {
		display: flex;
		flex-direction: column;
		gap: 4px;
	}

	.field-label {
		font-size: 12px;
		font-weight: 600;
		color: var(--text-secondary);
		text-transform: uppercase;
		letter-spacing: 0.02em;
		display: flex;
		justify-content: space-between;
		align-items: center;
	}

	.field-value {
		font-weight: 400;
		color: var(--text-muted);
		text-transform: none;
	}

	.field-input {
		max-width: 120px;
	}

	.field-hint {
		font-size: 11px;
		color: var(--text-muted);
	}

	.config-actions {
		display: flex;
		justify-content: flex-end;
	}

	.sounds-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
	}

	.btn-add {
		padding: 6px 14px;
		background: var(--brand-500);
		color: white;
		border: none;
		border-radius: 4px;
		cursor: pointer;
		font-size: 13px;
		font-weight: 500;
	}

	.btn-add:hover {
		filter: brightness(1.1);
	}

	.add-form {
		display: flex;
		flex-direction: column;
		gap: 12px;
		padding: 14px;
		background: var(--bg-tertiary);
		border-radius: 8px;
	}

	.form-row {
		display: flex;
		gap: 12px;
	}

	.form-field {
		flex: 1;
		display: flex;
		flex-direction: column;
		gap: 4px;
	}

	.emoji-field {
		max-width: 80px;
	}

	.file-input {
		font-size: 13px;
		color: var(--text-secondary);
	}

	.file-info {
		padding: 6px 10px;
		background: var(--bg-primary);
		border-radius: 4px;
		font-size: 12px;
		color: var(--text-secondary);
	}

	.slider {
		width: 100%;
		height: 4px;
		appearance: none;
		background: var(--bg-primary);
		border-radius: 2px;
		outline: none;
		cursor: pointer;
	}

	.slider::-webkit-slider-thumb {
		appearance: none;
		width: 14px;
		height: 14px;
		background: var(--brand-500);
		border-radius: 50%;
		cursor: pointer;
	}

	.form-actions {
		display: flex;
		gap: 8px;
		justify-content: flex-end;
	}

	.sounds-list {
		display: flex;
		flex-direction: column;
		gap: 4px;
	}

	.sound-item {
		display: flex;
		align-items: center;
		gap: 12px;
		padding: 10px 12px;
		background: var(--bg-tertiary);
		border-radius: 6px;
		transition: background 0.1s ease;
	}

	.sound-item:hover {
		background: var(--bg-primary);
	}

	.sound-icon {
		width: 36px;
		height: 36px;
		display: flex;
		align-items: center;
		justify-content: center;
		background: var(--bg-secondary);
		border-radius: 6px;
		color: var(--text-muted);
		flex-shrink: 0;
	}

	.emoji {
		font-size: 18px;
	}

	.sound-details {
		flex: 1;
		display: flex;
		flex-direction: column;
		gap: 2px;
		min-width: 0;
	}

	.sound-name {
		font-size: 14px;
		font-weight: 500;
		color: var(--text-primary);
	}

	.sound-meta {
		font-size: 12px;
		color: var(--text-muted);
	}

	.sound-actions {
		display: flex;
		gap: 4px;
		flex-shrink: 0;
	}

	.btn-delete {
		padding: 6px;
		background: none;
		color: var(--text-muted);
		border: none;
		border-radius: 4px;
		cursor: pointer;
	}

	.btn-delete:hover {
		background: rgba(239, 68, 68, 0.1);
		color: #ef4444;
	}

	.btn-delete-confirm {
		padding: 4px 10px;
		background: #ef4444;
		color: white;
		border: none;
		border-radius: 4px;
		cursor: pointer;
		font-size: 12px;
	}

	.btn-cancel-sm {
		padding: 4px 10px;
		background: var(--bg-primary);
		color: var(--text-secondary);
		border: none;
		border-radius: 4px;
		cursor: pointer;
		font-size: 12px;
	}

	.empty-state {
		padding: 24px;
		text-align: center;
		color: var(--text-muted);
	}

	.empty-state p {
		margin: 0 0 4px;
	}

	.empty-hint {
		font-size: 13px;
	}

	.loading-state {
		padding: 24px;
		text-align: center;
		color: var(--text-muted);
	}

	.message {
		padding: 10px 14px;
		border-radius: 6px;
		font-size: 13px;
	}

	.message.error {
		background: rgba(239, 68, 68, 0.1);
		color: #ef4444;
		border: 1px solid rgba(239, 68, 68, 0.3);
	}

	.message.success {
		background: rgba(34, 197, 94, 0.1);
		color: #22c55e;
		border: 1px solid rgba(34, 197, 94, 0.3);
	}

	.btn-primary {
		padding: 8px 16px;
		background: var(--brand-500);
		color: white;
		border: none;
		border-radius: 4px;
		cursor: pointer;
		font-size: 13px;
		font-weight: 500;
	}

	.btn-primary:hover {
		filter: brightness(1.1);
	}

	.btn-primary:disabled {
		opacity: 0.6;
		cursor: not-allowed;
	}

	.btn-secondary {
		padding: 8px 16px;
		background: var(--bg-primary);
		color: var(--text-secondary);
		border: none;
		border-radius: 4px;
		cursor: pointer;
		font-size: 13px;
	}

	.btn-secondary:hover {
		background: var(--bg-tertiary);
		color: var(--text-primary);
	}

	.input {
		width: 100%;
		padding: 8px 10px;
		background: var(--bg-primary);
		color: var(--text-primary);
		border: 1px solid var(--bg-tertiary);
		border-radius: 4px;
		font-size: 13px;
		outline: none;
		box-sizing: border-box;
	}

	.input:focus {
		border-color: var(--brand-500);
	}
</style>
