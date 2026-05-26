<script lang="ts">
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { api } from '$lib/api/client';
	import { addToast } from '$lib/stores/toast';
	import { canManageGuild } from '$lib/stores/permissions';
	import { loadGuildEvents } from '$lib/stores/guildEvents';
	import { confirmAction } from '$lib/stores/confirm';
	import { getErrorMessage } from '$lib/utils/apiError';
	import type { EventRSVP, GuildEvent } from '$lib/types';

	const guildId = $derived($page.params.guildId);

	let events = $state<GuildEvent[]>([]);
	let selectedEvent = $state<GuildEvent | null>(null);
	let rsvps = $state<EventRSVP[]>([]);
	let loading = $state(true);
	let saving = $state(false);
	let error = $state('');
	let editing = $state(false);

	let editName = $state('');
	let editDescription = $state('');
	let editLocation = $state('');
	let editStart = $state('');
	let editEnd = $state('');

	$effect(() => {
		if (guildId) loadEvents();
	});

	async function loadEvents() {
		if (!guildId) return;
		loading = true;
		error = '';
		try {
			events = await api.getGuildEvents(guildId, { status: 'scheduled', limit: 100 });
			if (selectedEvent) {
				const match = events.find((event) => event.id === selectedEvent?.id);
				selectedEvent = match ?? events[0] ?? null;
			} else {
				selectedEvent = events[0] ?? null;
			}
			if (selectedEvent) await loadSelectedEvent(selectedEvent.id);
		} catch (err: unknown) {
			error = getErrorMessage(err, 'Failed to load events');
		} finally {
			loading = false;
		}
	}

	async function loadSelectedEvent(eventId: string) {
		if (!guildId) return;
		try {
			selectedEvent = await api.getGuildEvent(guildId, eventId);
			rsvps = await api.getEventRsvps(guildId, eventId);
			startEditingState(selectedEvent);
		} catch (err: unknown) {
			addToast(getErrorMessage(err, 'Failed to load event details'), 'error');
		}
	}

	function startEditingState(event: GuildEvent) {
		editName = event.name;
		editDescription = event.description ?? '';
		editLocation = event.location ?? '';
		editStart = toDatetimeLocal(event.scheduled_start);
		editEnd = event.scheduled_end ? toDatetimeLocal(event.scheduled_end) : '';
	}

	function toDatetimeLocal(value: string) {
		const date = new Date(value);
		const offsetMs = date.getTimezoneOffset() * 60_000;
		return new Date(date.getTime() - offsetMs).toISOString().slice(0, 16);
	}

	async function setRsvp(status: 'interested' | 'going' | null) {
		if (!guildId || !selectedEvent) return;
		try {
			if (status) {
				await api.rsvpEvent(guildId, selectedEvent.id, status);
			} else {
				await api.deleteRsvp(guildId, selectedEvent.id);
			}
			await loadSelectedEvent(selectedEvent.id);
			await loadGuildEvents(guildId);
		} catch (err: unknown) {
			addToast(getErrorMessage(err, 'Failed to update RSVP'), 'error');
		}
	}

	async function saveEvent() {
		if (!guildId || !selectedEvent || !editName.trim() || !editStart) return;
		saving = true;
		try {
			const updated = await api.updateGuildEvent(guildId, selectedEvent.id, {
				name: editName.trim(),
				description: editDescription.trim(),
				location: editLocation.trim(),
				scheduled_start: new Date(editStart).toISOString(),
				scheduled_end: editEnd ? new Date(editEnd).toISOString() : undefined
			});
			events = events.map((event) => event.id === updated.id ? updated : event);
			selectedEvent = updated;
			editing = false;
			await loadGuildEvents(guildId);
			addToast('Event updated', 'success');
		} catch (err: unknown) {
			addToast(getErrorMessage(err, 'Failed to update event'), 'error');
		} finally {
			saving = false;
		}
	}

	async function deleteEvent() {
		if (!guildId || !selectedEvent) return;
		if (!(await confirmAction({ title: 'Delete Event', message: `Delete "${selectedEvent.name}"?`, confirmLabel: 'Delete Event' }))) return;
		try {
			await api.deleteGuildEvent(guildId, selectedEvent.id);
			events = events.filter((event) => event.id !== selectedEvent?.id);
			selectedEvent = events[0] ?? null;
			rsvps = [];
			await loadGuildEvents(guildId);
			addToast('Event deleted', 'info');
		} catch (err: unknown) {
			addToast(getErrorMessage(err, 'Failed to delete event'), 'error');
		}
	}
</script>

<svelte:head>
	<title>Events - AmityVox</title>
</svelte:head>

<div class="flex h-full flex-col bg-bg-primary">
	<div class="flex min-h-14 items-center gap-3 border-b border-bg-floating px-4">
		<button class="rounded p-2 text-text-muted hover:bg-bg-modifier hover:text-text-primary" onclick={() => goto(`/app/guilds/${guildId}`)} title="Back to server">
			<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24"><path d="M15 19l-7-7 7-7" /></svg>
		</button>
		<h1 class="text-base font-semibold text-text-primary">Scheduled Events</h1>
	</div>

	{#if loading}
		<div class="flex flex-1 items-center justify-center">
			<div class="h-6 w-6 animate-spin rounded-full border-2 border-brand-500 border-t-transparent"></div>
		</div>
	{:else if error}
		<div class="flex flex-1 flex-col items-center justify-center gap-3">
			<p class="text-sm text-red-400">{error}</p>
			<button class="btn-secondary text-sm" onclick={loadEvents}>Retry</button>
		</div>
	{:else}
		<div class="grid min-h-0 flex-1 grid-cols-[minmax(220px,320px)_1fr]">
			<aside class="overflow-y-auto border-r border-bg-floating p-3">
				{#if events.length === 0}
					<p class="rounded bg-bg-secondary p-4 text-sm text-text-muted">No scheduled events.</p>
				{:else}
					<div class="space-y-2">
						{#each events as event (event.id)}
							<button
								class="w-full rounded-lg border p-3 text-left transition-colors {selectedEvent?.id === event.id ? 'border-brand-500 bg-brand-500/10' : 'border-bg-modifier bg-bg-secondary hover:border-text-muted'}"
								onclick={() => { editing = false; loadSelectedEvent(event.id); }}
							>
								<div class="truncate text-sm font-semibold text-text-primary">{event.name}</div>
								<div class="mt-1 text-xs text-text-muted">{new Date(event.scheduled_start).toLocaleString()}</div>
								<div class="mt-2 text-xs text-text-muted">{event.interested_count} interested</div>
							</button>
						{/each}
					</div>
				{/if}
			</aside>

			<main class="overflow-y-auto p-6">
				{#if selectedEvent}
					<div class="mx-auto max-w-3xl space-y-5">
						<div class="flex items-start justify-between gap-4">
							<div>
								<h2 class="text-xl font-semibold text-text-primary">{selectedEvent.name}</h2>
								<p class="mt-1 text-sm text-text-muted">{new Date(selectedEvent.scheduled_start).toLocaleString()}</p>
							</div>
							{#if $canManageGuild}
								<div class="flex gap-2">
									<button class="btn-secondary text-sm" onclick={() => { editing = !editing; startEditingState(selectedEvent!); }}>
										{editing ? 'Cancel' : 'Edit'}
									</button>
									<button class="rounded bg-red-500 px-3 py-1.5 text-sm font-medium text-white hover:bg-red-600" onclick={deleteEvent}>Delete</button>
								</div>
							{/if}
						</div>

						{#if editing}
							<div class="space-y-3 rounded-lg bg-bg-secondary p-4">
								<input class="input w-full" bind:value={editName} maxlength="100" />
								<textarea class="input min-h-24 w-full resize-y" bind:value={editDescription} maxlength="1000"></textarea>
								<input class="input w-full" bind:value={editLocation} maxlength="200" placeholder="Location" />
								<div class="grid gap-3 sm:grid-cols-2">
									<input class="input w-full" type="datetime-local" bind:value={editStart} />
									<input class="input w-full" type="datetime-local" bind:value={editEnd} />
								</div>
								<div class="flex justify-end">
									<button class="btn-primary text-sm" onclick={saveEvent} disabled={saving || !editName.trim() || !editStart}>
										{saving ? 'Saving...' : 'Save Event'}
									</button>
								</div>
							</div>
						{:else}
							<div class="rounded-lg bg-bg-secondary p-4">
								{#if selectedEvent.description}
									<p class="whitespace-pre-wrap text-sm text-text-secondary">{selectedEvent.description}</p>
								{:else}
									<p class="text-sm text-text-muted">No description.</p>
								{/if}
								{#if selectedEvent.location}
									<p class="mt-3 text-sm text-text-muted">Location: {selectedEvent.location}</p>
								{/if}
							</div>

							<div class="flex flex-wrap gap-2">
								<button
									class="rounded px-3 py-1.5 text-sm {selectedEvent.user_rsvp === 'interested' ? 'bg-brand-500/20 text-brand-300' : 'bg-bg-secondary text-text-muted hover:text-text-primary'}"
									onclick={() => setRsvp(selectedEvent?.user_rsvp === 'interested' ? null : 'interested')}
								>
									Interested
								</button>
								<button
									class="rounded px-3 py-1.5 text-sm {selectedEvent.user_rsvp === 'going' ? 'bg-green-500/20 text-green-300' : 'bg-bg-secondary text-text-muted hover:text-text-primary'}"
									onclick={() => setRsvp(selectedEvent?.user_rsvp === 'going' ? null : 'going')}
								>
									Going
								</button>
							</div>

							<section class="rounded-lg bg-bg-secondary p-4">
								<h3 class="mb-3 text-sm font-semibold text-text-primary">RSVPs</h3>
								{#if rsvps.length === 0}
									<p class="text-sm text-text-muted">No RSVPs yet.</p>
								{:else}
									<div class="space-y-2">
										{#each rsvps as rsvp (`${rsvp.user_id}-${rsvp.status}`)}
											<div class="flex items-center justify-between rounded bg-bg-tertiary px-3 py-2 text-sm">
												<span class="text-text-secondary">{rsvp.user?.display_name || rsvp.user?.username || rsvp.user_id}</span>
												<span class="text-xs uppercase tracking-wide text-text-muted">{rsvp.status}</span>
											</div>
										{/each}
									</div>
								{/if}
							</section>
						{/if}
					</div>
				{:else}
					<div class="flex h-full items-center justify-center text-sm text-text-muted">Select an event.</div>
				{/if}
			</main>
		</div>
	{/if}
</div>
