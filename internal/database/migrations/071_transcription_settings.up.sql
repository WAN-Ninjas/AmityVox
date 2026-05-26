INSERT INTO instance_settings (key, value, updated_at) VALUES
    ('transcription_engine_type', 'none', now()),
    ('transcription_engine_endpoint', '', now()),
    ('transcription_save_enabled', 'false', now())
ON CONFLICT (key) DO NOTHING;
