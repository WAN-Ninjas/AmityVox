<script lang="ts">
	import { page } from '$app/stores';

	interface WidgetData {
		id: string;
		name: string;
		description: string | null;
		icon_id: string | null;
		online_count: number;
		member_count: number;
		invite_code: string | null;
		channels: { id: string; name: string; position: number }[];
		online_members: {
			id: string;
			username: string;
			display_name: string | null;
			avatar_id: string | null;
			status: string;
		}[];
	}

	let guildId = $derived($page.params.guildId);
	let widget = $state<WidgetData | null>(null);
	let loading = $state(true);
	let error = $state('');

	const statusColors: Record<string, string> = {
		online: 'bg-green-500',
		idle: 'bg-yellow-500',
		dnd: 'bg-red-500'
	};

	$effect(() => {
		if (guildId) loadWidget(guildId);
	});

	async function loadWidget(id: string) {
		loading = true;
		error = '';
		try {
			const resp = await fetch(`/api/v1/guilds/${id}/widget.json`);
			if (!resp.ok) {
				const err = await resp.json();
				throw new Error(err?.error?.message || 'Widget not available');
			}
			const data = await resp.json();
			widget = data.data;
		} catch (err: any) {
			error = err.message || 'Failed to load widget';
		} finally {
			loading = false;
		}
	}

	function getAvatarUrl(avatarId: string | null): string {
		if (!avatarId) return '';
		return `/api/v1/files/${avatarId}`;
	}

	function getInviteUrl(code: string): string {
		return `${location.origin}/invite/${code}`;
	}
</script>

<svelte:head>
	<title>{widget?.name || 'Server Widget'} - AmityVox</title>
</svelte:head>

<div class="flex min-h-screen items-center justify-center bg-transparent p-4">
	{#if loading}
		<div class="flex flex-col items-center gap-3">
			<span class="inline-block h-8 w-8 animate-spin rounded-full border-3 border-brand-500 border-t-transparent"></span>
			<p class="text-sm text-text-muted">Loading widget...</p>
		</div>
	{:else if error}
		<div class="w-full max-w-sm rounded-lg bg-bg-secondary p-6 text-center shadow-lg">
			<svg class="mx-auto h-12 w-12 text-text-muted" fill="none" stroke="currentColor" stroke-width="1.5" viewBox="0 0 24 24">
				<path d="M12 9v3.75m9-.75a9 9 0 11-18 0 9 9 0 0118 0zm-9 3.75h.008v.008H12v-.008z" />
			</svg>
			<p class="mt-3 text-sm text-text-muted">{error}</p>
		</div>
	{:else if widget}
		<div class="w-full max-w-sm overflow-hidden rounded-lg bg-bg-secondary shadow-xl">
			<!-- Header -->
			<div class="bg-brand-500/10 p-4">
				<div class="flex items-center gap-3">
					{#if widget.icon_id}
						<img
							src={getAvatarUrl(widget.icon_id)}
							alt=""
							class="h-12 w-12 rounded-full"
						/>
					{:else}
						<div class="flex h-12 w-12 items-center justify-center rounded-full bg-brand-500 text-lg font-bold text-white">
							{widget.name.charAt(0).toUpperCase()}
						</div>
					{/if}
					<div>
						<h2 class="text-lg font-bold text-text-primary">{widget.name}</h2>
						{#if widget.description}
							<p class="text-xs text-text-muted line-clamp-1">{widget.description}</p>
						{/if}
					</div>
				</div>

				<!-- Stats -->
				<div class="mt-3 flex items-center gap-4">
					<div class="flex items-center gap-1.5">
						<span class="h-2.5 w-2.5 rounded-full bg-green-500"></span>
						<span class="text-xs font-medium text-text-secondary">{widget.online_count} Online</span>
					</div>
					<div class="flex items-center gap-1.5">
						<span class="h-2.5 w-2.5 rounded-full bg-text-muted"></span>
						<span class="text-xs font-medium text-text-secondary">{widget.member_count} Members</span>
					</div>
				</div>
			</div>

			<!-- Channels -->
			{#if widget.channels && widget.channels.length > 0}
				<div class="border-b border-bg-modifier px-4 py-3">
					<h3 class="mb-2 text-xs font-bold uppercase tracking-wide text-text-muted">Channels</h3>
					<div class="flex flex-col gap-0.5">
						{#each widget.channels.slice(0, 10) as channel}
							<div class="flex items-center gap-2 rounded px-2 py-1 text-sm text-text-secondary hover:bg-bg-modifier">
								<span class="text-text-muted">#</span>
								<span class="truncate">{channel.name}</span>
							</div>
						{/each}
					</div>
				</div>
			{/if}

			<!-- Online Members -->
			{#if widget.online_members && widget.online_members.length > 0}
				<div class="max-h-48 overflow-y-auto px-4 py-3">
					<h3 class="mb-2 text-xs font-bold uppercase tracking-wide text-text-muted">
						Online - {widget.online_count}
					</h3>
					<div class="flex flex-col gap-1">
						{#each widget.online_members.slice(0, 30) as member}
							<div class="flex items-center gap-2 rounded px-2 py-1 hover:bg-bg-modifier">
								<div class="relative">
									{#if member.avatar_id}
										<img
											src={getAvatarUrl(member.avatar_id)}
											alt=""
											class="h-6 w-6 rounded-full"
										/>
									{:else}
										<div class="flex h-6 w-6 items-center justify-center rounded-full bg-brand-500 text-xs font-bold text-white">
											{member.username.charAt(0).toUpperCase()}
										</div>
									{/if}
									<span class="absolute -bottom-0.5 -right-0.5 h-2.5 w-2.5 rounded-full border-2 border-bg-secondary {statusColors[member.status] || 'bg-gray-500'}"></span>
								</div>
								<span class="truncate text-sm text-text-secondary">
									{member.display_name || member.username}
								</span>
							</div>
						{/each}
					</div>
				</div>
			{/if}

			<!-- Join button -->
			{#if widget.invite_code}
				<div class="border-t border-bg-modifier p-4">
					<a
						href={getInviteUrl(widget.invite_code)}
						target="_blank"
						rel="noopener"
						class="flex w-full items-center justify-center gap-2 rounded-md bg-brand-500 px-4 py-2.5 text-sm font-medium text-white transition-colors hover:bg-brand-600"
					>
						<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
							<path d="M18 9v3m0 0v3m0-3h3m-3 0h-3m-2-5a4 4 0 11-8 0 4 4 0 018 0zM3 20a6 6 0 0112 0v1H3v-1z" />
						</svg>
						Join Server
					</a>
				</div>
			{/if}

			<!-- Branding -->
			<div class="border-t border-bg-modifier px-4 py-2">
				<p class="text-center text-xs text-text-muted">
					Powered by <a href="https://amityvox.chat" target="_blank" rel="noopener" class="text-brand-400 hover:underline">AmityVox</a>
				</p>
			</div>
		</div>
	{/if}
</div>

<style>
	:global(body) {
		background: transparent;
	}
</style>
