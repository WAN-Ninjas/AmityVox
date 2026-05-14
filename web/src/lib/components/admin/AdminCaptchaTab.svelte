<script lang="ts">
	import { onMount } from 'svelte';
	import { api, type CaptchaConfig } from '$lib/api/client';
	import { addToast } from '$lib/stores/toast';

	let captchaConfig = $state<CaptchaConfig | null>(null);
	let loadingCaptcha = $state(false);
	let savingCaptcha = $state(false);
	let captchaProvider = $state<'none' | 'hcaptcha' | 'recaptcha'>('none');
	let captchaSiteKey = $state('');
	let captchaSecretKey = $state('');

	onMount(() => {
		loadCaptchaConfig();
	});

	async function loadCaptchaConfig() {
		loadingCaptcha = true;
		try {
			captchaConfig = await api.getCaptchaConfig();
			captchaProvider = (captchaConfig?.provider ?? 'none') as 'none' | 'hcaptcha' | 'recaptcha';
			captchaSiteKey = captchaConfig?.site_key ?? '';
			captchaSecretKey = '';
		} catch {
			captchaConfig = null;
		} finally {
			loadingCaptcha = false;
		}
	}

	async function saveCaptchaConfig() {
		savingCaptcha = true;
		try {
			const body: { provider: 'none' | 'hcaptcha' | 'recaptcha'; site_key?: string; secret_key?: string } = { provider: captchaProvider };
			if (captchaSiteKey) body.site_key = captchaSiteKey;
			if (captchaSecretKey) body.secret_key = captchaSecretKey;
			captchaConfig = await api.updateCaptchaConfig(body);
			captchaSiteKey = captchaConfig.site_key ?? '';
			captchaSecretKey = '';
			addToast('CAPTCHA settings saved', 'success');
		} catch (err: any) {
			addToast(err.message || 'Failed to save CAPTCHA settings', 'error');
		} finally {
			savingCaptcha = false;
		}
	}
</script>

<h1 class="mb-6 text-2xl font-bold text-text-primary">CAPTCHA Settings</h1>
<p class="mb-6 text-sm text-text-muted">
	Configure CAPTCHA verification for user registration. When enabled, new users must complete a CAPTCHA challenge before creating an account.
</p>

{#if loadingCaptcha}
	<p class="text-sm text-text-muted">Loading CAPTCHA settings...</p>
{:else}
	<div class="space-y-6">
		<!-- Provider Selection -->
		<div class="rounded-lg bg-bg-secondary p-4">
			<p class="mb-3 block text-xs font-bold uppercase tracking-wide text-text-muted">CAPTCHA Provider</p>
			<div class="space-y-2">
				<label class="flex items-center gap-3 rounded p-2 text-sm transition-colors hover:bg-bg-modifier {captchaProvider === 'none' ? 'text-text-primary' : 'text-text-secondary'}">
					<input type="radio" bind:group={captchaProvider} value="none" class="accent-brand-500" />
					<div>
						<span class="font-medium">Disabled</span>
						<p class="text-xs text-text-muted">No CAPTCHA verification on registration.</p>
					</div>
				</label>
				<label class="flex items-center gap-3 rounded p-2 text-sm transition-colors hover:bg-bg-modifier {captchaProvider === 'hcaptcha' ? 'text-text-primary' : 'text-text-secondary'}">
					<input type="radio" bind:group={captchaProvider} value="hcaptcha" class="accent-brand-500" />
					<div>
						<span class="font-medium">hCaptcha</span>
						<p class="text-xs text-text-muted">Privacy-focused CAPTCHA. Requires an hCaptcha account.</p>
					</div>
				</label>
				<label class="flex items-center gap-3 rounded p-2 text-sm transition-colors hover:bg-bg-modifier {captchaProvider === 'recaptcha' ? 'text-text-primary' : 'text-text-secondary'}">
					<input type="radio" bind:group={captchaProvider} value="recaptcha" class="accent-brand-500" />
					<div>
						<span class="font-medium">reCAPTCHA</span>
						<p class="text-xs text-text-muted">Google reCAPTCHA v2/v3. Requires a Google reCAPTCHA account.</p>
					</div>
				</label>
			</div>
		</div>

		<!-- API Keys (shown only when a provider is selected) -->
		{#if captchaProvider !== 'none'}
			<div class="space-y-4">
				<div>
					<label for="admin-captcha-site-key" class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">Site Key</label>
					<input
						id="admin-captcha-site-key"
						type="text"
						class="input w-full font-mono text-sm"
						bind:value={captchaSiteKey}
						placeholder="Enter your site key..."
					/>
					<p class="mt-1 text-xs text-text-muted">
						The public site key for the CAPTCHA widget. This is embedded in the registration page HTML.
					</p>
				</div>
				<div>
					<label for="admin-captcha-secret-key" class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">Secret Key</label>
					<input
						id="admin-captcha-secret-key"
						type="password"
						class="input w-full font-mono text-sm"
						bind:value={captchaSecretKey}
						placeholder={captchaConfig?.secret_key ? captchaConfig.secret_key : 'Enter your secret key...'}
					/>
					<p class="mt-1 text-xs text-text-muted">
						The private secret key for server-side verification. Leave blank to keep the existing key.
					</p>
				</div>
			</div>
		{/if}

		{#if captchaConfig}
			<div class="rounded-lg bg-bg-secondary p-4">
				<h3 class="mb-2 text-xs font-bold uppercase tracking-wide text-text-muted">Current Configuration</h3>
				<div class="grid gap-2 text-sm">
					<div class="flex justify-between">
						<span class="text-text-muted">Provider</span>
						<span class="text-text-primary">{captchaConfig.provider === 'none' ? 'Disabled' : captchaConfig.provider}</span>
					</div>
					<div class="flex justify-between">
						<span class="text-text-muted">Site Key</span>
						<span class="text-text-primary">{captchaConfig.site_key ? captchaConfig.site_key.slice(0, 20) + '...' : 'Not set'}</span>
					</div>
					<div class="flex justify-between">
						<span class="text-text-muted">Secret Key</span>
						<span class="text-text-primary">{captchaConfig.secret_key || 'Not set'}</span>
					</div>
				</div>
			</div>
		{/if}

		<button class="btn-primary" onclick={saveCaptchaConfig} disabled={savingCaptcha}>
			{savingCaptcha ? 'Saving...' : 'Save CAPTCHA Settings'}
		</button>
	</div>
{/if}
