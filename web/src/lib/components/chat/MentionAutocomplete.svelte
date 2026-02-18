<script lang="ts">
	import { guildMembers, guildRolesMap } from '$lib/stores/members';
	import { canManageRoles } from '$lib/stores/permissions';
	import type { GuildMember, Role } from '$lib/types';

	interface Props {
		query: string;
		onSelect: (syntax: string, displayText: string) => void;
		onClose: () => void;
	}

	let { query, onSelect, onClose }: Props = $props();
	let selectedIndex = $state(0);

	// --- Filtering logic (exported for testing) ---

	const lowerQuery = $derived(query.toLowerCase());

	const filteredMembers = $derived.by(() => {
		if (!$guildMembers.size) return [];
		const results: GuildMember[] = [];
		for (const [, member] of $guildMembers) {
			if (results.length >= 10) break;
			const username = member.user?.username?.toLowerCase() ?? '';
			const displayName = member.user?.display_name?.toLowerCase() ?? '';
			const nickname = member.nickname?.toLowerCase() ?? '';
			if (username.includes(lowerQuery) || displayName.includes(lowerQuery) || nickname.includes(lowerQuery)) {
				results.push(member);
			}
		}
		return results;
	});

	const filteredRoles = $derived.by(() => {
		if (!$guildRolesMap.size) return [];
		const results: Role[] = [];
		for (const [, role] of $guildRolesMap) {
			if (results.length >= 10) break;
			if (!role.mentionable && !$canManageRoles) continue;
			if (role.name.toLowerCase().includes(lowerQuery)) {
				results.push(role);
			}
		}
		return results;
	});

	const showHere = $derived('here'.startsWith(lowerQuery) || lowerQuery === '');

	// Build flat list of all items for keyboard navigation.
	interface AutocompleteItem {
		type: 'here' | 'user' | 'role';
		id: string;
		label: string;
		sublabel?: string;
		color?: string;
	}

	const items = $derived.by(() => {
		const list: AutocompleteItem[] = [];
		if (showHere) {
			list.push({ type: 'here', id: 'here', label: '@here', sublabel: 'Notify all channel members' });
		}
		for (const member of filteredMembers) {
			const displayName = member.nickname ?? member.user?.display_name ?? member.user?.username ?? 'Unknown';
			const username = member.user?.username ?? '';
			list.push({
				type: 'user',
				id: member.user_id,
				label: displayName,
				sublabel: username !== displayName ? username : undefined
			});
		}
		for (const role of filteredRoles) {
			list.push({
				type: 'role',
				id: role.id,
				label: role.name,
				color: role.color ?? undefined
			});
		}
		return list;
	});

	// Reset selection when items change.
	$effect(() => {
		items; // track dependency
		selectedIndex = 0;
	});

	function selectItem(item: AutocompleteItem) {
		if (item.type === 'here') {
			onSelect('@here', '@here');
		} else if (item.type === 'user') {
			onSelect(`<@${item.id}>`, `@${item.label}`);
		} else {
			onSelect(`<@&${item.id}>`, `@${item.label}`);
		}
	}

	export function handleKeydown(e: KeyboardEvent): boolean {
		if (items.length === 0) return false;

		if (e.key === 'ArrowDown') {
			e.preventDefault();
			selectedIndex = (selectedIndex + 1) % items.length;
			return true;
		}
		if (e.key === 'ArrowUp') {
			e.preventDefault();
			selectedIndex = (selectedIndex - 1 + items.length) % items.length;
			return true;
		}
		if (e.key === 'Enter' || e.key === 'Tab') {
			e.preventDefault();
			selectItem(items[selectedIndex]);
			return true;
		}
		if (e.key === 'Escape') {
			e.preventDefault();
			onClose();
			return true;
		}
		return false;
	}
</script>

{#if items.length > 0}
	<div class="absolute bottom-full left-0 right-0 mb-1 max-h-64 overflow-y-auto rounded-lg border border-border-primary bg-bg-secondary shadow-lg z-50">
		{#each items as item, i}
			<button
				class="flex w-full items-center gap-2 px-3 py-1.5 text-left text-sm transition-colors {i === selectedIndex ? 'bg-bg-modifier text-text-primary' : 'text-text-secondary hover:bg-bg-modifier/50'}"
				onmouseenter={() => selectedIndex = i}
				onclick={() => selectItem(item)}
				type="button"
			>
				{#if item.type === 'here'}
					<span class="flex h-6 w-6 items-center justify-center rounded-full bg-yellow-500/20 text-xs text-yellow-300">@</span>
					<div class="min-w-0 flex-1">
						<span class="font-medium text-yellow-300">{item.label}</span>
						<span class="ml-2 text-xs text-text-muted">{item.sublabel}</span>
					</div>
				{:else if item.type === 'user'}
					<span class="flex h-6 w-6 items-center justify-center rounded-full bg-brand-500/20 text-xs text-brand-300">
						{item.label.charAt(0).toUpperCase()}
					</span>
					<div class="min-w-0 flex-1">
						<span class="font-medium">{item.label}</span>
						{#if item.sublabel}
							<span class="ml-2 text-xs text-text-muted">{item.sublabel}</span>
						{/if}
					</div>
				{:else}
					<span
						class="flex h-6 w-6 items-center justify-center rounded-full text-xs"
						style="background-color: {item.color ?? '#99aab5'}30; color: {item.color ?? '#99aab5'}"
					>@</span>
					<div class="min-w-0 flex-1">
						<span class="font-medium" style="color: {item.color ?? 'inherit'}">{item.label}</span>
						<span class="ml-2 text-xs text-text-muted">Role</span>
					</div>
				{/if}
			</button>
		{/each}
	</div>
{/if}
