<script lang="ts">
	import MembersPanel from '$components/guild/MembersPanel.svelte';
	import { addToast } from '$lib/stores/toast';

	interface Props {
		guildId: string;
	}

	let { guildId }: Props = $props();
</script>

<h1 class="mb-6 text-xl font-bold text-text-primary">Members</h1>

<MembersPanel
	{guildId}
	onError={(msg) => addToast(msg, 'error')}
	onSuccess={(msg) => addToast(msg, 'success')}
/>
