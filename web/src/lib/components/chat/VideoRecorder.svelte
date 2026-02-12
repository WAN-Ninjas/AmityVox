<!-- VideoRecorder.svelte — In-app video and screen recording (Discord Clips equivalent). -->
<script lang="ts">
	import { api } from '$lib/api/client';
	import { addToast } from '$lib/stores/toast';

	interface Props {
		channelId: string;
		onclose?: () => void;
	}

	let { channelId, onclose }: Props = $props();

	let recording = $state(false);
	let paused = $state(false);
	let recordingTime = $state(0);
	let mediaRecorder = $state<MediaRecorder | null>(null);
	let stream = $state<MediaStream | null>(null);
	let chunks = $state<Blob[]>([]);
	let previewUrl = $state<string | null>(null);
	let error = $state('');
	let uploading = $state(false);
	let mode = $state<'camera' | 'screen'>('screen');
	let timerInterval = $state<ReturnType<typeof setInterval> | null>(null);
	let videoPreview: HTMLVideoElement;

	// Recording limits.
	const MAX_DURATION_SECONDS = 300; // 5 minutes.
	const SUPPORTED_MIME_TYPES = ['video/webm;codecs=vp9,opus', 'video/webm;codecs=vp8,opus', 'video/webm', 'video/mp4'];

	const formattedTime = $derived(() => {
		const mins = Math.floor(recordingTime / 60);
		const secs = recordingTime % 60;
		return `${mins.toString().padStart(2, '0')}:${secs.toString().padStart(2, '0')}`;
	});

	const progressPercent = $derived(Math.min((recordingTime / MAX_DURATION_SECONDS) * 100, 100));

	function getSupportedMimeType(): string {
		for (const type of SUPPORTED_MIME_TYPES) {
			if (MediaRecorder.isTypeSupported(type)) return type;
		}
		return 'video/webm';
	}

	async function startRecording() {
		error = '';
		chunks = [];
		previewUrl = null;
		recordingTime = 0;

		try {
			if (mode === 'screen') {
				stream = await navigator.mediaDevices.getDisplayMedia({
					video: { width: { ideal: 1920 }, height: { ideal: 1080 }, frameRate: { ideal: 30 } },
					audio: true
				});
			} else {
				stream = await navigator.mediaDevices.getUserMedia({
					video: { width: { ideal: 1280 }, height: { ideal: 720 }, facingMode: 'user' },
					audio: true
				});
			}

			// Show live preview.
			if (videoPreview) {
				videoPreview.srcObject = stream;
				videoPreview.play();
			}

			const mimeType = getSupportedMimeType();
			const recorder = new MediaRecorder(stream, {
				mimeType,
				videoBitsPerSecond: 2500000 // 2.5 Mbps
			});

			recorder.ondataavailable = (e) => {
				if (e.data.size > 0) {
					chunks = [...chunks, e.data];
				}
			};

			recorder.onstop = () => {
				const blob = new Blob(chunks, { type: mimeType });
				previewUrl = URL.createObjectURL(blob);
				stopTimer();
				stopStream();
			};

			// Auto-stop when screen sharing ends.
			stream.getVideoTracks()[0].addEventListener('ended', () => {
				if (recording) stopRecording();
			});

			recorder.start(1000); // Collect data every second.
			mediaRecorder = recorder;
			recording = true;
			paused = false;
			startTimer();
		} catch (err: any) {
			if (err.name === 'NotAllowedError') {
				error = 'Permission denied. Please allow screen/camera access.';
			} else {
				error = err.message || 'Failed to start recording';
			}
		}
	}

	function stopRecording() {
		if (mediaRecorder && mediaRecorder.state !== 'inactive') {
			mediaRecorder.stop();
		}
		recording = false;
		paused = false;
	}

	function togglePause() {
		if (!mediaRecorder) return;
		if (paused) {
			mediaRecorder.resume();
			startTimer();
		} else {
			mediaRecorder.pause();
			stopTimer();
		}
		paused = !paused;
	}

	function startTimer() {
		if (timerInterval) clearInterval(timerInterval);
		timerInterval = setInterval(() => {
			recordingTime++;
			if (recordingTime >= MAX_DURATION_SECONDS) {
				stopRecording();
			}
		}, 1000);
	}

	function stopTimer() {
		if (timerInterval) {
			clearInterval(timerInterval);
			timerInterval = null;
		}
	}

	function stopStream() {
		if (stream) {
			stream.getTracks().forEach((track) => track.stop());
			stream = null;
		}
		if (videoPreview) {
			videoPreview.srcObject = null;
		}
	}

	function discardRecording() {
		if (previewUrl) {
			URL.revokeObjectURL(previewUrl);
			previewUrl = null;
		}
		chunks = [];
		recordingTime = 0;
	}

	async function saveRecording() {
		if (chunks.length === 0) return;
		uploading = true;
		error = '';

		try {
			const blob = new Blob(chunks, { type: getSupportedMimeType() });

			// Upload via the file upload endpoint.
			const formData = new FormData();
			formData.append('file', blob, `recording_${Date.now()}.webm`);

			const uploadResponse = await fetch('/api/v1/files/upload', {
				method: 'POST',
				headers: {
					Authorization: `Bearer ${api.getToken()}`
				},
				body: formData
			});

			if (!uploadResponse.ok) {
				throw new Error('Upload failed');
			}

			const uploadResult = await uploadResponse.json();
			const fileData = uploadResult.data;

			// Register the recording.
			await api.request('POST', `/channels/${channelId}/experimental/recordings`, {
				title: `Recording ${new Date().toLocaleString()}`,
				s3_key: fileData.s3_key,
				s3_bucket: fileData.s3_bucket || 'amityvox',
				duration_ms: recordingTime * 1000,
				file_size_bytes: blob.size,
				width: mode === 'screen' ? 1920 : 1280,
				height: mode === 'screen' ? 1080 : 720
			});

			addToast('Recording saved successfully!', 'success');
			discardRecording();
			if (onclose) onclose();
		} catch (err: any) {
			error = err.message || 'Failed to save recording';
		} finally {
			uploading = false;
		}
	}

	function formatSize(bytes: number): string {
		if (bytes < 1024) return `${bytes} B`;
		if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
		return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
	}

	$effect(() => {
		return () => {
			stopTimer();
			stopStream();
			if (previewUrl) URL.revokeObjectURL(previewUrl);
		};
	});
</script>

<div class="bg-bg-secondary border border-border-primary rounded-lg overflow-hidden max-w-lg">
	<!-- Header -->
	<div class="flex items-center justify-between px-4 py-2 bg-bg-tertiary border-b border-border-primary">
		<div class="flex items-center gap-2">
			<svg class="w-5 h-5 text-red-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
				<path stroke-linecap="round" stroke-linejoin="round" d="M15 10l4.553-2.276A1 1 0 0121 8.618v6.764a1 1 0 01-1.447.894L15 14M5 18h8a2 2 0 002-2V8a2 2 0 00-2-2H5a2 2 0 00-2 2v8a2 2 0 002 2z" />
			</svg>
			<span class="text-text-primary text-sm font-medium">Video Recorder</span>
			{#if recording}
				<span class="inline-flex items-center gap-1 px-1.5 py-0.5 bg-red-500/20 text-red-400 text-xs rounded-full">
					<span class="w-1.5 h-1.5 bg-red-500 rounded-full {paused ? '' : 'animate-pulse'}"></span>
					{paused ? 'Paused' : 'Recording'}
				</span>
			{/if}
		</div>
		{#if onclose}
			<button type="button" class="text-text-muted hover:text-text-primary" onclick={onclose}>
				<svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
					<path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
				</svg>
			</button>
		{/if}
	</div>

	<div class="p-4">
		{#if error}
			<div class="mb-3 p-2 bg-red-500/10 border border-red-500/20 rounded text-red-400 text-sm">{error}</div>
		{/if}

		{#if !recording && !previewUrl}
			<!-- Mode selection -->
			<div class="flex gap-2 mb-4">
				<button
					type="button"
					class="flex-1 flex flex-col items-center gap-2 p-4 rounded-lg border transition-colors
						{mode === 'screen' ? 'border-brand-500 bg-brand-500/10' : 'border-border-primary hover:border-brand-500/30'}"
					onclick={() => (mode = 'screen')}
				>
					<svg class="w-8 h-8 {mode === 'screen' ? 'text-brand-400' : 'text-text-muted'}" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
						<path stroke-linecap="round" stroke-linejoin="round" d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
					</svg>
					<span class="text-sm {mode === 'screen' ? 'text-brand-400' : 'text-text-secondary'}">Screen</span>
				</button>
				<button
					type="button"
					class="flex-1 flex flex-col items-center gap-2 p-4 rounded-lg border transition-colors
						{mode === 'camera' ? 'border-brand-500 bg-brand-500/10' : 'border-border-primary hover:border-brand-500/30'}"
					onclick={() => (mode = 'camera')}
				>
					<svg class="w-8 h-8 {mode === 'camera' ? 'text-brand-400' : 'text-text-muted'}" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
						<path stroke-linecap="round" stroke-linejoin="round" d="M15 10l4.553-2.276A1 1 0 0121 8.618v6.764a1 1 0 01-1.447.894L15 14M5 18h8a2 2 0 002-2V8a2 2 0 00-2-2H5a2 2 0 00-2 2v8a2 2 0 002 2z" />
					</svg>
					<span class="text-sm {mode === 'camera' ? 'text-brand-400' : 'text-text-secondary'}">Camera</span>
				</button>
			</div>

			<button
				type="button"
				class="w-full btn-primary py-2 rounded text-sm flex items-center justify-center gap-2"
				onclick={startRecording}
			>
				<span class="w-3 h-3 bg-red-500 rounded-full"></span>
				Start Recording
			</button>

			<p class="text-text-muted text-xs mt-2 text-center">
				Max duration: {Math.floor(MAX_DURATION_SECONDS / 60)} minutes
			</p>
		{:else if recording}
			<!-- Recording view -->
			<div class="relative rounded-lg overflow-hidden bg-black mb-3" style="aspect-ratio: 16/9;">
				<video
					bind:this={videoPreview}
					class="w-full h-full object-contain"
					muted
					playsinline
				></video>

				<!-- Recording overlay -->
				<div class="absolute bottom-2 left-2 right-2 flex items-center justify-between">
					<span class="text-white text-sm font-mono bg-black/60 px-2 py-0.5 rounded">
						{formattedTime()}
					</span>
					<div class="flex items-center gap-2">
						<button
							type="button"
							class="w-8 h-8 rounded-full bg-white/20 backdrop-blur-sm flex items-center justify-center text-white hover:bg-white/30"
							onclick={togglePause}
							title={paused ? 'Resume' : 'Pause'}
						>
							{#if paused}
								<svg class="w-4 h-4" fill="currentColor" viewBox="0 0 24 24"><path d="M8 5v14l11-7z" /></svg>
							{:else}
								<svg class="w-4 h-4" fill="currentColor" viewBox="0 0 24 24"><path d="M6 19h4V5H6v14zm8-14v14h4V5h-4z" /></svg>
							{/if}
						</button>
						<button
							type="button"
							class="w-8 h-8 rounded-full bg-red-500 flex items-center justify-center text-white hover:bg-red-600"
							onclick={stopRecording}
							title="Stop"
						>
							<svg class="w-4 h-4" fill="currentColor" viewBox="0 0 24 24"><rect x="6" y="6" width="12" height="12" rx="1" /></svg>
						</button>
					</div>
				</div>

				<!-- Progress bar -->
				<div class="absolute bottom-0 left-0 right-0 h-1 bg-black/30">
					<div class="h-full bg-red-500 transition-all duration-1000" style="width: {progressPercent}%;"></div>
				</div>
			</div>
		{:else if previewUrl}
			<!-- Preview recorded video -->
			<div class="rounded-lg overflow-hidden bg-black mb-3" style="aspect-ratio: 16/9;">
				<video
					src={previewUrl}
					class="w-full h-full object-contain"
					controls
					playsinline
				></video>
			</div>

			<div class="flex items-center justify-between mb-3">
				<div class="text-text-muted text-xs">
					<span>Duration: {formattedTime()}</span>
					<span class="mx-2">|</span>
					<span>Size: {formatSize(chunks.reduce((acc, c) => acc + c.size, 0))}</span>
				</div>
			</div>

			<div class="flex gap-2">
				<button
					type="button"
					class="flex-1 btn-secondary py-2 rounded text-sm"
					onclick={discardRecording}
					disabled={uploading}
				>
					Discard
				</button>
				<button
					type="button"
					class="flex-1 btn-primary py-2 rounded text-sm flex items-center justify-center gap-2"
					onclick={saveRecording}
					disabled={uploading}
				>
					{#if uploading}
						<svg class="w-4 h-4 animate-spin" fill="none" viewBox="0 0 24 24">
							<circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" />
							<path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
						</svg>
						Uploading...
					{:else}
						Save & Share
					{/if}
				</button>
			</div>
		{/if}
	</div>
</div>
