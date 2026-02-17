<script lang="ts">
	import type { ReportedIssue } from '$lib/types';
	import { api } from '$lib/api/client';

	let issues = $state<ReportedIssue[]>([]);
	let loading = $state(true);

	$effect(() => {
		api.getMyIssues()
			.then((data) => { issues = data; })
			.catch(() => {})
			.finally(() => { loading = false; });
	});

	const statusColors: Record<string, string> = {
		open: 'bg-yellow-500/20 text-yellow-400',
		in_progress: 'bg-blue-500/20 text-blue-400',
		resolved: 'bg-green-500/20 text-green-400',
		dismissed: 'bg-text-muted/20 text-text-muted'
	};

	const categoryIcons: Record<string, string> = {
		bug: 'M12 8v4m0 4h.01',
		suggestion: 'M9.663 17h4.673M12 3v1m6.364 1.636l-.707.707M21 12h-1M4 12H3m3.343-5.657l-.707-.707m2.828 9.9a5 5 0 117.072 0l-.548.547A3.374 3.374 0 0014 18.469V19a2 2 0 11-4 0v-.531c0-.895-.356-1.754-.988-2.386l-.548-.547z',
		abuse: 'M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-2.5L13.732 4c-.77-.833-1.964-.833-2.732 0L4.082 16.5c-.77.833.192 2.5 1.732 2.5z',
		general: 'M8 10h.01M12 10h.01M16 10h.01M9 16H5a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v8a2 2 0 01-2 2h-5l-5 5v-5z'
	};
</script>

<div class="rounded-lg bg-bg-secondary p-4">
	<div class="mb-3 flex items-center gap-2">
		<svg class="h-5 w-5 text-text-muted" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
			<path stroke-linecap="round" stroke-linejoin="round" d="M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2" />
		</svg>
		<h3 class="text-sm font-semibold text-text-primary">My Issues</h3>
	</div>

	{#if loading}
		<div class="flex items-center gap-2 py-4 text-sm text-text-muted">
			<div class="h-4 w-4 animate-spin rounded-full border-2 border-brand-500 border-t-transparent"></div>
			Loading...
		</div>
	{:else if issues.length === 0}
		<p class="py-2 text-sm text-text-muted">No issues reported yet.</p>
	{:else}
		<div class="space-y-2">
			{#each issues.slice(0, 5) as issue (issue.id)}
				<div class="rounded-md bg-bg-primary p-3">
					<div class="flex items-start justify-between gap-2">
						<div class="flex items-center gap-2">
							<svg class="h-4 w-4 shrink-0 text-text-muted" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
								<path stroke-linecap="round" stroke-linejoin="round" d={categoryIcons[issue.category] ?? categoryIcons.general} />
							</svg>
							<span class="text-sm font-medium text-text-primary">{issue.title}</span>
						</div>
						<span class="shrink-0 rounded px-1.5 py-0.5 text-2xs font-medium {statusColors[issue.status] ?? statusColors.open}">
							{issue.status.replace('_', ' ')}
						</span>
					</div>
					<p class="mt-1 line-clamp-2 text-xs text-text-muted">{issue.description}</p>
					<p class="mt-1.5 text-2xs text-text-muted">
						{new Date(issue.created_at).toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: 'numeric' })}
						{#if issue.notes}
							<span class="ml-2 text-text-secondary">Staff note: {issue.notes}</span>
						{/if}
					</p>
				</div>
			{/each}
			{#if issues.length > 5}
				<p class="text-center text-xs text-text-muted">+{issues.length - 5} more</p>
			{/if}
		</div>
	{/if}
</div>
