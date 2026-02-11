<script lang="ts">
	// Typing indicator — shows who is typing in the current channel.
	// The gateway store will be extended to track typing state per channel.

	interface Props {
		typingUsers?: string[];
	}

	let { typingUsers = [] }: Props = $props();

	const text = $derived.by(() => {
		if (typingUsers.length === 0) return '';
		if (typingUsers.length === 1) return `${typingUsers[0]} is typing...`;
		if (typingUsers.length === 2) return `${typingUsers[0]} and ${typingUsers[1]} are typing...`;
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
