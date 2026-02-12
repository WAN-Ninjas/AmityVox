-- Rollback migration 037: Experimental features and Activities/Games framework

DROP TABLE IF EXISTS music_party_sessions CASCADE;
DROP TABLE IF EXISTS watch_together_sessions CASCADE;
DROP TABLE IF EXISTS game_leaderboard CASCADE;
DROP TABLE IF EXISTS game_players CASCADE;
DROP TABLE IF EXISTS game_sessions CASCADE;
DROP TABLE IF EXISTS activity_ratings CASCADE;
DROP TABLE IF EXISTS activity_participants CASCADE;
DROP TABLE IF EXISTS activity_sessions CASCADE;
DROP TABLE IF EXISTS activities CASCADE;
DROP TABLE IF EXISTS kanban_labels CASCADE;
DROP TABLE IF EXISTS kanban_cards CASCADE;
DROP TABLE IF EXISTS kanban_columns CASCADE;
DROP TABLE IF EXISTS kanban_boards CASCADE;
DROP TABLE IF EXISTS video_recordings CASCADE;
DROP TABLE IF EXISTS code_snippets CASCADE;
DROP TABLE IF EXISTS whiteboard_collaborators CASCADE;
DROP TABLE IF EXISTS whiteboards CASCADE;
DROP TABLE IF EXISTS voice_transcription_settings CASCADE;
DROP TABLE IF EXISTS voice_transcriptions CASCADE;
DROP TABLE IF EXISTS message_summaries CASCADE;
DROP TABLE IF EXISTS super_reactions CASCADE;
DROP TABLE IF EXISTS message_effects CASCADE;
DROP TABLE IF EXISTS location_shares CASCADE;
