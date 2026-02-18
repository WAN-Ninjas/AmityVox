<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api/client';
	import { addToast } from '$lib/stores/toast';

	interface MediaBreakdown {
		category: string;
		file_count: number;
		total_bytes: number;
		readable_size: string;
	}

	interface TableSize {
		name: string;
		size: string;
		row_count: number;
	}

	interface TopUploader {
		user_id: string;
		username: string;
		file_count: number;
		total_bytes: number;
		readable_size: string;
	}

	interface DailyUpload {
		date: string;
		file_count: number;
		total_bytes: number;
	}

	interface StorageData {
		total_files: number;
		total_bytes: number;
		total_readable: string;
		breakdown: MediaBreakdown[];
		database_size: string;
		tables: TableSize[];
		top_uploaders: TopUploader[];
		upload_trend_30d: DailyUpload[];
	}

	let loading = $state(true);
	let storage = $state<StorageData | null>(null);

	async function loadStorage() {
		loading = true;
		try {
			storage = await api.getAdminStorage();
		} catch {
			addToast('Failed to load storage data', 'error');
		}
		loading = false;
	}

	onMount(() => {
		loadStorage();
	});

	function categoryIcon(cat: string): string {
		switch (cat) {
			case 'images': return 'M4 16l4.586-4.586a2 2 0 012.828 0L16 16m-2-2l1.586-1.586a2 2 0 012.828 0L20 14m-6-6h.01M6 20h12a2 2 0 002-2V6a2 2 0 00-2-2H6a2 2 0 00-2 2v12a2 2 0 002 2z';
			case 'videos': return 'M15 10l4.553-2.276A1 1 0 0121 8.618v6.764a1 1 0 01-1.447.894L15 14M5 18h8a2 2 0 002-2V8a2 2 0 00-2-2H5a2 2 0 00-2 2v8a2 2 0 002 2z';
			case 'audio': return 'M9 19V6l12-3v13M9 19c0 1.105-1.343 2-3 2s-3-.895-3-2 1.343-2 3-2 3 .895 3 2zm12-3c0 1.105-1.343 2-3 2s-3-.895-3-2 1.343-2 3-2 3 .895 3 2zM9 10l12-3';
			case 'documents': return 'M7 21h10a2 2 0 002-2V9.414a1 1 0 00-.293-.707l-5.414-5.414A1 1 0 0012.586 3H7a2 2 0 00-2 2v14a2 2 0 002 2z';
			default: return 'M5 8h14M5 8a2 2 0 110-4h14a2 2 0 110 4M5 8v10a2 2 0 002 2h10a2 2 0 002-2V8m-9 4h4';
		}
	}

	function categoryColor(cat: string): string {
		switch (cat) {
			case 'images': return 'text-brand-400';
			case 'videos': return 'text-status-dnd';
			case 'audio': return 'text-status-online';
			case 'documents': return 'text-status-idle';
			default: return 'text-text-muted';
		}
	}

	function formatNumber(n: number): string {
		return n.toLocaleString();
	}
</script>

<div class="space-y-6">
	<!-- Header -->
	<div class="flex items-center justify-between">
		<div>
			<h2 class="text-xl font-bold text-text-primary">Storage Dashboard</h2>
			<p class="text-text-muted text-sm">File storage usage and database size breakdown</p>
		</div>
		<button class="btn-secondary text-sm px-3 py-1.5" onclick={loadStorage} disabled={loading}>
			{loading ? 'Loading...' : 'Refresh'}
		</button>
	</div>

	{#if loading && !storage}
		<div class="flex justify-center py-12">
			<div class="animate-spin w-8 h-8 border-2 border-brand-500 border-t-transparent rounded-full"></div>
		</div>
	{:else if storage}
		<!-- Summary Cards -->
		<div class="grid grid-cols-1 md:grid-cols-3 gap-4">
			<div class="bg-bg-tertiary rounded-lg p-5">
				<p class="text-text-muted text-sm">Total Files</p>
				<p class="text-2xl font-bold text-text-primary mt-1">{formatNumber(storage.total_files)}</p>
			</div>
			<div class="bg-bg-tertiary rounded-lg p-5">
				<p class="text-text-muted text-sm">Media Storage</p>
				<p class="text-2xl font-bold text-text-primary mt-1">{storage.total_readable}</p>
			</div>
			<div class="bg-bg-tertiary rounded-lg p-5">
				<p class="text-text-muted text-sm">Database Size</p>
				<p class="text-2xl font-bold text-text-primary mt-1">{storage.database_size}</p>
			</div>
		</div>

		<!-- Media Breakdown -->
		<div class="bg-bg-tertiary rounded-lg p-5">
			<h3 class="text-sm font-semibold text-text-secondary mb-4">Media Breakdown</h3>
			{#if storage.breakdown.length > 0}
				<div class="space-y-3">
					{#each storage.breakdown as item}
						{@const pct = storage.total_bytes > 0 ? (item.total_bytes / storage.total_bytes) * 100 : 0}
						<div>
							<div class="flex items-center justify-between mb-1">
								<div class="flex items-center gap-2">
									<svg class="w-4 h-4 {categoryColor(item.category)}" fill="none" stroke="currentColor" viewBox="0 0 24 24">
										<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d={categoryIcon(item.category)} />
									</svg>
									<span class="text-text-primary text-sm capitalize">{item.category}</span>
								</div>
								<div class="text-sm text-text-secondary">
									{formatNumber(item.file_count)} files - {item.readable_size}
								</div>
							</div>
							<div class="h-2 bg-bg-modifier rounded-full overflow-hidden">
								<div
									class="h-full rounded-full transition-all {item.category === 'images' ? 'bg-brand-400' : item.category === 'videos' ? 'bg-status-dnd' : item.category === 'audio' ? 'bg-status-online' : 'bg-status-idle'}"
									style="width: {Math.max(1, pct)}%"
								></div>
							</div>
						</div>
					{/each}
				</div>
			{:else}
				<p class="text-text-muted text-sm">No files uploaded yet.</p>
			{/if}
		</div>

		<div class="grid grid-cols-1 md:grid-cols-2 gap-6">
			<!-- Top Uploaders -->
			<div class="bg-bg-tertiary rounded-lg p-5">
				<h3 class="text-sm font-semibold text-text-secondary mb-4">Top Uploaders</h3>
				{#if storage.top_uploaders.length > 0}
					<div class="space-y-2">
						{#each storage.top_uploaders as uploader, i}
							<div class="flex items-center justify-between py-1.5 {i < storage.top_uploaders.length - 1 ? 'border-b border-bg-modifier' : ''}">
								<div class="flex items-center gap-2">
									<span class="text-text-muted text-xs w-5">{i + 1}.</span>
									<span class="text-text-primary text-sm">{uploader.username}</span>
								</div>
								<div class="text-right">
									<span class="text-text-secondary text-sm">{uploader.readable_size}</span>
									<span class="text-text-muted text-xs ml-1">({formatNumber(uploader.file_count)} files)</span>
								</div>
							</div>
						{/each}
					</div>
				{:else}
					<p class="text-text-muted text-sm">No upload data available.</p>
				{/if}
			</div>

			<!-- Database Tables -->
			<div class="bg-bg-tertiary rounded-lg p-5">
				<h3 class="text-sm font-semibold text-text-secondary mb-4">Database Tables (Top 10)</h3>
				{#if storage.tables.length > 0}
					<div class="space-y-2">
						{#each storage.tables.slice(0, 10) as table, i}
							<div class="flex items-center justify-between py-1.5 {i < Math.min(storage.tables.length, 10) - 1 ? 'border-b border-bg-modifier' : ''}">
								<span class="text-text-primary text-sm font-mono">{table.name}</span>
								<div class="text-right">
									<span class="text-text-secondary text-sm">{table.size}</span>
									<span class="text-text-muted text-xs ml-1">({formatNumber(table.row_count)} rows)</span>
								</div>
							</div>
						{/each}
					</div>
				{:else}
					<p class="text-text-muted text-sm">No table data available.</p>
				{/if}
			</div>
		</div>

		<!-- Upload Trend -->
		<div class="bg-bg-tertiary rounded-lg p-5">
			<h3 class="text-sm font-semibold text-text-secondary mb-4">Upload Trend (30 days)</h3>
			{#if storage.upload_trend_30d.length > 0}
				<div class="flex items-end gap-1 h-32">
					{#each storage.upload_trend_30d as day}
						{@const maxCount = Math.max(...storage.upload_trend_30d.map(d => d.file_count), 1)}
						<div class="flex-1 flex flex-col items-center">
							<div
								class="w-full bg-brand-500/60 rounded-t-sm hover:bg-brand-500 transition-colors cursor-default"
								style="height: {Math.max(2, (day.file_count / maxCount) * 100)}%"
								title="{day.date}: {day.file_count} files"
							></div>
						</div>
					{/each}
				</div>
				<div class="flex justify-between text-xs text-text-muted mt-2">
					<span>{storage.upload_trend_30d[0]?.date}</span>
					<span>{storage.upload_trend_30d[storage.upload_trend_30d.length - 1]?.date}</span>
				</div>
			{:else}
				<p class="text-text-muted text-sm">No upload data in the last 30 days.</p>
			{/if}
		</div>
	{/if}
</div>
