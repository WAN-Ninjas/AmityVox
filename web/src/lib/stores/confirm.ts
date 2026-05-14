import { writable } from 'svelte/store';

export interface ConfirmOptions {
	title?: string;
	message: string;
	confirmLabel?: string;
	cancelLabel?: string;
	variant?: 'danger' | 'primary';
}

interface PendingConfirm extends Required<Omit<ConfirmOptions, 'variant'>> {
	variant: 'danger' | 'primary';
	resolve: (confirmed: boolean) => void;
}

export const pendingConfirm = writable<PendingConfirm | null>(null);

export function confirmAction(options: ConfirmOptions): Promise<boolean> {
	return new Promise((resolve) => {
		pendingConfirm.set({
			title: options.title ?? 'Confirm action',
			message: options.message,
			confirmLabel: options.confirmLabel ?? 'Confirm',
			cancelLabel: options.cancelLabel ?? 'Cancel',
			variant: options.variant ?? 'danger',
			resolve
		});
	});
}

export function resolveConfirm(confirmed: boolean) {
	pendingConfirm.update((current) => {
		current?.resolve(confirmed);
		return null;
	});
}
