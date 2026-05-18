<script lang="ts">
	import { goto } from '$app/navigation';
	import Avatar from '$components/common/Avatar.svelte';
	import StatusPicker from '$components/common/StatusPicker.svelte';
	import { currentUser } from '$lib/stores/auth';
	import { presenceMap } from '$lib/stores/presence';
	import { avatarUrl } from '$lib/utils/avatar';

	interface Props {
		onreportissue: () => void;
	}

	let { onreportissue }: Props = $props();

	let showStatusPicker = $state(false);
</script>

{#if $currentUser}
	{@const myStatus = $presenceMap.get($currentUser.id) ?? $currentUser.status_presence ?? 'online'}
	<div class="relative border-t border-bg-floating bg-bg-primary/50 p-2">
		<StatusPicker bind:open={showStatusPicker} onclose={() => (showStatusPicker = false)} />
		<div class="flex items-center gap-2">
			<button
				class="flex min-w-0 flex-1 items-center gap-2 rounded-md px-1 py-0.5 transition-colors hover:bg-bg-modifier"
				onclick={(event) => { event.stopPropagation(); showStatusPicker = !showStatusPicker; }}
				title="Set status"
			>
				<Avatar name={$currentUser.display_name ?? $currentUser.username} src={avatarUrl($currentUser.avatar_id)} size="sm" status={myStatus} />
				<div class="min-w-0 flex-1 text-left">
					<p class="truncate text-sm font-medium text-text-primary">
						{$currentUser.display_name ?? $currentUser.username}
					</p>
					<p class="truncate text-xs text-text-muted">
						{$currentUser.status_text ?? myStatus}
					</p>
				</div>
			</button>
			<button
				class="rounded-md p-1.5 text-orange-400 hover:bg-bg-modifier hover:text-orange-300"
				onclick={onreportissue}
				title="Report Issue"
			>
				<svg class="h-6 w-6" fill="none" stroke="currentColor" stroke-width="1.5" viewBox="0 0 24 24">
					<path d="M5.072 19h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
					<path d="M12 9v4" stroke-linecap="round" />
					<circle cx="12" cy="16" r="0.5" fill="currentColor" />
				</svg>
			</button>
			<button
				class="rounded-md p-1.5 text-text-muted hover:bg-bg-modifier hover:text-text-primary"
				onclick={() => goto('/app/settings')}
				title="User Settings"
			>
				<svg class="h-6 w-6" fill="none" stroke="currentColor" stroke-width="1.5" viewBox="0 0 24 24">
					<path d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.066 2.573c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.573 1.066c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.066-2.573c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z" />
					<circle cx="12" cy="12" r="3" />
				</svg>
			</button>
		</div>
	</div>
{/if}
