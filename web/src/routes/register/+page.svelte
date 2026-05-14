<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/stores';
	import { api, ApiRequestError } from '$lib/api/client';
	import { register } from '$lib/stores/auth';
	import type { RegistrationSettings } from '$lib/types';

	let username = $state('');
	let email = $state('');
	let password = $state('');
	let confirmPassword = $state('');
	let registrationToken = $state('');
	let error = $state('');
	let notice = $state('');
	let loading = $state(false);
	let tokenFromUrl = $state(false);
	let loadedUrlToken = $state('');
	let registrationSettings = $state<RegistrationSettings | null>(null);
	let registrationSettingsLoaded = $state(false);
	const redirectUrl = $derived($page.url.searchParams.get('redirect') || '/app');
	const registrationClosed = $derived(registrationSettings?.mode === 'closed');
	const tokenRequired = $derived(registrationSettings?.mode === 'invite_only');
	const canSubmit = $derived(!registrationClosed && (!tokenRequired || Boolean(registrationToken.trim())));
	const loginUrl = $derived.by(() => {
		const params = new URLSearchParams();
		const redirect = $page.url.searchParams.get('redirect');
		const token = $page.url.searchParams.get('token') || $page.url.searchParams.get('registration_token');
		if (redirect) params.set('redirect', redirect);
		if (token) params.set('token', token);
		const qs = params.toString();
		return `/login${qs ? `?${qs}` : ''}`;
	});

	$effect(() => {
		const token = $page.url.searchParams.get('token') || $page.url.searchParams.get('registration_token') || '';
		if (token && token !== loadedUrlToken) {
			registrationToken = token;
			loadedUrlToken = token;
			tokenFromUrl = true;
			notice = 'Invitation token loaded from your link.';
		}
	});

	onMount(() => {
		api.getPublicRegistrationSettings()
			.then((settings) => {
				registrationSettings = settings;
				if (settings.message && (settings.mode === 'closed' || settings.mode === 'invite_only')) {
					notice = settings.message;
				}
			})
			.catch(() => {
				registrationSettings = null;
			})
			.finally(() => {
				registrationSettingsLoaded = true;
			});
	});

	async function handleSubmit(e: Event) {
		e.preventDefault();
		error = '';

		if (registrationClosed) {
			error = registrationSettings?.message || 'Registration is currently closed on this instance.';
			return;
		}
		if (tokenRequired && !registrationToken.trim()) {
			error = 'This instance is invite-only. Enter a registration token to create an account.';
			return;
		}
		if (password !== confirmPassword) {
			error = 'Passwords do not match';
			return;
		}
		if (password.length < 8) {
			error = 'Password must be at least 8 characters';
			return;
		}

		loading = true;

		try {
			await register(username, email, password, registrationToken.trim() || undefined);
			goto(redirectUrl);
		} catch (err: any) {
			if (err instanceof ApiRequestError) {
				switch (err.code) {
					case 'token_required':
						error = 'This instance is invite-only. Enter a registration token to create an account.';
						break;
					case 'invalid_token':
						error = 'That registration token is invalid or expired.';
						break;
					case 'registration_closed':
					case 'registration_disabled':
						error = err.message || 'Registration is currently closed on this instance.';
						break;
					default:
						error = err.message || 'Registration failed';
				}
			} else {
				error = err.message || 'Registration failed';
			}
		} finally {
			loading = false;
		}
	}
</script>

<svelte:head>
	<title>Register — AmityVox</title>
</svelte:head>

<div class="flex min-h-screen items-center justify-center bg-bg-primary p-4">
	<div class="w-full max-w-sm">
		<div class="rounded-lg bg-bg-secondary p-8 shadow-xl">
			<div class="mb-4 flex justify-center">
				<img src="/logo.png" alt="AmityVox" class="h-16 w-16" />
			</div>
			<h1 class="mb-2 text-center text-2xl font-bold text-text-primary">Create an account</h1>
			<p class="mb-6 text-center text-sm text-text-muted">Join the conversation</p>

			{#if registrationSettingsLoaded && registrationClosed}
				<div class="mb-4 rounded bg-red-500/10 px-3 py-2 text-sm text-red-400">
					{registrationSettings?.message || 'Registration is currently closed on this instance.'}
				</div>
			{:else if registrationSettingsLoaded && tokenRequired && !registrationToken.trim()}
				<div class="mb-4 rounded bg-yellow-500/10 px-3 py-2 text-sm text-yellow-300">
					This instance is invite-only. Enter a registration token or use an invite link.
				</div>
			{/if}

			{#if error}
				<div class="mb-4 rounded bg-red-500/10 px-3 py-2 text-sm text-red-400">{error}</div>
			{/if}
			{#if notice}
				<div class="mb-4 rounded bg-brand-500/10 px-3 py-2 text-sm text-brand-300">{notice}</div>
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
						required
						class="input w-full"
						autocomplete="username"
						minlength="2"
						maxlength="32"
					/>
				</div>

				<div class="mb-4">
					<label for="email" class="mb-2 block font-mono text-xs font-bold uppercase tracking-wide text-text-muted">
						Email
					</label>
					<input
						id="email"
						type="email"
						bind:value={email}
						required
						class="input w-full"
						autocomplete="email"
					/>
				</div>

				<div class="mb-4">
					<label for="password" class="mb-2 block font-mono text-xs font-bold uppercase tracking-wide text-text-muted">
						Password
					</label>
					<input
						id="password"
						type="password"
						bind:value={password}
						required
						class="input w-full"
						autocomplete="new-password"
						minlength="8"
					/>
				</div>

				<div class="mb-6">
					<label for="confirmPassword" class="mb-2 block font-mono text-xs font-bold uppercase tracking-wide text-text-muted">
						Confirm Password
					</label>
					<input
						id="confirmPassword"
						type="password"
						bind:value={confirmPassword}
						required
						class="input w-full"
						autocomplete="new-password"
					/>
				</div>

				<div class="mb-6">
					<label for="registrationToken" class="mb-2 block font-mono text-xs font-bold uppercase tracking-wide text-text-muted">
						Registration Token{tokenRequired ? '' : ' (optional)'}
					</label>
					<input
						id="registrationToken"
						type="text"
						bind:value={registrationToken}
						class="input w-full"
						autocomplete="off"
						required={tokenRequired}
						placeholder={tokenRequired ? 'Required for this invite-only instance' : 'Optional invitation token'}
					/>
					<p class="mt-2 text-xs text-text-muted">
						{tokenFromUrl ? 'This came from your invite link. You can replace it if needed.' : 'Leave blank unless this instance requires an invitation.'}
					</p>
				</div>

				<button type="submit" class="btn-primary w-full" disabled={loading || !canSubmit}>
					{loading ? 'Creating account...' : 'Register'}
				</button>
			</form>

			<p class="mt-4 text-sm text-text-muted">
				Already have an account?
				<a href={loginUrl} class="text-text-link hover:underline">Log in</a>
			</p>
		</div>
	</div>
</div>
