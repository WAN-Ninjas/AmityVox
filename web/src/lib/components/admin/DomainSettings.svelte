<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api/client';
	import { addToast } from '$lib/stores/toast';

	interface CustomDomain {
		id: string;
		guild_id: string;
		domain: string;
		verified: boolean;
		verification_token: string;
		ssl_provisioned: boolean;
		created_at: string;
		verified_at: string | null;
		guild_name: string;
	}

	let loading = $state(true);
	let domains = $state<CustomDomain[]>([]);
	let showAddForm = $state(false);
	let adding = $state(false);
	let verifyingId = $state('');

	// Add form
	let newGuildId = $state('');
	let newDomain = $state('');

	// Verification instructions display
	let showInstructions = $state<string | null>(null);

	async function loadDomains() {
		loading = true;
		try {
			const res = await fetch('/api/v1/admin/domains', {
				headers: { 'Authorization': `Bearer ${api.getToken()}` }
			});
			const json = await res.json();
			if (res.ok) {
				domains = json.data || [];
			}
		} catch {
			addToast('Failed to load custom domains', 'error');
		}
		loading = false;
	}

	async function addDomain() {
		if (!newGuildId.trim() || !newDomain.trim()) {
			addToast('Guild ID and domain are required', 'error');
			return;
		}
		adding = true;
		try {
			const res = await fetch('/api/v1/admin/domains', {
				method: 'POST',
				headers: {
					'Content-Type': 'application/json',
					'Authorization': `Bearer ${api.getToken()}`
				},
				body: JSON.stringify({ guild_id: newGuildId, domain: newDomain })
			});
			const json = await res.json();
			if (res.ok) {
				addToast('Domain added. Configure DNS to verify.', 'success');
				showAddForm = false;
				newGuildId = '';
				newDomain = '';
				await loadDomains();
			} else {
				addToast(json.error?.message || 'Failed to add domain', 'error');
			}
		} catch {
			addToast('Failed to add domain', 'error');
		}
		adding = false;
	}

	async function verifyDomain(domainId: string) {
		verifyingId = domainId;
		try {
			const res = await fetch(`/api/v1/admin/domains/${domainId}/verify`, {
				method: 'POST',
				headers: { 'Authorization': `Bearer ${api.getToken()}` }
			});
			const json = await res.json();
			if (res.ok) {
				addToast('Domain verified successfully', 'success');
				await loadDomains();
			} else {
				addToast(json.error?.message || 'Failed to verify domain', 'error');
			}
		} catch {
			addToast('Failed to verify domain', 'error');
		}
		verifyingId = '';
	}

	async function deleteDomain(domainId: string) {
		if (!confirm('Remove this custom domain?')) return;
		try {
			const res = await fetch(`/api/v1/admin/domains/${domainId}`, {
				method: 'DELETE',
				headers: { 'Authorization': `Bearer ${api.getToken()}` }
			});
			if (res.ok) {
				domains = domains.filter(d => d.id !== domainId);
				addToast('Domain removed', 'success');
			}
		} catch {
			addToast('Failed to remove domain', 'error');
		}
	}

	function formatDate(date: string | null): string {
		if (!date) return 'Never';
		return new Date(date).toLocaleDateString(undefined, {
			year: 'numeric', month: 'short', day: 'numeric'
		});
	}

	function copyToClipboard(text: string) {
		navigator.clipboard.writeText(text);
		addToast('Copied to clipboard', 'success');
	}

	onMount(() => {
		loadDomains();
	});
</script>

<div class="space-y-6">
	<!-- Header -->
	<div class="flex items-center justify-between">
		<div>
			<h2 class="text-xl font-bold text-text-primary">Custom Domains</h2>
			<p class="text-text-muted text-sm">Map custom domains to guilds (guild.example.com)</p>
		</div>
		<button
			class="btn-primary text-sm px-4 py-2"
			onclick={() => showAddForm = !showAddForm}
		>
			{showAddForm ? 'Cancel' : 'Add Domain'}
		</button>
	</div>

	<!-- How It Works -->
	<div class="bg-bg-tertiary rounded-lg p-4">
		<h3 class="text-sm font-semibold text-text-secondary mb-2">How Custom Domains Work</h3>
		<ol class="text-sm text-text-muted space-y-1 list-decimal list-inside">
			<li>Add a domain for a guild</li>
			<li>Add a TXT record to your DNS for verification</li>
			<li>Verify domain ownership</li>
			<li>Add a CNAME record pointing to your instance domain</li>
			<li>Caddy will auto-provision SSL for the domain</li>
		</ol>
	</div>

	<!-- Add Form -->
	{#if showAddForm}
		<div class="bg-bg-tertiary rounded-lg p-5 border border-bg-modifier">
			<h3 class="text-sm font-semibold text-text-secondary mb-4">Add Custom Domain</h3>
			<div class="space-y-4">
				<div>
					<label for="dom-guild-id" class="block text-sm font-medium text-text-secondary mb-1">Guild ID</label>
					<input id="dom-guild-id" type="text" class="input w-full" placeholder="Paste guild ULID" bind:value={newGuildId} />
				</div>
				<div>
					<label for="dom-domain" class="block text-sm font-medium text-text-secondary mb-1">Domain</label>
					<input id="dom-domain" type="text" class="input w-full" placeholder="community.example.com" bind:value={newDomain} />
					<p class="text-text-muted text-xs mt-1">Do not include http:// or https://</p>
				</div>
				<div class="flex justify-end gap-3">
					<button class="btn-secondary px-4 py-2 text-sm" onclick={() => { showAddForm = false; newGuildId = ''; newDomain = ''; }}>
						Cancel
					</button>
					<button class="btn-primary px-4 py-2 text-sm" onclick={addDomain} disabled={adding}>
						{adding ? 'Adding...' : 'Add Domain'}
					</button>
				</div>
			</div>
		</div>
	{/if}

	<!-- Domains List -->
	{#if loading && domains.length === 0}
		<div class="flex justify-center py-12">
			<div class="animate-spin w-8 h-8 border-2 border-brand-500 border-t-transparent rounded-full"></div>
		</div>
	{:else if domains.length === 0}
		<div class="bg-bg-tertiary rounded-lg p-8 text-center">
			<svg class="w-12 h-12 text-text-muted mx-auto mb-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M21 12a9 9 0 01-9 9m9-9a9 9 0 00-9-9m9 9H3m9 9a9 9 0 01-9-9m9 9c1.657 0 3-4.03 3-9s-1.343-9-3-9m0 18c-1.657 0-3-4.03-3-9s1.343-9 3-9m-9 9a9 9 0 019-9" />
			</svg>
			<p class="text-text-muted">No custom domains configured.</p>
		</div>
	{:else}
		<div class="space-y-3">
			{#each domains as domain}
				<div class="bg-bg-tertiary rounded-lg p-4">
					<div class="flex items-start justify-between">
						<div class="flex-1">
							<div class="flex items-center gap-2 mb-1">
								<span class="text-text-primary font-medium">{domain.domain}</span>
								{#if domain.verified}
									<span class="text-xs px-2 py-0.5 rounded-full bg-status-online/20 text-status-online">Verified</span>
								{:else}
									<span class="text-xs px-2 py-0.5 rounded-full bg-status-idle/20 text-status-idle">Pending Verification</span>
								{/if}
								{#if domain.ssl_provisioned}
									<span class="text-xs px-2 py-0.5 rounded-full bg-brand-500/20 text-brand-400">SSL Active</span>
								{/if}
							</div>
							<div class="text-sm text-text-muted">
								Guild: {domain.guild_name} - Created: {formatDate(domain.created_at)}
								{#if domain.verified_at}
									- Verified: {formatDate(domain.verified_at)}
								{/if}
							</div>
						</div>
						<div class="flex items-center gap-2 ml-4">
							{#if !domain.verified}
								<button
									class="btn-secondary text-xs px-3 py-1"
									onclick={() => showInstructions = showInstructions === domain.id ? null : domain.id}
								>
									DNS Setup
								</button>
								<button
									class="btn-primary text-xs px-3 py-1"
									onclick={() => verifyDomain(domain.id)}
									disabled={verifyingId === domain.id}
								>
									{verifyingId === domain.id ? 'Verifying...' : 'Verify'}
								</button>
							{/if}
							<button
								class="text-xs px-3 py-1 rounded bg-status-dnd/20 text-status-dnd hover:bg-status-dnd/30 transition-colors"
								onclick={() => deleteDomain(domain.id)}
							>
								Remove
							</button>
						</div>
					</div>

					<!-- Verification Instructions -->
					{#if showInstructions === domain.id}
						<div class="mt-3 p-3 bg-bg-primary rounded-lg border border-bg-modifier">
							<h4 class="text-sm font-semibold text-text-secondary mb-2">DNS Configuration</h4>
							<div class="space-y-3">
								<div>
									<p class="text-xs text-text-muted mb-1">Step 1: Add a TXT record for verification:</p>
									<div class="flex items-center gap-2">
										<code class="text-xs bg-bg-modifier px-2 py-1 rounded font-mono text-text-primary flex-1 overflow-x-auto">
											_amityvox-verify.{domain.domain} TXT {domain.verification_token}
										</code>
										<button
											class="text-xs text-brand-400 hover:text-brand-300 flex-shrink-0"
											onclick={() => copyToClipboard(domain.verification_token)}
										>
											Copy
										</button>
									</div>
								</div>
								<div>
									<p class="text-xs text-text-muted mb-1">Step 2: Add a CNAME record:</p>
									<code class="text-xs bg-bg-modifier px-2 py-1 rounded font-mono text-text-primary block overflow-x-auto">
										{domain.domain} CNAME your-instance-domain.com
									</code>
								</div>
								<p class="text-xs text-text-muted">
									After adding both DNS records, click "Verify" to confirm ownership.
									DNS propagation may take up to 48 hours.
								</p>
							</div>
						</div>
					{/if}
				</div>
			{/each}
		</div>
	{/if}
</div>
