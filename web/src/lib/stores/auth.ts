// Auth store — manages current user session state.

import { writable, derived } from 'svelte/store';
import type { User } from '$lib/types';
import { api } from '$lib/api/client';
import { setLocalInstanceId } from '$lib/utils/avatar';

export const currentUser = writable<User | null>(null);
export const isAuthenticated = derived(currentUser, ($user) => $user !== null);
export const isLoading = writable(true);

currentUser.subscribe((user) => {
	setLocalInstanceId(user?.instance_id ?? null);
});

export async function initAuth() {
	const token = api.getToken();
	if (!token) {
		isLoading.set(false);
		return;
	}

	try {
		const user = await api.getMe();
		currentUser.set(user);
	} catch {
		api.setToken(null);
	} finally {
		isLoading.set(false);
	}
}

export async function login(username: string, password: string, totpCode?: string) {
	const { user } = await api.login(username, password, totpCode);
	currentUser.set(user);
	return user;
}

export async function register(username: string, email: string, password: string, token?: string) {
	const { user } = await api.register(username, email, password, token);
	currentUser.set(user);
	return user;
}

export async function logout() {
	try {
		await api.logout();
	} finally {
		currentUser.set(null);
		api.setToken(null);
	}
}
