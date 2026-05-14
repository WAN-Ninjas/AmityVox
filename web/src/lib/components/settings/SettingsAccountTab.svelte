<script lang="ts">
	import { onMount } from 'svelte';
	import { currentUser } from '$lib/stores/auth';
	import { api } from '$lib/api/client';
	import { avatarUrl, fileUrl } from '$lib/utils/avatar';
	import Avatar from '$components/common/Avatar.svelte';
	import ProfileLinkEditor from '$components/common/ProfileLinkEditor.svelte';
	import ImageCropper from '$components/common/ImageCropper.svelte';
	import Modal from '$components/common/Modal.svelte';
	import type { User } from '$lib/types';

	interface Props {
		importedProfile?: User | null;
	}

	let { importedProfile = null }: Props = $props();

	let displayName = $state('');
	let bio = $state('');
	let statusText = $state('');
	let saving = $state(false);
	let error = $state('');
	let success = $state('');
	let avatarFile = $state<File | null>(null);
	let avatarPreview = $state<string | null>(null);
	let accentColor = $state('#5865f2');
	let bannerFile = $state<File | null>(null);
	let bannerPreview = $state<string | null>(null);
	let bannerRemoved = $state(false);
	let cropperFile = $state<File | null>(null);
	let cropperTarget = $state<'avatar' | 'banner'>('avatar');
	let appliedImportedProfile: User | null = null;

	onMount(() => {
		if ($currentUser) {
			applyUserProfile($currentUser);
		}
	});

	$effect(() => {
		if (importedProfile && importedProfile !== appliedImportedProfile) {
			applyUserProfile(importedProfile);
			appliedImportedProfile = importedProfile;
		}
	});

	function applyUserProfile(user: User) {
		displayName = user.display_name ?? '';
		bio = user.bio ?? '';
		statusText = user.status_text ?? '';
		accentColor = user.accent_color ?? '#5865f2';
	}

	function handleBannerSelect(e: Event) {
		const target = e.target as HTMLInputElement;
		const file = target.files?.[0];
		if (!file) return;
		cropperTarget = 'banner';
		cropperFile = file;
		target.value = '';
	}

	async function handleSaveProfile() {
		saving = true;
		error = '';
		success = '';

		try {
			let avatarId: string | undefined;
			if (avatarFile) {
				const uploaded = await api.uploadFile(avatarFile);
				avatarId = uploaded.id;
			}

			let bannerId: string | undefined;
			if (bannerFile) {
				const uploaded = await api.uploadFile(bannerFile);
				bannerId = uploaded.id;
			}

			const payload: Record<string, unknown> = {
				display_name: displayName || undefined,
				bio: bio || undefined,
				status_text: statusText || undefined,
				accent_color: accentColor || undefined
			};
			if (avatarId) payload.avatar_id = avatarId;
			if (bannerId) payload.banner_id = bannerId;
			if (bannerRemoved && !bannerId) payload.banner_id = null;

			const updated = await api.updateMe(payload as any);
			currentUser.set(updated);
			avatarFile = null;
			avatarPreview = null;
			bannerFile = null;
			bannerPreview = null;
			bannerRemoved = false;
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
		cropperTarget = 'avatar';
		cropperFile = file;
		target.value = '';
	}

	function handleCropComplete(blob: Blob) {
		const file = new File([blob], `${cropperTarget}.png`, { type: 'image/png' });
		if (cropperTarget === 'avatar') {
			avatarFile = file;
			avatarPreview = URL.createObjectURL(blob);
		} else {
			bannerFile = file;
			bannerPreview = URL.createObjectURL(blob);
		}
		cropperFile = null;
	}
</script>

{#if error}
	<div class="mb-4 rounded bg-red-500/10 px-3 py-2 text-sm text-red-400">{error}</div>
{/if}
{#if success}
	<div class="mb-4 rounded bg-green-500/10 px-3 py-2 text-sm text-green-400">{success}</div>
{/if}

<h1 class="mb-6 text-xl font-bold text-text-primary">My Account</h1>

{#if $currentUser}
	<!-- Profile card with banner -->
	<div class="mb-8 overflow-hidden rounded-lg bg-bg-secondary">
		<!-- Banner area -->
		<div class="group relative h-28">
			{#if bannerPreview}
				<img class="h-full w-full object-cover" src={bannerPreview} alt="Banner preview" />
			{:else if $currentUser.banner_id}
				<img class="h-full w-full object-cover" src={fileUrl($currentUser.banner_id)} alt="Profile banner" />
			{:else}
				<div class="h-full w-full" style="background-color: {accentColor}"></div>
			{/if}
			<label class="absolute inset-0 flex cursor-pointer items-center justify-center bg-black/40 opacity-0 transition-opacity group-hover:opacity-100">
				<div class="flex flex-col items-center gap-1 text-white">
					<svg class="h-6 w-6" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
						<path d="M3 9a2 2 0 012-2h.93a2 2 0 001.664-.89l.812-1.22A2 2 0 0110.07 4h3.86a2 2 0 011.664.89l.812 1.22A2 2 0 0018.07 7H19a2 2 0 012 2v9a2 2 0 01-2 2H5a2 2 0 01-2-2V9z" />
						<circle cx="12" cy="13" r="3" />
					</svg>
					<span class="text-xs font-medium">Change Banner</span>
				</div>
				<input type="file" accept="image/*" class="hidden" onchange={handleBannerSelect} />
			</label>
		</div>

		<!-- Avatar + info, overlapping banner -->
		<div class="relative px-6 pb-4">
			<div class="flex items-end gap-4">
				<div class="relative -mt-10">
					<div class="rounded-xl bg-bg-secondary p-1">
						<Avatar
							name={$currentUser.display_name ?? $currentUser.username}
							src={avatarPreview ?? avatarUrl($currentUser.avatar_id)}
							size="lg"
							status={$currentUser.status_presence}
						/>
					</div>
					<label class="absolute inset-1 flex cursor-pointer items-center justify-center rounded-md bg-black/50 opacity-0 transition-opacity hover:opacity-100">
						<svg class="h-5 w-5 text-white" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
							<path d="M3 9a2 2 0 012-2h.93a2 2 0 001.664-.89l.812-1.22A2 2 0 0110.07 4h3.86a2 2 0 011.664.89l.812 1.22A2 2 0 0018.07 7H19a2 2 0 012 2v9a2 2 0 01-2 2H5a2 2 0 01-2-2V9z" />
							<circle cx="12" cy="13" r="3" />
						</svg>
						<input type="file" accept="image/*" class="hidden" onchange={handleAvatarSelect} />
					</label>
				</div>
				<div class="mb-1">
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
	</div>

	<!-- Banner color -->
	<div class="mb-4">
		<label for="profile-color" class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">
			Profile Color
		</label>
		<p class="mb-2 text-xs text-text-muted">Used as your profile banner background when no banner image is set.</p>
		<div class="flex items-center gap-3">
			<input id="profile-color" type="color" class="h-9 w-9 cursor-pointer rounded border border-border-primary bg-bg-secondary" bind:value={accentColor} />
			<input type="text" class="input w-28 font-mono text-xs" bind:value={accentColor} maxlength="7" aria-label="Profile color hex value" />
			{#if $currentUser.banner_id || bannerPreview}
				<button
					class="text-xs text-red-400 hover:text-red-300"
					onclick={() => { bannerFile = null; bannerPreview = null; bannerRemoved = true; }}
					title="Remove banner image to use color instead"
				>
					Remove Banner
				</button>
			{/if}
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

	<!-- Profile Links -->
	<div class="mt-8 rounded-lg bg-bg-secondary p-6">
		<ProfileLinkEditor />
	</div>
{/if}

<!-- Image Cropper Modal -->
<Modal open={!!cropperFile} title={cropperTarget === 'avatar' ? 'Crop Avatar' : 'Crop Banner'} onclose={() => (cropperFile = null)}>
	{#if cropperFile}
		<ImageCropper
			file={cropperFile}
			shape={cropperTarget === 'avatar' ? 'circle' : 'rect'}
			outputWidth={cropperTarget === 'avatar' ? 256 : 960}
			outputHeight={cropperTarget === 'avatar' ? 256 : 320}
			oncrop={handleCropComplete}
			oncancel={() => (cropperFile = null)}
		/>
	{/if}
</Modal>
