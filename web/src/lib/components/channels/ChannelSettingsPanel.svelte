<script lang="ts">
	import { api } from '$lib/api/client';
	import { addToast } from '$lib/stores/toast';
	import type { Channel, Role, ForumTag, GalleryTag } from '$lib/types';

	let {
		channel,
		roles = [],
		onUpdate
	}: {
		channel: Channel;
		roles: Role[];
		onUpdate: (updated: Channel) => void;
	} = $props();

	// Read-only settings
	let readOnly = $state(false);
	let readOnlyRoleIds = $state<string[]>([]);

	// Auto-archive settings
	let autoArchiveDuration = $state(0);

	// Forum-specific settings
	let forumDefaultSort = $state<string>('latest_activity');
	let forumPostGuidelines = $state('');
	let forumRequireTags = $state(false);
	let forumTags = $state<ForumTag[]>([]);
	let newTagName = $state('');
	let newTagEmoji = $state('');
	let newTagColor = $state('#6366f1');
	let creatingTag = $state(false);

	// Gallery-specific settings
	let galleryDefaultSort = $state<string>('newest');
	let galleryPostGuidelines = $state('');
	let galleryRequireTags = $state(false);
	let galleryTags = $state<GalleryTag[]>([]);
	let newGalleryTagName = $state('');
	let newGalleryTagEmoji = $state('');
	let newGalleryTagColor = $state('#6366f1');
	let creatingGalleryTag = $state(false);

	let saving = $state(false);

	const isForum = $derived(channel.channel_type === 'forum');
	const isGallery = $derived(channel.channel_type === 'gallery');

	// Initialize state from channel when it changes.
	$effect(() => {
		if (channel) {
			readOnly = (channel as any).read_only ?? false;
			readOnlyRoleIds = [...((channel as any).read_only_role_ids ?? [])];
			autoArchiveDuration = (channel as any).default_auto_archive_duration ?? 0;
			forumDefaultSort = channel.forum_default_sort ?? 'latest_activity';
			forumPostGuidelines = channel.forum_post_guidelines ?? '';
			forumRequireTags = channel.forum_require_tags ?? false;
			galleryDefaultSort = channel.gallery_default_sort ?? 'newest';
			galleryPostGuidelines = channel.gallery_post_guidelines ?? '';
			galleryRequireTags = channel.gallery_require_tags ?? false;
		}
	});

	// Load forum tags
	$effect(() => {
		if (isForum) {
			api.getForumTags(channel.id).then((t) => (forumTags = t)).catch(() => (forumTags = []));
		}
	});

	// Load gallery tags
	$effect(() => {
		if (isGallery) {
			api.getGalleryTags(channel.id).then((t) => (galleryTags = t)).catch(() => (galleryTags = []));
		}
	});

	async function createTag() {
		const name = newTagName.trim();
		if (!name) return;
		creatingTag = true;
		try {
			const tag = await api.createForumTag(channel.id, {
				name,
				emoji: newTagEmoji.trim() || undefined,
				color: newTagColor || undefined
			});
			forumTags = [...forumTags, tag];
			newTagName = '';
			newTagEmoji = '';
			addToast('Tag created', 'success');
		} catch (err: any) {
			addToast(err.message || 'Failed to create tag', 'error');
		} finally {
			creatingTag = false;
		}
	}

	async function deleteTag(tagId: string) {
		try {
			await api.deleteForumTag(channel.id, tagId);
			forumTags = forumTags.filter((t) => t.id !== tagId);
			addToast('Tag deleted', 'success');
		} catch (err: any) {
			addToast(err.message || 'Failed to delete tag', 'error');
		}
	}

	async function createGalleryTag() {
		const name = newGalleryTagName.trim();
		if (!name) return;
		creatingGalleryTag = true;
		try {
			const tag = await api.createGalleryTag(channel.id, {
				name,
				emoji: newGalleryTagEmoji.trim() || undefined,
				color: newGalleryTagColor || undefined
			});
			galleryTags = [...galleryTags, tag];
			newGalleryTagName = '';
			newGalleryTagEmoji = '';
			addToast('Tag created', 'success');
		} catch (err: any) {
			addToast(err.message || 'Failed to create tag', 'error');
		} finally {
			creatingGalleryTag = false;
		}
	}

	async function deleteGalleryTag(tagId: string) {
		try {
			await api.deleteGalleryTag(channel.id, tagId);
			galleryTags = galleryTags.filter((t) => t.id !== tagId);
			addToast('Tag deleted', 'success');
		} catch (err: any) {
			addToast(err.message || 'Failed to delete tag', 'error');
		}
	}

	const archiveOptions = [
		{ label: 'Never', value: 0 },
		{ label: '1 Hour', value: 60 },
		{ label: '1 Day', value: 1440 },
		{ label: '3 Days', value: 4320 },
		{ label: '1 Week', value: 10080 }
	];

	function toggleRole(roleId: string) {
		if (readOnlyRoleIds.includes(roleId)) {
			readOnlyRoleIds = readOnlyRoleIds.filter(id => id !== roleId);
		} else {
			readOnlyRoleIds = [...readOnlyRoleIds, roleId];
		}
	}

	async function handleSave() {
		saving = true;
		try {
			const payload: any = {
				read_only: readOnly,
				read_only_role_ids: readOnlyRoleIds,
				default_auto_archive_duration: autoArchiveDuration
			};
			if (isForum) {
				payload.forum_default_sort = forumDefaultSort;
				payload.forum_post_guidelines = forumPostGuidelines || null;
				payload.forum_require_tags = forumRequireTags;
			}
			if (isGallery) {
				payload.gallery_default_sort = galleryDefaultSort;
				payload.gallery_post_guidelines = galleryPostGuidelines || null;
				payload.gallery_require_tags = galleryRequireTags;
			}
			const updated = await api.updateChannel(channel.id, payload);
			onUpdate(updated);
			addToast('Channel settings updated', 'success');
		} catch (err: any) {
			addToast(err.message || 'Failed to update channel settings', 'error');
		} finally {
			saving = false;
		}
	}
</script>

<div class="space-y-6">
	<!-- Read-Only Channel -->
	<div class="rounded-lg bg-bg-secondary p-4">
		<h3 class="mb-2 text-sm font-semibold text-text-primary">Read-Only Channel</h3>
		<p class="mb-3 text-xs text-text-muted">
			When enabled, only users with selected roles (or guild owners/admins) can send messages. Everyone else can only read.
		</p>
		<label class="mb-3 flex items-center gap-2">
			<input type="checkbox" bind:checked={readOnly} class="rounded" />
			<span class="text-sm text-text-primary">Enable read-only mode</span>
		</label>

		{#if readOnly}
			<div class="mt-3">
				<p class="mb-2 text-xs font-medium text-text-muted">Roles that can still post:</p>
				{#if roles.length === 0}
					<p class="text-xs text-text-muted">No roles available. Guild owner and admins can always post.</p>
				{:else}
					<div class="flex flex-wrap gap-2">
						{#each roles as role (role.id)}
							<button
								class="rounded px-2 py-1 text-xs transition-colors {readOnlyRoleIds.includes(role.id)
									? 'bg-brand-500/20 text-brand-400 ring-1 ring-brand-500/50'
									: 'bg-bg-modifier text-text-muted hover:bg-bg-modifier/80'}"
								onclick={() => toggleRole(role.id)}
							>
								{#if role.color}
									<span class="mr-1 inline-block h-2 w-2 rounded-full" style="background-color: {role.color}"></span>
								{/if}
								{role.name}
							</button>
						{/each}
					</div>
				{/if}
			</div>
		{/if}
	</div>

	<!-- Thread Auto-Archive Duration -->
	{#if channel.channel_type === 'text' || channel.channel_type === 'forum' || channel.channel_type === 'gallery'}
		<div class="rounded-lg bg-bg-secondary p-4">
			<h3 class="mb-2 text-sm font-semibold text-text-primary">Thread Auto-Archive</h3>
			<p class="mb-3 text-xs text-text-muted">
				Threads will be automatically archived after this duration of inactivity (no new messages).
			</p>
			<select class="input w-full" bind:value={autoArchiveDuration}>
				{#each archiveOptions as opt (opt.value)}
					<option value={opt.value}>{opt.label}</option>
				{/each}
			</select>
		</div>
	{/if}

	<!-- Forum Settings -->
	{#if isForum}
		<!-- Default Sort -->
		<div class="rounded-lg bg-bg-secondary p-4">
			<h3 class="mb-2 text-sm font-semibold text-text-primary">Default Sort Order</h3>
			<p class="mb-3 text-xs text-text-muted">
				How posts are sorted by default when users open this forum.
			</p>
			<select class="input w-full" bind:value={forumDefaultSort}>
				<option value="latest_activity">Latest Activity</option>
				<option value="creation_date">Creation Date (Newest)</option>
			</select>
		</div>

		<!-- Post Guidelines -->
		<div class="rounded-lg bg-bg-secondary p-4">
			<h3 class="mb-2 text-sm font-semibold text-text-primary">Post Guidelines</h3>
			<p class="mb-3 text-xs text-text-muted">
				Shown to users when creating a new post. Use this to explain rules or formatting expectations.
			</p>
			<textarea
				class="input min-h-[60px] w-full resize-y"
				bind:value={forumPostGuidelines}
				placeholder="e.g. Include steps to reproduce for bug reports..."
				rows="3"
				maxlength="1000"
			></textarea>
		</div>

		<!-- Tags -->
		<div class="rounded-lg bg-bg-secondary p-4">
			<h3 class="mb-2 text-sm font-semibold text-text-primary">Forum Tags</h3>
			<p class="mb-3 text-xs text-text-muted">
				Tags help categorize posts. Users can filter the forum by tag.
			</p>

			<label class="mb-3 flex items-center gap-2">
				<input type="checkbox" bind:checked={forumRequireTags} class="rounded" />
				<span class="text-sm text-text-primary">Require at least one tag on new posts</span>
			</label>

			<!-- Existing tags -->
			{#if forumTags.length > 0}
				<div class="mb-3 flex flex-wrap gap-2">
					{#each forumTags as tag (tag.id)}
						<div
							class="group inline-flex items-center gap-1.5 rounded-full border border-bg-modifier px-2.5 py-1 text-xs font-medium"
							style="color: {tag.color || 'var(--text-secondary)'}"
						>
							{#if tag.emoji}<span>{tag.emoji}</span>{/if}
							{tag.name}
							<button
								class="ml-0.5 opacity-0 transition-opacity hover:text-red-400 group-hover:opacity-100"
								onclick={() => deleteTag(tag.id)}
								title="Delete tag"
							>
								<svg class="h-3 w-3" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
									<path d="M6 18L18 6M6 6l12 12" />
								</svg>
							</button>
						</div>
					{/each}
				</div>
			{/if}

			<!-- Add new tag -->
			<div class="flex items-end gap-2">
				<div class="flex-1">
					<label class="mb-1 block text-xs text-text-muted">Name</label>
					<input
						bind:value={newTagName}
						type="text"
						placeholder="Tag name"
						class="input w-full text-sm"
						maxlength="30"
					/>
				</div>
				<div class="w-16">
					<label class="mb-1 block text-xs text-text-muted">Emoji</label>
					<input
						bind:value={newTagEmoji}
						type="text"
						placeholder="🔖"
						class="input w-full text-center text-sm"
						maxlength="4"
					/>
				</div>
				<div class="w-12">
					<label class="mb-1 block text-xs text-text-muted">Color</label>
					<input
						bind:value={newTagColor}
						type="color"
						class="h-[34px] w-full cursor-pointer rounded border border-bg-modifier bg-bg-tertiary"
					/>
				</div>
				<button
					class="btn-primary shrink-0 text-xs"
					onclick={createTag}
					disabled={creatingTag || !newTagName.trim()}
				>
					{creatingTag ? '...' : 'Add'}
				</button>
			</div>
		</div>
	{/if}

	<!-- Gallery Settings -->
	{#if isGallery}
		<!-- Default Sort -->
		<div class="rounded-lg bg-bg-secondary p-4">
			<h3 class="mb-2 text-sm font-semibold text-text-primary">Default Sort Order</h3>
			<p class="mb-3 text-xs text-text-muted">
				How posts are sorted by default when users open this gallery.
			</p>
			<select class="input w-full" bind:value={galleryDefaultSort}>
				<option value="newest">Newest</option>
				<option value="oldest">Oldest</option>
				<option value="most_comments">Most Comments</option>
			</select>
		</div>

		<!-- Post Guidelines -->
		<div class="rounded-lg bg-bg-secondary p-4">
			<h3 class="mb-2 text-sm font-semibold text-text-primary">Post Guidelines</h3>
			<p class="mb-3 text-xs text-text-muted">
				Shown to users when creating a new gallery post.
			</p>
			<textarea
				class="input min-h-[60px] w-full resize-y"
				bind:value={galleryPostGuidelines}
				placeholder="e.g. Only post original artwork..."
				rows="3"
				maxlength="1000"
			></textarea>
		</div>

		<!-- Tags -->
		<div class="rounded-lg bg-bg-secondary p-4">
			<h3 class="mb-2 text-sm font-semibold text-text-primary">Gallery Tags</h3>
			<p class="mb-3 text-xs text-text-muted">
				Tags help categorize gallery posts. Users can filter the gallery by tag.
			</p>

			<label class="mb-3 flex items-center gap-2">
				<input type="checkbox" bind:checked={galleryRequireTags} class="rounded" />
				<span class="text-sm text-text-primary">Require at least one tag on new posts</span>
			</label>

			<!-- Existing tags -->
			{#if galleryTags.length > 0}
				<div class="mb-3 flex flex-wrap gap-2">
					{#each galleryTags as tag (tag.id)}
						<div
							class="group inline-flex items-center gap-1.5 rounded-full border border-bg-modifier px-2.5 py-1 text-xs font-medium"
							style="color: {tag.color || 'var(--text-secondary)'}"
						>
							{#if tag.emoji}<span>{tag.emoji}</span>{/if}
							{tag.name}
							<button
								class="ml-0.5 opacity-0 transition-opacity hover:text-red-400 group-hover:opacity-100"
								onclick={() => deleteGalleryTag(tag.id)}
								title="Delete tag"
							>
								<svg class="h-3 w-3" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
									<path d="M6 18L18 6M6 6l12 12" />
								</svg>
							</button>
						</div>
					{/each}
				</div>
			{/if}

			<!-- Add new tag -->
			<div class="flex items-end gap-2">
				<div class="flex-1">
					<label class="mb-1 block text-xs text-text-muted">Name</label>
					<input
						bind:value={newGalleryTagName}
						type="text"
						placeholder="Tag name"
						class="input w-full text-sm"
						maxlength="30"
					/>
				</div>
				<div class="w-16">
					<label class="mb-1 block text-xs text-text-muted">Emoji</label>
					<input
						bind:value={newGalleryTagEmoji}
						type="text"
						placeholder="🔖"
						class="input w-full text-center text-sm"
						maxlength="4"
					/>
				</div>
				<div class="w-12">
					<label class="mb-1 block text-xs text-text-muted">Color</label>
					<input
						bind:value={newGalleryTagColor}
						type="color"
						class="h-[34px] w-full cursor-pointer rounded border border-bg-modifier bg-bg-tertiary"
					/>
				</div>
				<button
					class="btn-primary shrink-0 text-xs"
					onclick={createGalleryTag}
					disabled={creatingGalleryTag || !newGalleryTagName.trim()}
				>
					{creatingGalleryTag ? '...' : 'Add'}
				</button>
			</div>
		</div>
	{/if}

	<!-- Retention Info -->
	{#if channel.channel_type === 'text' || channel.channel_type === 'announcement' || channel.channel_type === 'forum' || channel.channel_type === 'gallery'}
		<div class="rounded-lg bg-bg-secondary p-4">
			<h3 class="mb-2 text-sm font-semibold text-text-primary">Message Retention</h3>
			<p class="text-xs text-text-muted">
				Per-channel retention policies can be configured in the guild's <strong>Message Retention</strong> settings tab. If no per-channel policy exists, the guild-wide policy applies.
			</p>
		</div>
	{/if}

	<!-- Save Button -->
	<button
		class="btn-primary text-sm"
		onclick={handleSave}
		disabled={saving}
	>
		{saving ? 'Saving...' : 'Save Channel Settings'}
	</button>
</div>
