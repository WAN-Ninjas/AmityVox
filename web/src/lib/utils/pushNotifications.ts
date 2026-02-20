// Push notification registration and management utilities.
// Uses the Web Push API to subscribe/unsubscribe via VAPID keys.

import { api } from '$lib/api/client';

// Convert a base64url-encoded VAPID key to Uint8Array for PushManager.subscribe().
export function urlBase64ToUint8Array(base64String: string): Uint8Array {
	const padding = '='.repeat((4 - (base64String.length % 4)) % 4);
	const base64 = (base64String + padding).replace(/-/g, '+').replace(/_/g, '/');
	const rawData = atob(base64);
	const outputArray = new Uint8Array(rawData.length);
	for (let i = 0; i < rawData.length; i++) {
		outputArray[i] = rawData.charCodeAt(i);
	}
	return outputArray;
}

// Register the service worker and subscribe to push notifications.
export async function initPushNotifications(): Promise<boolean> {
	if (typeof navigator === 'undefined' || !('serviceWorker' in navigator)) return false;
	if (!('PushManager' in window)) return false;

	try {
		const registration = await navigator.serviceWorker.ready;

		// Check if already subscribed.
		const existing = await registration.pushManager.getSubscription();
		if (existing) return true;

		// Get VAPID key from server.
		const { vapid_public_key } = await api.getVapidKey();
		if (!vapid_public_key) return false;

		const applicationServerKey = urlBase64ToUint8Array(vapid_public_key);

		const subscription = await registration.pushManager.subscribe({
			userVisibleOnly: true,
			applicationServerKey,
		});

		// Extract keys and send to server.
		const json = subscription.toJSON();
		await api.subscribePush({
			endpoint: subscription.endpoint,
			keys: {
				p256dh: json.keys?.p256dh ?? '',
				auth: json.keys?.auth ?? '',
			},
		});

		return true;
	} catch (err) {
		console.warn('[Push] Failed to subscribe:', err);
		return false;
	}
}

// Unsubscribe from push notifications and remove from server.
export async function unsubscribePush(): Promise<boolean> {
	if (typeof navigator === 'undefined' || !('serviceWorker' in navigator)) return false;

	try {
		const registration = await navigator.serviceWorker.ready;
		const subscription = await registration.pushManager.getSubscription();
		if (!subscription) return true;

		// Find this subscription on the server by endpoint and delete it.
		const serverSubs = await api.getPushSubscriptions();
		const match = serverSubs.find((s) => s.endpoint === subscription.endpoint);
		if (match) {
			await api.deletePushSubscription(match.id);
		}

		await subscription.unsubscribe();
		return true;
	} catch (err) {
		console.warn('[Push] Failed to unsubscribe:', err);
		return false;
	}
}

// Check if push notifications are currently subscribed.
export async function isPushSubscribed(): Promise<boolean> {
	if (typeof navigator === 'undefined' || !('serviceWorker' in navigator)) return false;
	if (!('PushManager' in window)) return false;

	try {
		const registration = await navigator.serviceWorker.ready;
		const subscription = await registration.pushManager.getSubscription();
		return subscription !== null;
	} catch {
		return false;
	}
}
