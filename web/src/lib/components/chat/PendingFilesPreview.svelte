<script lang="ts">
	interface Props {
		files: File[];
		altTexts: Record<number, string>;
		maxFileSizeBytes: number;
		uploading: boolean;
		sendDisabled: boolean;
		sendLabel: string;
		onclear: () => void;
		onremove: (index: number) => void;
		onsend: () => void;
	}

	let { files, altTexts = $bindable({}), maxFileSizeBytes, uploading, sendDisabled, sendLabel, onclear, onremove, onsend }: Props = $props();

	function formatFileSize(bytes: number): string {
		if (bytes < 1024) return `${bytes} B`;
		if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
		if (bytes < 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
		return `${(bytes / (1024 * 1024 * 1024)).toFixed(2)} GB`;
	}
</script>

<div class="mb-2 rounded-lg bg-bg-secondary p-3">
	<div class="mb-2 flex items-center justify-between">
		<span class="text-xs font-semibold text-text-muted">
			{files.length} file{files.length > 1 ? 's' : ''} attached
			<span class="ml-1 font-normal text-text-muted">(max {formatFileSize(maxFileSizeBytes)})</span>
		</span>
		<div class="flex items-center gap-2">
			<button class="text-xs text-text-muted hover:text-text-primary" onclick={onclear}>Clear all</button>
			<button class="btn-primary text-xs px-3 py-1" onclick={onsend} disabled={sendDisabled}>
				{uploading ? 'Uploading...' : sendLabel}
			</button>
		</div>
	</div>
	<div class="space-y-1.5">
		{#each files as file, i (file.name + i)}
			{@const overLimit = file.size > maxFileSizeBytes}
			{@const isImage = file.type.startsWith('image/')}
			<div class="rounded {overLimit ? 'bg-red-500/10' : 'bg-bg-primary'}">
				<div class="flex items-center gap-2 px-2 py-1.5">
					<svg class="h-4 w-4 shrink-0 {overLimit ? 'text-red-400' : 'text-text-muted'}" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
						<path d="M14 2H6a2 2 0 00-2 2v16a2 2 0 002 2h12a2 2 0 002-2V8z" />
						<polyline points="14 2 14 8 20 8" />
					</svg>
					<span class="flex-1 truncate text-xs {overLimit ? 'text-red-400' : 'text-text-primary'}">{file.name}</span>
					<span class="shrink-0 text-2xs {overLimit ? 'font-semibold text-red-400' : 'text-text-muted'}">
						{formatFileSize(file.size)}
						{#if overLimit}
							-- exceeds limit
						{/if}
					</span>
					<button class="shrink-0 text-text-muted hover:text-text-primary" onclick={() => onremove(i)} title="Remove file">
						<svg class="h-3.5 w-3.5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
							<path d="M6 18L18 6M6 6l12 12" />
						</svg>
					</button>
				</div>
				{#if isImage}
					<div class="px-2 pb-1.5">
						<input
							type="text"
							class="w-full rounded border border-bg-floating bg-bg-secondary px-2 py-1 text-2xs text-text-primary outline-none placeholder:text-text-muted focus:border-text-link"
							placeholder="Alt text (describe this image for accessibility)"
							value={altTexts[i] ?? ''}
							oninput={(e) => { altTexts = { ...altTexts, [i]: (e.target as HTMLInputElement).value }; }}
						/>
					</div>
				{/if}
			</div>
		{/each}
	</div>
</div>
