<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { api } from '$lib/api/client';
	import { addToast } from '$lib/stores/toast';
	import { currentUser } from '$lib/stores/auth';
	import { clientConfig, isFeatureEnabled, loadClientConfig } from '$lib/stores/clientConfig';
	import { createAsyncOp } from '$lib/utils/asyncOp';
	import type {
		ModerationStats,
		ModerationMessageReport,
		UserReport,
		ReportedIssue,
		IssueAccessToken
	} from '$lib/types';

	type Tab = 'dashboard' | 'message_reports' | 'user_reports' | 'issues';
	type IssueFilter = '' | ReportedIssue['status'] | 'all';
	type ResolutionStatus = 'resolved' | 'dismissed';
	type IssueResolutionStatus = ResolutionStatus | 'in_progress';
	let currentTab = $state<Tab>('dashboard');

	// --- Dashboard ---
	let stats = $state<ModerationStats | null>(null);
	let statsOp = $state(createAsyncOp());

	// --- Message Reports ---
	let messageReports = $state<ModerationMessageReport[]>([]);
	let messageReportsOp = $state(createAsyncOp());
	let messageReportsLoaded = $state(false);

	// --- User Reports ---
	let userReports = $state<UserReport[]>([]);
	let userReportsOp = $state(createAsyncOp());
	let userReportsLoaded = $state(false);

	// --- Issues ---
	let issues = $state<ReportedIssue[]>([]);
	let issuesOp = $state(createAsyncOp());
	let issuesLoaded = $state(false);
	let issueFilter = $state<IssueFilter>('');  // '' = active (open+in_progress), 'all', 'open', 'in_progress', 'resolved', 'dismissed'
	let issueTokens = $state<IssueAccessToken[]>([]);
	let issueTokensLoaded = $state(false);
	let issueTokensRequested = $state(false);
	let issueTokenHours = $state(24);
	let issueTokenNote = $state('');
	let createdIssueToken = $state<string | null>(null);
	let issueExportOp = $state(createAsyncOp());
	let issueTokenOp = $state(createAsyncOp());

	// --- Resolve modal ---
	let resolveModalOpen = $state(false);
	let resolveType = $state<'message_report' | 'user_report' | 'issue'>('message_report');
	let resolveId = $state('');
	let resolveNotes = $state('');
	let resolving = $state(false);

	const tabs: { id: Tab; label: string }[] = [
		{ id: 'dashboard', label: 'Dashboard' },
		{ id: 'message_reports', label: 'Message Reports' },
		{ id: 'user_reports', label: 'User Reports' },
		{ id: 'issues', label: 'Issues' }
	];

	async function loadStats() {
		const result = await statsOp.run(() => api.getModerationStats(), msg => addToast(msg, 'error'), 'Failed to load moderation stats');
		if (result) stats = result;
	}

	async function loadMessageReports() {
		const result = await messageReportsOp.run(() => api.getModerationMessageReports(), msg => addToast(msg, 'error'), 'Failed to load message reports');
		if (result) {
			messageReports = result;
			messageReportsLoaded = true;
		}
	}

	async function loadUserReports() {
		const result = await userReportsOp.run(() => api.getModerationUserReports(), msg => addToast(msg, 'error'), 'Failed to load user reports');
		if (result) {
			userReports = result;
			userReportsLoaded = true;
		}
	}

	async function loadIssues() {
		const result = await issuesOp.run(() => api.getModerationIssues(issueFilter || undefined), msg => addToast(msg, 'error'), 'Failed to load issues');
		if (result) {
			issues = result;
			issuesLoaded = true;
		}
	}

	async function loadIssueTokens() {
		if (!isAdmin || issueTokensRequested) return;
		issueTokensRequested = true;
		const result = await issueTokenOp.run(() => api.getIssueAccessTokens(), msg => addToast(msg, 'error'), 'Failed to load issue access tokens');
		if (result) {
			issueTokens = result;
			issueTokensLoaded = true;
		}
	}

	async function exportIssues() {
		const result = await issueExportOp.run(() => api.exportModerationIssues('all'), msg => addToast(msg, 'error'), 'Failed to export issues');
		if (!result) return;

		const blob = new Blob([JSON.stringify(result, null, 2)], { type: 'application/json' });
		const url = URL.createObjectURL(blob);
		const link = document.createElement('a');
		link.href = url;
		link.download = `amityvox-issues-${new Date().toISOString().slice(0, 10)}.json`;
		document.body.appendChild(link);
		link.click();
		link.remove();
		URL.revokeObjectURL(url);
	}

	async function createIssueToken() {
		const token = await issueTokenOp.run(
			() => api.createIssueAccessToken(issueTokenHours, issueTokenNote.trim() || undefined),
			msg => addToast(msg, 'error'),
			'Failed to create issue access token'
		);
		if (!token) return;
		createdIssueToken = token.token ?? null;
		issueTokenNote = '';
		issueTokens = [token, ...issueTokens];
		issueTokensRequested = true;
		issueTokensLoaded = true;
		addToast('Issue access token created', 'success');
	}

	async function revokeIssueToken(tokenId: string) {
		await issueTokenOp.run(
			() => api.revokeIssueAccessToken(tokenId),
			msg => addToast(msg, 'error'),
			'Failed to revoke issue access token'
		);
		if (!issueTokenOp.error) {
			issueTokens = issueTokens.map((token) => token.id === tokenId ? { ...token, revoked_at: new Date().toISOString() } : token);
			addToast('Issue access token revoked', 'success');
		}
	}

	function copyIssueToken() {
		if (!createdIssueToken) return;
		navigator.clipboard.writeText(createdIssueToken).then(
			() => addToast('Token copied', 'success'),
			() => addToast('Failed to copy token', 'error')
		);
	}

	function setIssueFilter(filter: IssueFilter) {
		issueFilter = filter;
		issuesLoaded = false;
		loadIssues();
	}

	function openResolve(type: 'message_report' | 'user_report' | 'issue', id: string) {
		resolveType = type;
		resolveId = id;
		resolveNotes = '';
		resolveModalOpen = true;
	}

	async function submitResolve(status: IssueResolutionStatus) {
		resolving = true;
		try {
			if (resolveType === 'message_report') {
				if (status === 'in_progress') return;
				await api.resolveModerationMessageReport(resolveId, status, resolveNotes || undefined);
				messageReports = messageReports.map(r => r.id === resolveId ? { ...r, status } : r);
			} else if (resolveType === 'user_report') {
				if (status === 'in_progress') return;
				await api.resolveModerationUserReport(resolveId, status, resolveNotes || undefined);
				userReports = userReports.map(r => r.id === resolveId ? { ...r, status } : r);
			} else if (resolveType === 'issue') {
				await api.resolveModerationIssue(resolveId, status, resolveNotes || undefined);
				issues = issues.map(i => i.id === resolveId ? { ...i, status } : i);
			}
			addToast(`Item ${status}`, 'success');
			resolveModalOpen = false;
			// Refresh stats
			loadStats();
		} catch {
			addToast('Failed to update status', 'error');
		} finally {
			resolving = false;
		}
	}

	function statusBadgeClass(status: string): string {
		switch (status) {
			case 'open':
			case 'admin_pending':
				return 'bg-red-500/20 text-red-400';
			case 'in_progress':
				return 'bg-yellow-500/20 text-yellow-400';
			case 'resolved':
				return 'bg-green-500/20 text-green-400';
			case 'dismissed':
				return 'bg-text-muted/20 text-text-muted';
			default:
				return 'bg-bg-modifier text-text-muted';
		}
	}

	function formatDate(dateStr: string): string {
		return new Date(dateStr).toLocaleString();
	}

	const issueFilters: { value: IssueFilter; label: string }[] = [
		{ value: '', label: 'Active' },
		{ value: 'open', label: 'Open' },
		{ value: 'in_progress', label: 'In Progress' },
		{ value: 'resolved', label: 'Resolved' },
		{ value: 'dismissed', label: 'Dismissed' },
		{ value: 'all', label: 'All' }
	];

	// GlobalMod = 1<<5 = 32, Admin = 1<<2 = 4
	const isGlobalMod = $derived(($currentUser?.flags ?? 0) & 32);
	const isAdmin = $derived(($currentUser?.flags ?? 0) & 4);
	const canAccessModeration = $derived(isGlobalMod || isAdmin);
	const hasModerationReports = $derived(isFeatureEnabled($clientConfig, 'moderation_reports'));

	onMount(() => {
		loadClientConfig().catch(() => {});
	});

	$effect(() => {
		if (!canAccessModeration || !hasModerationReports) return;
		if (currentTab === 'dashboard') loadStats();
		if (currentTab === 'message_reports' && !messageReportsLoaded) loadMessageReports();
		if (currentTab === 'user_reports' && !userReportsLoaded) loadUserReports();
		if (currentTab === 'issues' && !issuesLoaded) loadIssues();
		if (currentTab === 'issues' && isAdmin && !issueTokensLoaded) loadIssueTokens();
	});
</script>

{#if !canAccessModeration}
<div class="flex h-full items-center justify-center bg-bg-tertiary">
	<div class="text-center">
		<h1 class="mb-2 text-2xl font-bold text-text-primary">Access Denied</h1>
		<p class="text-sm text-text-muted">You don't have permission to view the moderation panel.</p>
		<a href="/app" class="mt-4 inline-block text-sm text-brand-400 hover:text-brand-300">Back to app</a>
	</div>
</div>
{:else if !hasModerationReports}
<div class="flex h-full items-center justify-center bg-bg-tertiary">
	<div class="text-center">
		<h1 class="mb-2 text-2xl font-bold text-text-primary">Moderation Reports Disabled</h1>
		<p class="text-sm text-text-muted">Reporting and moderation queues are disabled on this instance.</p>
		<a href="/app" class="mt-4 inline-block text-sm text-brand-400 hover:text-brand-300">Back to app</a>
	</div>
</div>
{:else}
<div class="flex h-full">
	<!-- Sidebar -->
	<nav class="flex w-48 shrink-0 flex-col overflow-y-auto bg-bg-secondary p-4">
		<h3 class="mb-2 text-xs font-bold uppercase tracking-wide text-text-muted">Moderation</h3>
		<ul class="space-y-0.5">
			{#each tabs as tab (tab.id)}
				<li>
					<button
						class="w-full rounded px-2 py-1.5 text-left text-sm transition-colors {currentTab === tab.id ? 'bg-bg-modifier text-text-primary' : 'text-text-muted hover:bg-bg-modifier hover:text-text-secondary'}"
						onclick={() => (currentTab = tab.id)}
					>
						{tab.label}
					</button>
				</li>
			{/each}
		</ul>
		<div class="mt-auto pt-4">
			<button
				class="flex w-full items-center gap-2 rounded px-2 py-1.5 text-sm text-text-muted transition-colors hover:bg-bg-modifier hover:text-text-secondary"
				onclick={() => goto('/app')}
			>
				<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24" aria-hidden="true">
					<path d="M15 19l-7-7 7-7" />
				</svg>
				Back to App
			</button>
		</div>
	</nav>

	<!-- Content -->
	<div class="flex-1 overflow-y-auto p-8">
		<div class="max-w-4xl">

			<!-- ==================== DASHBOARD ==================== -->
			{#if currentTab === 'dashboard'}
				<div class="mb-6 flex items-center justify-between">
					<h1 class="text-2xl font-bold text-text-primary">Moderation Dashboard</h1>
					<button class="btn-secondary text-sm" onclick={loadStats} disabled={statsOp.loading}>
						{statsOp.loading ? 'Loading...' : 'Refresh'}
					</button>
				</div>

				{#if statsOp.loading && !stats}
					<p class="text-text-muted">Loading stats...</p>
				{:else if stats}
					<div class="grid gap-4 sm:grid-cols-3">
						<div class="rounded-lg bg-bg-secondary p-4">
							<p class="text-sm text-text-muted">Open Message Reports</p>
							<p class="mt-1 text-2xl font-bold text-red-400">{stats.open_message_reports}</p>
						</div>
						<div class="rounded-lg bg-bg-secondary p-4">
							<p class="text-sm text-text-muted">Open User Reports</p>
							<p class="mt-1 text-2xl font-bold text-orange-400">{stats.open_user_reports}</p>
						</div>
						<div class="rounded-lg bg-bg-secondary p-4">
							<p class="text-sm text-text-muted">Open Issues</p>
							<p class="mt-1 text-2xl font-bold text-yellow-400">{stats.open_issues}</p>
						</div>
					</div>

					{#if stats.open_message_reports + stats.open_user_reports + stats.open_issues === 0}
						<div class="mt-6 rounded-lg bg-green-500/10 p-4 text-center">
							<p class="text-sm text-green-400">All clear! No open moderation items.</p>
						</div>
					{/if}
				{/if}

			<!-- ==================== MESSAGE REPORTS ==================== -->
			{:else if currentTab === 'message_reports'}
				<div class="mb-6 flex items-center justify-between">
					<h1 class="text-2xl font-bold text-text-primary">Message Reports</h1>
					<button class="btn-secondary text-sm" onclick={loadMessageReports} disabled={messageReportsOp.loading}>
						{messageReportsOp.loading ? 'Loading...' : 'Refresh'}
					</button>
				</div>

				{#if messageReportsOp.loading && messageReports.length === 0}
					<p class="text-text-muted">Loading message reports...</p>
				{:else if messageReports.length === 0}
					<p class="text-text-muted">No message reports found.</p>
				{:else}
					<div class="space-y-3">
						{#each messageReports as report (report.id)}
							<div class="rounded-lg bg-bg-secondary p-4">
								<div class="flex items-start justify-between">
									<div class="min-w-0 flex-1">
										<div class="flex items-center gap-2">
											<span class="rounded px-1.5 py-0.5 text-2xs font-bold {statusBadgeClass(report.status)}">{report.status}</span>
											<span class="text-sm font-medium text-text-primary">Message Report</span>
										</div>
										<p class="mt-1 text-sm text-text-secondary"><strong>Reason:</strong> {report.reason}</p>
										<p class="mt-0.5 text-xs text-text-muted">
											Reporter: {report.reporter_name ?? report.reporter_id.slice(0, 8)} &middot;
											Channel: {report.channel_id.slice(0, 8)}... &middot;
											{formatDate(report.created_at)}
										</p>
									</div>
									{#if report.status === 'admin_pending'}
										<button
											class="shrink-0 rounded bg-brand-500 px-2 py-1 text-xs font-medium text-white hover:bg-brand-600"
											onclick={() => openResolve('message_report', report.id)}
										>
											Resolve
										</button>
									{/if}
								</div>
							</div>
						{/each}
					</div>
				{/if}

			<!-- ==================== USER REPORTS ==================== -->
			{:else if currentTab === 'user_reports'}
				<div class="mb-6 flex items-center justify-between">
					<h1 class="text-2xl font-bold text-text-primary">User Reports</h1>
					<button class="btn-secondary text-sm" onclick={loadUserReports} disabled={userReportsOp.loading}>
						{userReportsOp.loading ? 'Loading...' : 'Refresh'}
					</button>
				</div>

				{#if userReportsOp.loading && userReports.length === 0}
					<p class="text-text-muted">Loading user reports...</p>
				{:else if userReports.length === 0}
					<p class="text-text-muted">No user reports found.</p>
				{:else}
					<div class="space-y-3">
						{#each userReports as report (report.id)}
							<div class="rounded-lg bg-bg-secondary p-4">
								<div class="flex items-start justify-between">
									<div class="min-w-0 flex-1">
										<div class="flex items-center gap-2">
											<span class="rounded px-1.5 py-0.5 text-2xs font-bold {statusBadgeClass(report.status)}">{report.status}</span>
											<span class="text-sm font-medium text-text-primary">
												{report.reported_user_name ?? report.reported_user_id.slice(0, 8)}
											</span>
										</div>
										<p class="mt-1 text-sm text-text-secondary"><strong>Reason:</strong> {report.reason}</p>
										<p class="mt-0.5 text-xs text-text-muted">
											Reporter: {report.reporter_name ?? report.reporter_id.slice(0, 8)} &middot;
											{formatDate(report.created_at)}
										</p>
										{#if report.notes}
											<p class="mt-1 text-xs text-text-muted italic">Notes: {report.notes}</p>
										{/if}
									</div>
									{#if report.status === 'open'}
										<div class="flex shrink-0 gap-1">
											<button
												class="rounded bg-green-500 px-2 py-1 text-xs font-medium text-white hover:bg-green-600"
												onclick={() => openResolve('user_report', report.id)}
											>
												Resolve
											</button>
										</div>
									{/if}
								</div>
							</div>
						{/each}
					</div>
				{/if}

			<!-- ==================== ISSUES ==================== -->
			{:else if currentTab === 'issues'}
				<div class="mb-4 flex items-center justify-between">
					<h1 class="text-2xl font-bold text-text-primary">Reported Issues</h1>
					<div class="flex gap-2">
						<button class="btn-secondary text-sm" onclick={exportIssues} disabled={issueExportOp.loading}>
							{issueExportOp.loading ? 'Exporting...' : 'Export All'}
						</button>
						<button class="btn-secondary text-sm" onclick={loadIssues} disabled={issuesOp.loading}>
							{issuesOp.loading ? 'Loading...' : 'Refresh'}
						</button>
					</div>
				</div>
				<div class="mb-4 flex flex-wrap gap-1.5">
					{#each issueFilters as filter (filter.value)}
						<button
							class="rounded-full px-3 py-1 text-xs font-medium transition-colors {issueFilter === filter.value ? 'bg-brand-500 text-white' : 'bg-bg-modifier text-text-muted hover:text-text-secondary'}"
							onclick={() => setIssueFilter(filter.value)}
						>
							{filter.label}
						</button>
					{/each}
				</div>

				{#if isAdmin}
					<div class="mb-4 rounded-lg bg-bg-secondary p-4">
						<div class="flex flex-col gap-3 lg:flex-row lg:items-end">
							<div class="flex-1">
								<h2 class="text-sm font-semibold text-text-primary">Temporary Issue API Access</h2>
								<p class="mt-1 text-xs text-text-muted">
									Remote endpoint: <code class="rounded bg-bg-primary px-1 py-0.5">/api/v1/support/issues</code>
								</p>
							</div>
							<div>
								<label for="issue-token-hours" class="mb-1 block text-xs font-bold uppercase text-text-muted">Hours</label>
								<input id="issue-token-hours" type="number" class="input w-24" bind:value={issueTokenHours} min="1" max="168" />
							</div>
							<div class="min-w-0 flex-1">
								<label for="issue-token-note" class="mb-1 block text-xs font-bold uppercase text-text-muted">Note</label>
								<input id="issue-token-note" type="text" class="input w-full" bind:value={issueTokenNote} maxlength="200" placeholder="e.g., maintainer support" />
							</div>
							<button class="btn-secondary text-sm" onclick={createIssueToken} disabled={issueTokenOp.loading || issueTokenHours < 1 || issueTokenHours > 168}>
								{issueTokenOp.loading ? 'Creating...' : 'Create Token'}
							</button>
						</div>

						{#if createdIssueToken}
							<div class="mt-3 rounded bg-bg-primary p-3">
								<div class="mb-2 flex items-center justify-between gap-2">
									<p class="text-xs font-semibold text-yellow-400">Token shown once</p>
									<button class="text-xs text-brand-400 hover:text-brand-300" onclick={copyIssueToken}>Copy</button>
								</div>
								<code class="block break-all text-xs text-text-secondary">{createdIssueToken}</code>
							</div>
						{/if}

						{#if issueTokens.length > 0}
							<div class="mt-3 space-y-2">
								{#each issueTokens.slice(0, 5) as token (token.id)}
									<div class="flex items-center justify-between gap-3 rounded bg-bg-primary px-3 py-2 text-xs">
										<div class="min-w-0">
											<p class="truncate text-text-secondary">{token.note || 'Issue access token'}</p>
											<p class="text-text-muted">
												Expires {formatDate(token.expires_at)}
												{#if token.last_used_at} &middot; Used {formatDate(token.last_used_at)}{/if}
												{#if token.revoked_at} &middot; Revoked{/if}
											</p>
										</div>
										{#if !token.revoked_at && new Date(token.expires_at) > new Date()}
											<button class="shrink-0 text-xs text-red-400 hover:text-red-300" onclick={() => revokeIssueToken(token.id)}>Revoke</button>
										{/if}
									</div>
								{/each}
							</div>
						{/if}
					</div>
				{/if}

				{#if issuesOp.loading && issues.length === 0}
					<p class="text-text-muted">Loading issues...</p>
				{:else if issues.length === 0}
					<p class="text-text-muted">No reported issues found.</p>
				{:else}
					<div class="space-y-3">
						{#each issues as issue (issue.id)}
							<div class="rounded-lg bg-bg-secondary p-4">
								<div class="flex items-start justify-between">
									<div class="min-w-0 flex-1">
										<div class="flex items-center gap-2">
											<span class="rounded px-1.5 py-0.5 text-2xs font-bold {statusBadgeClass(issue.status)}">{issue.status}</span>
											<span class="rounded bg-bg-modifier px-1.5 py-0.5 text-2xs text-text-muted">{issue.category}</span>
											<span class="text-sm font-medium text-text-primary">{issue.title}</span>
										</div>
										<p class="mt-1 text-sm text-text-secondary">{issue.description}</p>
										<p class="mt-0.5 text-xs text-text-muted">
											Reporter: {issue.reporter_name ?? issue.reporter_id.slice(0, 8)} &middot;
											{formatDate(issue.created_at)}
										</p>
										{#if issue.notes}
											<p class="mt-1 text-xs text-text-muted italic">Notes: {issue.notes}</p>
										{/if}
									</div>
									{#if issue.status === 'open' || issue.status === 'in_progress'}
										<div class="flex shrink-0 gap-1">
											{#if issue.status === 'open'}
												<button
													class="rounded bg-yellow-500 px-2 py-1 text-xs font-medium text-white hover:bg-yellow-600"
													onclick={() => { resolveType = 'issue'; resolveId = issue.id; resolveNotes = ''; submitResolve('in_progress'); }}
												>
													In Progress
												</button>
											{/if}
											<button
												class="rounded bg-green-500 px-2 py-1 text-xs font-medium text-white hover:bg-green-600"
												onclick={() => openResolve('issue', issue.id)}
											>
												Resolve
											</button>
										</div>
									{/if}
								</div>
							</div>
						{/each}
					</div>
				{/if}
			{/if}
		</div>
	</div>
</div>

	<!-- Resolve Modal -->
	{#if resolveModalOpen}
		<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
		<div class="fixed inset-0 z-[100] flex items-center justify-center bg-black/50" onclick={() => resolveModalOpen = false} onkeydown={(e) => e.key === 'Escape' && (resolveModalOpen = false)} role="dialog" aria-modal="true" aria-labelledby="resolve-item-title" tabindex="-1">
			<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
			<div class="w-96 rounded-lg bg-bg-secondary p-4 shadow-xl" onclick={(e) => e.stopPropagation()} onkeydown={() => {}} role="document" tabindex="-1">
			<h3 id="resolve-item-title" class="mb-3 text-lg font-semibold text-text-primary">Resolve Item</h3>
			<textarea
				class="mb-3 w-full rounded-md border border-bg-modifier bg-bg-primary p-2 text-sm text-text-primary placeholder:text-text-muted focus:border-brand-500 focus:outline-none"
				placeholder="Optional notes..."
				rows="3"
				bind:value={resolveNotes}
			></textarea>
			<div class="flex justify-end gap-2">
				<button
					class="rounded-md px-3 py-1.5 text-sm text-text-muted hover:text-text-primary"
					onclick={() => resolveModalOpen = false}
				>Cancel</button>
				<button
					class="rounded-md bg-text-muted/30 px-3 py-1.5 text-sm font-medium text-text-muted hover:bg-text-muted/50"
					disabled={resolving}
					onclick={() => submitResolve('dismissed')}
				>Dismiss</button>
				<button
					class="rounded-md bg-green-500 px-3 py-1.5 text-sm font-medium text-white hover:bg-green-600"
					disabled={resolving}
					onclick={() => submitResolve('resolved')}
				>{resolving ? 'Resolving...' : 'Resolve'}</button>
			</div>
		</div>
	</div>
{/if}
{/if}
