<script lang="ts">
	import Modal from './Modal.svelte';
	import { markAllRead } from '$lib/stores/unreads';
	import { goto } from '$app/navigation';

	interface Props {
		onToggleSearch?: () => void;
		onToggleMembers?: () => void;
		onToggleQuickSwitcher?: () => void;
	}

	let { onToggleSearch, onToggleMembers, onToggleQuickSwitcher }: Props = $props();
	let showHelp = $state(false);

	function handleGlobalKeydown(e: KeyboardEvent) {
		// Ignore if focused in an input/textarea
		const tag = (e.target as HTMLElement)?.tagName;
		const isInput = tag === 'INPUT' || tag === 'TEXTAREA' || (e.target as HTMLElement)?.isContentEditable;

		// Ctrl+K or Cmd+K: toggle search
		if ((e.ctrlKey || e.metaKey) && e.key === 'k') {
			e.preventDefault();
			onToggleSearch?.();
			return;
		}

		// Ctrl+G or Cmd+G: toggle quick switcher
		if ((e.ctrlKey || e.metaKey) && e.key === 'g') {
			e.preventDefault();
			onToggleQuickSwitcher?.();
			return;
		}

		// Ctrl+Shift+M: mark all as read
		if ((e.ctrlKey || e.metaKey) && e.shiftKey && e.key === 'M') {
			e.preventDefault();
			markAllRead();
			return;
		}

		// Ctrl+Shift+U: toggle member list
		if ((e.ctrlKey || e.metaKey) && e.shiftKey && e.key === 'U') {
			e.preventDefault();
			onToggleMembers?.();
			return;
		}

		// Ctrl+,: open settings
		if ((e.ctrlKey || e.metaKey) && e.key === ',') {
			e.preventDefault();
			goto('/app/settings');
			return;
		}

		// ? key (not in input): show shortcuts help
		if (e.key === '?' && !isInput && !e.ctrlKey && !e.metaKey) {
			e.preventDefault();
			showHelp = !showHelp;
			return;
		}
	}

	const shortcuts = [
		{ keys: 'Ctrl+K', description: 'Open command palette' },
		{ keys: 'Ctrl+G', description: 'Quick switcher (jump to channel/guild)' },
		{ keys: 'Ctrl+Shift+M', description: 'Mark all as read' },
		{ keys: 'Ctrl+Shift+U', description: 'Toggle member list' },
		{ keys: 'Ctrl+,', description: 'Open settings' },
		{ keys: 'Enter', description: 'Send message' },
		{ keys: 'Shift+Enter', description: 'New line in message' },
		{ keys: 'Escape', description: 'Close panel/modal, cancel edit/reply' },
		{ keys: 'Arrow Up', description: 'Edit last message (when input is empty)' },
		{ keys: '?', description: 'Show keyboard shortcuts' },
		{ keys: '+/-', description: 'Zoom in/out (in image lightbox)' },
		{ keys: '0', description: 'Reset zoom (in image lightbox)' }
	];
</script>

<svelte:document onkeydown={handleGlobalKeydown} />

<Modal open={showHelp} title="Keyboard Shortcuts" onclose={() => (showHelp = false)}>
	<div class="space-y-2">
		{#each shortcuts as shortcut}
			<div class="flex items-center justify-between rounded px-2 py-1.5 hover:bg-bg-modifier">
				<span class="text-sm text-text-secondary">{shortcut.description}</span>
				<kbd class="rounded bg-bg-primary px-2 py-0.5 text-xs font-mono text-text-muted">{shortcut.keys}</kbd>
			</div>
		{/each}
	</div>
</Modal>
