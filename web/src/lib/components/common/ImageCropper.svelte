<!-- ImageCropper.svelte — Canvas-based image cropper with pan, zoom, and circular/rectangular mask. -->
<script lang="ts">
	import { onMount } from 'svelte';

	interface Props {
		file: File;
		/** 'circle' for avatars, 'rect' for banners */
		shape?: 'circle' | 'rect';
		/** Output size in pixels (width for circle diameter, width for rect) */
		outputWidth?: number;
		/** Output height (only for rect; circle uses outputWidth) */
		outputHeight?: number;
		oncrop: (blob: Blob) => void;
		oncancel: () => void;
	}

	let {
		file,
		shape = 'circle',
		outputWidth = 256,
		outputHeight = 256,
		oncrop,
		oncancel
	}: Props = $props();

	let canvas = $state<HTMLCanvasElement | undefined>(undefined);
	let img = $state<HTMLImageElement | null>(null);

	// Transform state
	let zoom = $state(1);
	let panX = $state(0);
	let panY = $state(0);
	let dragging = $state(false);
	let lastMouse = $state({ x: 0, y: 0 });

	const CANVAS_SIZE = 300;
	const cropW = $derived(shape === 'rect' ? CANVAS_SIZE : CANVAS_SIZE * 0.75);
	const cropH = $derived(shape === 'rect' ? Math.round(CANVAS_SIZE * (outputHeight / outputWidth)) : CANVAS_SIZE * 0.75);

	onMount(() => {
		const image = new Image();
		const url = URL.createObjectURL(file);
		image.onload = () => {
			img = image;
			// Fit image to canvas initially
			const scale = Math.max(cropW / image.width, cropH / image.height);
			zoom = scale * 1.1; // Slight extra so image fills crop area
			panX = 0;
			panY = 0;
			draw();
		};
		image.src = url;
		return () => URL.revokeObjectURL(url);
	});

	function draw() {
		if (!canvas || !img) return;
		const ctx = canvas.getContext('2d');
		if (!ctx) return;

		ctx.clearRect(0, 0, CANVAS_SIZE, CANVAS_SIZE);

		// Draw the image centered with pan/zoom
		const iw = img.width * zoom;
		const ih = img.height * zoom;
		const ix = (CANVAS_SIZE - iw) / 2 + panX;
		const iy = (CANVAS_SIZE - ih) / 2 + panY;
		ctx.drawImage(img, ix, iy, iw, ih);

		// Draw semi-transparent overlay outside crop area
		ctx.fillStyle = 'rgba(0, 0, 0, 0.3)';

		if (shape === 'circle') {
			// Full overlay, then clear circle
			ctx.fillRect(0, 0, CANVAS_SIZE, CANVAS_SIZE);
			ctx.globalCompositeOperation = 'destination-out';
			ctx.beginPath();
			ctx.arc(CANVAS_SIZE / 2, CANVAS_SIZE / 2, cropW / 2, 0, Math.PI * 2);
			ctx.fill();
			ctx.globalCompositeOperation = 'source-over';
			// Draw circle border
			ctx.strokeStyle = 'rgba(255, 255, 255, 0.8)';
			ctx.lineWidth = 3;
			ctx.beginPath();
			ctx.arc(CANVAS_SIZE / 2, CANVAS_SIZE / 2, cropW / 2, 0, Math.PI * 2);
			ctx.stroke();
		} else {
			const rx = (CANVAS_SIZE - cropW) / 2;
			const ry = (CANVAS_SIZE - cropH) / 2;
			// Top
			ctx.fillRect(0, 0, CANVAS_SIZE, ry);
			// Bottom
			ctx.fillRect(0, ry + cropH, CANVAS_SIZE, CANVAS_SIZE - ry - cropH);
			// Left
			ctx.fillRect(0, ry, rx, cropH);
			// Right
			ctx.fillRect(rx + cropW, ry, CANVAS_SIZE - rx - cropW, cropH);
			// Border
			ctx.strokeStyle = 'rgba(255, 255, 255, 0.8)';
			ctx.lineWidth = 3;
			ctx.strokeRect(rx, ry, cropW, cropH);
		}
	}

	$effect(() => {
		// Redraw when zoom/pan changes
		void zoom;
		void panX;
		void panY;
		draw();
	});

	function handleMouseDown(e: MouseEvent) {
		dragging = true;
		lastMouse = { x: e.clientX, y: e.clientY };
	}

	function handleMouseMove(e: MouseEvent) {
		if (!dragging) return;
		panX += e.clientX - lastMouse.x;
		panY += e.clientY - lastMouse.y;
		lastMouse = { x: e.clientX, y: e.clientY };
	}

	function handleMouseUp() {
		dragging = false;
	}

	function handleWheel(e: WheelEvent) {
		e.preventDefault();
		const delta = e.deltaY > 0 ? -0.05 : 0.05;
		zoom = Math.max(0.1, Math.min(5, zoom + delta));
	}

	function handleZoomInput(e: Event) {
		zoom = parseFloat((e.target as HTMLInputElement).value);
	}

	async function crop() {
		if (!img) return;
		const outCanvas = document.createElement('canvas');
		outCanvas.width = outputWidth;
		outCanvas.height = shape === 'circle' ? outputWidth : outputHeight;
		const ctx = outCanvas.getContext('2d');
		if (!ctx) return;

		// Calculate what part of the image is in the crop area
		const iw = img.width * zoom;
		const ih = img.height * zoom;
		const ix = (CANVAS_SIZE - iw) / 2 + panX;
		const iy = (CANVAS_SIZE - ih) / 2 + panY;

		const cropLeft = (CANVAS_SIZE - cropW) / 2;
		const cropTop = (CANVAS_SIZE - cropH) / 2;

		// Source coordinates in original image space
		const sx = (cropLeft - ix) / zoom;
		const sy = (cropTop - iy) / zoom;
		const sw = cropW / zoom;
		const sh = cropH / zoom;

		if (shape === 'circle') {
			// Clip to circle
			ctx.beginPath();
			ctx.arc(outputWidth / 2, outputWidth / 2, outputWidth / 2, 0, Math.PI * 2);
			ctx.clip();
		}

		ctx.drawImage(img, sx, sy, sw, sh, 0, 0, outCanvas.width, outCanvas.height);

		outCanvas.toBlob(
			(blob) => {
				if (blob) oncrop(blob);
			},
			'image/png',
			0.92
		);
	}
</script>

<div class="flex flex-col items-center gap-4">
	<canvas
		bind:this={canvas}
		width={CANVAS_SIZE}
		height={CANVAS_SIZE}
		class="cursor-grab rounded-lg border border-bg-modifier {dragging ? 'cursor-grabbing' : ''}"
		onmousedown={handleMouseDown}
		onmousemove={handleMouseMove}
		onmouseup={handleMouseUp}
		onmouseleave={handleMouseUp}
		onwheel={handleWheel}
	></canvas>

	<div class="flex w-full items-center gap-2">
		<svg class="h-4 w-4 shrink-0 text-text-muted" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
			<path d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0zM10 7v6m3-3H7" />
		</svg>
		<input
			type="range"
			min="0.1"
			max="5"
			step="0.01"
			value={zoom}
			oninput={handleZoomInput}
			class="w-full accent-brand-500"
		/>
		<svg class="h-4 w-4 shrink-0 text-text-muted" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
			<path d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0zM10 7v6m3-3H7" />
		</svg>
	</div>

	<p class="text-2xs text-text-muted">Drag to position, scroll or use slider to zoom</p>

	<div class="flex gap-2">
		<button class="btn-secondary" onclick={oncancel}>Cancel</button>
		<button class="btn-primary" onclick={crop}>Apply Crop</button>
	</div>
</div>
