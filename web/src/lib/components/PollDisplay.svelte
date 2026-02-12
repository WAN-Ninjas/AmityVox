<script lang="ts">
	import { api } from '$lib/api/client';
	import type { Poll } from '$lib/types';

	interface Props {
		poll: Poll;
	}

	let { poll }: Props = $props();

	let voting = $state(false);
	let voteError = $state('');

	const hasVoted = $derived(poll.user_votes.length > 0);

	const totalVotes = $derived(poll.total_votes);

	const maxVoteCount = $derived(
		Math.max(...poll.options.map((o) => o.vote_count), 1)
	);

	function percentage(voteCount: number): number {
		if (totalVotes === 0) return 0;
		return Math.round((voteCount / totalVotes) * 100);
	}

	function barWidth(voteCount: number): number {
		if (maxVoteCount === 0) return 0;
		return Math.round((voteCount / maxVoteCount) * 100);
	}

	async function handleVote(optionId: string) {
		if (hasVoted || voting || poll.closed) return;
		voting = true;
		voteError = '';
		try {
			const result = await api.votePoll(poll.channel_id, poll.id, [optionId]);
			poll.user_votes = result.option_ids;
			// Refresh the poll to get updated vote counts.
			const updated = await api.getPoll(poll.channel_id, poll.id);
			poll.options = updated.options;
			poll.total_votes = updated.total_votes;
			poll.user_votes = updated.user_votes;
			poll.closed = updated.closed;
		} catch (err: any) {
			voteError = err.message || 'Failed to vote';
		} finally {
			voting = false;
		}
	}

	const isExpired = $derived(
		poll.expires_at ? new Date(poll.expires_at).getTime() < Date.now() : false
	);

	const sortedOptions = $derived(
		[...poll.options].sort((a, b) => a.position - b.position)
	);
</script>

<div class="rounded-lg bg-bg-secondary p-4">
	<!-- Question -->
	<h4 class="mb-3 text-sm font-semibold text-text-primary">{poll.question}</h4>

	{#if voteError}
		<div class="mb-3 rounded bg-red-500/10 px-3 py-2 text-xs text-red-400">{voteError}</div>
	{/if}

	<!-- Options -->
	<div class="space-y-2">
		{#each sortedOptions as option (option.id)}
			<div class="relative">
				<!-- Background bar -->
				<div class="relative overflow-hidden rounded bg-bg-secondary">
					<div
						class="absolute inset-y-0 left-0 rounded bg-brand-500 transition-all duration-300"
						class:opacity-30={!hasVoted && !poll.closed && !isExpired}
						class:opacity-20={hasVoted || poll.closed || isExpired}
						style="width: {barWidth(option.vote_count)}%"
					></div>

					<div class="relative flex items-center justify-between px-3 py-2">
						<div class="flex items-center gap-2">
							{#if !hasVoted && !poll.closed && !isExpired}
								<button
									class="flex h-5 w-5 shrink-0 items-center justify-center rounded-full border border-text-muted text-text-muted transition-colors hover:border-brand-500 hover:text-brand-500"
									onclick={() => handleVote(option.id)}
									disabled={voting}
									title="Vote for this option"
								>
									{#if voting}
										<div class="h-3 w-3 animate-spin rounded-full border border-brand-500 border-t-transparent"></div>
									{/if}
								</button>
							{:else if poll.user_votes.includes(option.id)}
								<div class="flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-brand-500 text-white">
									<svg class="h-3 w-3" fill="none" stroke="currentColor" stroke-width="3" viewBox="0 0 24 24">
										<path d="M5 13l4 4L19 7" />
									</svg>
								</div>
							{/if}
							<span class="text-sm text-text-primary">{option.text}</span>
						</div>

						<div class="flex items-center gap-2 shrink-0">
							<span class="text-xs font-medium text-text-muted">
								({option.vote_count})
							</span>
							<span class="text-xs font-semibold text-text-primary">
								{percentage(option.vote_count)}%
							</span>
						</div>
					</div>
				</div>
			</div>
		{/each}
	</div>

	<!-- Footer -->
	<div class="mt-3 flex items-center justify-between">
		<span class="text-xs text-text-muted">
			{totalVotes} vote{totalVotes !== 1 ? 's' : ''}
		</span>
		{#if poll.closed}
			<span class="text-xs font-medium text-text-muted">Poll closed</span>
		{:else if isExpired}
			<span class="text-xs font-medium text-text-muted">Poll expired</span>
		{:else if poll.expires_at}
			<span class="text-xs text-text-muted">
				Expires {new Date(poll.expires_at).toLocaleString()}
			</span>
		{/if}
		{#if poll.multi_vote}
			<span class="text-xs text-text-muted">Multiple votes allowed</span>
		{/if}
	</div>
</div>
