// Audio playback utility for notification sounds.
// Uses Web Audio API oscillator tones -- no audio files needed.

export interface SoundPreset {
	id: string;
	name: string;
}

export const SOUND_PRESETS: SoundPreset[] = [
	{ id: 'default', name: 'Default Beep' },
	{ id: 'chime', name: 'Chime' },
	{ id: 'bell', name: 'Bell' },
	{ id: 'pop', name: 'Pop' },
	{ id: 'none', name: 'None' }
];

let audioContext: AudioContext | null = null;

function getAudioContext(): AudioContext | null {
	if (typeof window === 'undefined') return null;
	if (!audioContext) {
		try {
			audioContext = new AudioContext();
		} catch {
			return null;
		}
	}
	// Resume if suspended (browsers require user gesture before playing).
	if (audioContext.state === 'suspended') {
		audioContext.resume();
	}
	return audioContext;
}

function playDefaultBeep(ctx: AudioContext, volume: number) {
	const now = ctx.currentTime;
	const osc = ctx.createOscillator();
	const gain = ctx.createGain();

	osc.type = 'sine';
	osc.frequency.setValueAtTime(440, now);

	gain.gain.setValueAtTime(volume, now);
	gain.gain.exponentialRampToValueAtTime(0.001, now + 0.15);

	osc.connect(gain);
	gain.connect(ctx.destination);

	osc.start(now);
	osc.stop(now + 0.15);
}

function playChime(ctx: AudioContext, volume: number) {
	const now = ctx.currentTime;

	// First tone: 440Hz
	const osc1 = ctx.createOscillator();
	const gain1 = ctx.createGain();
	osc1.type = 'sine';
	osc1.frequency.setValueAtTime(440, now);
	gain1.gain.setValueAtTime(volume, now);
	gain1.gain.exponentialRampToValueAtTime(0.001, now + 0.12);
	osc1.connect(gain1);
	gain1.connect(ctx.destination);
	osc1.start(now);
	osc1.stop(now + 0.12);

	// Second tone: 660Hz (rising)
	const osc2 = ctx.createOscillator();
	const gain2 = ctx.createGain();
	osc2.type = 'sine';
	osc2.frequency.setValueAtTime(660, now + 0.1);
	gain2.gain.setValueAtTime(0.001, now);
	gain2.gain.setValueAtTime(volume, now + 0.1);
	gain2.gain.exponentialRampToValueAtTime(0.001, now + 0.25);
	osc2.connect(gain2);
	gain2.connect(ctx.destination);
	osc2.start(now + 0.1);
	osc2.stop(now + 0.25);
}

function playBell(ctx: AudioContext, volume: number) {
	const now = ctx.currentTime;
	const osc = ctx.createOscillator();
	const gain = ctx.createGain();

	osc.type = 'sine';
	osc.frequency.setValueAtTime(880, now);
	// Slight frequency drop for bell-like decay.
	osc.frequency.exponentialRampToValueAtTime(440, now + 0.4);

	gain.gain.setValueAtTime(volume, now);
	gain.gain.exponentialRampToValueAtTime(0.001, now + 0.4);

	osc.connect(gain);
	gain.connect(ctx.destination);

	osc.start(now);
	osc.stop(now + 0.4);
}

function playPop(ctx: AudioContext, volume: number) {
	const now = ctx.currentTime;
	const osc = ctx.createOscillator();
	const gain = ctx.createGain();

	osc.type = 'sine';
	osc.frequency.setValueAtTime(330, now);
	osc.frequency.exponentialRampToValueAtTime(165, now + 0.06);

	gain.gain.setValueAtTime(volume, now);
	gain.gain.exponentialRampToValueAtTime(0.001, now + 0.08);

	osc.connect(gain);
	gain.connect(ctx.destination);

	osc.start(now);
	osc.stop(now + 0.08);
}

function playVoiceJoin(ctx: AudioContext, volume: number) {
	const now = ctx.currentTime;

	// First tone: E5 (659Hz)
	const osc1 = ctx.createOscillator();
	const gain1 = ctx.createGain();
	osc1.type = 'sine';
	osc1.frequency.setValueAtTime(659, now);
	gain1.gain.setValueAtTime(volume, now);
	gain1.gain.exponentialRampToValueAtTime(0.001, now + 0.1);
	osc1.connect(gain1);
	gain1.connect(ctx.destination);
	osc1.start(now);
	osc1.stop(now + 0.1);

	// Second tone: A5 (880Hz) — ascending, welcoming
	const osc2 = ctx.createOscillator();
	const gain2 = ctx.createGain();
	osc2.type = 'sine';
	osc2.frequency.setValueAtTime(880, now + 0.1);
	gain2.gain.setValueAtTime(0.001, now);
	gain2.gain.setValueAtTime(volume, now + 0.1);
	gain2.gain.exponentialRampToValueAtTime(0.001, now + 0.2);
	osc2.connect(gain2);
	gain2.connect(ctx.destination);
	osc2.start(now + 0.1);
	osc2.stop(now + 0.2);
}

function playRingtone(ctx: AudioContext, volume: number) {
	const now = ctx.currentTime;

	// Classic phone ring: two-tone alternating pattern (440Hz + 480Hz).
	// Plays 3 ring bursts with gaps for a distinctive incoming call sound.
	for (let burst = 0; burst < 3; burst++) {
		const offset = burst * 0.7; // Each burst 0.7s apart

		// First tone: 440Hz
		const osc1 = ctx.createOscillator();
		const gain1 = ctx.createGain();
		osc1.type = 'sine';
		osc1.frequency.setValueAtTime(440, now + offset);
		gain1.gain.setValueAtTime(0.001, now);
		gain1.gain.setValueAtTime(volume * 0.6, now + offset);
		gain1.gain.setValueAtTime(volume * 0.6, now + offset + 0.4);
		gain1.gain.exponentialRampToValueAtTime(0.001, now + offset + 0.45);
		osc1.connect(gain1);
		gain1.connect(ctx.destination);
		osc1.start(now + offset);
		osc1.stop(now + offset + 0.45);

		// Second tone: 480Hz (mixed for richer ring)
		const osc2 = ctx.createOscillator();
		const gain2 = ctx.createGain();
		osc2.type = 'sine';
		osc2.frequency.setValueAtTime(480, now + offset);
		gain2.gain.setValueAtTime(0.001, now);
		gain2.gain.setValueAtTime(volume * 0.4, now + offset);
		gain2.gain.setValueAtTime(volume * 0.4, now + offset + 0.4);
		gain2.gain.exponentialRampToValueAtTime(0.001, now + offset + 0.45);
		osc2.connect(gain2);
		gain2.connect(ctx.destination);
		osc2.start(now + offset);
		osc2.stop(now + offset + 0.45);
	}
}

function playVoiceLeave(ctx: AudioContext, volume: number) {
	const now = ctx.currentTime;

	// First tone: A4 (440Hz)
	const osc1 = ctx.createOscillator();
	const gain1 = ctx.createGain();
	osc1.type = 'sine';
	osc1.frequency.setValueAtTime(440, now);
	gain1.gain.setValueAtTime(volume * 0.8, now);
	gain1.gain.exponentialRampToValueAtTime(0.001, now + 0.1);
	osc1.connect(gain1);
	gain1.connect(ctx.destination);
	osc1.start(now);
	osc1.stop(now + 0.1);

	// Second tone: E4 (330Hz) — descending, softer
	const osc2 = ctx.createOscillator();
	const gain2 = ctx.createGain();
	osc2.type = 'sine';
	osc2.frequency.setValueAtTime(330, now + 0.1);
	gain2.gain.setValueAtTime(0.001, now);
	gain2.gain.setValueAtTime(volume * 0.8, now + 0.1);
	gain2.gain.exponentialRampToValueAtTime(0.001, now + 0.2);
	osc2.connect(gain2);
	gain2.connect(ctx.destination);
	osc2.start(now + 0.1);
	osc2.stop(now + 0.2);
}

/**
 * Play a notification sound using the Web Audio API.
 * @param preset - The sound preset id ('default', 'chime', 'bell', 'pop', 'voice-join', 'voice-leave', 'none')
 * @param volume - Volume level from 0 to 100 (default 80)
 */
export function playNotificationSound(preset: string, volume: number = 80): void {
	if (preset === 'none' || volume <= 0) return;

	const ctx = getAudioContext();
	if (!ctx) return;

	// Normalize volume from 0-100 range to 0.0-1.0 gain.
	const normalizedVolume = Math.max(0, Math.min(1, volume / 100));

	switch (preset) {
		case 'chime':
			playChime(ctx, normalizedVolume);
			break;
		case 'bell':
			playBell(ctx, normalizedVolume);
			break;
		case 'pop':
			playPop(ctx, normalizedVolume);
			break;
		case 'voice-join':
			playVoiceJoin(ctx, normalizedVolume);
			break;
		case 'voice-leave':
			playVoiceLeave(ctx, normalizedVolume);
			break;
		case 'ringtone':
			playRingtone(ctx, normalizedVolume);
			break;
		case 'default':
		default:
			playDefaultBeep(ctx, normalizedVolume);
			break;
	}
}
