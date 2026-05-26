<script lang="ts">
	import VideoRecorder from '$components/chat/VideoRecorder.svelte';
	import { api, type VideoRecording } from '$lib/api/client';
	import { createAsyncOp } from '$lib/utils/asyncOp';
	import { getErrorMessage } from '$lib/utils/apiError';

	interface Props {
		channelId: string;
	}

	let { channelId }: Props = $props();

	let recordings = $state<VideoRecording[]>([]);
	let showRecorder = $state(false);
	let error = $state('');
	let loadOp = $state(createAsyncOp());
	let loadedChannelId = $state('');

	function formatDuration(ms: number): string {
		const totalSeconds = Math.round(ms / 1000);
		const minutes = Math.floor(totalSeconds / 60);
		const seconds = totalSeconds % 60;
		return `${minutes}:${seconds.toString().padStart(2, '0')}`;
	}

	function formatSize(bytes: number): string {
		if (bytes < 1024) return `${bytes} B`;
		if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
		return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
	}

	function fileUrl(recording: VideoRecording): string | null {
		return recording.attachment_id ? `/api/v1/files/${recording.attachment_id}` : null;
	}

	async function loadRecordings() {
		error = '';
		const result = await loadOp.run(
			() => api.getVideoRecordings(channelId),
			(message) => {
				error = message;
			}
		);
		if (result) recordings = result;
	}

	function recordingAuthor(recording: VideoRecording): string {
		return recording.display_name || recording.username;
	}

	$effect(() => {
		if (channelId && channelId !== loadedChannelId) {
			loadedChannelId = channelId;
			loadRecordings().catch((err: unknown) => {
				error = getErrorMessage(err, 'Failed to load recordings');
			});
		}
	});
</script>

<div class="space-y-3">
	<div class="flex items-center justify-between gap-3">
		<div>
			<h3 class="text-sm font-semibold text-text-primary">Recordings</h3>
			<p class="text-xs text-text-muted">Capture a screen or camera clip, then replay saved voice-channel recordings.</p>
		</div>
		<button class="btn-primary text-xs" type="button" onclick={() => (showRecorder = !showRecorder)}>
			{showRecorder ? 'Hide Recorder' : 'New Recording'}
		</button>
	</div>

	{#if showRecorder}
		<VideoRecorder
			{channelId}
			onclose={() => {
				showRecorder = false;
				loadRecordings();
			}}
		/>
	{/if}

	{#if error}
		<div class="rounded border border-red-500/20 bg-red-500/10 p-2 text-sm text-red-400">{error}</div>
	{/if}

	{#if loadOp.loading && recordings.length === 0}
		<div class="rounded bg-bg-secondary p-3 text-sm text-text-muted">Loading recordings...</div>
	{:else if recordings.length === 0}
		<div class="rounded bg-bg-secondary p-3 text-sm text-text-muted">No saved recordings yet.</div>
	{:else}
		<div class="space-y-2">
			{#each recordings as recording (recording.id)}
				<div class="rounded border border-border-primary bg-bg-secondary p-3">
					<div class="mb-2 flex items-start justify-between gap-3">
						<div class="min-w-0">
							<p class="truncate text-sm font-medium text-text-primary">{recording.title || 'Untitled recording'}</p>
							<p class="text-xs text-text-muted">
								{recordingAuthor(recording)} · {formatDuration(recording.duration_ms)} · {formatSize(recording.file_size_bytes)}
							</p>
						</div>
						<span class="rounded bg-bg-modifier px-2 py-0.5 text-2xs uppercase text-text-muted">{recording.status}</span>
					</div>

					{#if fileUrl(recording)}
						<!-- svelte-ignore a11y_media_has_caption -->
						<video class="w-full rounded bg-black" controls preload="metadata" src={fileUrl(recording) ?? undefined}></video>
					{:else}
						<div class="rounded bg-bg-primary p-3 text-xs text-text-muted">Recording media is unavailable.</div>
					{/if}
				</div>
			{/each}
		</div>
	{/if}
</div>
