<script lang="ts">
	import { api } from '$lib/api/client';
	import { currentGuildId } from '$lib/stores/guilds';
	import { clientConfig, isFeatureEnabled } from '$lib/stores/clientConfig';
	import type { CustomEmoji } from '$lib/types';

	interface Props {
		onselect: (emoji: string) => void;
		onclose: () => void;
	}

	let { onselect, onclose }: Props = $props();
	let search = $state('');
	let activeCategory = $state('smileys');
	let customEmoji = $state<CustomEmoji[]>([]);
	const hasCustomEmoji = $derived(isFeatureEnabled($clientConfig, 'custom_emoji'));

	$effect(() => {
		const guildId = $currentGuildId;
		if (!guildId || !hasCustomEmoji) {
			customEmoji = [];
			if (activeCategory === 'custom') activeCategory = 'smileys';
			return;
		}
		api.getGuildEmoji(guildId)
			.then((emoji) => {
				customEmoji = emoji;
				if (emoji.length > 0 && activeCategory === 'custom') return;
			})
			.catch(() => {
				customEmoji = [];
			});
	});

	const categories: { id: string; label: string; icon: string; emojis: string[] }[] = [
		{ id: 'smileys', label: 'Smileys', icon: '😊', emojis: ['😀','😃','😄','😁','😆','😅','🤣','😂','🙂','🙃','😉','😊','😇','🥰','😍','🤩','😘','😗','😚','😙','🥲','😋','😛','😜','🤪','😝','🤑','🤗','🤭','🤫','🤔','🫡','🤐','🤨','😐','😑','😶','🫥','😏','😒','🙄','😬','🤥','😌','😔','😪','🤤','😴','😷','🤒','🤕','🤢','🤮','🥵','🥶','🥴','😵','🤯','🤠','🥳','🥸','😎','🤓','🧐','😕','🫤','😟','🙁','😮','😯','😲','😳','🥺','🥹','😦','😧','😨','😰','😥','😢','😭','😱','😖','😣','😞','😓','😩','😫','🥱','😤','😡','😠','🤬','😈','👿','💀','☠️','💩','🤡','👹','👺','👻','👽','👾','🤖'] },
		{ id: 'gestures', label: 'Gestures', icon: '👋', emojis: ['👋','🤚','🖐️','✋','🖖','🫱','🫲','🫳','🫴','👌','🤌','🤏','✌️','🤞','🫰','🤟','🤘','🤙','👈','👉','👆','🖕','👇','☝️','🫵','👍','👎','✊','👊','🤛','🤜','👏','🙌','🫶','👐','🤲','🤝','🙏','💪','🦾','🦿','🦵','🦶','👂','🦻','👃','🧠','🫀','🫁','🦷','🦴','👀','👁️','👅','👄'] },
		{ id: 'people', label: 'People', icon: '👤', emojis: ['👶','👧','🧒','👦','👩','🧑','👨','👩‍🦱','🧑‍🦱','👨‍🦱','👩‍🦰','🧑‍🦰','👨‍🦰','👱‍♀️','👱','👱‍♂️','👩‍🦳','🧑‍🦳','👨‍🦳','👩‍🦲','🧑‍🦲','👨‍🦲','🧔‍♀️','🧔','🧔‍♂️','👵','🧓','👴','👲','👳‍♀️','👳','👳‍♂️','🧕','👮‍♀️','👮','👮‍♂️','👷‍♀️','👷','👷‍♂️','💂‍♀️','💂','💂‍♂️','🕵️‍♀️','🕵️','🕵️‍♂️'] },
		{ id: 'hearts', label: 'Hearts', icon: '❤️', emojis: ['❤️','🧡','💛','💚','💙','💜','🖤','🤍','🤎','💔','❤️‍🔥','❤️‍🩹','❣️','💕','💞','💓','💗','💖','💘','💝','💟','♥️','🫶','💑','💏','💋'] },
		{ id: 'nature', label: 'Nature', icon: '🌿', emojis: ['🐶','🐱','🐭','🐹','🐰','🦊','🐻','🐼','🐻‍❄️','🐨','🐯','🦁','🐮','🐷','🐸','🐵','🙈','🙉','🙊','🐒','🐔','🐧','🐦','🐤','🐣','🐥','🦆','🦅','🦉','🦇','🐺','🐗','🐴','🦄','🐝','🪱','🐛','🦋','🐌','🐞','🐜','🪰','🪲','🪳','🦟','🦗','🕷️','🕸️','🦂','🐢','🐍','🦎','🦖','🦕','🐙','🦑','🦐','🦞','🦀','🐡','🐠','🐟','🐬','🐳','🐋','🦈','🐊'] },
		{ id: 'food', label: 'Food', icon: '🍔', emojis: ['🍏','🍎','🍐','🍊','🍋','🍌','🍉','🍇','🍓','🫐','🍈','🍒','🍑','🥭','🍍','🥥','🥝','🍅','🍆','🥑','🥦','🥬','🥒','🌶️','🫑','🌽','🥕','🫒','🧄','🧅','🥔','🍠','🫘','🥐','🥖','🍞','🥨','🥯','🧀','🥚','🍳','🧈','🥞','🧇','🥓','🥩','🍗','🍖','🦴','🌭','🍔','🍟','🍕','🫓','🥪','🥙','🧆','🌮','🌯','🫔','🥗','🥘','🫕','🥫','🍝','🍜','🍲','🍛','🍣','🍱','🥟','🦪','🍤','🍙','🍚','🍘','🍥'] },
		{ id: 'activities', label: 'Activities', icon: '⚽', emojis: ['⚽','🏀','🏈','⚾','🥎','🎾','🏐','🏉','🥏','🎱','🪀','🏓','🏸','🏒','🏑','🥍','🏏','🪃','🥅','⛳','🪁','🏹','🎣','🤿','🥊','🥋','🎽','🛹','🛼','🛷','⛸️','🥌','🎿','⛷️','🏂','🪂','🏋️','🤸','🤺','⛹️','🤾','🏌️','🏇','🧘','🏄','🏊','🤽','🚣','🧗','🚵','🚴','🏆','🥇','🥈','🥉','🏅','🎖️','🏵️','🎗️','🎪','🤹','🎭','🎨','🎬','🎤','🎧','🎼','🎹','🥁','🪘','🎷','🎺','🪗','🎸','🪕','🎻'] },
		{ id: 'objects', label: 'Objects', icon: '💡', emojis: ['⌚','📱','📲','💻','⌨️','🖥️','🖨️','🖱️','🖲️','🕹️','🗜️','💽','💾','💿','📀','📼','📷','📸','📹','🎥','📽️','🎞️','📞','☎️','📟','📠','📺','📻','🎙️','🎚️','🎛️','🧭','⏱️','⏲️','⏰','🕰️','⌛','⏳','📡','🔋','🔌','💡','🔦','🕯️','🪔','🧯','🛢️','💰','🪙','💴','💵','💶','💷','🪪','💳','💎','⚖️','🪜','🧰','🪛','🔧','🔨','⚒️','🛠️','⛏️','🪚','🔩','⚙️','🪤','🧱','⛓️','🧲','🔫','💣','🧨','🪓','🔪','🗡️','⚔️','🛡️','🚬','⚰️','🪦','⚱️','🏺','🔮','📿','🧿','🪬','💈'] },
		{ id: 'symbols', label: 'Symbols', icon: '✨', emojis: ['✨','🎆','🎇','🧨','🎉','🎊','🎈','🎀','🎁','🎗️','🎟️','🎫','🔮','🪄','🎯','🎲','🎰','🧩','♠️','♥️','♦️','♣️','🃏','🀄','🎴','🔇','🔈','🔉','🔊','📢','📣','📯','🔔','🔕','🎵','🎶','💹','🏧','🚮','🚰','♿','🚹','🚺','🚻','🚼','🚾','🛂','🛃','🛄','🛅','⚠️','🚸','⛔','🚫','🚳','🚭','🚯','🚱','🚷','📵','🔞','☢️','☣️','✅','❌','❓','❗','‼️','⁉️','💯','🔅','🔆'] },
		{ id: 'flags', label: 'Flags', icon: '🏁', emojis: ['🏁','🚩','🎌','🏴','🏳️','🏳️‍🌈','🏳️‍⚧️','🏴‍☠️','🇺🇸','🇬🇧','🇫🇷','🇩🇪','🇯🇵','🇰🇷','🇨🇳','🇮🇳','🇧🇷','🇷🇺','🇨🇦','🇦🇺','🇲🇽','🇮🇹','🇪🇸','🇳🇱'] }
	];

	const filteredEmojis = $derived.by(() => {
		if (activeCategory === 'custom') {
			const q = search.trim().toLowerCase();
			return customEmoji
				.filter((emoji) => !q || emoji.name.toLowerCase().includes(q))
				.map((emoji) => `:${emoji.name}:`);
		}
		if (!search.trim()) {
			return categories.find((c) => c.id === activeCategory)?.emojis ?? [];
		}
		// Simple search: return emojis from all categories, deduplicated to avoid key conflicts.
		return [...new Set(categories.flatMap((c) => c.emojis))];
	});

	function handleClickOutside(e: MouseEvent) {
		const target = e.target as HTMLElement;
		if (!target.closest('.emoji-picker')) onclose();
	}

	function handleKeydown(e: KeyboardEvent) {
		if (e.key === 'Escape') onclose();
	}
</script>

<svelte:document onclick={handleClickOutside} onkeydown={handleKeydown} />

<div class="emoji-picker fixed inset-x-0 bottom-0 z-50 max-h-[60vh] rounded-t-xl bg-bg-floating shadow-xl md:absolute md:inset-auto md:bottom-full md:right-0 md:mb-2 md:w-80 md:max-h-none md:rounded-lg" role="dialog" aria-label="Emoji picker">
	<!-- Search -->
	<div class="border-b border-bg-modifier p-2">
		<input
			type="text"
			class="w-full rounded bg-bg-primary px-3 py-1.5 text-sm text-text-primary placeholder:text-text-muted outline-none"
			placeholder="Search emoji..."
			bind:value={search}
		/>
	</div>

	<!-- Category tabs -->
	{#if !search.trim()}
		<div class="flex border-b border-bg-modifier px-1">
			{#if customEmoji.length > 0}
				<button
					class="flex-1 p-1.5 text-center text-sm transition-colors {activeCategory === 'custom' ? 'border-b-2 border-brand-500' : 'hover:bg-bg-modifier'}"
					onclick={() => (activeCategory = 'custom')}
					title="Custom"
				>
					:
				</button>
			{/if}
			{#each categories as cat (cat.id)}
				<button
					class="flex-1 p-1.5 text-center text-sm transition-colors {activeCategory === cat.id ? 'border-b-2 border-brand-500' : 'hover:bg-bg-modifier'}"
					onclick={() => (activeCategory = cat.id)}
					title={cat.label}
				>
					{cat.icon}
				</button>
			{/each}
		</div>
	{/if}

	<!-- Emoji grid -->
	<div class="grid max-h-56 grid-cols-8 gap-0.5 overflow-y-auto p-2">
		{#each filteredEmojis as emoji (emoji)}
			<button
				class="flex h-8 w-8 items-center justify-center rounded text-xl hover:bg-bg-modifier {emoji.startsWith(':') ? 'px-1 text-[10px] font-medium text-brand-300' : ''}"
				onclick={() => onselect(emoji)}
				title={emoji}
			>
				{emoji.startsWith(':') ? emoji.slice(1, -1).slice(0, 4) : emoji}
			</button>
		{/each}
	</div>
</div>
