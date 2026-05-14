<script lang="ts">
	import { api } from '$lib/api/client';
	import { addToast } from '$lib/stores/toast';
	import { confirmAction } from '$lib/stores/confirm';
	import type { Category } from '$lib/types';

	interface Props {
		guildId: string;
	}

	let { guildId }: Props = $props();

	let categories = $state<Category[]>([]);
	let loadingCategories = $state(false);
	let newCategoryName = $state('');
	let creatingCategory = $state(false);
	let editingCategoryId = $state<string | null>(null);
	let editingCategoryName = $state('');
	let loadedGuildId = $state<string | null>(null);

	$effect(() => {
		if (guildId && loadedGuildId !== guildId && !loadingCategories) {
			loadCategories();
		}
	});

	async function loadCategories() {
		loadingCategories = true;
		try {
			categories = await api.getCategories(guildId);
			loadedGuildId = guildId;
		} catch {
			categories = [];
		} finally {
			loadingCategories = false;
		}
	}

	async function handleCreateCategory() {
		if (!newCategoryName.trim()) return;
		creatingCategory = true;
		try {
			const category = await api.createCategory(guildId, newCategoryName.trim());
			categories = [...categories, category];
			newCategoryName = '';
			addToast('Category created', 'success');
		} catch (err: any) {
			addToast(err.message || 'Failed to create category', 'error');
		} finally {
			creatingCategory = false;
		}
	}

	async function handleRenameCategory() {
		if (!editingCategoryId || !editingCategoryName.trim()) return;
		try {
			const updated = await api.updateCategory(guildId, editingCategoryId, { name: editingCategoryName.trim() });
			categories = categories.map((category) => (category.id === editingCategoryId ? updated : category));
			editingCategoryId = null;
			editingCategoryName = '';
			addToast('Category renamed', 'success');
		} catch (err: any) {
			addToast(err.message || 'Failed to rename category', 'error');
		}
	}

	async function handleDeleteCategory(categoryId: string) {
		if (!(await confirmAction({ title: 'Delete Category', message: 'Delete this category? Channels in it will become uncategorized.', confirmLabel: 'Delete' }))) return;
		try {
			await api.deleteCategory(guildId, categoryId);
			categories = categories.filter((category) => category.id !== categoryId);
			addToast('Category deleted', 'success');
		} catch (err: any) {
			addToast(err.message || 'Failed to delete category', 'error');
		}
	}
</script>

<h1 class="mb-6 text-xl font-bold text-text-primary">Channel Categories</h1>

<div class="mb-6 flex gap-2">
	<input
		type="text" class="input flex-1" placeholder="New category name..."
		bind:value={newCategoryName} maxlength="100"
		onkeydown={(e) => e.key === 'Enter' && handleCreateCategory()}
	/>
	<button class="btn-primary" onclick={handleCreateCategory} disabled={creatingCategory || !newCategoryName.trim()}>
		{creatingCategory ? 'Creating...' : 'Create Category'}
	</button>
</div>

{#if loadingCategories}
	<p class="text-sm text-text-muted">Loading categories...</p>
{:else if categories.length === 0}
	<p class="text-sm text-text-muted">No categories yet. Channels will appear uncategorized.</p>
{:else}
	<div class="space-y-2">
		{#each categories as cat (cat.id)}
			<div class="flex items-center justify-between rounded-lg bg-bg-secondary p-3">
				{#if editingCategoryId === cat.id}
					<div class="flex flex-1 items-center gap-2">
						<input
							type="text" class="input flex-1" bind:value={editingCategoryName}
							onkeydown={(e) => e.key === 'Enter' && handleRenameCategory()}
						/>
						<button class="btn-primary text-xs" onclick={handleRenameCategory}>Save</button>
						<button class="btn-secondary text-xs" onclick={() => (editingCategoryId = null)}>Cancel</button>
					</div>
				{:else}
					<div class="flex items-center gap-3">
						<svg class="h-4 w-4 text-text-muted" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
							<path d="M3 7v10a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-6l-2-2H5a2 2 0 00-2 2z" />
						</svg>
						<span class="text-sm font-medium text-text-primary">{cat.name}</span>
					</div>
					<div class="flex items-center gap-2">
						<span class="text-xs text-text-muted">Pos: {cat.position}</span>
						<button
							class="text-xs text-brand-400 hover:text-brand-300"
							onclick={() => { editingCategoryId = cat.id; editingCategoryName = cat.name; }}
						>
							Rename
						</button>
						<button
							class="text-xs text-red-400 hover:text-red-300"
							onclick={() => handleDeleteCategory(cat.id)}
						>
							Delete
						</button>
					</div>
				{/if}
			</div>
		{/each}
	</div>
{/if}
