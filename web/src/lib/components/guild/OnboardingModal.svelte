<script lang="ts">
	import { api } from '$lib/api/client';
	import type { OnboardingConfig } from '$lib/types';

	interface Props {
		open?: boolean;
		guildId: string;
		guildName: string;
		onboarding: OnboardingConfig;
		onComplete: () => void;
	}

	let { open = $bindable(false), guildId, guildName, onboarding, onComplete }: Props = $props();

	let step = $state(0);
	let rulesAccepted = $state(false);
	let promptResponses = $state<Record<string, string[]>>({});
	let submitting = $state(false);
	let error = $state('');

	// Step layout:
	// 0 = Welcome
	// 1 = Rules (skipped if no rules)
	// 2..2+N-1 = Prompts (one per screen)
	// last = Complete

	const hasRules = $derived(onboarding.rules.length > 0);
	const prompts = $derived(onboarding.prompts.slice().sort((a, b) => a.position - b.position));
	const requiredPrompts = $derived(prompts.filter((p) => p.required));

	// Calculate total steps
	const totalSteps = $derived(1 + (hasRules ? 1 : 0) + prompts.length + 1);

	// Map step index to content
	const stepType = $derived.by(() => {
		if (step === 0) return 'welcome';
		let s = 1;
		if (hasRules) {
			if (step === s) return 'rules';
			s++;
		}
		const promptIndex = step - s;
		if (promptIndex >= 0 && promptIndex < prompts.length) return 'prompt';
		return 'complete';
	});

	const currentPromptIndex = $derived.by(() => {
		let s = 1 + (hasRules ? 1 : 0);
		return step - s;
	});

	const currentPrompt = $derived(
		currentPromptIndex >= 0 && currentPromptIndex < prompts.length
			? prompts[currentPromptIndex]
			: null
	);

	function canContinue(): boolean {
		if (stepType === 'rules') return rulesAccepted;
		if (stepType === 'prompt' && currentPrompt?.required) {
			const selected = promptResponses[currentPrompt.id] ?? [];
			return selected.length > 0;
		}
		return true;
	}

	function handleNext() {
		if (!canContinue()) return;
		step++;
	}

	function toggleOption(promptId: string, optionId: string, singleSelect: boolean) {
		const current = promptResponses[promptId] ?? [];
		if (singleSelect) {
			promptResponses = { ...promptResponses, [promptId]: [optionId] };
		} else {
			if (current.includes(optionId)) {
				promptResponses = { ...promptResponses, [promptId]: current.filter((id) => id !== optionId) };
			} else {
				promptResponses = { ...promptResponses, [promptId]: [...current, optionId] };
			}
		}
	}

	function isOptionSelected(promptId: string, optionId: string): boolean {
		return (promptResponses[promptId] ?? []).includes(optionId);
	}

	async function handleComplete() {
		submitting = true;
		error = '';
		try {
			await api.completeOnboarding(guildId, promptResponses);
			open = false;
			onComplete();
		} catch (err: any) {
			error = err.message || 'Failed to complete onboarding';
		} finally {
			submitting = false;
		}
	}

	// Progress percentage
	const progress = $derived(Math.round((step / (totalSteps - 1)) * 100));
</script>

{#if open}
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div
		class="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4"
		role="dialog"
		aria-modal="true"
		tabindex="-1"
	>
		<div class="w-full max-w-lg rounded-xl bg-bg-secondary shadow-2xl">
			<!-- Progress bar -->
			<div class="h-1 w-full overflow-hidden rounded-t-xl bg-bg-modifier">
				<div
					class="h-full bg-brand-500 transition-all duration-300"
					style="width: {progress}%"
				></div>
			</div>

			<div class="p-6">
				{#if error}
					<div class="mb-4 rounded bg-red-500/10 px-3 py-2 text-sm text-red-400">{error}</div>
				{/if}

				<!-- STEP: Welcome -->
				{#if stepType === 'welcome'}
					<div class="text-center">
						<div class="mx-auto mb-4 flex h-16 w-16 items-center justify-center rounded-2xl bg-brand-600 text-2xl font-bold text-white">
							{guildName
								.split(' ')
								.map((w) => w[0])
								.join('')
								.slice(0, 2)
								.toUpperCase()}
						</div>
						<h2 class="mb-2 text-2xl font-bold text-text-primary">Welcome to {guildName}!</h2>
						{#if onboarding.welcome_message}
							<p class="mb-6 text-sm leading-relaxed text-text-muted">{onboarding.welcome_message}</p>
						{:else}
							<p class="mb-6 text-sm text-text-muted">Let's get you set up. This will only take a moment.</p>
						{/if}
						<button class="btn-primary w-full" onclick={handleNext}>
							Continue
						</button>
					</div>

				<!-- STEP: Rules -->
				{:else if stepType === 'rules'}
					<div>
						<h2 class="mb-1 text-xl font-bold text-text-primary">Server Rules</h2>
						<p class="mb-4 text-sm text-text-muted">Please read and agree to the rules before continuing.</p>

						<div class="mb-4 max-h-64 space-y-2 overflow-y-auto rounded-lg bg-bg-primary p-4">
							{#each onboarding.rules as rule, i}
								<div class="flex gap-3">
									<span class="mt-0.5 flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-brand-600/20 text-xs font-bold text-brand-400">
										{i + 1}
									</span>
									<p class="text-sm text-text-primary">{rule}</p>
								</div>
							{/each}
						</div>

						<label class="mb-4 flex cursor-pointer items-center gap-3">
							<input
								type="checkbox"
								bind:checked={rulesAccepted}
								class="h-4 w-4 rounded border-text-muted"
							/>
							<span class="text-sm text-text-primary">I have read and agree to these rules</span>
						</label>

						<button
							class="btn-primary w-full"
							onclick={handleNext}
							disabled={!rulesAccepted}
						>
							Continue
						</button>
					</div>

				<!-- STEP: Prompt -->
				{:else if stepType === 'prompt' && currentPrompt}
					<div>
						<h2 class="mb-1 text-xl font-bold text-text-primary">{currentPrompt.title}</h2>
						<p class="mb-4 text-sm text-text-muted">
							{#if currentPrompt.single_select}
								Choose one option{currentPrompt.required ? '' : ' (optional)'}.
							{:else}
								Select all that apply{currentPrompt.required ? '' : ' (optional)'}.
							{/if}
						</p>

						<div class="mb-4 max-h-72 space-y-2 overflow-y-auto">
							{#each currentPrompt.options as option (option.id)}
								{@const selected = isOptionSelected(currentPrompt.id, option.id)}
								<button
									class="flex w-full items-center gap-3 rounded-lg border p-3 text-left transition-colors {selected
										? 'border-brand-500 bg-brand-500/10'
										: 'border-bg-modifier bg-bg-primary hover:border-text-muted/30 hover:bg-bg-primary/80'}"
									onclick={() => toggleOption(currentPrompt.id, option.id, currentPrompt.single_select)}
								>
									<!-- Selection indicator -->
									<div class="shrink-0">
										{#if currentPrompt.single_select}
											<div class="flex h-5 w-5 items-center justify-center rounded-full border-2 {selected ? 'border-brand-500' : 'border-text-muted'}">
												{#if selected}
													<div class="h-2.5 w-2.5 rounded-full bg-brand-500"></div>
												{/if}
											</div>
										{:else}
											<div class="flex h-5 w-5 items-center justify-center rounded border-2 {selected ? 'border-brand-500 bg-brand-500' : 'border-text-muted'}">
												{#if selected}
													<svg class="h-3 w-3 text-white" fill="none" stroke="currentColor" stroke-width="3" viewBox="0 0 24 24">
														<path d="M5 13l4 4L19 7" />
													</svg>
												{/if}
											</div>
										{/if}
									</div>

									<div class="min-w-0 flex-1">
										<div class="flex items-center gap-2">
											{#if option.emoji}
												<span class="text-lg">{option.emoji}</span>
											{/if}
											<span class="text-sm font-medium text-text-primary">{option.label}</span>
										</div>
										{#if option.description}
											<p class="mt-0.5 text-xs text-text-muted">{option.description}</p>
										{/if}
									</div>
								</button>
							{/each}
						</div>

						<button
							class="btn-primary w-full"
							onclick={handleNext}
							disabled={currentPrompt.required && (promptResponses[currentPrompt.id] ?? []).length === 0}
						>
							Continue
						</button>
					</div>

				<!-- STEP: Complete -->
				{:else if stepType === 'complete'}
					<div class="text-center">
						<div class="mx-auto mb-4 flex h-16 w-16 items-center justify-center rounded-full bg-green-500/20">
							<svg class="h-8 w-8 text-green-400" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
								<path d="M5 13l4 4L19 7" />
							</svg>
						</div>
						<h2 class="mb-2 text-2xl font-bold text-text-primary">You're all set!</h2>
						<p class="mb-6 text-sm text-text-muted">
							Welcome to <strong class="text-text-primary">{guildName}</strong>. You're ready to start chatting.
						</p>
						<button
							class="btn-primary w-full"
							onclick={handleComplete}
							disabled={submitting}
						>
							{submitting ? 'Finishing...' : 'Start chatting'}
						</button>
					</div>
				{/if}

				<!-- Step indicator -->
				{#if totalSteps > 2}
					<div class="mt-4 flex items-center justify-center gap-1.5">
						{#each Array(totalSteps) as _, i}
							<div
								class="h-1.5 rounded-full transition-all {i === step
									? 'w-4 bg-brand-500'
									: i < step
										? 'w-1.5 bg-brand-500/40'
										: 'w-1.5 bg-bg-modifier'}"
							></div>
						{/each}
					</div>
				{/if}
			</div>
		</div>
	</div>
{/if}
