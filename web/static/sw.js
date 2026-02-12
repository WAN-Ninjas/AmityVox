// AmityVox Service Worker — Offline Support & Caching
// Implements a stale-while-revalidate strategy for static assets
// and a network-first strategy for API calls.

const CACHE_VERSION = 'amityvox-v1';
const STATIC_CACHE = `${CACHE_VERSION}-static`;
const API_CACHE = `${CACHE_VERSION}-api`;

// Static assets to pre-cache on install.
const PRECACHE_URLS = [
	'/',
	'/app',
	'/login',
	'/register'
];

// URL patterns for different caching strategies.
const STATIC_PATTERNS = [
	/\/_app\/immutable\//,    // SvelteKit immutable assets (hashed filenames)
	/\.(?:js|css|woff2?|ttf|eot|svg|png|jpg|jpeg|gif|webp|ico)$/
];

const API_PATTERNS = [
	/\/api\/v1\//
];

const NO_CACHE_PATTERNS = [
	/\/api\/v1\/auth\//,       // Never cache auth endpoints
	/\/ws$/,                    // Never cache WebSocket
	/\/health/                  // Never cache health checks
];

// ============================================================
// Install — Pre-cache critical assets
// ============================================================
self.addEventListener('install', (event) => {
	event.waitUntil(
		caches.open(STATIC_CACHE)
			.then((cache) => cache.addAll(PRECACHE_URLS))
			.then(() => self.skipWaiting())
			.catch((err) => {
				console.warn('[SW] Pre-cache failed (non-fatal):', err);
				return self.skipWaiting();
			})
	);
});

// ============================================================
// Activate — Clean up old caches
// ============================================================
self.addEventListener('activate', (event) => {
	event.waitUntil(
		caches.keys()
			.then((keys) => {
				return Promise.all(
					keys
						.filter((key) => key.startsWith('amityvox-') && key !== STATIC_CACHE && key !== API_CACHE)
						.map((key) => caches.delete(key))
				);
			})
			.then(() => self.clients.claim())
	);
});

// ============================================================
// Fetch — Route to appropriate caching strategy
// ============================================================
self.addEventListener('fetch', (event) => {
	const url = new URL(event.request.url);

	// Skip non-GET requests.
	if (event.request.method !== 'GET') return;

	// Skip cross-origin requests.
	if (url.origin !== self.location.origin) return;

	// Skip no-cache patterns.
	if (NO_CACHE_PATTERNS.some((p) => p.test(url.pathname))) return;

	// API requests — network first, fall back to cache.
	if (API_PATTERNS.some((p) => p.test(url.pathname))) {
		event.respondWith(networkFirst(event.request, API_CACHE));
		return;
	}

	// Static assets — stale while revalidate.
	if (STATIC_PATTERNS.some((p) => p.test(url.pathname))) {
		event.respondWith(staleWhileRevalidate(event.request, STATIC_CACHE));
		return;
	}

	// Navigation requests — network first with offline fallback.
	if (event.request.mode === 'navigate') {
		event.respondWith(networkFirstNavigation(event.request));
		return;
	}
});

// ============================================================
// Caching Strategies
// ============================================================

/**
 * Network First — Try network, fall back to cache.
 * Best for API requests where freshness matters.
 */
async function networkFirst(request, cacheName) {
	try {
		const response = await fetch(request);
		if (response.ok) {
			const cache = await caches.open(cacheName);
			cache.put(request, response.clone());
		}
		return response;
	} catch {
		const cached = await caches.match(request);
		if (cached) return cached;
		return new Response(
			JSON.stringify({ error: { code: 'offline', message: 'You are offline' } }),
			{
				status: 503,
				headers: { 'Content-Type': 'application/json' }
			}
		);
	}
}

/**
 * Stale While Revalidate — Serve from cache immediately, update in background.
 * Best for static assets where speed matters more than freshness.
 */
async function staleWhileRevalidate(request, cacheName) {
	const cache = await caches.open(cacheName);
	const cached = await cache.match(request);

	const fetchPromise = fetch(request)
		.then((response) => {
			if (response.ok) {
				cache.put(request, response.clone());
			}
			return response;
		})
		.catch(() => cached);

	return cached || fetchPromise;
}

/**
 * Network First Navigation — For HTML page loads.
 * Falls back to cached pages or a simple offline message.
 */
async function networkFirstNavigation(request) {
	try {
		const response = await fetch(request);
		if (response.ok) {
			const cache = await caches.open(STATIC_CACHE);
			cache.put(request, response.clone());
		}
		return response;
	} catch {
		const cached = await caches.match(request);
		if (cached) return cached;

		// Return a minimal offline page.
		return new Response(
			`<!DOCTYPE html>
<html lang="en">
<head>
	<meta charset="utf-8">
	<meta name="viewport" content="width=device-width, initial-scale=1">
	<title>AmityVox - Offline</title>
	<style>
		body {
			font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
			display: flex; align-items: center; justify-content: center;
			min-height: 100vh; margin: 0;
			background: #1a1a2e; color: #e0e0e0;
		}
		.container { text-align: center; padding: 2rem; }
		h1 { font-size: 1.5rem; margin-bottom: 0.5rem; }
		p { color: #999; margin-bottom: 1.5rem; }
		button {
			padding: 0.75rem 1.5rem; border: none; border-radius: 0.5rem;
			background: #6366f1; color: white; cursor: pointer; font-size: 1rem;
		}
		button:hover { background: #5558e6; }
	</style>
</head>
<body>
	<div class="container">
		<h1>You're Offline</h1>
		<p>AmityVox can't connect to the server right now. Check your internet connection and try again.</p>
		<button onclick="location.reload()">Retry</button>
	</div>
</body>
</html>`,
			{
				status: 503,
				headers: { 'Content-Type': 'text/html' }
			}
		);
	}
}

// ============================================================
// Push Notifications
// ============================================================
self.addEventListener('push', (event) => {
	if (!event.data) return;

	try {
		const data = event.data.json();
		const options = {
			body: data.body || '',
			icon: '/favicon.png',
			badge: '/favicon.png',
			tag: data.tag || 'amityvox-notification',
			data: {
				url: data.url || '/app'
			},
			actions: data.actions || [],
			requireInteraction: data.requireInteraction || false
		};

		event.waitUntil(
			self.registration.showNotification(data.title || 'AmityVox', options)
		);
	} catch {
		// Fallback for non-JSON push data.
		event.waitUntil(
			self.registration.showNotification('AmityVox', {
				body: event.data.text(),
				icon: '/favicon.png'
			})
		);
	}
});

// Handle notification clicks — navigate to the relevant page.
self.addEventListener('notificationclick', (event) => {
	event.notification.close();

	const url = event.notification.data?.url || '/app';

	event.waitUntil(
		self.clients.matchAll({ type: 'window', includeUncontrolled: true })
			.then((clients) => {
				// Focus an existing window if one is open.
				for (const client of clients) {
					if (client.url.includes('/app') && 'focus' in client) {
						client.navigate(url);
						return client.focus();
					}
				}
				// Otherwise open a new window.
				return self.clients.openWindow(url);
			})
	);
});

// ============================================================
// Background Sync (future use)
// ============================================================
self.addEventListener('sync', (event) => {
	if (event.tag === 'amityvox-outbox') {
		// Future: retry failed message sends when back online.
		event.waitUntil(Promise.resolve());
	}
});
