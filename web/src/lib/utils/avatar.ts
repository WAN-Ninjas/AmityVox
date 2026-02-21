/** Build the correct avatar/file URL, routing through federation proxy for remote instances. */
export function avatarUrl(avatarId: string | null | undefined, instanceId?: string | null): string | null {
	if (!avatarId) return null;
	if (instanceId) return `/api/v1/federation/media/${encodeURIComponent(instanceId)}/${encodeURIComponent(avatarId)}`;
	return `/api/v1/files/${avatarId}`;
}

/** Build a file URL, routing through federation proxy for remote instances. */
export function fileUrl(fileId: string, instanceId?: string | null): string {
	if (instanceId) return `/api/v1/federation/media/${encodeURIComponent(instanceId)}/${encodeURIComponent(fileId)}`;
	return `/api/v1/files/${fileId}`;
}
