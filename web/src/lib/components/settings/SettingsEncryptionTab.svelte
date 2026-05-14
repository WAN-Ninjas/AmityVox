<script lang="ts">
	import { e2ee } from '$lib/encryption/e2eeManager';
	import { addToast } from '$lib/stores/toast';

	function clearAllKeys() {
		e2ee.reset();
		addToast('All encryption keys cleared', 'success');
	}
</script>

<h1 class="mb-6 text-xl font-bold text-text-primary">Encryption</h1>

<!-- E2EE info -->
<div class="mb-6 rounded-lg bg-bg-secondary p-4">
	<div class="flex items-center gap-3">
		<div class="flex h-10 w-10 shrink-0 items-center justify-center rounded-full bg-status-online/15">
			<svg class="h-5 w-5 text-status-online" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
				<path d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z" />
			</svg>
		</div>
		<div>
			<h3 class="text-sm font-semibold text-text-primary">End-to-End Encryption</h3>
			<p class="text-xs text-text-muted">
				Encryption is managed per-channel using passphrases. When you enable encryption on a channel,
				you set a passphrase that is used to derive the encryption key. Share the passphrase with channel
				members out-of-band. The server never sees the passphrase or the encryption key.
			</p>
		</div>
	</div>
</div>

<!-- Reset all keys -->
<div class="mb-6 rounded-lg bg-bg-secondary p-4">
	<h3 class="mb-1 text-sm font-semibold text-text-primary">Clear All Keys</h3>
	<p class="mb-3 text-xs text-text-muted">
		Remove all stored encryption keys from this device. You will need to re-enter
		passphrases for any encrypted channels.
	</p>
	<button
		class="rounded-md bg-red-500 px-4 py-2 text-xs font-medium text-white hover:bg-red-600"
		onclick={clearAllKeys}
	>
		Clear All Keys
	</button>
</div>
