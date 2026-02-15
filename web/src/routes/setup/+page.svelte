<script lang="ts">
	import { onMount } from 'svelte';

	const API_BASE = '/api/v1';

	let step = $state(1);
	let totalSteps = 4;
	let loading = $state(true);
	let submitting = $state(false);
	let error = $state('');
	let setupComplete = $state(false);
	let alreadyCompleted = $state(false);

	// Step 1: Instance info
	let instanceName = $state('');
	let description = $state('');
	let domain = $state('');

	// Step 2: Admin account
	let adminUsername = $state('');
	let adminEmail = $state('');
	let adminPassword = $state('');
	let adminPasswordConfirm = $state('');

	// Step 3: Configuration
	let federationMode = $state('closed');
	let registrationMode = $state('open');

	// Step 4: Review

	onMount(async () => {
		try {
			const res = await fetch(`${API_BASE}/admin/setup/status`);
			const json = await res.json();
			if (json.data?.completed) {
				alreadyCompleted = true;
			}
			if (json.data?.instance_name) {
				instanceName = json.data.instance_name;
			}
		} catch {
			// Setup endpoint may not require auth
		}
		loading = false;
	});

	function nextStep() {
		error = '';
		if (step === 1) {
			if (!instanceName.trim()) {
				error = 'Instance name is required.';
				return;
			}
		}
		if (step === 2) {
			if (!adminUsername.trim() || !adminEmail.trim() || !adminPassword) {
				error = 'All admin account fields are required.';
				return;
			}
			if (adminPassword !== adminPasswordConfirm) {
				error = 'Passwords do not match.';
				return;
			}
			if (adminPassword.length < 8) {
				error = 'Password must be at least 8 characters.';
				return;
			}
		}
		if (step < totalSteps) step++;
	}

	function prevStep() {
		error = '';
		if (step > 1) step--;
	}

	async function completeSetup() {
		error = '';
		submitting = true;
		try {
			// Step 1: Register admin account if credentials provided.
			if (adminUsername && adminEmail && adminPassword) {
				const regRes = await fetch(`${API_BASE}/auth/register`, {
					method: 'POST',
					headers: { 'Content-Type': 'application/json' },
					body: JSON.stringify({
						username: adminUsername,
						email: adminEmail,
						password: adminPassword
					})
				});
				const regJson = await regRes.json();
				if (!regRes.ok) {
					// If user already exists, try login instead.
					if (regJson.error?.code !== 'username_taken' && regJson.error?.code !== 'email_taken') {
						error = regJson.error?.message || 'Failed to create admin account.';
						submitting = false;
						return;
					}
				}

				// Get token from register or login.
				let token = regJson.data?.token;
				if (!token) {
					const loginRes = await fetch(`${API_BASE}/auth/login`, {
						method: 'POST',
						headers: { 'Content-Type': 'application/json' },
						body: JSON.stringify({ username: adminUsername, password: adminPassword })
					});
					const loginJson = await loginRes.json();
					token = loginJson.data?.token;
				}

				if (token) {
					localStorage.setItem('token', token);

					// Promote to admin.
					const meRes = await fetch(`${API_BASE}/users/@me`, {
						headers: { 'Authorization': `Bearer ${token}` }
					});
					const meJson = await meRes.json();
					const userId = meJson.data?.id;

					if (userId) {
						await fetch(`${API_BASE}/admin/users/${userId}/set-admin`, {
							method: 'POST',
							headers: {
								'Content-Type': 'application/json',
								'Authorization': `Bearer ${token}`
							},
							body: JSON.stringify({ admin: true })
						});
					}
				}
			}

			// Step 2: Complete setup.
			const token = localStorage.getItem('token');
			const headers: Record<string, string> = { 'Content-Type': 'application/json' };
			if (token) headers['Authorization'] = `Bearer ${token}`;

			const setupRes = await fetch(`${API_BASE}/admin/setup/complete`, {
				method: 'POST',
				headers,
				body: JSON.stringify({
					instance_name: instanceName,
					description,
					domain,
					federation_mode: federationMode,
					registration_mode: registrationMode,
					admin_username: adminUsername,
					admin_email: adminEmail
				})
			});

			const setupJson = await setupRes.json();
			if (!setupRes.ok) {
				error = setupJson.error?.message || 'Setup failed.';
				submitting = false;
				return;
			}

			setupComplete = true;
		} catch (e: unknown) {
			error = e instanceof Error ? e.message : 'An unexpected error occurred.';
		}
		submitting = false;
	}

	function goToApp() {
		window.location.href = '/app';
	}
</script>

<div class="min-h-screen bg-bg-primary flex items-center justify-center p-4">
	<div class="w-full max-w-2xl">
		{#if loading}
			<div class="bg-bg-secondary rounded p-12 text-center">
				<div class="animate-spin w-8 h-8 border-2 border-brand-500 border-t-transparent rounded-full mx-auto"></div>
				<p class="text-text-secondary mt-4">Checking setup status...</p>
			</div>
		{:else if setupComplete}
			<div class="bg-bg-secondary rounded border-t-2 border-brand-500 p-12 text-center">
				<div class="w-16 h-16 bg-status-online/20 rounded-full flex items-center justify-center mx-auto mb-4">
					<svg class="w-8 h-8 text-status-online" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
					</svg>
				</div>
				<h1 class="text-2xl font-bold text-text-primary mb-2">Setup Complete!</h1>
				<p class="text-text-secondary mb-6">
					Your AmityVox instance "{instanceName}" is ready to use.
				</p>
				<button class="btn-primary px-8 py-3 text-lg" onclick={goToApp}>
					Launch AmityVox
				</button>
			</div>
		{:else if alreadyCompleted}
			<div class="bg-bg-secondary rounded border-t-2 border-brand-500 p-12 text-center">
				<h1 class="text-2xl font-bold text-text-primary mb-2">Already Configured</h1>
				<p class="text-text-secondary mb-6">
					This instance has already been set up. You can reconfigure it from the admin dashboard.
				</p>
				<button class="btn-primary px-8 py-3" onclick={goToApp}>
					Go to App
				</button>
			</div>
		{:else}
			<!-- Header -->
			<div class="text-center mb-8">
				<h1 class="text-3xl font-bold text-text-primary mb-2">Welcome to AmityVox</h1>
				<p class="text-text-secondary">Let's set up your instance. This will only take a minute.</p>
			</div>

			<!-- Progress bar -->
			<div class="flex items-center gap-2 mb-8">
				{#each Array(totalSteps) as _, i}
					<div class="flex-1 h-2 rounded-sm transition-colors {i < step ? 'bg-brand-500' : 'bg-bg-modifier'}"></div>
				{/each}
			</div>

			<div class="bg-bg-secondary rounded border-t-2 border-brand-500 p-8">
				{#if error}
					<div class="bg-status-dnd/10 border border-status-dnd/30 rounded-lg p-3 mb-6">
						<p class="text-sm text-status-dnd">{error}</p>
					</div>
				{/if}

				<!-- Step 1: Instance Info -->
				{#if step === 1}
					<h2 class="text-xl font-semibold text-text-primary mb-1">Instance Information</h2>
					<p class="text-text-muted text-sm mb-6">Give your instance a name and description.</p>

					<div class="space-y-4">
						<div>
							<label for="instance-name" class="block text-sm font-medium text-text-secondary mb-1">Instance Name *</label>
							<input
								id="instance-name"
								type="text"
								class="input w-full"
								placeholder="My AmityVox Server"
								bind:value={instanceName}
							/>
						</div>
						<div>
							<label for="instance-desc" class="block text-sm font-medium text-text-secondary mb-1">Description</label>
							<textarea
								id="instance-desc"
								class="input w-full h-24 resize-none"
								placeholder="A brief description of your instance..."
								bind:value={description}
							></textarea>
						</div>
						<div>
							<label for="instance-domain" class="block text-sm font-medium text-text-secondary mb-1">Domain</label>
							<input
								id="instance-domain"
								type="text"
								class="input w-full"
								placeholder="chat.example.com"
								bind:value={domain}
							/>
							<p class="text-text-muted text-xs mt-1">The domain this instance will be accessible at.</p>
						</div>
					</div>
				{/if}

				<!-- Step 2: Admin Account -->
				{#if step === 2}
					<h2 class="text-xl font-semibold text-text-primary mb-1">Admin Account</h2>
					<p class="text-text-muted text-sm mb-6">Create the first admin user for your instance.</p>

					<div class="space-y-4">
						<div>
							<label for="admin-username" class="block text-sm font-medium text-text-secondary mb-1">Username *</label>
							<input
								id="admin-username"
								type="text"
								class="input w-full"
								placeholder="admin"
								bind:value={adminUsername}
							/>
						</div>
						<div>
							<label for="admin-email" class="block text-sm font-medium text-text-secondary mb-1">Email *</label>
							<input
								id="admin-email"
								type="email"
								class="input w-full"
								placeholder="admin@example.com"
								bind:value={adminEmail}
							/>
						</div>
						<div>
							<label for="admin-password" class="block text-sm font-medium text-text-secondary mb-1">Password *</label>
							<input
								id="admin-password"
								type="password"
								class="input w-full"
								placeholder="Minimum 8 characters"
								bind:value={adminPassword}
							/>
						</div>
						<div>
							<label for="admin-password-confirm" class="block text-sm font-medium text-text-secondary mb-1">Confirm Password *</label>
							<input
								id="admin-password-confirm"
								type="password"
								class="input w-full"
								placeholder="Re-enter password"
								bind:value={adminPasswordConfirm}
							/>
						</div>
					</div>
				{/if}

				<!-- Step 3: Configuration -->
				{#if step === 3}
					<h2 class="text-xl font-semibold text-text-primary mb-1">Configuration</h2>
					<p class="text-text-muted text-sm mb-6">Configure federation and registration settings.</p>

					<div class="space-y-6">
						<div>
							<label class="block text-sm font-medium text-text-secondary mb-2">Federation Mode</label>
							<div class="space-y-2">
								<label class="flex items-start gap-3 p-3 rounded-lg bg-bg-tertiary cursor-pointer hover:bg-bg-modifier transition-colors">
									<input type="radio" bind:group={federationMode} value="closed" class="mt-0.5" />
									<div>
										<p class="text-text-primary font-medium text-sm">Closed</p>
										<p class="text-text-muted text-xs">No federation. Standalone instance.</p>
									</div>
								</label>
								<label class="flex items-start gap-3 p-3 rounded-lg bg-bg-tertiary cursor-pointer hover:bg-bg-modifier transition-colors">
									<input type="radio" bind:group={federationMode} value="allowlist" class="mt-0.5" />
									<div>
										<p class="text-text-primary font-medium text-sm">Allowlist</p>
										<p class="text-text-muted text-xs">Only federate with approved instances.</p>
									</div>
								</label>
								<label class="flex items-start gap-3 p-3 rounded-lg bg-bg-tertiary cursor-pointer hover:bg-bg-modifier transition-colors">
									<input type="radio" bind:group={federationMode} value="open" class="mt-0.5" />
									<div>
										<p class="text-text-primary font-medium text-sm">Open</p>
										<p class="text-text-muted text-xs">Federate with any compatible instance.</p>
									</div>
								</label>
							</div>
						</div>

						<div>
							<label class="block text-sm font-medium text-text-secondary mb-2">Registration Mode</label>
							<div class="space-y-2">
								<label class="flex items-start gap-3 p-3 rounded-lg bg-bg-tertiary cursor-pointer hover:bg-bg-modifier transition-colors">
									<input type="radio" bind:group={registrationMode} value="open" class="mt-0.5" />
									<div>
										<p class="text-text-primary font-medium text-sm">Open</p>
										<p class="text-text-muted text-xs">Anyone can register an account.</p>
									</div>
								</label>
								<label class="flex items-start gap-3 p-3 rounded-lg bg-bg-tertiary cursor-pointer hover:bg-bg-modifier transition-colors">
									<input type="radio" bind:group={registrationMode} value="invite_only" class="mt-0.5" />
									<div>
										<p class="text-text-primary font-medium text-sm">Invite Only</p>
										<p class="text-text-muted text-xs">Require an invitation token to register.</p>
									</div>
								</label>
								<label class="flex items-start gap-3 p-3 rounded-lg bg-bg-tertiary cursor-pointer hover:bg-bg-modifier transition-colors">
									<input type="radio" bind:group={registrationMode} value="closed" class="mt-0.5" />
									<div>
										<p class="text-text-primary font-medium text-sm">Closed</p>
										<p class="text-text-muted text-xs">No new registrations (admin creates accounts).</p>
									</div>
								</label>
							</div>
						</div>
					</div>
				{/if}

				<!-- Step 4: Review -->
				{#if step === 4}
					<h2 class="text-xl font-semibold text-text-primary mb-1">Review & Finish</h2>
					<p class="text-text-muted text-sm mb-6">Review your configuration before completing setup.</p>

					<div class="space-y-4">
						<div class="bg-bg-tertiary rounded-lg p-4">
							<h3 class="text-sm font-semibold text-text-secondary mb-2">Instance</h3>
							<div class="grid grid-cols-2 gap-2 text-sm">
								<span class="text-text-muted">Name:</span>
								<span class="text-text-primary">{instanceName}</span>
								{#if description}
									<span class="text-text-muted">Description:</span>
									<span class="text-text-primary">{description}</span>
								{/if}
								{#if domain}
									<span class="text-text-muted">Domain:</span>
									<span class="text-text-primary">{domain}</span>
								{/if}
							</div>
						</div>

						<div class="bg-bg-tertiary rounded-lg p-4">
							<h3 class="text-sm font-semibold text-text-secondary mb-2">Admin Account</h3>
							<div class="grid grid-cols-2 gap-2 text-sm">
								<span class="text-text-muted">Username:</span>
								<span class="text-text-primary">{adminUsername}</span>
								<span class="text-text-muted">Email:</span>
								<span class="text-text-primary">{adminEmail}</span>
							</div>
						</div>

						<div class="bg-bg-tertiary rounded-lg p-4">
							<h3 class="text-sm font-semibold text-text-secondary mb-2">Settings</h3>
							<div class="grid grid-cols-2 gap-2 text-sm">
								<span class="text-text-muted">Federation:</span>
								<span class="text-text-primary capitalize">{federationMode}</span>
								<span class="text-text-muted">Registration:</span>
								<span class="text-text-primary capitalize">{registrationMode.replace('_', ' ')}</span>
							</div>
						</div>
					</div>
				{/if}

				<!-- Navigation -->
				<div class="flex justify-between mt-8">
					{#if step > 1}
						<button class="btn-secondary px-6 py-2" onclick={prevStep} disabled={submitting}>
							Back
						</button>
					{:else}
						<div></div>
					{/if}

					{#if step < totalSteps}
						<button class="btn-primary px-6 py-2" onclick={nextStep}>
							Continue
						</button>
					{:else}
						<button
							class="btn-primary px-8 py-2"
							onclick={completeSetup}
							disabled={submitting}
						>
							{#if submitting}
								<span class="inline-block animate-spin w-4 h-4 border-2 border-white border-t-transparent rounded-full mr-2"></span>
								Setting up...
							{:else}
								Complete Setup
							{/if}
						</button>
					{/if}
				</div>
			</div>

			<!-- Step indicator -->
			<p class="text-center text-text-muted text-sm mt-4">
				Step {step} of {totalSteps}
			</p>
		{/if}
	</div>
</div>
