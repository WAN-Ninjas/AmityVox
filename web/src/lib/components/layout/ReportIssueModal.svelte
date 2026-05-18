<script lang="ts">
	import { api } from '$lib/api/client';
	import Modal from '$components/common/Modal.svelte';
	import { addToast } from '$lib/stores/toast';
	import { createAsyncOp } from '$lib/utils/asyncOp';

	interface Props {
		open: boolean;
		onclose: () => void;
	}

	let { open = $bindable(false), onclose }: Props = $props();

	let title = $state('');
	let description = $state('');
	let category = $state('general');
	let submitOp = $state(createAsyncOp());

	async function submitIssue() {
		if (!title.trim() || !description.trim()) return;
		await submitOp.run(
			() => api.createIssue(title.trim(), description.trim(), category),
			(message) => addToast(message, 'error')
		);
		if (!submitOp.error) {
			addToast('Issue reported successfully', 'success');
			title = '';
			description = '';
			category = 'general';
			onclose();
		}
	}
</script>

<Modal {open} title="Report Issue" persistent {onclose}>
	<div class="mb-4">
		<label for="issueTitle" class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">Title</label>
		<input id="issueTitle" type="text" class="input w-full" bind:value={title} placeholder="Brief summary" maxlength="200" />
	</div>
	<div class="mb-4">
		<label for="issueCategory" class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">Category</label>
		<select id="issueCategory" class="input w-full" bind:value={category}>
			<option value="general">General</option>
			<option value="bug">Bug</option>
			<option value="abuse">Abuse</option>
			<option value="suggestion">Suggestion</option>
		</select>
	</div>
	<div class="mb-4">
		<label for="issueDesc" class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">Description</label>
		<textarea id="issueDesc" class="input w-full" rows="4" bind:value={description} placeholder="Describe the issue in detail..."></textarea>
	</div>
	<div class="flex justify-end gap-2">
		<button class="btn-secondary" onclick={onclose}>Cancel</button>
		<button class="btn-primary" onclick={submitIssue} disabled={submitOp.loading || !title.trim() || !description.trim()}>
			{submitOp.loading ? 'Submitting...' : 'Submit'}
		</button>
	</div>
</Modal>
