<!-- LocationShare.svelte — Displays GPS coordinates on an interactive map tile. -->
<script lang="ts">
	import { api } from '$lib/api/client';

	interface LocationData {
		id: string;
		user_id: string;
		channel_id: string;
		latitude: number;
		longitude: number;
		accuracy?: number;
		altitude?: number;
		label?: string;
		live: boolean;
		expires_at?: string;
		created_at: string;
		updated_at: string;
		username: string;
		display_name?: string;
		avatar_id?: string;
	}

	interface Props {
		channelId: string;
		location?: LocationData;
		compact?: boolean;
	}

	let { channelId, location, compact = false }: Props = $props();

	let locations = $state<LocationData[]>([]);
	let loading = $state(false);
	let sharing = $state(false);
	let error = $state('');
	let liveLocationId = $state<string | null>(null);
	let liveInterval = $state<ReturnType<typeof setInterval> | null>(null);

	// Map tile URL using OpenStreetMap (no API key needed).
	function mapTileUrl(lat: number, lon: number, zoom: number = 15): string {
		return `https://www.openstreetmap.org/export/embed.html?bbox=${lon - 0.01},${lat - 0.01},${lon + 0.01},${lat + 0.01}&layer=mapnik&marker=${lat},${lon}`;
	}

	function staticMapUrl(lat: number, lon: number): string {
		// Uses an OSM-based static map tile service.
		const zoom = 15;
		const x = Math.floor(((lon + 180) / 360) * Math.pow(2, zoom));
		const y = Math.floor(
			((1 - Math.log(Math.tan((lat * Math.PI) / 180) + 1 / Math.cos((lat * Math.PI) / 180)) / Math.PI) / 2) *
				Math.pow(2, zoom)
		);
		return `https://tile.openstreetmap.org/${zoom}/${x}/${y}.png`;
	}

	function formatCoords(lat: number, lon: number): string {
		const latDir = lat >= 0 ? 'N' : 'S';
		const lonDir = lon >= 0 ? 'E' : 'W';
		return `${Math.abs(lat).toFixed(6)}${latDir}, ${Math.abs(lon).toFixed(6)}${lonDir}`;
	}

	function timeAgo(dateStr: string): string {
		const seconds = Math.floor((Date.now() - new Date(dateStr).getTime()) / 1000);
		if (seconds < 60) return 'just now';
		if (seconds < 3600) return `${Math.floor(seconds / 60)}m ago`;
		if (seconds < 86400) return `${Math.floor(seconds / 3600)}h ago`;
		return `${Math.floor(seconds / 86400)}d ago`;
	}

	async function loadLocations() {
		loading = true;
		error = '';
		try {
			const data = await api.request<LocationData[]>(
				'GET',
				`/channels/${channelId}/experimental/locations`
			);
			locations = data ?? [];
		} catch (err: any) {
			error = err.message || 'Failed to load locations';
		} finally {
			loading = false;
		}
	}

	async function shareMyLocation(live: boolean = false) {
		sharing = true;
		error = '';
		try {
			const pos = await new Promise<GeolocationPosition>((resolve, reject) => {
				navigator.geolocation.getCurrentPosition(resolve, reject, {
					enableHighAccuracy: true,
					timeout: 10000
				});
			});

			const result = await api.request<LocationData>('POST', `/channels/${channelId}/experimental/location`, {
				latitude: pos.coords.latitude,
				longitude: pos.coords.longitude,
				accuracy: pos.coords.accuracy,
				altitude: pos.coords.altitude,
				live,
				duration: live ? 3600 : 0
			});

			if (live && result) {
				liveLocationId = result.id;
				startLiveUpdates();
			}

			await loadLocations();
		} catch (err: any) {
			if (err.code === 1) {
				error = 'Location permission denied. Please allow location access.';
			} else {
				error = err.message || 'Failed to share location';
			}
		} finally {
			sharing = false;
		}
	}

	function startLiveUpdates() {
		if (liveInterval) clearInterval(liveInterval);
		liveInterval = setInterval(async () => {
			if (!liveLocationId) {
				stopLiveSharing();
				return;
			}
			try {
				const pos = await new Promise<GeolocationPosition>((resolve, reject) => {
					navigator.geolocation.getCurrentPosition(resolve, reject, {
						enableHighAccuracy: true,
						timeout: 5000
					});
				});
				await api.request('PATCH', `/channels/${channelId}/experimental/location/${liveLocationId}`, {
					latitude: pos.coords.latitude,
					longitude: pos.coords.longitude,
					accuracy: pos.coords.accuracy,
					altitude: pos.coords.altitude
				});
			} catch {
				// Silently retry on next interval.
			}
		}, 10000); // Update every 10 seconds.
	}

	async function stopLiveSharing() {
		if (liveInterval) {
			clearInterval(liveInterval);
			liveInterval = null;
		}
		if (liveLocationId) {
			try {
				await api.request('DELETE', `/channels/${channelId}/experimental/location/${liveLocationId}`);
			} catch {
				// Ignore errors on cleanup.
			}
			liveLocationId = null;
		}
	}

	function openInMaps(lat: number, lon: number) {
		window.open(`https://www.openstreetmap.org/?mlat=${lat}&mlon=${lon}#map=16/${lat}/${lon}`, '_blank');
	}

	$effect(() => {
		if (channelId && !location) {
			loadLocations();
		}
		return () => {
			if (liveInterval) clearInterval(liveInterval);
		};
	});
</script>

{#if location}
	<!-- Single location display (inline in a message) -->
	<div class="rounded-lg border border-border-primary bg-bg-secondary overflow-hidden {compact ? 'max-w-xs' : 'max-w-sm'}">
		<button
			type="button"
			class="w-full cursor-pointer"
			onclick={() => openInMaps(location.latitude, location.longitude)}
		>
			<img
				src={staticMapUrl(location.latitude, location.longitude)}
				alt="Map showing shared location"
				class="w-full h-32 object-cover"
			/>
		</button>
		<div class="p-3">
			<div class="flex items-center gap-2 mb-1">
				<svg class="w-4 h-4 text-brand-400 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
					<path stroke-linecap="round" stroke-linejoin="round" d="M17.657 16.657L13.414 20.9a1.998 1.998 0 01-2.827 0l-4.244-4.243a8 8 0 1111.314 0z" />
					<path stroke-linecap="round" stroke-linejoin="round" d="M15 11a3 3 0 11-6 0 3 3 0 016 0z" />
				</svg>
				{#if location.label}
					<span class="text-text-primary text-sm font-medium truncate">{location.label}</span>
				{:else}
					<span class="text-text-secondary text-xs font-mono">{formatCoords(location.latitude, location.longitude)}</span>
				{/if}
				{#if location.live}
					<span class="inline-flex items-center gap-1 px-1.5 py-0.5 bg-green-500/20 text-green-400 text-xs rounded-full">
						<span class="w-1.5 h-1.5 bg-green-400 rounded-full animate-pulse"></span>
						Live
					</span>
				{/if}
			</div>
			{#if location.accuracy}
				<p class="text-text-muted text-xs">Accuracy: ~{Math.round(location.accuracy)}m</p>
			{/if}
			<p class="text-text-muted text-xs mt-0.5">
				{location.display_name ?? location.username} - {timeAgo(location.updated_at)}
			</p>
		</div>
	</div>
{:else}
	<!-- Location sharing panel -->
	<div class="p-4">
		{#if error}
			<div class="mb-3 p-2 bg-red-500/10 border border-red-500/20 rounded text-red-400 text-sm">{error}</div>
		{/if}

		<div class="flex gap-2 mb-4">
			<button
				class="btn-primary text-sm px-3 py-1.5 rounded flex items-center gap-1.5"
				disabled={sharing}
				onclick={() => shareMyLocation(false)}
			>
				<svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
					<path stroke-linecap="round" stroke-linejoin="round" d="M17.657 16.657L13.414 20.9a1.998 1.998 0 01-2.827 0l-4.244-4.243a8 8 0 1111.314 0z" />
				</svg>
				{sharing ? 'Sharing...' : 'Share Location'}
			</button>

			{#if !liveLocationId}
				<button
					class="btn-secondary text-sm px-3 py-1.5 rounded flex items-center gap-1.5"
					disabled={sharing}
					onclick={() => shareMyLocation(true)}
				>
					<span class="w-2 h-2 bg-green-400 rounded-full"></span>
					Share Live (1h)
				</button>
			{:else}
				<button
					class="text-sm px-3 py-1.5 rounded bg-red-500/20 text-red-400 hover:bg-red-500/30 flex items-center gap-1.5"
					onclick={stopLiveSharing}
				>
					Stop Live
				</button>
			{/if}
		</div>

		{#if loading}
			<div class="text-text-muted text-sm">Loading locations...</div>
		{:else if locations.length === 0}
			<div class="text-text-muted text-sm">No shared locations in this channel.</div>
		{:else}
			<div class="space-y-2">
				{#each locations as loc (loc.id)}
					<button
						type="button"
						class="w-full flex items-center gap-3 p-2 rounded hover:bg-bg-tertiary cursor-pointer text-left"
						onclick={() => openInMaps(loc.latitude, loc.longitude)}
					>
						<div class="w-12 h-12 rounded overflow-hidden shrink-0">
							<img src={staticMapUrl(loc.latitude, loc.longitude)} alt="Map tile" class="w-full h-full object-cover" />
						</div>
						<div class="min-w-0 flex-1">
							<div class="flex items-center gap-2">
								<span class="text-text-primary text-sm font-medium truncate">
									{loc.label || formatCoords(loc.latitude, loc.longitude)}
								</span>
								{#if loc.live}
									<span class="inline-flex items-center gap-1 px-1.5 py-0.5 bg-green-500/20 text-green-400 text-xs rounded-full shrink-0">
										<span class="w-1.5 h-1.5 bg-green-400 rounded-full animate-pulse"></span>
										Live
									</span>
								{/if}
							</div>
							<p class="text-text-muted text-xs truncate">
								{loc.display_name ?? loc.username} - {timeAgo(loc.updated_at)}
							</p>
						</div>
					</button>
				{/each}
			</div>
		{/if}
	</div>
{/if}
