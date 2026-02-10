<script lang="ts">
	import { currentChannelId, currentChannel } from '$lib/stores/channels';
	import { api } from '$lib/api/client';
	import { getGatewayClient } from '$lib/stores/gateway';

	let content = $state('');
	let inputEl: HTMLTextAreaElement;
	let typingTimeout: ReturnType<typeof setTimeout> | null = null;

	async function handleSubmit() {
		const channelId = $currentChannelId;
		if (!channelId || !content.trim()) return;

		const msg = content.trim();
		content = '';

		// Reset textarea height.
		if (inputEl) inputEl.style.height = 'auto';

		try {
			await api.sendMessage(channelId, msg);
		} catch (e) {
			// Restore content on failure.
			content = msg;
			console.error('Failed to send message:', e);
		}
	}

	function handleKeydown(e: KeyboardEvent) {
		if (e.key === 'Enter' && !e.shiftKey) {
			e.preventDefault();
			handleSubmit();
			return;
		}

		// Send typing indicator (throttled).
		const channelId = $currentChannelId;
		if (channelId && !typingTimeout) {
			getGatewayClient()?.sendTyping(channelId);
			typingTimeout = setTimeout(() => {
				typingTimeout = null;
			}, 5000);
		}
	}

	function handleInput() {
		// Auto-resize textarea.
		if (inputEl) {
			inputEl.style.height = 'auto';
			inputEl.style.height = Math.min(inputEl.scrollHeight, 200) + 'px';
		}
	}

	async function handleFileUpload(e: Event) {
		const target = e.target as HTMLInputElement;
		const file = target.files?.[0];
		if (!file || !$currentChannelId) return;

		try {
			const uploaded = await api.uploadFile(file);
			await api.sendMessage($currentChannelId, '', { nonce: uploaded.id });
		} catch (err) {
			console.error('Upload failed:', err);
		}
		target.value = '';
	}
</script>

{#if $currentChannelId}
	<div class="border-t border-bg-floating px-4 pb-4 pt-2">
		<div class="flex items-end gap-2 rounded-lg bg-bg-modifier px-4 py-2">
			<!-- File upload -->
			<label class="cursor-pointer self-center text-text-muted hover:text-text-primary">
				<svg class="h-5 w-5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
					<path d="M21.44 11.05l-9.19 9.19a6 6 0 01-8.49-8.49l9.19-9.19a4 4 0 015.66 5.66l-9.2 9.19a2 2 0 01-2.83-2.83l8.49-8.48" />
				</svg>
				<input type="file" class="hidden" onchange={handleFileUpload} />
			</label>

			<!-- Text input -->
			<textarea
				bind:this={inputEl}
				bind:value={content}
				onkeydown={handleKeydown}
				oninput={handleInput}
				placeholder="Message #{$currentChannel?.name ?? 'channel'}"
				class="max-h-[200px] min-h-[24px] flex-1 resize-none bg-transparent text-sm text-text-primary outline-none placeholder:text-text-muted"
				rows="1"
			></textarea>
		</div>
	</div>
{/if}
