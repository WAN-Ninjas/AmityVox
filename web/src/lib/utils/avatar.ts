/** Build the correct avatar URL, pointing at the remote instance for federated users. */
export function avatarUrl(avatarId: string | null | undefined, instanceDomain?: string | null): string | null {
	if (!avatarId) return null;
	if (instanceDomain) return `https://${instanceDomain}/api/v1/files/${avatarId}`;
	return `/api/v1/files/${avatarId}`;
}
