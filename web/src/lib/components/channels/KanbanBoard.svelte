<!-- KanbanBoard.svelte — Kanban board channel type for project management within a guild. -->
<script lang="ts">
	import { api } from '$lib/api/client';
	import { createAsyncOp } from '$lib/utils/asyncOp';
	import { currentUser } from '$lib/stores/auth';

	interface KanbanCard {
		id: string;
		column_id: string;
		title: string;
		description?: string;
		color?: string;
		position: number;
		assignee_ids: string[];
		label_ids: string[];
		due_date?: string;
		completed: boolean;
		creator_id: string;
		created_at: string;
	}

	interface KanbanColumn {
		id: string;
		name: string;
		color: string;
		position: number;
		wip_limit?: number;
		cards: KanbanCard[];
	}

	interface KanbanLabel {
		id: string;
		name: string;
		color: string;
	}

	interface BoardData {
		id: string;
		channel_id: string;
		guild_id: string;
		name: string;
		description?: string;
		creator_id: string;
		columns: KanbanColumn[];
		labels: KanbanLabel[];
	}

	interface Props {
		channelId: string;
		boardId?: string;
	}

	let { channelId, boardId }: Props = $props();

	let board = $state<BoardData | null>(null);
	let error = $state('');

	let loadOp = $state(createAsyncOp());
	let createBoardOp = $state(createAsyncOp());
	let createCardOp = $state(createAsyncOp());

	// Create mode.
	let showCreateForm = $state(!boardId);
	let boardName = $state('Project Board');
	let boardDescription = $state('');

	// Card creation state.
	let addingToColumn = $state<string | null>(null);
	let newCardTitle = $state('');

	// Column creation state.
	let addingColumn = $state(false);
	let newColumnName = $state('');
	let newColumnColor = $state('#6366f1');

	// Drag state.
	let draggedCard = $state<KanbanCard | null>(null);
	let dragOverColumn = $state<string | null>(null);

	// Card detail.
	let selectedCard = $state<KanbanCard | null>(null);

	async function createBoard() {
		error = '';
		const result = await createBoardOp.run(
			() => api.createKanbanBoard<BoardData>(channelId, { name: boardName, description: boardDescription }),
			msg => (error = msg)
		);
		if (result) {
			boardId = result.id;
			showCreateForm = false;
			await loadBoard();
		}
	}

	async function loadBoard() {
		if (!boardId) return;
		error = '';
		const data = await loadOp.run(
			() => api.getKanbanBoard<BoardData>(channelId, boardId!),
			msg => (error = msg)
		);
		if (data) board = data;
	}

	async function addCard(columnId: string) {
		if (!newCardTitle.trim() || !boardId) return;
		await createCardOp.run(
			() => api.createKanbanCard(channelId, boardId!, columnId, {
				title: newCardTitle,
				assignee_ids: [],
				label_ids: []
			}),
			msg => (error = msg)
		);
		if (!createCardOp.error) {
			newCardTitle = '';
			addingToColumn = null;
			await loadBoard();
		}
	}

	async function addColumn() {
		if (!newColumnName.trim() || !boardId) return;
		try {
			await api.createKanbanColumn(channelId, boardId, {
				name: newColumnName,
				color: newColumnColor
			});
			newColumnName = '';
			newColumnColor = '#6366f1';
			addingColumn = false;
			await loadBoard();
		} catch (err: any) {
			error = err.message || 'Failed to create column';
		}
	}

	async function moveCard(cardId: string, targetColumnId: string, position: number) {
		if (!boardId) return;
		try {
			await api.moveKanbanCard(channelId, boardId, cardId, {
				column_id: targetColumnId,
				position
			});
			await loadBoard();
		} catch (err: any) {
			error = err.message || 'Failed to move card';
		}
	}

	async function deleteCard(cardId: string) {
		if (!boardId) return;
		try {
			await api.deleteKanbanCard(channelId, boardId, cardId);
			selectedCard = null;
			await loadBoard();
		} catch (err: any) {
			error = err.message || 'Failed to delete card';
		}
	}

	function handleDragStart(card: KanbanCard) {
		draggedCard = card;
	}

	function handleDragOver(e: DragEvent, columnId: string) {
		e.preventDefault();
		dragOverColumn = columnId;
	}

	function handleDrop(e: DragEvent, columnId: string) {
		e.preventDefault();
		if (draggedCard && draggedCard.column_id !== columnId) {
			moveCard(draggedCard.id, columnId, 0);
		}
		draggedCard = null;
		dragOverColumn = null;
	}

	function handleDragEnd() {
		draggedCard = null;
		dragOverColumn = null;
	}

	function formatDueDate(dateStr: string): string {
		const date = new Date(dateStr);
		const now = new Date();
		const diff = date.getTime() - now.getTime();
		const days = Math.floor(diff / (1000 * 60 * 60 * 24));
		if (days < 0) return `${Math.abs(days)}d overdue`;
		if (days === 0) return 'Due today';
		if (days === 1) return 'Due tomorrow';
		return `Due in ${days}d`;
	}

	function isDueSoon(dateStr: string): boolean {
		const diff = new Date(dateStr).getTime() - Date.now();
		return diff < 86400000 * 2; // 2 days.
	}

	function isOverdue(dateStr: string): boolean {
		return new Date(dateStr).getTime() < Date.now();
	}

	$effect(() => {
		if (boardId && !showCreateForm) {
			loadBoard();
		}
	});
</script>

{#if showCreateForm}
	<div class="p-6 max-w-md mx-auto">
		<h2 class="text-text-primary text-lg font-medium mb-4">Create Kanban Board</h2>
		{#if error}
			<div class="mb-3 p-2 bg-red-500/10 border border-red-500/20 rounded text-red-400 text-sm">{error}</div>
		{/if}
		<div class="space-y-3">
			<input
				type="text"
				class="w-full bg-bg-primary border border-border-primary rounded px-3 py-2 text-sm text-text-primary focus:border-brand-500 focus:outline-none"
				placeholder="Board name"
				bind:value={boardName}
			/>
			<textarea
				class="w-full bg-bg-primary border border-border-primary rounded px-3 py-2 text-sm text-text-primary focus:border-brand-500 focus:outline-none resize-none h-20"
				placeholder="Description (optional)"
				bind:value={boardDescription}
			></textarea>
			<button
				type="button"
				class="btn-primary w-full text-sm py-2 rounded"
				disabled={createBoardOp.loading || !boardName.trim()}
				onclick={createBoard}
			>
				{createBoardOp.loading ? 'Creating...' : 'Create Board'}
			</button>
		</div>
	</div>
{:else if loadOp.loading}
	<div class="flex items-center justify-center h-64 text-text-muted">Loading board...</div>
{:else if board}
	<div class="flex flex-col h-full">
		<!-- Header -->
		<div class="flex items-center justify-between px-4 py-3 border-b border-border-primary">
			<div>
				<h2 class="text-text-primary font-medium">{board.name}</h2>
				{#if board.description}
					<p class="text-text-muted text-xs mt-0.5">{board.description}</p>
				{/if}
			</div>
			<div class="text-text-muted text-xs">
				{board.columns.reduce((acc, col) => acc + col.cards.length, 0)} cards
			</div>
		</div>

		{#if error}
			<div class="mx-4 mt-2 p-2 bg-red-500/10 border border-red-500/20 rounded text-red-400 text-sm">{error}</div>
		{/if}

		<!-- Board -->
		<div class="flex-1 overflow-x-auto p-4">
			<div class="flex gap-4 h-full" style="min-width: max-content;">
				{#each board.columns as column (column.id)}
					<div
						class="flex flex-col w-72 shrink-0 bg-bg-secondary rounded-lg border transition-colors
							{dragOverColumn === column.id ? 'border-brand-500' : 'border-border-primary'}"
						role="region"
						aria-label="{column.name} column"
						ondragover={(e) => handleDragOver(e, column.id)}
						ondrop={(e) => handleDrop(e, column.id)}
						ondragleave={() => { if (dragOverColumn === column.id) dragOverColumn = null; }}
					>
						<!-- Column header -->
						<div class="flex items-center justify-between px-3 py-2 border-b border-border-primary">
							<div class="flex items-center gap-2">
								<span class="w-3 h-3 rounded-full" style="background-color: {column.color};"></span>
								<span class="text-text-primary text-sm font-medium">{column.name}</span>
								<span class="text-text-muted text-xs bg-bg-tertiary px-1.5 py-0.5 rounded-full">{column.cards.length}</span>
							</div>
							{#if column.wip_limit}
								<span class="text-xs {column.cards.length >= column.wip_limit ? 'text-red-400' : 'text-text-muted'}">
									WIP: {column.wip_limit}
								</span>
							{/if}
						</div>

						<!-- Cards -->
						<div class="flex-1 overflow-y-auto p-2 space-y-2">
							{#each column.cards as card (card.id)}
								<div
									class="group bg-bg-primary border border-border-primary rounded-lg p-3 cursor-pointer hover:border-brand-500/50 transition-colors
										{draggedCard?.id === card.id ? 'opacity-50' : ''}"
									draggable="true"
									ondragstart={() => handleDragStart(card)}
									ondragend={handleDragEnd}
									role="button"
									tabindex="0"
									onclick={() => (selectedCard = card)}
									onkeydown={(e) => { if (e.key === 'Enter') selectedCard = card; }}
								>
									{#if card.color}
										<div class="w-full h-1 rounded-full mb-2" style="background-color: {card.color};"></div>
									{/if}
									<p class="text-text-primary text-sm {card.completed ? 'line-through text-text-muted' : ''}">
										{card.title}
									</p>
									{#if card.description}
										<p class="text-text-muted text-xs mt-1 line-clamp-2">{card.description}</p>
									{/if}
									<div class="flex items-center gap-2 mt-2 flex-wrap">
										{#if card.due_date}
											<span class="text-xs px-1.5 py-0.5 rounded {isOverdue(card.due_date) ? 'bg-red-500/20 text-red-400' : isDueSoon(card.due_date) ? 'bg-yellow-500/20 text-yellow-400' : 'bg-bg-tertiary text-text-muted'}">
												{formatDueDate(card.due_date)}
											</span>
										{/if}
										{#if card.assignee_ids.length > 0}
											<span class="text-xs text-text-muted">
												{card.assignee_ids.length} assignee{card.assignee_ids.length !== 1 ? 's' : ''}
											</span>
										{/if}
									</div>
								</div>
							{/each}

							<!-- Add card button -->
							{#if addingToColumn === column.id}
								<div class="bg-bg-primary border border-border-primary rounded-lg p-2">
									<input
										type="text"
										class="w-full bg-transparent border-none text-sm text-text-primary placeholder:text-text-muted focus:outline-none"
										placeholder="Card title..."
										bind:value={newCardTitle}
										onkeydown={(e) => { if (e.key === 'Enter') addCard(column.id); if (e.key === 'Escape') addingToColumn = null; }}
									/>
									<div class="flex justify-end gap-1 mt-2">
										<button type="button" class="text-text-muted text-xs px-2 py-1 hover:text-text-primary" onclick={() => (addingToColumn = null)}>Cancel</button>
										<button type="button" class="btn-primary text-xs px-2 py-1 rounded" disabled={createCardOp.loading || !newCardTitle.trim()} onclick={() => addCard(column.id)}>Add</button>
									</div>
								</div>
							{:else}
								<button
									type="button"
									class="w-full text-left text-text-muted text-sm p-2 rounded hover:bg-bg-tertiary transition-colors flex items-center gap-1"
									onclick={() => { addingToColumn = column.id; newCardTitle = ''; }}
								>
									<svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
										<path stroke-linecap="round" stroke-linejoin="round" d="M12 4v16m8-8H4" />
									</svg>
									Add card
								</button>
							{/if}
						</div>
					</div>
				{/each}

				<!-- Add column -->
				{#if addingColumn}
					<div class="w-72 shrink-0 bg-bg-secondary rounded-lg border border-border-primary p-3">
						<input
							type="text"
							class="w-full bg-bg-primary border border-border-primary rounded px-2 py-1.5 text-sm text-text-primary mb-2 focus:border-brand-500 focus:outline-none"
							placeholder="Column name"
							bind:value={newColumnName}
							onkeydown={(e) => { if (e.key === 'Enter') addColumn(); if (e.key === 'Escape') addingColumn = false; }}
						/>
						<div class="flex items-center gap-2 mb-2">
							<span class="text-text-muted text-xs">Color:</span>
							<input type="color" bind:value={newColumnColor} class="w-6 h-6 rounded cursor-pointer" />
						</div>
						<div class="flex justify-end gap-1">
							<button type="button" class="text-text-muted text-xs px-2 py-1" onclick={() => (addingColumn = false)}>Cancel</button>
							<button type="button" class="btn-primary text-xs px-2 py-1 rounded" onclick={addColumn}>Add</button>
						</div>
					</div>
				{:else}
					<button
						type="button"
						class="w-72 shrink-0 h-12 bg-bg-secondary/50 border border-dashed border-border-primary rounded-lg flex items-center justify-center text-text-muted text-sm hover:bg-bg-secondary hover:border-brand-500/30 transition-colors"
						onclick={() => (addingColumn = true)}
					>
						+ Add Column
					</button>
				{/if}
			</div>
		</div>
	</div>

	<!-- Card detail modal -->
	{#if selectedCard}
		<div class="fixed inset-0 z-50 flex items-center justify-center bg-black/50" role="dialog" aria-modal="true">
			<div class="bg-bg-secondary border border-border-primary rounded-lg w-full max-w-md p-4 shadow-xl">
				<div class="flex items-center justify-between mb-3">
					<h3 class="text-text-primary font-medium">{selectedCard.title}</h3>
					<button type="button" class="text-text-muted hover:text-text-primary" aria-label="Close card details" onclick={() => (selectedCard = null)}>
						<svg class="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
							<path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
						</svg>
					</button>
				</div>

				{#if selectedCard.description}
					<p class="text-text-secondary text-sm mb-3">{selectedCard.description}</p>
				{/if}

				<div class="space-y-2 text-sm text-text-muted">
					{#if selectedCard.due_date}
						<p>Due: {new Date(selectedCard.due_date).toLocaleDateString()}</p>
					{/if}
					<p>Created: {new Date(selectedCard.created_at).toLocaleDateString()}</p>
					<p>Assignees: {selectedCard.assignee_ids.length || 'None'}</p>
					<p>Status: {selectedCard.completed ? 'Completed' : 'Open'}</p>
				</div>

				<div class="flex justify-end gap-2 mt-4">
					<button
						type="button"
						class="text-sm px-3 py-1.5 rounded text-red-400 hover:bg-red-500/10"
						onclick={() => deleteCard(selectedCard!.id)}
					>
						Delete
					</button>
					<button type="button" class="btn-secondary text-sm px-3 py-1.5 rounded" onclick={() => (selectedCard = null)}>
						Close
					</button>
				</div>
			</div>
		</div>
	{/if}
{:else}
	<div class="flex items-center justify-center h-64 text-text-muted">Board not found</div>
{/if}
