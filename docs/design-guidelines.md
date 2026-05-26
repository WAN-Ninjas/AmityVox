# Design Guidelines

The AmityVox web client is an operational communication app. UI work should optimize for repeated daily use, scanning, and low-friction moderation/admin workflows.

## Product Feel

- Dense but readable.
- Clear hierarchy over decorative layout.
- Fast access to guilds, channels, DMs, notifications, moderation, and settings.
- Avoid marketing-style hero sections inside the authenticated app.
- Prefer familiar chat/admin patterns over novel controls.

## Layout

- Keep the main app shell stable: guild rail, channel/DM navigation, content pane, and contextual side panels.
- Avoid layout shifts when messages load, reactions update, members join, or voice state changes.
- Use modals and drawers for focused tasks; avoid nested cards.
- Make admin screens scan-friendly with tables, filters, segmented controls, and compact status summaries.

## Controls

- Use icon buttons for common actions when the icon is clear.
- Use text buttons for destructive or uncommon actions where clarity matters.
- Use toggles for binary settings.
- Use select/menu controls for fixed option sets.
- Use tabs for sibling views under one context.
- Use confirmation dialogs for destructive actions.

## Messaging UI

- Preserve message reading flow.
- Keep reaction, reply, thread, pin, report, and moderation actions reachable but not visually dominant.
- Display errors inline or as toasts depending on scope.
- Avoid blocking the whole app for one failed secondary request.

## Mobile/PWA

- Keep touch targets large enough for mobile.
- Use bottom sheets or full-screen panels for dense secondary views on narrow screens.
- Preserve primary chat actions near the bottom thumb zone.
- Test responsive behavior for admin and moderation screens, not only chat.

## Accessibility

- Keep visible focus states.
- Use semantic buttons and inputs.
- Do not rely on color alone for status.
- Keep text contrast high enough against dark and light themes.

## Themes

- Reuse existing theme variables and utility classes.
- Avoid one-off hardcoded colors unless a domain object requires it.
- Theme editor changes must not make system text unreadable.
