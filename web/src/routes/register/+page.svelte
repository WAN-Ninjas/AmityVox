<script lang="ts">
	import { goto } from '$app/navigation';
	import { register } from '$lib/stores/auth';

	let username = $state('');
	let email = $state('');
	let password = $state('');
	let confirmPassword = $state('');
	let error = $state('');
	let loading = $state(false);

	async function handleSubmit(e: Event) {
		e.preventDefault();
		error = '';

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
			await register(username, email, password);
			goto('/app');
		} catch (err: any) {
			error = err.message || 'Registration failed';
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
			<h1 class="mb-2 text-center text-2xl font-bold text-text-primary">Create an account</h1>
			<p class="mb-6 text-center text-sm text-text-muted">Join the conversation</p>

			{#if error}
				<div class="mb-4 rounded bg-red-500/10 px-3 py-2 text-sm text-red-400">{error}</div>
			{/if}

			<form onsubmit={handleSubmit}>
				<div class="mb-4">
					<label for="username" class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">
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
					<label for="email" class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">
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
					<label for="password" class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">
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
					<label for="confirmPassword" class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">
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

				<button type="submit" class="btn-primary w-full" disabled={loading}>
					{loading ? 'Creating account...' : 'Register'}
				</button>
			</form>

			<p class="mt-4 text-sm text-text-muted">
				Already have an account?
				<a href="/login" class="text-text-link hover:underline">Log in</a>
			</p>
		</div>
	</div>
</div>
