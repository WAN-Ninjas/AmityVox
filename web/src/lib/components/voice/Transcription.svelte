<!-- Transcription.svelte — Voice channel transcription UI with opt-in toggle and live transcript display. -->
<script lang="ts">
	import { api } from '$lib/api/client';
	import { currentUser } from '$lib/stores/auth';

	interface TranscriptionEntry {
		id: string;
		channel_id: string;
		user_id: string;
		content: string;
		confidence?: number;
		language: string;
		duration_ms: number;
		started_at: string;
		ended_at: string;
		created_at: string;
		username: string;
		display_name?: string;
		avatar_id?: string;
	}

	interface TranscriptionSettings {
		channel_id: string;
		user_id: string;
		enabled: boolean;
		language: string;
	}

	interface Props {
		channelId: string;
	}

	let { channelId }: Props = $props();

	let settings = $state<TranscriptionSettings | null>(null);
	let transcriptions = $state<TranscriptionEntry[]>([]);
	let loading = $state(false);
	let saving = $state(false);
	let error = $state('');
	let autoScroll = $state(true);
	let transcriptContainer: HTMLDivElement;

	const supportedLanguages = [
		{ code: 'en', name: 'English' },
		{ code: 'es', name: 'Spanish' },
		{ code: 'fr', name: 'French' },
		{ code: 'de', name: 'German' },
		{ code: 'it', name: 'Italian' },
		{ code: 'pt', name: 'Portuguese' },
		{ code: 'ja', name: 'Japanese' },
		{ code: 'ko', name: 'Korean' },
		{ code: 'zh', name: 'Chinese' },
		{ code: 'ru', name: 'Russian' },
		{ code: 'ar', name: 'Arabic' },
		{ code: 'hi', name: 'Hindi' }
	];

	function formatTime(dateStr: string): string {
		return new Date(dateStr).toLocaleTimeString(undefined, {
			hour: '2-digit',
			minute: '2-digit',
			second: '2-digit'
		});
	}

	function formatDuration(ms: number): string {
		const seconds = Math.floor(ms / 1000);
		if (seconds < 60) return `${seconds}s`;
		const minutes = Math.floor(seconds / 60);
		const remaining = seconds % 60;
		return `${minutes}m ${remaining}s`;
	}

	function confidenceColor(confidence: number | undefined): string {
		if (confidence === undefined) return 'text-text-muted';
		if (confidence >= 0.9) return 'text-green-400';
		if (confidence >= 0.7) return 'text-yellow-400';
		return 'text-red-400';
	}

	function confidenceLabel(confidence: number | undefined): string {
		if (confidence === undefined) return '';
		return `${Math.round(confidence * 100)}%`;
	}

	async function loadSettings() {
		try {
			const data = await api.request<TranscriptionSettings>(
				'GET',
				`/channels/${channelId}/experimental/transcription/settings`
			);
			if (data) {
				settings = data;
			}
		} catch {
			settings = { channel_id: channelId, user_id: $currentUser?.id ?? '', enabled: false, language: 'en' };
		}
	}

	async function updateSettings(enabled: boolean, language?: string) {
		saving = true;
		error = '';
		try {
			const data = await api.request<TranscriptionSettings>(
				'PATCH',
				`/channels/${channelId}/experimental/transcription/settings`,
				{
					enabled,
					language: language ?? settings?.language ?? 'en'
				}
			);
			if (data) {
				settings = data;
			}
		} catch (err: any) {
			error = err.message || 'Failed to update settings';
		} finally {
			saving = false;
		}
	}

	async function loadTranscriptions() {
		loading = true;
		try {
			const data = await api.request<TranscriptionEntry[]>(
				'GET',
				`/channels/${channelId}/experimental/transcriptions`
			);
			transcriptions = (data ?? []).reverse(); // chronological order
		} catch {
			// Ignore load errors.
		} finally {
			loading = false;
		}
	}

	function scrollToBottom() {
		if (transcriptContainer && autoScroll) {
			transcriptContainer.scrollTop = transcriptContainer.scrollHeight;
		}
	}

	async function copyTranscript() {
		const text = transcriptions
			.map((t) => `[${formatTime(t.started_at)}] ${t.display_name ?? t.username}: ${t.content}`)
			.join('\n');
		try {
			await navigator.clipboard.writeText(text);
		} catch {
			// Fallback.
		}
	}

	$effect(() => {
		if (channelId) {
			loadSettings();
			loadTranscriptions();
		}
	});

	$effect(() => {
		if (transcriptions.length > 0) {
			scrollToBottom();
		}
	});

	// Poll for new transcriptions when enabled.
	$effect(() => {
		if (settings?.enabled) {
			const interval = setInterval(loadTranscriptions, 5000);
			return () => clearInterval(interval);
		}
	});
</script>

<div class="flex flex-col h-full bg-bg-primary">
	<!-- Header -->
	<div class="flex items-center justify-between px-4 py-3 border-b border-border-primary">
		<div class="flex items-center gap-2">
			<svg class="w-5 h-5 text-brand-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
				<path stroke-linecap="round" stroke-linejoin="round" d="M19 11a7 7 0 01-7 7m0 0a7 7 0 01-7-7m7 7v4m0 0H8m4 0h4m-4-8a3 3 0 01-3-3V5a3 3 0 116 0v6a3 3 0 01-3 3z" />
			</svg>
			<h3 class="text-text-primary font-medium text-sm">Voice Transcription</h3>
			{#if settings?.enabled}
				<span class="inline-flex items-center gap-1 px-1.5 py-0.5 bg-green-500/20 text-green-400 text-xs rounded-full">
					<span class="w-1.5 h-1.5 bg-green-400 rounded-full animate-pulse"></span>
					Active
				</span>
			{/if}
		</div>

		<div class="flex items-center gap-2">
			{#if transcriptions.length > 0}
				<button
					type="button"
					class="text-text-muted hover:text-text-primary text-xs px-2 py-1 rounded hover:bg-bg-tertiary"
					onclick={copyTranscript}
					title="Copy transcript"
				>
					Copy
				</button>
			{/if}
		</div>
	</div>

	<!-- Settings panel -->
	<div class="px-4 py-3 border-b border-border-primary bg-bg-secondary">
		<div class="flex items-center justify-between">
			<div class="flex items-center gap-3">
				<label class="flex items-center gap-2 cursor-pointer">
					<button
						type="button"
						class="relative w-10 h-5 rounded-full transition-colors {settings?.enabled ? 'bg-brand-500' : 'bg-bg-tertiary'}"
						role="switch"
						aria-checked={settings?.enabled}
						disabled={saving}
						onclick={() => updateSettings(!settings?.enabled)}
					>
						<span
							class="absolute top-0.5 left-0.5 w-4 h-4 rounded-full bg-white transition-transform shadow-sm
								{settings?.enabled ? 'translate-x-5' : ''}"
						></span>
					</button>
					<span class="text-text-secondary text-sm">
						{settings?.enabled ? 'Transcription on' : 'Transcription off'}
					</span>
				</label>
			</div>

			<select
				class="bg-bg-primary border border-border-primary rounded px-2 py-1 text-xs text-text-primary focus:border-brand-500 focus:outline-none"
				value={settings?.language ?? 'en'}
				onchange={(e) => updateSettings(settings?.enabled ?? false, (e.target as HTMLSelectElement).value)}
				disabled={saving}
			>
				{#each supportedLanguages as lang}
					<option value={lang.code}>{lang.name}</option>
				{/each}
			</select>
		</div>

		{#if error}
			<div class="mt-2 text-red-400 text-xs">{error}</div>
		{/if}

		<p class="text-text-muted text-xs mt-2">
			Speech-to-text transcription is opt-in per user. Your voice will only be transcribed when you enable this feature. Other participants in the voice channel will see a transcription indicator.
		</p>
	</div>

	<!-- Transcript -->
	<div class="flex-1 overflow-y-auto px-4 py-2" bind:this={transcriptContainer}>
		{#if loading && transcriptions.length === 0}
			<div class="text-text-muted text-sm py-4 text-center">Loading transcriptions...</div>
		{:else if transcriptions.length === 0}
			<div class="text-center py-8">
				<svg class="w-12 h-12 mx-auto text-text-muted/20 mb-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1">
					<path stroke-linecap="round" stroke-linejoin="round" d="M19 11a7 7 0 01-7 7m0 0a7 7 0 01-7-7m7 7v4m0 0H8m4 0h4m-4-8a3 3 0 01-3-3V5a3 3 0 116 0v6a3 3 0 01-3 3z" />
				</svg>
				<p class="text-text-muted text-sm">No transcriptions yet.</p>
				<p class="text-text-muted text-xs mt-1">
					{settings?.enabled ? 'Waiting for speech...' : 'Enable transcription to start.'}
				</p>
			</div>
		{:else}
			<div class="space-y-1">
				{#each transcriptions as entry (entry.id)}
					<div class="group flex gap-2 py-1 hover:bg-bg-secondary/50 rounded px-1 -mx-1">
						<span class="text-text-muted text-xs font-mono shrink-0 mt-0.5 w-16 text-right">
							{formatTime(entry.started_at)}
						</span>
						<div class="min-w-0 flex-1">
							<div class="flex items-baseline gap-1.5">
								<span class="text-text-primary text-sm font-medium shrink-0">
									{entry.display_name ?? entry.username}
								</span>
								<span class="text-text-secondary text-sm">{entry.content}</span>
							</div>
							<div class="flex items-center gap-2 mt-0.5 opacity-0 group-hover:opacity-100 transition-opacity">
								{#if entry.confidence !== undefined}
									<span class="text-[10px] {confidenceColor(entry.confidence)}">
										{confidenceLabel(entry.confidence)}
									</span>
								{/if}
								<span class="text-[10px] text-text-muted">{formatDuration(entry.duration_ms)}</span>
								<span class="text-[10px] text-text-muted uppercase">{entry.language}</span>
							</div>
						</div>
					</div>
				{/each}
			</div>
		{/if}
	</div>

	<!-- Bottom bar -->
	{#if transcriptions.length > 0}
		<div class="flex items-center justify-between px-4 py-2 border-t border-border-primary bg-bg-secondary text-xs text-text-muted">
			<span>{transcriptions.length} entries</span>
			<label class="flex items-center gap-1.5 cursor-pointer">
				<input type="checkbox" bind:checked={autoScroll} class="rounded text-brand-500" />
				Auto-scroll
			</label>
		</div>
	{/if}
</div>
