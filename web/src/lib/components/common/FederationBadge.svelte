<script lang="ts">
	/**
	 * FederationBadge -- displays a small domain badge next to remote usernames.
	 * Shows the instance domain to distinguish federated users from local ones.
	 *
	 * Usage:
	 * <FederationBadge domain="other.instance.com" />
	 */

	interface Props {
		domain: string;    // remote instance domain
		compact?: boolean; // even smaller inline style
	}

	let { domain, compact = false }: Props = $props();
</script>

{#if compact}
	<span
		class="text-[10px] text-text-muted font-normal ml-0.5"
		title="Federated from {domain}"
	>@{domain}</span>
{:else}
	<span
		class="inline-flex items-center gap-0.5 px-1.5 py-0.5 rounded text-[10px] font-medium bg-blue-500/15 text-blue-400"
		title="Federated from {domain}"
	>
		<svg class="w-2.5 h-2.5" viewBox="0 0 16 16" fill="currentColor">
			<path d="M8 0a8 8 0 100 16A8 8 0 008 0zm5.3 5H11a13 13 0 00-1-3.3A6 6 0 0113.3 5zM8 1.5c.7.8 1.3 2 1.7 3.5H6.3C6.7 3.5 7.3 2.3 8 1.5zM1.5 9a6.5 6.5 0 010-2h2.8a13 13 0 000 2H1.5zm.2 1h2.5a13 13 0 001 3.3A6 6 0 011.7 10zm2.5-5H1.7A6 6 0 016 1.7 13 13 0 004.2 5zM8 14.5c-.7-.8-1.3-2-1.7-3.5h3.4c-.4 1.5-1 2.7-1.7 3.5zm2-4.5H6a12 12 0 010-4h4a12 12 0 010 4zm.1 3.3a13 13 0 001-3.3h2.5a6 6 0 01-3.5 3.3zM11.7 9a13 13 0 000-2h2.8a6.5 6.5 0 010 2h-2.8z"/>
		</svg>
		{domain}
	</span>
{/if}
