// IndexedDB key storage for AmityVox E2EE.
// Database: amityvox-e2ee
// Object stores:
//   - channelSessions: per-channel AES-256-GCM session keys (keyed by channelId)

const DB_NAME = 'amityvox-e2ee';
const DB_VERSION = 2;
const STORE_CHANNEL_SESSIONS = 'channelSessions';

interface StoredChannelSession {
	channelId: string;
	sessionKey: JsonWebKey;
}

function openDB(): Promise<IDBDatabase> {
	return new Promise((resolve, reject) => {
		const request = indexedDB.open(DB_NAME, DB_VERSION);

		request.onupgradeneeded = () => {
			const db = request.result;
			// Remove legacy deviceKeys store if it exists
			if (db.objectStoreNames.contains('deviceKeys')) {
				db.deleteObjectStore('deviceKeys');
			}
			if (!db.objectStoreNames.contains(STORE_CHANNEL_SESSIONS)) {
				db.createObjectStore(STORE_CHANNEL_SESSIONS, { keyPath: 'channelId' });
			}
		};

		request.onsuccess = () => resolve(request.result);
		request.onerror = () => reject(request.error);
	});
}

function txGet<T>(db: IDBDatabase, storeName: string, key: string): Promise<T | undefined> {
	return new Promise((resolve, reject) => {
		const tx = db.transaction(storeName, 'readonly');
		const store = tx.objectStore(storeName);
		const request = store.get(key);
		request.onsuccess = () => resolve(request.result as T | undefined);
		request.onerror = () => reject(request.error);
	});
}

function txPut<T>(db: IDBDatabase, storeName: string, value: T): Promise<void> {
	return new Promise((resolve, reject) => {
		const tx = db.transaction(storeName, 'readwrite');
		const store = tx.objectStore(storeName);
		const request = store.put(value);
		request.onsuccess = () => resolve();
		request.onerror = () => reject(request.error);
	});
}

function txDelete(db: IDBDatabase, storeName: string, key: string): Promise<void> {
	return new Promise((resolve, reject) => {
		const tx = db.transaction(storeName, 'readwrite');
		const store = tx.objectStore(storeName);
		const request = store.delete(key);
		request.onsuccess = () => resolve();
		request.onerror = () => reject(request.error);
	});
}

function txGetAll<T>(db: IDBDatabase, storeName: string): Promise<T[]> {
	return new Promise((resolve, reject) => {
		const tx = db.transaction(storeName, 'readonly');
		const store = tx.objectStore(storeName);
		const request = store.getAll();
		request.onsuccess = () => resolve(request.result as T[]);
		request.onerror = () => reject(request.error);
	});
}

function txClear(db: IDBDatabase, storeName: string): Promise<void> {
	return new Promise((resolve, reject) => {
		const tx = db.transaction(storeName, 'readwrite');
		const store = tx.objectStore(storeName);
		const request = store.clear();
		request.onsuccess = () => resolve();
		request.onerror = () => reject(request.error);
	});
}

// --- Channel Session Keys ---

export async function saveChannelSessionKey(
	channelId: string,
	sessionKey: JsonWebKey
): Promise<void> {
	const db = await openDB();
	try {
		await txPut<StoredChannelSession>(db, STORE_CHANNEL_SESSIONS, { channelId, sessionKey });
	} finally {
		db.close();
	}
}

export async function loadChannelSessionKey(channelId: string): Promise<JsonWebKey | null> {
	const db = await openDB();
	try {
		const result = await txGet<StoredChannelSession>(db, STORE_CHANNEL_SESSIONS, channelId);
		return result?.sessionKey ?? null;
	} finally {
		db.close();
	}
}

export async function removeChannelSessionKey(channelId: string): Promise<void> {
	const db = await openDB();
	try {
		await txDelete(db, STORE_CHANNEL_SESSIONS, channelId);
	} finally {
		db.close();
	}
}

// --- Bulk Export / Import ---

export interface KeyExportData {
	channelSessions: { channelId: string; sessionKey: JsonWebKey }[];
}

export async function exportAllKeys(): Promise<KeyExportData> {
	const db = await openDB();
	try {
		const sessions = await txGetAll<StoredChannelSession>(db, STORE_CHANNEL_SESSIONS);
		return {
			channelSessions: sessions.map((s) => ({
				channelId: s.channelId,
				sessionKey: s.sessionKey
			}))
		};
	} finally {
		db.close();
	}
}

export async function importAllKeys(data: KeyExportData): Promise<void> {
	const db = await openDB();
	try {
		await txClear(db, STORE_CHANNEL_SESSIONS);
		for (const session of data.channelSessions) {
			await txPut<StoredChannelSession>(db, STORE_CHANNEL_SESSIONS, session);
		}
	} finally {
		db.close();
	}
}

// --- Clear All Keys ---

export async function clearAll(): Promise<void> {
	const db = await openDB();
	try {
		await txClear(db, STORE_CHANNEL_SESSIONS);
	} finally {
		db.close();
	}
}
