<script lang="ts">
	import { api } from '$lib/api/client';
	import { confirmAction } from '$lib/stores/confirm';
	import type { BotToken, SlashCommand, User } from '$lib/types';

	let myBots = $state<User[]>([]);
	let loadingBots = $state(false);
	let newBotName = $state('');
	let newBotDescription = $state('');
	let creatingBot = $state(false);
	let botError = $state('');
	let botSuccess = $state('');
	let expandedBotId = $state<string | null>(null);
	let botTokens = $state<Record<string, BotToken[]>>({});
	let botCommands = $state<Record<string, SlashCommand[]>>({});
	let loadingBotTokens = $state<string | null>(null);
	let loadingBotCommands = $state<string | null>(null);
	let newTokenName = $state('');
	let createdTokenRaw = $state<string | null>(null);
	let creatingToken = $state(false);
	let editingBotId = $state<string | null>(null);
	let editBotName = $state('');
	let editBotDescription = $state('');
	let savingBot = $state(false);
	let newCommandName = $state('');
	let newCommandDescription = $state('');
	let creatingCommand = $state(false);
	let loaded = false;

	$effect(() => {
		if (!loaded) {
			loaded = true;
			loadBots();
		}
	});

	async function loadBots() {
		loadingBots = true;
		botError = '';
		try {
			myBots = await api.getMyBots();
		} catch (err: any) {
			botError = err.message || 'Failed to load bots';
			myBots = [];
		} finally {
			loadingBots = false;
		}
	}

	async function handleCreateBot() {
		if (!newBotName.trim()) return;
		creatingBot = true;
		botError = '';
		botSuccess = '';
		try {
			const bot = await api.createBot(newBotName.trim(), newBotDescription.trim() || undefined);
			myBots = [bot, ...myBots];
			newBotName = '';
			newBotDescription = '';
			botSuccess = `Bot "${bot.username}" created!`;
			setTimeout(() => (botSuccess = ''), 3000);
		} catch (err: any) {
			botError = err.message || 'Failed to create bot';
		} finally {
			creatingBot = false;
		}
	}

	async function handleDeleteBot(botId: string) {
		if (!(await confirmAction({ title: 'Delete Bot', message: 'Are you sure you want to delete this bot? This cannot be undone.', confirmLabel: 'Delete Bot' }))) return;
		try {
			await api.deleteBot(botId);
			myBots = myBots.filter((bot) => bot.id !== botId);
			if (expandedBotId === botId) expandedBotId = null;
			botSuccess = 'Bot deleted.';
			setTimeout(() => (botSuccess = ''), 3000);
		} catch (err: any) {
			botError = err.message || 'Failed to delete bot';
		}
	}

	function startEditBot(bot: User) {
		editingBotId = bot.id;
		editBotName = bot.username;
		editBotDescription = bot.display_name ?? '';
	}

	function cancelEditBot() {
		editingBotId = null;
		editBotName = '';
		editBotDescription = '';
	}

	async function handleSaveBot() {
		if (!editingBotId || !editBotName.trim()) return;
		savingBot = true;
		botError = '';
		try {
			const updated = await api.updateBot(editingBotId, {
				name: editBotName.trim(),
				description: editBotDescription.trim() || undefined
			});
			myBots = myBots.map((bot) => bot.id === updated.id ? updated : bot);
			editingBotId = null;
			botSuccess = 'Bot updated.';
			setTimeout(() => (botSuccess = ''), 3000);
		} catch (err: any) {
			botError = err.message || 'Failed to update bot';
		} finally {
			savingBot = false;
		}
	}

	async function toggleBotExpand(botId: string) {
		if (expandedBotId === botId) {
			expandedBotId = null;
			return;
		}

		expandedBotId = botId;
		createdTokenRaw = null;
		if (!botTokens[botId]) {
			loadingBotTokens = botId;
			try {
				const tokens = await api.getBotTokens(botId);
				botTokens = { ...botTokens, [botId]: tokens };
			} catch {
				botTokens = { ...botTokens, [botId]: [] };
			} finally {
				loadingBotTokens = null;
			}
		}
		if (!botCommands[botId]) {
			loadingBotCommands = botId;
			try {
				const cmds = await api.getBotCommands(botId);
				botCommands = { ...botCommands, [botId]: cmds };
			} catch {
				botCommands = { ...botCommands, [botId]: [] };
			} finally {
				loadingBotCommands = null;
			}
		}
	}

	async function handleCreateToken(botId: string) {
		creatingToken = true;
		botError = '';
		try {
			const token = await api.createBotToken(botId, newTokenName.trim() || undefined);
			createdTokenRaw = token.token ?? null;
			botTokens = { ...botTokens, [botId]: [token, ...(botTokens[botId] ?? [])] };
			newTokenName = '';
		} catch (err: any) {
			botError = err.message || 'Failed to create token';
		} finally {
			creatingToken = false;
		}
	}

	async function handleDeleteToken(botId: string, tokenId: string) {
		try {
			await api.deleteBotToken(botId, tokenId);
			botTokens = { ...botTokens, [botId]: (botTokens[botId] ?? []).filter((token) => token.id !== tokenId) };
		} catch (err: any) {
			botError = err.message || 'Failed to delete token';
		}
	}

	async function handleRegisterCommand(botId: string) {
		if (!newCommandName.trim() || !newCommandDescription.trim()) return;
		creatingCommand = true;
		botError = '';
		try {
			const cmd = await api.registerBotCommand(botId, {
				name: newCommandName.trim().toLowerCase(),
				description: newCommandDescription.trim()
			});
			botCommands = { ...botCommands, [botId]: [...(botCommands[botId] ?? []), cmd] };
			newCommandName = '';
			newCommandDescription = '';
			botSuccess = `Command "/${cmd.name}" registered.`;
			setTimeout(() => (botSuccess = ''), 3000);
		} catch (err: any) {
			botError = err.message || 'Failed to register command';
		} finally {
			creatingCommand = false;
		}
	}

	async function handleDeleteCommand(botId: string, commandId: string) {
		try {
			await api.deleteBotCommand(botId, commandId);
			botCommands = { ...botCommands, [botId]: (botCommands[botId] ?? []).filter((cmd) => cmd.id !== commandId) };
		} catch (err: any) {
			botError = err.message || 'Failed to delete command';
		}
	}

	function copyToClipboard(text: string) {
		navigator.clipboard.writeText(text).catch(() => {});
	}
</script>

<h1 class="mb-6 text-xl font-bold text-text-primary">Bot Management</h1>

{#if botError}
	<div class="mb-4 rounded bg-red-500/10 px-3 py-2 text-sm text-red-400">{botError}</div>
{/if}
{#if botSuccess}
	<div class="mb-4 rounded bg-green-500/10 px-3 py-2 text-sm text-green-400">{botSuccess}</div>
{/if}

<div class="mb-6 rounded-lg bg-bg-secondary p-4">
	<h3 class="mb-2 text-sm font-semibold text-text-primary">Create a Bot</h3>
	<p class="mb-3 text-xs text-text-muted">Bots can interact with the API using generated tokens.</p>
	<div class="mb-3">
		<label for="newBotName" class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">
			Bot Name
		</label>
		<input
			id="newBotName"
			type="text"
			class="input w-full"
			placeholder="my-bot"
			maxlength="32"
			bind:value={newBotName}
		/>
	</div>
	<div class="mb-3">
		<label for="newBotDesc" class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">
			Description (optional)
		</label>
		<input
			id="newBotDesc"
			type="text"
			class="input w-full"
			placeholder="What does this bot do?"
			maxlength="128"
			bind:value={newBotDescription}
		/>
	</div>
	<button class="btn-primary" onclick={handleCreateBot} disabled={creatingBot || !newBotName.trim()}>
		{creatingBot ? 'Creating...' : 'Create Bot'}
	</button>
</div>

{#if loadingBots}
	<div class="flex items-center gap-2 py-4">
		<div class="h-4 w-4 animate-spin rounded-full border-2 border-brand-500 border-t-transparent"></div>
		<span class="text-sm text-text-muted">Loading bots...</span>
	</div>
{:else if myBots.length === 0}
	<div class="rounded-lg bg-bg-secondary p-6 text-center">
		<p class="text-sm text-text-muted">You have no bots yet.</p>
		<p class="text-xs text-text-muted">Create one above to get started.</p>
	</div>
{:else}
	<div class="space-y-3">
		{#each myBots as bot (bot.id)}
			<div class="rounded-lg bg-bg-secondary">
				<div class="flex items-center justify-between p-4">
					<div class="flex items-center gap-3">
						<div class="flex h-10 w-10 items-center justify-center rounded-full bg-brand-500/20 text-brand-400">
							<svg class="h-5 w-5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
								<path d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
							</svg>
						</div>
						<div>
							{#if editingBotId === bot.id}
								<div class="flex items-center gap-2">
									<input type="text" class="input w-40 text-sm" bind:value={editBotName} maxlength="32" />
									<input
										type="text"
										class="input w-48 text-sm"
										bind:value={editBotDescription}
										placeholder="Description"
										maxlength="128"
									/>
									<button class="text-xs text-brand-400 hover:text-brand-300" onclick={handleSaveBot} disabled={savingBot}>
										{savingBot ? 'Saving...' : 'Save'}
									</button>
									<button class="text-xs text-text-muted hover:text-text-primary" onclick={cancelEditBot}>
										Cancel
									</button>
								</div>
							{:else}
								<h4 class="text-sm font-semibold text-text-primary">{bot.username}</h4>
								{#if bot.display_name}
									<p class="text-xs text-text-muted">{bot.display_name}</p>
								{/if}
								<p class="font-mono text-2xs text-text-muted">{bot.id}</p>
							{/if}
						</div>
					</div>
					<div class="flex items-center gap-2">
						<button class="text-xs text-text-muted hover:text-text-primary" onclick={() => toggleBotExpand(bot.id)}>
							{expandedBotId === bot.id ? 'Collapse' : 'Expand'}
						</button>
						{#if editingBotId !== bot.id}
							<button class="text-xs text-brand-400 hover:text-brand-300" onclick={() => startEditBot(bot)}>
								Edit
							</button>
						{/if}
						<button class="text-xs text-red-400 hover:text-red-300" onclick={() => handleDeleteBot(bot.id)}>
							Delete
						</button>
					</div>
				</div>

				{#if expandedBotId === bot.id}
					<div class="border-t border-bg-modifier p-4">
						<div class="mb-6">
							<h5 class="mb-3 text-xs font-bold uppercase tracking-wide text-text-muted">API Tokens</h5>

							{#if createdTokenRaw}
								<div class="mb-3 rounded bg-green-500/10 p-3">
									<p class="mb-1 text-xs font-semibold text-green-400">Token created! Copy it now -- it will not be shown again.</p>
									<div class="flex items-center gap-2">
										<code class="flex-1 break-all rounded bg-bg-primary px-2 py-1 text-xs text-text-primary">{createdTokenRaw}</code>
										<button class="btn-secondary text-xs" onclick={() => { copyToClipboard(createdTokenRaw!); }}>
											Copy
										</button>
									</div>
								</div>
							{/if}

							<div class="mb-3 flex items-center gap-2">
								<input
									type="text"
									class="input flex-1 text-sm"
									placeholder="Token name (optional)"
									maxlength="64"
									bind:value={newTokenName}
								/>
								<button class="btn-primary text-xs" onclick={() => handleCreateToken(bot.id)} disabled={creatingToken}>
									{creatingToken ? 'Generating...' : 'Generate Token'}
								</button>
							</div>

							{#if loadingBotTokens === bot.id}
								<p class="text-xs text-text-muted">Loading tokens...</p>
							{:else if (botTokens[bot.id] ?? []).length === 0}
								<p class="text-xs text-text-muted">No tokens yet. Generate one to authenticate your bot.</p>
							{:else}
								<div class="space-y-2">
									{#each botTokens[bot.id] ?? [] as token (token.id)}
										<div class="flex items-center justify-between rounded bg-bg-primary p-2">
											<div>
												<span class="text-sm text-text-primary">{token.name}</span>
												<span class="ml-2 text-2xs text-text-muted">
													Created {new Date(token.created_at).toLocaleDateString()}
													{#if token.last_used_at}
														&middot; Last used {new Date(token.last_used_at).toLocaleDateString()}
													{/if}
												</span>
											</div>
											<button class="text-xs text-red-400 hover:text-red-300" onclick={() => handleDeleteToken(bot.id, token.id)}>
												Revoke
											</button>
										</div>
									{/each}
								</div>
							{/if}
						</div>

						<div>
							<h5 class="mb-3 text-xs font-bold uppercase tracking-wide text-text-muted">Slash Commands</h5>

							<div class="mb-3 flex items-end gap-2">
								<div class="flex-1">
									<label for="command-name-{bot.id}" class="mb-1 block text-2xs text-text-muted">Name</label>
									<input
										id="command-name-{bot.id}"
										type="text"
										class="input w-full text-sm"
										placeholder="command-name"
										maxlength="32"
										bind:value={newCommandName}
									/>
								</div>
								<div class="flex-1">
									<label for="command-description-{bot.id}" class="mb-1 block text-2xs text-text-muted">Description</label>
									<input
										id="command-description-{bot.id}"
										type="text"
										class="input w-full text-sm"
										placeholder="What does this command do?"
										maxlength="100"
										bind:value={newCommandDescription}
									/>
								</div>
								<button
									class="btn-primary text-xs"
									onclick={() => handleRegisterCommand(bot.id)}
									disabled={creatingCommand || !newCommandName.trim() || !newCommandDescription.trim()}
								>
									{creatingCommand ? 'Adding...' : 'Add'}
								</button>
							</div>

							{#if loadingBotCommands === bot.id}
								<p class="text-xs text-text-muted">Loading commands...</p>
							{:else if (botCommands[bot.id] ?? []).length === 0}
								<p class="text-xs text-text-muted">No slash commands registered.</p>
							{:else}
								<div class="space-y-2">
									{#each botCommands[bot.id] ?? [] as cmd (cmd.id)}
										<div class="flex items-center justify-between rounded bg-bg-primary p-2">
											<div>
												<span class="text-sm font-medium text-text-primary">/{cmd.name}</span>
												<span class="ml-2 text-xs text-text-muted">{cmd.description}</span>
												{#if cmd.guild_id}
													<span class="ml-1 rounded bg-bg-modifier px-1 py-0.5 text-2xs text-text-muted">Server-scoped</span>
												{:else}
													<span class="ml-1 rounded bg-brand-500/10 px-1 py-0.5 text-2xs text-brand-400">Global</span>
												{/if}
											</div>
											<button class="text-xs text-red-400 hover:text-red-300" onclick={() => handleDeleteCommand(bot.id, cmd.id)}>
												Delete
											</button>
										</div>
									{/each}
								</div>
							{/if}
						</div>
					</div>
				{/if}
			</div>
		{/each}
	</div>
{/if}
