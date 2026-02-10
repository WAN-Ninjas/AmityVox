<script lang="ts">
	// Admin dashboard — shows instance stats and quick actions.
	// Only visible to instance admins (UserFlagAdmin = 4).
</script>

<svelte:head>
	<title>Admin — AmityVox</title>
</svelte:head>

<div class="p-8">
	<h1 class="mb-6 text-2xl font-bold text-text-primary">Instance Dashboard</h1>

	<div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
		<div class="rounded-lg bg-bg-secondary p-4">
			<p class="text-sm text-text-muted">Total Users</p>
			<p class="mt-1 text-2xl font-bold text-text-primary">—</p>
		</div>
		<div class="rounded-lg bg-bg-secondary p-4">
			<p class="text-sm text-text-muted">Total Guilds</p>
			<p class="mt-1 text-2xl font-bold text-text-primary">—</p>
		</div>
		<div class="rounded-lg bg-bg-secondary p-4">
			<p class="text-sm text-text-muted">Messages Today</p>
			<p class="mt-1 text-2xl font-bold text-text-primary">—</p>
		</div>
		<div class="rounded-lg bg-bg-secondary p-4">
			<p class="text-sm text-text-muted">Active Connections</p>
			<p class="mt-1 text-2xl font-bold text-text-primary">—</p>
		</div>
	</div>

	<div class="mt-8">
		<h2 class="mb-4 text-lg font-semibold text-text-primary">Instance Info</h2>
		<div class="rounded-lg bg-bg-secondary p-4">
			<div class="grid gap-3 text-sm">
				<div class="flex justify-between">
					<span class="text-text-muted">Software</span>
					<span class="text-text-primary">AmityVox v0.3.0</span>
				</div>
				<div class="flex justify-between">
					<span class="text-text-muted">Federation</span>
					<span class="text-text-primary">—</span>
				</div>
				<div class="flex justify-between">
					<span class="text-text-muted">Storage</span>
					<span class="text-text-primary">—</span>
				</div>
			</div>
		</div>
	</div>
</div>
