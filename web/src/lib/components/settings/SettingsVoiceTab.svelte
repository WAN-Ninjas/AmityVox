<script lang="ts">
	import { api } from '$lib/api/client';
	import type { VoicePreferences } from '$lib/types';

	let voicePrefs = $state<VoicePreferences | null>(null);
	let voiceLoading = $state(false);
	let voiceSaving = $state(false);
	let voiceSuccess = $state('');
	let voiceError = $state('');
	let inputDeviceId = $state('');
	let outputDeviceId = $state('');
	let availableInputDevices = $state<MediaDeviceInfo[]>([]);
	let availableOutputDevices = $state<MediaDeviceInfo[]>([]);
	let recordingVoicePTTKey = $state(false);
	let loaded = false;

	$effect(() => {
		if (!loaded) {
			loaded = true;
			loadVoicePreferences();
		}
	});

	$effect(() => {
		if (recordingVoicePTTKey && voicePrefs) {
			const handleKeyDown = (e: KeyboardEvent) => {
				e.preventDefault();
				if (voicePrefs) {
					voicePrefs.ptt_key = e.code;
				}
				recordingVoicePTTKey = false;
			};
			window.addEventListener('keydown', handleKeyDown);
			return () => window.removeEventListener('keydown', handleKeyDown);
		}
	});

	async function loadVoicePreferences() {
		voiceLoading = true;
		voiceError = '';
		try {
			voicePrefs = await api.getVoicePreferences();
			inputDeviceId = localStorage.getItem('av-voice-input-device') ?? '';
			outputDeviceId = localStorage.getItem('av-voice-output-device') ?? '';
			if (navigator.mediaDevices?.enumerateDevices) {
				const devices = await navigator.mediaDevices.enumerateDevices();
				availableInputDevices = devices.filter((device) => device.kind === 'audioinput');
				availableOutputDevices = devices.filter((device) => device.kind === 'audiooutput');
			}
		} catch (err: any) {
			voiceError = err.message || 'Failed to load voice preferences';
		} finally {
			voiceLoading = false;
		}
	}

	async function saveVoicePreferences() {
		if (!voicePrefs) return;
		voiceSaving = true;
		voiceError = '';
		voiceSuccess = '';
		try {
			voicePrefs = await api.updateVoicePreferences({
				input_mode: voicePrefs.input_mode,
				ptt_key: voicePrefs.ptt_key,
				vad_threshold: voicePrefs.vad_threshold,
				noise_suppression: voicePrefs.noise_suppression,
				echo_cancellation: voicePrefs.echo_cancellation,
				auto_gain_control: voicePrefs.auto_gain_control,
				input_volume: voicePrefs.input_volume,
				output_volume: voicePrefs.output_volume,
				camera_resolution: voicePrefs.camera_resolution,
				camera_framerate: voicePrefs.camera_framerate,
				screenshare_resolution: voicePrefs.screenshare_resolution,
				screenshare_framerate: voicePrefs.screenshare_framerate,
				screenshare_audio: voicePrefs.screenshare_audio
			});
			localStorage.setItem('av-voice-input-device', inputDeviceId);
			localStorage.setItem('av-voice-output-device', outputDeviceId);
			voiceSuccess = 'Voice preferences saved!';
			setTimeout(() => (voiceSuccess = ''), 3000);
		} catch (err: any) {
			voiceError = err.message || 'Failed to save voice preferences';
		} finally {
			voiceSaving = false;
		}
	}

	function formatVoiceKeyName(code: string): string {
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

<h1 class="mb-6 text-xl font-bold text-text-primary">Voice & Video</h1>

{#if voiceLoading}
	<p class="text-sm text-text-muted">Loading voice preferences...</p>
{:else if voicePrefs}
	{#if voiceError}
		<div class="mb-4 rounded-lg bg-red-500/10 px-4 py-2 text-sm text-red-400">{voiceError}</div>
	{/if}
	{#if voiceSuccess}
		<div class="mb-4 rounded-lg bg-status-online/10 px-4 py-2 text-sm text-status-online">{voiceSuccess}</div>
	{/if}

	<div class="mb-6">
		<h2 class="mb-3 text-2xs font-medium uppercase tracking-wide text-text-muted">Audio Input</h2>
		<div class="flex flex-col gap-4 rounded-lg bg-bg-secondary p-4">
			<div class="flex flex-col gap-1">
				<label for="voice-input-device" class="text-sm font-medium text-text-secondary">Input Device</label>
				<select
					id="voice-input-device"
					class="rounded border border-bg-tertiary bg-bg-primary px-3 py-2 text-sm text-text-primary outline-none focus:border-brand-500"
					bind:value={inputDeviceId}
				>
					<option value="">Default</option>
					{#each availableInputDevices as device}
						<option value={device.deviceId}>{device.label || `Microphone ${device.deviceId.slice(0, 8)}`}</option>
					{/each}
				</select>
			</div>

			<div class="flex flex-col gap-1">
				<label for="voice-input-volume" class="flex items-center justify-between text-sm font-medium text-text-secondary">
					Input Volume
					<span class="text-xs text-text-muted">{Math.round(voicePrefs.input_volume * 100)}%</span>
				</label>
				<input
					id="voice-input-volume"
					type="range"
					min="0"
					max="2"
					step="0.05"
					bind:value={voicePrefs.input_volume}
					class="w-full accent-brand-500"
				/>
			</div>

			<label class="flex items-center justify-between">
				<span class="text-sm text-text-primary">Noise Suppression</span>
				<input type="checkbox" bind:checked={voicePrefs.noise_suppression} class="accent-brand-500" />
			</label>

			<label class="flex items-center justify-between">
				<span class="text-sm text-text-primary">Echo Cancellation</span>
				<input type="checkbox" bind:checked={voicePrefs.echo_cancellation} class="accent-brand-500" />
			</label>

			<label class="flex items-center justify-between">
				<span class="text-sm text-text-primary">Auto Gain Control</span>
				<input type="checkbox" bind:checked={voicePrefs.auto_gain_control} class="accent-brand-500" />
			</label>
		</div>
	</div>

	<div class="mb-6">
		<h2 class="mb-3 text-2xs font-medium uppercase tracking-wide text-text-muted">Audio Output</h2>
		<div class="flex flex-col gap-4 rounded-lg bg-bg-secondary p-4">
			<div class="flex flex-col gap-1">
				<label for="voice-output-device" class="text-sm font-medium text-text-secondary">Output Device</label>
				<select
					id="voice-output-device"
					class="rounded border border-bg-tertiary bg-bg-primary px-3 py-2 text-sm text-text-primary outline-none focus:border-brand-500"
					bind:value={outputDeviceId}
				>
					<option value="">Default</option>
					{#each availableOutputDevices as device}
						<option value={device.deviceId}>{device.label || `Speaker ${device.deviceId.slice(0, 8)}`}</option>
					{/each}
				</select>
			</div>

			<div class="flex flex-col gap-1">
				<label for="voice-output-volume" class="flex items-center justify-between text-sm font-medium text-text-secondary">
					Output Volume
					<span class="text-xs text-text-muted">{Math.round(voicePrefs.output_volume * 100)}%</span>
				</label>
				<input
					id="voice-output-volume"
					type="range"
					min="0"
					max="2"
					step="0.05"
					bind:value={voicePrefs.output_volume}
					class="w-full accent-brand-500"
				/>
			</div>
		</div>
	</div>

	<div class="mb-6">
		<h2 class="mb-3 text-2xs font-medium uppercase tracking-wide text-text-muted">Voice Activity</h2>
		<div class="flex flex-col gap-4 rounded-lg bg-bg-secondary p-4">
			<div class="flex flex-col gap-1">
				<label for="voice-input-mode" class="text-sm font-medium text-text-secondary">Input Mode</label>
				<select
					id="voice-input-mode"
					class="rounded border border-bg-tertiary bg-bg-primary px-3 py-2 text-sm text-text-primary outline-none focus:border-brand-500"
					bind:value={voicePrefs.input_mode}
				>
					<option value="vad">Voice Activity Detection</option>
					<option value="ptt">Push to Talk</option>
				</select>
			</div>

			{#if voicePrefs.input_mode === 'vad'}
				<div class="flex flex-col gap-1">
					<label for="voice-vad-threshold" class="flex items-center justify-between text-sm font-medium text-text-secondary">
						VAD Sensitivity
						<span class="text-xs text-text-muted">{Math.round(voicePrefs.vad_threshold * 100)}%</span>
					</label>
					<input
						id="voice-vad-threshold"
						type="range"
						min="0"
						max="1"
						step="0.05"
						bind:value={voicePrefs.vad_threshold}
						class="w-full accent-brand-500"
					/>
				</div>
			{:else}
				<div class="flex flex-col gap-1">
					<label for="voice-ptt-keybind" class="text-sm font-medium text-text-secondary">PTT Keybind</label>
					<button
						id="voice-ptt-keybind"
						class="rounded border px-4 py-2 text-sm font-mono {recordingVoicePTTKey ? 'border-brand-500 text-brand-400 animate-pulse' : 'border-bg-tertiary bg-bg-primary text-text-primary'}"
						onclick={() => recordingVoicePTTKey = !recordingVoicePTTKey}
					>
						{recordingVoicePTTKey ? 'Press a key...' : formatVoiceKeyName(voicePrefs.ptt_key)}
					</button>
				</div>
			{/if}
		</div>
	</div>

	<div class="mb-6">
		<h2 class="mb-3 text-2xs font-medium uppercase tracking-wide text-text-muted">Camera Defaults</h2>
		<div class="flex flex-col gap-4 rounded-lg bg-bg-secondary p-4">
			<div class="flex flex-col gap-1">
				<label for="camera-resolution" class="text-sm font-medium text-text-secondary">Resolution</label>
				<select
					id="camera-resolution"
					class="rounded border border-bg-tertiary bg-bg-primary px-3 py-2 text-sm text-text-primary outline-none focus:border-brand-500"
					bind:value={voicePrefs.camera_resolution}
				>
					<option value="360p">360p (Low bandwidth)</option>
					<option value="720p">720p (HD)</option>
					<option value="1080p">1080p (Full HD)</option>
				</select>
			</div>

			<div class="flex flex-col gap-1">
				<label for="camera-framerate" class="text-sm font-medium text-text-secondary">Frame Rate</label>
				<select
					id="camera-framerate"
					class="rounded border border-bg-tertiary bg-bg-primary px-3 py-2 text-sm text-text-primary outline-none focus:border-brand-500"
					bind:value={voicePrefs.camera_framerate}
				>
					<option value={15}>15 fps (Low bandwidth)</option>
					<option value={30}>30 fps (Standard)</option>
					<option value={60}>60 fps (Smooth)</option>
				</select>
			</div>
		</div>
	</div>

	<div class="mb-6">
		<h2 class="mb-3 text-2xs font-medium uppercase tracking-wide text-text-muted">Screen Share Defaults</h2>
		<div class="flex flex-col gap-4 rounded-lg bg-bg-secondary p-4">
			<div class="flex flex-col gap-1">
				<label for="screenshare-resolution" class="text-sm font-medium text-text-secondary">Resolution</label>
				<select
					id="screenshare-resolution"
					class="rounded border border-bg-tertiary bg-bg-primary px-3 py-2 text-sm text-text-primary outline-none focus:border-brand-500"
					bind:value={voicePrefs.screenshare_resolution}
				>
					<option value="720p">720p (HD)</option>
					<option value="1080p">1080p (Full HD)</option>
					<option value="4k">4K (Ultra HD)</option>
				</select>
			</div>

			<div class="flex flex-col gap-1">
				<label for="screenshare-framerate" class="text-sm font-medium text-text-secondary">Frame Rate</label>
				<select
					id="screenshare-framerate"
					class="rounded border border-bg-tertiary bg-bg-primary px-3 py-2 text-sm text-text-primary outline-none focus:border-brand-500"
					bind:value={voicePrefs.screenshare_framerate}
				>
					<option value={15}>15 fps (Low bandwidth)</option>
					<option value={30}>30 fps (Standard)</option>
					<option value={60}>60 fps (Smooth)</option>
				</select>
			</div>

			<label class="flex items-center justify-between">
				<span class="text-sm text-text-primary">Share System Audio</span>
				<input type="checkbox" bind:checked={voicePrefs.screenshare_audio} class="accent-brand-500" />
			</label>
		</div>
	</div>

	<button
		class="rounded bg-brand-500 px-6 py-2 text-sm font-semibold text-white hover:brightness-110 disabled:cursor-not-allowed disabled:opacity-60"
		onclick={saveVoicePreferences}
		disabled={voiceSaving}
	>
		{voiceSaving ? 'Saving...' : 'Save Changes'}
	</button>
{:else if voiceError}
	<div class="rounded-lg bg-red-500/10 px-4 py-2 text-sm text-red-400">{voiceError}</div>
{/if}
