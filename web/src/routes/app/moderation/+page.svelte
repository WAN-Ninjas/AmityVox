<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { api } from '$lib/api/client';
	import { addToast } from '$lib/stores/toast';
	import type {
		ModerationStats,
		ModerationMessageReport,
		UserReport,
		ReportedIssue
	} from '$lib/types';

	type Tab = 'dashboard' | 'message_reports' | 'user_reports' | 'issues';
	let currentTab = $state<Tab>('dashboard');

	// --- Dashboard ---
	let stats = $state<ModerationStats | null>(null);
	let loadingStats = $state(false);

	// --- Message Reports ---
	let messageReports = $state<ModerationMessageReport[]>([]);
	let loadingMessageReports = $state(false);
	let messageReportsLoaded = $state(false);

	// --- User Reports ---
	let userReports = $state<UserReport[]>([]);
	let loadingUserReports = $state(false);
	let userReportsLoaded = $state(false);

	// --- Issues ---
	let issues = $state<ReportedIssue[]>([]);
	let loadingIssues = $state(false);
	let issuesLoaded = $state(false);

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
		loadingStats = true;
		try {
			stats = await api.getModerationStats();
		} catch {
			addToast('Failed to load moderation stats', 'error');
		} finally {
			loadingStats = false;
		}
	}

	async function loadMessageReports() {
		loadingMessageReports = true;
		try {
			messageReports = await api.getModerationMessageReports();
			messageReportsLoaded = true;
		} catch {
			addToast('Failed to load message reports', 'error');
		} finally {
			loadingMessageReports = false;
		}
	}

	async function loadUserReports() {
		loadingUserReports = true;
		try {
			userReports = await api.getModerationUserReports();
			userReportsLoaded = true;
		} catch {
			addToast('Failed to load user reports', 'error');
		} finally {
			loadingUserReports = false;
		}
	}

	async function loadIssues() {
		loadingIssues = true;
		try {
			issues = await api.getModerationIssues();
			issuesLoaded = true;
		} catch {
			addToast('Failed to load issues', 'error');
		} finally {
			loadingIssues = false;
		}
	}

	function openResolve(type: 'message_report' | 'user_report' | 'issue', id: string) {
		resolveType = type;
		resolveId = id;
		resolveNotes = '';
		resolveModalOpen = true;
	}

	async function submitResolve(status: string) {
		resolving = true;
		try {
			if (resolveType === 'message_report') {
				await api.resolveModerationMessageReport(resolveId, status, resolveNotes || undefined);
				messageReports = messageReports.map(r => r.id === resolveId ? { ...r, status } : r);
			} else if (resolveType === 'user_report') {
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

	$effect(() => {
		if (currentTab === 'dashboard') loadStats();
		if (currentTab === 'message_reports' && !messageReportsLoaded) loadMessageReports();
		if (currentTab === 'user_reports' && !userReportsLoaded) loadUserReports();
		if (currentTab === 'issues' && !issuesLoaded) loadIssues();
	});

	onMount(() => {
		loadStats();
	});
</script>

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
					<button class="btn-secondary text-sm" onclick={loadStats} disabled={loadingStats}>
						{loadingStats ? 'Loading...' : 'Refresh'}
					</button>
				</div>

				{#if loadingStats && !stats}
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
					<button class="btn-secondary text-sm" onclick={loadMessageReports} disabled={loadingMessageReports}>
						{loadingMessageReports ? 'Loading...' : 'Refresh'}
					</button>
				</div>

				{#if loadingMessageReports && messageReports.length === 0}
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
					<button class="btn-secondary text-sm" onclick={loadUserReports} disabled={loadingUserReports}>
						{loadingUserReports ? 'Loading...' : 'Refresh'}
					</button>
				</div>

				{#if loadingUserReports && userReports.length === 0}
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
				<div class="mb-6 flex items-center justify-between">
					<h1 class="text-2xl font-bold text-text-primary">Reported Issues</h1>
					<button class="btn-secondary text-sm" onclick={loadIssues} disabled={loadingIssues}>
						{loadingIssues ? 'Loading...' : 'Refresh'}
					</button>
				</div>

				{#if loadingIssues && issues.length === 0}
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
	<div class="fixed inset-0 z-[100] flex items-center justify-center bg-black/50" onclick={() => resolveModalOpen = false} onkeydown={(e) => e.key === 'Escape' && (resolveModalOpen = false)} role="dialog" tabindex="-1">
		<div class="w-96 rounded-lg bg-bg-secondary p-4 shadow-xl" onclick={(e) => e.stopPropagation()} onkeydown={() => {}} role="document" tabindex="-1">
			<h3 class="mb-3 text-lg font-semibold text-text-primary">Resolve Item</h3>
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
