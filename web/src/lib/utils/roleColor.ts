import type { Role } from '$lib/types';

/**
 * Returns the CSS color of a member's highest-position role that has a color set.
 * Returns null if no colored role is found.
 */
export function getMemberRoleColor(
	memberRoleIds: string[] | undefined,
	roleMap: Map<string, Role>
): string | null {
	if (!memberRoleIds || memberRoleIds.length === 0) return null;

	let bestColor: string | null = null;
	let bestPosition = -1;

	for (const rid of memberRoleIds) {
		const role = roleMap.get(rid);
		if (role && role.color && role.position > bestPosition) {
			bestColor = role.color;
			bestPosition = role.position;
		}
	}

	return bestColor;
}
