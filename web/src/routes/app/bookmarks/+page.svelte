<script lang="ts">
	import { api } from '$lib/api/client';
	import { onMount } from 'svelte';
	import { addToast } from '$lib/stores/toast';
	import type { MessageBookmark } from '$lib/types';

	let bookmarks = $state<MessageBookmark[]>([]);
	let loading = $state(true);
	let error = $state('');
	let reminderOpenId = $state<string | null>(null);
	let customReminderDate = $state('');

	onMount(async () => {
		try {
			bookmarks = await api.getBookmarks();
		} catch (err: any) {
			error = err.message || 'Failed to load bookmarks';
		} finally {
			loading = false;
		}
	});

	async function removeBookmark(messageId: string) {
		try {
			await api.deleteBookmark(messageId);
			bookmarks = bookmarks.filter(b => b.message_id !== messageId);
		} catch (err: any) {
			error = err.message || 'Failed to remove bookmark';
		}
	}

	function formatDate(iso: string): string {
		return new Date(iso).toLocaleString();
	}

	function toggleReminderMenu(messageId: string) {
		if (reminderOpenId === messageId) {
			reminderOpenId = null;
			customReminderDate = '';
		} else {
			reminderOpenId = messageId;
			customReminderDate = '';
		}
	}

	async function setReminder(bookmark: MessageBookmark, reminderAt: string | null) {
		reminderOpenId = null;
		customReminderDate = '';
		try {
			const updated = await api.createBookmark(
				bookmark.message_id,
				bookmark.note ?? undefined,
				reminderAt ?? undefined
			);
			bookmarks = bookmarks.map(b =>
				b.message_id === bookmark.message_id
					? { ...b, reminder_at: updated.reminder_at, reminded: updated.reminded }
					: b
			);
			if (reminderAt) {
				addToast('Reminder set', 'success');
			} else {
				addToast('Reminder cleared', 'info');
			}
		} catch (err: any) {
			addToast('Failed to set reminder', 'error');
		}
	}

	function getPresetTime(offset: string): string {
		const now = new Date();
		switch (offset) {
			case '15m':
				return new Date(now.getTime() + 15 * 60 * 1000).toISOString();
			case '1h':
				return new Date(now.getTime() + 60 * 60 * 1000).toISOString();
			case '1d':
				return new Date(now.getTime() + 24 * 60 * 60 * 1000).toISOString();
			default:
				return now.toISOString();
		}
	}

	function handleCustomReminder(bookmark: MessageBookmark) {
		if (!customReminderDate) return;
		const iso = new Date(customReminderDate).toISOString();
		setReminder(bookmark, iso);
	}
</script>

<svelte:head>
	<title>Saved Messages — AmityVox</title>
</svelte:head>

<div class="flex h-full flex-col">
	<div class="flex h-12 items-center border-b border-bg-modifier px-4">
		<svg class="mr-2 h-5 w-5 text-text-muted" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
			<path d="M5 5a2 2 0 012-2h10a2 2 0 012 2v16l-7-3.5L5 21V5z" />
		</svg>
		<h1 class="text-base font-semibold text-text-primary">Saved Messages</h1>
	</div>

	<div class="flex-1 overflow-y-auto p-6">
		<div class="mx-auto max-w-2xl">
			{#if loading}
				<p class="text-sm text-text-muted">Loading saved messages...</p>
			{:else if error}
				<p class="text-sm text-red-400">{error}</p>
			{:else if bookmarks.length === 0}
				<div class="flex flex-col items-center justify-center py-20 text-center">
					<svg class="mb-4 h-16 w-16 text-text-muted opacity-50" fill="none" stroke="currentColor" stroke-width="1.5" viewBox="0 0 24 24">
						<path d="M5 5a2 2 0 012-2h10a2 2 0 012 2v16l-7-3.5L5 21V5z" />
					</svg>
					<h2 class="mb-2 text-lg font-semibold text-text-primary">No saved messages</h2>
					<p class="text-sm text-text-muted">Right-click any message and select "Bookmark" to save it here.</p>
				</div>
			{:else}
				<div class="space-y-3">
					{#each bookmarks as bookmark (bookmark.message_id)}
						<div class="group rounded-lg bg-bg-secondary p-4">
							<div class="flex items-start justify-between">
								<div class="flex-1">
									{#if bookmark.message}
										<div class="mb-1 flex items-center gap-2">
											<span class="text-sm font-semibold text-text-primary">
												{bookmark.message.author?.display_name ?? bookmark.message.author?.username ?? 'Unknown'}
											</span>
											<span class="text-xs text-text-muted">{formatDate(bookmark.message.created_at)}</span>
										</div>
										<p class="text-sm text-text-secondary">{bookmark.message.content ?? '[No content]'}</p>
									{:else}
										<p class="text-sm text-text-muted">Message unavailable</p>
									{/if}
									{#if bookmark.note}
										<p class="mt-2 border-l-2 border-brand-500 pl-2 text-xs text-text-muted italic">{bookmark.note}</p>
									{/if}
									{#if bookmark.reminder_at && !bookmark.reminded}
										<div class="mt-2 flex items-center gap-1 text-xs text-yellow-400">
											<svg class="h-3.5 w-3.5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
												<circle cx="12" cy="12" r="10" />
												<path d="M12 6v6l4 2" />
											</svg>
											<span>Reminder: {formatDate(bookmark.reminder_at)}</span>
										</div>
									{/if}
								</div>
								<div class="ml-3 flex shrink-0 items-center gap-1 opacity-0 transition-opacity group-hover:opacity-100">
									<!-- Reminder button -->
									<div class="relative">
										<button
											class="rounded p-1 text-text-muted hover:bg-bg-modifier hover:text-text-primary"
											title="Set reminder"
											onclick={() => toggleReminderMenu(bookmark.message_id)}
										>
											<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
												<circle cx="12" cy="12" r="10" />
												<path d="M12 6v6l4 2" />
											</svg>
										</button>
										{#if reminderOpenId === bookmark.message_id}
											<!-- svelte-ignore a11y_no_static_element_interactions -->
											<div
												class="absolute right-0 top-8 z-20 w-56 rounded-lg bg-bg-floating p-2 shadow-xl"
												onclick={(e) => e.stopPropagation()}
											>
												<button
													class="w-full rounded px-3 py-1.5 text-left text-sm text-text-secondary hover:bg-bg-modifier"
													onclick={() => setReminder(bookmark, getPresetTime('15m'))}
												>
													In 15 minutes
												</button>
												<button
													class="w-full rounded px-3 py-1.5 text-left text-sm text-text-secondary hover:bg-bg-modifier"
													onclick={() => setReminder(bookmark, getPresetTime('1h'))}
												>
													In 1 hour
												</button>
												<button
													class="w-full rounded px-3 py-1.5 text-left text-sm text-text-secondary hover:bg-bg-modifier"
													onclick={() => setReminder(bookmark, getPresetTime('1d'))}
												>
													In 1 day
												</button>
												<div class="my-1 border-t border-bg-modifier"></div>
												<div class="px-3 py-1.5">
													<label class="mb-1 block text-xs text-text-muted">Custom date/time</label>
													<input
														type="datetime-local"
														class="w-full rounded bg-bg-primary px-2 py-1 text-xs text-text-primary"
														bind:value={customReminderDate}
														onkeydown={(e) => { if (e.key === 'Enter') handleCustomReminder(bookmark); }}
													/>
													{#if customReminderDate}
														<button
															class="mt-1 w-full rounded bg-brand-500 px-2 py-1 text-xs text-white hover:bg-brand-600"
															onclick={() => handleCustomReminder(bookmark)}
														>
															Set reminder
														</button>
													{/if}
												</div>
												{#if bookmark.reminder_at}
													<div class="my-1 border-t border-bg-modifier"></div>
													<button
														class="w-full rounded px-3 py-1.5 text-left text-sm text-red-400 hover:bg-bg-modifier"
														onclick={() => setReminder(bookmark, null)}
													>
														Clear reminder
													</button>
												{/if}
											</div>
										{/if}
									</div>
									<!-- Remove button -->
									<button
										class="text-xs text-red-400 hover:text-red-300"
										onclick={() => removeBookmark(bookmark.message_id)}
									>
										Remove
									</button>
								</div>
							</div>
							<p class="mt-1 text-xs text-text-muted">Saved {formatDate(bookmark.created_at)}</p>
						</div>
					{/each}
				</div>
			{/if}
		</div>
	</div>
</div>
