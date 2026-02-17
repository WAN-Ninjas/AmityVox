// Announcement store — manages real-time instance announcements.

import { writable, derived, readable } from 'svelte/store';

export interface AnnouncementData {
	id: string;
	title: string;
	content: string;
	severity: string;
	active?: boolean;
	expires_at?: string | null;
}

const announcements = writable<Map<string, AnnouncementData>>(new Map());

// Tick every 60s so expired announcements auto-dismiss.
const now = readable(Date.now(), (set) => {
	const id = setInterval(() => set(Date.now()), 60_000);
	return () => clearInterval(id);
});

export const activeAnnouncements = derived([announcements, now], ([$map, $now]) => {
	return Array.from($map.values()).filter((a) => {
		if (a.active === false) return false;
		if (a.expires_at && new Date(a.expires_at).getTime() < $now) return false;
		return true;
	});
});

export function setAnnouncements(list: AnnouncementData[]) {
	const map = new Map<string, AnnouncementData>();
	for (const a of list) {
		map.set(a.id, a);
	}
	announcements.set(map);
}

export function addAnnouncement(a: AnnouncementData) {
	announcements.update((map) => {
		const next = new Map(map);
		next.set(a.id, a);
		return next;
	});
}

export function updateAnnouncement(data: { id: string; active?: boolean | null; title?: string | null; content?: string | null }) {
	announcements.update((map) => {
		const existing = map.get(data.id);
		if (!existing) return map;
		const next = new Map(map);
		next.set(data.id, {
			...existing,
			...(data.active !== undefined && data.active !== null ? { active: data.active } : {}),
			...(data.title !== undefined && data.title !== null ? { title: data.title } : {}),
			...(data.content !== undefined && data.content !== null ? { content: data.content } : {})
		});
		return next;
	});
}

export function removeAnnouncement(id: string) {
	announcements.update((map) => {
		const next = new Map(map);
		next.delete(id);
		return next;
	});
}
