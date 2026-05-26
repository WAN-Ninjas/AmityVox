INSERT INTO activities (
    id, name, description, activity_type, icon_url, url,
    developer_id, max_participants, min_participants, category,
    public, verified, rating_sum, rating_count, created_at
) VALUES
    ('builtin-watch-together', 'Watch Together', 'Share a video URL and watch it with everyone in the channel.', 'watch_together', NULL, 'builtin://watch-together', NULL, 50, 1, 'entertainment', true, true, 0, 0, now()),
    ('builtin-music-party', 'Music Party', 'Share a music or stream URL with everyone in the channel.', 'music_party', NULL, 'builtin://music-party', NULL, 50, 1, 'entertainment', true, true, 0, 0, now()),
    ('builtin-shared-notes', 'Shared Notes', 'Keep lightweight shared notes for the current channel session.', 'custom', NULL, 'builtin://shared-notes', NULL, 50, 1, 'productivity', true, true, 0, 0, now())
ON CONFLICT (id) DO UPDATE SET
    name = EXCLUDED.name,
    description = EXCLUDED.description,
    activity_type = EXCLUDED.activity_type,
    url = EXCLUDED.url,
    category = EXCLUDED.category,
    public = EXCLUDED.public,
    verified = EXCLUDED.verified;
