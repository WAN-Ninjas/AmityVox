<script lang="ts">
	// Friends page — placeholder for friends list / DM management.
</script>

<svelte:head>
	<title>Friends — AmityVox</title>
</svelte:head>

<div class="flex h-full flex-col bg-bg-tertiary">
	<header class="flex h-12 items-center border-b border-bg-floating px-4">
		<h1 class="font-semibold text-text-primary">Friends</h1>
		<div class="ml-4 flex gap-2">
			<button class="btn-secondary text-xs">All</button>
			<button class="rounded px-3 py-1 text-xs text-text-muted hover:bg-bg-modifier hover:text-text-secondary">
				Online
			</button>
			<button class="rounded px-3 py-1 text-xs text-text-muted hover:bg-bg-modifier hover:text-text-secondary">
				Pending
			</button>
			<button class="rounded px-3 py-1 text-xs text-text-muted hover:bg-bg-modifier hover:text-text-secondary">
				Blocked
			</button>
			<button class="btn-primary text-xs">Add Friend</button>
		</div>
	</header>

	<div class="flex flex-1 items-center justify-center">
		<div class="text-center">
			<p class="text-lg text-text-secondary">No friends yet</p>
			<p class="text-sm text-text-muted">Add friends to start chatting!</p>
		</div>
	</div>
</div>
