-- Rollback migration 031: Admin Moderation - Rate Limiting, Content Scanning, CAPTCHA

DROP TABLE IF EXISTS content_scan_log;
DROP TABLE IF EXISTS content_scan_rules;
DROP TABLE IF EXISTS rate_limit_log;

DELETE FROM instance_settings WHERE key IN (
    'captcha_provider',
    'captcha_site_key',
    'captcha_secret_key',
    'rate_limit_requests_per_window',
    'rate_limit_window_seconds'
);
