<script lang="ts">
	import { page } from '$app/stores';
	import type { Snippet } from 'svelte';
	import { setGuild } from '$lib/stores/guilds';
	import { loadChannels, loadHiddenThreads } from '$lib/stores/channels';
	import { loadClientConfig } from '$lib/stores/clientConfig';

	interface Props {
		children: Snippet;
	}

	let { children }: Props = $props();

	// Set current guild and load channels when route params change.
	$effect(() => {
		const guildId = $page.params.guildId;
		if (guildId) {
			setGuild(guildId);
			loadClientConfig(guildId).then(() => loadHiddenThreads());
			loadChannels(guildId);
		}
	});
</script>

{@render children()}
