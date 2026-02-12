<script lang="ts">
	import { api } from '$lib/api/client';

	let {
		guildId,
		channelId
	}: {
		guildId: string;
		channelId: string;
	} = $props();

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

	let sounds = $state<SoundboardSound[]>([]);
	let loading = $state(false);
	let playing = $state<string | null>(null);
	let cooldownActive = $state(false);
	let error = $state<string | null>(null);
	let expanded = $state(true);

	$effect(() => {
		if (guildId) {
			loadSounds();
		}
	});

	async function loadSounds() {
		try {
			loading = true;
			error = null;
			const res = await fetch(`/api/v1/guilds/${guildId}/soundboard/sounds`, {
				headers: { 'Authorization': `Bearer ${api.getToken()}` }
			});
			if (res.ok) {
				const json = await res.json();
				sounds = json.data || [];
			} else {
				const errJson = await res.json();
				error = errJson.error?.message || 'Failed to load sounds';
			}
		} catch (err) {
			error = 'Failed to load soundboard sounds';
			console.error('Soundboard load error:', err);
		} finally {
			loading = false;
		}
	}

	async function playSound(sound: SoundboardSound) {
		if (cooldownActive || playing) return;

		try {
			playing = sound.id;
			error = null;

			const res = await fetch(`/api/v1/guilds/${guildId}/soundboard/sounds/${sound.id}/play`, {
				method: 'POST',
				headers: {
					'Content-Type': 'application/json',
					'Authorization': `Bearer ${api.getToken()}`
				}
			});

			if (!res.ok) {
				const errJson = await res.json();
				if (res.status === 429) {
					cooldownActive = true;
					setTimeout(() => cooldownActive = false, 5000);
				}
				error = errJson.error?.message || 'Failed to play sound';
				return;
			}

			// Play the sound locally as preview
			const audio = new Audio(`/api/v1/files/${sound.file_url}`);
			audio.volume = Math.min(sound.volume, 1.0);
			audio.play().catch(() => {});

			// Auto-clear playing state after duration
			setTimeout(() => {
				playing = null;
			}, sound.duration_ms);

		} catch (err) {
			error = 'Failed to play sound';
			console.error('Sound play error:', err);
		} finally {
			// Clear playing state after a short delay if not already cleared
			setTimeout(() => {
				if (playing === sound.id) playing = null;
			}, 500);
		}
	}

	function formatDuration(ms: number): string {
		const seconds = Math.floor(ms / 1000);
		const remainder = ms % 1000;
		return `${seconds}.${Math.floor(remainder / 100)}s`;
	}
</script>

<div class="soundboard">
	<button
		class="soundboard-header"
		onclick={() => expanded = !expanded}
	>
		<svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor">
			<path d="M3 9v6h4l5 5V4L7 9H3zm13.5 3c0-1.77-1.02-3.29-2.5-4.03v8.05c1.48-.73 2.5-2.25 2.5-4.02zM14 3.23v2.06c2.89.86 5 3.54 5 6.71s-2.11 5.85-5 6.71v2.06c4.01-.91 7-4.49 7-8.77s-2.99-7.86-7-8.77z"/>
		</svg>
		<span class="header-text">Soundboard</span>
		<span class="sound-count">{sounds.length}</span>
		<svg
			width="12"
			height="12"
			viewBox="0 0 24 24"
			fill="currentColor"
			class="chevron"
			class:rotated={expanded}
		>
			<path d="M7 10l5 5 5-5z"/>
		</svg>
	</button>

	{#if expanded}
		{#if loading}
			<div class="loading">Loading sounds...</div>
		{:else if sounds.length === 0}
			<div class="empty">No sounds available</div>
		{:else}
			<div class="sound-grid">
				{#each sounds as sound (sound.id)}
					<button
						class="sound-btn"
						class:playing={playing === sound.id}
						class:disabled={cooldownActive && playing !== sound.id}
						onclick={() => playSound(sound)}
						disabled={cooldownActive && playing !== sound.id}
						title="{sound.name} ({formatDuration(sound.duration_ms)})"
					>
						{#if sound.emoji}
							<span class="sound-emoji">{sound.emoji}</span>
						{:else}
							<svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor" class="sound-icon">
								<path d="M3 9v6h4l5 5V4L7 9H3z"/>
							</svg>
						{/if}
						<span class="sound-name">{sound.name}</span>
						{#if playing === sound.id}
							<div class="playing-indicator">
								<span class="bar"></span>
								<span class="bar"></span>
								<span class="bar"></span>
							</div>
						{/if}
					</button>
				{/each}
			</div>
		{/if}

		{#if error}
			<div class="error-msg">{error}</div>
		{/if}

		{#if cooldownActive}
			<div class="cooldown-msg">Cooldown active...</div>
		{/if}
	{/if}
</div>

<style>
	.soundboard {
		background: var(--bg-secondary);
		border-radius: 8px;
		overflow: hidden;
	}

	.soundboard-header {
		display: flex;
		align-items: center;
		gap: 8px;
		width: 100%;
		padding: 10px 12px;
		background: none;
		border: none;
		color: var(--text-secondary);
		cursor: pointer;
		font-size: 12px;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.02em;
	}

	.soundboard-header:hover {
		color: var(--text-primary);
	}

	.header-text {
		flex: 1;
		text-align: left;
	}

	.sound-count {
		padding: 1px 6px;
		background: var(--bg-tertiary);
		border-radius: 10px;
		font-size: 11px;
		color: var(--text-muted);
	}

	.chevron {
		transition: transform 0.2s ease;
	}

	.chevron.rotated {
		transform: rotate(180deg);
	}

	.loading, .empty {
		padding: 16px;
		text-align: center;
		font-size: 13px;
		color: var(--text-muted);
	}

	.sound-grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(90px, 1fr));
		gap: 4px;
		padding: 4px 8px 8px;
	}

	.sound-btn {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 4px;
		padding: 8px 4px;
		background: var(--bg-tertiary);
		border: none;
		border-radius: 6px;
		cursor: pointer;
		color: var(--text-secondary);
		transition: all 0.15s ease;
		position: relative;
		overflow: hidden;
	}

	.sound-btn:hover:not(.disabled) {
		background: var(--bg-primary);
		color: var(--text-primary);
		transform: scale(1.02);
	}

	.sound-btn.playing {
		background: var(--brand-500);
		color: white;
	}

	.sound-btn.disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}

	.sound-emoji {
		font-size: 20px;
	}

	.sound-icon {
		opacity: 0.7;
	}

	.sound-name {
		font-size: 11px;
		font-weight: 500;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
		max-width: 100%;
	}

	.playing-indicator {
		display: flex;
		gap: 2px;
		align-items: flex-end;
		height: 12px;
		position: absolute;
		bottom: 2px;
		right: 4px;
	}

	.bar {
		width: 2px;
		background: white;
		border-radius: 1px;
		animation: soundbar 0.6s ease-in-out infinite;
	}

	.bar:nth-child(1) { height: 4px; animation-delay: 0s; }
	.bar:nth-child(2) { height: 8px; animation-delay: 0.15s; }
	.bar:nth-child(3) { height: 6px; animation-delay: 0.3s; }

	@keyframes soundbar {
		0%, 100% { transform: scaleY(1); }
		50% { transform: scaleY(0.4); }
	}

	.error-msg {
		padding: 6px 12px;
		font-size: 12px;
		color: #ef4444;
	}

	.cooldown-msg {
		padding: 6px 12px;
		font-size: 12px;
		color: var(--text-muted);
		text-align: center;
	}
</style>
