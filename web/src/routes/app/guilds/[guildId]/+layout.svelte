<script lang="ts">
	import { page } from '$app/stores';
	import type { Snippet } from 'svelte';
	import { setGuild, federatedGuilds } from '$lib/stores/guilds';
	import { loadChannels, loadFederatedChannels, loadHiddenThreads } from '$lib/stores/channels';

	interface Props {
		children: Snippet;
	}

	let { children }: Props = $props();

	// Set current guild and load channels when route params change.
	$effect(() => {
		const guildId = $page.params.guildId;
		if (guildId) {
			setGuild(guildId);
			const fg = $federatedGuilds.get(guildId);
			if (fg) {
				loadFederatedChannels(fg.channels_json);
			} else {
				loadChannels(guildId);
				loadHiddenThreads();
			}
		}
	});
</script>

{@render children()}
