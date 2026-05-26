<script lang="ts">
	import WebhookPanel from '$components/guild/WebhookPanel.svelte';
	import { api } from '$lib/api/client';
	import { addToast } from '$lib/stores/toast';
	import { getErrorMessage } from '$lib/utils/apiError';
	import { createAsyncOp } from '$lib/utils/asyncOp';
	import type { Channel, Webhook } from '$lib/types';

	interface Props {
		guildId: string;
	}

	let { guildId }: Props = $props();

	let webhooks = $state<Webhook[]>([]);
	let loadOp = $state(createAsyncOp());
	let webhookChannels = $state<Channel[]>([]);
	let loadedGuildId = $state<string | null>(null);

	$effect(() => {
		if (guildId && loadedGuildId !== guildId && !loadOp.loading) {
			loadWebhooks();
		}
	});

	$effect(() => {
		function handleWebhooksChanged(event: Event) {
			const detail = (event as CustomEvent<{ guild_id?: string }>).detail;
			if (!detail?.guild_id || detail.guild_id === guildId) {
				loadWebhooks();
			}
		}

		window.addEventListener('amityvox:webhooks-changed', handleWebhooksChanged);
		return () => window.removeEventListener('amityvox:webhooks-changed', handleWebhooksChanged);
	});

	async function loadWebhooks() {
		const result = await loadOp.run(
			() => Promise.all([
				api.getGuildWebhooks(guildId),
				api.getGuildChannels(guildId)
			]),
			msg => addToast(msg, 'error'),
			'Failed to load webhooks'
		);
		if (result) {
			const [wh, ch] = result;
			webhooks = wh;
			webhookChannels = ch;
			loadedGuildId = guildId;
		}
	}
</script>

<h1 class="mb-6 text-xl font-bold text-text-primary">Webhooks</h1>

{#if loadOp.loading}
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
