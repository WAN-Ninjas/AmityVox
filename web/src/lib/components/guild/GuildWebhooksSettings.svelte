<script lang="ts">
	import WebhookPanel from '$components/guild/WebhookPanel.svelte';
	import { api } from '$lib/api/client';
	import { addToast } from '$lib/stores/toast';
	import type { Channel, Webhook } from '$lib/types';

	interface Props {
		guildId: string;
	}

	let { guildId }: Props = $props();

	let webhooks = $state<Webhook[]>([]);
	let loadingWebhooks = $state(false);
	let webhookChannels = $state<Channel[]>([]);
	let loadedGuildId = $state<string | null>(null);

	$effect(() => {
		if (guildId && loadedGuildId !== guildId && !loadingWebhooks) {
			loadWebhooks();
		}
	});

	async function loadWebhooks() {
		loadingWebhooks = true;
		try {
			const [wh, ch] = await Promise.all([
				api.getGuildWebhooks(guildId),
				api.getGuildChannels(guildId)
			]);
			webhooks = wh;
			webhookChannels = ch;
			loadedGuildId = guildId;
		} catch (err: any) {
			addToast(err.message || 'Failed to load webhooks', 'error');
		} finally {
			loadingWebhooks = false;
		}
	}
</script>

<h1 class="mb-6 text-xl font-bold text-text-primary">Webhooks</h1>

{#if loadingWebhooks}
	<p class="text-sm text-text-muted">Loading webhooks...</p>
{:else}
	<WebhookPanel
		{guildId}
		bind:webhooks
		channels={webhookChannels}
		onError={(msg) => addToast(msg, 'error')}
		onSuccess={(msg) => addToast(msg, 'success')}
	/>
{/if}
