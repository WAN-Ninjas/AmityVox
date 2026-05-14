<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api/client';
	import { addToast } from '$lib/stores/toast';
	import { confirmAction } from '$lib/stores/confirm';
	import Modal from '$components/common/Modal.svelte';
	import type { Announcement, AnnouncementSeverity } from '$lib/types';

	let announcements = $state<Announcement[]>([]);
	let loadingAnnouncements = $state(false);
	let createAnnouncementModalOpen = $state(false);
	let editAnnouncementModalOpen = $state(false);
	let editingAnnouncement = $state<Announcement | null>(null);
	let announcementTitle = $state('');
	let announcementContent = $state('');
	let announcementSeverity = $state<AnnouncementSeverity>('info');
	let announcementExpiryHours = $state(0);
	let savingAnnouncement = $state(false);

	onMount(() => {
		loadAnnouncements();
	});

	async function loadAnnouncements() {
		loadingAnnouncements = true;
		try {
			announcements = await api.getAdminAnnouncements();
		} catch {
			announcements = [];
		} finally {
			loadingAnnouncements = false;
		}
	}

	function openCreateAnnouncement() {
		announcementTitle = '';
		announcementContent = '';
		announcementSeverity = 'info';
		announcementExpiryHours = 0;
		createAnnouncementModalOpen = true;
	}

	function openEditAnnouncement(announcement: Announcement) {
		editingAnnouncement = announcement;
		announcementTitle = announcement.title;
		announcementContent = announcement.content;
		editAnnouncementModalOpen = true;
	}

	async function handleCreateAnnouncement() {
		if (!announcementTitle.trim() || !announcementContent.trim()) return;
		savingAnnouncement = true;
		try {
			const announcement = await api.createAnnouncement({
				title: announcementTitle.trim(),
				content: announcementContent.trim(),
				severity: announcementSeverity,
				expires_in_hours: announcementExpiryHours || undefined
			});
			announcements = [announcement, ...announcements];
			createAnnouncementModalOpen = false;
			addToast('Announcement created', 'success');
		} catch {
			addToast('Failed to create announcement', 'error');
		} finally {
			savingAnnouncement = false;
		}
	}

	async function handleUpdateAnnouncement() {
		if (!editingAnnouncement) return;
		savingAnnouncement = true;
		try {
			const updated = await api.updateAnnouncement(editingAnnouncement.id, {
				title: announcementTitle.trim(),
				content: announcementContent.trim()
			});
			announcements = announcements.map((announcement) => announcement.id === updated.id ? updated : announcement);
			editAnnouncementModalOpen = false;
			editingAnnouncement = null;
			addToast('Announcement updated', 'success');
		} catch {
			addToast('Failed to update announcement', 'error');
		} finally {
			savingAnnouncement = false;
		}
	}

	async function handleToggleAnnouncement(announcement: Announcement) {
		try {
			const updated = await api.updateAnnouncement(announcement.id, { active: !announcement.active });
			announcements = announcements.map((value) => value.id === updated.id ? updated : value);
			addToast(updated.active ? 'Announcement activated' : 'Announcement deactivated', 'success');
		} catch {
			addToast('Failed to update announcement', 'error');
		}
	}

	async function handleDeleteAnnouncement(id: string) {
		if (!(await confirmAction({ title: 'Delete Announcement', message: 'Permanently delete this announcement?', confirmLabel: 'Delete' }))) return;
		try {
			await api.deleteAnnouncement(id);
			announcements = announcements.filter((announcement) => announcement.id !== id);
			addToast('Announcement deleted', 'success');
		} catch {
			addToast('Failed to delete announcement', 'error');
		}
	}

	function severityLabel(severity: AnnouncementSeverity): string {
		return severity.charAt(0).toUpperCase() + severity.slice(1);
	}

	function severityClasses(severity: AnnouncementSeverity): string {
		switch (severity) {
			case 'info': return 'bg-blue-500/20 text-blue-400';
			case 'warning': return 'bg-yellow-500/20 text-yellow-400';
			case 'critical': return 'bg-red-500/20 text-red-400';
		}
	}
</script>

<div class="mb-6 flex items-center justify-between">
	<h1 class="text-2xl font-bold text-text-primary">Announcements</h1>
	<button class="btn-primary text-sm" onclick={openCreateAnnouncement}>
		Create Announcement
	</button>
</div>

{#if loadingAnnouncements}
	<p class="text-sm text-text-muted">Loading announcements...</p>
{:else if announcements.length === 0}
	<div class="rounded-lg bg-bg-secondary p-6 text-center">
		<p class="text-sm text-text-muted">No announcements yet.</p>
		<p class="mt-1 text-xs text-text-muted">Create an announcement to notify all users on this instance.</p>
	</div>
{:else}
	<div class="space-y-3">
		{#each announcements as announcement (announcement.id)}
			<div class="rounded-lg bg-bg-secondary p-4">
				<div class="mb-2 flex items-start justify-between">
					<div class="flex items-center gap-2">
						<h3 class="text-sm font-semibold text-text-primary">{announcement.title}</h3>
						<span class="rounded px-1.5 py-0.5 text-2xs font-bold {severityClasses(announcement.severity)}">
							{severityLabel(announcement.severity)}
						</span>
						<span class="rounded px-1.5 py-0.5 text-2xs font-bold {announcement.active ? 'bg-green-500/20 text-green-400' : 'bg-gray-500/20 text-gray-400'}">
							{announcement.active ? 'Active' : 'Inactive'}
						</span>
					</div>
					<div class="flex items-center gap-2">
						<button
							class="text-xs text-brand-400 hover:text-brand-300"
							onclick={() => openEditAnnouncement(announcement)}
						>
							Edit
						</button>
						<button
							class="text-xs {announcement.active ? 'text-yellow-400 hover:text-yellow-300' : 'text-green-400 hover:text-green-300'}"
							onclick={() => handleToggleAnnouncement(announcement)}
						>
							{announcement.active ? 'Deactivate' : 'Activate'}
						</button>
						<button
							class="text-xs text-red-400 hover:text-red-300"
							onclick={() => handleDeleteAnnouncement(announcement.id)}
						>
							Delete
						</button>
					</div>
				</div>
				<p class="mb-2 text-sm text-text-secondary">{announcement.content}</p>
				<div class="flex gap-4 text-xs text-text-muted">
					<span>Created {new Date(announcement.created_at).toLocaleString()}</span>
					{#if announcement.expires_at}
						<span>Expires {new Date(announcement.expires_at).toLocaleString()}</span>
					{/if}
				</div>
			</div>
		{/each}
	</div>
{/if}

<!-- Create Announcement Modal -->
<Modal open={createAnnouncementModalOpen} title="Create Announcement" onclose={() => (createAnnouncementModalOpen = false)}>
	<div class="space-y-4">
		<div>
			<label for="admin-create-announcement-title" class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">Title</label>
			<input id="admin-create-announcement-title" type="text" class="input w-full" bind:value={announcementTitle} maxlength="200" placeholder="Announcement title" />
		</div>
		<div>
			<label for="admin-create-announcement-content" class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">Content</label>
			<textarea id="admin-create-announcement-content" class="input w-full" rows="4" bind:value={announcementContent} maxlength="2000" placeholder="Announcement content..."></textarea>
		</div>
		<div>
			<p class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">Severity</p>
			<div class="flex gap-4">
				<label class="flex items-center gap-2 text-sm text-text-secondary">
					<input type="radio" bind:group={announcementSeverity} value="info" class="accent-blue-500" />
					Info
				</label>
				<label class="flex items-center gap-2 text-sm text-text-secondary">
					<input type="radio" bind:group={announcementSeverity} value="warning" class="accent-yellow-500" />
					Warning
				</label>
				<label class="flex items-center gap-2 text-sm text-text-secondary">
					<input type="radio" bind:group={announcementSeverity} value="critical" class="accent-red-500" />
					Critical
				</label>
			</div>
		</div>
		<div>
			<label for="admin-create-announcement-expiry-hours" class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">Expires In (Hours)</label>
			<input id="admin-create-announcement-expiry-hours" type="number" class="input w-full" bind:value={announcementExpiryHours} min="0" max="8760" />
			<p class="mt-1 text-xs text-text-muted">Set to 0 for no automatic expiration.</p>
		</div>
		<div class="flex justify-end gap-2">
			<button class="btn-secondary text-sm" onclick={() => (createAnnouncementModalOpen = false)}>Cancel</button>
			<button
				class="btn-primary text-sm"
				onclick={handleCreateAnnouncement}
				disabled={savingAnnouncement || !announcementTitle.trim() || !announcementContent.trim()}
			>
				{savingAnnouncement ? 'Creating...' : 'Create Announcement'}
			</button>
		</div>
	</div>
</Modal>

<!-- Edit Announcement Modal -->
<Modal open={editAnnouncementModalOpen} title="Edit Announcement" onclose={() => (editAnnouncementModalOpen = false)}>
	<div class="space-y-4">
		<div>
			<label for="admin-edit-announcement-title" class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">Title</label>
			<input id="admin-edit-announcement-title" type="text" class="input w-full" bind:value={announcementTitle} maxlength="200" />
		</div>
		<div>
			<label for="admin-edit-announcement-content" class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">Content</label>
			<textarea id="admin-edit-announcement-content" class="input w-full" rows="4" bind:value={announcementContent} maxlength="2000"></textarea>
		</div>
		<div class="flex justify-end gap-2">
			<button class="btn-secondary text-sm" onclick={() => (editAnnouncementModalOpen = false)}>Cancel</button>
			<button
				class="btn-primary text-sm"
				onclick={handleUpdateAnnouncement}
				disabled={savingAnnouncement || !announcementTitle.trim() || !announcementContent.trim()}
			>
				{savingAnnouncement ? 'Saving...' : 'Save Changes'}
			</button>
		</div>
	</div>
</Modal>
