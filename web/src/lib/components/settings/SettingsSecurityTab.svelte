<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api/client';
	import { addToast } from '$lib/stores/toast';
	import { createAsyncOp } from '$lib/utils/asyncOp';
	import { getErrorMessage } from '$lib/utils/apiError';
	import type { Session } from '$lib/types';

	let currentPassword = $state('');
	let newPassword = $state('');
	let confirmPassword = $state('');
	let changingPassword = $state(false);
	let passwordError = $state('');
	let passwordSuccess = $state('');

	let totpSecret = $state('');
	let totpQrUrl = $state('');
	let totpCode = $state('');
	let backupCodes = $state<string[]>([]);
	let enablingTotp = $state(false);
	let verifyingTotp = $state(false);
	let totpError = $state('');
	let totpStep = $state<'idle' | 'setup' | 'verify' | 'done'>('idle');

	let sessions = $state<Session[]>([]);
	let revokingSession = $state<string | null>(null);
	let sessionsOp = $state(createAsyncOp());

	onMount(() => {
		loadSessions();
	});

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
		} catch (err: unknown) {
			passwordError = getErrorMessage(err, 'Failed to change password');
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
		} catch (err: unknown) {
			totpError = getErrorMessage(err, 'Failed to enable 2FA');
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
		} catch (err: unknown) {
			totpError = getErrorMessage(err, 'Invalid code');
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

	async function loadSessions() {
		const result = await sessionsOp.run(() => api.getSessions());
		if (result) {
			sessions = result;
		} else {
			sessions = [];
		}
	}

	async function revokeSession(sessionId: string) {
		revokingSession = sessionId;
		try {
			await api.deleteSession(sessionId);
			sessions = sessions.filter((session) => session.id !== sessionId);
		} catch (err: unknown) {
			addToast(getErrorMessage(err, 'Failed to revoke session'), 'error');
		} finally {
			revokingSession = null;
		}
	}

	function formatSessionTime(iso: string): string {
		return new Date(iso).toLocaleString();
	}

	function parseUserAgent(userAgent: string): string {
		if (userAgent.includes('Firefox')) return 'Firefox';
		if (userAgent.includes('Chrome')) return 'Chrome';
		if (userAgent.includes('Safari')) return 'Safari';
		if (userAgent.includes('Edge')) return 'Edge';
		return 'Unknown Browser';
	}
</script>

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

	{#if sessionsOp.loading}
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
