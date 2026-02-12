<!-- Whiteboard.svelte — Collaborative whiteboard widget with drawing tools. -->
<script lang="ts">
	import { api } from '$lib/api/client';
	import { currentUser } from '$lib/stores/auth';

	interface WhiteboardData {
		id: string;
		channel_id: string;
		name: string;
		creator_id: string;
		state: { objects: DrawObject[]; version: number };
		width: number;
		height: number;
		background_color: string;
		locked: boolean;
		collaborators: Array<{ user_id: string; username: string; cursor_x: number; cursor_y: number }>;
	}

	interface DrawObject {
		id: string;
		type: 'path' | 'rect' | 'circle' | 'text' | 'line' | 'arrow';
		points?: number[];
		x?: number;
		y?: number;
		width?: number;
		height?: number;
		radius?: number;
		text?: string;
		color: string;
		strokeWidth: number;
		fill?: string;
		userId: string;
	}

	interface Props {
		channelId: string;
		whiteboardId?: string;
		onclose?: () => void;
	}

	let { channelId, whiteboardId, onclose }: Props = $props();

	let canvas: HTMLCanvasElement;
	let ctx: CanvasRenderingContext2D | null = null;
	let whiteboard = $state<WhiteboardData | null>(null);
	let objects = $state<DrawObject[]>([]);
	let loading = $state(false);
	let error = $state('');

	// Drawing state.
	let tool = $state<'pen' | 'rect' | 'circle' | 'line' | 'arrow' | 'text' | 'eraser'>('pen');
	let color = $state('#ffffff');
	let strokeWidth = $state(3);
	let isDrawing = $state(false);
	let currentPath = $state<number[]>([]);
	let startX = $state(0);
	let startY = $state(0);

	// Create mode.
	let boardName = $state('Untitled Whiteboard');
	let creating = $state(false);
	let showCreateForm = $state(!whiteboardId);

	const tools = [
		{ id: 'pen', icon: 'M15.232 5.232l3.536 3.536m-2.036-5.036a2.5 2.5 0 113.536 3.536L6.5 21.036H3v-3.572L16.732 3.732z', label: 'Pen' },
		{ id: 'rect', icon: 'M4 6a2 2 0 012-2h12a2 2 0 012 2v12a2 2 0 01-2 2H6a2 2 0 01-2-2V6z', label: 'Rectangle' },
		{ id: 'circle', icon: 'M12 22C6.477 22 2 17.523 2 12S6.477 2 12 2s10 4.477 10 10-4.477 10-10 10z', label: 'Circle' },
		{ id: 'line', icon: 'M4 20L20 4', label: 'Line' },
		{ id: 'text', icon: 'M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z', label: 'Text' },
		{ id: 'eraser', icon: 'M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16', label: 'Eraser' }
	];

	const colors = ['#ffffff', '#ef4444', '#f97316', '#eab308', '#22c55e', '#3b82f6', '#8b5cf6', '#ec4899', '#6b7280'];

	async function createWhiteboard() {
		creating = true;
		error = '';
		try {
			const result = await api.request<WhiteboardData>(
				'POST',
				`/channels/${channelId}/experimental/whiteboards`,
				{ name: boardName, width: 1920, height: 1080, background_color: '#1a1a2e' }
			);
			if (result) {
				whiteboard = result;
				whiteboardId = result.id;
				showCreateForm = false;
				initCanvas();
			}
		} catch (err: any) {
			error = err.message || 'Failed to create whiteboard';
		} finally {
			creating = false;
		}
	}

	async function loadWhiteboard() {
		if (!whiteboardId) return;
		loading = true;
		try {
			const data = await api.request<WhiteboardData>(
				'GET',
				`/channels/${channelId}/experimental/whiteboards/${whiteboardId}`
			);
			if (data) {
				whiteboard = data;
				objects = data.state?.objects ?? [];
				initCanvas();
				redraw();
			}
		} catch (err: any) {
			error = err.message || 'Failed to load whiteboard';
		} finally {
			loading = false;
		}
	}

	function initCanvas() {
		if (!canvas) return;
		ctx = canvas.getContext('2d');
		if (ctx) {
			canvas.width = whiteboard?.width ?? 1920;
			canvas.height = whiteboard?.height ?? 1080;
			redraw();
		}
	}

	function redraw() {
		if (!ctx || !canvas) return;
		ctx.fillStyle = whiteboard?.background_color ?? '#1a1a2e';
		ctx.fillRect(0, 0, canvas.width, canvas.height);

		for (const obj of objects) {
			drawObject(ctx, obj);
		}
	}

	function drawObject(c: CanvasRenderingContext2D, obj: DrawObject) {
		c.strokeStyle = obj.color;
		c.lineWidth = obj.strokeWidth;
		c.lineCap = 'round';
		c.lineJoin = 'round';

		switch (obj.type) {
			case 'path':
				if (obj.points && obj.points.length >= 4) {
					c.beginPath();
					c.moveTo(obj.points[0], obj.points[1]);
					for (let i = 2; i < obj.points.length; i += 2) {
						c.lineTo(obj.points[i], obj.points[i + 1]);
					}
					c.stroke();
				}
				break;
			case 'rect':
				if (obj.x !== undefined && obj.y !== undefined && obj.width !== undefined && obj.height !== undefined) {
					if (obj.fill) {
						c.fillStyle = obj.fill;
						c.fillRect(obj.x, obj.y, obj.width, obj.height);
					}
					c.strokeRect(obj.x, obj.y, obj.width, obj.height);
				}
				break;
			case 'circle':
				if (obj.x !== undefined && obj.y !== undefined && obj.radius !== undefined) {
					c.beginPath();
					c.arc(obj.x, obj.y, obj.radius, 0, Math.PI * 2);
					if (obj.fill) {
						c.fillStyle = obj.fill;
						c.fill();
					}
					c.stroke();
				}
				break;
			case 'line':
			case 'arrow':
				if (obj.points && obj.points.length >= 4) {
					c.beginPath();
					c.moveTo(obj.points[0], obj.points[1]);
					c.lineTo(obj.points[2], obj.points[3]);
					c.stroke();
					if (obj.type === 'arrow') {
						const angle = Math.atan2(obj.points[3] - obj.points[1], obj.points[2] - obj.points[0]);
						const headLen = 15;
						c.beginPath();
						c.moveTo(obj.points[2], obj.points[3]);
						c.lineTo(obj.points[2] - headLen * Math.cos(angle - Math.PI / 6), obj.points[3] - headLen * Math.sin(angle - Math.PI / 6));
						c.moveTo(obj.points[2], obj.points[3]);
						c.lineTo(obj.points[2] - headLen * Math.cos(angle + Math.PI / 6), obj.points[3] - headLen * Math.sin(angle + Math.PI / 6));
						c.stroke();
					}
				}
				break;
			case 'text':
				if (obj.x !== undefined && obj.y !== undefined && obj.text) {
					c.font = `${obj.strokeWidth * 6}px sans-serif`;
					c.fillStyle = obj.color;
					c.fillText(obj.text, obj.x, obj.y);
				}
				break;
		}
	}

	function getCanvasCoords(e: MouseEvent): { x: number; y: number } {
		if (!canvas) return { x: 0, y: 0 };
		const rect = canvas.getBoundingClientRect();
		const scaleX = canvas.width / rect.width;
		const scaleY = canvas.height / rect.height;
		return {
			x: (e.clientX - rect.left) * scaleX,
			y: (e.clientY - rect.top) * scaleY
		};
	}

	function handleMouseDown(e: MouseEvent) {
		if (whiteboard?.locked && whiteboard.creator_id !== $currentUser?.id) return;
		const { x, y } = getCanvasCoords(e);
		isDrawing = true;
		startX = x;
		startY = y;

		if (tool === 'pen' || tool === 'eraser') {
			currentPath = [x, y];
		}
		if (tool === 'text') {
			const text = prompt('Enter text:');
			if (text) {
				const obj: DrawObject = {
					id: crypto.randomUUID(),
					type: 'text',
					x, y,
					text,
					color,
					strokeWidth,
					userId: $currentUser?.id ?? ''
				};
				objects = [...objects, obj];
				redraw();
				saveState();
			}
			isDrawing = false;
		}
	}

	function handleMouseMove(e: MouseEvent) {
		if (!isDrawing || !ctx) return;
		const { x, y } = getCanvasCoords(e);

		if (tool === 'pen' || tool === 'eraser') {
			currentPath = [...currentPath, x, y];
			// Live preview.
			redraw();
			ctx.strokeStyle = tool === 'eraser' ? (whiteboard?.background_color ?? '#1a1a2e') : color;
			ctx.lineWidth = tool === 'eraser' ? strokeWidth * 3 : strokeWidth;
			ctx.lineCap = 'round';
			ctx.beginPath();
			ctx.moveTo(currentPath[0], currentPath[1]);
			for (let i = 2; i < currentPath.length; i += 2) {
				ctx.lineTo(currentPath[i], currentPath[i + 1]);
			}
			ctx.stroke();
		} else if (tool === 'rect' || tool === 'circle' || tool === 'line' || tool === 'arrow') {
			redraw();
			const previewObj: DrawObject = {
				id: 'preview',
				type: tool === 'rect' ? 'rect' : tool === 'circle' ? 'circle' : tool,
				color,
				strokeWidth,
				userId: ''
			};
			if (tool === 'rect') {
				previewObj.x = Math.min(startX, x);
				previewObj.y = Math.min(startY, y);
				previewObj.width = Math.abs(x - startX);
				previewObj.height = Math.abs(y - startY);
			} else if (tool === 'circle') {
				previewObj.x = startX;
				previewObj.y = startY;
				previewObj.radius = Math.sqrt((x - startX) ** 2 + (y - startY) ** 2);
			} else {
				previewObj.points = [startX, startY, x, y];
			}
			drawObject(ctx, previewObj);
		}
	}

	function handleMouseUp(e: MouseEvent) {
		if (!isDrawing) return;
		isDrawing = false;
		const { x, y } = getCanvasCoords(e);
		const userId = $currentUser?.id ?? '';

		let newObj: DrawObject | null = null;

		if (tool === 'pen') {
			if (currentPath.length >= 4) {
				newObj = { id: crypto.randomUUID(), type: 'path', points: [...currentPath], color, strokeWidth, userId };
			}
		} else if (tool === 'eraser') {
			if (currentPath.length >= 4) {
				newObj = { id: crypto.randomUUID(), type: 'path', points: [...currentPath], color: whiteboard?.background_color ?? '#1a1a2e', strokeWidth: strokeWidth * 3, userId };
			}
		} else if (tool === 'rect') {
			newObj = { id: crypto.randomUUID(), type: 'rect', x: Math.min(startX, x), y: Math.min(startY, y), width: Math.abs(x - startX), height: Math.abs(y - startY), color, strokeWidth, userId };
		} else if (tool === 'circle') {
			newObj = { id: crypto.randomUUID(), type: 'circle', x: startX, y: startY, radius: Math.sqrt((x - startX) ** 2 + (y - startY) ** 2), color, strokeWidth, userId };
		} else if (tool === 'line' || tool === 'arrow') {
			newObj = { id: crypto.randomUUID(), type: tool, points: [startX, startY, x, y], color, strokeWidth, userId };
		}

		if (newObj) {
			objects = [...objects, newObj];
			redraw();
			saveState();
		}

		currentPath = [];
	}

	async function saveState() {
		if (!whiteboardId) return;
		try {
			await api.request('PATCH', `/channels/${channelId}/experimental/whiteboards/${whiteboardId}`, {
				state: JSON.stringify({ objects, version: (whiteboard?.state?.version ?? 0) + 1 })
			});
		} catch {
			// Silent save.
		}
	}

	function clearCanvas() {
		objects = [];
		redraw();
		saveState();
	}

	function undo() {
		if (objects.length === 0) return;
		objects = objects.slice(0, -1);
		redraw();
		saveState();
	}

	$effect(() => {
		if (whiteboardId && !showCreateForm) {
			loadWhiteboard();
		}
	});
</script>

{#if showCreateForm}
	<div class="p-4 bg-bg-secondary border border-border-primary rounded-lg max-w-md">
		<h3 class="text-text-primary font-medium mb-3">Create Whiteboard</h3>
		{#if error}
			<div class="mb-3 p-2 bg-red-500/10 border border-red-500/20 rounded text-red-400 text-sm">{error}</div>
		{/if}
		<input
			type="text"
			class="w-full bg-bg-primary border border-border-primary rounded px-3 py-2 text-sm text-text-primary mb-3 focus:border-brand-500 focus:outline-none"
			placeholder="Board name"
			bind:value={boardName}
		/>
		<div class="flex justify-end gap-2">
			{#if onclose}
				<button type="button" class="btn-secondary text-sm px-3 py-1.5 rounded" onclick={onclose}>Cancel</button>
			{/if}
			<button type="button" class="btn-primary text-sm px-3 py-1.5 rounded" disabled={creating} onclick={createWhiteboard}>
				{creating ? 'Creating...' : 'Create'}
			</button>
		</div>
	</div>
{:else}
	<div class="flex flex-col h-full bg-bg-secondary border border-border-primary rounded-lg overflow-hidden">
		<!-- Toolbar -->
		<div class="flex items-center gap-2 px-3 py-2 bg-bg-tertiary border-b border-border-primary flex-wrap">
			<span class="text-text-primary text-sm font-medium mr-2">{whiteboard?.name ?? 'Whiteboard'}</span>

			<!-- Tools -->
			<div class="flex items-center gap-0.5 bg-bg-primary rounded p-0.5">
				{#each tools as t}
					<button
						type="button"
						class="p-1.5 rounded transition-colors {tool === t.id ? 'bg-brand-500 text-white' : 'text-text-muted hover:text-text-primary hover:bg-bg-tertiary'}"
						title={t.label}
						onclick={() => (tool = t.id as typeof tool)}
					>
						<svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
							<path stroke-linecap="round" stroke-linejoin="round" d={t.icon} />
						</svg>
					</button>
				{/each}
			</div>

			<!-- Colors -->
			<div class="flex items-center gap-0.5">
				{#each colors as c}
					<button
						type="button"
						class="w-5 h-5 rounded-full border-2 transition-transform {color === c ? 'border-brand-400 scale-110' : 'border-transparent hover:scale-105'}"
						style="background-color: {c};"
						onclick={() => (color = c)}
					></button>
				{/each}
			</div>

			<!-- Stroke width -->
			<input
				type="range"
				min="1"
				max="20"
				bind:value={strokeWidth}
				class="w-20"
				title="Stroke width: {strokeWidth}px"
			/>

			<div class="flex-1"></div>

			<!-- Actions -->
			<button
				type="button"
				class="text-text-muted hover:text-text-primary text-xs px-2 py-1 rounded hover:bg-bg-primary"
				onclick={undo}
				title="Undo"
			>
				Undo
			</button>
			<button
				type="button"
				class="text-text-muted hover:text-red-400 text-xs px-2 py-1 rounded hover:bg-bg-primary"
				onclick={clearCanvas}
				title="Clear all"
			>
				Clear
			</button>
			{#if onclose}
				<button type="button" class="text-text-muted hover:text-text-primary p-1" onclick={onclose}>
					<svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
						<path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
					</svg>
				</button>
			{/if}
		</div>

		<!-- Canvas -->
		<div class="flex-1 overflow-auto relative" style="min-height: 400px;">
			{#if loading}
				<div class="absolute inset-0 flex items-center justify-center">
					<span class="text-text-muted">Loading whiteboard...</span>
				</div>
			{:else}
				<canvas
					bind:this={canvas}
					class="w-full h-full cursor-crosshair"
					style="background-color: {whiteboard?.background_color ?? '#1a1a2e'};"
					onmousedown={handleMouseDown}
					onmousemove={handleMouseMove}
					onmouseup={handleMouseUp}
					onmouseleave={() => { if (isDrawing) handleMouseUp(new MouseEvent('mouseup')); }}
				></canvas>
			{/if}

			{#if error}
				<div class="absolute bottom-2 left-2 right-2 p-2 bg-red-500/10 border border-red-500/20 rounded text-red-400 text-sm">
					{error}
				</div>
			{/if}
		</div>

		<!-- Collaborator bar -->
		{#if whiteboard?.collaborators && whiteboard.collaborators.length > 0}
			<div class="flex items-center gap-2 px-3 py-1.5 bg-bg-tertiary border-t border-border-primary text-xs text-text-muted">
				<span>{whiteboard.collaborators.length} collaborator{whiteboard.collaborators.length !== 1 ? 's' : ''}</span>
				{#each whiteboard.collaborators.slice(0, 5) as collab}
					<span class="px-1.5 py-0.5 bg-bg-primary rounded">{collab.username}</span>
				{/each}
			</div>
		{/if}
	</div>
{/if}
