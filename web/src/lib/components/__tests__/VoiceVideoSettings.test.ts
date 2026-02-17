import { describe, it, expect } from 'vitest';

// Test voice/video settings as pure data definitions and validation logic.
// Component rendering is not possible with happy-dom SSR (see MarkdownRenderer.test.ts).

// --- Camera resolution options (match CameraSettings.svelte + settings page) ---
const cameraResolutions = [
	{ value: '360p', label: '360p (Low bandwidth)', width: 640, height: 360 },
	{ value: '720p', label: '720p (HD)', width: 1280, height: 720 },
	{ value: '1080p', label: '1080p (Full HD)', width: 1920, height: 1080 }
] as const;

// --- Screenshare resolution options ---
const screenshareResolutions = [
	{ value: '720p', label: '720p (HD)', width: 1280, height: 720 },
	{ value: '1080p', label: '1080p (Full HD)', width: 1920, height: 1080 },
	{ value: '4k', label: '4K (Ultra HD)', width: 3840, height: 2160 }
] as const;

// --- Frame rate options ---
const framerates = [15, 30, 60] as const;

// --- Default voice preferences (match backend defaults) ---
const defaultVoicePrefs = {
	input_mode: 'vad' as const,
	ptt_key: 'Space',
	vad_threshold: 0.3,
	noise_suppression: true,
	echo_cancellation: true,
	auto_gain_control: true,
	input_volume: 1.0,
	output_volume: 1.0,
	camera_resolution: '720p' as const,
	camera_framerate: 30 as const,
	screenshare_resolution: '1080p' as const,
	screenshare_framerate: 30 as const,
	screenshare_audio: false
};

// --- Validation helpers (mirror what the backend enforces) ---
function isValidCameraResolution(res: string): boolean {
	return ['360p', '720p', '1080p'].includes(res);
}

function isValidScreenshareResolution(res: string): boolean {
	return ['720p', '1080p', '4k'].includes(res);
}

function isValidFramerate(fps: number): boolean {
	return [15, 30, 60].includes(fps);
}

function isValidVolume(vol: number): boolean {
	return vol >= 0.0 && vol <= 2.0;
}

function isValidVadThreshold(t: number): boolean {
	return t >= 0.0 && t <= 1.0;
}

// --- Device localStorage persistence helpers ---
const DEVICE_STORAGE_KEYS = {
	input: 'av-voice-input-device',
	output: 'av-voice-output-device'
} as const;

function saveDeviceId(type: 'input' | 'output', deviceId: string): void {
	localStorage.setItem(DEVICE_STORAGE_KEYS[type], deviceId);
}

function loadDeviceId(type: 'input' | 'output'): string {
	return localStorage.getItem(DEVICE_STORAGE_KEYS[type]) ?? '';
}

describe('VoiceVideoSettings', () => {
	describe('camera resolution options', () => {
		it('should have exactly 3 camera resolution options', () => {
			expect(cameraResolutions).toHaveLength(3);
		});

		it('should include 360p, 720p, 1080p', () => {
			const values = cameraResolutions.map((r) => r.value);
			expect(values).toEqual(['360p', '720p', '1080p']);
		});

		it('should have correct pixel dimensions', () => {
			const r720 = cameraResolutions.find((r) => r.value === '720p');
			expect(r720?.width).toBe(1280);
			expect(r720?.height).toBe(720);

			const r1080 = cameraResolutions.find((r) => r.value === '1080p');
			expect(r1080?.width).toBe(1920);
			expect(r1080?.height).toBe(1080);
		});

		it('should all have labels', () => {
			for (const res of cameraResolutions) {
				expect(res.label).toBeTruthy();
			}
		});
	});

	describe('screenshare resolution options', () => {
		it('should have exactly 3 screenshare resolution options', () => {
			expect(screenshareResolutions).toHaveLength(3);
		});

		it('should include 720p, 1080p, 4k', () => {
			const values = screenshareResolutions.map((r) => r.value);
			expect(values).toEqual(['720p', '1080p', '4k']);
		});

		it('should have correct 4k dimensions', () => {
			const r4k = screenshareResolutions.find((r) => r.value === '4k');
			expect(r4k?.width).toBe(3840);
			expect(r4k?.height).toBe(2160);
		});
	});

	describe('framerate options', () => {
		it('should have exactly 3 framerate options', () => {
			expect(framerates).toHaveLength(3);
		});

		it('should include 15, 30, 60', () => {
			expect([...framerates]).toEqual([15, 30, 60]);
		});

		it('should validate correct framerates', () => {
			expect(isValidFramerate(15)).toBe(true);
			expect(isValidFramerate(30)).toBe(true);
			expect(isValidFramerate(60)).toBe(true);
		});

		it('should reject invalid framerates', () => {
			expect(isValidFramerate(0)).toBe(false);
			expect(isValidFramerate(24)).toBe(false);
			expect(isValidFramerate(120)).toBe(false);
		});
	});

	describe('volume slider range', () => {
		it('should accept volume at boundaries', () => {
			expect(isValidVolume(0.0)).toBe(true);
			expect(isValidVolume(1.0)).toBe(true);
			expect(isValidVolume(2.0)).toBe(true);
		});

		it('should accept volume within range', () => {
			expect(isValidVolume(0.5)).toBe(true);
			expect(isValidVolume(1.5)).toBe(true);
		});

		it('should reject volume out of range', () => {
			expect(isValidVolume(-0.1)).toBe(false);
			expect(isValidVolume(2.1)).toBe(false);
			expect(isValidVolume(3.0)).toBe(false);
		});
	});

	describe('VAD threshold range', () => {
		it('should accept threshold at boundaries', () => {
			expect(isValidVadThreshold(0.0)).toBe(true);
			expect(isValidVadThreshold(1.0)).toBe(true);
		});

		it('should reject threshold out of range', () => {
			expect(isValidVadThreshold(-0.1)).toBe(false);
			expect(isValidVadThreshold(1.1)).toBe(false);
		});
	});

	describe('resolution validation', () => {
		it('should validate camera resolutions', () => {
			expect(isValidCameraResolution('360p')).toBe(true);
			expect(isValidCameraResolution('720p')).toBe(true);
			expect(isValidCameraResolution('1080p')).toBe(true);
			expect(isValidCameraResolution('4k')).toBe(false);
			expect(isValidCameraResolution('480p')).toBe(false);
		});

		it('should validate screenshare resolutions', () => {
			expect(isValidScreenshareResolution('720p')).toBe(true);
			expect(isValidScreenshareResolution('1080p')).toBe(true);
			expect(isValidScreenshareResolution('4k')).toBe(true);
			expect(isValidScreenshareResolution('360p')).toBe(false);
		});
	});

	describe('default voice preferences', () => {
		it('should default to VAD input mode', () => {
			expect(defaultVoicePrefs.input_mode).toBe('vad');
		});

		it('should default PTT key to Space', () => {
			expect(defaultVoicePrefs.ptt_key).toBe('Space');
		});

		it('should default all audio processing to true', () => {
			expect(defaultVoicePrefs.noise_suppression).toBe(true);
			expect(defaultVoicePrefs.echo_cancellation).toBe(true);
			expect(defaultVoicePrefs.auto_gain_control).toBe(true);
		});

		it('should default volumes to 1.0 (100%)', () => {
			expect(defaultVoicePrefs.input_volume).toBe(1.0);
			expect(defaultVoicePrefs.output_volume).toBe(1.0);
		});

		it('should default camera to 720p@30fps', () => {
			expect(defaultVoicePrefs.camera_resolution).toBe('720p');
			expect(defaultVoicePrefs.camera_framerate).toBe(30);
		});

		it('should default screenshare to 1080p@30fps with no audio', () => {
			expect(defaultVoicePrefs.screenshare_resolution).toBe('1080p');
			expect(defaultVoicePrefs.screenshare_framerate).toBe(30);
			expect(defaultVoicePrefs.screenshare_audio).toBe(false);
		});

		it('should have valid values for all fields', () => {
			expect(isValidCameraResolution(defaultVoicePrefs.camera_resolution)).toBe(true);
			expect(isValidScreenshareResolution(defaultVoicePrefs.screenshare_resolution)).toBe(true);
			expect(isValidFramerate(defaultVoicePrefs.camera_framerate)).toBe(true);
			expect(isValidFramerate(defaultVoicePrefs.screenshare_framerate)).toBe(true);
			expect(isValidVolume(defaultVoicePrefs.input_volume)).toBe(true);
			expect(isValidVolume(defaultVoicePrefs.output_volume)).toBe(true);
			expect(isValidVadThreshold(defaultVoicePrefs.vad_threshold)).toBe(true);
		});
	});

	describe('device localStorage persistence', () => {
		it('should save and load input device ID', () => {
			saveDeviceId('input', 'test-mic-123');
			expect(loadDeviceId('input')).toBe('test-mic-123');
		});

		it('should save and load output device ID', () => {
			saveDeviceId('output', 'test-speaker-456');
			expect(loadDeviceId('output')).toBe('test-speaker-456');
		});

		it('should return empty string when no device saved', () => {
			localStorage.removeItem(DEVICE_STORAGE_KEYS.input);
			localStorage.removeItem(DEVICE_STORAGE_KEYS.output);
			expect(loadDeviceId('input')).toBe('');
			expect(loadDeviceId('output')).toBe('');
		});

		it('should overwrite existing device ID', () => {
			saveDeviceId('input', 'old-device');
			saveDeviceId('input', 'new-device');
			expect(loadDeviceId('input')).toBe('new-device');
		});

		it('should use correct localStorage keys', () => {
			expect(DEVICE_STORAGE_KEYS.input).toBe('av-voice-input-device');
			expect(DEVICE_STORAGE_KEYS.output).toBe('av-voice-output-device');
		});
	});
});
