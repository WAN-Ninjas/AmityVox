# Current Sprint: Thread Redesign — Nested Sidebar Threads

## Goal

Make threads first-class sidebar citizens: they appear nested/indented beneath their parent channel with a distinct speech-bubble icon, can be hidden per-user, and channels gain a right-click "Show Threads" submenu with activity time filters.

## Changes

### Database (Migration 042)

- `channels.parent_channel_id` — direct FK from thread to parent channel
- `channels.last_activity_at` — updated on each message post, avoids expensive subqueries
- `user_hidden_threads` table — per-user thread hide preferences
- Backfill existing threads from `messages.thread_id`

### Backend

- **Model**: `ParentChannelID` and `LastActivityAt` added to `Channel` struct
- **All Channel SELECT/Scan queries updated** across `channels.go`, `guilds.go`, `users.go`
- **HandleCreateThread**: Sets `parent_channel_id` and `last_activity_at`
- **HandleGetThreads**: Uses `parent_channel_id` instead of JOIN through messages
- **HandleCreateMessage**: Updates `last_activity_at` for thread channels
- **New handlers**: `HandleHideThread`, `HandleUnhideThread`, `HandleGetHiddenThreads`
- **Routes**: Hide/unhide under `/channels/{id}/threads/{id}/hide`, hidden list under `/users/@me/hidden-threads`

### Frontend

- **Types**: `parent_channel_id` and `last_activity_at` on Channel interface
- **API Client**: `hideThread()`, `unhideThread()`, `getHiddenThreads()`
- **Channels Store**:
  - `textChannels` excludes threads (`!parent_channel_id`)
  - `threadsByParent` derived store: grouped by parent, sorted by activity DESC, excluding hidden
  - `hiddenThreadIds` writable store with optimistic updates
  - `getThreadActivityFilter()` / `setThreadActivityFilter()` — localStorage-based per-channel
- **Gateway**: Handles `THREAD_CREATE` event; updates `last_activity_at` on `MESSAGE_CREATE`
- **ChannelSidebar**: Threads render nested under parent with speech-bubble icon, thread context menu (hide/archive), channel context menu with "Show Threads" activity filter submenu
- **Guild Layout**: Loads hidden threads on guild init

### Tests

- Channel store tests updated for thread exclusion from `textChannels`
- `threadsByParent` grouping, sorting, hidden filtering
- Thread activity filter localStorage round-trip
- Per-channel filter independence

## Verification Checklist

- [ ] Migration 042 runs cleanly
- [ ] `HandleCreateThread` sets `parent_channel_id` and `last_activity_at`
- [ ] `HandleCreateMessage` updates `last_activity_at` for threads
- [ ] `HandleGetGuildChannels` returns threads with `parent_channel_id`
- [ ] Threads appear nested in sidebar with speech-bubble icon
- [ ] Right-click thread -> Hide Thread works
- [ ] Right-click channel -> Show Threads -> filter by time works
- [ ] Threads with unreads bypass activity filter
- [ ] Frontend tests pass
- [ ] Docker build passes with --no-cache
