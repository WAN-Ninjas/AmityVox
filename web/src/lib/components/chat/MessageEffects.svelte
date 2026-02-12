<!-- MessageEffects.svelte — Renders confetti, fireworks, hearts, snow, and super reactions with particle effects. -->
<script lang="ts">
	import { api } from '$lib/api/client';

	interface EffectEvent {
		id: string;
		message_id: string;
		channel_id: string;
		user_id: string;
		effect_type: string;
		config: Record<string, unknown>;
	}

	interface SuperReaction {
		id: string;
		message_id: string;
		user_id: string;
		emoji: string;
		intensity: number;
		username: string;
	}

	interface Props {
		messageId: string;
		channelId: string;
	}

	let { messageId, channelId }: Props = $props();

	let activeEffect = $state<EffectEvent | null>(null);
	let particles = $state<Array<{ id: number; x: number; y: number; emoji: string; scale: number; opacity: number; rotation: number; vx: number; vy: number }>>([]);
	let superReactions = $state<SuperReaction[]>([]);
	let showEffectMenu = $state(false);
	let sending = $state(false);
	let animFrame = $state<number>(0);

	const effectTypes = [
		{ type: 'confetti', label: 'Confetti', icon: '🎊' },
		{ type: 'fireworks', label: 'Fireworks', icon: '🎆' },
		{ type: 'hearts', label: 'Hearts', icon: '💕' },
		{ type: 'snow', label: 'Snow', icon: '❄️' },
		{ type: 'party_popper', label: 'Party Popper', icon: '🎉' },
		{ type: 'sparkles', label: 'Sparkles', icon: '✨' },
		{ type: 'spotlight', label: 'Spotlight', icon: '🔦' },
		{ type: 'shake', label: 'Shake', icon: '📳' }
	];

	const confettiColors = ['#ff6b6b', '#4ecdc4', '#45b7d1', '#f9ca24', '#6c5ce7', '#a29bfe', '#fd79a8', '#00cec9'];

	async function sendEffect(effectType: string) {
		sending = true;
		showEffectMenu = false;
		try {
			const result = await api.request<EffectEvent>(
				'POST',
				`/channels/${channelId}/messages/${messageId}/effects`,
				{ effect_type: effectType, config: {} }
			);
			if (result) {
				triggerEffect(result);
			}
		} catch {
			// Silently handle (effect is cosmetic).
		} finally {
			sending = false;
		}
	}

	async function addSuperReaction(emoji: string, intensity: number = 1) {
		try {
			await api.request<SuperReaction>(
				'POST',
				`/channels/${channelId}/messages/${messageId}/super-reactions`,
				{ emoji, intensity }
			);
			await loadSuperReactions();
		} catch {
			// Silently handle.
		}
	}

	async function loadSuperReactions() {
		try {
			const data = await api.request<SuperReaction[]>(
				'GET',
				`/channels/${channelId}/messages/${messageId}/super-reactions`
			);
			superReactions = data ?? [];
		} catch {
			// Ignore.
		}
	}

	function triggerEffect(effect: EffectEvent) {
		activeEffect = effect;
		particles = [];

		switch (effect.effect_type) {
			case 'confetti':
				spawnConfetti();
				break;
			case 'fireworks':
				spawnFireworks();
				break;
			case 'hearts':
				spawnFloatingEmoji('💕', 30);
				break;
			case 'snow':
				spawnFloatingEmoji('❄️', 40);
				break;
			case 'party_popper':
				spawnConfetti();
				spawnFloatingEmoji('🎉', 10);
				break;
			case 'sparkles':
				spawnFloatingEmoji('✨', 25);
				break;
		}

		// Clear effect after animation.
		setTimeout(() => {
			activeEffect = null;
			particles = [];
			if (animFrame) cancelAnimationFrame(animFrame);
		}, 3000);
	}

	function spawnConfetti() {
		const newParticles = [];
		for (let i = 0; i < 60; i++) {
			newParticles.push({
				id: i,
				x: Math.random() * 100,
				y: -10 - Math.random() * 20,
				emoji: '',
				scale: 0.5 + Math.random() * 0.5,
				opacity: 1,
				rotation: Math.random() * 360,
				vx: (Math.random() - 0.5) * 3,
				vy: 1 + Math.random() * 3
			});
		}
		particles = newParticles;
		animateParticles();
	}

	function spawnFireworks() {
		const cx = 50;
		const cy = 50;
		const newParticles = [];
		for (let i = 0; i < 40; i++) {
			const angle = (i / 40) * Math.PI * 2;
			const speed = 2 + Math.random() * 3;
			newParticles.push({
				id: i,
				x: cx,
				y: cy,
				emoji: '',
				scale: 0.3 + Math.random() * 0.4,
				opacity: 1,
				rotation: 0,
				vx: Math.cos(angle) * speed,
				vy: Math.sin(angle) * speed
			});
		}
		particles = newParticles;
		animateParticles();
	}

	function spawnFloatingEmoji(emoji: string, count: number) {
		const newParticles = [];
		for (let i = 0; i < count; i++) {
			newParticles.push({
				id: i,
				x: Math.random() * 100,
				y: 110 + Math.random() * 20,
				emoji,
				scale: 0.6 + Math.random() * 0.8,
				opacity: 1,
				rotation: Math.random() * 30 - 15,
				vx: (Math.random() - 0.5) * 0.5,
				vy: -(1 + Math.random() * 2)
			});
		}
		particles = newParticles;
		animateParticles();
	}

	function animateParticles() {
		if (particles.length === 0) return;
		particles = particles
			.map((p) => ({
				...p,
				x: p.x + p.vx,
				y: p.y + p.vy,
				vy: p.vy + 0.05,
				opacity: Math.max(0, p.opacity - 0.008),
				rotation: p.rotation + p.vx * 2
			}))
			.filter((p) => p.opacity > 0 && p.y < 120);

		if (particles.length > 0) {
			animFrame = requestAnimationFrame(animateParticles);
		}
	}

	// Trigger super reaction particle burst.
	function triggerSuperReactionBurst(emoji: string, intensity: number) {
		const count = intensity * 8;
		spawnFloatingEmoji(emoji, count);
		setTimeout(() => { particles = []; }, 2000);
	}

	$effect(() => {
		if (messageId) {
			loadSuperReactions();
		}
	});
</script>

<div class="relative">
	<!-- Effect trigger button -->
	<div class="relative inline-block">
		<button
			type="button"
			class="text-text-muted hover:text-text-primary transition-colors p-1 rounded hover:bg-bg-tertiary"
			title="Add effect"
			onclick={() => (showEffectMenu = !showEffectMenu)}
			disabled={sending}
		>
			<svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
				<path stroke-linecap="round" stroke-linejoin="round" d="M5 3v4M3 5h4M6 17v4m-2-2h4m5-16l2.286 6.857L21 12l-5.714 2.143L13 21l-2.286-6.857L5 12l5.714-2.143L13 3z" />
			</svg>
		</button>

		{#if showEffectMenu}
			<div class="absolute bottom-full left-0 mb-1 bg-bg-secondary border border-border-primary rounded-lg shadow-lg p-2 z-50 min-w-[180px]">
				<div class="text-text-muted text-xs font-medium px-2 py-1 mb-1">Message Effects</div>
				<div class="grid grid-cols-4 gap-1">
					{#each effectTypes as effect}
						<button
							type="button"
							class="flex flex-col items-center gap-0.5 p-1.5 rounded hover:bg-bg-tertiary transition-colors"
							title={effect.label}
							onclick={() => sendEffect(effect.type)}
						>
							<span class="text-lg">{effect.icon}</span>
							<span class="text-[10px] text-text-muted">{effect.label}</span>
						</button>
					{/each}
				</div>

				<div class="border-t border-border-primary mt-2 pt-2">
					<div class="text-text-muted text-xs font-medium px-2 py-1 mb-1">Super React</div>
					<div class="flex gap-1 px-1">
						{#each ['🔥', '💀', '👀', '💯', '🫡'] as emoji}
							<button
								type="button"
								class="text-lg p-1 rounded hover:bg-bg-tertiary hover:scale-125 transition-all"
								title="Super React with {emoji}"
								onclick={() => addSuperReaction(emoji, 3)}
							>
								{emoji}
							</button>
						{/each}
					</div>
				</div>
			</div>
		{/if}
	</div>

	<!-- Super reactions display -->
	{#if superReactions.length > 0}
		<div class="flex flex-wrap gap-1 mt-1">
			{#each groupSuperReactions(superReactions) as group}
				<button
					type="button"
					class="inline-flex items-center gap-1 px-2 py-0.5 rounded-full bg-brand-500/10 border border-brand-500/20 hover:bg-brand-500/20 transition-colors text-sm"
					title="{group.users.join(', ')} super reacted"
					onclick={() => triggerSuperReactionBurst(group.emoji, group.maxIntensity)}
				>
					<span class="text-base">{group.emoji}</span>
					{#if group.maxIntensity > 1}
						{#each Array(Math.min(group.maxIntensity, 3)) as _}
							<span class="text-xs text-brand-400">+</span>
						{/each}
					{/if}
					<span class="text-xs text-text-secondary">{group.count}</span>
				</button>
			{/each}
		</div>
	{/if}

	<!-- Particle effects overlay -->
	{#if particles.length > 0}
		<div class="absolute inset-0 pointer-events-none overflow-hidden z-50" aria-hidden="true">
			{#each particles as p (p.id)}
				{#if p.emoji}
					<span
						class="absolute text-lg select-none"
						style="left: {p.x}%; top: {p.y}%; opacity: {p.opacity}; transform: scale({p.scale}) rotate({p.rotation}deg);"
					>
						{p.emoji}
					</span>
				{:else}
					<span
						class="absolute w-2 h-2 rounded-full select-none"
						style="left: {p.x}%; top: {p.y}%; opacity: {p.opacity}; transform: scale({p.scale}) rotate({p.rotation}deg); background-color: {confettiColors[p.id % confettiColors.length]};"
					></span>
				{/if}
			{/each}
		</div>
	{/if}

	<!-- Shake effect on the message -->
	{#if activeEffect?.effect_type === 'shake'}
		<style>
			@keyframes msgShake {
				0%, 100% { transform: translateX(0); }
				10%, 30%, 50%, 70%, 90% { transform: translateX(-4px); }
				20%, 40%, 60%, 80% { transform: translateX(4px); }
			}
		</style>
	{/if}
</div>

<script lang="ts" context="module">
	function groupSuperReactions(reactions: Array<{ emoji: string; intensity: number; username: string }>) {
		const groups = new Map<string, { emoji: string; count: number; maxIntensity: number; users: string[] }>();
		for (const r of reactions) {
			const existing = groups.get(r.emoji);
			if (existing) {
				existing.count++;
				existing.maxIntensity = Math.max(existing.maxIntensity, r.intensity);
				existing.users.push(r.username);
			} else {
				groups.set(r.emoji, { emoji: r.emoji, count: 1, maxIntensity: r.intensity, users: [r.username] });
			}
		}
		return Array.from(groups.values());
	}
</script>
