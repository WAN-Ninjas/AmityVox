# AmityVox Frontend UI Standards

Conventions and patterns for the SvelteKit frontend. All new code must follow these patterns.

---

## Tech Stack

- **SvelteKit 5** with Svelte 5 runes (`$state`, `$derived`, `$effect`, `$props`)
- **TypeScript** strict mode
- **Tailwind CSS 3.4** with custom Discord-like theme
- **Vite 5** for dev server and build
- **Static adapter** — builds to `build/` served by Caddy

---

## Component Patterns

### Props (Svelte 5 `$props`)

Always define an interface and destructure:

```svelte
<script lang="ts">
interface Props {
  message: Message;
  isCompact?: boolean;
  ondelete?: (id: string) => void;
}

let { message, isCompact = false, ondelete }: Props = $props();
</script>
```

### Reactive State

Use `$state` for mutable local state:
```ts
let content = $state('');
let isOpen = $state(false);
```

Use `$derived` for computed values:
```ts
const isOwnMessage = $derived($currentUser?.id === message.author_id);
const timestamp = $derived(new Date(message.created_at).toLocaleTimeString());
```

Use `$derived.by` for complex computations:
```ts
const filteredMessages = $derived.by(() => {
  return messages.filter(m => m.channel_id === channelId);
});
```

Use `$effect` for side effects:
```ts
$effect(() => {
  if (channelId) loadMessages(channelId);
});
```

### Children (Snippets)

Use Svelte 5 Snippet type for component children:
```svelte
<script lang="ts">
import type { Snippet } from 'svelte';

interface Props {
  title: string;
  children: Snippet;
}
let { title, children }: Props = $props();
</script>

<div>
  <h2>{title}</h2>
  {@render children()}
</div>
```

---

## Store Patterns

### Writable + Derived

Use Map-based stores for entity collections:
```ts
import { writable, derived } from 'svelte/store';

export const guilds = writable<Map<string, Guild>>(new Map());
export const currentGuildId = writable<string | null>(null);

export const guildList = derived(guilds, ($guilds) =>
  Array.from($guilds.values())
);

export const currentGuild = derived(
  [guilds, currentGuildId],
  ([$guilds, $id]) => ($id ? $guilds.get($id) ?? null : null)
);
```

### Mutation Functions

Export named functions, never expose the raw writable:
```ts
export function updateGuild(guild: Guild) {
  guilds.update((map) => {
    map.set(guild.id, guild);
    return new Map(map);
  });
}
```

Always return `new Map(map)` to trigger reactivity.

---

## API Client Pattern

### Making Requests

Use the singleton `api` client from `$lib/api/client.ts`:
```ts
import { api } from '$lib/api/client';

const guilds = await api.getGuilds();
const msg = await api.sendMessage(channelId, content);
```

### Error Handling

Always use try-catch with `ApiRequestError`:
```ts
import { ApiRequestError } from '$lib/api/client';

try {
  await api.deleteMessage(channelId, messageId);
} catch (err) {
  if (err instanceof ApiRequestError) {
    if (err.status === 403) {
      showToast('You do not have permission to delete this message');
    } else {
      showToast(err.message);
    }
  }
}
```

### Response Envelope

Backend returns `{"data": ...}` for success, `{"error": {"code": "...", "message": "..."}}` for errors. The API client unwraps this automatically — you always get the `data` value or an `ApiRequestError`.

---

## Error Handling in Components

### Pattern: State-Based Error Display

```svelte
<script lang="ts">
let error = $state('');
let loading = $state(false);

async function handleSubmit() {
  error = '';
  loading = true;
  try {
    await api.doSomething();
  } catch (err: any) {
    error = err.message || 'Something went wrong';
  } finally {
    loading = false;
  }
}
</script>

{#if error}
  <div class="rounded bg-red-500/10 px-3 py-2 text-sm text-red-400">{error}</div>
{/if}

<button class="btn-primary" disabled={loading}>
  {loading ? 'Saving...' : 'Save'}
</button>
```

### Restore-on-Failure (Optimistic UI)

For message sending, restore content if send fails:
```ts
const msg = content.trim();
content = ''; // optimistic clear
try {
  await api.sendMessage(channelId, msg);
} catch {
  content = msg; // restore on failure
}
```

---

## Loading States

### Full-Page Loading
```svelte
{#if loading}
  <div class="flex h-full items-center justify-center">
    <div class="mx-auto h-8 w-8 animate-spin rounded-full border-2 border-brand-500 border-t-transparent"></div>
  </div>
{/if}
```

### Button Loading
```svelte
<button class="btn-primary" disabled={loading}>
  {loading ? 'Loading...' : 'Submit'}
</button>
```

### Empty States
Always show a helpful message when a list is empty:
```svelte
{#if items.length === 0}
  <div class="flex h-full items-center justify-center text-center">
    <div>
      <p class="text-lg text-text-secondary">No messages yet</p>
      <p class="text-sm text-text-muted">Be the first to say something!</p>
    </div>
  </div>
{/if}
```

---

## Context Menu Pattern

Reusable right-click menus. Use the `<ContextMenu>` component:

```svelte
<script lang="ts">
import ContextMenu from '$components/ContextMenu.svelte';

let contextMenu = $state<{ x: number; y: number } | null>(null);

function handleContextMenu(e: MouseEvent) {
  e.preventDefault();
  contextMenu = { x: e.clientX, y: e.clientY };
}
</script>

<div oncontextmenu={handleContextMenu}>
  <!-- content -->
</div>

{#if contextMenu}
  <ContextMenu x={contextMenu.x} y={contextMenu.y} onclose={() => contextMenu = null}>
    <button onclick={handleEdit}>Edit</button>
    <button onclick={handleDelete} class="text-red-400">Delete</button>
  </ContextMenu>
{/if}
```

### ContextMenu Component Requirements:
- Position near cursor, flip if near viewport edges
- Close on: click outside, Escape key, any item click
- Support divider lines between groups
- Support disabled items (grayed out, no click)
- Animate in (fade + scale)

---

## Modal/Dialog Pattern

Use the `<Modal>` component:

```svelte
<script lang="ts">
import Modal from '$components/Modal.svelte';

let showModal = $state(false);
</script>

<button onclick={() => showModal = true}>Open</button>

<Modal open={showModal} title="Confirm Delete" onclose={() => showModal = false}>
  <p>Are you sure?</p>
  <div class="mt-4 flex justify-end gap-2">
    <button class="btn-secondary" onclick={() => showModal = false}>Cancel</button>
    <button class="btn-danger" onclick={handleConfirm}>Delete</button>
  </div>
</Modal>
```

### Modal Requirements:
- Backdrop click closes (unless prevented)
- Escape key closes
- Focus trap inside modal
- Body scroll lock when open
- Animate in (fade backdrop + slide content)

---

## Toast/Notification Pattern

In-app toast notifications (bottom-right corner):

```ts
import { showToast } from '$lib/stores/toasts';

showToast('Message deleted', 'success');
showToast('Failed to send message', 'error');
showToast('Link copied to clipboard', 'info');
```

### Toast Requirements:
- Stack vertically, newest on bottom
- Auto-dismiss after 5 seconds
- Manual dismiss with X button
- Types: success (green), error (red), info (blue), warning (yellow)
- Animate in (slide from right) and out (fade)
- Max 5 visible at once

---

## CSS/Styling Conventions

### Theme Colors (Tailwind)

Use semantic color names, not raw hex:
```
bg-bg-primary    (#1e1f22)  — main background
bg-bg-secondary  (#2b2d31)  — sidebar background
bg-bg-tertiary   (#313338)  — content area background
bg-bg-modifier   (#383a40)  — hover/active states
bg-bg-floating   (#111214)  — dropdowns, modals, tooltips

text-text-primary   (#f2f3f5)  — main text
text-text-secondary (#b5bac1)  — secondary text
text-text-muted     (#949ba4)  — muted/placeholder text
text-text-link      (#00a8fc)  — links

bg-brand-500  (#3f51b5)  — primary action color
bg-brand-600  (#3949ab)  — hover state

text-status-online  (#23a55a)  — green
text-status-idle    (#f0b232)  — yellow
text-status-dnd     (#f23f43)  — red
text-status-offline (#80848e)  — gray
```

### Utility Classes

Use the pre-defined component classes:
```
.btn         — base button (rounded, padding, transition)
.btn-primary — brand color button
.btn-secondary — modifier color button
.btn-danger  — red destructive button
.input       — text input (rounded, bg-primary, ring)
```

### Responsive Design

Mobile-first with these breakpoints:
- `sm:` (640px) — small tablets
- `md:` (768px) — tablets
- `lg:` (1024px) — desktop
- `xl:` (1280px) — wide desktop

Sidebars collapse to drawers on `< md`.

---

## File Structure

```
web/src/lib/
├── api/
│   ├── client.ts          # API singleton, request methods
│   └── ws.ts              # WebSocket gateway client
├── components/
│   ├── __tests__/         # Component tests (*.test.ts)
│   ├── Avatar.svelte
│   ├── ContextMenu.svelte # Reusable right-click menu
│   ├── Modal.svelte       # Reusable dialog
│   ├── Toast.svelte       # Toast notification item
│   ├── ToastContainer.svelte
│   └── ...
├── stores/
│   ├── __tests__/         # Store tests (*.test.ts)
│   ├── auth.ts
│   ├── guilds.ts
│   ├── channels.ts
│   ├── messages.ts
│   ├── presence.ts
│   ├── typing.ts
│   ├── unreads.ts
│   ├── settings.ts
│   └── toasts.ts
└── types/
    └── index.ts           # Shared TypeScript interfaces
```

### Naming Conventions
- Components: PascalCase (`MessageItem.svelte`)
- Stores: camelCase (`messages.ts`)
- Tests: `ComponentName.test.ts` or `storeName.test.ts`
- Routes: kebab-case directories (`/guild/[id]/settings/roles/`)

---

## WebSocket Event Handling

All real-time updates flow through the gateway. The event handler in `stores/gateway.ts` dispatches to individual stores:

```ts
client.on((event, data) => {
  switch (event) {
    case 'MESSAGE_CREATE':
      appendMessage(data as Message);
      break;
    case 'MESSAGE_UPDATE':
      updateMessage(data as Message);
      break;
    case 'MESSAGE_DELETE':
      removeMessage(data as { channel_id: string; id: string });
      break;
  }
});
```

Never make API calls to "refresh" after receiving a WebSocket event — the event payload contains all needed data.

---

## Testing Standards

### Unit Tests (vitest)

Every component and store must have a corresponding test file:
```ts
// MessageItem.test.ts
import { render, fireEvent } from '@testing-library/svelte';
import MessageItem from './MessageItem.svelte';

describe('MessageItem', () => {
  it('renders message content', () => {
    const { getByText } = render(MessageItem, {
      props: { message: mockMessage }
    });
    expect(getByText('Hello world')).toBeInTheDocument();
  });

  it('shows edit option only for own messages', () => {
    // ...
  });
});
```

### E2E Tests (Playwright)

Critical user flows:
```ts
// tests/messaging.spec.ts
test('can send and edit a message', async ({ page }) => {
  await page.goto('/app');
  await page.fill('[data-testid="message-input"]', 'Hello');
  await page.keyboard.press('Enter');
  await expect(page.locator('.message-content')).toContainText('Hello');
});
```

### Test Data

Use factories for test data, never hardcode ULIDs:
```ts
function createMockMessage(overrides?: Partial<Message>): Message {
  return {
    id: crypto.randomUUID(),
    channel_id: 'test-channel',
    author_id: 'test-user',
    content: 'Test message',
    created_at: new Date().toISOString(),
    ...overrides,
  };
}
```
