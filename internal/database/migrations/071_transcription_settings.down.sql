DELETE FROM instance_settings
WHERE key IN (
    'transcription_engine_type',
    'transcription_engine_endpoint',
    'transcription_save_enabled'
);
