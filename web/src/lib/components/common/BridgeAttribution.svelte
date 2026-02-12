<script lang="ts">
	/**
	 * BridgeAttribution -- displays the origin platform badge for bridged messages.
	 * Shows the platform icon, original author name, and avatar when a message
	 * was relayed from an external platform (Matrix, Discord, Telegram, etc.).
	 *
	 * Usage:
	 * <BridgeAttribution
	 *   source="discord"
	 *   authorName="DiscordUser#1234"
	 *   authorAvatar="https://cdn.discordapp.com/avatars/..."
	 * />
	 */

	interface Props {
		source: string;                // bridge platform: 'matrix', 'discord', 'telegram', 'slack', 'irc', 'xmpp'
		authorName?: string | null;    // original author name on the source platform
		authorAvatar?: string | null;  // original author avatar URL
		compact?: boolean;             // compact mode (inline badge only)
	}

	let { source, authorName = null, authorAvatar = null, compact = false }: Props = $props();

	const PLATFORM_CONFIG: Record<string, { label: string; color: string; icon: string; bgColor: string }> = {
		matrix: {
			label: 'Matrix',
			color: 'text-green-400',
			bgColor: 'bg-green-500/15',
			icon: 'M',
		},
		discord: {
			label: 'Discord',
			color: 'text-indigo-400',
			bgColor: 'bg-indigo-500/15',
			icon: 'D',
		},
		telegram: {
			label: 'Telegram',
			color: 'text-blue-400',
			bgColor: 'bg-blue-500/15',
			icon: 'T',
		},
		slack: {
			label: 'Slack',
			color: 'text-purple-400',
			bgColor: 'bg-purple-500/15',
			icon: 'S',
		},
		irc: {
			label: 'IRC',
			color: 'text-orange-400',
			bgColor: 'bg-orange-500/15',
			icon: '#',
		},
		xmpp: {
			label: 'XMPP',
			color: 'text-cyan-400',
			bgColor: 'bg-cyan-500/15',
			icon: 'X',
		},
	};

	let config = $derived(PLATFORM_CONFIG[source] || {
		label: source,
		color: 'text-text-muted',
		bgColor: 'bg-bg-modifier',
		icon: '?',
	});
</script>

{#if compact}
	<!-- Compact: just a small inline badge -->
	<span
		class="inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-[10px] font-medium {config.bgColor} {config.color}"
		title="Bridged from {config.label}{authorName ? ` (${authorName})` : ''}"
	>
		<span class="font-bold">{config.icon}</span>
		{config.label}
	</span>
{:else}
	<!-- Full: shows platform badge with original author info -->
	<div class="flex items-center gap-2 text-xs">
		<!-- Platform badge -->
		<span class="inline-flex items-center gap-1 px-2 py-0.5 rounded-full {config.bgColor} {config.color} font-medium">
			<span class="w-3.5 h-3.5 rounded-full bg-current/20 flex items-center justify-center text-[9px] font-bold">
				{config.icon}
			</span>
			via {config.label}
		</span>

		<!-- Original author -->
		{#if authorName}
			<span class="text-text-muted">from</span>
			<span class="flex items-center gap-1.5">
				{#if authorAvatar}
					<img
						src={authorAvatar}
						alt="{authorName}'s avatar"
						class="w-4 h-4 rounded-full object-cover"
						onerror={(e) => {
							const target = e.currentTarget as HTMLImageElement;
							target.style.display = 'none';
						}}
					/>
				{/if}
				<span class="text-text-secondary font-medium">{authorName}</span>
			</span>
		{/if}
	</div>
{/if}
