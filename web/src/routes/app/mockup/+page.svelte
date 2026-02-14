<script lang="ts">
	let showComparison = $state(false);

	// Mock data
	const guilds = [
		{ id: 'home', name: 'AmityVox', abbr: 'AV', active: false, home: true },
		{ id: 'tf', name: 'Terminal Forge', abbr: 'TF', active: true },
		{ id: 'os', name: 'Open Source Hub', abbr: 'OS', active: false },
		{ id: 'dv', name: 'DevOps Central', abbr: 'DV', active: false }
	];

	const textChannels = [
		{ id: '1', name: 'general', active: true },
		{ id: '2', name: 'introductions', active: false },
		{ id: '3', name: 'dev-talk', active: false },
		{ id: '4', name: 'contributions', active: false },
		{ id: '5', name: 'off-topic', active: false }
	];

	const voiceChannels = [
		{ id: 'v1', name: 'Lounge', users: 2 },
		{ id: 'v2', name: 'Pair Programming', users: 0 }
	];

	const messages = [
		{
			id: '1',
			author: 'Mira Chen',
			avatar: 'MC',
			avatarColor: '#0ea37a',
			time: 'Today at 10:32 AM',
			content: 'Just pushed the new auth middleware. Anyone want to review before I merge?',
			reactions: []
		},
		{
			id: '2',
			author: 'Kai Nakamura',
			avatar: 'KN',
			avatarColor: '#3b82f6',
			time: 'Today at 10:34 AM',
			content: "I'll take a look — which branch?",
			reactions: []
		},
		{
			id: '3',
			author: 'Mira Chen',
			avatar: 'MC',
			avatarColor: '#0ea37a',
			time: 'Today at 10:35 AM',
			content: null,
			codeBlock: {
				lang: 'bash',
				code: 'git checkout feat/auth-middleware\ngo test ./internal/auth/...'
			},
			reactions: [
				{ emoji: '👍', count: 3 },
				{ emoji: '🚀', count: 1 }
			]
		},
		{
			id: '4',
			author: 'Sable Torres',
			avatar: 'ST',
			avatarColor: '#a855f7',
			time: 'Today at 10:41 AM',
			content:
				'Nice work! The TOTP flow looks clean. One thing — we should rate-limit the verification endpoint.',
			reactions: []
		},
		{
			id: '5',
			author: 'Dev Bot',
			avatar: 'DB',
			avatarColor: '#6e7681',
			time: 'Today at 10:42 AM',
			content:
				'CI passed on feat/auth-middleware — 47 tests, 0 failures. Coverage: 83.2%',
			reactions: [{ emoji: '✅', count: 2 }]
		}
	];

	const members = {
		online: [
			{ name: 'Mira Chen', avatar: 'MC', color: '#0ea37a', role: 'Maintainer' },
			{ name: 'Kai Nakamura', avatar: 'KN', color: '#3b82f6', role: 'Contributor' },
			{ name: 'Sable Torres', avatar: 'ST', color: '#a855f7', role: 'Contributor' }
		],
		offline: [
			{ name: 'Dev Bot', avatar: 'DB', color: '#6e7681', role: 'Bot' },
			{ name: 'Jordan Lee', avatar: 'JL', color: '#f59e0b', role: 'Member' }
		]
	};

	const comparison = [
		{ label: 'bg-primary', old: '#1e1f22', proposed: '#0d1117' },
		{ label: 'bg-secondary', old: '#2b2d31', proposed: '#161b22' },
		{ label: 'bg-tertiary', old: '#313338', proposed: '#1c2128' },
		{ label: 'text-primary', old: '#f2f3f5', proposed: '#e6edf3' },
		{ label: 'text-muted', old: '#949ba4', proposed: '#6e7681' },
		{ label: 'brand-500', old: '#3f51b5', proposed: '#0ea37a' },
		{ label: 'border', old: '#383a40', proposed: '#30363d' },
		{ label: 'Guild icons', old: '48px circle', proposed: '36px rounded-md' },
		{ label: 'Avatars', old: 'Circle', proposed: 'Squared (6px)' },
		{ label: 'Buttons', old: 'rounded (4px)', proposed: 'rounded-sm (2px)' }
	];
</script>

<svelte:head>
	<title>Terminal Forge — UI Mockup</title>
</svelte:head>

<div class="mockup-root">
	<!-- Top accent stripe -->
	<div class="accent-stripe"></div>

	<!-- App shell -->
	<div class="app-shell">
		<!-- Guild sidebar -->
		<div class="guild-sidebar">
			{#each guilds as guild}
				{#if guild.home}
					<div class="guild-icon home-icon" class:active={guild.active}>
						{#if guild.active}<div class="active-indicator"></div>{/if}
						<span>{guild.abbr}</span>
					</div>
					<div class="guild-divider"></div>
				{:else}
					<div class="guild-icon" class:active={guild.active} title={guild.name}>
						{#if guild.active}<div class="active-indicator"></div>{/if}
						<span>{guild.abbr}</span>
					</div>
				{/if}
			{/each}

			<div class="guild-icon add-guild" title="Add a Server">
				<span>+</span>
			</div>

			<div class="guild-spacer"></div>

			<!-- Settings cog -->
			<div class="guild-icon settings-icon" title="Settings">
				<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
					<circle cx="12" cy="12" r="3" />
					<path
						d="M12 1v2M12 21v2M4.22 4.22l1.42 1.42M18.36 18.36l1.42 1.42M1 12h2M21 12h2M4.22 19.78l1.42-1.42M18.36 5.64l1.42-1.42"
					/>
				</svg>
			</div>
		</div>

		<!-- Channel sidebar -->
		<div class="channel-sidebar">
			<div class="server-header">
				<span class="server-name">Terminal Forge</span>
				<svg width="14" height="14" viewBox="0 0 24 24" fill="currentColor" class="chevron">
					<path d="M7 10l5 5 5-5z" />
				</svg>
			</div>

			<div class="channel-section">
				<div class="section-label">
					<svg width="10" height="10" viewBox="0 0 24 24" fill="currentColor" class="collapse-icon">
						<path d="M7 10l5 5 5-5z" />
					</svg>
					TEXT CHANNELS
				</div>
				{#each textChannels as ch}
					<div class="channel-item" class:active={ch.active}>
						<span class="channel-hash">#</span>
						<span class="channel-name">{ch.name}</span>
					</div>
				{/each}
			</div>

			<div class="channel-section">
				<div class="section-label">
					<svg width="10" height="10" viewBox="0 0 24 24" fill="currentColor" class="collapse-icon">
						<path d="M7 10l5 5 5-5z" />
					</svg>
					VOICE CHANNELS
				</div>
				{#each voiceChannels as ch}
					<div class="channel-item voice">
						<svg width="14" height="14" viewBox="0 0 24 24" fill="currentColor" class="voice-icon">
							<path
								d="M12 1a4 4 0 00-4 4v6a4 4 0 008 0V5a4 4 0 00-4-4zM19 11a1 1 0 10-2 0 5 5 0 01-10 0 1 1 0 10-2 0 7 7 0 006 6.93V21H8a1 1 0 100 2h8a1 1 0 100-2h-3v-3.07A7 7 0 0019 11z"
							/>
						</svg>
						<span class="channel-name">{ch.name}</span>
						{#if ch.users > 0}
							<span class="voice-count">{ch.users}</span>
						{/if}
					</div>
				{/each}
			</div>

			<!-- User panel -->
			<div class="user-panel">
				<div class="user-avatar" style="background-color: #0ea37a">HC</div>
				<div class="user-info">
					<div class="user-name">Horatio</div>
					<div class="user-status">Online</div>
				</div>
				<div class="user-actions">
					<svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor" class="action-icon">
						<path
							d="M12 1a4 4 0 00-4 4v6a4 4 0 008 0V5a4 4 0 00-4-4zM19 11a1 1 0 10-2 0 5 5 0 01-10 0 1 1 0 10-2 0 7 7 0 006 6.93V21H8a1 1 0 100 2h8a1 1 0 100-2h-3v-3.07A7 7 0 0019 11z"
						/>
					</svg>
					<svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor" class="action-icon">
						<path
							d="M12 15.5A3.5 3.5 0 018.5 12 3.5 3.5 0 0112 8.5a3.5 3.5 0 013.5 3.5 3.5 3.5 0 01-3.5 3.5m7.43-2.53a7.76 7.76 0 000-1.94l2.11-1.65a.5.5 0 00.12-.64l-2-3.46a.5.5 0 00-.61-.22l-2.49 1a7.3 7.3 0 00-1.69-.98l-.38-2.65A.49.49 0 0014 2h-4a.49.49 0 00-.49.42l-.38 2.65a7.3 7.3 0 00-1.69.98l-2.49-1a.5.5 0 00-.61.22l-2 3.46a.49.49 0 00.12.64l2.11 1.65a7.93 7.93 0 000 1.94l-2.11 1.65a.5.5 0 00-.12.64l2 3.46a.5.5 0 00.61.22l2.49-1a7.3 7.3 0 001.69.98l.38 2.65c.05.24.26.42.49.42h4c.24 0 .44-.18.49-.42l.38-2.65a7.3 7.3 0 001.69-.98l2.49 1a.5.5 0 00.61-.22l2-3.46a.49.49 0 00-.12-.64l-2.11-1.65z"
						/>
					</svg>
				</div>
			</div>
		</div>

		<!-- Main content area -->
		<div class="main-content">
			<!-- Top bar -->
			<div class="top-bar">
				<div class="top-bar-left">
					<span class="top-channel-hash">#</span>
					<span class="top-channel-name">general</span>
					<span class="top-divider"></span>
					<span class="top-topic">Main discussion — keep it on-topic</span>
				</div>
				<div class="top-bar-right">
					<svg width="18" height="18" viewBox="0 0 24 24" fill="currentColor" class="toolbar-icon">
						<path
							d="M15.5 14h-.79l-.28-.27A6.47 6.47 0 0016 9.5 6.5 6.5 0 109.5 16c1.61 0 3.09-.59 4.23-1.57l.27.28v.79l5 4.99L20.49 19l-4.99-5zm-6 0C7.01 14 5 11.99 5 9.5S7.01 5 9.5 5 14 7.01 14 9.5 11.99 14 9.5 14z"
						/>
					</svg>
					<svg width="18" height="18" viewBox="0 0 24 24" fill="currentColor" class="toolbar-icon">
						<path d="M20 2H4a2 2 0 00-2 2v18l4-4h14a2 2 0 002-2V4a2 2 0 00-2-2z" />
					</svg>
					<svg width="18" height="18" viewBox="0 0 24 24" fill="currentColor" class="toolbar-icon">
						<path
							d="M16 11c1.66 0 2.99-1.34 2.99-3S17.66 5 16 5c-1.66 0-3 1.34-3 3s1.34 3 3 3zm-8 0c1.66 0 2.99-1.34 2.99-3S9.66 5 8 5C6.34 5 5 6.34 5 8s1.34 3 3 3zm0 2c-2.33 0-7 1.17-7 3.5V19h14v-2.5c0-2.33-4.67-3.5-7-3.5zm8 0c-.29 0-.62.02-.97.05 1.16.84 1.97 1.97 1.97 3.45V19h6v-2.5c0-2.33-4.67-3.5-7-3.5z"
						/>
					</svg>
				</div>
			</div>

			<!-- Message list -->
			<div class="message-list">
				{#each messages as msg}
					<div class="message">
						<div class="msg-avatar" style="background-color: {msg.avatarColor}">
							{msg.avatar}
						</div>
						<div class="msg-body">
							<div class="msg-header">
								<span class="msg-author">{msg.author}</span>
								<span class="msg-time">{msg.time}</span>
							</div>
							{#if msg.content}
								<div class="msg-text">{msg.content}</div>
							{/if}
							{#if msg.codeBlock}
								<div class="msg-code">
									<div class="code-header">{msg.codeBlock.lang}</div>
									<pre><code>{msg.codeBlock.code}</code></pre>
								</div>
							{/if}
							{#if msg.reactions && msg.reactions.length > 0}
								<div class="msg-reactions">
									{#each msg.reactions as r}
										<button class="reaction">
											<span>{r.emoji}</span>
											<span class="reaction-count">{r.count}</span>
										</button>
									{/each}
								</div>
							{/if}
						</div>
					</div>
				{/each}
			</div>

			<!-- Message input -->
			<div class="message-input-area">
				<div class="input-wrapper">
					<button class="input-icon-btn" title="Attach file">
						<svg width="18" height="18" viewBox="0 0 24 24" fill="currentColor">
							<path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm5 11h-4v4h-2v-4H7v-2h4V7h2v4h4v2z" />
						</svg>
					</button>
					<input type="text" class="message-input" placeholder="Message #general" readonly />
					<div class="input-actions">
						<button class="input-icon-btn" title="Emoji">
							<svg width="18" height="18" viewBox="0 0 24 24" fill="currentColor">
								<path
									d="M12 2C6.47 2 2 6.47 2 12s4.47 10 10 10 10-4.47 10-10S17.53 2 12 2zm0 18c-4.41 0-8-3.59-8-8s3.59-8 8-8 8 3.59 8 8-3.59 8-8 8zm3.5-9c.83 0 1.5-.67 1.5-1.5S16.33 8 15.5 8 14 8.67 14 9.5s.67 1.5 1.5 1.5zm-7 0c.83 0 1.5-.67 1.5-1.5S9.33 8 8.5 8 7 8.67 7 9.5 7.67 11 8.5 11zm3.5 6.5c2.33 0 4.31-1.46 5.11-3.5H6.89c.8 2.04 2.78 3.5 5.11 3.5z"
								/>
							</svg>
						</button>
					</div>
				</div>
			</div>
		</div>

		<!-- Member list -->
		<div class="member-list">
			<div class="member-section-label">ONLINE — {members.online.length}</div>
			{#each members.online as m}
				<div class="member-item">
					<div class="member-avatar" style="background-color: {m.color}">{m.avatar}</div>
					<div class="member-info">
						<span class="member-name">{m.name}</span>
						{#if m.role !== 'Member'}
							<span class="member-role">{m.role}</span>
						{/if}
					</div>
					<div class="status-dot online"></div>
				</div>
			{/each}

			<div class="member-section-label">OFFLINE — {members.offline.length}</div>
			{#each members.offline as m}
				<div class="member-item offline">
					<div class="member-avatar" style="background-color: {m.color}">{m.avatar}</div>
					<div class="member-info">
						<span class="member-name">{m.name}</span>
						{#if m.role !== 'Member'}
							<span class="member-role">{m.role}</span>
						{/if}
					</div>
					<div class="status-dot"></div>
				</div>
			{/each}
		</div>
	</div>

	<!-- Back to App button -->
	<a href="/app" class="back-btn">
		<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
			<path d="M19 12H5M12 19l-7-7 7-7" />
		</svg>
		Back to App
	</a>

	<!-- Comparison toggle -->
	<button class="compare-toggle" onclick={() => (showComparison = !showComparison)}>
		{showComparison ? 'Hide' : 'Compare'}
	</button>

	<!-- Comparison panel -->
	{#if showComparison}
		<div class="compare-panel">
			<div class="compare-title">Design Comparison</div>
			<table class="compare-table">
				<thead>
					<tr>
						<th>Token</th>
						<th>Current</th>
						<th>Proposed</th>
					</tr>
				</thead>
				<tbody>
					{#each comparison as row}
						<tr>
							<td class="token-name">{row.label}</td>
							<td>
								{#if row.old.startsWith('#')}
									<span class="color-swatch" style="background-color: {row.old}"></span>
								{/if}
								<code>{row.old}</code>
							</td>
							<td>
								{#if row.proposed.startsWith('#')}
									<span class="color-swatch" style="background-color: {row.proposed}"></span>
								{/if}
								<code>{row.proposed}</code>
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
</div>

<style>
	@import url('https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600&family=JetBrains+Mono:wght@400;500&display=swap');

	/* ===== Root: full-viewport overlay ===== */
	.mockup-root {
		position: fixed;
		inset: 0;
		z-index: 200;
		display: flex;
		flex-direction: column;
		font-family: 'Inter', system-ui, -apple-system, sans-serif;
		color: #e6edf3;
		background: #0d1117;
		overflow: hidden;
	}

	/* ===== Accent stripe ===== */
	.accent-stripe {
		height: 2px;
		flex-shrink: 0;
		background: linear-gradient(90deg, #0ea37a, #22d3ee);
	}

	/* ===== App shell ===== */
	.app-shell {
		display: flex;
		flex: 1;
		min-height: 0;
	}

	/* ===== Guild sidebar ===== */
	.guild-sidebar {
		width: 56px;
		flex-shrink: 0;
		display: flex;
		flex-direction: column;
		align-items: center;
		padding: 8px 0;
		gap: 4px;
		background: #0a0d12;
		border-right: 1px solid #30363d;
		overflow-y: auto;
	}

	.guild-icon {
		position: relative;
		width: 36px;
		height: 36px;
		display: flex;
		align-items: center;
		justify-content: center;
		border-radius: 6px;
		background: #161b22;
		border: 1px solid #30363d;
		font-family: 'JetBrains Mono', monospace;
		font-size: 12px;
		font-weight: 500;
		color: #8b949e;
		cursor: pointer;
		transition: background 0.15s, border-color 0.15s, color 0.15s;
	}

	.guild-icon:hover {
		background: #1c2128;
		border-color: #0ea37a;
		color: #e6edf3;
	}

	.guild-icon.active {
		background: #0ea37a;
		border-color: #0ea37a;
		color: #fff;
	}

	.guild-icon.home-icon {
		background: #161b22;
		font-weight: 600;
	}

	.active-indicator {
		position: absolute;
		left: -10px;
		top: 50%;
		transform: translateY(-50%);
		width: 2px;
		height: 20px;
		background: #e6edf3;
		border-radius: 0 1px 1px 0;
	}

	.guild-divider {
		width: 24px;
		height: 1px;
		background: #30363d;
		margin: 4px 0;
	}

	.add-guild {
		color: #0ea37a;
		font-size: 18px;
		font-family: 'Inter', sans-serif;
		border-style: dashed;
	}

	.add-guild:hover {
		background: #0ea37a;
		color: #fff;
		border-style: solid;
	}

	.guild-spacer {
		flex: 1;
	}

	.settings-icon {
		color: #8b949e;
	}

	/* ===== Channel sidebar ===== */
	.channel-sidebar {
		width: 224px;
		flex-shrink: 0;
		display: flex;
		flex-direction: column;
		background: #161b22;
		border-right: 1px solid #30363d;
	}

	.server-header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: 10px 12px;
		border-bottom: 1px solid #30363d;
		cursor: pointer;
	}

	.server-header:hover {
		background: #1c2128;
	}

	.server-name {
		font-weight: 600;
		font-size: 14px;
		color: #e6edf3;
	}

	.chevron {
		color: #8b949e;
	}

	.channel-section {
		padding: 12px 0 4px;
	}

	.section-label {
		display: flex;
		align-items: center;
		gap: 4px;
		padding: 0 12px 6px;
		font-family: 'JetBrains Mono', monospace;
		font-size: 10px;
		font-weight: 500;
		letter-spacing: 0.04em;
		color: #6e7681;
		text-transform: uppercase;
		cursor: pointer;
	}

	.section-label:hover {
		color: #8b949e;
	}

	.collapse-icon {
		color: #6e7681;
	}

	.channel-item {
		display: flex;
		align-items: center;
		gap: 6px;
		padding: 4px 12px;
		margin: 0 8px;
		border-radius: 4px;
		cursor: pointer;
		color: #8b949e;
		transition: background 0.1s, color 0.1s;
	}

	.channel-item:hover {
		background: #1c2128;
		color: #e6edf3;
	}

	.channel-item.active {
		background: #272d36;
		color: #e6edf3;
	}

	.channel-hash {
		font-family: 'JetBrains Mono', monospace;
		font-size: 15px;
		font-weight: 500;
		color: #0ea37a;
	}

	.channel-name {
		font-family: 'JetBrains Mono', monospace;
		font-size: 13px;
	}

	.voice-icon {
		color: #8b949e;
		flex-shrink: 0;
	}

	.voice-count {
		margin-left: auto;
		font-family: 'JetBrains Mono', monospace;
		font-size: 11px;
		color: #6e7681;
	}

	/* ===== User panel ===== */
	.user-panel {
		margin-top: auto;
		display: flex;
		align-items: center;
		gap: 8px;
		padding: 8px 12px;
		border-top: 1px solid #30363d;
		background: #0d1117;
	}

	.user-avatar {
		width: 28px;
		height: 28px;
		border-radius: 6px;
		display: flex;
		align-items: center;
		justify-content: center;
		font-family: 'JetBrains Mono', monospace;
		font-size: 10px;
		font-weight: 500;
		color: #fff;
		flex-shrink: 0;
	}

	.user-info {
		flex: 1;
		min-width: 0;
	}

	.user-name {
		font-size: 12px;
		font-weight: 500;
		color: #e6edf3;
	}

	.user-status {
		font-size: 10px;
		color: #6e7681;
	}

	.user-actions {
		display: flex;
		gap: 4px;
	}

	.action-icon {
		color: #6e7681;
		cursor: pointer;
		transition: color 0.15s;
	}

	.action-icon:hover {
		color: #e6edf3;
	}

	/* ===== Main content ===== */
	.main-content {
		flex: 1;
		display: flex;
		flex-direction: column;
		min-width: 0;
		background: #0d1117;
	}

	/* ===== Top bar ===== */
	.top-bar {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: 0 16px;
		height: 40px;
		flex-shrink: 0;
		border-bottom: 1px solid #30363d;
	}

	.top-bar-left {
		display: flex;
		align-items: center;
		gap: 8px;
		min-width: 0;
	}

	.top-channel-hash {
		font-family: 'JetBrains Mono', monospace;
		font-size: 16px;
		font-weight: 500;
		color: #0ea37a;
	}

	.top-channel-name {
		font-family: 'JetBrains Mono', monospace;
		font-size: 14px;
		font-weight: 500;
		color: #e6edf3;
	}

	.top-divider {
		width: 1px;
		height: 18px;
		background: #30363d;
	}

	.top-topic {
		font-size: 12px;
		color: #6e7681;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.top-bar-right {
		display: flex;
		align-items: center;
		gap: 12px;
	}

	.toolbar-icon {
		color: #8b949e;
		cursor: pointer;
		transition: color 0.15s;
	}

	.toolbar-icon:hover {
		color: #e6edf3;
	}

	/* ===== Message list ===== */
	.message-list {
		flex: 1;
		overflow-y: auto;
		padding: 16px;
		display: flex;
		flex-direction: column;
		gap: 2px;
	}

	.message {
		display: flex;
		gap: 12px;
		padding: 6px 8px;
		border-radius: 4px;
		transition: background 0.1s;
	}

	.message:hover {
		background: #161b22;
	}

	.msg-avatar {
		width: 32px;
		height: 32px;
		border-radius: 6px;
		display: flex;
		align-items: center;
		justify-content: center;
		font-family: 'JetBrains Mono', monospace;
		font-size: 11px;
		font-weight: 500;
		color: #fff;
		flex-shrink: 0;
	}

	.msg-body {
		flex: 1;
		min-width: 0;
	}

	.msg-header {
		display: flex;
		align-items: baseline;
		gap: 8px;
		margin-bottom: 2px;
	}

	.msg-author {
		font-size: 13px;
		font-weight: 500;
		color: #e6edf3;
	}

	.msg-time {
		font-family: 'JetBrains Mono', monospace;
		font-size: 10px;
		color: #6e7681;
	}

	.msg-text {
		font-size: 13px;
		line-height: 1.45;
		color: #e6edf3;
	}

	/* ===== Code block ===== */
	.msg-code {
		margin-top: 4px;
		border: 1px solid #30363d;
		border-radius: 4px;
		overflow: hidden;
		max-width: 480px;
	}

	.code-header {
		font-family: 'JetBrains Mono', monospace;
		font-size: 10px;
		color: #6e7681;
		padding: 4px 10px;
		background: #161b22;
		border-bottom: 1px solid #30363d;
		text-transform: uppercase;
		letter-spacing: 0.04em;
	}

	.msg-code pre {
		margin: 0;
		padding: 10px;
		background: #0a0d12;
		overflow-x: auto;
	}

	.msg-code code {
		font-family: 'JetBrains Mono', monospace;
		font-size: 12px;
		color: #e6edf3;
		line-height: 1.5;
	}

	/* ===== Reactions ===== */
	.msg-reactions {
		display: flex;
		gap: 4px;
		margin-top: 6px;
	}

	.reaction {
		display: flex;
		align-items: center;
		gap: 4px;
		padding: 2px 8px;
		border: 1px solid #30363d;
		border-radius: 2px;
		background: #161b22;
		color: #e6edf3;
		font-size: 12px;
		cursor: pointer;
		transition: border-color 0.15s;
	}

	.reaction:hover {
		border-color: #0ea37a;
	}

	.reaction-count {
		font-family: 'JetBrains Mono', monospace;
		font-size: 11px;
		color: #8b949e;
	}

	/* ===== Message input ===== */
	.message-input-area {
		padding: 0 16px 16px;
		flex-shrink: 0;
	}

	.input-wrapper {
		display: flex;
		align-items: center;
		gap: 8px;
		padding: 8px 12px;
		border: 1px solid #30363d;
		border-radius: 4px;
		background: #161b22;
		transition: border-color 0.15s, box-shadow 0.15s;
	}

	.input-wrapper:focus-within {
		border-color: #0ea37a;
		box-shadow: 0 0 0 1px #0ea37a;
	}

	.message-input {
		flex: 1;
		background: transparent;
		border: none;
		outline: none;
		color: #e6edf3;
		font-family: 'JetBrains Mono', monospace;
		font-size: 13px;
	}

	.message-input::placeholder {
		color: #6e7681;
	}

	.input-icon-btn {
		display: flex;
		align-items: center;
		justify-content: center;
		background: none;
		border: none;
		color: #6e7681;
		cursor: pointer;
		padding: 2px;
		transition: color 0.15s;
	}

	.input-icon-btn:hover {
		color: #e6edf3;
	}

	.input-actions {
		display: flex;
		gap: 4px;
	}

	/* ===== Member list ===== */
	.member-list {
		width: 200px;
		flex-shrink: 0;
		border-left: 1px solid #30363d;
		background: #161b22;
		padding: 12px 8px;
		overflow-y: auto;
	}

	.member-section-label {
		font-family: 'JetBrains Mono', monospace;
		font-size: 10px;
		font-weight: 500;
		letter-spacing: 0.04em;
		color: #6e7681;
		padding: 8px 8px 6px;
		text-transform: uppercase;
	}

	.member-item {
		display: flex;
		align-items: center;
		gap: 8px;
		padding: 4px 8px;
		border-radius: 4px;
		cursor: pointer;
		transition: background 0.1s;
	}

	.member-item:hover {
		background: #1c2128;
	}

	.member-item.offline {
		opacity: 0.5;
	}

	.member-avatar {
		width: 26px;
		height: 26px;
		border-radius: 6px;
		display: flex;
		align-items: center;
		justify-content: center;
		font-family: 'JetBrains Mono', monospace;
		font-size: 9px;
		font-weight: 500;
		color: #fff;
		flex-shrink: 0;
	}

	.member-info {
		flex: 1;
		min-width: 0;
	}

	.member-name {
		font-size: 12px;
		color: #e6edf3;
		display: block;
	}

	.member-role {
		font-family: 'JetBrains Mono', monospace;
		font-size: 9px;
		color: #6e7681;
	}

	.status-dot {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		background: #6e7681;
		flex-shrink: 0;
	}

	.status-dot.online {
		background: #23a55a;
	}

	/* ===== Back to App button ===== */
	.back-btn {
		position: fixed;
		top: 12px;
		right: 16px;
		z-index: 210;
		display: flex;
		align-items: center;
		gap: 6px;
		padding: 6px 12px;
		border: 1px solid #30363d;
		border-radius: 4px;
		background: #161b22;
		color: #8b949e;
		font-family: 'JetBrains Mono', monospace;
		font-size: 11px;
		text-decoration: none;
		cursor: pointer;
		transition: border-color 0.15s, color 0.15s;
	}

	.back-btn:hover {
		border-color: #0ea37a;
		color: #e6edf3;
	}

	/* ===== Comparison toggle ===== */
	.compare-toggle {
		position: fixed;
		bottom: 16px;
		right: 16px;
		z-index: 210;
		padding: 6px 14px;
		border: 1px solid #30363d;
		border-radius: 2px;
		background: #161b22;
		color: #8b949e;
		font-family: 'JetBrains Mono', monospace;
		font-size: 11px;
		cursor: pointer;
		transition: border-color 0.15s, color 0.15s, background 0.15s;
	}

	.compare-toggle:hover {
		border-color: #0ea37a;
		color: #e6edf3;
	}

	/* ===== Comparison panel ===== */
	.compare-panel {
		position: fixed;
		bottom: 48px;
		right: 16px;
		z-index: 210;
		width: 420px;
		max-height: 400px;
		overflow-y: auto;
		border: 1px solid #30363d;
		border-radius: 4px;
		background: #0a0d12;
		padding: 12px;
		box-shadow: 0 8px 24px rgba(0, 0, 0, 0.4);
	}

	.compare-title {
		font-family: 'JetBrains Mono', monospace;
		font-size: 12px;
		font-weight: 500;
		color: #e6edf3;
		margin-bottom: 10px;
		letter-spacing: 0.02em;
	}

	.compare-table {
		width: 100%;
		border-collapse: collapse;
		font-size: 11px;
	}

	.compare-table th {
		font-family: 'JetBrains Mono', monospace;
		font-weight: 500;
		color: #6e7681;
		text-align: left;
		padding: 4px 8px;
		border-bottom: 1px solid #30363d;
		font-size: 10px;
		text-transform: uppercase;
		letter-spacing: 0.04em;
	}

	.compare-table td {
		padding: 5px 8px;
		border-bottom: 1px solid #1c2128;
		color: #8b949e;
		vertical-align: middle;
	}

	.token-name {
		font-family: 'JetBrains Mono', monospace;
		color: #e6edf3 !important;
	}

	.compare-table code {
		font-family: 'JetBrains Mono', monospace;
		font-size: 11px;
	}

	.color-swatch {
		display: inline-block;
		width: 12px;
		height: 12px;
		border-radius: 2px;
		border: 1px solid #30363d;
		vertical-align: middle;
		margin-right: 6px;
	}

	/* ===== Scrollbar styling ===== */
	.mockup-root ::-webkit-scrollbar {
		width: 6px;
	}

	.mockup-root ::-webkit-scrollbar-track {
		background: transparent;
	}

	.mockup-root ::-webkit-scrollbar-thumb {
		background: #30363d;
		border-radius: 3px;
	}

	.mockup-root ::-webkit-scrollbar-thumb:hover {
		background: #484f58;
	}
</style>
