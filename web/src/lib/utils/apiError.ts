import { ApiRequestError } from '$lib/api/client';

export function getErrorMessage(error: unknown, fallback = 'Something went wrong'): string {
	if (error instanceof ApiRequestError) {
		const prefix = error.status === 403 ? 'Permission denied' :
			error.status === 401 ? 'Authentication required' :
			error.status === 413 ? 'Upload too large' :
			error.status >= 500 ? 'Server error' : '';
		return prefix && error.message ? `${prefix}: ${error.message}` : error.message || prefix || fallback;
	}
	if (error instanceof Error && error.message) {
		return error.message;
	}
	return fallback;
}
