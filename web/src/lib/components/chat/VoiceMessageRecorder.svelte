<script lang="ts">
	interface Props {
		onsend: (audioBlob: Blob, waveform: number[], durationMs: number) => void;
		oncancel: () => void;
	}
	let { onsend, oncancel }: Props = $props();

	let recording = $state(false);
	let elapsed = $state(0);
	let waveformData = $state<number[]>([]);
	let mediaRecorder: MediaRecorder | null = null;
	let audioChunks: Blob[] = [];
	let analyser: AnalyserNode | null = null;
	let audioCtx: AudioContext | null = null;
	let animFrame: number | null = null;
	let timer: ReturnType<typeof setInterval> | null = null;
	let startTime = 0;

	const MAX_DURATION_MS = 5 * 60 * 1000; // 5 minutes

	async function startRecording() {
		try {
			const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
			audioCtx = new AudioContext();
			const source = audioCtx.createMediaStreamSource(stream);
			analyser = audioCtx.createAnalyser();
			analyser.fftSize = 256;
			source.connect(analyser);

			mediaRecorder = new MediaRecorder(stream, { mimeType: getSupportedMimeType() });
			audioChunks = [];
			waveformData = [];

			mediaRecorder.ondataavailable = (e) => {
				if (e.data.size > 0) audioChunks.push(e.data);
			};

			mediaRecorder.start(100); // collect data every 100ms
			recording = true;
			startTime = Date.now();
			elapsed = 0;

			timer = setInterval(() => {
				elapsed = Date.now() - startTime;
				if (elapsed >= MAX_DURATION_MS) stopAndSend();
			}, 100);

			captureWaveform();
		} catch {
			// Microphone permission denied or not available
		}
	}

	function getSupportedMimeType(): string {
		const types = ['audio/webm;codecs=opus', 'audio/webm', 'audio/ogg;codecs=opus', 'audio/mp4'];
		for (const type of types) {
			if (MediaRecorder.isTypeSupported(type)) return type;
		}
		return 'audio/webm';
	}

	function captureWaveform() {
		if (!analyser || !recording) return;
		const data = new Uint8Array(analyser.frequencyBinCount);
		analyser.getByteTimeDomainData(data);
		// Get peak amplitude for this frame
		let peak = 0;
		for (let i = 0; i < data.length; i++) {
			const val = Math.abs(data[i] - 128);
			if (val > peak) peak = val;
		}
		waveformData = [...waveformData, Math.round((peak / 128) * 255)];
		animFrame = requestAnimationFrame(captureWaveform);
	}

	function stopAndSend() {
		if (!mediaRecorder || mediaRecorder.state === 'inactive') return;
		if (timer) clearInterval(timer);
		if (animFrame) cancelAnimationFrame(animFrame);

		const duration = Date.now() - startTime;
		const recorder = mediaRecorder;

		recorder.onstop = () => {
			recorder.stream.getTracks().forEach(t => t.stop());
			audioCtx?.close();
			const blob = new Blob(audioChunks, { type: recorder.mimeType });
			// Downsample waveform to ~100 bars
			const downsampled = downsampleWaveform(waveformData, 100);
			onsend(blob, downsampled, duration);
		};
		recorder.stop();
		recording = false;
	}

	function cancel() {
		if (mediaRecorder && mediaRecorder.state !== 'inactive') {
			mediaRecorder.stream.getTracks().forEach(t => t.stop());
			mediaRecorder.stop();
		}
		if (timer) clearInterval(timer);
		if (animFrame) cancelAnimationFrame(animFrame);
		audioCtx?.close();
		recording = false;
		oncancel();
	}

	function downsampleWaveform(data: number[], targetLen: number): number[] {
		if (data.length <= targetLen) return data;
		const result: number[] = [];
		const step = data.length / targetLen;
		for (let i = 0; i < targetLen; i++) {
			const start = Math.floor(i * step);
			const end = Math.floor((i + 1) * step);
			let max = 0;
			for (let j = start; j < end; j++) {
				if (data[j] > max) max = data[j];
			}
			result.push(max);
		}
		return result;
	}

	function formatTime(ms: number): string {
		const s = Math.floor(ms / 1000);
		const m = Math.floor(s / 60);
		return `${m}:${String(s % 60).padStart(2, '0')}`;
	}
</script>

<div class="flex items-center gap-3 rounded-lg bg-bg-modifier px-4 py-2">
	{#if !recording}
		<button
			class="rounded-full bg-red-500 p-2 text-white hover:bg-red-600"
			onclick={startRecording}
			title="Start recording"
		>
			<svg class="h-5 w-5" fill="currentColor" viewBox="0 0 24 24">
				<path d="M12 14c1.66 0 3-1.34 3-3V5c0-1.66-1.34-3-3-3S9 3.34 9 5v6c0 1.66 1.34 3 3 3z" />
				<path d="M17 11c0 2.76-2.24 5-5 5s-5-2.24-5-5H5c0 3.53 2.61 6.43 6 6.92V21h2v-3.08c3.39-.49 6-3.39 6-6.92h-2z" />
			</svg>
		</button>
		<span class="text-sm text-text-muted">Click to record a voice message</span>
		<button
			class="ml-auto text-text-muted hover:text-text-primary"
			onclick={cancel}
			title="Cancel"
		>
			<svg class="h-5 w-5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
				<path d="M6 18L18 6M6 6l12 12" />
			</svg>
		</button>
	{:else}
		<!-- Recording indicator -->
		<div class="flex h-8 w-8 items-center justify-center">
			<div class="h-3 w-3 animate-pulse rounded-full bg-red-500"></div>
		</div>

		<!-- Live waveform -->
		<div class="flex h-8 flex-1 items-center gap-px overflow-hidden">
			{#each waveformData.slice(-60) as bar}
				<div
					class="w-1 shrink-0 rounded-full bg-red-400"
					style="height: {Math.max(2, (bar / 255) * 32)}px"
				></div>
			{/each}
		</div>

		<!-- Timer -->
		<span class="text-sm font-mono text-text-primary">{formatTime(elapsed)}</span>

		<!-- Cancel -->
		<button
			class="rounded p-1.5 text-text-muted hover:text-text-primary"
			onclick={cancel}
			title="Cancel"
		>
			<svg class="h-5 w-5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
				<path d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
			</svg>
		</button>

		<!-- Send -->
		<button
			class="rounded-full bg-brand-500 p-2 text-white hover:bg-brand-600"
			onclick={stopAndSend}
			title="Send voice message"
		>
			<svg class="h-5 w-5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
				<path d="M5 12h14M12 5l7 7-7 7" />
			</svg>
		</button>
	{/if}
</div>
