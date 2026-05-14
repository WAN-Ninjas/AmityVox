<script lang="ts">
	import { api } from '$lib/api/client';
	import { addToast } from '$lib/stores/toast';
	import type { MessageReport } from '$lib/types';

	interface Props {
		guildId: string;
	}

	let { guildId }: Props = $props();

	let reports = $state<MessageReport[]>([]);
	let loadingReports = $state(false);
	let reportFilter = $state<string>('open');
	let loadedGuildId = $state<string | null>(null);

	$effect(() => {
		if (guildId && loadedGuildId !== guildId && !loadingReports) {
			loadReports();
		}
	});

	async function loadReports() {
		loadingReports = true;
		try {
			reports = await api.getReports(guildId, { status: reportFilter });
			loadedGuildId = guildId;
		} catch (err: any) {
			addToast(err.message || 'Failed to load reports', 'error');
		} finally {
			loadingReports = false;
		}
	}

	async function handleResolveReport(reportId: string, status: 'resolved' | 'dismissed') {
		try {
			const updated = await api.resolveReport(guildId, reportId, status);
			reports = reports.map((report) => report.id === reportId ? updated : report);
			addToast(status === 'resolved' ? 'Report resolved' : 'Report dismissed', 'success');
		} catch (err: any) {
			addToast(err.message || 'Failed to resolve report', 'error');
		}
	}

	function formatDate(iso: string): string {
		return new Date(iso).toLocaleString();
	}
</script>

<h1 class="mb-6 text-xl font-bold text-text-primary">Moderation</h1>

<div class="mb-6">
	<h2 class="mb-3 text-lg font-semibold text-text-primary">Message Reports</h2>
	<div class="mb-3 flex items-center gap-2">
		<select class="input" bind:value={reportFilter} onchange={loadReports}>
			<option value="open">Open</option>
			<option value="resolved">Resolved</option>
			<option value="dismissed">Dismissed</option>
			<option value="">All</option>
		</select>
	</div>

	{#if loadingReports}
		<p class="text-sm text-text-muted">Loading reports...</p>
	{:else if reports.length === 0}
		<p class="text-sm text-text-muted">No reports found.</p>
	{:else}
		<div class="space-y-2">
			{#each reports as report (report.id)}
				<div class="rounded-lg bg-bg-secondary p-3">
					<div class="flex items-center justify-between gap-3">
						<div>
							<span class="text-sm text-text-primary">{report.reason}</span>
							<div class="mt-1 flex flex-wrap gap-2 text-xs text-text-muted">
								<span class="rounded px-1.5 py-0.5 {report.status === 'open' ? 'bg-yellow-500/20 text-yellow-400' : report.status === 'resolved' ? 'bg-green-500/20 text-green-400' : 'bg-text-muted/20'}">
									{report.status}
								</span>
								<span>Channel: {report.channel_id.slice(0, 8)}...</span>
								<span>{formatDate(report.created_at)}</span>
							</div>
						</div>
						{#if report.status === 'open'}
							<div class="flex gap-2">
								<button class="text-xs text-green-400 hover:text-green-300" onclick={() => handleResolveReport(report.id, 'resolved')}>
									Resolve
								</button>
								<button class="text-xs text-text-muted hover:text-text-secondary" onclick={() => handleResolveReport(report.id, 'dismissed')}>
									Dismiss
								</button>
							</div>
						{/if}
					</div>
				</div>
			{/each}
		</div>
	{/if}
</div>
