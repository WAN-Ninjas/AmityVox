<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/stores';
	import { api, ApiRequestError } from '$lib/api/client';
	import { login } from '$lib/stores/auth';
	import type { RegistrationSettings } from '$lib/types';

	let username = $state('');
	let password = $state('');
	let totpCode = $state('');
	let totpRequired = $state(false);
	let error = $state('');
	let loading = $state(false);
	let registrationSettings = $state<RegistrationSettings | null>(null);
	let registrationSettingsLoaded = $state(false);
	const inviteToken = $derived($page.url.searchParams.get('token') || $page.url.searchParams.get('registration_token') || '');
	const canRegister = $derived(
		registrationSettings?.mode === 'open' ||
		(registrationSettings?.mode === 'invite_only' && Boolean(inviteToken))
	);
	const registrationMessage = $derived.by(() => {
		if (!registrationSettingsLoaded || !registrationSettings || canRegister) return '';
		if (registrationSettings.message) return registrationSettings.message;
		if (registrationSettings.mode === 'closed') return 'Registration is currently closed on this instance.';
		return 'Registration is invite-only. Use an invite link to create an account.';
	});
	const registerUrl = $derived.by(() => {
		const params = new URLSearchParams();
		const redirect = $page.url.searchParams.get('redirect');
		const token = inviteToken;
		if (redirect) params.set('redirect', redirect);
		if (token) params.set('token', token);
		const qs = params.toString();
		return `/register${qs ? `?${qs}` : ''}`;
	});
	const redirectUrl = $derived($page.url.searchParams.get('redirect') || '/app');

	async function handleSubmit(e: Event) {
		e.preventDefault();
		error = '';
		loading = true;

		try {
			await login(username, password, totpRequired ? totpCode.trim() : undefined);
			goto(redirectUrl);
		} catch (err: any) {
			if (err instanceof ApiRequestError && err.code === 'totp_required') {
				totpRequired = true;
				error = '';
			} else if (err instanceof ApiRequestError && err.code === 'invalid_totp') {
				error = 'Invalid two-factor authentication code';
				totpCode = '';
			} else {
				error = err.message || 'Login failed';
			}
		} finally {
			loading = false;
		}
	}

	function handleCredentialsChange() {
		totpRequired = false;
		totpCode = '';
		error = '';
	}

	onMount(() => {
		api.getPublicRegistrationSettings()
			.then((settings) => {
				registrationSettings = settings;
			})
			.catch(() => {
				registrationSettings = null;
			})
			.finally(() => {
				registrationSettingsLoaded = true;
			});
	});
</script>

<svelte:head>
	<title>Login — AmityVox</title>
</svelte:head>

<div class="flex min-h-screen items-center justify-center bg-bg-primary p-4">
	<div class="w-full max-w-sm">
		<div class="rounded border-t-2 border-brand-500 bg-bg-secondary p-8 shadow-xl">
			<div class="mb-4 flex justify-center">
				<img src="/logo.png" alt="AmityVox" class="h-16 w-16" />
			</div>
			<h1 class="mb-2 text-center text-2xl font-bold text-text-primary">Welcome back!</h1>
			<p class="mb-6 text-center text-sm text-text-muted">We're so excited to see you again!</p>

			{#if error}
				<div class="mb-4 rounded bg-red-500/10 px-3 py-2 text-sm text-red-400">{error}</div>
			{/if}

			<form onsubmit={handleSubmit}>
				<div class="mb-4">
					<label for="username" class="mb-2 block font-mono text-xs font-bold uppercase tracking-wide text-text-muted">
						Username
					</label>
					<input
						id="username"
						type="text"
						bind:value={username}
						oninput={handleCredentialsChange}
						required
						class="input w-full"
						autocomplete="username"
					/>
				</div>

				<div class="mb-6">
					<label for="password" class="mb-2 block font-mono text-xs font-bold uppercase tracking-wide text-text-muted">
						Password
					</label>
					<input
						id="password"
						type="password"
						bind:value={password}
						oninput={handleCredentialsChange}
						required
						class="input w-full"
						autocomplete="current-password"
					/>
				</div>

				{#if totpRequired}
					<div class="mb-6">
						<label for="totpCode" class="mb-2 block font-mono text-xs font-bold uppercase tracking-wide text-text-muted">
							Two-factor code
						</label>
						<input
							id="totpCode"
							type="text"
							inputmode="numeric"
							pattern="[0-9]*"
							bind:value={totpCode}
							required
							class="input w-full"
							autocomplete="one-time-code"
							minlength="6"
							maxlength="6"
						/>
						<p class="mt-2 text-xs text-text-muted">Enter the 6-digit code from your authenticator app.</p>
					</div>
				{/if}

				<button type="submit" class="btn-primary w-full" disabled={loading}>
					{loading ? 'Logging in...' : totpRequired ? 'Verify and Log In' : 'Log In'}
				</button>
			</form>

			{#if canRegister}
				<p class="mt-4 text-sm text-text-muted">
					Need an account?
					<a href={registerUrl} class="text-text-link hover:underline">Register</a>
				</p>
			{:else if registrationMessage}
				<p class="mt-4 rounded bg-bg-primary px-3 py-2 text-sm text-text-muted">{registrationMessage}</p>
			{/if}
		</div>
	</div>
</div>
