<script lang="ts">
	import { page } from '$app/stores';
	import type { Snippet } from 'svelte';
	import { setGuild } from '$lib/stores/guilds';
	import { loadChannels, loadHiddenThreads } from '$lib/stores/channels';

	interface Props {
		children: Snippet;
	}

	let { children }: Props = $props();

	// Set current guild and load channels when route params change.
	$effect(() => {
		const guildId = $page.params.guildId;
		if (guildId) {
			setGuild(guildId);
			loadChannels(guildId);
			loadHiddenThreads();
		}
	});
</script>

{@render children()}
