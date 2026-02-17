ALTER TABLE voice_preferences
  DROP COLUMN IF EXISTS camera_resolution,
  DROP COLUMN IF EXISTS camera_framerate,
  DROP COLUMN IF EXISTS screenshare_resolution,
  DROP COLUMN IF EXISTS screenshare_framerate,
  DROP COLUMN IF EXISTS screenshare_audio;
