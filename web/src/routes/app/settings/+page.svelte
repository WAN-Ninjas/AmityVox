<script lang="ts">
	import { onMount } from 'svelte';
	import { currentUser, logout } from '$lib/stores/auth';
	import { api } from '$lib/api/client';
	import { goto } from '$app/navigation';
	import Avatar from '$components/common/Avatar.svelte';
	import type { Session } from '$lib/types';

	type Tab = 'account' | 'security' | 'notifications' | 'privacy' | 'appearance';
	let currentTab = $state<Tab>('account');

	// --- Account tab state ---
	let displayName = $state('');
	let bio = $state('');
	let statusText = $state('');
	let saving = $state(false);
	let error = $state('');
	let success = $state('');
	let avatarFile = $state<File | null>(null);
	let avatarPreview = $state<string | null>(null);

	// --- Security tab state ---
	let currentPassword = $state('');
	let newPassword = $state('');
	let confirmPassword = $state('');
	let changingPassword = $state(false);
	let passwordError = $state('');
	let passwordSuccess = $state('');

	// --- 2FA state ---
	let totpSecret = $state('');
	let totpQrUrl = $state('');
	let totpCode = $state('');
	let backupCodes = $state<string[]>([]);
	let enablingTotp = $state(false);
	let verifyingTotp = $state(false);
	let totpError = $state('');
	let totpStep = $state<'idle' | 'setup' | 'verify' | 'done'>('idle');

	// --- Sessions state ---
	let sessions = $state<Session[]>([]);
	let loadingSessions = $state(false);
	let revokingSession = $state<string | null>(null);

	// --- Notifications tab state ---
	let desktopNotifications = $state(true);
	let notificationSounds = $state(true);
	let notifLoading = $state(false);
	let notifSuccess = $state('');

	// --- Privacy tab state ---
	let dmPrivacy = $state<'everyone' | 'friends' | 'nobody'>('everyone');
	let friendRequestPrivacy = $state<'everyone' | 'mutual_guilds' | 'nobody'>('everyone');
	let privacyLoading = $state(false);
	let privacySuccess = $state('');

	// --- Appearance tab state ---
	let theme = $state<'dark' | 'light'>('dark');
	let fontSize = $state(16);
	let compactMode = $state(false);

	onMount(() => {
		if ($currentUser) {
			displayName = $currentUser.display_name ?? '';
			bio = $currentUser.bio ?? '';
			statusText = $currentUser.status_text ?? '';
		}

		theme = (localStorage.getItem('av-theme') as 'dark' | 'light') ?? 'dark';
		fontSize = parseInt(localStorage.getItem('av-font-size') ?? '16', 10);
		compactMode = localStorage.getItem('av-compact') === 'true';
	});

	// --- Account actions ---

	async function handleSaveProfile() {
		saving = true;
		error = '';
		success = '';

		try {
			// Upload avatar if selected.
			let avatarId: string | undefined;
			if (avatarFile) {
				const uploaded = await api.uploadFile(avatarFile);
				avatarId = uploaded.id;
			}

			const payload: Record<string, unknown> = {
				display_name: displayName || undefined,
				bio: bio || undefined,
				status_text: statusText || undefined
			};
			if (avatarId) payload.avatar_id = avatarId;

			const updated = await api.updateMe(payload as any);
			currentUser.set(updated);
			avatarFile = null;
			avatarPreview = null;
			success = 'Profile updated!';
			setTimeout(() => (success = ''), 3000);
		} catch (err: any) {
			error = err.message || 'Failed to save';
		} finally {
			saving = false;
		}
	}

	function handleAvatarSelect(e: Event) {
		const target = e.target as HTMLInputElement;
		const file = target.files?.[0];
		if (!file) return;
		if (!file.type.startsWith('image/')) {
			error = 'Please select an image file.';
			return;
		}
		avatarFile = file;
		avatarPreview = URL.createObjectURL(file);
	}

	// --- Security actions ---

	async function handleChangePassword() {
		if (newPassword !== confirmPassword) {
			passwordError = 'Passwords do not match.';
			return;
		}
		if (newPassword.length < 8) {
			passwordError = 'Password must be at least 8 characters.';
			return;
		}

		changingPassword = true;
		passwordError = '';
		passwordSuccess = '';

		try {
			await api.changePassword(currentPassword, newPassword);
			currentPassword = '';
			newPassword = '';
			confirmPassword = '';
			passwordSuccess = 'Password changed successfully!';
			setTimeout(() => (passwordSuccess = ''), 3000);
		} catch (err: any) {
			passwordError = err.message || 'Failed to change password';
		} finally {
			changingPassword = false;
		}
	}

	async function handleEnableTotp() {
		enablingTotp = true;
		totpError = '';
		try {
			const result = await api.enableTOTP();
			totpSecret = result.secret;
			totpQrUrl = result.qr_url;
			totpStep = 'setup';
		} catch (err: any) {
			totpError = err.message || 'Failed to enable 2FA';
		} finally {
			enablingTotp = false;
		}
	}

	async function handleVerifyTotp() {
		if (totpCode.length !== 6) {
			totpError = 'Enter a 6-digit code.';
			return;
		}
		verifyingTotp = true;
		totpError = '';
		try {
			const result = await api.verifyTOTP(totpCode);
			backupCodes = result.backup_codes;
			totpStep = 'done';
			totpCode = '';
		} catch (err: any) {
			totpError = err.message || 'Invalid code';
		} finally {
			verifyingTotp = false;
		}
	}

	function resetTotpFlow() {
		totpStep = 'idle';
		totpSecret = '';
		totpQrUrl = '';
		totpCode = '';
		backupCodes = [];
		totpError = '';
	}

	// --- Sessions ---

	async function loadSessions() {
		loadingSessions = true;
		try {
			sessions = await api.getSessions();
		} catch {
			sessions = [];
		} finally {
			loadingSessions = false;
		}
	}

	async function revokeSession(sessionId: string) {
		revokingSession = sessionId;
		try {
			await api.deleteSession(sessionId);
			sessions = sessions.filter((s) => s.id !== sessionId);
		} catch (err: any) {
			alert(err.message || 'Failed to revoke session');
		} finally {
			revokingSession = null;
		}
	}

	// Load sessions when switching to security tab.
	$effect(() => {
		if (currentTab === 'security') {
			loadSessions();
		}
	});

	// Load settings when switching to notifications/privacy tabs.
	$effect(() => {
		if (currentTab === 'notifications' || currentTab === 'privacy') {
			loadUserSettings();
		}
	});

	async function loadUserSettings() {
		try {
			const settings = await api.getUserSettings();
			desktopNotifications = settings.desktop_notifications ?? true;
			notificationSounds = settings.notification_sounds ?? true;
			dmPrivacy = settings.dm_privacy ?? 'everyone';
			friendRequestPrivacy = settings.friend_request_privacy ?? 'everyone';
		} catch {
			// Use defaults if settings don't exist yet.
		}
	}

	async function saveNotifications() {
		notifLoading = true;
		notifSuccess = '';
		try {
			await api.updateUserSettings({
				desktop_notifications: desktopNotifications,
				notification_sounds: notificationSounds
			});
			notifSuccess = 'Notification preferences saved!';
			setTimeout(() => (notifSuccess = ''), 3000);

			// Request browser notification permission if enabling desktop notifications.
			if (desktopNotifications && 'Notification' in window && Notification.permission === 'default') {
				await Notification.requestPermission();
			}
		} catch (err: any) {
			error = err.message || 'Failed to save';
		} finally {
			notifLoading = false;
		}
	}

	async function savePrivacy() {
		privacyLoading = true;
		privacySuccess = '';
		try {
			await api.updateUserSettings({
				dm_privacy: dmPrivacy,
				friend_request_privacy: friendRequestPrivacy
			});
			privacySuccess = 'Privacy settings saved!';
			setTimeout(() => (privacySuccess = ''), 3000);
		} catch (err: any) {
			error = err.message || 'Failed to save';
		} finally {
			privacyLoading = false;
		}
	}

	// --- Appearance ---

	function saveAppearance() {
		localStorage.setItem('av-theme', theme);
		localStorage.setItem('av-font-size', String(fontSize));
		localStorage.setItem('av-compact', String(compactMode));
		document.documentElement.style.fontSize = `${fontSize}px`;
		success = 'Appearance settings saved!';
		setTimeout(() => (success = ''), 3000);
	}

	async function handleLogout() {
		await logout();
		goto('/login');
	}

	const tabs: { id: Tab; label: string }[] = [
		{ id: 'account', label: 'My Account' },
		{ id: 'security', label: 'Security' },
		{ id: 'notifications', label: 'Notifications' },
		{ id: 'privacy', label: 'Privacy' },
		{ id: 'appearance', label: 'Appearance' }
	];

	function themeButtonClass(t: 'dark' | 'light'): string {
		const base = 'rounded-lg border-2 px-4 py-2 text-sm transition-colors';
		if (theme === t) return `${base} border-brand-500 bg-brand-500/10 text-text-primary`;
		return `${base} border-bg-modifier text-text-muted`;
	}

	function formatSessionTime(iso: string): string {
		return new Date(iso).toLocaleString();
	}

	function parseUserAgent(ua: string): string {
		if (ua.includes('Firefox')) return 'Firefox';
		if (ua.includes('Chrome')) return 'Chrome';
		if (ua.includes('Safari')) return 'Safari';
		if (ua.includes('Edge')) return 'Edge';
		return 'Unknown Browser';
	}
</script>

<svelte:head>
	<title>Settings — AmityVox</title>
</svelte:head>

<div class="flex h-full">
	<!-- Settings sidebar -->
	<nav class="w-52 shrink-0 overflow-y-auto bg-bg-secondary p-4">
		<h3 class="mb-2 text-xs font-bold uppercase tracking-wide text-text-muted">User Settings</h3>
		<ul class="space-y-0.5">
			{#each tabs as tab (tab.id)}
				<li>
					<button
						class="w-full rounded px-2 py-1.5 text-left text-sm transition-colors {currentTab === tab.id ? 'bg-bg-modifier text-text-primary' : 'text-text-muted hover:bg-bg-modifier hover:text-text-secondary'}"
						onclick={() => (currentTab = tab.id)}
					>
						{tab.label}
					</button>
				</li>
			{/each}
		</ul>

		<div class="my-2 border-t border-bg-modifier"></div>
		<button
			class="w-full rounded px-2 py-1.5 text-left text-sm text-text-muted hover:bg-bg-modifier hover:text-text-secondary"
			onclick={() => goto('/app')}
		>
			Back
		</button>

		<div class="my-2 border-t border-bg-modifier"></div>
		<button
			class="w-full rounded px-2 py-1.5 text-left text-sm text-red-400 hover:bg-bg-modifier"
			onclick={handleLogout}
		>
			Log Out
		</button>
	</nav>

	<!-- Settings content -->
	<div class="flex-1 overflow-y-auto bg-bg-tertiary p-8">
		<div class="max-w-xl">
			{#if error}
				<div class="mb-4 rounded bg-red-500/10 px-3 py-2 text-sm text-red-400">{error}</div>
			{/if}
			{#if success}
				<div class="mb-4 rounded bg-green-500/10 px-3 py-2 text-sm text-green-400">{success}</div>
			{/if}

			<!-- ==================== MY ACCOUNT ==================== -->
			{#if currentTab === 'account'}
				<h1 class="mb-6 text-xl font-bold text-text-primary">My Account</h1>

				{#if $currentUser}
					<!-- Profile card -->
					<div class="mb-8 rounded-lg bg-bg-secondary p-6">
						<div class="flex items-center gap-4">
							<div class="relative">
								<Avatar
									name={$currentUser.display_name ?? $currentUser.username}
									src={avatarPreview ?? ($currentUser.avatar_id ? `/api/v1/files/${$currentUser.avatar_id}` : null)}
									size="lg"
									status={$currentUser.status_presence}
								/>
								<label class="absolute inset-0 flex cursor-pointer items-center justify-center rounded-full bg-black/50 opacity-0 transition-opacity hover:opacity-100">
									<svg class="h-6 w-6 text-white" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
										<path d="M3 9a2 2 0 012-2h.93a2 2 0 001.664-.89l.812-1.22A2 2 0 0110.07 4h3.86a2 2 0 011.664.89l.812 1.22A2 2 0 0018.07 7H19a2 2 0 012 2v9a2 2 0 01-2 2H5a2 2 0 01-2-2V9z" />
										<circle cx="12" cy="13" r="3" />
									</svg>
									<input type="file" accept="image/*" class="hidden" onchange={handleAvatarSelect} />
								</label>
							</div>
							<div>
								<h2 class="text-lg font-semibold text-text-primary">
									{$currentUser.display_name ?? $currentUser.username}
								</h2>
								<p class="text-sm text-text-muted">{$currentUser.username}</p>
								{#if $currentUser.email}
									<p class="text-sm text-text-muted">{$currentUser.email}</p>
								{/if}
							</div>
						</div>
					</div>

					<div class="mb-4">
						<label for="displayName" class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">
							Display Name
						</label>
						<input id="displayName" type="text" bind:value={displayName} class="input w-full" maxlength="32" />
					</div>

					<div class="mb-4">
						<label for="statusText" class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">
							Custom Status
						</label>
						<input id="statusText" type="text" bind:value={statusText} class="input w-full" maxlength="128" />
					</div>

					<div class="mb-6">
						<label for="bio" class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">
							About Me
						</label>
						<textarea id="bio" bind:value={bio} class="input w-full" rows="3" maxlength="190"></textarea>
					</div>

					<button class="btn-primary" onclick={handleSaveProfile} disabled={saving}>
						{saving ? 'Saving...' : 'Save Changes'}
					</button>
				{/if}

			<!-- ==================== SECURITY ==================== -->
			{:else if currentTab === 'security'}
				<h1 class="mb-6 text-xl font-bold text-text-primary">Security</h1>

				<!-- Password Change -->
				<div class="mb-8 rounded-lg bg-bg-secondary p-6">
					<h3 class="mb-4 text-sm font-semibold text-text-primary">Change Password</h3>

					{#if passwordError}
						<div class="mb-3 rounded bg-red-500/10 px-3 py-2 text-sm text-red-400">{passwordError}</div>
					{/if}
					{#if passwordSuccess}
						<div class="mb-3 rounded bg-green-500/10 px-3 py-2 text-sm text-green-400">{passwordSuccess}</div>
					{/if}

					<div class="mb-3">
						<label for="curPw" class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">
							Current Password
						</label>
						<input id="curPw" type="password" bind:value={currentPassword} class="input w-full" />
					</div>
					<div class="mb-3">
						<label for="newPw" class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">
							New Password
						</label>
						<input id="newPw" type="password" bind:value={newPassword} class="input w-full" />
					</div>
					<div class="mb-4">
						<label for="confirmPw" class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">
							Confirm New Password
						</label>
						<input id="confirmPw" type="password" bind:value={confirmPassword} class="input w-full" />
					</div>
					<button
						class="btn-primary"
						onclick={handleChangePassword}
						disabled={changingPassword || !currentPassword || !newPassword || !confirmPassword}
					>
						{changingPassword ? 'Changing...' : 'Change Password'}
					</button>
				</div>

				<!-- Two-Factor Authentication -->
				<div class="mb-8 rounded-lg bg-bg-secondary p-6">
					<h3 class="mb-2 text-sm font-semibold text-text-primary">Two-Factor Authentication</h3>
					<p class="mb-4 text-xs text-text-muted">
						Add an extra layer of security with a TOTP authenticator app.
					</p>

					{#if totpError}
						<div class="mb-3 rounded bg-red-500/10 px-3 py-2 text-sm text-red-400">{totpError}</div>
					{/if}

					{#if totpStep === 'idle'}
						<button class="btn-primary" onclick={handleEnableTotp} disabled={enablingTotp}>
							{enablingTotp ? 'Setting up...' : 'Enable 2FA'}
						</button>
					{:else if totpStep === 'setup'}
						<div class="space-y-4">
							<p class="text-sm text-text-secondary">
								Scan this QR code with your authenticator app (Google Authenticator, Authy, etc.):
							</p>
							{#if totpQrUrl}
								<div class="flex justify-center rounded bg-white p-4">
									<img src={totpQrUrl} alt="2FA QR Code" class="h-48 w-48" />
								</div>
							{/if}
							<div class="rounded bg-bg-primary p-3">
								<p class="mb-1 text-xs font-bold uppercase tracking-wide text-text-muted">Manual entry code:</p>
								<code class="break-all text-sm text-text-primary">{totpSecret}</code>
							</div>
							<div>
								<label for="totpCode" class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">
									Enter 6-digit verification code
								</label>
								<input
									id="totpCode"
									type="text"
									inputmode="numeric"
									maxlength="6"
									bind:value={totpCode}
									class="input w-40"
									placeholder="000000"
									onkeydown={(e) => e.key === 'Enter' && handleVerifyTotp()}
								/>
							</div>
							<div class="flex gap-2">
								<button class="btn-primary" onclick={handleVerifyTotp} disabled={verifyingTotp || totpCode.length !== 6}>
									{verifyingTotp ? 'Verifying...' : 'Verify & Enable'}
								</button>
								<button class="btn-secondary" onclick={resetTotpFlow}>Cancel</button>
							</div>
						</div>
					{:else if totpStep === 'done'}
						<div class="space-y-4">
							<div class="rounded bg-green-500/10 px-3 py-2 text-sm text-green-400">
								Two-factor authentication has been enabled!
							</div>
							{#if backupCodes.length > 0}
								<div class="rounded bg-bg-primary p-4">
									<p class="mb-2 text-sm font-semibold text-text-primary">Backup Codes</p>
									<p class="mb-3 text-xs text-text-muted">
										Save these codes in a safe place. Each can be used once if you lose access to your authenticator.
									</p>
									<div class="grid grid-cols-2 gap-2">
										{#each backupCodes as code}
											<code class="rounded bg-bg-secondary px-2 py-1 text-center text-sm text-text-primary">{code}</code>
										{/each}
									</div>
								</div>
							{/if}
							<button class="btn-secondary" onclick={resetTotpFlow}>Done</button>
						</div>
					{/if}
				</div>

				<!-- Active Sessions -->
				<div class="rounded-lg bg-bg-secondary p-6">
					<h3 class="mb-4 text-sm font-semibold text-text-primary">Active Sessions</h3>

					{#if loadingSessions}
						<div class="flex items-center gap-2 py-4">
							<div class="h-4 w-4 animate-spin rounded-full border-2 border-brand-500 border-t-transparent"></div>
							<span class="text-sm text-text-muted">Loading sessions...</span>
						</div>
					{:else if sessions.length === 0}
						<p class="text-sm text-text-muted">No sessions found.</p>
					{:else}
						<div class="space-y-3">
							{#each sessions as session (session.id)}
								<div class="flex items-center justify-between rounded bg-bg-primary p-3">
									<div>
										<div class="flex items-center gap-2">
											<span class="text-sm font-medium text-text-primary">
												{parseUserAgent(session.user_agent)}
											</span>
											{#if session.current}
												<span class="rounded bg-green-500/10 px-1.5 py-0.5 text-2xs font-bold text-green-400">Current</span>
											{/if}
										</div>
										<p class="text-xs text-text-muted">
											{session.ip_address} &middot; Last active {formatSessionTime(session.last_active_at)}
										</p>
									</div>
									{#if !session.current}
										<button
											class="text-xs text-red-400 hover:text-red-300"
											onclick={() => revokeSession(session.id)}
											disabled={revokingSession === session.id}
										>
											{revokingSession === session.id ? 'Revoking...' : 'Revoke'}
										</button>
									{/if}
								</div>
							{/each}
						</div>
					{/if}
				</div>

			<!-- ==================== NOTIFICATIONS ==================== -->
			{:else if currentTab === 'notifications'}
				<h1 class="mb-6 text-xl font-bold text-text-primary">Notifications</h1>

				{#if notifSuccess}
					<div class="mb-4 rounded bg-green-500/10 px-3 py-2 text-sm text-green-400">{notifSuccess}</div>
				{/if}

				<div class="space-y-6">
					<div class="rounded-lg bg-bg-secondary p-4">
						<h3 class="mb-1 text-sm font-semibold text-text-primary">Desktop Notifications</h3>
						<p class="mb-3 text-xs text-text-muted">Show browser notifications for new messages and mentions.</p>
						<label class="flex items-center gap-2">
							<input type="checkbox" bind:checked={desktopNotifications} class="accent-brand-500" />
							<span class="text-sm text-text-secondary">Enable desktop notifications</span>
						</label>
						{#if 'Notification' in globalThis && Notification.permission === 'denied'}
							<p class="mt-2 text-xs text-red-400">
								Browser notifications are blocked. Please allow notifications in your browser settings.
							</p>
						{/if}
					</div>

					<div class="rounded-lg bg-bg-secondary p-4">
						<h3 class="mb-1 text-sm font-semibold text-text-primary">Notification Sounds</h3>
						<p class="mb-3 text-xs text-text-muted">Play a sound when you receive a notification.</p>
						<label class="flex items-center gap-2">
							<input type="checkbox" bind:checked={notificationSounds} class="accent-brand-500" />
							<span class="text-sm text-text-secondary">Enable notification sounds</span>
						</label>
					</div>

					<button class="btn-primary" onclick={saveNotifications} disabled={notifLoading}>
						{notifLoading ? 'Saving...' : 'Save Notification Preferences'}
					</button>
				</div>

			<!-- ==================== PRIVACY ==================== -->
			{:else if currentTab === 'privacy'}
				<h1 class="mb-6 text-xl font-bold text-text-primary">Privacy</h1>

				{#if privacySuccess}
					<div class="mb-4 rounded bg-green-500/10 px-3 py-2 text-sm text-green-400">{privacySuccess}</div>
				{/if}

				<div class="space-y-6">
					<div class="rounded-lg bg-bg-secondary p-4">
						<h3 class="mb-1 text-sm font-semibold text-text-primary">Direct Messages</h3>
						<p class="mb-3 text-xs text-text-muted">Control who can send you direct messages.</p>
						<div class="space-y-2">
							<label class="flex items-center gap-2">
								<input type="radio" name="dmPrivacy" value="everyone" bind:group={dmPrivacy} class="accent-brand-500" />
								<span class="text-sm text-text-secondary">Everyone</span>
							</label>
							<label class="flex items-center gap-2">
								<input type="radio" name="dmPrivacy" value="friends" bind:group={dmPrivacy} class="accent-brand-500" />
								<span class="text-sm text-text-secondary">Friends only</span>
							</label>
							<label class="flex items-center gap-2">
								<input type="radio" name="dmPrivacy" value="nobody" bind:group={dmPrivacy} class="accent-brand-500" />
								<span class="text-sm text-text-secondary">Nobody</span>
							</label>
						</div>
					</div>

					<div class="rounded-lg bg-bg-secondary p-4">
						<h3 class="mb-1 text-sm font-semibold text-text-primary">Friend Requests</h3>
						<p class="mb-3 text-xs text-text-muted">Control who can send you friend requests.</p>
						<div class="space-y-2">
							<label class="flex items-center gap-2">
								<input type="radio" name="friendPrivacy" value="everyone" bind:group={friendRequestPrivacy} class="accent-brand-500" />
								<span class="text-sm text-text-secondary">Everyone</span>
							</label>
							<label class="flex items-center gap-2">
								<input type="radio" name="friendPrivacy" value="mutual_guilds" bind:group={friendRequestPrivacy} class="accent-brand-500" />
								<span class="text-sm text-text-secondary">People in mutual guilds</span>
							</label>
							<label class="flex items-center gap-2">
								<input type="radio" name="friendPrivacy" value="nobody" bind:group={friendRequestPrivacy} class="accent-brand-500" />
								<span class="text-sm text-text-secondary">Nobody</span>
							</label>
						</div>
					</div>

					<button class="btn-primary" onclick={savePrivacy} disabled={privacyLoading}>
						{privacyLoading ? 'Saving...' : 'Save Privacy Settings'}
					</button>
				</div>

			<!-- ==================== APPEARANCE ==================== -->
			{:else if currentTab === 'appearance'}
				<h1 class="mb-6 text-xl font-bold text-text-primary">Appearance</h1>

				<div class="space-y-6">
					<div class="rounded-lg bg-bg-secondary p-4">
						<h3 class="mb-1 text-sm font-semibold text-text-primary">Theme</h3>
						<p class="mb-3 text-xs text-text-muted">Choose your interface theme.</p>
						<div class="flex gap-3">
							<button class={themeButtonClass('dark')} onclick={() => (theme = 'dark')}>
								Dark
							</button>
							<button class={themeButtonClass('light')} onclick={() => (theme = 'light')}>
								Light
							</button>
						</div>
					</div>

					<div class="rounded-lg bg-bg-secondary p-4">
						<h3 class="mb-1 text-sm font-semibold text-text-primary">Font Size</h3>
						<p class="mb-3 text-xs text-text-muted">Adjust the base font size ({fontSize}px).</p>
						<input
							type="range"
							min="12"
							max="20"
							bind:value={fontSize}
							class="w-full accent-brand-500"
						/>
						<div class="mt-1 flex justify-between text-xs text-text-muted">
							<span>12px</span>
							<span>16px</span>
							<span>20px</span>
						</div>
					</div>

					<div class="rounded-lg bg-bg-secondary p-4">
						<h3 class="mb-1 text-sm font-semibold text-text-primary">Compact Mode</h3>
						<label class="flex items-center gap-2">
							<input type="checkbox" bind:checked={compactMode} class="accent-brand-500" />
							<span class="text-sm text-text-secondary">Use compact message layout</span>
						</label>
					</div>

					<button class="btn-primary" onclick={saveAppearance}>Save Appearance</button>
				</div>
			{/if}
		</div>
	</div>
</div>
