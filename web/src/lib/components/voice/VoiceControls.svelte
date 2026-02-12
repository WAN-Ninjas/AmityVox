<script lang="ts">
	import { api } from '$lib/api/client';

	let {
		channelId,
		guildId,
		connected = false,
		selfMute = false,
		selfDeaf = false
	}: {
		channelId: string;
		guildId: string;
		connected: boolean;
		selfMute: boolean;
		selfDeaf: boolean;
	} = $props();

	// Voice preferences state
	let inputMode = $state<'vad' | 'ptt'>('vad');
	let pttKey = $state('Space');
	let vadThreshold = $state(0.3);
	let noiseSuppression = $state(true);
	let echoCancellation = $state(true);
	let autoGainControl = $state(true);
	let inputVolume = $state(1.0);
	let outputVolume = $state(1.0);
	let isPrioritySpeaker = $state(false);
	let showSettings = $state(false);
	let pttActive = $state(false);
	let loading = $state(false);
	let saving = $state(false);
	let recordingPTTKey = $state(false);

	// Load voice preferences on mount
	$effect(() => {
		if (connected) {
			loadPreferences();
		}
	});

	// PTT key listener
	$effect(() => {
		if (inputMode === 'ptt' && connected) {
			const handleKeyDown = (e: KeyboardEvent) => {
				if (recordingPTTKey) {
					e.preventDefault();
					pttKey = e.code;
					recordingPTTKey = false;
					savePreferences();
					return;
				}
				if (e.code === pttKey && !pttActive) {
					pttActive = true;
				}
			};
			const handleKeyUp = (e: KeyboardEvent) => {
				if (e.code === pttKey && pttActive) {
					pttActive = false;
				}
			};

			window.addEventListener('keydown', handleKeyDown);
			window.addEventListener('keyup', handleKeyUp);

			return () => {
				window.removeEventListener('keydown', handleKeyDown);
				window.removeEventListener('keyup', handleKeyUp);
			};
		}
	});

	async function loadPreferences() {
		try {
			loading = true;
			const res = await fetch('/api/v1/voice/preferences', {
				headers: { 'Authorization': `Bearer ${api.getToken()}` }
			});
			if (res.ok) {
				const json = await res.json();
				const prefs = json.data;
				inputMode = prefs.input_mode || 'vad';
				pttKey = prefs.ptt_key || 'Space';
				vadThreshold = prefs.vad_threshold ?? 0.3;
				noiseSuppression = prefs.noise_suppression ?? true;
				echoCancellation = prefs.echo_cancellation ?? true;
				autoGainControl = prefs.auto_gain_control ?? true;
				inputVolume = prefs.input_volume ?? 1.0;
				outputVolume = prefs.output_volume ?? 1.0;
			}
		} catch (err) {
			console.error('Failed to load voice preferences:', err);
		} finally {
			loading = false;
		}
	}

	async function savePreferences() {
		try {
			saving = true;
			await fetch('/api/v1/voice/preferences', {
				method: 'PATCH',
				headers: {
					'Content-Type': 'application/json',
					'Authorization': `Bearer ${api.getToken()}`
				},
				body: JSON.stringify({
					input_mode: inputMode,
					ptt_key: pttKey,
					vad_threshold: vadThreshold,
					noise_suppression: noiseSuppression,
					echo_cancellation: echoCancellation,
					auto_gain_control: autoGainControl,
					input_volume: inputVolume,
					output_volume: outputVolume
				})
			});
		} catch (err) {
			console.error('Failed to save voice preferences:', err);
		} finally {
			saving = false;
		}
	}

	async function toggleInputMode() {
		inputMode = inputMode === 'vad' ? 'ptt' : 'vad';
		await savePreferences();

		// Update active voice state
		if (connected) {
			try {
				await fetch(`/api/v1/voice/${channelId}/input-mode`, {
					method: 'POST',
					headers: {
						'Content-Type': 'application/json',
						'Authorization': `Bearer ${api.getToken()}`
					},
					body: JSON.stringify({ mode: inputMode })
				});
			} catch (err) {
				console.error('Failed to set input mode:', err);
			}
		}
	}

	async function togglePrioritySpeaker() {
		try {
			// We need the current user's ID - derive from token context
			const meRes = await fetch('/api/v1/users/@me', {
				headers: { 'Authorization': `Bearer ${api.getToken()}` }
			});
			if (!meRes.ok) return;
			const meJson = await meRes.json();
			const userId = meJson.data.id;

			const newState = !isPrioritySpeaker;
			await fetch(`/api/v1/voice/${channelId}/members/${userId}/priority`, {
				method: 'POST',
				headers: {
					'Content-Type': 'application/json',
					'Authorization': `Bearer ${api.getToken()}`
				},
				body: JSON.stringify({ priority: newState })
			});
			isPrioritySpeaker = newState;
		} catch (err) {
			console.error('Failed to toggle priority speaker:', err);
		}
	}

	function formatKeyName(code: string): string {
		return code
			.replace('Key', '')
			.replace('Digit', '')
			.replace('Arrow', '')
			.replace('Numpad', 'Num ')
			.replace('Control', 'Ctrl')
			.replace('Semicolon', ';')
			.replace('Quote', "'")
			.replace('BracketLeft', '[')
			.replace('BracketRight', ']')
			.replace('Backslash', '\\')
			.replace('Slash', '/')
			.replace('Period', '.')
			.replace('Comma', ',')
			.replace('Minus', '-')
			.replace('Equal', '=')
			.replace('Backquote', '`');
	}
</script>

{#if connected}
	<div class="voice-controls">
		<!-- Input Mode Toggle -->
		<div class="control-row">
			<button
				class="btn-control"
				class:active={inputMode === 'ptt'}
				onclick={toggleInputMode}
				title={inputMode === 'vad' ? 'Switch to Push-to-Talk' : 'Switch to Voice Activity'}
			>
				{#if inputMode === 'vad'}
					<svg width="20" height="20" viewBox="0 0 24 24" fill="currentColor">
						<path d="M12 14c1.66 0 3-1.34 3-3V5c0-1.66-1.34-3-3-3S9 3.34 9 5v6c0 1.66 1.34 3 3 3z"/>
						<path d="M17 11c0 2.76-2.24 5-5 5s-5-2.24-5-5H5c0 3.53 2.61 6.43 6 6.92V21h2v-3.08c3.39-.49 6-3.39 6-6.92h-2z"/>
					</svg>
					<span class="label">VAD</span>
				{:else}
					<svg width="20" height="20" viewBox="0 0 24 24" fill="currentColor">
						<path d="M20 5V19H4V5H20M20 3H4C2.9 3 2 3.9 2 5V19C2 20.1 2.9 21 4 21H20C21.1 21 22 20.1 22 19V5C22 3.9 21.1 3 20 3M11 7H13V17H11V7Z"/>
					</svg>
					<span class="label">PTT</span>
				{/if}
			</button>

			<!-- Priority Speaker -->
			<button
				class="btn-control"
				class:active={isPrioritySpeaker}
				onclick={togglePrioritySpeaker}
				title={isPrioritySpeaker ? 'Disable Priority Speaker' : 'Enable Priority Speaker'}
			>
				<svg width="20" height="20" viewBox="0 0 24 24" fill="currentColor">
					<path d="M12 1L3 5v6c0 5.55 3.84 10.74 9 12 5.16-1.26 9-6.45 9-12V5l-9-4zm0 10.99h7c-.53 4.12-3.28 7.79-7 8.94V12H5V6.3l7-3.11v8.8z"/>
				</svg>
				<span class="label">Priority</span>
			</button>

			<!-- Settings toggle -->
			<button
				class="btn-control"
				class:active={showSettings}
				onclick={() => showSettings = !showSettings}
				title="Voice Settings"
			>
				<svg width="20" height="20" viewBox="0 0 24 24" fill="currentColor">
					<path d="M19.14 12.94c.04-.3.06-.61.06-.94 0-.32-.02-.64-.07-.94l2.03-1.58c.18-.14.23-.41.12-.61l-1.92-3.32c-.12-.22-.37-.29-.59-.22l-2.39.96c-.5-.38-1.03-.7-1.62-.94l-.36-2.54c-.04-.24-.24-.41-.48-.41h-3.84c-.24 0-.43.17-.47.41l-.36 2.54c-.59.24-1.13.57-1.62.94l-2.39-.96c-.22-.08-.47 0-.59.22L2.74 8.87c-.12.21-.08.47.12.61l2.03 1.58c-.05.3-.09.63-.09.94s.02.64.07.94l-2.03 1.58c-.18.14-.23.41-.12.61l1.92 3.32c.12.22.37.29.59.22l2.39-.96c.5.38 1.03.7 1.62.94l.36 2.54c.05.24.24.41.48.41h3.84c.24 0 .44-.17.47-.41l.36-2.54c.59-.24 1.13-.56 1.62-.94l2.39.96c.22.08.47 0 .59-.22l1.92-3.32c.12-.22.07-.47-.12-.61l-2.01-1.58zM12 15.6c-1.98 0-3.6-1.62-3.6-3.6s1.62-3.6 3.6-3.6 3.6 1.62 3.6 3.6-1.62 3.6-3.6 3.6z"/>
				</svg>
			</button>
		</div>

		<!-- PTT indicator -->
		{#if inputMode === 'ptt'}
			<div class="ptt-indicator" class:active={pttActive}>
				<span class="ptt-key">{formatKeyName(pttKey)}</span>
				<span class="ptt-status">{pttActive ? 'Transmitting' : 'Press to talk'}</span>
			</div>
		{/if}

		<!-- Settings panel -->
		{#if showSettings}
			<div class="settings-panel">
				<h4 class="settings-title">Voice Settings</h4>

				<!-- Input Mode -->
				<div class="setting-group">
					<label class="setting-label">Input Mode</label>
					<div class="radio-group">
						<label class="radio-option">
							<input
								type="radio"
								name="inputMode"
								value="vad"
								checked={inputMode === 'vad'}
								onchange={() => { inputMode = 'vad'; savePreferences(); }}
							/>
							<span>Voice Activity</span>
						</label>
						<label class="radio-option">
							<input
								type="radio"
								name="inputMode"
								value="ptt"
								checked={inputMode === 'ptt'}
								onchange={() => { inputMode = 'ptt'; savePreferences(); }}
							/>
							<span>Push to Talk</span>
						</label>
					</div>
				</div>

				<!-- PTT Keybind -->
				{#if inputMode === 'ptt'}
					<div class="setting-group">
						<label class="setting-label">PTT Keybind</label>
						<button
							class="keybind-btn"
							class:recording={recordingPTTKey}
							onclick={() => recordingPTTKey = !recordingPTTKey}
						>
							{recordingPTTKey ? 'Press a key...' : formatKeyName(pttKey)}
						</button>
					</div>
				{/if}

				<!-- VAD Threshold -->
				{#if inputMode === 'vad'}
					<div class="setting-group">
						<label class="setting-label">
							Sensitivity
							<span class="setting-value">{Math.round(vadThreshold * 100)}%</span>
						</label>
						<input
							type="range"
							min="0"
							max="1"
							step="0.05"
							bind:value={vadThreshold}
							onchange={savePreferences}
							class="slider"
						/>
					</div>
				{/if}

				<!-- Input Volume -->
				<div class="setting-group">
					<label class="setting-label">
						Input Volume
						<span class="setting-value">{Math.round(inputVolume * 100)}%</span>
					</label>
					<input
						type="range"
						min="0"
						max="2"
						step="0.05"
						bind:value={inputVolume}
						onchange={savePreferences}
						class="slider"
					/>
				</div>

				<!-- Output Volume -->
				<div class="setting-group">
					<label class="setting-label">
						Output Volume
						<span class="setting-value">{Math.round(outputVolume * 100)}%</span>
					</label>
					<input
						type="range"
						min="0"
						max="2"
						step="0.05"
						bind:value={outputVolume}
						onchange={savePreferences}
						class="slider"
					/>
				</div>

				<!-- Audio Processing -->
				<div class="setting-group">
					<label class="setting-label">Audio Processing</label>
					<div class="toggle-list">
						<label class="toggle-option">
							<input
								type="checkbox"
								bind:checked={noiseSuppression}
								onchange={savePreferences}
							/>
							<span>Noise Suppression</span>
						</label>
						<label class="toggle-option">
							<input
								type="checkbox"
								bind:checked={echoCancellation}
								onchange={savePreferences}
							/>
							<span>Echo Cancellation</span>
						</label>
						<label class="toggle-option">
							<input
								type="checkbox"
								bind:checked={autoGainControl}
								onchange={savePreferences}
							/>
							<span>Auto Gain Control</span>
						</label>
					</div>
				</div>

				{#if saving}
					<p class="saving-indicator">Saving...</p>
				{/if}
			</div>
		{/if}
	</div>
{/if}

<style>
	.voice-controls {
		display: flex;
		flex-direction: column;
		gap: 8px;
		padding: 8px;
		background: var(--bg-secondary);
		border-radius: 8px;
	}

	.control-row {
		display: flex;
		gap: 4px;
		align-items: center;
	}

	.btn-control {
		display: flex;
		align-items: center;
		gap: 4px;
		padding: 6px 10px;
		background: var(--bg-tertiary);
		color: var(--text-secondary);
		border: none;
		border-radius: 6px;
		cursor: pointer;
		font-size: 12px;
		transition: all 0.15s ease;
	}

	.btn-control:hover {
		background: var(--bg-primary);
		color: var(--text-primary);
	}

	.btn-control.active {
		background: var(--brand-500);
		color: white;
	}

	.label {
		font-weight: 500;
	}

	.ptt-indicator {
		display: flex;
		align-items: center;
		gap: 8px;
		padding: 8px 12px;
		background: var(--bg-tertiary);
		border-radius: 6px;
		font-size: 13px;
		color: var(--text-secondary);
		transition: all 0.15s ease;
	}

	.ptt-indicator.active {
		background: var(--brand-500);
		color: white;
	}

	.ptt-key {
		display: inline-block;
		padding: 2px 8px;
		background: var(--bg-primary);
		border-radius: 4px;
		font-family: monospace;
		font-weight: 600;
		font-size: 12px;
	}

	.ptt-indicator.active .ptt-key {
		background: rgba(255, 255, 255, 0.2);
	}

	.ptt-status {
		flex: 1;
	}

	.settings-panel {
		display: flex;
		flex-direction: column;
		gap: 12px;
		padding: 12px;
		background: var(--bg-tertiary);
		border-radius: 8px;
		max-height: 400px;
		overflow-y: auto;
	}

	.settings-title {
		margin: 0;
		font-size: 14px;
		font-weight: 600;
		color: var(--text-primary);
	}

	.setting-group {
		display: flex;
		flex-direction: column;
		gap: 6px;
	}

	.setting-label {
		display: flex;
		justify-content: space-between;
		align-items: center;
		font-size: 12px;
		font-weight: 500;
		color: var(--text-secondary);
		text-transform: uppercase;
		letter-spacing: 0.02em;
	}

	.setting-value {
		color: var(--text-muted);
		font-weight: 400;
		text-transform: none;
	}

	.radio-group {
		display: flex;
		gap: 12px;
	}

	.radio-option {
		display: flex;
		align-items: center;
		gap: 6px;
		font-size: 13px;
		color: var(--text-primary);
		cursor: pointer;
	}

	.keybind-btn {
		padding: 6px 12px;
		background: var(--bg-primary);
		color: var(--text-primary);
		border: 1px solid var(--bg-secondary);
		border-radius: 4px;
		cursor: pointer;
		font-family: monospace;
		font-size: 13px;
		text-align: center;
	}

	.keybind-btn.recording {
		border-color: var(--brand-500);
		color: var(--brand-400);
		animation: pulse 1s infinite;
	}

	@keyframes pulse {
		0%, 100% { opacity: 1; }
		50% { opacity: 0.6; }
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

	.toggle-list {
		display: flex;
		flex-direction: column;
		gap: 6px;
	}

	.toggle-option {
		display: flex;
		align-items: center;
		gap: 8px;
		font-size: 13px;
		color: var(--text-primary);
		cursor: pointer;
	}

	.saving-indicator {
		margin: 0;
		font-size: 11px;
		color: var(--text-muted);
		text-align: center;
	}
</style>
