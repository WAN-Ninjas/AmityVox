<script lang="ts">
	// Typing indicator — shows who is typing in the current channel.
	// Receives user IDs and resolves display names from the member cache.

	import { api } from '$lib/api/client';

	interface Props {
		typingUsers?: string[];
	}

	let { typingUsers = [] }: Props = $props();

	// Simple cache for user display names.
	let nameCache = $state<Map<string, string>>(new Map());

	$effect(() => {
		for (const userId of typingUsers) {
			if (!nameCache.has(userId)) {
				api.getUser(userId)
					.then((u) => {
						nameCache.set(userId, u.display_name ?? u.username);
						nameCache = new Map(nameCache);
					})
					.catch(() => {
						nameCache.set(userId, userId.slice(0, 8));
						nameCache = new Map(nameCache);
					});
			}
		}
	});

	const names = $derived(typingUsers.map((id) => nameCache.get(id) ?? id.slice(0, 8)));

	const text = $derived.by(() => {
		if (names.length === 0) return '';
		if (names.length === 1) return `${names[0]} is typing...`;
		if (names.length === 2) return `${names[0]} and ${names[1]} are typing...`;
		return 'Several people are typing...';
	});
</script>

{#if text}
	<div class="flex items-center gap-2 px-4 pb-1 text-xs text-text-muted">
		<span class="flex gap-0.5">
			<span class="inline-block h-1.5 w-1.5 animate-bounce rounded-full bg-text-muted [animation-delay:0ms]"></span>
			<span class="inline-block h-1.5 w-1.5 animate-bounce rounded-full bg-text-muted [animation-delay:150ms]"></span>
			<span class="inline-block h-1.5 w-1.5 animate-bounce rounded-full bg-text-muted [animation-delay:300ms]"></span>
		</span>
		<span>{text}</span>
	</div>
{/if}
