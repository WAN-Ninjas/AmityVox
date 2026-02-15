<script lang="ts">
	import { goto } from '$app/navigation';
	import { login } from '$lib/stores/auth';

	let username = $state('');
	let password = $state('');
	let error = $state('');
	let loading = $state(false);

	async function handleSubmit(e: Event) {
		e.preventDefault();
		error = '';
		loading = true;

		try {
			await login(username, password);
			goto('/app');
		} catch (err: any) {
			error = err.message || 'Login failed';
		} finally {
			loading = false;
		}
	}
</script>

<svelte:head>
	<title>Login — AmityVox</title>
</svelte:head>

<div class="flex min-h-screen items-center justify-center bg-bg-primary p-4">
	<div class="w-full max-w-sm">
		<div class="rounded border-t-2 border-brand-500 bg-bg-secondary p-8 shadow-xl">
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
						required
						class="input w-full"
						autocomplete="current-password"
					/>
				</div>

				<button type="submit" class="btn-primary w-full" disabled={loading}>
					{loading ? 'Logging in...' : 'Log In'}
				</button>
			</form>

			<p class="mt-4 text-sm text-text-muted">
				Need an account?
				<a href="/register" class="text-text-link hover:underline">Register</a>
			</p>
		</div>
	</div>
</div>
