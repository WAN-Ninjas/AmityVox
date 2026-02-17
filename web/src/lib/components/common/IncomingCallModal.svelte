<!-- IncomingCallModal.svelte — Full-screen overlay for incoming DM/Group voice/video calls. -->
<script lang="ts">
	import { activeIncomingCall, dismissIncomingCall } from '$lib/stores/callRing';
	import { joinVoice, toggleCamera } from '$lib/stores/voice';
	import { playNotificationSound } from '$lib/utils/sounds';
	import { addToast } from '$lib/stores/toast';
	import { goto } from '$app/navigation';
	import Avatar from './Avatar.svelte';

	let accepting = $state(false);

	// Plain variable (not $state) to avoid reactive loops inside $effect.
	let ringIntervalId: ReturnType<typeof setInterval> | null = null;

	function stopRingtone() {
		if (ringIntervalId) {
			clearInterval(ringIntervalId);
			ringIntervalId = null;
		}
	}

	// Play ringtone on loop while there's an active call.
	$effect(() => {
		const call = $activeIncomingCall;
		if (call) {
			// Play immediately, then repeat every 2.5s.
			playNotificationSound('ringtone', 90);
			ringIntervalId = setInterval(() => {
				playNotificationSound('ringtone', 90);
			}, 2500);
		}

		return () => {
			stopRingtone();
		};
	});

	async function acceptCall(withVideo: boolean) {
		const call = $activeIncomingCall;
		if (!call || accepting) return;
		accepting = true;
		const channelId = call.channelId;
		const displayName = call.callerDisplayName ?? call.callerName;
		stopRingtone();
		dismissIncomingCall(channelId);
		try {
			await joinVoice(channelId, '', displayName);
			if (withVideo) {
				await toggleCamera();
			}
			goto(`/app/dms/${channelId}`);
		} catch {
			addToast('Failed to join call', 'error');
		} finally {
			accepting = false;
		}
	}

	function declineCall() {
		const call = $activeIncomingCall;
		if (!call) return;
		stopRingtone();
		dismissIncomingCall(call.channelId);
	}

	function handleKeydown(e: KeyboardEvent) {
		if (e.key === 'Escape' && $activeIncomingCall) {
			declineCall();
		}
	}
</script>

<svelte:window onkeydown={handleKeydown} />

{#if $activeIncomingCall}
	{@const call = $activeIncomingCall}
	<div class="fixed inset-0 z-[100] flex items-center justify-center bg-black/60 backdrop-blur-sm">
		<div class="animate-slide-up flex w-80 flex-col items-center gap-4 rounded-2xl bg-bg-secondary p-6 shadow-2xl ring-1 ring-white/10">
			<!-- Caller avatar with pulsing ring -->
			<div class="relative">
				<div class="ring-pulse pointer-events-none absolute -inset-2 rounded-full"></div>
				<Avatar
					name={call.callerDisplayName ?? call.callerName}
					src={call.callerAvatarId ? `/api/v1/files/${call.callerAvatarId}` : null}
					size="xl"
				/>
			</div>

			<!-- Caller info -->
			<div class="text-center">
				<h2 class="text-lg font-semibold text-text-primary">
					{call.callerDisplayName ?? call.callerName}
				</h2>
				<p class="text-sm text-text-muted">
					Incoming {call.channelType === 'group' ? 'group ' : ''}call...
				</p>
			</div>

			<!-- Action buttons -->
			<div class="flex items-center gap-4">
				<!-- Decline -->
				<button
					type="button"
					class="flex h-14 w-14 items-center justify-center rounded-full bg-red-500 text-white shadow-lg transition-transform hover:scale-110 hover:bg-red-600 active:scale-95"
					onclick={declineCall}
					title="Decline"
				>
					<svg class="h-6 w-6" fill="currentColor" viewBox="0 0 24 24">
						<path d="M12 9c-1.6 0-3.15.25-4.6.72v3.1c0 .39-.23.74-.56.9-.98.49-1.87 1.12-2.66 1.85-.18.18-.43.28-.7.28-.28 0-.53-.11-.71-.29L.29 13.08a.956.956 0 01-.29-.7c0-.28.11-.53.29-.71C3.34 8.78 7.46 7 12 7s8.66 1.78 11.71 4.67c.18.18.29.43.29.71 0 .28-.11.53-.29.71l-2.48 2.48c-.18.18-.43.29-.71.29-.27 0-.52-.11-.7-.28a11.27 11.27 0 00-2.67-1.85.996.996 0 01-.56-.9v-3.1C15.15 9.25 13.6 9 12 9z" />
					</svg>
				</button>

				<!-- Accept Voice -->
				<button
					type="button"
					class="flex h-14 w-14 items-center justify-center rounded-full bg-green-500 text-white shadow-lg transition-transform hover:scale-110 hover:bg-green-600 active:scale-95 disabled:opacity-50"
					onclick={() => acceptCall(false)}
					disabled={accepting}
					title="Accept Voice Call"
				>
					<svg class="h-6 w-6" fill="currentColor" viewBox="0 0 24 24">
						<path d="M20.01 15.38c-1.23 0-2.42-.2-3.53-.56a.977.977 0 00-1.01.24l-1.57 1.97c-2.83-1.35-5.48-3.9-6.89-6.83l1.95-1.66c.27-.28.35-.67.24-1.02-.37-1.11-.56-2.3-.56-3.53 0-.54-.45-.99-.99-.99H4.19C3.65 3 3 3.24 3 3.99 3 13.28 10.73 21 20.01 21c.71 0 .99-.63.99-1.18v-3.45c0-.54-.45-.99-.99-.99z" />
					</svg>
				</button>

				<!-- Accept Video -->
				<button
					type="button"
					class="flex h-14 w-14 items-center justify-center rounded-full bg-blue-500 text-white shadow-lg transition-transform hover:scale-110 hover:bg-blue-600 active:scale-95 disabled:opacity-50"
					onclick={() => acceptCall(true)}
					disabled={accepting}
					title="Accept Video Call"
				>
					<svg class="h-6 w-6" fill="currentColor" viewBox="0 0 24 24">
						<path d="M17 10.5V7c0-.55-.45-1-1-1H4c-.55 0-1 .45-1 1v10c0 .55.45 1 1 1h12c.55 0 1-.45 1-1v-3.5l4 4v-11l-4 4z" />
					</svg>
				</button>
			</div>
		</div>
	</div>
{/if}

<style>
	@keyframes ring-pulse-anim {
		0%, 100% { box-shadow: 0 0 0 0 rgba(34, 197, 94, 0.6); }
		50% { box-shadow: 0 0 0 12px rgba(34, 197, 94, 0); }
	}
	.ring-pulse {
		animation: ring-pulse-anim 1.5s ease-in-out infinite;
		border-radius: 9999px;
	}

	@keyframes slide-up {
		from { transform: translateY(20px); opacity: 0; }
		to { transform: translateY(0); opacity: 1; }
	}
	.animate-slide-up {
		animation: slide-up 0.3s ease-out;
	}
</style>
