<script lang="ts">
	import { currentGuild } from '$lib/stores/guilds';
	import { textChannels } from '$lib/stores/channels';
	import { goto } from '$app/navigation';
	import { page } from '$app/stores';

	// Auto-redirect to the first text channel.
	$effect(() => {
		const channels = $textChannels;
		const guildId = $page.params.guildId;
		if (channels.length > 0 && guildId) {
			goto(`/app/guilds/${guildId}/channels/${channels[0].id}`, { replaceState: true });
		}
	});
</script>

<svelte:head>
	<title>{$currentGuild?.name ?? 'Guild'} — AmityVox</title>
</svelte:head>

<div class="flex h-full items-center justify-center bg-bg-tertiary">
	<p class="text-text-muted">Loading channels...</p>
</div>
