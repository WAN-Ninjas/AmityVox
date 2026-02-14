<script lang="ts">
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { api } from '$lib/api/client';
	import { addToast } from '$lib/stores/toast';
	import { updateGuild } from '$lib/stores/guilds';

	let guildName = $state('');
	let memberCount = $state(0);
	let loading = $state(true);
	let joining = $state(false);
	let error = $state('');
	let loggedIn = $state(!!api.getToken());

	$effect(() => {
		loadInvite();
	});

	async function loadInvite() {
		const code = $page.params.code;
		if (!code) return;

		loading = true;
		error = '';

		if (!loggedIn) {
			loading = false;
			return;
		}

		try {
			const data = await api.getInvite(code);
			// The API returns { invite, guild_name, member_count } wrapped in data envelope
			// but api.get() already unwraps the data envelope
			guildName = (data as any).guild_name ?? 'Unknown Server';
			memberCount = (data as any).member_count ?? 0;
		} catch (err: any) {
			error = err.message || 'Invite not found or has expired';
		} finally {
			loading = false;
		}
	}

	async function acceptInvite() {
		const code = $page.params.code;
		if (!code) return;

		joining = true;
		try {
			const guild = await api.acceptInvite(code);
			updateGuild(guild);
			addToast(`Joined ${guildName || 'server'}!`, 'success');
			goto(`/app/guilds/${guild.id}`);
		} catch (err: any) {
			if (err.message?.includes('already a member')) {
				addToast('You are already a member of this server', 'info');
				goto('/app');
			} else {
				error = err.message || 'Failed to join server';
			}
		} finally {
			joining = false;
		}
	}

	function goToLogin() {
		// Store invite URL to redirect back after login
		const returnUrl = `/invite/${$page.params.code}`;
		goto(`/login?redirect=${encodeURIComponent(returnUrl)}`);
	}
</script>

<svelte:head>
	<title>{guildName ? `Join ${guildName}` : 'Server Invite'} — AmityVox</title>
</svelte:head>

<div class="flex min-h-screen items-center justify-center bg-bg-primary p-4">
	<div class="w-full max-w-md rounded-xl bg-bg-secondary p-8 shadow-lg">
		{#if loading}
			<div class="flex flex-col items-center gap-4">
				<svg class="h-8 w-8 animate-spin text-brand-400" fill="none" viewBox="0 0 24 24">
					<circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
					<path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
				</svg>
				<p class="text-text-muted">Loading invite...</p>
			</div>
		{:else if !loggedIn}
			<div class="flex flex-col items-center gap-4 text-center">
				<div class="flex h-16 w-16 items-center justify-center rounded-full bg-brand-500/20">
					<svg class="h-8 w-8 text-brand-400" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
						<path d="M18 9v3m0 0v3m0-3h3m-3 0h-3m-2-5a4 4 0 11-8 0 4 4 0 018 0zM3 20a6 6 0 0112 0v1H3v-1z" />
					</svg>
				</div>
				<h1 class="text-xl font-bold text-text-primary">You've been invited!</h1>
				<p class="text-text-muted">Log in or create an account to accept this invite.</p>
				<div class="flex w-full flex-col gap-2">
					<button
						onclick={goToLogin}
						class="w-full rounded-lg bg-brand-500 px-4 py-2.5 font-medium text-white hover:bg-brand-600"
					>
						Log In
					</button>
					<a
						href="/register?redirect={encodeURIComponent(`/invite/${$page.params.code}`)}"
						class="w-full rounded-lg border border-bg-floating px-4 py-2.5 text-center font-medium text-text-primary hover:bg-bg-modifier"
					>
						Create Account
					</a>
				</div>
			</div>
		{:else if error}
			<div class="flex flex-col items-center gap-4 text-center">
				<div class="flex h-16 w-16 items-center justify-center rounded-full bg-red-500/20">
					<svg class="h-8 w-8 text-red-400" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
						<path d="M6 18L18 6M6 6l12 12" />
					</svg>
				</div>
				<h1 class="text-xl font-bold text-text-primary">Invalid Invite</h1>
				<p class="text-text-muted">{error}</p>
				<a
					href="/app"
					class="rounded-lg bg-bg-modifier px-6 py-2.5 font-medium text-text-primary hover:bg-bg-floating"
				>
					Go Home
				</a>
			</div>
		{:else}
			<div class="flex flex-col items-center gap-4 text-center">
				<div class="flex h-16 w-16 items-center justify-center rounded-full bg-brand-500/20">
					<svg class="h-8 w-8 text-brand-400" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
						<path d="M17 20h5v-2a3 3 0 00-5.356-1.857M17 20H7m10 0v-2c0-.656-.126-1.283-.356-1.857M7 20H2v-2a3 3 0 015.356-1.857M7 20v-2c0-.656.126-1.283.356-1.857m0 0a5.002 5.002 0 019.288 0M15 7a3 3 0 11-6 0 3 3 0 016 0z" />
					</svg>
				</div>
				<h1 class="text-xl font-bold text-text-primary">You've been invited to join</h1>
				<p class="text-2xl font-bold text-brand-400">{guildName}</p>
				<p class="text-sm text-text-muted">{memberCount} {memberCount === 1 ? 'member' : 'members'}</p>
				<button
					onclick={acceptInvite}
					disabled={joining}
					class="w-full rounded-lg bg-brand-500 px-6 py-2.5 font-medium text-white hover:bg-brand-600 disabled:opacity-50"
				>
					{#if joining}
						<span class="flex items-center justify-center gap-2">
							<svg class="h-4 w-4 animate-spin" fill="none" viewBox="0 0 24 24">
								<circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
								<path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
							</svg>
							Joining...
						</span>
					{:else}
						Accept Invite
					{/if}
				</button>
			</div>
		{/if}
	</div>
</div>
