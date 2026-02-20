<script lang="ts">
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { api } from '$lib/api/client';
	import { addToast } from '$lib/stores/toast';
	import { updateGuild } from '$lib/stores/guilds';
	import FederationBadge from '$lib/components/common/FederationBadge.svelte';

	let guildName = $state('');
	let guildId = $state('');
	let memberCount = $state(0);
	let loading = $state(true);
	let joining = $state(false);
	let error = $state('');
	let loggedIn = $state(!!api.getToken());

	// Federation state
	let isFederated = $state(false);
	let instanceDomain = $state('');
	let inviteCode = $state('');
	let notFederated = $state(false);
	let notFederatedDomain = $state('');
	let requestingFederation = $state(false);
	let federationRequested = $state(false);

	function extractDomainFromInvite(code: string): string {
		if (code.includes('@')) return code.split('@').pop() ?? '';
		const slash = code.indexOf('/');
		if (slash > 0) return code.slice(0, slash);
		return '';
	}

	$effect(() => {
		loadInvite();
	});

	async function loadInvite() {
		const code = $page.params.code;
		if (!code) return;

		loading = true;
		error = '';
		isFederated = false;
		notFederated = false;

		if (!loggedIn) {
			loading = false;
			return;
		}

		try {
			const data = await api.getInvite(code);

			if ((data as any).federated) {
				isFederated = true;
				instanceDomain = (data as any).instance_domain ?? '';
				inviteCode = (data as any).invite_code ?? code;
				guildName = (data as any).guild_name ?? `Server on ${instanceDomain}`;
				memberCount = (data as any).member_count ?? 0;
			} else {
				guildName = (data as any).guild_name ?? 'Unknown Server';
				guildId = (data as any).guild_id ?? '';
				memberCount = (data as any).member_count ?? 0;
			}
		} catch (err: any) {
			// Check for not_federated error
			if (err.code === 'not_federated' || err.message?.includes('not federated')) {
				notFederated = true;
				notFederatedDomain = err.domain || extractDomainFromInvite(code) || '';
			} else {
				error = err.message || 'Invite not found or has expired';
			}
		} finally {
			loading = false;
		}
	}

	async function acceptInvite() {
		const code = $page.params.code;
		if (!code) return;

		joining = true;
		try {
			if (isFederated) {
				await api.joinFederatedGuild(instanceDomain, undefined, inviteCode);
				addToast(`Joined ${guildName}!`, 'success');
				goto('/app');
			} else {
				const guild = await api.acceptInvite(code);

				if ((guild as any).federated) {
					// Server returned redirect to federation
					await api.joinFederatedGuild(
						(guild as any).instance_domain,
						undefined,
						(guild as any).invite_code
					);
					addToast(`Joined server on ${(guild as any).instance_domain}!`, 'success');
					goto('/app');
				} else {
					updateGuild(guild);
					addToast(`Joined ${guildName || 'server'}!`, 'success');
					goto(`/app/guilds/${guild.id}`);
				}
			}
		} catch (err: any) {
			if (err.message?.includes('already a member')) {
				addToast('You are already a member of this server', 'info');
				goto(guildId ? `/app/guilds/${guildId}` : '/app', { replaceState: true });
			} else {
				error = err.message || 'Failed to join server';
			}
		} finally {
			joining = false;
		}
	}

	async function requestFederation() {
		if (!notFederatedDomain) return;
		requestingFederation = true;
		try {
			await api.createIssue(
				`Federation request: ${notFederatedDomain}`,
				`A user attempted to accept an invite from ${notFederatedDomain} but this instance is not federated with it. Please review adding ${notFederatedDomain} to the federation peers list.`,
				'suggestion'
			);
			federationRequested = true;
			addToast('Federation request submitted!', 'success');
		} catch (err: any) {
			addToast(err.message || 'Failed to submit request', 'error');
		} finally {
			requestingFederation = false;
		}
	}

	function goToLogin() {
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
		{:else if notFederated}
			<!-- Not Federated Dialog -->
			<div class="flex flex-col items-center gap-4 text-center">
				<div class="flex h-16 w-16 items-center justify-center rounded-full bg-yellow-500/20">
					<svg class="h-8 w-8 text-yellow-400" viewBox="0 0 16 16" fill="currentColor">
						<path d="M8 0a8 8 0 100 16A8 8 0 008 0zm5.3 5H11a13 13 0 00-1-3.3A6 6 0 0113.3 5zM8 1.5c.7.8 1.3 2 1.7 3.5H6.3C6.7 3.5 7.3 2.3 8 1.5zM1.5 9a6.5 6.5 0 010-2h2.8a13 13 0 000 2H1.5zm.2 1h2.5a13 13 0 001 3.3A6 6 0 011.7 10zm2.5-5H1.7A6 6 0 016 1.7 13 13 0 004.2 5zM8 14.5c-.7-.8-1.3-2-1.7-3.5h3.4c-.4 1.5-1 2.7-1.7 3.5zm2-4.5H6a12 12 0 010-4h4a12 12 0 010 4zm.1 3.3a13 13 0 001-3.3h2.5a6 6 0 01-3.5 3.3zM11.7 9a13 13 0 000-2h2.8a6.5 6.5 0 010 2h-2.8z"/>
					</svg>
				</div>
				<h1 class="text-xl font-bold text-text-primary">Not Federated</h1>
				<p class="text-text-muted">
					This instance is not federated with <span class="font-medium text-text-secondary">{notFederatedDomain}</span>.
					You cannot accept this invite until federation is established.
				</p>
				{#if federationRequested}
					<p class="text-sm text-green-400">Federation request submitted! An admin will review it.</p>
				{:else}
					<button
						onclick={requestFederation}
						disabled={requestingFederation}
						class="w-full rounded-lg bg-yellow-500 px-4 py-2.5 font-medium text-white hover:bg-yellow-600 disabled:opacity-50"
					>
						{requestingFederation ? 'Submitting...' : 'Request Federation'}
					</button>
				{/if}
				<a
					href="/app"
					class="rounded-lg bg-bg-modifier px-6 py-2.5 font-medium text-text-primary hover:bg-bg-floating"
				>
					Go Home
				</a>
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
				<div class="flex h-16 w-16 items-center justify-center rounded-full {isFederated ? 'bg-blue-500/20' : 'bg-brand-500/20'}">
					{#if isFederated}
						<svg class="h-8 w-8 text-blue-400" viewBox="0 0 16 16" fill="currentColor">
							<path d="M8 0a8 8 0 100 16A8 8 0 008 0zm5.3 5H11a13 13 0 00-1-3.3A6 6 0 0113.3 5zM8 1.5c.7.8 1.3 2 1.7 3.5H6.3C6.7 3.5 7.3 2.3 8 1.5zM1.5 9a6.5 6.5 0 010-2h2.8a13 13 0 000 2H1.5zm.2 1h2.5a13 13 0 001 3.3A6 6 0 011.7 10zm2.5-5H1.7A6 6 0 016 1.7 13 13 0 004.2 5zM8 14.5c-.7-.8-1.3-2-1.7-3.5h3.4c-.4 1.5-1 2.7-1.7 3.5zm2-4.5H6a12 12 0 010-4h4a12 12 0 010 4zm.1 3.3a13 13 0 001-3.3h2.5a6 6 0 01-3.5 3.3zM11.7 9a13 13 0 000-2h2.8a6.5 6.5 0 010 2h-2.8z"/>
						</svg>
					{:else}
						<svg class="h-8 w-8 text-brand-400" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
							<path d="M17 20h5v-2a3 3 0 00-5.356-1.857M17 20H7m10 0v-2c0-.656-.126-1.283-.356-1.857M7 20H2v-2a3 3 0 015.356-1.857M7 20v-2c0-.656.126-1.283.356-1.857m0 0a5.002 5.002 0 019.288 0M15 7a3 3 0 11-6 0 3 3 0 016 0z" />
						</svg>
					{/if}
				</div>
				<h1 class="text-xl font-bold text-text-primary">You've been invited to join</h1>
				<p class="text-2xl font-bold {isFederated ? 'text-blue-400' : 'text-brand-400'}">{guildName}</p>
				{#if isFederated && instanceDomain}
					<FederationBadge domain={instanceDomain} />
				{/if}
				<p class="text-sm text-text-muted">{memberCount} {memberCount === 1 ? 'member' : 'members'}</p>
				<button
					onclick={acceptInvite}
					disabled={joining}
					class="w-full rounded-lg {isFederated ? 'bg-blue-500 hover:bg-blue-600' : 'bg-brand-500 hover:bg-brand-600'} px-6 py-2.5 font-medium text-white disabled:opacity-50"
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
